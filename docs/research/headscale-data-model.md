# Headscale Data Model + Single-Tailnet Scope Map

Research for ticket #26. Upstream target: `github.com/juanfont/headscale`
@ commit `565fd254` (main, 2026-07-29), Go 1.26. GORM + SQLite/Postgres.

Ground truth for tables/columns is `hscontrol/db/schema.sql` (the repo's
canonical schema used to validate migrations). GORM structs live in
`hscontrol/types/`.

## TL;DR

Headscale is single-tenant by construction: **one global IP pool, one global
ACL policy row, one MagicDNS base domain, and globally-unique usernames** — no
`tenant_id`/`tailnet_id` column exists anywhere. To become multi-tailnet every
core table needs a non-null `tenant_id` (Org) + `tailnet_id`, and four
instance-global code paths must be reworked to be tailnet-scoped: IP allocation,
user uniqueness, policy loading, and the MagicDNS suffix.

## Models / Tables (current)

| Model (struct) | Table | Key columns | File |
|---|---|---|---|
| `User` | `users` | id, name, display_name, email, provider_identifier, provider, profile_pic_url, timestamps | `hscontrol/types/users.go:65` |
| `PreAuthKey` | `pre_auth_keys` | id, key, prefix, hash, **user_id (FK)**, description, reusable, ephemeral, used, tags, expiration, revoked, created_at | `hscontrol/types/preauth_key.go:38` |
| `APIKey` | `api_keys` | id, prefix, hash, **user_id**, expiration, last_seen, created_at | `hscontrol/types/api_key.go` |
| `OAuthClient` | `oauth_clients` | id, client_id, secret_hash, scopes, tags, description, **user_id**, created_at, revoked | `hscontrol/types/oauth.go` |
| `OAuthAccessToken` | `oauth_access_tokens` | id, prefix, hash, client_id, scopes, tags, expiration, created_at | `hscontrol/types/oauth.go` |
| `Node` | `nodes` | id, machine_key, node_key, disco_key, endpoints, host_info, **ipv4, ipv6**, hostname, given_name, **user_id (FK, NULL for tagged)**, register_method, tags, auth_key_id, last_seen, expiry, **approved_routes**, timestamps | `hscontrol/types/node.go:121` |
| `Policy` | `policies` | id, **data** (whole HuJSON ACL doc), timestamps | `hscontrol/types/policy.go` |
| `DatabaseVersion` | `database_versions` | id, version, updated_at | `hscontrol/db/` |
| (migrations) | `migrations` | id | GORM/atlas bookkeeping |

Notes:
- **Routes have no table.** A node's approved routes are a JSON column
  `nodes.approved_routes` (`Node.ApprovedRoutes`, `node.go:177`). `types.Route`
  (`routes.go:11`) is an in-memory view, not persisted. ACL/auto-approval lives
  in the single `policies.data` blob.
- **DNS / MagicDNS has no table.** The suffix is config-only
  (`dns.base_domain`), see below.
- **IP pool has no table.** Prefixes are config (`prefix.v4`/`prefix.v6`);
  allocation state is derived at boot by scanning `nodes.ipv4/ipv6`.
- FKs: `nodes.user_id → users.id` (ON DELETE CASCADE),
  `nodes.auth_key_id → pre_auth_keys.id`,
  `pre_auth_keys.user_id → users.id` (ON DELETE SET NULL).
- `TaggedDevicesUserID = 2147455555` — a hardcoded global sentinel "user" for
  tagged nodes (`users.go:36`); it too is instance-global.

## Where a single global tailnet is assumed

1. **Global IP allocation — `hscontrol/db/ip.go`.**
   `IPAllocator` is an explicit singleton: *"There can only be one and it needs
   to be created before any other database writes occur"* (`ip.go:26-31`).
   `NewIPAllocator` (`ip.go:57`) plucks **every** `nodes.ipv4`/`ipv6` in the DB
   into one used-IP set and hands addresses from the single config prefix
   (`prefix4/prefix6`). IP uniqueness is enforced instance-wide, not per tailnet.
   `BackfillNodeIPs` (`ip.go:306`) iterates all nodes globally.

2. **User uniqueness — `hscontrol/db/schema.sql:37-39`.**
   Three global `UNIQUE` indexes: `idx_provider_identifier`,
   `idx_name_provider_identifier (name, provider_identifier)`,
   `idx_name_no_provider_identifier (name WHERE provider_identifier IS NULL)`.
   Uniqueness is enforced across the whole instance (see also the doc comment on
   `User.Name`, `users.go:76`). Two tailnets cannot both have a local user
   "alice".

