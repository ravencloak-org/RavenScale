# Headscale IP Allocation — Current Mechanism & Per-Tailnet /10 Change

**Ticket:** #27 · **ADR:** 0002 / decision #3
**Research target:** upstream `github.com/juanfont/headscale` (`main`). RavenScale has no Headscale code yet.

## TL;DR

Headscale allocates node IPs from a **single, server-wide pool**. One `IPAllocator`
singleton holds every handed-out address across the entire deployment in one `usedIPs`
set, so `IPv4`/`IPv6` are effectively unique across all users. There is **no tenant
scoping** anywhere in the allocation path. Making each Tailnet own an independent
`100.64.0.0/10` keyed on `(tailnet_id, addr)` requires (a) making the allocator
per-tenant instead of a singleton, (b) scoping the "used IPs" query and the free/backfill
paths by `tenant_id`, and (c) replacing global address uniqueness with a composite
`UNIQUE(tenant_id, ipv4)` / `UNIQUE(tenant_id, ipv6)` constraint.

## Current mechanism (file / function pointers)

### 1. Config → prefixes (the pool definition)
`hscontrol/types/config.go`
- Config keys: `prefixes.v4`, `prefixes.v6`, `prefixes.allocation` (no `ip_prefixes` / base-IP key exists on `main`).
- `parsePrefixConfig(key, standardRange, family)` parses each prefix and flags it as
  "non-standard" if it falls outside the standard range. Standard ranges come from
  `tsaddr.CGNATRange()` (IPv4 `100.64.0.0/10`) and `tsaddr.TailscaleULARange()` (IPv6 ULA).
- At least one prefix is required (`ErrNoPrefixConfigured`).
- `IPAllocationStrategy` = `"sequential"` (default) or `"random"` (`ErrInvalidAllocationStrategy`).
- Result stored on `Config` as `PrefixV4 *netip.Prefix`, `PrefixV6 *netip.Prefix`, `IPAllocation IPAllocationStrategy`.
- **These prefixes are global.** One `PrefixV4` / `PrefixV6` for the whole server.

