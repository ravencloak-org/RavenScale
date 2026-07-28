#!/usr/bin/env bash
#
# upgrade.sh [--dev] — upgrade a running RavenScale stack in place.
#
# Order is deliberate: prove the backup is restorable BEFORE touching anything,
# then pull, re-render config, recreate only changed containers, and health-gate.
# Schema migrations apply automatically when the control plane (re)boots `serve`.
#
# To pin a target version, edit RAVENSCALE_IMAGE / SQLD_VERSION in .env first,
# then run this. To roll back, set them to the previous tags and run again.
#
set -euo pipefail
cd "$(dirname "$0")"

DEV=0
[ "${1:-}" = "--dev" ] && DEV=1

log()  { printf '\033[1m== %s ==\033[0m\n' "$*"; }
warn() { printf '\033[33mWARN: %s\033[0m\n' "$*" >&2; }
die()  { printf '\033[31mERROR: %s\033[0m\n' "$*" >&2; exit 1; }

[ -f .env ] || die "no .env — run ./install.sh first"
set -a; . ./.env; set +a
# Empty SQLD_AUTH_JWT_KEY => libsql-server 401s everything. Drop the empty .env
# line so compose omits the var and sqld stays open (a real key is kept).
if grep -q '^SQLD_AUTH_JWT_KEY=$' .env; then
  grep -v '^SQLD_AUTH_JWT_KEY=$' .env > .env.tmp && mv .env.tmp .env
  unset SQLD_AUTH_JWT_KEY
fi

COMPOSE=(docker compose -f docker-compose.yml)
[ "$DEV" = "1" ] && COMPOSE+=(-f compose.dev.yml)
"${COMPOSE[@]}" ps -q sqld | grep -q . || die "stack not running — run ./install.sh"

# --- 1. backup safety gate (before any change) ------------------------------
log "verify datastore backup is restorable before upgrading"
if [ "$DEV" = "1" ]; then
  # dev MinIO lives on the compose network; let bottomless-cli reach it. A fresh
  # stack may not have flushed a generation yet, so this is best-effort in dev.
  DOCKER_NETWORK="$("${COMPOSE[@]}" ps sqld --format '{{.Names}}' | sed 's/-sqld.*//')_default"
  if DOCKER_NETWORK="$DOCKER_NETWORK" bash datastore/scripts/backup-verify.sh; then
    log "backup verified (dev/MinIO)"
  else
    warn "backup not yet verifiable in dev (no generation flushed?) — continuing"
  fi
else
  bash datastore/scripts/backup-verify.sh \
    || die "backup NOT restorable — refusing to upgrade. Fix backups first."
  log "backup verified (S3)"
fi

# --- 2. pull target images --------------------------------------------------
log "pull target images (RAVENSCALE_IMAGE=${RAVENSCALE_IMAGE}, SQLD_VERSION=${SQLD_VERSION:-latest})"
"${COMPOSE[@]}" pull --quiet 2>/dev/null || "${COMPOSE[@]}" pull

# --- 3. re-render config (template may have changed) ------------------------
log "re-render config/control-plane.yaml"
envsubst '${HS_SERVER_URL} ${HS_BASE_DOMAIN}' \
  < config/control-plane.yaml.tmpl > config/control-plane.yaml

# --- 4. recreate changed containers -----------------------------------------
# Migrations run on control-plane `serve` boot. Compose recreates only services
# whose image/config changed; unchanged ones (and their volumes) are untouched.
log "apply: recreate changed containers"
"${COMPOSE[@]}" up -d

# --- 5. health gate ---------------------------------------------------------
SQLD_HOST_PORT="${SQLD_BIND##*:}"
HS_HOST_PORT="${HS_BIND##*:}"
log "wait for datastore"
bash scripts/wait-health.sh "http://127.0.0.1:${SQLD_HOST_PORT}/health" 60 "$(${COMPOSE[@]} ps -q sqld)"
log "wait for control plane"
bash scripts/wait-health.sh "http://127.0.0.1:${HS_HOST_PORT}/health" 90 "$(${COMPOSE[@]} ps -q control-plane)"

cat <<EOF

$(printf '\033[32mUpgrade complete.\033[0m') control plane and datastore healthy.
  image: ${RAVENSCALE_IMAGE}   sqld: ${SQLD_VERSION:-latest}

Rollback if needed:
  1. set RAVENSCALE_IMAGE / SQLD_VERSION in .env back to the previous tags
  2. ./upgrade.sh $([ "$DEV" = 1 ] && echo --dev)
  3. if data must be rewound, restore from S3: see deploy/datastore/RESTORE_DRILL.md
EOF
