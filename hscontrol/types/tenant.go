package types

import "gorm.io/gorm"

// RavenScale multi-tenancy scaffold.
//
// Isolation is row-level on shared infrastructure (ADR-0001): every
// tenant-scoped row carries a TenantID (the Org) and, where it lives in a
// network namespace, a TailnetID. Self-host is simply N=1 — a default Org +
// Tailnet at fixed ids, tenant_id never null (ADR-0003, ADR-0008), so there is
// exactly one code path.
const (
	// DefaultTenantID is the fixed Org id for the N=1 self-host seed (ADR-0008).
	DefaultTenantID uint = 1
	// DefaultTailnetID is the fixed Tailnet id for the N=1 self-host seed (ADR-0008).
	DefaultTailnetID uint = 1
)

// Tenant is the billing / isolation boundary — an "Org". It owns one or more
// Tailnets. Every tenant-scoped table refers to a Tenant via TenantID.
// See ADR-0001 (tenant model, row-level isolation).
type Tenant struct {
	gorm.Model

	Name string
}

// Tailnet is a single virtual network namespace within a Tenant: its own IP
// space (ADR-0002), DNS suffix and ACL policy. An Org owns one or more.
type Tailnet struct {
	gorm.Model

	// TenantID is the owning Org; never null (ADR-0001).
	TenantID uint `gorm:"not null;index"`

	// Slug is the immutable external handle, unique within a deployment (ADR-0004).
	// Numeric ID is internal; the slug is the stable namespace in node configs + DNS.
	Slug string `gorm:"uniqueIndex"`

	Name string
}

// SeedDefaultTenant ensures the N=1 default Org + Tailnet exist at their fixed
// ids. Idempotent (ADR-0008) — safe to run on every boot and re-run. Runs on
// the raw tx (no tenant scoping yet), i.e. the migration's own cross-tenant path.
func SeedDefaultTenant(tx *gorm.DB) error {
	tenant := Tenant{Model: gorm.Model{ID: DefaultTenantID}, Name: "default"}
	if err := tx.Where(Tenant{Model: gorm.Model{ID: DefaultTenantID}}).FirstOrCreate(&tenant).Error; err != nil {
		return err
	}

	tailnet := Tailnet{
		Model:    gorm.Model{ID: DefaultTailnetID},
		TenantID: DefaultTenantID,
		Slug:     "default",
		Name:     "default",
	}

	return tx.Where(Tailnet{Model: gorm.Model{ID: DefaultTailnetID}}).FirstOrCreate(&tailnet).Error
}
