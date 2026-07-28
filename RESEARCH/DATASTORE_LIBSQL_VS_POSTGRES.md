# RavenScale datastore comparison

Decision record + options survey behind ADR-0005. **Outcome: libSQL single-node (no cluster) + WAL→S3 backup for Phase 0–1; revisit at HA milestone #11.**

## Why the workload makes this easy

A Tailscale-style control plane is **read-heavy, low-write**: nodes poll the coordination map frequently; writes are only node registrations and config/ACL changes. Single-node SQLite/libSQL handles very high read volume. Clustering buys automatic multi-node failover — not throughput we're short on.

Crucially, the engine choice is **reversible**: GORM abstracts the SQL dialect and upstream Headscale already supports both SQLite and Postgres, so switching later is a contained migration, not a rewrite. (Contrast the tenancy schema, ADR-0001, which is genuinely hard to reverse.)

## Options

| Option | HA / failover | Ops weight | Fit with Headscale/GORM | Verdict |
|--------|---------------|-----------|-------------------------|---------|
| **libSQL `sqld`, single node** (chosen) | None built-in; restore from S3/WAL | Lowest — one process + volume + object store | Native (SQLite wire) | **Phase 0–1** |
| libSQL `sqld` + read replicas | Read-scaling; **no auto-failover** (manual primary promotion) | Low–medium | Native | Add replicas if reads climb |
| **rqlite** | Real auto-failover via Raft, strong consistency, single Go binary | Medium | Talk over HTTP API — **not** a drop-in SQLite file | #11 candidate |
| **LiteFS** (Fly.io) | Primary/replica, **auto-failover via Consul lease**; app uses a normal SQLite file | Medium (adds Consul) | Mostly transparent; still single-writer | #11 candidate |
| **dqlite** (Canonical; LXD/MicroK8s) | Raft-based HA | Medium–high | C library to embed — heavy retrofit | Unlikely |
| **Marmot** | Multi-writer via NATS CDC, eventually consistent | Medium | Sidecar/CDC; niche | Unlikely |
| **Postgres + CloudNativePG/Patroni** | Mature auto-failover, PITR, RLS as 2nd isolation layer | Highest (separate clustered DB) | Native (Headscale supports PG) | #11 if enterprise-HA/compliance is the wedge |

## The deciding question (for #11)

Is RavenScale's wedge at that point **enterprise HA / compliance**?
- **Yes** → Postgres + operator (auto-failover + PITR + RLS for free).
- **No** → stay on libSQL; add LiteFS/rqlite only if a concrete availability SLA forces auto-failover.

## Backup posture now

- `sqld` streams WAL to S3-compatible storage ("bottomless") + periodic volume snapshots.
- P0-3 acceptance = a **documented restore drill that passes**. At this stage "HA" means fast restore, not automatic failover.

## Sources

libSQL/`sqld` (Apache-2.0) replication + embedded-replica model; rqlite (Raft); LiteFS (FUSE + Consul); dqlite (Canonical/Raft); Marmot (NATS CDC); CloudNativePG/Patroni for Postgres HA. From current project knowledge; verify version-specific `sqld` replica-promotion behaviour against live docs before building #11.
