# Restore drill (P0-3 acceptance)

**Acceptance for #3 is "a documented restore drill that passes."** This is it.
It is a script, not a claim. It fails loudly if restore is not byte-faithful.

## What it proves

A total loss of the `sqld` node — process **and** local volume gone — is
recoverable from S3 alone, with no data drift. It runs the real cycle:

1. start `sqld` with bottomless WAL→S3 replication on, backed by S3
2. seed a known dataset; record a fingerprint (row counts + checksums)
3. write enough to cross autocheckpoint so a WAL batch lands in S3; wait; then
   **destroy the sqld container and its volume** — S3 is the only surviving copy
4. boot a fresh `sqld` against the same bucket → it auto-restores on boot
5. re-fingerprint and **assert it equals** the pre-disaster fingerprint

## Run it

Needs only Docker. Hermetic by default — it runs its own MinIO as the S3
backend, so it is CI-safe and needs no real bucket:

```sh
cd deploy/datastore
bash scripts/restore-drill.sh
```

Drill against a **real** bucket instead (use a scratch namespace, not prod data):

```sh
EXTERNAL_S3=1 S3_ENDPOINT=https://s3.us-east-1.amazonaws.com \
S3_BUCKET=ravenscale-drill S3_REGION=us-east-1 \
S3_ACCESS_KEY_ID=... S3_SECRET_ACCESS_KEY=... \
bash scripts/restore-drill.sh
```

Exit `0` = passed. Non-zero = do not ship; the output shows the mismatch.

## Passing output (recorded 2026-07-28, libsql-server:latest)

```
== start hermetic MinIO S3 backend ==
bucket ravenscale-db ready
== start sqld (bottomless replication ON) and seed a known dataset ==
== pre-disaster fingerprint: 500	125250	6000	12288000 ==
== wait for periodic WAL->S3 backup ==
bottomless replicator: local backup replicated frames until 6024
bottomless replicator: shutdown complete
== sqld node and its local volume DESTROYED — S3 is the only copy now ==
== boot a fresh sqld against the same bucket (auto-restore on empty local WAL) ==
bottomless::replicator: Finished database restoration in 371.245835ms
bottomless::replicator: Restoring from generation ...: action=SnapshotMainDbFile, recovered=true
== post-restore fingerprint:  500	125250	6000	12288000 ==
DRILL PASSED: restore is byte-faithful (500	125250	6000	12288000)
```

Pre- and post-disaster fingerprints are identical: 500 nodes (id-sum 125250),
6000 blob rows (12,288,000 bytes). Restore took ~0.4 s for a ~24 MB database.

## For the verifier (@Gavin)

Run it yourself on a fresh checkout — a restore that only works on my machine is
not a restore. `bash scripts/restore-drill.sh`, expect exit 0 and
`DRILL PASSED`. To confirm it is not a no-op, break it on purpose (e.g. change a
seed count between fingerprint and assert) and confirm it exits non-zero.
```
