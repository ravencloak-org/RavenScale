# RavenScale — Build Order & Phases

Phases and backlog IDs come from `buzzorchestratorbrief.md` (the orchestrator brief, authoritative). Where the design-grill resolved a hard-to-reverse call, the item links to `CONTEXT.md` / an ADR under `docs/adr/`. Sequencing rule from the brief: cheapest, zero-client-risk work first; never start a task before its `Depends` items are done; do not begin Phase 4 until a Phase-3 trigger fires.

GitHub milestones mirror these phases; every backlog item below is tracked as a GitHub issue with its role label and dependency links.

## Phase 0 — Fork & harden base · milestone: `Phase 0 — Fork & harden base`

Foundation. The data model (P0-2) is the root nobody can route around once real data exists.

- **P0-1** [legal] Confirm BSD-3 obligations for fork + redistribution; flag trademark boundaries (no "Tailscale" branding). AC: written do/don't guardrail doc.
- **P0-2** [backend] Fork Headscale; remove single-tailnet scope at the data-model level. AC: multi-tenant-ready schema, builds green. → design: `CONTEXT.md` §1–2, ADR-0001; N=1 migration ADR-0003.
- **P0-3** [infra] Production datastore + migrations + backup/restore. AC: documented restore drill passes. → **Resolved: libSQL single-node + WAL→S3 backup, no cluster** (ADR-0005). Revisit clustering/Postgres at HA #11.
- **P0-4** [qa] Client-compat test harness pinned to specific official Tailscale client versions; run in CI. AC: CI red on protocol drift. Depends: P0-2.
- **P0-5** [infra] Clean install/upgrade tooling + ops runbook. AC: one-command deploy + upgrade path.

## Phase 1 — Server-side differentiation (the wedge) · milestone: `Phase 1 — Server-side differentiation`

Pure control-plane, no client risk. Ends at the first sellable release.

- **P1-1** [backend] Multi-tenancy: multiple isolated tailnets. Depends: P0-2. → design: `CONTEXT.md` §3–8, ADR-0002 (per-Tailnet IP), ADR-0004 (slug/MagicDNS).
- **P1-2** [backend] Full control/management REST API at parity target.
- **P1-3** [frontend] Official admin dashboard on the control API. Depends: P1-2.
- **P1-4** [backend] IdP group sync into ACLs (close Headscale's OIDC-groups gap).
- **P1-5** [security] Audit logs of control-plane actions, tamper-evident.
- **P1-6** [infra] Control-plane clustering / HA. Depends: P0-3.
- **P1-7** [backend] Kubernetes operator (adapt/ship). Depends: P1-2.

> MILESTONE: first sellable release. PRODUCT lines up design partners.

## Phase 2 — Client-supported features · milestone: `Phase 2 — Client-supported features`

Capabilities + infra. Every task must pass the P0-4 compat harness before merge.

- **P2-1** [backend] Emit signed node capabilities; wire Tailscale Serve.
- **P2-2** [backend] App Connectors (routing orchestration).
- **P2-3** [backend] Device posture checks (consume client posture, enforce policy).
- **P2-4** [security] Tailnet Lock: TKA protocol server-side. Depends: P2-1.
- **P2-5** [security] SSH session recording + recorder endpoint. Depends: P2-1.
- **P2-6** [infra] Flow-log ingestion pipeline. Depends: P2-1.
- **P2-7** [infra] Funnel-equivalent: own public ingress relays + domain to replace `*.ts.net`. OPTIONAL — only if a customer needs it.

## Phase 3 — Decision gate (PRODUCT-owned) · milestone: `Phase 3 — Decision gate`

- **P3-1** [product] Track the three client-fork triggers: (a) a paid feature needs app-UI change, (b) protocol-drift maintenance cost exceeds fork cost, (c) trademark blocks an enterprise deal. AC: monthly trigger report; Phase 4 starts only when one fires. Until then, HOLD Phase 4.

## Phase 4 — Own the client (deferred) · milestone: `Phase 4 — Own the client`

- **P4-1** [mobile] Fork open-source tailscaled; build branded GUI + mobile apps; unlock truly-locked features. Cut the client dependency. Blocked-by: P3-1 trigger.

## Reconciliation (resolved)

- **Datastore — RESOLVED 2026-07-28: libSQL single-node + WAL→S3 backup, no cluster** (ADR-0005; `RESEARCH/DATASTORE_LIBSQL_VS_POSTGRES.md`). Postgres line in the brief for P0-3 is superseded. Tenant isolation stays app-layer (no RLS). Reversible via GORM dialect abstraction — revisit clustering (LiteFS/rqlite) or Postgres at HA #11 with real load data. The remaining design-grill ceilings (DB-per-tenant, external per-tenant IdP, cross-Tailnet devices) are deferred and additive; see `CONTEXT.md` "Known ceilings".
