# RavenScale — CONTEXT

Multi-tenant SaaS control plane built on Headscale (open-source Tailscale coordination server). This file captures resolved terminology and foundational design decisions from the design-grill sessions. Hard-to-reverse calls are recorded as ADRs under `docs/adr/`.

## Terminology (nailed)

| Term | Meaning |
|------|---------|
| **Org** (tenant) | The billing/isolation boundary. Owns one or more Tailnets. The `tenant_id` on every table refers to the Org. |
| **Tailnet** | A single virtual network namespace: its own IP space, DNS suffix, ACL policy, nodes, and users-in-network. An Org owns ≥1. |
| **User** | An identity that authenticates to the control plane and belongs to exactly one Org. Distinct concept from Org and Tailnet. |
| **Node** | A registered device (WireGuard peer). Belongs to exactly one Tailnet. |
| **Tenant boundary** | = Org. Isolation is logical (row-level), on shared infrastructure. |

Org / Tailnet / User are three distinct concepts from day one, even though the launch UI exposes exactly one Tailnet per Org.

## Resolved decisions (design grill, session 1 — Q1–Q8)

1. **Tenant model** — Tenant = Org, owns one-or-more Tailnets. Row-level `tenant_id`, shared schema. Mirrors Tailscale's shared-infra logical isolation. → ADR-0001
2. **Isolation enforcement** — App-layer mandatory scoping: all tenant-table access routes through one scoped repository; a CI guard fails the build on any unscoped access to a tenant table. SQLite is the backend (Turso/libSQL for horizontal scale later — same wire, still no RLS, so app-layer stays primary). Postgres RLS is *not* used (SQLite has none).
3. **IP address space** — Per-Tailnet independent `100.64.0.0/10`. Nodes in different Tailnets may hold identical addresses; they never route to each other. → ADR-0002
4. **Tenant resolution** — Internal user→Org mapping stamps a trusted `tenant_id` claim at auth. Nodes join via **tenant-scoped pre-auth keys** (the key encodes its Tailnet); the registration endpoint stays single, tenant is derived from the credential, never the URL. External per-tenant IdP is a later, additive phase.
5. **Single-tenant self-host** — No separate mode. Always multi-tenant with **N=1**: a default Org + Tailnet, `tenant_id` never null. Migration of existing Headscale installs = create the default Org+Tailnet and backfill `tenant_id` on every row (one-way, versioned). This IS P0-2. → ADR-0003
6. **Tailnet naming / MagicDNS** — Each Tailnet gets an **immutable slug** at creation; MagicDNS suffix is `<host>.<tailnet-slug>.<base-domain>`, base-domain per deployment. Numeric PK stays internal; the slug is the external handle. Rename is allowed but expensive (breaks DNS) and treated as a rare op. → ADR-0004
7. **ACL policy scope** — One HuJSON ACL policy per Tailnet. Users/groups/tags resolve only within their Tailnet; no cross-Tailnet references. Cross-Tailnet isolation is *structural* (separate IP space + row-level scoping), never expressed as an ACL rule — so a policy bug can never open a cross-tenant path.
8. **Node identity** — A node registers to exactly one Tailnet; its WireGuard pubkey is unique **within a Tailnet**, not globally. One device in two Tailnets is out of scope for Phase 1, but the node row already carries `tenant_id` + `tailnet_id`, so it is not blocked later.

## Known ceilings (deliberate, revisit-when)

- **DB-per-tenant** — deferred; revisit only if a customer needs physical isolation for compliance, or noisy-neighbor bites. Row-level keeps HA / backup-restore / k8s-operator simple.
- **External per-tenant IdP (OIDC/SSO)** — deferred to enterprise phase; additive, not hard to reverse.
- **Cross-Tailnet devices** — deferred; schema already carries the columns.

## Sources

Design grill session 1, Buzz channel `ravenscale-grill` (#d4978296-701f-4425-a5d5-d440e572ec93), 2026-07-27. Tailscale architecture claims are from public docs/architecture knowledge, not a live doc pull.
