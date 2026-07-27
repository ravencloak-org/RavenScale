# ADR-0004 — Immutable Tailnet slug as MagicDNS suffix

- Status: Accepted
- Date: 2026-07-27
- Deciders: Jobin, Monica (design grill session 1)

## Context

Each Tailnet needs a stable external identity that also drives MagicDNS. Names get baked into node configs and DNS, so the external handle is hard to reverse once in use.

## Decision

Each Tailnet gets an **immutable slug** at creation. MagicDNS suffix = `<host>.<tailnet-slug>.<base-domain>`, where `base-domain` is configured per deployment. The numeric primary key stays internal; the slug is the external handle everywhere (DNS, URLs, API).

Rename is *allowed* but treated as a rare, expensive operation because it breaks existing DNS.

## Consequences

- Slug uniqueness is enforced per base-domain.
- DNS names never collide across Tailnets — the slug is the namespace.
- Internal PK churn (re-keying, sharding) never leaks to users, who only ever see the slug.
