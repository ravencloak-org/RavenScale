package db

import (
	"context"
	"errors"
	"testing"

	"github.com/juanfont/headscale/hscontrol/types"
)

// TestTenantScoping_ArmedFailsClosed verifies that a database created through
// NewHeadscaleDatabase (via newSQLiteTestDB) has the auto-scoping callbacks
// armed: a query on a tenant-scoped table with no tenant in context must fail
// closed, and the same query under DefaultTenantCtx must succeed.
func TestTenantScoping_ArmedFailsClosed(t *testing.T) {
	db, err := newSQLiteTestDB()
	if err != nil {
		t.Fatal(err)
	}

	var users []types.User
	err = db.DB.WithContext(context.Background()).Find(&users).Error
	if !errors.Is(err, ErrNoTenantInContext) {
		t.Fatalf("armed db must fail closed on no-tenant query, got %v", err)
	}

	// and with DefaultTenantCtx it must succeed:
	if err := db.DB.WithContext(DefaultTenantCtx()).Find(&users).Error; err != nil {
		t.Fatalf("DefaultTenantCtx query failed: %v", err)
	}
}
