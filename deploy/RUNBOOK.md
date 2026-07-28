# RavenScale — Ops Runbook (P0-5)

The operational contract for standing up, upgrading, and recovering a RavenScale
deployment. The stack is a single unit: **control plane + libSQL datastore**
(`sqld`, single-node, WAL→S3 backup — ADR-0005, no cluster). Everything here is
driven by two scripts in this directory; you should rarely call `docker compose`
by hand.

| Task | Command |
|------|---------|
| Deploy (prod) | `./install.sh` |
| Deploy (hermetic, bundled MinIO) | `./install.sh --dev` |
| Upgrade in place | `./upgrade.sh` (add `--dev` for a dev stack) |
| Health | `curl -fsS http://<host>:8080/health` (control plane), `:8081/health` (datastore) |
| Backup verify | `. ./.env && ./datastore/scripts/backup-verify.sh` |
| Restore drill | `./datastore/scripts/restore-drill.sh` (see `datastore/RESTORE_DRILL.md`) |

## Prerequisites

- Docker Engine + Compose v2 (`docker compose version`). Verified on Docker 29.x.
- `envsubst` (from `gettext`) and `python3` on the deploy host — used by the scripts.
- **Production only:** an S3 (or S3-compatible) bucket for WAL backups, and its
  credentials. `--dev` bundles a MinIO container instead, so it needs nothing
  but Docker.

## First deploy

```bash
cp .env.example .env      # then edit: HS_SERVER_URL, HS_BASE_DOMAIN, S3_* creds
./install.sh              # preflights, renders config, brings up, health-gates
```

`install.sh` is **idempotent** — re-running reconciles the stack to the current
`.env`/config. It refuses to start in production if the S3 credentials are empty
(a backup that silently no-ops is worse than a loud failure). For a zero-config
local bring-up with no external S3:

```bash
./install.sh --dev
```

After it returns healthy, create a user and a pre-auth key (nodes join via
tenant-scoped pre-auth keys — CONTEXT §4):

```bash
docker compose -f docker-compose.yml exec control-plane headscale users create default
docker compose -f docker-compose.yml exec control-plane headscale preauthkeys create --user 1 --reusable
```

## Upgrade

```bash
# pin the target version(s) first:
$EDITOR .env      # bump RAVENSCALE_IMAGE and/or SQLD_VERSION
./upgrade.sh      # (add --dev on a dev stack)
```

`upgrade.sh` order is deliberate and safe-by-default:

1. **Backup gate** — runs `bottomless-cli verify` against the live backup and
   **refuses to proceed** (production) if it is not restorable. Fix backups
   first. (In `--dev` this is best-effort: a brand-new stack may not have
   flushed a WAL generation yet.)
2. Pull the target images.
3. Re-render `config/control-plane.yaml` from the template.
4. `docker compose up -d` — recreates **only** the containers whose image/config
   changed; volumes (and therefore data) are untouched. Schema migrations apply
   automatically when the control plane reboots `serve`.
5. Health-gate both services before returning.

Data survives upgrades because state lives in named volumes
(`ravenscale_control-plane-data`, `ravenscale_sqld-data`), not in the
containers. Verified: a control-plane container replacement preserves users.

### Rollback

1. Set `RAVENSCALE_IMAGE` / `SQLD_VERSION` in `.env` back to the previous tags.
2. `./upgrade.sh` (add `--dev`).
3. If **data** must be rewound (not just the binary), restore the datastore from
   S3 — see `datastore/RESTORE_DRILL.md`. The backup is byte-faithful and the
   restore is drilled.

## Backup & restore

Backups are continuous: `sqld` streams WAL frames to S3 (bottomless). There is
no backup cron to babysit — but a backup you never restore-test is a rumor, so:

- **Verify** (cheap, run from monitoring/cron): `./datastore/scripts/backup-verify.sh`
- **Restore drill** (proves it end-to-end): `./datastore/scripts/restore-drill.sh` —
  exit 0 means the restore is byte-faithful. Full procedure in
  `datastore/RESTORE_DRILL.md`.

## Datastore seam (why the control plane runs on SQLite today)

The default `RAVENSCALE_IMAGE` is **upstream Headscale**, used as a real,
health-checkable stand-in until **P0-2** ships the RavenScale fork image. Upstream
Headscale speaks only `sqlite`/`postgres`, so it cannot talk to `sqld` over the
wire — it runs on its own SQLite volume. `sqld` runs alongside as the datastore
the fork will bind to (GORM libSQL dialect), so the full topology, backup path,
install, and upgrade machinery are all real and tested **now**.

When P0-2 lands, the switch is two edits, no new deploy machinery:

1. `.env`: set `RAVENSCALE_IMAGE` to the fork image.
2. `config/control-plane.yaml.tmpl`: point the `database` block at `http://sqld:8080`.

Then `./upgrade.sh`.

## Security notes

- **`HS_SERVER_URL`** must be the URL clients actually reach (use `https://` in
  production behind your TLS terminator) and its host must differ from
  `HS_BASE_DOMAIN` (Headscale hard-requires this; `install.sh` checks it).
- **Datastore auth** is OFF by default and `sqld` is bound to loopback
  (`SQLD_BIND=127.0.0.1:8081`). Before exposing `sqld` to anything but the app,
  set a real `SQLD_AUTH_JWT_KEY` (Ed25519) in `.env`. An *empty* key is treated
  as "open", not "auth on" — do not rely on an empty value for protection.
- **Metrics** (`:9090`) are bound to loopback by default; keep them private.

## Common failures

| Symptom | Cause / fix |
|---------|-------------|
| `install.sh` refuses: S3 creds empty | Fill `S3_ACCESS_KEY_ID`/`S3_SECRET_ACCESS_KEY` in `.env`, or run `--dev`. |
| `HS_SERVER_URL host must not contain HS_BASE_DOMAIN` | They share a domain; make `HS_BASE_DOMAIN` a distinct suffix (e.g. `ravenscale.internal`). |
| Datastore health never comes up | `docker logs ravenscale-sqld-1`. If bottomless can't reach S3, check `S3_ENDPOINT`/creds/bucket exists. |
| Every `sqld` query returns `401` | An empty-but-present `SQLD_AUTH_JWT_KEY`. The scripts strip the empty line; if you hand-edit `.env`, remove the empty assignment or set a real key. |
| Control plane health never comes up | `docker logs ravenscale-control-plane-1`. Usually a bad `config/control-plane.yaml` — re-run `install.sh` to re-render. |
| Upgrade aborts at the backup gate | Correct behavior: backups aren't restorable. Do **not** bypass — fix S3/bottomless first. |
