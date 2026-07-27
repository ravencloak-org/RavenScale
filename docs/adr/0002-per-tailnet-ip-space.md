# ADR-0002 — Per-Tailnet IP address space

- Status: Accepted
- Date: 2026-07-27
- Deciders: Jobin, Monica (design grill session 1)

## Context

Headscale assigns each node an address from `100.64.0.0/10` (CGNAT range, ~4M addresses). In a multi-tenant world we must decide whether that pool is shared across all Tailnets or independent per Tailnet. Addresses get cached on nodes, so this is hard to reverse once devices register.

## Decision

Each **Tailnet gets its own independent `100.64.0.0/10`.** Nodes in different Tailnets may hold identical addresses; because Tailnets are structurally isolated (ADR-0001 + app-layer scoping), they never route to each other.

## Consequences

- Full address range available per tenant — no cross-tenant exhaustion.
- Isolation by construction: an address is only meaningful within its Tailnet.
- Address allocation logic keys on `(tailnet_id, addr)`, not `addr` alone.
- Matches Tailscale behaviour.
