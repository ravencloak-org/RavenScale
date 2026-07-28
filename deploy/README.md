# deploy/ — RavenScale install, upgrade & ops (P0-5)

One-command deploy of the full RavenScale stack (control plane + libSQL
datastore, ADR-0005) and a backup-gated upgrade path. Ops procedures live in
[`RUNBOOK.md`](RUNBOOK.md).

```bash
cp .env.example .env    # edit HS_SERVER_URL, HS_BASE_DOMAIN, S3_* for production
./install.sh            # or: ./install.sh --dev  (hermetic, bundled MinIO)
./upgrade.sh            # backup-gated, health-gated in-place upgrade
```

## Layout

| Path | What |
|------|------|
| `install.sh` | One-command deploy. Preflight → render config → up → health-gate. Idempotent. |
| `upgrade.sh` | In-place upgrade: backup-verify gate → pull → recreate changed → health-gate. |
| `docker-compose.yml` | Full stack: control-plane service + `include` of the P0-3 datastore unit. |
| `compose.dev.yml` | Dev overlay: bundled MinIO so the stack runs with only Docker. |
| `config/control-plane.yaml.tmpl` | Control-plane config template (rendered by the scripts). |
| `.env.example` | All config; copy to `.env` (gitignored). |
| `scripts/wait-health.sh` | Shared HTTP health poller. |
| `datastore/` | The P0-3 libSQL datastore unit (sqld + WAL→S3 backup, restore drill). |

## Datastore seam

The control plane defaults to **upstream Headscale** as a health-checkable
stand-in until **P0-2** ships the RavenScale fork image (which binds GORM to
`sqld`). The deploy machinery is real and tested today; swapping to the fork is
two edits — see [`RUNBOOK.md` → Datastore seam](RUNBOOK.md#datastore-seam-why-the-control-plane-runs-on-sqlite-today).

## Verified

`./install.sh --dev` on a clean machine (Docker 29.6.1) brings both services to
`/health`; the control plane issues users/pre-auth keys; `sqld` replicates WAL
to S3 (6.6 MiB generation observed in MinIO); `./upgrade.sh --dev` passes the
`bottomless-cli verify` gate and preserves data across a container replacement.
