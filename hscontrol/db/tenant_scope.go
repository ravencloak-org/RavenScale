package db

// Auto-scoping tenant session (ADR-0007 / CONTEXT decision 2).
//
// Every access to a tenant-scoped table (any model with a TenantID field)
// routes through GORM callbacks that inject `WHERE tenant_id = ?` from the
// request context. The rules, validated in tenant_scope_prototype.html:
//
//   - tenant in context      -> query/update/delete scoped to that Org;
//                               create stamps tenant_id.
//   - no tenant in context   -> FAIL CLOSED: the op errors, never runs
//                               unscoped. A dropped tenant leaks nothing.
//   - WithAllTenants(ctx)     -> explicit admin escape: no scope injected.
//                               Create still refuses unless tenant_id is set
//                               explicitly (cannot guess which Org owns a
//                               new row).
//
// Known gap: raw SQL and hand-rolled joins bypass model callbacks entirely.
// The CI scoping guard (P0-2 step 7) bans those; the callback cannot.
//
// This file is the mechanism only. Wiring it onto the production connection
// (and setting tenant context at the registration chokepoints) is step 5;
// until then existing plain-context queries are unaffected.

import (
	"context"
	"errors"

	"github.com/juanfont/headscale/hscontrol/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrNoTenantInContext is returned (fail-closed) when a tenant-scoped table is
// accessed but the context carries neither a tenant nor WithAllTenants.
var ErrNoTenantInContext = errors.New("tenant scope: no tenant in context (fail-closed)")

// ErrCannotCreateAllTenants is returned when a row is created under
// WithAllTenants without an explicit tenant_id: the Org to own it is ambiguous.
var ErrCannotCreateAllTenants = errors.New("tenant scope: cannot create under WithAllTenants without explicit tenant_id")

type tenantCtxKey struct{}

// tenantScope is what the context carries. all=true means WithAllTenants.
type tenantScope struct {
	tenantID uint
	all      bool
}

// WithTenant binds an Org (tenant) id to the context. All tenant-scoped ops on
// a session using this context are confined to that Org.
func WithTenant(ctx context.Context, tenantID uint) context.Context {
	return context.WithValue(ctx, tenantCtxKey{}, tenantScope{tenantID: tenantID})
}

// DefaultTenantCtx returns a context scoped to the N=1 default Org (ADR-0003).
// Interim seam for #36: production/test call sites that do not yet thread a
// real per-request tenant use this so the armed scoping session resolves to
// the single self-host Org. Replace with a real per-request WithTenant when
// N>1 request-boundary plumbing lands (phase 2-proper).
func DefaultTenantCtx() context.Context {
	return WithTenant(context.Background(), types.DefaultTenantID)
}

// WithAllTenants marks the context as an explicit cross-Org (admin) path: no
// tenant_id filter is injected. Use sparingly and greppably.
func WithAllTenants(ctx context.Context) context.Context {
	return context.WithValue(ctx, tenantCtxKey{}, tenantScope{all: true})
}

// tenantFromContext reports the bound tenant id, whether it is an all-tenants
// context, and whether any tenant scope is present at all.
func tenantFromContext(ctx context.Context) (tenantID uint, all bool, present bool) {
	if ctx == nil {
		return 0, false, false
	}
	s, ok := ctx.Value(tenantCtxKey{}).(tenantScope)
	if !ok {
		return 0, false, false
	}
	return s.tenantID, s.all, true
}

// isTenantScoped reports whether the statement's model has a TenantID column.
// Tables without one (Tenant itself, api_keys, migrations) are never scoped.
func isTenantScoped(db *gorm.DB) bool {
	return db.Statement.Schema != nil && db.Statement.Schema.LookUpField("TenantID") != nil
}

// RegisterTenantScoping installs the auto-scoping callbacks on a connection.
// After this call, tenant-scoped tables fail closed unless the session context
// carries WithTenant or WithAllTenants.
func RegisterTenantScoping(db *gorm.DB) error {
	q := db.Callback().Query()
	if err := q.Before("gorm:query").Register("tenant:scope_query", scopeRead); err != nil {
		return err
	}
	if err := db.Callback().Row().Before("gorm:row").Register("tenant:scope_row", scopeRead); err != nil {
		return err
	}
	if err := db.Callback().Update().Before("gorm:update").Register("tenant:scope_update", scopeRead); err != nil {
		return err
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("tenant:scope_delete", scopeRead); err != nil {
		return err
	}
	if err := db.Callback().Create().Before("gorm:create").Register("tenant:stamp_create", stampCreate); err != nil {
		return err
	}
	return nil
}

// scopeRead injects the tenant_id filter for query/update/delete, fail-closed.
func scopeRead(db *gorm.DB) {
	if !isTenantScoped(db) {
		return
	}
	tenantID, all, present := tenantFromContext(db.Statement.Context)
	if all {
		return // explicit admin escape: no scope
	}
	if !present {
		_ = db.AddError(ErrNoTenantInContext) // fail closed
		return
	}
	// Qualify tenant_id with the primary table so joined queries (e.g. a node
	// joined to pre_auth_keys, both tenant-scoped) do not error on an ambiguous
	// column. Falls back to the schema table name if Statement.Table is unset.
	table := db.Statement.Table
	if table == "" && db.Statement.Schema != nil {
		table = db.Statement.Schema.Table
	}
	db.Statement.AddClause(clause.Where{Exprs: []clause.Expression{
		clause.Eq{Column: clause.Column{Table: table, Name: "tenant_id"}, Value: tenantID},
	}})
}

// stampCreate stamps tenant_id on new rows, fail-closed.
func stampCreate(db *gorm.DB) {
	if !isTenantScoped(db) {
		return
	}
	tenantID, all, present := tenantFromContext(db.Statement.Context)

	// Respect an explicitly-set tenant_id (seed rows, admin creating for a
	// chosen Org). ponytail: single-struct check; batch inserts revisit at
	// step 5 when the create chokepoints are known.
	field := db.Statement.Schema.LookUpField("TenantID")
	if _, isZero := field.ValueOf(db.Statement.Context, db.Statement.ReflectValue); !isZero {
		return // caller chose the Org
	}

	if all {
		_ = db.AddError(ErrCannotCreateAllTenants)
		return
	}
	if !present {
		_ = db.AddError(ErrNoTenantInContext)
		return
	}
	db.Statement.SetColumn("tenant_id", tenantID)
}
