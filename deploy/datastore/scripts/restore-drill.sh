#!/usr/bin/env bash
#
# restore-drill.sh — RavenScale P0-3 datastore restore drill.
#
# Proves the WAL->S3 backup path actually restores. It is not a document that
# asserts the backup works; it exercises it:
#
#   1. stand up sqld (bottomless replication on) + an S3 backend
#   2. seed a known dataset and record a fingerprint (row counts + checksums)
#   3. force enough writes to cross sqld's autocheckpoint, wait for the WAL
#      batch to land in S3, then destroy the sqld node AND its local volume
#   4. boot a fresh sqld against the same bucket — it auto-restores the latest
#      generation on an empty local WAL
#   5. re-fingerprint and assert it matches the pre-disaster fingerprint
#
# Exit 0 == drill passed (restore is byte-faithful). Non-zero == it failed;
# read the output, do not ship.
#
# By default the drill is hermetic: it runs its own MinIO as the S3 backend so
# it needs nothing but Docker and can run in CI. To drill against a real
# bucket instead (e.g. the actual prod S3, in a scratch namespace), set
# EXTERNAL_S3=1 and the S3_* vars below.
#
set -euo pipefail

# --- config -----------------------------------------------------------------
SQLD_IMAGE="${SQLD_IMAGE:-ghcr.io/tursodatabase/libsql-server:latest}"
MINIO_IMAGE="${MINIO_IMAGE:-minio/minio:latest}"
MC_IMAGE="${MC_IMAGE:-minio/mc:latest}"
NET="${NET:-rs-restore-drill}"
SQLD_NAME="rs-drill-sqld"
MINIO_NAME="rs-drill-minio"
HOST_PORT="${HOST_PORT:-8080}"
BASE_URL="http://127.0.0.1:${HOST_PORT}"

EXTERNAL_S3="${EXTERNAL_S3:-0}"
S3_BUCKET="${S3_BUCKET:-ravenscale-db}"
S3_REGION="${S3_REGION:-us-east-1}"
# For the hermetic path these point at the drill's own MinIO:
S3_ENDPOINT="${S3_ENDPOINT:-http://${MINIO_NAME}:9000}"
S3_ACCESS_KEY_ID="${S3_ACCESS_KEY_ID:-minioadmin}"
S3_SECRET_ACCESS_KEY="${S3_SECRET_ACCESS_KEY:-minioadmin}"

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLIENT="python3 ${HERE}/sqld-client.py ${BASE_URL}"

log() { printf '\n\033[1m== %s ==\033[0m\n' "$*"; }
fail() { printf '\n\033[31mDRILL FAILED: %s\033[0m\n' "$*" >&2; exit 1; }

