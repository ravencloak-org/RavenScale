#!/usr/bin/env bash
# P0-3 restore drill (ADR-0005). Proves the litestream WAL→replica→restore path
# recovers the control-plane SQLite DB with zero data loss. Uses a local *file*
# replica so it runs anywhere with no cloud creds — the S3 code path is the same
# backend interface, exercised identically in production via litestream.yml.
#
# AC (issue #3): "documented restore drill that passes." This IS that drill; run
# it in CI or by hand. Exits 0 on success, non-zero (with a diff) on any loss.
#
# Requires: litestream (v0.5+) on PATH or $LITESTREAM, and python3 (stdlib
# sqlite3 — no extra deps). Mirrors Headscale's WAL journal mode
# (sqliteconfig.Default) so the drill matches the real DB's on-disk shape.
set -euo pipefail

LITESTREAM="${LITESTREAM:-litestream}"
if ! command -v "$LITESTREAM" >/dev/null 2>&1; then
  echo "restore-drill: litestream not found (set \$LITESTREAM or add to PATH)" >&2
  exit 127
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
DB="$WORK/control.db"
REPLICA="file://$WORK/replica"
SEED=100    # rows before first backup
EXTRA=50    # rows added before second backup (exercises WAL capture, not just snapshot)
WANT=$((SEED + EXTRA))

# 1. Seed a WAL-mode DB (Headscale's default journal_mode).
python3 - "$DB" "$SEED" <<'PY'
import sqlite3, sys
db, n = sys.argv[1], int(sys.argv[2])
c = sqlite3.connect(db)
c.execute("PRAGMA journal_mode=WAL")
c.execute("CREATE TABLE node(id INTEGER PRIMARY KEY, name TEXT)")
c.executemany("INSERT INTO node(name) VALUES(?)", [(f"seed-{i}",) for i in range(n)])
c.commit(); c.close()
PY
echo "restore-drill: seeded $SEED rows (WAL)"

# 2. First backup: snapshot.
"$LITESTREAM" replicate -once "$DB" "$REPLICA" >/dev/null
echo "restore-drill: snapshot replicated"

# 3. Mutate after the snapshot — these rows exist ONLY in the WAL, so a passing
#    restore proves WAL replay, not just snapshot recovery.
python3 - "$DB" "$EXTRA" <<'PY'
import sqlite3, sys
db, n = sys.argv[1], int(sys.argv[2])
c = sqlite3.connect(db)
c.execute("PRAGMA journal_mode=WAL")
c.executemany("INSERT INTO node(name) VALUES(?)", [(f"post-{i}",) for i in range(n)])
c.commit(); c.close()
PY
"$LITESTREAM" replicate -once "$DB" "$REPLICA" >/dev/null
echo "restore-drill: post-snapshot WAL replicated ($WANT rows total)"

# 4. Simulate catastrophic loss of the primary volume.
rm -f "$DB" "$DB-wal" "$DB-shm"
echo "restore-drill: primary DB destroyed"

# 5. Restore from the replica alone.
"$LITESTREAM" restore -o "$DB" "$REPLICA" >/dev/null
echo "restore-drill: restored from replica"

# 6. Verify: exact row count AND content integrity (both seed + post-snapshot rows).
GOT="$(python3 - "$DB" <<'PY'
import sqlite3, sys
c = sqlite3.connect(sys.argv[1])
total = c.execute("SELECT count(*) FROM node").fetchone()[0]
posts = c.execute("SELECT count(*) FROM node WHERE name LIKE 'post-%'").fetchone()[0]
print(f"{total} {posts}")
PY
)"
read -r TOTAL POSTS <<<"$GOT"

if [[ "$TOTAL" -ne "$WANT" || "$POSTS" -ne "$EXTRA" ]]; then
  echo "restore-drill: FAIL — expected total=$WANT post=$EXTRA, got total=$TOTAL post=$POSTS" >&2
  exit 1
fi
echo "restore-drill: PASS — recovered $TOTAL rows ($POSTS from WAL replay), zero loss"
