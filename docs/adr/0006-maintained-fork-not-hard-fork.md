# ADR-0006: Maintained fork of Headscale, not a hard fork

Status: Accepted (2026-08-12) · Decides [#30](https://github.com/ravencloak-org/RavenScale/issues/30) · Map [#25](https://github.com/ravencloak-org/RavenScale/issues/25)

## Context

RavenScale forks `juanfont/headscale` to add multi-tenancy. The tenant changes
are invasive (new tables, `tenant_id`/`tailnet_id` columns, a de-singletoned IP
allocator, tenant stamping inside `state.go`). Headscale ships upstream security
and protocol fixes on its own cadence, and P0-4 maintains a client-compat
harness. We must decide how much of upstream we keep receiving.

## Decision

Maintain an **upstream-tracking fork**, not a hard fork.

- **Go module path stays `github.com/juanfont/headscale`.** Imports are untouched
  so `git merge upstream/main` stays low-conflict. A module-path rename was
  rejected: it rewrites the import line in every `.go` file and turns every
  upstream sync into an all-files conflict.
- **New identity is brand-layer only** — repo name, CLI binary, Docker image,
  config keys, user-facing product name. None of these touch `go.mod`.
- **Upstream tracked** via a live `upstream` remote. A scheduled CI job fetches
  upstream, attempts the merge, and runs the P0-4 compat harness: green → auto-PR
  for review; conflict or red → a human resolves.
- **P0-4 harness doubles as the upstream-sync merge gate**, in addition to its
  Phase-2 role.

## Consequences

- Upstream security/protocol fixes flow in with modest effort; the invasive
  tenant surface will conflict whenever upstream edits those files, and those
  conflicts are resolved at merge time behind the harness gate.
- Fully-hands-off auto-merge is explicitly **not** a goal — automation is
  best-effort and harness-gated (the accepted ceiling).
- Publishing under an org-owned module path and "owning the tree" are given up
  for Phase 0–1; revisit only if divergence grows so large that tracking upstream
  stops paying off (that would be a fresh decision, effectively a hard fork).
