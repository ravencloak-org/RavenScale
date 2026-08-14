# Headscale GORM dialect + migrations on libSQL/SQLite (Ticket #29)

Research target: upstream `github.com/juanfont/headscale` @ `565fd25` (2026-07-29, post-v0.25).
All file pointers are into upstream `hscontrol/db/`.

## TL;DR

- Headscale's DB layer is GORM with a two-dialect switch: **SQLite** (`github.com/glebarez/sqlite`, pure-Go, no CGO) and **Postgres** (`gorm.io/driver/postgres`). No other dialect is accepted.
- Migrations are **explicit versioned migrations** via `github.com/go-gormigrate/gormigrate/v2` (ordered list of timestamp-ID steps), *not* bare `AutoMigrate`. `AutoMigrate` is used only inside `InitSchema` (fresh-DB fast path) and inside a few individual migration steps.
- libSQL is SQLite-wire/file compatible, so a **local libSQL file works through the existing glebarez/SQLite path**. A *remote* libSQL/Turso (sqld) server would need a new GORM driver — it is not wired in today.
- SQLite (and libSQL) has **no RLS** and Headscale does **not** use Postgres RLS either — scoping is entirely application-layer (`WHERE user_id = ?`). The CI scoping guard must be a query/code guard, not a DB-enforced one.
- `nodes.machine_key` / `nodes.node_key` have **no unique index** in the schema — pubkey uniqueness is app-enforced, not DB-enforced.

## 1. Dialect abstraction

`openDB(cfg types.DatabaseConfig)` in `hscontrol/db/db.go` (~line 1046) switches on `cfg.Type`:

- `types.DatabaseSqlite` → `gorm.Open(sqlite.Open(connectionURL), ...)` where `sqlite` is `github.com/glebarez/sqlite` (pure-Go, `modernc.org/sqlite` under the hood — no cgo). Connection pool is pinned to `SetMaxIdleConns(1)` / `SetMaxOpenConns(1)` because the pure-Go lib does not share the C lib's locking model.
- `types.DatabasePostgres` → `gorm.Open(postgres.Open(dbString), ...)` (`gorm.io/driver/postgres`).
- anything else → `errDatabaseNotSupported`.

Driver versions (`go.mod`): `glebarez/sqlite v1.11.0`, `gorm.io/driver/postgres v1.6.0`, `gorm.io/gorm v1.31.1`, `go-gormigrate/gormigrate/v2 v2.1.6`, `tailscale/squibble v0.0.0-20260411…`.

SQLite pragmas are set at connection time via a URL built from `sqliteconfig.Default()` (`hscontrol/db/sqliteconfig/config.go`):
`BusyTimeout`, `JournalMode=WAL`, `AutoVacuum=INCREMENTAL`, `Synchronous=NORMAL`, `ForeignKeys=true`, `TxLock=IMMEDIATE`, `WALAutocheckpoint=1000`.

## 2. Migration mechanism

Entry point: `NewHeadscaleDatabase(...)` in `hscontrol/db/db.go`:

1. `checkVersionUpgradePath(dbConn)` — refuses illegal jumps (migrations start at v0.25.0; a v0.24.x DB must first pass through v0.25.1). See `hscontrol/db/versioncheck.go`.
2. Build `gormigrate.New(dbConn, gormigrate.DefaultOptions, []*gormigrate.Migration{ ... })` — the migration list (~line 66). Each entry is `{ ID: "YYYYMMDDhhmm[-slug]", Migrate: func(tx *gorm.DB) error, Rollback: ... }`. New migrations are appended to the end. Steps use dialect-neutral GORM migrator calls (`tx.Migrator().AddColumn/RenameColumn/HasColumn/DropTable/DropIndex`, `tx.AutoMigrate(&Model{})`, or raw SQL) so the same list runs on both dialects; a few steps branch on `tx.Name() != "sqlite"`.
3. `migrations.InitSchema(func(tx) { ... })` — the **fresh-DB fast path**. On an empty DB gormigrate runs this instead of replaying every step: `tx.AutoMigrate` for all model structs (`User, PreAuthKey, APIKey, Node, Policy, OAuthClient, OAuthAccessToken`), then explicit `DROP INDEX IF EXISTS` + `CREATE [UNIQUE] INDEX` to normalize index DDL to match `schema.sql` (no backticks, correct partial-index predicates).
4. `runMigrations(cfg, dbConn, migrations)` — see §3.
5. On success, `setDatabaseVersion(...)` stores the running binary version (skipped for dev builds).
6. **SQLite only**: `squibble.Validate(ctx, sqlConn, dbSchema, opts)` compares the live schema against the embedded golden schema (`//go:embed schema.sql` → `dbSchema`), ignoring litestream tables. This is the source-of-truth check and does **not** run on Postgres (squibble is SQLite-only).

Migration IDs currently in the list (v0.25.0 → head), in order:
`202501221827, 202501311657, 202502070949, 202502131714, 202502171819, 202505091439, 202505141324, 202507021200, 202510311551, 202511101554-drop-old-idx, 202511011637-preauthkey-bcrypt, 202511122344-remove-newline-index, 202511131445-node-forced-tags-to-tags, 202601121700-migrate-hostinfo-request-tags, 202602201200-clear-tagged-node-user-id, 202605221435-clear-zero-time-node-expiry, 202606181200-recover-null-tags-node-user-id, 202606191500-api-key-user-id, 202606191501-pre-auth-key-description, 202606201200-pre-auth-key-revoked, 202606211200-oauth-clients-and-tokens, 202607241200-clear-tagged-node-expiry`.

