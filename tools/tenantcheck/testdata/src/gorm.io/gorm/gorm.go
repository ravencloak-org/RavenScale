// Package gorm is a minimal stub of gorm.io/gorm for analysistest fixtures.
// It provides just enough of the *DB chaining surface for tenantcheck to
// resolve receiver types; it is never executed.
package gorm

// DB mirrors the subset of gorm.io/gorm.DB that the fixtures call.
type DB struct{}

func (db *DB) Raw(sql string, values ...interface{}) *DB { return db }

func (db *DB) Exec(sql string, values ...interface{}) *DB { return db }

func (db *DB) Joins(query string, args ...interface{}) *DB { return db }

func (db *DB) Where(query interface{}, args ...interface{}) *DB { return db }

func (db *DB) Find(dest interface{}, conds ...interface{}) *DB { return db }

func (db *DB) First(dest interface{}, conds ...interface{}) *DB { return db }

func (db *DB) Scan(dest interface{}) *DB { return db }
