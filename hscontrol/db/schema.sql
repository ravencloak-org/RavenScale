-- This file is the representation of the SQLite schema of Headscale.
-- It is the "source of truth" and is used to validate any migrations
-- that are run against the database to ensure it ends in the expected state.

CREATE TABLE migrations(id text,PRIMARY KEY(id));

-- RavenScale multi-tenancy (ADR-0001): Tenant = Org (isolation boundary),
-- owns 1..* Tailnets. Every tenant-scoped table carries a non-null tenant_id
-- (the Org) and, where it lives in a network namespace, a tailnet_id.
CREATE TABLE tenants(
  id integer PRIMARY KEY AUTOINCREMENT,
  name text,

  created_at datetime,
  updated_at datetime,
  deleted_at datetime
);
CREATE INDEX `idx_tenants_deleted_at` ON `tenants`(`deleted_at`);

CREATE TABLE tailnets(
  id integer PRIMARY KEY AUTOINCREMENT,
  tenant_id integer NOT NULL,
  slug text,
  name text,

  created_at datetime,
  updated_at datetime,
  deleted_at datetime
);
CREATE INDEX `idx_tailnets_deleted_at` ON `tailnets`(`deleted_at`);
CREATE INDEX `idx_tailnets_tenant_id` ON `tailnets`(`tenant_id`);
CREATE UNIQUE INDEX `idx_tailnets_slug` ON `tailnets`(`slug`);

CREATE TABLE users(
  id integer PRIMARY KEY AUTOINCREMENT,
  tenant_id integer NOT NULL DEFAULT 1,
  name text,
  display_name text,
  email text,
  provider_identifier text,
  provider text,
  profile_pic_url text,

  created_at datetime,
  updated_at datetime,
  deleted_at datetime
);
CREATE INDEX idx_users_deleted_at ON users(deleted_at);
CREATE INDEX `idx_users_tenant_id` ON `users`(`tenant_id`);


-- The following three UNIQUE indexes work together to enforce the user identity model:
--
-- 1. Users can be either local (provider_identifier is NULL) or from external providers (provider_identifier set)
-- 2. Each external provider identifier must be unique across the system
-- 3. Local usernames must be unique among local users
-- 4. The same username can exist across different providers with different identifiers
--
-- Examples:
-- - Can create local user "alice" (provider_identifier=NULL)
-- - Can create external user "alice" with GitHub (name="alice", provider_identifier="alice_github")
-- - Can create external user "alice" with Google (name="alice", provider_identifier="alice_google")
-- - Cannot create another local user "alice" (blocked by idx_name_no_provider_identifier)
-- - Cannot create another user with provider_identifier="alice_github" (blocked by idx_provider_identifier)
-- - Cannot create user "bob" with provider_identifier="alice_github" (blocked by idx_name_provider_identifier)
CREATE UNIQUE INDEX idx_provider_identifier ON users(provider_identifier) WHERE provider_identifier IS NOT NULL;
CREATE UNIQUE INDEX idx_name_provider_identifier ON users(name, provider_identifier);
CREATE UNIQUE INDEX idx_name_no_provider_identifier ON users(name) WHERE provider_identifier IS NULL;

CREATE TABLE pre_auth_keys(
  id integer PRIMARY KEY AUTOINCREMENT,
  tenant_id integer NOT NULL DEFAULT 1,
  tailnet_id integer NOT NULL DEFAULT 1,
  key text,
  prefix text,
  hash blob,
  user_id integer,
  reusable numeric,
  ephemeral numeric DEFAULT false,
  used numeric DEFAULT false,
  tags text,
  expiration datetime,

  created_at datetime,

  CONSTRAINT fk_pre_auth_keys_user FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE SET NULL
);
CREATE UNIQUE INDEX idx_pre_auth_keys_prefix ON pre_auth_keys(prefix) WHERE prefix IS NOT NULL AND prefix != '';
CREATE INDEX `idx_pre_auth_keys_tenant_id` ON `pre_auth_keys`(`tenant_id`);
CREATE INDEX `idx_pre_auth_keys_tailnet_id` ON `pre_auth_keys`(`tailnet_id`);

CREATE TABLE api_keys(
  id integer PRIMARY KEY AUTOINCREMENT,
  prefix text,
  hash blob,
  expiration datetime,
  last_seen datetime,

  created_at datetime
);
CREATE UNIQUE INDEX idx_api_keys_prefix ON api_keys(prefix);

CREATE TABLE nodes(
  id integer PRIMARY KEY AUTOINCREMENT,
  tenant_id integer NOT NULL DEFAULT 1,
  tailnet_id integer NOT NULL DEFAULT 1,
  machine_key text,
  node_key text,
  disco_key text,

  endpoints text,
  host_info text,
  ipv4 text,
  ipv6 text,
  hostname text,
  given_name varchar(63),
  -- user_id is NULL for tagged nodes (owned by tags, not a user).
  -- Only set for user-owned nodes (no tags).
  user_id integer,
  register_method text,
  tags text,
  auth_key_id integer,
  last_seen datetime,
  expiry datetime,
  approved_routes text,

  created_at datetime,
  updated_at datetime,
  deleted_at datetime,

  CONSTRAINT fk_nodes_user FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_nodes_auth_key FOREIGN KEY(auth_key_id) REFERENCES pre_auth_keys(id)
);
CREATE INDEX `idx_nodes_tenant_id` ON `nodes`(`tenant_id`);
CREATE INDEX `idx_nodes_tailnet_id` ON `nodes`(`tailnet_id`);

CREATE TABLE policies(
  id integer PRIMARY KEY AUTOINCREMENT,
  tenant_id integer NOT NULL DEFAULT 1,
  tailnet_id integer NOT NULL DEFAULT 1,
  data text,

  created_at datetime,
  updated_at datetime,
  deleted_at datetime
);
CREATE INDEX idx_policies_deleted_at ON policies(deleted_at);
CREATE INDEX `idx_policies_tenant_id` ON `policies`(`tenant_id`);
CREATE INDEX `idx_policies_tailnet_id` ON `policies`(`tailnet_id`);

CREATE TABLE database_versions(
  id integer PRIMARY KEY,
  version text NOT NULL,
  updated_at datetime
);
