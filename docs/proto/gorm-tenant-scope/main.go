// Prototype for wayfinder #34: does an auto-injecting GORM callback scope
// tenant_id across Headscale's real query shapes? Fail-closed + escape hatch.
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/glebarez/sqlite" // pure-Go SQLite, same as Headscale (#29)
	"gorm.io/gorm"
	"gorm.io/gorm/callbacks"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

type ctxKey string

const tenantKey ctxKey = "tenant_id"
const allTenantsKey ctxKey = "all_tenants"

// WithTenant binds a tenant to the request context.
func WithTenant(ctx context.Context, id uint) context.Context {
	return context.WithValue(ctx, tenantKey, id)
}

// WithAllTenants is the explicit, greppable escape hatch (#31).
func WithAllTenants(ctx context.Context) context.Context {
	return context.WithValue(ctx, allTenantsKey, true)
}

// --- models. tenant-scoped ones carry TenantID. ---
type Node struct {
	ID         uint `gorm:"primarykey"`
	TenantID   uint `gorm:"index"`
	TailnetID  uint
	MachineKey string
	Routes     []Route // for Preload test
}
type Route struct {
	ID       uint `gorm:"primarykey"`
	TenantID uint `gorm:"index"`
	NodeID   uint
	Prefix   string
}
type Global struct { // NOT tenant-scoped — callback must skip it
	ID  uint `gorm:"primarykey"`
	Key string
}

// tenantScope is the BeforeQuery/Update/Delete callback.
func tenantScope(db *gorm.DB) {
	if db.Statement.Schema == nil {
		return // raw SQL — no schema, cannot inject (documented gap)
	}
	// only scope models that actually have a tenant_id column
	field := db.Statement.Schema.LookUpField("TenantID")
	if field == nil {
		return // e.g. Global — not tenant-scoped, leave alone
	}
	if allow, _ := db.Statement.Context.Value(allTenantsKey).(bool); allow {
		return // escape hatch
	}
	tid, ok := db.Statement.Context.Value(tenantKey).(uint)
	if !ok {
		// FAIL CLOSED: tenant-table access with no tenant in context.
		_ = db.AddError(errors.New("tenantScope: no tenant_id in context (fail-closed)"))
		return
	}
	db.Statement.AddClause(clause.Where{Exprs: []clause.Expression{
		clause.Eq{Column: clause.Column{Table: db.Statement.Table, Name: "tenant_id"}, Value: tid},
	}})
}

var captured []string // capture SQL for inspection

func sqlOf(db *gorm.DB, fn func(tx *gorm.DB) *gorm.DB) (string, error) {
	stmt := fn(db.Session(&gorm.Session{DryRun: true}))
	if stmt.Error != nil {
		return "", stmt.Error
	}
	return db.Dialector.Explain(stmt.Statement.SQL.String(), stmt.Statement.Vars...), nil
}

func main() {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	// register the scope callback on the query/update/delete processors
	_ = db.Callback().Query().Before("gorm:query").Register("scope:query", tenantScope)
	_ = db.Callback().Update().Before("gorm:update").Register("scope:update", tenantScope)
	_ = db.Callback().Delete().Before("gorm:delete").Register("scope:delete", tenantScope)
	_ = callbacks.RegisterDefaultCallbacks // keep import
	_ = schema.ErrUnsupportedDataType      // keep import

	db.AutoMigrate(&Node{}, &Route{}, &Global{})
	// seed two tenants
	root := db.Session(&gorm.Session{Context: WithAllTenants(context.Background())})
	root.Create(&Node{TenantID: 1, TailnetID: 1, MachineKey: "mkA", Routes: []Route{{TenantID: 1, Prefix: "10.0.0.0/24"}}})
	root.Create(&Node{TenantID: 2, TailnetID: 2, MachineKey: "mkB", Routes: []Route{{TenantID: 2, Prefix: "10.9.0.0/24"}}})
	root.Create(&Global{Key: "cluster-secret"})

	t1 := db.Session(&gorm.Session{Context: WithTenant(context.Background(), 1)})

	line := func(name, verdict, detail string) {
		fmt.Printf("%-26s %-8s %s\n", name, verdict, detail)
	}
	fmt.Println("CASE                       VERDICT  DETAIL")
	fmt.Println(strings.Repeat("-", 78))

	// 1. plain Find
	{
		var ns []Node
		t1.Find(&ns)
		sql, _ := sqlOf(db, func(tx *gorm.DB) *gorm.DB { return tx.Session(&gorm.Session{Context: WithTenant(context.Background(), 1)}).Find(&[]Node{}) })
		ok := len(ns) == 1 && strings.Contains(sql, "tenant_id")
		line("1 Find", verdict(ok), fmt.Sprintf("rows=%d scoped=%v", len(ns), strings.Contains(sql, "tenant_id")))
	}
	// 2. Preload (separate query through query callbacks)
	{
		var ns []Node
		t1.Preload("Routes").Find(&ns)
		leak := 0
		for _, n := range ns {
			for _, r := range n.Routes {
				if r.TenantID != 1 {
					leak++
				}
			}
		}
		line("2 Preload", verdict(leak == 0 && len(ns) == 1), fmt.Sprintf("nodes=%d preload_leak_rows=%d", len(ns), leak))
	}
	// 3. Joins to a tenant table (secondary table not auto-scoped)
	{
		sql, _ := sqlOf(db, func(tx *gorm.DB) *gorm.DB {
			return tx.Session(&gorm.Session{Context: WithTenant(context.Background(), 1)}).
				Model(&Node{}).Joins("JOIN routes ON routes.node_id = nodes.id").Find(&[]Node{})
		})
		joinedScoped := strings.Count(sql, "tenant_id") >= 2 // both nodes+routes
		line("3 Joins", verdict(joinedScoped), fmt.Sprintf("tenant_id_occurrences=%d (need>=2)", strings.Count(sql, "tenant_id")))
	}
	// 4. Raw SQL (bypasses statement builder)
	{
		var ns []Node
		t1.Raw("SELECT * FROM nodes").Scan(&ns)
		line("4 Raw SQL", verdict(len(ns) == 1), fmt.Sprintf("rows=%d (2=UNSCOPED leak)", len(ns)))
	}
	// 5. Fail-closed: no tenant in context
	{
		none := db.Session(&gorm.Session{Context: context.Background()})
		var ns []Node
		err := none.Find(&ns).Error
		line("5 Fail-closed", verdict(err != nil && len(ns) == 0), fmt.Sprintf("err=%v rows=%d", err != nil, len(ns)))
	}
	// 6. Escape hatch
	{
		all := db.Session(&gorm.Session{Context: WithAllTenants(context.Background())})
		var ns []Node
		all.Find(&ns)
		line("6 WithAllTenants", verdict(len(ns) == 2), fmt.Sprintf("rows=%d (expect 2)", len(ns)))
	}
	// 7. non-tenant model (Global) must not be scoped/blocked
	{
		none := db.Session(&gorm.Session{Context: context.Background()})
		var g []Global
		err := none.Find(&g).Error
		line("7 Global (untenanted)", verdict(err == nil && len(g) == 1), fmt.Sprintf("err=%v rows=%d", err != nil, len(g)))
	}
}

func verdict(ok bool) string {
	if ok {
		return "COVERED"
	}
	return "GAP"
}