Several of these (`clear-tagged-node-user-id`, `recover-null-tags-node-user-id`, `api-key-user-id`) are **data-backfill migrations** that set/clear `user_id` — direct precedent for an N=1 tenant/user backfill migration: model it as an appended timestamp-ID step doing `UPDATE ... SET user_id = ?` via raw SQL or GORM, idempotent (guarded by `HasColumn`/value checks), with a no-op `Rollback`.

### 3. Startup run order (`runMigrations`)

- **SQLite**: `PRAGMA foreign_keys = OFF` → `migrations.MigrateTo("202501311657")` (the early route/pre-auth-key automigrations that GORM can't do safely with FKs on) → `PRAGMA foreign_keys = ON` → `migrations.Migrate()` (everything else) → `PRAGMA foreign_key_check` and abort if any violation. Comment in-code: **no new migration may run with FK disabled.**
- **Postgres**: single `migrations.Migrate()` (no FK toggling needed).

gormigrate records applied IDs in a `migrations` bookkeeping table and skips already-applied steps, so this is safe/idempotent across restarts.

## 4. libSQL / SQLite fit + caveats

**Fit.** libSQL is a SQLite fork that is file- and wire-compatible. Everything Headscale's SQLite path relies on — WAL journal mode, `PRAGMA foreign_keys`, `PRAGMA foreign_key_check`, partial (`WHERE`) unique indexes, `AUTOINCREMENT` — is supported by libSQL. A **local libSQL database file opened through `glebarez/sqlite` behaves as plain SQLite**, so ADR-0005's dialect abstraction holds with zero GORM changes for the embedded/local case.

**Caveats / risks:**

1. **Driver, not just wire format.** `glebarez/sqlite` embeds `modernc.org/sqlite`; it is not the libSQL client. To talk to a *remote* libSQL/Turso `sqld` server (HTTP/gRPC, embedded replicas) you would need a libSQL GORM driver (e.g. wrap `github.com/tursodatabase/go-libsql`) and register it as a third dialect in `openDB`. Not present upstream — treat remote libSQL as new work.
2. **squibble validation is SQLite-only and strict.** It digests the live schema and compares to embedded `schema.sql`. libSQL must produce byte-identical schema DDL/digest; any libSQL-specific system table or DDL normalization difference will fail startup validation. Validate the digest early on the target libSQL build. (litestream tables are already ignored; libSQL-specific internal tables may need adding to `IgnoreTables`.)
3. **No RLS anywhere.** SQLite/libSQL has no row-level security, and Headscale does not use Postgres RLS. Tenant/user **scoping is 100% application-layer** (`WHERE user_id = ?` in the query builders). The CI scoping guard therefore has to be a static/lint/query-interception check over the Go data layer — it cannot lean on the database to enforce isolation. Note `nodes.user_id` is **nullable** (NULL == tagged node owned by a tag, not a user), so the guard must treat NULL `user_id` as a distinct, intentional case rather than a missing filter.
4. **Unique-index / uniqueness behaviour.**
   - `nodes.machine_key` and `nodes.node_key` have **no unique index** (`schema.sql`): pubkey uniqueness is enforced in application code, not the DB. If RavenScale wants DB-level pubkey uniqueness, that's a new (partial) unique index migration — and SQLite/libSQL treat **multiple NULLs as distinct** in a unique index, so a nullable key column needs a partial `WHERE key IS NOT NULL` predicate (the pattern already used for users/pre-auth-keys).
   - Users rely on **partial unique indexes**: `idx_provider_identifier … WHERE provider_identifier IS NOT NULL`, `idx_name_no_provider_identifier ON users(name) WHERE provider_identifier IS NULL`, plus `idx_name_provider_identifier ON users(name, provider_identifier)`. Partial indexes are fully supported by libSQL.
   - **Collation is binary/case-sensitive** — there is no `COLLATE NOCASE` in the schema. Uniqueness is exact-byte. libSQL matches SQLite's default `BINARY` collation, so no behavioural drift; but any future case-insensitive uniqueness needs an explicit `COLLATE NOCASE` index (works identically on both).

## 5. Implications for RavenScale ADR-0005

- Keep the GORM two-dialect abstraction; local libSQL rides the existing SQLite driver unchanged. Remote/Turso libSQL = add a dialect in `openDB` + a driver dep.
- Model the N=1 backfill as an appended `gormigrate` step (timestamp ID, idempotent `UPDATE`, no-op rollback) — mirrors upstream `*-node-user-id` backfills. It will run with FK ON (post-`202501311657`), so it must satisfy FK constraints and pass the final `foreign_key_check`.
- CI scoping guard = application-layer static check (no RLS to lean on) and must tolerate legitimately-NULL `user_id` (tagged nodes).
- If DB-level pubkey uniqueness is desired, add a partial unique index (`WHERE … IS NOT NULL`) and update `schema.sql`, or squibble validation will fail.
