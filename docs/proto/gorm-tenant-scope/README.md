# Prototype: GORM tenant auto-scoping (wayfinder #34)

Validates the #31 auto-scoping decision against Headscale's real query shapes.
Pure-Go SQLite (glebarez) + GORM `BeforeQuery` callback that injects
`WHERE tenant_id = ?`, with fail-closed + `WithAllTenants` escape hatch.

Run: `go run .`

## Coverage (verified)

| Query shape        | Verdict | Note |
|--------------------|---------|------|
| `Find` / `First`   | COVERED | callback injects `tenant_id` |
| `Preload`          | COVERED | preload is a separate query *through* the query callbacks → scoped, no leak |
| manual `.Joins`    | **GAP** | only the primary table is scoped; joined tenant table filtered only transitively (mis-attributed row leaks) |
| raw `.Raw`/`.Exec` | **GAP** | bypasses the statement builder; callback cannot inject |
| no tenant in ctx   | COVERED | fail-closed (query errors) |
| `WithAllTenants`   | COVERED | escape hatch returns all tenants |
| non-tenant model   | COVERED | table without `tenant_id` left untouched |

## Implication for #31's CI guard

Mechanism (a) covers the common paths incl. Preload. The `go/analysis` guard
must additionally FAIL the build on, unless annotated / `WithAllTenants`:
- `.Raw(` / `.Exec(` touching a tenant table, and
- `.Joins(` whose joined table carries `tenant_id` without an explicit
  `<joined>.tenant_id = ?` predicate.

The JOIN SQL emitted (proof):
`... FROM nodes JOIN routes ON routes.node_id = nodes.id WHERE nodes.tenant_id = 1`
— `routes.tenant_id` is NOT enforced.
