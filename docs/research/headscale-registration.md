# Headscale registration + pre-auth key flow (RESEARCH #28)

Upstream traced: `github.com/juanfont/headscale` @ `565fd25` (paths below are upstream paths; RavenScale has no code yet).

Goal: locate the single registration endpoint, how pre-auth keys are minted/validated, where node→user binding happens, and the exact **injection points** for (a) tenant-scoped pre-auth keys (key encodes its Tailnet) and (b) a trusted `tenant_id` claim stamped at auth. Per decision #4 the tenant must be **derived from the credential, never from the URL**.

---

## 1. The single registration endpoint

All node registration enters through **one** authenticated Noise (ts2021) endpoint — there is no per-tenant URL, which is exactly why decision #4 (tenant-from-credential) is achievable cleanly.

- `hscontrol/noise.go:169` — `r.Post("/register", ns.RegistrationHandler)` (mounted on the Noise mux; the client is already cryptographically identified by its `MachineKey` = the Noise session peer).
- `hscontrol/noise.go:750` — `ns.headscale.handleRegister(req.Context(), regReq, ns.conn.Peer())`. **`ns.conn.Peer()` is the trusted `key.MachinePublic`** — cryptographic machine identity, not URL-derived.
- `hscontrol/auth.go:38` — `handleRegister(ctx, req tailcfg.RegisterRequest, machineKey key.MachinePublic)`. Central dispatcher. Order:
  1. Past-expiry / `Auth==nil` → logout / re-auth paths (`handleLogout`, `auth.go:157`).
  2. `req.Followup != ""` → `waitForFollowup` (`auth.go:302`).
  3. `isAuthKey(req)` (`auth.go:275`, true when `req.Auth.AuthKey != ""`) → **pre-auth key path** `handleRegisterWithAuthKey` (`auth.go:412`).
  4. else → **interactive/OIDC path** `handleRegisterInteractive` (`auth.go:476`).

Interactive/OIDC uses a second HTTP surface for the browser leg only (still not tenant-in-URL): `hscontrol/app.go:474` `GET /register/{auth_id}` and `app.go:479` `POST /register/confirm/{auth_id}`, backed by `hscontrol/oidc.go`. The `{auth_id}` is a server-minted opaque cache key (`types.NewAuthID`), not a tenant selector.

---

## 2. Pre-auth key: minting

- Model: `hscontrol/types/preauth_key.go:38` `type PreAuthKey struct`. Key fields: `ID`, `Prefix`, `Hash []byte` (bcrypt), `UserID *uint` + `User *User` (owner; **nil for tags-only/system keys**), `Reusable`, `Ephemeral`, `Used`, `Tags []string` (json), `Expiration`, `Revoked`. **There is no Tailnet/tenant column today.**
- Mint: `hscontrol/db/preauth_keys.go:70` `CreatePreAuthKey(tx, uid *UserID, reusable, ephemeral, expiration, aclTags)`.
  - Format constants `preauth_keys.go:60`: `authKeyPrefix = "hskey-auth-"`, 12-char prefix, 64-char secret. Wire key = `hskey-auth-{prefix}-{secret}` (`:110`).
  - Only the bcrypt **hash** of the secret is stored; `prefix` is the DB lookup handle (`:106-127`). Plaintext key returned once as `types.PreAuthKeyNew` (`:133`).
  - Must be **tagged OR user-owned** (`:79`, `ErrPreAuthKeyNotTaggedOrOwned`). Tags validated by `validateACLTags` (`:31`, requires `tag:` prefix).

## 3. Pre-auth key: validation

- Lookup: `hscontrol/db/preauth_keys.go:187` `findAuthKey(tx, keyStr)` → cut on `hskey-auth-`, `parsePrefixedKey` (`:238`) splits fixed-length prefix/secret, `First(&pak, "prefix = ?", prefix)` then `bcrypt.CompareHashAndPassword` (`:219-228`). Legacy plaintext keys matched on `key =` (`:199`).
- Usability: `hscontrol/types/preauth_key.go:89` `(*PreAuthKey).Validate()` — rejects `Revoked`, past `Expiration`, and (non-reusable) `Used`. Single-use consumption is atomic: `db/preauth_keys.go:433` `UsePreAuthKey` does `UPDATE ... WHERE id=? AND used=false` (race-safe).