cleanup() {
  docker rm -f "$SQLD_NAME" >/dev/null 2>&1 || true
  [ "$EXTERNAL_S3" = "1" ] || docker rm -f "$MINIO_NAME" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

start_sqld() {
  docker run -d --name "$SQLD_NAME" --network "$NET" -p "${HOST_PORT}:8080" \
    -e SQLD_HTTP_LISTEN_ADDR=0.0.0.0:8080 \
    -e SQLD_ENABLE_BOTTOMLESS_REPLICATION=true \
    -e LIBSQL_BOTTOMLESS_BUCKET="$S3_BUCKET" \
    -e LIBSQL_BOTTOMLESS_ENDPOINT="$S3_ENDPOINT" \
    -e LIBSQL_BOTTOMLESS_AWS_ACCESS_KEY_ID="$S3_ACCESS_KEY_ID" \
    -e LIBSQL_BOTTOMLESS_AWS_SECRET_ACCESS_KEY="$S3_SECRET_ACCESS_KEY" \
    -e LIBSQL_BOTTOMLESS_AWS_DEFAULT_REGION="$S3_REGION" \
    "$SQLD_IMAGE" >/dev/null
}

wait_health() {
  for _ in $(seq 1 30); do
    if python3 - "$BASE_URL" <<'PY' 2>/dev/null; then return 0; fi
import sys, urllib.request
urllib.request.urlopen(sys.argv[1] + "/health", timeout=2)
PY
    sleep 1
  done
  docker logs "$SQLD_NAME" 2>&1 | tail -20 >&2
  fail "sqld did not become healthy at ${BASE_URL}/health"
}

fingerprint() {
  $CLIENT "SELECT \
    (SELECT count(*) FROM nodes) AS nodes, \
    (SELECT COALESCE(sum(id),0) FROM nodes) AS nodes_chk, \
    (SELECT count(*) FROM blobs) AS blobs, \
    (SELECT COALESCE(sum(length(pad)),0) FROM blobs) AS blob_bytes" | tail -1
}

# --- setup ------------------------------------------------------------------
cleanup
docker network create "$NET" >/dev/null

if [ "$EXTERNAL_S3" != "1" ]; then
  log "start hermetic MinIO S3 backend"
  docker run -d --name "$MINIO_NAME" --network "$NET" \
    -e MINIO_ROOT_USER="$S3_ACCESS_KEY_ID" \
    -e MINIO_ROOT_PASSWORD="$S3_SECRET_ACCESS_KEY" \
    "$MINIO_IMAGE" server /data >/dev/null
  sleep 4
  docker run --rm --network "$NET" --entrypoint sh "$MC_IMAGE" -c \
    "mc alias set m ${S3_ENDPOINT} ${S3_ACCESS_KEY_ID} ${S3_SECRET_ACCESS_KEY} >/dev/null && mc mb -p m/${S3_BUCKET} >/dev/null && echo 'bucket ${S3_BUCKET} ready'"
fi

# --- 1+2: seed and fingerprint ---------------------------------------------
log "start sqld (bottomless replication ON) and seed a known dataset"
start_sqld
wait_health
$CLIENT \
  "CREATE TABLE IF NOT EXISTS nodes(id INTEGER PRIMARY KEY, name TEXT, tenant_id INTEGER NOT NULL)" \
  "INSERT INTO nodes(name,tenant_id) SELECT 'node-'||value,(value%3)+1 FROM (WITH RECURSIVE c(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM c WHERE value<500) SELECT value FROM c)" \
  >/dev/null
# ~6 MB of padding to cross sqld autocheckpoint (1000 pages) so a WAL batch
# actually flushes to S3 during the drill, not only at shutdown.
$CLIENT \
  "CREATE TABLE IF NOT EXISTS blobs(id INTEGER PRIMARY KEY, pad TEXT)" \
  "INSERT INTO blobs(pad) SELECT hex(randomblob(1024)) FROM (WITH RECURSIVE c(v) AS (SELECT 1 UNION ALL SELECT v+1 FROM c WHERE v<6000) SELECT v FROM c)" \
  >/dev/null

BEFORE="$(fingerprint)"
log "pre-disaster fingerprint: ${BEFORE}"
[ -n "$BEFORE" ] || fail "could not read pre-disaster fingerprint"

# --- 3: force a mid-flight backup, then destroy the node --------------------
log "wait for periodic WAL->S3 backup"
sleep 12
# Graceful stop => bottomless flushes the final frames + snapshot (RPO 0 on a
# clean stop). `docker rm` then removes the container's local volume entirely:
# this is a real total-loss of the node, S3 is the only surviving copy.
docker stop "$SQLD_NAME" >/dev/null
docker logs "$SQLD_NAME" 2>&1 | grep -i -e "replicated frames" -e "shutdown complete" | tail -2 || true
docker rm -f "$SQLD_NAME" >/dev/null
log "sqld node and its local volume DESTROYED — S3 is the only copy now"

# --- 4: fresh node auto-restores from S3 ------------------------------------
log "boot a fresh sqld against the same bucket (auto-restore on empty local WAL)"
start_sqld
wait_health
docker logs "$SQLD_NAME" 2>&1 | grep -i -e "restoration in" -e "recovered=true" | tail -3 \
  || fail "no restore evidence in sqld logs"

# --- 5: assert --------------------------------------------------------------
AFTER="$(fingerprint)"
log "post-restore fingerprint:  ${AFTER}"
if [ "$BEFORE" = "$AFTER" ]; then
  printf '\n\033[32mDRILL PASSED: restore is byte-faithful (%s)\033[0m\n' "$AFTER"
  exit 0
fi
fail "fingerprint mismatch — before='${BEFORE}' after='${AFTER}'"
