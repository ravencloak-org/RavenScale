# RavenScale

A self-hosted, multi-tenant control plane for [Tailscale](https://tailscale.com)
clients — built by forking [Headscale](https://github.com/juanfont/headscale)
(BSD-3-licensed). The bet: reach ~90% of Tailscale's feature set **without**
forking the client, ship revenue-generating differentiation fast, and defer the
expensive client fork until a paying customer forces it.

## Operating principles

1. Cheapest, zero-client-risk work first — pure control-plane features before
   anything that touches the client.
2. Never fork the client until a Phase-3 trigger fires.
3. Every "client-supported" feature depends on the official client honoring
   server-granted node capabilities. Tailscale owns that release train, so a
   version-pinned compatibility test harness is the early-warning system.
4. Prefer shipping a smaller thing to production over a bigger thing in progress.

## Roadmap (phased)

| Phase | Theme | Client risk |
|-------|-------|-------------|
| 0 | Fork & harden base (multi-tenant schema, libSQL single-node, compat harness) | none |
| 1 | Pure server-side differentiation — the wedge (multi-tenancy, control API, admin UI, IdP group sync, audit logs, HA, k8s operator) | none |
| 2 | Light up client-supported features via signed node capabilities (Serve, App Connectors, posture, Tailnet Lock, SSH recording, flow logs) | low — gated by compat harness |
| 3 | Decision gate — track the three client-fork triggers (PRODUCT-owned) | — |
| 4 | Own the client — fork `tailscaled`, branded GUI/mobile (staffs **only** on a Phase-3 trigger) | high |

The authoritative backlog, dependencies, and acceptance criteria live in
[`buzzorchestratorbrief.md`](./buzzorchestratorbrief.md).

## Documentation

- `CONTEXT.md` — domain glossary (canonical terms). Created as terms are resolved.
- `docs/adr/` — architecture decision records. Created as hard-to-reverse decisions are made.

Branding note: no "Tailscale" trademark usage. BSD-3 obligations tracked in Phase 0 (P0-1).
