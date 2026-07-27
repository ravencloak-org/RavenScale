# ADR-0003 — Single-tenant self-host is N=1, not a separate mode

- Status: Accepted
- Date: 2026-07-27
- Deciders: Jobin, Monica (design grill session 1)

## Context

Headscale's existing user base self-hosts a single tailnet. We could keep that as a distinct code path (skip tenant scoping when unconfigured) or fold it into the multi-tenant model as one tenant.

## Decision

**Always multi-tenant, N=1.** Self-host = one default Org + Tailnet; `tenant_id` is never null; the UI hides the Org layer. There is exactly one code path.

Migration of existing installs (this IS **P0-2**): create the default Org + Tailnet, then backfill `tenant_id` on every existing row. One-way, versioned migration.

## Consequences

- No divergent single-tenant path to maintain — two code paths are the thing you can never re-merge later.
- Every query is tenant-scoped in dev and prod alike, so the CI scoping guard (CONTEXT.md §2) is meaningful even for self-host.
- The migration is a schema-and-data change on live installs → must be idempotent, reversible-forward-only, and covered by tests before release.
