# Datastore backup & restore (P0-3)

How RavenScale's control-plane SQLite datastore is backed up and recovered.
Design: **single-node libSQL/SQLite + Litestream WAL→S3, no clustering**
(ADR-0005). At Phase 0–1 "HA" means *fast restore*, not automatic failover.

## Mechanism

Headscale writes its DB in **WAL mode** by default
(`hscontrol/db/sqliteconfig.Default` → `JournalModeWAL`). [Litestream](https://litestream.io)
(v0.5+) runs alongside the control plane and streams the WAL to S3-compatible
object storage continuously, enabling point-in-time restore.

```
headscale ──writes──▶ db.sqlite (WAL) ──litestream replicate──▶ S3
                       restore ◀── litestream restore ── S3
```

- **RPO** ≈ 1s of writes (`sync-interval: 1s`). The control plane is
  read-heavy/low-write (registrations + config changes), so real data at risk
  is minimal.
- **RTO** = time to pull the latest snapshot + WAL from S3 and replay — seconds
  to low minutes for a control-plane-sized DB.
- **Retention**: 72h point-in-time window, daily snapshots (tune in
  `litestream.yml`).

## Production setup

1. Install Litestream on the control-plane host (NixOS: `services.litestream`;
   see `nix/example-configuration.nix`).
2. Ship `litestream.yml` (repo root) and set the environment:

   ```sh
   RAVENSCALE_DB_PATH=/var/lib/headscale/db.sqlite
   LITESTREAM_S3_BUCKET=my-bucket
   LITESTREAM_S3_PATH=ravenscale/control
   LITESTREAM_S3_REGION=us-east-1
   LITESTREAM_S3_ENDPOINT=            # blank for AWS; set for MinIO/R2/etc.
   AWS_ACCESS_KEY_ID=...
   AWS_SECRET_ACCESS_KEY=...
   ```

3. Run the replication daemon (systemd unit or `-exec` to co-supervise
   headscale):

   ```sh
   litestream replicate -config litestream.yml
   # or, to tie litestream's lifecycle to headscale:
   litestream replicate -config litestream.yml -exec "headscale serve"
   ```

4. Validate config and confirm the DB is tracked:

   ```sh
   litestream databases -config litestream.yml
   litestream status   -config litestream.yml
   ```

## Restore procedure (real incident)

Primary volume lost. On a fresh host with `litestream.yml` + env in place:

1. **Stop** headscale (nothing must write to the DB path during restore).
2. Restore the latest backup to the DB path:

   ```sh
   litestream restore -config litestream.yml "$RAVENSCALE_DB_PATH"
   # point-in-time: add -timestamp 2026-08-17T09:00:00Z  (or -txid <hex>)
   ```

3. **Start** headscale. It reopens the DB in WAL mode and resumes; Litestream
   begins replicating the restored DB forward again.

> Restore refuses to overwrite an existing file — restore to a clean path (or
> move the corrupt DB aside first).

## Restore drill (the AC)

`scripts/restore-drill.sh` proves the WAL→replica→restore path recovers with
zero loss, using a local *file* replica (same backend interface as S3, no cloud
creds). It seeds rows, snapshots, adds more rows that live **only in the WAL**,
destroys the primary, restores, and asserts every row — including the
WAL-replayed ones — came back.

```sh
litestream --version            # v0.5+ required
./scripts/restore-drill.sh      # exits 0 on PASS, non-zero (with a diff) on loss
```

Run it in CI and after any change to the datastore or backup config.

## Revisit

Clustering / auto-failover (LiteFS, rqlite, or Postgres) is deferred to the HA
milestone (#11), to be decided with real load data — see ADR-0005.