---

## 4. node → user (tenant) binding

**Pre-auth key path** — `hscontrol/state/state.go:2458` `HandleNodeFromPreAuthKey(regReq, machineKey)` (called from `auth.go:416`):
- `GetPreAuthKey` → `pak` (`state.go:2466`). Validation gated at `state.go:2552` (`pak.Validate()`) for new nodes / key rotation / retag; existing same-machine re-registrations skip it (`:2532`).
- **Ownership is copied off the key**: new node created via `createAndSaveNewNode(newNodeParams{ User: pakUser, ..., PreAuthKey: pak })` (`state.go:2807`) where `pakUser = *pak.User` (`:2792`). Tags-only key ⇒ empty user ⇒ tagged node; the node's `UserID`/`Tags` come **from the key**, never from the request or URL (`:2638-2659`).
- `createAndSaveNewNode` at `state.go:1890`.

**Interactive/OIDC path** — user identity comes from the IdP claim, then node is bound:
- `hscontrol/oidc.go:222` `OIDCCallbackHandler` → `oidc.go:323` `createOrUpdateUserFromClaim(&claims)` (`oidc.go:620`) resolves/creates a `types.User` by `claims.Identifier()` (`GetUserByOIDCIdentifier`), `user.FromClaim(...)`.
- `oidc.go:866` `HandleNodeFromAuthPath(authID, userID, expiry, util.RegisterMethodOIDC)` (`state.go:2205`) pulls the cached `RegistrationData` (machineKey/nodeKey) and binds the node to that `userID`.
- User model: `hscontrol/types/users.go` — `Name`, `Email`, `ProviderIdentifier sql.NullString`, `Provider`. **No tenant/org column today.**

---

## 5. Injection points for multi-tenancy (decision #4: tenant from credential, never URL)

### A. Tenant-scoped pre-auth keys (key encodes its Tailnet)
1. **Schema**: add `TenantID` (FK) to `PreAuthKey` — `hscontrol/types/preauth_key.go:38`. Mirror onto `types.User` (`users.go`) and `types.Node`.
2. **Mint**: thread tenant into `CreatePreAuthKey` (`db/preauth_keys.go:70`) — stamp `TenantID` on the row; derive it from the authenticated admin/API caller's tenant, not a param the caller picks freely. (API/CLI mint call sites: `hscontrol/grpcv1`/`apiv1_preauthkeys` handlers.)
3. **Validate**: in `(*PreAuthKey).Validate()` (`preauth_key.go:89`) and/or `HandleNodeFromPreAuthKey` (`state.go:2458`) assert the key's tenant is active/consistent. No URL parsing involved — the key *is* the tenant assertion.
4. **Bind**: at the single stamping point `createAndSaveNewNode(newNodeParams{...PreAuthKey: pak})` (`state.go:2807`) copy `pak.TenantID` onto the node alongside `User`/`Tags` (`:2638-2659`). This is the one chokepoint where node tenant is set on the auth-key path.

### B. Trusted `tenant_id` claim stamped at auth (OIDC)
1. **Read claim**: extend `types.OIDCClaims` / `User.FromClaim` and `createOrUpdateUserFromClaim` (`oidc.go:620`) to read a configured `tenant_id` (or org/hd) claim from the verified ID token — trusted because it is inside the IdP-signed token, not the URL.
2. **Stamp**: set `user.TenantID` there; it flows to the node via `HandleNodeFromAuthPath(authID, userID, ...)` (`state.go:2205`), which already binds node→user. Optionally validate against `validateOIDCAllowedUsers` (`oidc.go:546`).
3. **Guard**: the `authID` in `/register/{auth_id}` is a server-minted cache key, so no tenant ever comes from that URL — keep it that way.

### The two chokepoints to enforce tenant isolation
- **Auth-key nodes**: `state.go:2807` `createAndSaveNewNode(... PreAuthKey: pak)` — tenant = `pak.TenantID`.
- **OIDC/interactive nodes**: `state.go:2205` `HandleNodeFromAuthPath(authID, userID, ...)` — tenant = `user.TenantID` (from claim).
Both derive tenant from a signed/stored credential; neither reads the request URL. Every downstream query (peer maps, policy, DNS) must then be filtered by `TenantID`.
