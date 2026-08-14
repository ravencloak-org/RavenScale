package a

import "gorm.io/gorm"

// elem is a non-gorm type with a Raw method, standing in for the HTML
// templating helper (elem.Raw). tenantcheck must NOT flag it even when the
// string mentions a tenant table name.
type elem struct{}

func (elem) Raw(s string) elem { return elem{} }

func flagged(db *gorm.DB) {
	var out []int

	// 1. Raw SQL on a tenant table -> flagged.
	db.Raw("SELECT * FROM nodes WHERE id = ?", 1).Scan(&out) // want `raw SQL references tenant-scoped table "nodes"`

	// 2. Join onto a tenant table -> flagged.
	db.Joins("JOIN users u ON u.id = x.user_id").Find(&out) // want `join references tenant-scoped table "users"`
}

func suppressed(db *gorm.DB) {
	var out []int

	// 3a. Allow directive on the same line -> suppressed.
	db.Exec("DELETE FROM policies WHERE id = 1") //tenantcheck:allow one-off cleanup

	// 3b. Allow directive on the line directly above -> suppressed.
	//tenantcheck:allow bulk maintenance
	db.Raw("SELECT * FROM tailnets").Scan(&out)
}

func clean(db *gorm.DB) {
	var out []int

	// 4. A normally-scoped query (no Raw/Exec/Joins) -> never flagged.
	db.Where("id = ?", 1).Find(&out)

	// 5. Raw SQL on a NON-tenant table (api_keys) -> never flagged.
	db.Raw("SELECT COUNT(*) FROM api_keys").Scan(&out)

	// 6. Non-gorm Raw with a tenant table name -> never flagged (wrong receiver).
	var e elem
	e.Raw("<svg>nodes</svg>")
}