### 2. The allocator (a singleton)
`hscontrol/db/ip.go`
- `type IPAllocator struct` — comment: *"a singleton responsible for allocating IP
  addresses for nodes"*. Fields: `mu sync.Mutex`, `prefix4/prefix6 *netip.Prefix`,
  `prev4/prev6 netip.Addr`, `strategy`, and `usedIPs netipx.IPSetBuilder` (*"Set of all
  IPs handed out"*).
- `NewIPAllocator(db, prefix4, prefix6, strategy)` — reads **all** existing IPv4/IPv6
  addresses from the DB (no user/tenant filter), seeds `usedIPs` with them plus the
  network & broadcast addresses, and validates the set.
- `Next() (*netip.Addr, *netip.Addr, error)` — allocates one v4 and/or v6.
- `allocateNext(prev, prefix)` → locks `mu`, calls `next`, advances `prev`.
- `next(prev, prefix)` — walks forward from a start address, accepting an IP only when:
  the prefix contains it **AND** `usedIPs` does not **AND** it is not
  `isTailscaleReservedIP`; then adds it to `usedIPs`.
- `randomNext(pfx)` — random-strategy start point (handles `/32`,`/128`; `FillBytes` pads).
- `BackfillNodeIPs(i)` — write-txn over **all** nodes, allocating/removing IP families.
- `FreeIPs(ips)` — returns addresses to the pool.
- Sentinels: `errGeneratedIPBytesInvalid`, `errGeneratedIPNotInPrefix`, `errIPAllocatorNil`, `ErrCouldNotAllocateIP`.

### 3. Wiring (one instance per server)
`hscontrol/state/state.go`
- Built once in `NewState`:
  `ipAlloc, err := hsdb.NewIPAllocator(db, cfg.PrefixV4, cfg.PrefixV6, cfg.IPAllocation)`,
  stored on `State.ipAlloc *hsdb.IPAllocator` (*"ipAlloc manages IP address allocation for nodes"*).
- Registration path `createAndSaveNewNode`:
  `ipv4, ipv6, err := s.ipAlloc.Next()` → `nodeToRegister.IPv4 = ipv4; nodeToRegister.IPv6 = ipv6`.
- Deletion frees IPs: `s.ipAlloc.FreeIPs(node.IPs())`. Backfill: `s.db.BackfillNodeIPs(s.ipAlloc)`.

### 4. Node schema (no DB-level uniqueness, no tenant column)
`hscontrol/types/node.go`
- `IPv4 *netip.Addr `gorm:"column:ipv4;serializer:text"``
- `IPv6 *netip.Addr `gorm:"column:ipv6;serializer:text"``
- Neither field carries `unique`/`unique_index` (unlike `GivenName`). Uniqueness is
  enforced *only* in-process by the singleton's `usedIPs` set.
- Ownership: `UserID *uint` + `User *User` (or `Tags` for tagged nodes). **There is no
  tenant/namespace/Tailnet column** — "user" is the closest ownership concept and it is
  not used to scope allocation.

## Collision handling today

- **In-memory, global.** The singleton's `usedIPs` set is the sole collision guard: an
  address is skipped if already in the set. The set is seeded from every node in the DB at
  startup (`NewIPAllocator`), so it reflects the entire deployment, not a tenant.
- Network/broadcast + Tailscale-reserved addresses are pre-excluded.
- **Sequential** never wraps → past prefix end returns `ErrCouldNotAllocateIP`.
- **Random** picks a random start, scans deterministically, wraps within the prefix, and
  treats "returned to start" as exhaustion.
- No DB unique constraint backstops the in-memory set, and because the set is global,
  **two different users can never receive the same address** today.

## Concrete change for RavenScale (ADR-0002 / decision #3)

Goal: each Tailnet gets an independent `100.64.0.0/10`; allocation keyed on
`(tenant_id, addr)`; nodes in different Tailnets may hold identical addresses and never
route to each other.

1. **Add a tenant column + composite uniqueness** (`hscontrol/types/node.go` + migration):
   - Add `TenantID uint` (or reuse the Tailnet id) to `Node`.
   - Replace implicit global uniqueness with composite DB constraints:
     `UNIQUE(tenant_id, ipv4)` and `UNIQUE(tenant_id, ipv6)`. This is the DB-level
     expression of "keyed on `(tenant_id, addr)`" and lets identical addresses coexist
     across tenants.

2. **De-singleton the allocator** (`hscontrol/db/ip.go`):
   - Either (a) an allocator instance per tenant, or (b) a `TenantIPAllocator` map
     `tenant_id -> *IPAllocator` guarded by a mutex, lazily created on first allocation.
   - `NewIPAllocator` gains a `tenantID` and filters the seed query
     (`WHERE tenant_id = ?`) so `usedIPs` reflects only that tenant's addresses.
   - Every tenant reuses the **same** `PrefixV4` (`100.64.0.0/10`) and `PrefixV6`; the
     prefixes stay global config, only the *used-set* becomes per-tenant.

3. **Thread tenant through the call sites** (`hscontrol/state/state.go`):
   - `createAndSaveNewNode`: resolve the node's tenant, call
     `s.ipAllocFor(tenantID).Next()`.
   - `FreeIPs` / `BackfillNodeIPs`: scope by tenant so a freed address only returns to its
     own tenant's pool and backfill iterates per-tenant.

4. **Isolation invariant:** because each tenant allocates from its own `usedIPs` seeded
   only from its own nodes, address `100.64.0.5` in Tailnet A and Tailnet B are distinct
   `(tenant_id, addr)` rows; the mapper/route logic must never cross tenants, guaranteeing
   they never route to each other.

### Key upstream files to fork/modify
- `hscontrol/db/ip.go` — allocator: make per-tenant, filter seed query, `FreeIPs`/backfill scoping.
- `hscontrol/state/state.go` — wiring: per-tenant allocator lookup at `Next()`/free/backfill.
- `hscontrol/types/node.go` — add `TenantID`, composite unique constraints (+ migration).
- `hscontrol/types/config.go` — prefixes stay global; no change needed to the /10 default.
