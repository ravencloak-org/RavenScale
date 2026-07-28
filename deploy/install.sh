#!/usr/bin/env bash
#
# install.sh [--dev] — one-command RavenScale deploy.
#
# Stands up the full stack (control plane + libSQL datastore, ADR-0005) and does
# not return until both services answer /health. Idempotent: safe to re-run; it
# reconciles the running stack to the current config.
#
#   ./install.sh          production — real S3 from .env (refuses empty creds)
#   ./install.sh --dev    hermetic  — bundled MinIO, zero external deps
#
set -euo pipefail
cd "$(dirname "$0")"

DEV=0
[ "${1:-}" = "--dev" ] && DEV=1

log()  { printf '\033[1m== %s ==\033[0m\n' "$*"; }
die()  { printf '\033[31mERROR: %s\033[0m\n' "$*" >&2; exit 1; }

# --- 1. preflight -----------------------------------------------------------
log "preflight"
command -v docker >/dev/null || die "docker not found"
docker compose version >/dev/null 2>&1 || die "docker compose v2 not found"
docker info >/dev/null 2>&1 || die "docker daemon not reachable"

# --- 2. .env ----------------------------------------------------------------
if [ ! -f .env ]; then
  cp .env.example .env
  log "created .env from .env.example"
  if [ "$DEV" = "1" ]; then
    # Hermetic: point the datastore's S3 at the bundled MinIO. Appended, so
    # these win over the example defaults (dotenv: last occurrence wins).
    cat >> .env <<'EOF'

# --- appended by install.sh --dev: bundled MinIO ---
S3_ENDPOINT=http://minio:9000
S3_ACCESS_KEY_ID=minioadmin
S3_SECRET_ACCESS_KEY=minioadmin
EOF
    log "dev .env points S3 at bundled MinIO"
  fi
fi

# load .env for rendering + validation
set -a; . ./.env; set +a

# libsql-server turns JWT auth ON whenever SQLD_AUTH_JWT_KEY is present, even
# empty (=> every query 401s). The contract is empty = OPEN. Compose's bare
# `- SQLD_AUTH_JWT_KEY` still injects an empty value if .env carries the empty
# assignment, so physically drop that line (only the empty one; a real key
# `SQLD_AUTH_JWT_KEY=<key>` is kept) so the var is omitted and sqld stays open.
if grep -q '^SQLD_AUTH_JWT_KEY=$' .env; then
  grep -v '^SQLD_AUTH_JWT_KEY=$' .env > .env.tmp && mv .env.tmp .env
  unset SQLD_AUTH_JWT_KEY
fi

# --- 3. validate ------------------------------------------------------------
: "${HS_SERVER_URL:?set HS_SERVER_URL in .env}"
: "${HS_BASE_DOMAIN:?set HS_BASE_DOMAIN in .env}"
if [ "$DEV" != "1" ]; then
  [ -n "${S3_ACCESS_KEY_ID:-}" ] && [ -n "${S3_SECRET_ACCESS_KEY:-}" ] \
    || die "S3_ACCESS_KEY_ID / S3_SECRET_ACCESS_KEY are empty in .env (prod backup would be a no-op). Fill them, or run --dev."
fi
# server_url host must differ from base_domain (Headscale hard-requires this).
case "$HS_SERVER_URL" in
  *"$HS_BASE_DOMAIN"*) die "HS_SERVER_URL host must not contain HS_BASE_DOMAIN ($HS_BASE_DOMAIN)";;
esac

# --- 4. render control-plane config ----------------------------------------
log "render config/control-plane.yaml"
envsubst '${HS_SERVER_URL} ${HS_BASE_DOMAIN}' \
  < config/control-plane.yaml.tmpl > config/control-plane.yaml

# --- 5. compose up ----------------------------------------------------------
COMPOSE=(docker compose -f docker-compose.yml)
[ "$DEV" = "1" ] && COMPOSE+=(-f compose.dev.yml)

log "pull images"
"${COMPOSE[@]}" pull --quiet 2>/dev/null || "${COMPOSE[@]}" pull
log "start stack"
"${COMPOSE[@]}" up -d

# --- 6. health gate ---------------------------------------------------------
log "wait for datastore"
SQLD_HOST_PORT="${SQLD_BIND##*:}"
bash scripts/wait-health.sh "http://127.0.0.1:${SQLD_HOST_PORT}/health" 60 "$(${COMPOSE[@]} ps -q sqld)"

log "wait for control plane"
HS_HOST_PORT="${HS_BIND##*:}"
bash scripts/wait-health.sh "http://127.0.0.1:${HS_HOST_PORT}/health" 90 "$(${COMPOSE[@]} ps -q control-plane)"

# --- 7. summary -------------------------------------------------------------
cat <<EOF

$(printf '\033[32mRavenScale is up.\033[0m')
  control plane : http://127.0.0.1:${HS_HOST_PORT}   (health: /health)
  datastore     : http://127.0.0.1:${SQLD_HOST_PORT}   (health: /health)
  image         : ${RAVENSCALE_IMAGE}
  mode          : $([ "$DEV" = 1 ] && echo 'DEV (bundled MinIO)' || echo 'PRODUCTION (S3 from .env)')

Next: create a pre-auth key to join a node —
  ${COMPOSE[*]} exec control-plane headscale preauthkeys create --user 1 --reusable
Ops: see deploy/RUNBOOK.md . Upgrade: ./upgrade.sh $([ "$DEV" = 1 ] && echo --dev)
EOF
