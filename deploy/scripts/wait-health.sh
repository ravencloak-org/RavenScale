#!/usr/bin/env bash
#
# wait-health.sh URL [TIMEOUT_SECONDS] [CONTAINER]
#
# Poll URL until it returns HTTP 2xx/3xx or TIMEOUT elapses. Exit 0 on healthy,
# 1 on timeout. On timeout, dumps the last container logs if CONTAINER is given.
# Uses python3 urllib (the sqld image ships no curl; we stay consistent).
set -euo pipefail

URL="${1:?usage: wait-health.sh URL [TIMEOUT] [CONTAINER]}"
TIMEOUT="${2:-60}"
CONTAINER="${3:-}"

for _ in $(seq 1 "$TIMEOUT"); do
  if python3 - "$URL" <<'PY' 2>/dev/null; then
import sys, urllib.request
r = urllib.request.urlopen(sys.argv[1], timeout=2)
sys.exit(0 if r.status < 400 else 1)
PY
    echo "healthy: $URL"
    exit 0
  fi
  sleep 1
done

echo "TIMEOUT: $URL did not become healthy in ${TIMEOUT}s" >&2
if [ -n "$CONTAINER" ]; then
  echo "--- last logs: $CONTAINER ---" >&2
  docker logs "$CONTAINER" 2>&1 | tail -30 >&2 || true
fi
exit 1
