package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// test-only models: one tenant-scoped (has TenantID), one not.
type scopedThing struct {
	ID       uint
	TenantID uint
	Name     string
}

type unscopedThing struct {
	ID   uint
	Name string
}

func newScopeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "scope.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&scopedThing{}, &unscopedThing{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Seed with explicit tenant_id (bypass stamp): tenant 1 owns 1,2; tenant 2 owns 3,4.
	seed := []scopedThing{
		{ID: 1, TenantID: 1, Name: "alice"},
		{ID: 2, TenantID: 1, Name: "bob"},
		{ID: 3, TenantID: 2, Name: "carol"},
		{ID: 4, TenantID: 2, Name: "erin"},
	}
	if err := db.Create(&seed).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := RegisterTenantScoping(db); err != nil {
		t.Fatalf("register: %v", err)
	}
	return db
}

func TestTenantScope_ScopedRead(t *testing.T) {
	db := newScopeTestDB(t)
	ctx := WithTenant(context.Background(), 1)

	var got []scopedThing
	if err := db.WithContext(ctx).Find(&got).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("tenant 1 saw %d rows, want 2: %+v", len(got), got)
	}
	for _, r := range got {
		if r.TenantID != 1 {
			t.Fatalf("leaked foreign row: %+v", r)
		}
	}
}

func TestTenantScope_FailClosed(t *testing.T) {
	db := newScopeTestDB(t)

	var got []scopedThing
	err := db.WithContext(context.Background()).Find(&got).Error
	if !errors.Is(err, ErrNoTenantInContext) {
		t.Fatalf("want ErrNoTenantInContext, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("fail-closed must return no rows, got %d", len(got))
	}
}

func TestTenantScope_AllTenants(t *testing.T) {
	db := newScopeTestDB(t)
	ctx := WithAllTenants(context.Background())

	var got []scopedThing
	if err := db.WithContext(ctx).Find(&got).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("WithAllTenants saw %d rows, want 4", len(got))
	}
}

func TestTenantScope_CrossTenantUpdateIsNoOp(t *testing.T) {
	db := newScopeTestDB(t)
	ctx := WithTenant(context.Background(), 1) // tenant 1 tries to touch tenant 2's row (id=3)

	res := db.WithContext(ctx).Model(&scopedThing{}).Where("id = ?", 3).Update("name", "hijacked")
	if res.Error != nil {
		t.Fatalf("update: %v", res.Error)
	}
	if res.RowsAffected != 0 {
		t.Fatalf("cross-tenant update affected %d rows, want 0", res.RowsAffected)
	}

	// confirm ground truth unchanged, via all-tenants read.
	var row scopedThing
	db.WithContext(WithAllTenants(context.Background())).First(&row, 3)
	if row.Name != "carol" {
		t.Fatalf("foreign row mutated: %+v", row)
	}
}

func TestTenantScope_CrossTenantDeleteIsNoOp(t *testing.T) {
	db := newScopeTestDB(t)
	ctx := WithTenant(context.Background(), 1)

	res := db.WithContext(ctx).Where("id = ?", 3).Delete(&scopedThing{})
	if res.Error != nil {
		t.Fatalf("delete: %v", res.Error)
	}
	if res.RowsAffected != 0 {
		t.Fatalf("cross-tenant delete affected %d rows, want 0", res.RowsAffected)
	}
}

func TestTenantScope_CreateStampsTenant(t *testing.T) {
	db := newScopeTestDB(t)
	ctx := WithTenant(context.Background(), 2)

	row := scopedThing{Name: "dave"} // no TenantID set
	if err := db.WithContext(ctx).Create(&row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if row.TenantID != 2 {
		t.Fatalf("create stamped tenant_id=%d, want 2", row.TenantID)
	}
}

func TestTenantScope_CreateFailClosed(t *testing.T) {
	db := newScopeTestDB(t)

	row := scopedThing{Name: "nobody"}
	err := db.WithContext(context.Background()).Create(&row).Error
	if !errors.Is(err, ErrNoTenantInContext) {
		t.Fatalf("want ErrNoTenantInContext, got %v", err)
	}
}

func TestTenantScope_CreateUnderAllTenantsRefusesWithoutExplicit(t *testing.T) {
	db := newScopeTestDB(t)
	ctx := WithAllTenants(context.Background())

	row := scopedThing{Name: "ambiguous"} // no tenant set -> which Org?
	err := db.WithContext(ctx).Create(&row).Error
	if !errors.Is(err, ErrCannotCreateAllTenants) {
		t.Fatalf("want ErrCannotCreateAllTenants, got %v", err)
	}
}

func TestTenantScope_CreateUnderAllTenantsAllowsExplicit(t *testing.T) {
	db := newScopeTestDB(t)
	ctx := WithAllTenants(context.Background())

	row := scopedThing{TenantID: 2, Name: "admin-chose"} // explicit Org
	if err := db.WithContext(ctx).Create(&row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if row.TenantID != 2 {
		t.Fatalf("explicit tenant_id overwritten: %+v", row)
	}
}

func TestTenantScope_UnscopedTableUnaffected(t *testing.T) {
	db := newScopeTestDB(t)
	// no tenant context at all: a table without TenantID must still work.
	if err := db.WithContext(context.Background()).Create(&unscopedThing{Name: "x"}).Error; err != nil {
		t.Fatalf("unscoped create failed: %v", err)
	}
	var got []unscopedThing
	if err := db.WithContext(context.Background()).Find(&got).Error; err != nil {
		t.Fatalf("unscoped find failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("unscoped find got %d, want 1", len(got))
	}
}
