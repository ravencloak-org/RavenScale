# ADR-0007: Tenant scoping via an auto-scoping GORM session + CI guard

Status: Accepted (2026-08-12) · Decides [#31](https://github.com/ravencloak-org/RavenScale/issues/31), validated by [#34](https://github.com/ravencloak-org/RavenScale/issues/34) · Map [#25](https://github.com/ravencloak-org/RavenScale/issues/25)

## Context

Isolation is row-level on shared infrastructure (ADR-0001). The datastore is
libSQL/SQLite (ADR-0005), which has **no row-level security** — so tenant
scoping must be guaranteed entirely in application code. ADR-0006 (maintained
fork) requires that whatever we choose does not force rewrites of upstream's
hundreds of unscoped `db.Where(...)` call sites, or every upstream merge becomes
a war.

## Decision

Enforce scoping with a **context-scoped GORM session and a global auto-injecting
callback**, not a hand-written repository layer.

- `tenant_id` is bound into the request `context` at the auth seam — from the
  trusted OIDC tenant claim or the pre-auth key's tenant (the two chokepoints in
  Headscale's registration flow). A `TenantDB(ctx)` accessor returns a `*gorm.DB`
  whose registered `BeforeQuery`/`BeforeUpdate`/`BeforeDelete` callback appends
  `WHERE tenant_id = ?` to any tenant-table statement. Upstream call sites inherit
  scope without modification.
- **Fail-closed**: a tenant-table operation with no `tenant_id` in context
  **errors**; it never returns all tenants' rows.
- **Explicit escape hatch**: legitimate cross-tenant/no-tenant operations
  (IP-allocator seed, admin/ops, the N=1 seed) go through a greppable
  `WithAllTenants(ctx)`. Cross-tenant access is opt-in and auditable.
- **CI guard**: a custom `go/analysis` vet pass fails the build on (1) raw
  root-handle use outside the sanctioned `db` package and (2) un-annotated
  `WithAllTenants` calls. Full type-level unexporting of the root handle is
  deferred (it fights upstream's direct handle use).

## Coverage and the two guard gaps (validated in #34)

Prototype `proto/gorm-tenant-scope` confirmed the callback scopes `Find`/`First`
and — importantly — **`Preload`** (which runs as a separate query through the
same callbacks). It does **not** cover two shapes, so the CI guard must also fail
the build (unless annotated / `WithAllTenants`) on:

- `.Raw(` / `.Exec(` touching a tenant table (bypasses the statement builder), and
- `.Joins(` whose joined table carries `tenant_id` without an explicit
  `<joined>.tenant_id = ?` predicate (only the primary table is auto-scoped).

## Consequences

- Upstream merges stay clean (ADR-0006 honored) because scoping lives at one seam.
- Correctness rests on `tenant_id` always being in context on request paths and on
  the CI guard catching the raw-SQL and join gaps; both are enforced in code/CI,
  not the DB.
- Every request path must carry a tenant-bound context; background jobs and
  migrations must use `WithAllTenants` deliberately.
