# ADR-0001 — Tenant model: Org owns Tailnets, row-level isolation

- Status: Accepted
- Date: 2026-07-27
- Deciders: Jobin, Monica (design grill session 1)

## Context

Headscale is single-tailnet: one flat namespace of users + nodes. P0-2 requires removing that limitation at the data-model level — the root dependency for nearly the whole backlog and the hardest thing to reverse once real data exists. We must fix (a) the tenant hierarchy and (b) where isolation lives, before any schema.

## Decision

- **Tenant = Organization**, owning one-or-more **Tailnets**. Model Org, Tailnet, User as three distinct concepts now, even though the launch UI exposes one Tailnet per Org.
- **Row-level `tenant_id` on every table, shared schema.** Not schema-per-tenant, not DB-per-tenant.

This mirrors Tailscale's own SaaS: one control plane serving many tailnets with logical (row-level) isolation on shared infrastructure; physical isolation is reserved for special gov/high-compliance deployments.

## Consequences

- An unused nullable Org FK today is free; retrofitting an Org layer after data exists would be a migration nightmare.
- Row-level keeps HA (P1-6), backup/restore (P0-3), and the k8s operator (P1-7) operationally simple; per-DB would multiply the cost of all three.
- **Ceiling:** DB-per-tenant is revisited only for a compliance customer needing physical isolation, or if noisy-neighbor bites. Recorded, not built.
- Enforcement of the row-level scheme is a separate, security-critical decision — see the app-layer scoping decision in CONTEXT.md §2.
