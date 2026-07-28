# License & Trademark Guardrails (P0-1)

**Status:** active · **Owner:** Legal/BizOps · **Applies to:** the entire RavenScale
fork of Headscale and everything we ship, host, or publish from it.

This is an operational do/don't guide, not legal advice. It exists so engineering
can move fast without tripping either of the two obligations that come with
building on Headscale: **the BSD-3 license** (a copyright obligation we must
honor) and **the Tailscale trademark** (a naming boundary we must not cross).
Escalate any real-world edge case to counsel before shipping.

---

## 1. What we are building on

| Item | Fact | Source |
|------|------|--------|
| Upstream project | Headscale — `github.com/juanfont/headscale` | README.md |
| Upstream license | **BSD 3-Clause "New" License** | `LICENSE` in upstream repo |
| Copyright line | `Copyright (c) 2020, Juan Font. All rights reserved.` | `LICENSE` in upstream repo |
| Our relationship | RavenScale is a **fork** we modify, host (SaaS), and redistribute | `README.md`, `CONTEXT.md` |
| Tailscale | Separate company; owns the **Tailscale** name/logo and the official client | tailscale.com |

BSD-3 is permissive: we may use, modify, sublicense, host as a paid SaaS, and
redistribute in source or binary form, for commercial purposes, **without**
releasing our own additions. There is **no copyleft** — our proprietary
multi-tenant control plane, admin UI, and differentiation code stay closed if we
choose. The obligations below are the *entire* price of that freedom.

---

## 2. BSD-3 obligations — the DO list

BSD-3 imposes exactly three affirmative conditions plus a naming restriction.
Meeting these = full compliance.

- **DO keep the upstream `LICENSE` file** (with `Copyright (c) 2020, Juan Font`
  and the full 3-clause text + disclaimer) in the repo, unmodified, at every
  fork commit. Never delete or rewrite it. *(Clause 1 — source redistribution.)*
- **DO retain existing copyright notices and license headers** in any Headscale
  source file we keep or modify. Add our own notice alongside; never replace
  theirs. *(Clause 1.)*
- **DO ship the license + copyright notice with any binary/Docker image/release**
  we distribute — in the image (e.g. `/usr/share/.../LICENSE`), release notes, or
  an accompanying `THIRD_PARTY_LICENSES` / NOTICE file. A binary handed to a
  customer or pulled from a registry must carry the notice "in the documentation
  and/or other materials." *(Clause 2 — binary redistribution.)*
- **DO maintain a `THIRD_PARTY_LICENSES.md`** (or `NOTICE`) that lists Headscale
  and every other third-party dependency's license. Headscale itself vendors
  Tailscale's own BSD-3 libraries (e.g. `tailscale.com/...`) — those copyright
  notices ride along under the same clauses and must be preserved too.
- **DO pin the exact upstream commit/tag we forked from** and record it (e.g. in
  `CONTEXT.md` or a `FORK_BASE` marker), so the provenance of the retained
  copyright is auditable.
- **DO re-verify the `LICENSE` at each upstream merge/rebase** — confirm upstream
  has not relicensed or added new copyright holders that need listing.

### Does NOT apply to us (so don't over-engineer compliance)

- **DON'T** feel obligated to open-source our own code — BSD-3 is not copyleft.
- **DON'T** add per-file attribution to *our own* net-new files — the retained
  upstream notice + LICENSE file covers the derivative-work obligation.

---

## 3. BSD-3 — the DON'T list

- **DON'T** remove, truncate, or "clean up" the upstream `LICENSE` file or any
  in-file copyright headers. This is the single most common BSD-3 violation.
- **DON'T** strip the warranty **DISCLAIMER** — clauses 1 & 2 require the full
  disclaimer text to travel with both source and binary.
