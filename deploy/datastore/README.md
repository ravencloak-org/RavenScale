# RavenScale datastore (P0-3)

libSQL (`sqld`), **single node**, with continuous **WAL → S3 backup**. No
cluster, no Postgres. This is the resolved call in
[ADR-0005](../../docs/adr/0005-datastore-libsql-single-node.md); the reasoning
(read-heavy, low-write control plane) lives there and in
[`RESEARCH/DATASTORE_LIBSQL_VS_POSTGRES.md`](../../RESEARCH/DATASTORE_LIBSQL_VS_POSTGRES.md).

"HA" at this stage means **fast restore, not automatic failover**. Auto-failover
is deferred to HA #11, which builds on this.

## Layout

| File | What |
|------|------|
| `docker-compose.yml` | The datastore unit: one `sqld` + bottomless WAL→S3. |
| `.env.example` | Config. Copy to `.env`, fill in S3 creds. |
| `scripts/restore-drill.sh` | The acceptance artifact — see [RESTORE_DRILL.md](./RESTORE_DRILL.md). |
| `scripts/backup-verify.sh` | Cron/monitoring: `bottomless-cli verify` the latest generation. |
| `scripts/sqld-client.py` | Stdlib-only HTTP client used by the drill to seed/fingerprint. |

## Run it

```sh
cp .env.example .env      # set S3_BUCKET, S3_ENDPOINT, S3_* creds
docker compose up -d
```

Liveness: `GET http://<bind>/health` → `200`. There is no in-container
healthcheck — the image ships no HTTP client — so the P0-5 install script and
external monitors own the probe.

## Backup: how it works

`sqld` runs with `SQLD_ENABLE_BOTTOMLESS_REPLICATION=true`. The **bottomless**
replicator streams committed WAL frames to S3 continuously and uploads a main-db
snapshot per *generation*. Objects live under `s3://<bucket>/ns-:default-<uuid>/`.

- **On a clean stop** (`docker stop`, 30s grace) bottomless flushes the final
  frames + snapshot before exit → **RPO 0**.
- **On a hard crash**, everything already streamed to S3 survives; the exposure
  is the frames written since the last WAL batch reached S3. That window is
  bounded by write volume vs. sqld's autocheckpoint (default **1000 pages**).
  Tune `SQLD_MAX_LOG_SIZE` / `SQLD_MAX_LOG_DURATION` if a tighter crash RPO is
  needed. For a control plane whose writes are registrations + config changes,
  this is comfortably small.

Verify backups are actually restorable (not just present) on a schedule:

```sh
set -a; . ./.env; set +a
scripts/backup-verify.sh        # -> "Verification: ok", exit 0
```

## Restore

**The primary restore path is: point a fresh `sqld` at the same bucket.** On an
empty local WAL, bottomless restores the latest generation on boot. That is
exactly what a total node loss looks like, and it is what the drill exercises.

```sh
# node/volume is gone; same .env (same bucket) still applies:
docker compose up -d          # sqld auto-restores latest generation on boot
```

Point-in-time / specific generation (manual, via `bottomless-cli`):

```sh
set -a; . ./.env; set +a
# list generations + timestamps:
docker run --rm \
  -e LIBSQL_BOTTOMLESS_AWS_ACCESS_KEY_ID=$S3_ACCESS_KEY_ID \
  -e LIBSQL_BOTTOMLESS_AWS_SECRET_ACCESS_KEY=$S3_SECRET_ACCESS_KEY \
  -e LIBSQL_BOTTOMLESS_AWS_DEFAULT_REGION=$S3_REGION \
  --entrypoint bottomless-cli ghcr.io/tursodatabase/libsql-server:$SQLD_VERSION \
  -e $S3_ENDPOINT -b $S3_BUCKET -n 'ns-:default' ls -v

# restore into a local db file at/just-before a timestamp (UTC), then mount it:
#   ... bottomless-cli ... restore -n 'ns-:default' [--generation <uuid>]
```

RTO is seconds-to-minutes: the drill restores a ~24 MB db in ~0.4 s; wall time
in production is dominated by pulling the snapshot from S3, not by sqld.

## Migrations

RavenScale inherits Headscale's **GORM AutoMigrate**, which runs at process
start. There is no separate migration tool to invoke; deploying a new binary
migrates the schema on boot. Two ops rules follow (enforced by P0-5's upgrade
script, not here):

1. **Back up before every upgrade.** With bottomless on, a clean restart already
   flushes a generation; the upgrade path additionally `verify`s it before
   swapping the binary, so there is a known-good restore point pre-migration.
2. **The one irreversible migration is the N=1 tenant backfill (ADR-0003),
   owned by #2.** It must be forward-only + idempotent and is tested against a
   seeded pre-fork DB there — out of scope for this datastore card, called out
   so it is not lost.

## Handoff to #2 (fork / P0-2)

This stands up the *server*. The app (forked Headscale) must talk to it. Upstream
Headscale opens a local SQLite file via GORM; to use `sqld` instead, the fork
swaps the GORM dialector to the libSQL driver and points it at the DSN:

```
libsql://<sqld-host>:8080?authToken=<jwt>     # or http:// for plaintext/dev
```

Go: `github.com/tursodatabase/libsql-client-go/libsql` (database/sql driver) +
a GORM SQLite-compatible dialector over that connection. The dialect stays
abstracted (ADR-0005) so a later engine switch is a contained migration. I'll
drop this same note on #2.
