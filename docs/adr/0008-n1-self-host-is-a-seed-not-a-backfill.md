# ADR-0008: N=1 self-host is a seed, not a backfill

Status: Accepted (2026-08-12) · Decides [#32](https://github.com/ravencloak-org/RavenScale/issues/32) · Refines ADR-0003 · Map [#25](https://github.com/ravencloak-org/RavenScale/issues/25)

## Context

ADR-0003 makes self-host simply N=1 (a default Org + Tailnet, `tenant_id` never
null) so there is a single code path. Decision #5 originally described this as
"backfill `tenant_id` on every existing row." But RavenScale is greenfield and
multi-tenant from the first real schema (P0-2): every row is born with a
`tenant_id` stamped at the auth chokepoints (ADR-0007). Untenanted rows only
exist if we import a pre-existing single-tailnet Headscale deployment.

## Decision

Treat N=1 as an **idempotent first-boot seed**, not a data backfill.

- A migration seeds a **default Org + default Tailnet at fixed IDs** (Org 1 /
  Tailnet 1). Self-host (N=1) and multi-tenant SaaS run the **identical** seed —
  there is no separate single-tenant mode and no `if single_tenant` branch.
- RavenScale migrations are namespaced `rs-NNNN-<name>` in the `go-gormigrate`
  slice and pinned to run **after** upstream's schema migrations (gormigrate runs
  in slice order); each upstream merge keeps the `rs-*` block appended after
  theirs.
- **One-way** (no down function), each migration transaction-wrapped and
  re-runnable safe (seed via `FirstOrCreate`). **Fail-closed**: the default
  Org+Tailnet row is guaranteed before any `tenant_id NOT NULL` column is added,
  and the migration aborts its transaction on any row it cannot tenant.
- Cross-tenant/no-tenant migration work runs through `WithAllTenants(ctx)`
  (ADR-0007).

## Out of scope

**In-place import of an existing `juanfont/headscale` deployment → RavenScale.**
No Phase-0 deployments exist, so there are no legacy rows to backfill or IPs to
re-key (the `BackfillNodeIPs` concern from the IP-allocation research has nothing
to act on). Revisit as a fresh effort only if a real migrating customer appears.

## Consequences

- The migration surface is tiny: seed data plus the tenant columns.
- Because even self-host carries a non-null `tenant_id`, the ADR-0007 CI scoping
  guard stays meaningful on self-host.
- If Headscale-DB import is later needed, it is a new, one-way, versioned
  migration designed then — not something the current schema must anticipate.
