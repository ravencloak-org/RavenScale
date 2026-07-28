#!/usr/bin/env python3
"""Minimal sqld HTTP pipeline client — used by the restore drill to seed and
fingerprint the database without pulling in a libSQL driver.

Usage:
    sqld-client.py <base_url> "SQL" ["SQL2" ...]

Prints result rows (tab-separated, header first) for any statement that returns
rows. Exits non-zero on the first SQL error. Uses only the stdlib so it runs
anywhere python3 does — no libsql client needed for ops tooling.

Note: sqld rejects `PRAGMA wal_checkpoint(...)` over the wire; the server owns
checkpointing (autocheckpoint, default 1000 pages). Do not try to checkpoint
from here.
"""
import json
import sys
import urllib.request

def main() -> int:
    if len(sys.argv) < 3:
        print(__doc__, file=sys.stderr)
        return 2
    base = sys.argv[1].rstrip("/")
    stmts = sys.argv[2:]
    reqs = [{"type": "execute", "stmt": {"sql": s}} for s in stmts]
    reqs.append({"type": "close"})
    body = json.dumps({"requests": reqs}).encode()
    req = urllib.request.Request(
        base + "/v2/pipeline", data=body,
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        out = json.load(resp)
    for res in out.get("results", []):
        if res.get("type") == "error":
            print("ERROR:", res.get("error"), file=sys.stderr)
            return 1
        r = res.get("response", {})
        if r.get("type") != "execute":
            continue
        result = r.get("result", {})
        cols = [c.get("name") for c in result.get("cols", [])]
        rows = result.get("rows", [])
        if cols and rows:
            print("\t".join(cols))
            for row in rows:
                print("\t".join(str(c.get("value")) for c in row))
    return 0

if __name__ == "__main__":
    sys.exit(main())