- **DON'T** use **"Juan Font"** or the Headscale contributors' names to endorse
  or promote RavenScale without prior written permission. *(Clause 3 — the
  no-endorsement clause. "Built by the Headscale team" ❌; "Forked from the
  open-source Headscale project" ✅ — factual, not an endorsement claim.)*
- **DON'T** relicense the upstream-derived files under terms that drop the BSD-3
  conditions; our combined work may be proprietary, but the Headscale-origin
  portions stay governed by BSD-3.

---

## 4. Trademark boundaries — Tailscale (and Headscale)

Copyright ≠ trademark. BSD-3 grants us **no** right to any Tailscale or Headscale
name/logo. Trademark law is about *confusion and endorsement*, so the test is:
would a user think RavenScale is *made by, affiliated with, or endorsed by*
Tailscale? If yes → don't.

### DO

- **DO name the product `RavenScale`** and use only our own name, logo, domains,
  and marks everywhere: UI, marketing, repo, package names, Docker images,
  DNS suffixes, CLI binary.
- **DO use "Tailscale" nominatively — sparingly — only to state a true fact**
  about interoperability, e.g. *"Compatible with the official Tailscale client"*
  or *"works with `tailscaled`."* Nominative fair use requires: (a) you couldn't
  identify the thing without the name, (b) use no more of the mark than needed,
  (c) nothing implying sponsorship/endorsement.
- **DO add a disclaimer** where we reference Tailscale: *"RavenScale is an
  independent project and is not affiliated with, endorsed by, or sponsored by
  Tailscale Inc. Tailscale is a trademark of Tailscale Inc."* (Headscale itself
  carries an equivalent notice.)
- **DO treat the word "Headscale" the same way** — it's fine to state we forked
  Headscale (factual), but the shipped product is branded RavenScale, not
  "Headscale Enterprise" or similar.

### DON'T

- **DON'T** put **"Tailscale"** (or "Headscale") in the product name, company
  name, logo, domain, subdomain, app-store listing, social handle, package name,
  or Docker image name. No `tailscale-*`, no `*-tailscale`, no lookalike logos or
  the Tailscale color/wordmark.
- **DON'T** use the **Tailscale logo, wordmark, or any confusingly similar mark**
  anywhere — including admin UI chrome, favicons, or marketing.
- **DON'T** imply partnership, endorsement, certification, or official status
  ("official Tailscale alternative", "Tailscale-approved", "powered by
  Tailscale"). ❌
- **DON'T** register domains/handles/trademarks incorporating "Tailscale" or
  "Headscale," or bid on them as your *own* brand identity.
- **DON'T** use Tailscale marketing copy, screenshots, or brand assets as our own.
- **DON'T** rely on nominative use as cover for prominence — a fact-stating
  footnote is fine; "Tailscale" as a hero headline is not.

---

## 5. Quick pre-ship checklist

Before any release, image push, or public page:

- [ ] Upstream `LICENSE` (Juan Font, full BSD-3 text + disclaimer) present at HEAD.
- [ ] `THIRD_PARTY_LICENSES` / `NOTICE` includes Headscale + vendored Tailscale libs.
- [ ] License + notice bundled in the distributed binary/image/release.
- [ ] Fork-base commit recorded and re-checked since last upstream merge.
- [ ] Product name, logo, domains, package/image names contain **no** "Tailscale"/"Headscale".
- [ ] Any "Tailscale" reference is nominative + factual + carries the non-affiliation disclaimer.
- [ ] No endorsement/affiliation/partnership language anywhere.

---

## 6. Sources

- Upstream `LICENSE` — `github.com/juanfont/headscale` (`BSD 3-Clause`,
  `Copyright (c) 2020, Juan Font`), fetched via `gh api repos/juanfont/headscale/contents/LICENSE`.
- BSD-3-Clause reference text — SPDX `BSD-3-Clause`.
- RavenScale `README.md`, `CONTEXT.md` — fork relationship and branding note.
- Trademark analysis reflects the *New Kids on the Block v. News America* nominative
  fair use factors; this is guidance, not legal advice — escalate edge cases to counsel.