3. **Policy loading — `hscontrol/db/policy.go`.**
   `GetPolicy` runs `SELECT * FROM policies ORDER BY id DESC LIMIT 1`
   (`policy.go:36-54`): the newest single row is *the* policy for the entire
   server. `PolicyBytes` (`policy.go:60`) similarly returns one global doc
   (file-mode reads one config path). The `policies` table has no owner column.

4. **MagicDNS suffix — `hscontrol/types/config.go`.**
   `DNSConfig.BaseDomain` is a single global string (`config.go:119, 159`);
   `cfg.Domains = []string{dns.BaseDomain}` (`config.go:1006-1007`). One
   MagicDNS suffix serves the whole instance. `dns/extrarecords.go` and the
   mapper build DNS from this one config, not per-tailnet.

5. **Node ownership is user-scoped only.** `nodes.user_id` ties a node to a
   user, but there is no tenant/tailnet above the user. The mapper/poll build
   peer lists from all nodes filtered by policy, assuming one shared tailnet
   graph.

## Change table → add non-null `tenant_id` (Org) + `tailnet_id`

Add two non-null columns (`tenant_id`, `tailnet_id`) to every owned table and
re-scope the global code paths. Composite uniqueness and FKs must include
`tailnet_id`.

| Table | Columns to add | Additional change needed |
|---|---|---|
| `users` | `tenant_id`, `tailnet_id` NOT NULL | Rewrite the 3 UNIQUE indexes to be `(tailnet_id, name, provider_identifier)` etc. so usernames are unique **per tailnet**. |
| `pre_auth_keys` | `tenant_id`, `tailnet_id` NOT NULL | Scope key lookup by prefix within tailnet; FK `user_id` must reference a user in the same tailnet. |
| `api_keys` | `tenant_id`, `tailnet_id` NOT NULL | Scope API auth to its tailnet/org; admin vs tailnet-scoped keys. |
| `oauth_clients` / `oauth_access_tokens` | `tenant_id`, `tailnet_id` NOT NULL | Client-credentials scoped to a tailnet. |
| `nodes` | `tenant_id`, `tailnet_id` NOT NULL | **IP uniqueness becomes per-tailnet**: `IPAllocator` must be per-tailnet (or key its used-set by tailnet_id); `NewIPAllocator`/`Next`/`BackfillNodeIPs` scoped by tailnet. `given_name`/MagicDNS names unique per tailnet. |
| `policies` | `tenant_id`, `tailnet_id` NOT NULL | `GetPolicy` becomes `WHERE tailnet_id = ? ORDER BY id DESC LIMIT 1` — one policy **per tailnet**. `PolicyBytes` and the policy manager keyed by tailnet. |
| new `tailnets` table | id, `tenant_id`, name, **base_domain**, ip_prefix_v4, ip_prefix_v6, magic_dns, created_at | Promote today's instance-global config (`dns.base_domain`, `prefix.v4/v6`, `dns.magic_dns`) into per-tailnet rows so each tailnet gets its own MagicDNS suffix and IP pool. |
| new `tenants`/orgs table | id, name, ... | Top-level Org owning tailnets; `tenant_id` FK target. |
| `database_versions` / `migrations` | (none) | Instance-global; leave unscoped. |

### Code paths to re-scope (beyond columns)
- `hscontrol/db/ip.go` — one `IPAllocator` per tailnet (or tailnet-keyed used-set); prefixes read from the `tailnets` row, not global config.
- `hscontrol/db/policy.go` `GetPolicy`/`PolicyBytes` + `hscontrol/policy/` manager — load/compile policy per tailnet.
- `hscontrol/db/schema.sql` user unique indexes — prefix all three with `tailnet_id`.
- `hscontrol/types/config.go` `BaseDomain`/`Domains`/prefixes — move into per-tailnet storage; MagicDNS suffix resolved per tailnet in `dns/` + `mapper/`.
- `hscontrol/mapper` + `hscontrol/poll` — filter peer graph by `tailnet_id`; `TaggedDevicesUserID` sentinel must be namespaced per tailnet.
- Every `db` query that fetches users/nodes/keys must carry a `tailnet_id` filter to prevent cross-tailnet leakage.
