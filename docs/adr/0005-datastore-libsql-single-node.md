# ADR-0005 — Datastore: libSQL single-node (no cluster) for Phase 0–1

- Status: Accepted
- Date: 2026-07-28
- Deciders: Jobin, Monica (design grill, datastore thread)

## Context

The orchestrator brief specified **Postgres** for `P0-3` (#3). The design grill (CONTEXT.md §2) had instead leaned **SQLite / libSQL** with app-layer tenant scoping and no RLS. This was the one open conflict blocking `P0-3`.

A RavenScale control plane is **read-heavy and low-write** — nodes poll map updates constantly; writes are only registrations and config changes. Single-node SQLite/libSQL comfortably serves that load. The heavy machinery (Postgres clustering, or a SQLite-clustering stack like rqlite / LiteFS / dqlite) buys automatic multi-node failover we do not need while proving the product.

## Decision

**Back RavenScale with libSQL (`sqld`), single node, plus WAL→S3 backup, for Phases 0–1. No clustering.**

- Tenant isolation stays **app-layer** (ADR-0001 direction; no RLS — SQLite has none), guarded by the CI check on unscoped tenant-table access.
- Add a read replica only if read load actually climbs.
- **Revisit at the HA milestone (#11):** if auto-failover HA becomes a hard requirement, either adopt a SQLite-clustering stack (LiteFS/rqlite) or migrate to Postgres + a mature operator (CloudNativePG/Patroni) then — with real load data.

## Consequences

- Lightest possible ops now: one process, one volume, object-storage backup. No distributed-DB operational burden while the product is unproven.
- **This call is reversible** — unlike the tenancy schema (ADR-0001). GORM abstracts the SQL dialect and upstream Headscale already supports both SQLite and Postgres, so a later engine switch is a contained migration, not a rewrite. That is *why* it is safe to pick lean now.
- HA (#11) and backup/restore (#3) design against single-writer libSQL semantics: backup = WAL streaming to S3 + volume snapshots; "HA" at this stage = fast restore, not automatic failover.
- Supersedes the brief's Postgres line for `P0-3`; the brief and BUILD_ORDER are updated to match.

## Alternatives considered

- **Postgres + CloudNativePG now** — proven auto-failover HA, PITR, RLS as defense-in-depth. Rejected for now: ops overhead unjustified pre-product-fit; revisit at #11 if the wedge becomes enterprise-HA/compliance.
- **SQLite-clustering OSS (rqlite / LiteFS / dqlite)** — real HA but none is a zero-effort drop-in for Headscale's GORM/SQLite layer. Deferred to the #11 evaluation.

## Amendment 2026-08-17 — backup tool: Litestream, not `sqld` bottomless

The options survey (RESEARCH/DATASTORE_LIBSQL_VS_POSTGRES.md) named `sqld` +
"bottomless" as the WAL→S3 mechanism, and flagged that its version-specific
behaviour needed verifying against live docs before building. On implementing
`P0-3` (#3) that verification resolved it:

- Using `sqld` as the engine forces Headscale off its `glebarez/sqlite` file
  driver onto a libSQL client/dialector, and reworks the squibble schema
  validation — real risk for **zero** Phase-0 benefit (no clustering or read
  replicas needed yet).
- **Litestream** (v0.5.x, actively maintained) delivers exactly the WAL→S3 +
  point-in-time restore this ADR calls for, operating on the *existing* SQLite
  file with no driver or app-code change. `hscontrol/db/db.go` already ignores
  `_litestream_*` tables — the choice was pre-anticipated.

**Decision unchanged** (single-node SQLite family + WAL→S3, no cluster,
reversible); only the tool is pinned: **Litestream on the SQLite file.**
Config `litestream.yml`, drill `scripts/restore-drill.sh`, runbook
`docs/ops/backup-restore.md`. The engine remains reversible; revisit at #11.
