# Agent Assignments

In-repo source of truth for **proposed** ownership of the RavenScale backlog. The live tracker is the Roadmap board, [Project #4](https://github.com/orgs/ravencloak-org/projects/4) — this file is the durable mapping; the board reflects real-time status.

Owners are assigned by **role fit inferred from persona**, not from documented specialties. Treat every assignment below as a proposal to confirm with Jobin.

## Role → agent rationale

| Role | Proposed lead(s) | Rationale |
|------|------------------|-----------|
| Architecture / tech lead | Richard | Founder/architect archetype — owns the ROOT data model and protocol-shaped work |
| Backend | Dinesh | Core control-plane Go engineering |
| Infra / SRE | Gilfoyle | Systems/infra + security architect — datastore, HA, pipelines |
| Security | Gilfoyle | Most security-minded persona (TKA, audit integrity, session recording) |
| Frontend | Dinesh | Full-stack engineer — also owns the admin dashboard |
| QA | Erlich | Owns the client-compat harness / test gate |
| Verification / test | Gavin ⚠ | Cross-cutting — tests and verifies delivered work before an issue closes. **Not yet in the active-agents roster; must be added/created before he can be assigned** |
| Product | Monica | Product/partner archetype and a design-grill decider |
| Legal / ops | Jared | Operations/business persona — BSD-3 + trademark guardrail |
| Mobile | Jian-Yang | App/mobile builder — deferred client fork |

## Proposed owners

| Issue | id | Role | Proposed owner (confirm with Jobin) | Depends |
|-------|-----|------|-------------------------------------|---------|
| #1 | P0-1 | legal | Jared | none |
| #2 | P0-2 | backend | Richard (ROOT / architecture) | none |
| #3 | P0-3 | infra | Gilfoyle | none |
| #4 | P0-4 | qa | Erlich | #2 |
| #5 | P0-5 | infra | Gilfoyle | none |
| #6 | P1-1 | backend | Dinesh | #2 |
| #7 | P1-2 | backend | Richard (API design) | builds on #2 |
| #8 | P1-3 | frontend | Dinesh | #7 |
| #9 | P1-4 | backend | Dinesh | none stated |
| #10 | P1-5 | security | Gilfoyle | none stated |
| #11 | P1-6 | infra | Gilfoyle | #3 |
| #12 | P1-7 | backend | Dinesh | #7 |
| #13 | P2-1 | backend | Richard (capabilities/protocol root) | gate #4 |
| #14 | P2-2 | backend | Dinesh | gate #4 |
| #15 | P2-3 | backend | Dinesh | gate #4 |
| #16 | P2-4 | security | Gilfoyle | #13, gate #4 |
| #17 | P2-5 | security | Gilfoyle | #13, gate #4 |
| #18 | P2-6 | infra | Gilfoyle | #13, gate #4 |
| #19 | P2-7 | infra | Gilfoyle | gate #4 |
| #20 | P3-1 | product | Monica | none |
| #21 | P4-1 | mobile / architect | Jian-Yang (with Richard on protocol/architecture) | #20 trigger |

**Verification (cross-cutting):** Gavin tests and verifies each deliverable before its issue closes. ⚠ Gavin is not yet in the active-agents roster (`AGENTS.md`) — he must be added/created before he can be assigned on GitHub.

**Bench / unassigned:** Big Head, Bumble, Fizz, Honey, Russ Hanneman — available as reviewers or extra capacity. Gilfoyle is proposed on 8 issues (all infra + all security); rebalancing toward the bench is worth considering. Russ Hanneman is a natural product-side voice on the Phase-3 gate (#20).

## Claim protocol

An issue is **claimed** when an agent:

1. **Self-assigns** on the GitHub issue.
2. **Moves its card to _In Progress_** on Project #4.
3. **Respects `Depends`** — do not start a task until its prerequisite issues are closed.
4. For **Phase-2** issues, treats the `P0-4` (#4) client-compat harness as a **merge gate** — no merge until it passes.
5. Before **close**, the deliverable is verified/tested by **Gavin** against the issue's acceptance criteria.

Report per task: owner, status, blockers, and acceptance-criteria evidence. If work assumed "server-side only" turns out to need a client change, **escalate to PRODUCT** — that is a Phase-3 signal.

## References

- Backlog & principles: [`buzzorchestratorbrief.md`](../buzzorchestratorbrief.md)
- Terminology & locked decisions: [`CONTEXT.md`](../CONTEXT.md)
- Phases & dependencies: [`BUILD_ORDER.md`](../BUILD_ORDER.md)
- ADRs: [`docs/adr/`](./adr)
- Wiki mirror of this mapping: `Team` page in the [project wiki](https://github.com/ravencloak-org/RavenScale/wiki/Team)

> Note: the datastore engine for `P0-3` (#3) is **unresolved** (brief: Postgres; grill: SQLite/libSQL). Confirm with Jobin before that issue starts.
