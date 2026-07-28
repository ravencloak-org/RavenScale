# ORCHESTRATOR BRIEF: "Headscale Fork → Tailscale Competitor"

## MISSION
We are forking Headscale (BSD-3-licensed, open-source Tailscale control server)
to build a self-hosted, multi-tenant, enterprise-grade control plane that is
compatible with the official Tailscale clients. Goal: reach ~90% of Tailscale's
feature set WITHOUT forking the client, ship revenue-generating differentiation
fast, and defer the expensive client fork until a paying customer forces it.

## OPERATING PRINCIPLES (enforce these when sequencing)
1. Cheapest, zero-client-risk work first. Pure control-plane features before
   anything that touches the client.
2. Never fork the client until a Phase-3 trigger fires (see gate).
3. Every "client-supported" feature depends on the official client honoring
   server-granted node capabilities — Tailscale owns that release train, so
   maintain a version-pinned compatibility test harness as the early-warning system.
4. Prefer shipping a smaller thing to production over a bigger thing in progress.

## AGENT ROLES AVAILABLE (assign each task to the best-fit role)
- ARCHITECT (tech lead: sequencing, protocol/TKA design, cross-cutting decisions)
- BACKEND (control-plane Go engineering: forked Headscale core, API, capabilities)
- INFRA/SRE (libSQL/SQLite datastore, HA/clustering, DERP/ingress relays, log pipelines, CI)
- FRONTEND (admin dashboard, web UI)
- MOBILE (deferred — only staffs in Phase 4)
- SECURITY (Tailnet Lock/TKA, posture, SSH session recording, audit integrity)
- QA (integration + client-compat test harness, release gating)
- PRODUCT (customer triggers, prioritization, Phase-3 decision data)
- LEGAL (BSD-3 compliance, trademark/branding, redistribution review)

## BACKLOG (assign by tag; respect dependencies)

### PHASE 0 — Fork & harden base
P0-1  [LEGAL]     Confirm BSD-3 obligations for fork + redistribution; flag
                  trademark boundaries (no "Tailscale" branding). AC: written
                  do/don't guardrail doc.
P0-2  [BACKEND]   Fork Headscale; remove single-tailnet scope limitation at the
                  data-model level. AC: multi-tenant-ready schema, builds green.
P0-3  [INFRA]     Production datastore: libSQL single-node + WAL→S3 backup, migrations, backup/restore. (Was Postgres; resolved to libSQL — ADR-0005. Revisit clustering/Postgres at HA #11.)
                  AC: documented restore drill passes.
P0-4  [QA]        Build client-compat test harness pinned to specific official
                  Tailscale client versions; run integration suite in CI.
                  AC: CI red on protocol drift. (Depends: P0-2)
P0-5  [INFRA]     Clean install/upgrade tooling + ops runbook. AC: one-command
                  deploy + upgrade path.

### PHASE 1 — Pure server-side differentiation (the wedge; no client risk)
P1-1  [BACKEND]   Multi-tenancy: multiple isolated tailnets. (Depends: P0-2)
P1-2  [BACKEND]   Full control/management REST API at parity target.
P1-3  [FRONTEND]  Official admin dashboard on the control API. (Depends: P1-2)
P1-4  [BACKEND]   IdP group sync into ACLs (close Headscale's OIDC-groups gap).
P1-5  [SECURITY]  Audit logs (control-plane actions), tamper-evident.
P1-6  [INFRA]     Control-plane clustering / HA. (Depends: P0-3)
P1-7  [BACKEND]   Kubernetes operator (adapt/ship). (Depends: P1-2)
      >>> MILESTONE: first sellable release. PRODUCT to line up design partners.

### PHASE 2 — Light up client-supported features (capabilities + infra)
P2-1  [BACKEND]   Emit signed node capabilities; wire Tailscale Serve.
P2-2  [BACKEND]   App Connectors (routing orchestration).
P2-3  [BACKEND]   Device posture checks (consume client posture, enforce policy).
P2-4  [SECURITY]  Tailnet Lock: implement TKA protocol server-side. (Depends: P2-1)
P2-5  [SECURITY]  SSH session recording + recorder endpoint. (Depends: P2-1)
P2-6  [INFRA]     Flow-log ingestion pipeline. (Depends: P2-1)
P2-7  [INFRA]     Funnel-equivalent: own public ingress relays + domain to
                  replace *.ts.net. (OPTIONAL — only if a customer needs it.)
      >>> Each P2 task must pass P0-4 client-compat harness before merge.

### PHASE 3 — Decision gate (PRODUCT-owned; do not skip)
P3-1  [PRODUCT]   Track the three client-fork triggers: (a) a paid feature needs
                  app-UI change (e.g. mobile switcher/branding), (b) protocol-drift
                  maintenance cost exceeds fork cost, (c) trademark blocks an
                  enterprise deal. AC: monthly trigger report; Phase 4 starts only
                  when one fires. Until then, HOLD Phase 4.

### PHASE 4 — Own the client (staffs ONLY on a Phase-3 trigger)
P4-1  [MOBILE/ARCHITECT]  Fork open-source tailscaled; build branded GUI + mobile
                          apps; unlock truly-locked features (mobile switcher, UX).
                          Cut the client dependency. (Blocked-by: P3-1 trigger)

## ORCHESTRATION INSTRUCTIONS
- Assign tasks to agents by [TAG]. Parallelize within a phase where no dependency
  exists; do not start a task before its "Depends" items are done.
- Do NOT begin Phase 4 until PRODUCT reports a fired trigger (P3-1).
- Treat P0-4 (compat harness) as a merge gate for all Phase-2 work.
- Report per-task: owner, status, blockers, and acceptance-criteria evidence.
- Escalate any task where a feature that was assumed "server-side only" turns out
  to require a client change — that's a Phase-3 signal, route it to PRODUCT.
