# ADR-0009: Node MachineKey stays globally unique in Phase 1

Status: Accepted (2026-08-12) · Decides [#33](https://github.com/ravencloak-org/RavenScale/issues/33) · **Supersedes design decision #8 (unique-within-Tailnet)** · Map [#25](https://github.com/ravencloak-org/RavenScale/issues/25)

## Context

Design decision #8 stated node WireGuard pubkeys should be unique **within a
Tailnet**, treating a device in two Tailnets as merely "out of Phase-1 scope."
Investigation of Headscale's registration flow showed this framing is wrong:

- The official Tailscale client identifies a node by its **MachineKey alone** —
  there is no Tailnet selector in the protocol, and Tailscale owns that client
  (operating principle #3), so we cannot add one.
- The known-node **reconnect lookup runs by MachineKey in the Noise handshake,
  before any credential** (and therefore before any Tailnet) is presented.

A per-Tailnet uniqueness constraint would make that pre-credential lookup
ambiguous the moment one MachineKey exists in two Tailnets — a latent bug the
client cannot help resolve.

## Decision

**MachineKey uniqueness stays global in Phase 1.**

- Uniqueness is app-enforced (lookup-before-insert), as upstream. The constraint
  is **not** composited to `(tailnet_id, machine_key)`.
- The `tenant_id`/`tailnet_id` columns are still added (queries scope), so nothing
  about the schema blocks a future cross-Tailnet-device case.
- **Exactly one deliberate global lookup survives**: the pre-credential
  known-node MachineKey lookup in the Noise handshake. Everything downstream
  (peer lists, mapresponse, node-by-id, policy) is tenant-scoped via ADR-0007.
- **Cross-Tailnet same-device is reclassified** from "deferred" to a
  **client-fork trigger candidate** (a feature needing a client change), tracked
  on P3-1 ([#20](https://github.com/ravencloak-org/RavenScale/issues/20)) — not a
  Phase-1 data-model choice.

## Consequences

- The reconnect path stays simple and matches upstream; no protocol change needed.
- One physical device cannot join two Tailnets until the client-fork trigger
  fires and RavenScale owns the client; the schema already carries the columns for
  that day.
- **The wiki Architecture page decision #8 is corrected** to match this ADR
  (constraint global, not within-Tailnet).
