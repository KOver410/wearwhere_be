# AI & Personalization (UC31 / UC29 / UC30) — Design

**Date:** 2026-06-16
**Status:** APPROVED FOR PLANNING
**Scope:** The three remaining sub-features of the "AI & Personalization" area for logged-in customers:

- **UC31 Set Style Preferences** — persist a per-user style profile (style tags + budget).
- **UC29 Get AI Recommendations** — a deterministic, heuristic "For You" feed.
- **UC30 View Smart Wardrobe** — Gemini-composed outfits from owned items, with complementary products to buy.

**Sibling (already designed, not in this spec):** UC28 Chat with AI Stylist — see `2026-06-05-ai-stylist-chatbot-design.md` (APPROVED, only `quota.go` implemented so far). This spec consumes the same shared `internal/shared/llm` port and the catalog retriever introduced by UC28; whichever feature lands first creates the shared port, the later one imports it (no duplicate definition).

The Admin Features cluster (UC52–62) is **descoped** from the backend and is not addressed here.

---

## 1. Goals

| Goal | UC | Coverage |
|------|----|---------|
| Customer defines fashion style preferences (quiz onboarding or settings) | UC31 | Full — `PUT/GET /me/style-profile`; editable anytime |
| Personalized product feed based on preferences + history | UC29 | Full — deterministic heuristic scoring; daily-cached |
| Cold-start for new users with no profile/history | UC29 | Full — trending feed + onboarding prompt flag |
| Outfit combinations from purchased items | UC30 | Full — Gemini composes from the digital closet |
| Suggest complementary products to complete a look | UC30 | Full — retriever attaches `to_buy[]` per outfit |
| Useful even before first purchase | UC30 | Full — empty closet → full buy-the-outfit suggestions grounded in the style profile |
| Provider-agnostic, testable | UC30 | Full — reuses `llm.Client` port (`gemini` + `mock`) |

**Non-goals (deferred):**
- Browsing/clickstream event tracking (UC29 uses profile + purchase/wishlist/follow only).
- LLM-based recommendations (UC29 is deterministic by decision).
- Streaming responses anywhere.
- Chat/cart actions from these features (cards/links only).
- Admin analytics over token usage.

## 2. Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Module layout | Three separate modules: `internal/styleprofile`, `internal/recommendation`, `internal/wardrobe` | Matches the codebase's one-domain-per-module pattern; each tests/ships independently |
| UC29 engine | **Deterministic heuristic** (no LLM) | Fast, cheap, fully testable, no per-feed token cost |
| UC30 engine | **Gemini** via shared `internal/shared/llm` port | Outfit composition is genuinely generative |
| Favorite brands signal | **Reuse existing brand-follow (UC35)** | No duplicate "likes a brand" source; profile stays minimal |
| UC30 eligibility | **No order minimum** | Empty closet still useful: suggest a full outfit to buy |
| UC30 result handling | **Persisted snapshot, regenerate on change** | Avoids a Gemini call on every open; supports "update after each order" |
| UC29 freshness | **Redis daily cache per user**, invalidated on profile/order change | Matches "update recommendations daily" business rule |
| Discovery mixing | **Deterministic** (top-N + round-robin discovery slice, no randomness) | Reproducible in tests; still "mixes styles for discovery" |
| Error format | **Format A** (`{error:{code,message,details}}`) via `pkg/httpx` | Consistent with the other new modules |
| Ownership | All endpoints scoped by `user_id` (IDOR-safe) | Same pattern as addresses/orders/stylist |

---

## 3. UC31 — Set Style Preferences (`internal/styleprofile`)

Backend stores the profile; the image-based quiz UI is frontend. The profile is the foundation consumed by UC29 and by the UC28 B1 seam.

### 3.1 Data model

`style_profiles` — one row per user (upsert):

| Column | Type | Notes |
|--------|------|-------|
| user_id | uuid PK | FK → users(id), ON DELETE CASCADE |
| budget_min | int NULL | VND, optional |
| budget_max | int NULL | VND, ≥ budget_min when both set |
| onboarded_at | timestamptz NULL | set on first completion of the quiz |
| created_at / updated_at | timestamptz | |

`style_profile_tags` — M-N:

| Column | Type | Notes |
|--------|------|-------|
| user_id | uuid | FK → style_profiles(user_id), ON DELETE CASCADE |
| style_tag_id | uuid | FK → `style_tags(id)` (same table products are tagged with) |

PK `(user_id, style_tag_id)`. Reusing the product `style_tags` table guarantees recommendation tag-matching works against real product tags.

**Favorite brands** are NOT stored here — brand-follow (UC35) is the signal.

### 3.2 API (`/api/v1/me/style-profile`, auth + role=customer)

| Method | Path | Body | Response |
|--------|------|------|----------|
| GET | `/me/style-profile` | — | `200` profile `{style_tags[], budget_min, budget_max, onboarded_at}`; empty profile object if never set |
| PUT | `/me/style-profile` | `{style_tag_ids[] (≤10), budget_min?, budget_max?}` | `200` profile (upsert; sets `onboarded_at` on first set) |

- **Validation:** every `style_tag_id` must exist in `style_tags` (else `VALIDATION_FAILED` with the offending ids in `details`); `budget_max ≥ budget_min` when both present. Max 10 tags.
- **Editable anytime** (business rule): `PUT` is idempotent and fully overwrites the tag set and budget.
- Exposes an internal `LoadProfile(ctx, userID) (*StyleProfile, error)` for in-process use by `recommendation` and (later) `stylist` — not over HTTP.
- A `PUT` invalidates the UC29 daily cache for that user (see §4.3).

### 3.3 Error codes

| Code | HTTP | When |
|------|------|------|
| `VALIDATION_FAILED` | 400 | unknown style_tag_id, >10 tags, or budget_max < budget_min |

---

## 4. UC29 — Get AI Recommendations (`internal/recommendation`)

Deterministic "For You" feed. No LLM call.

### 4.1 Signals (all already in the DB)
- Style profile: `style_tag_ids`, `budget_min/max` (UC31).
- Followed brands (UC35).
- Purchase history (order items) and wishlist.

### 4.2 Scoring (warm path — user has a profile or any history)

1. **Candidate set:** `active`, in-stock products, excluding products the user already purchased.
2. **Score = weighted sum:**
   - **Style-tag overlap** — count of shared tags between product and profile. Highest weight.
   - **Brand affinity** — product belongs to a followed brand → bonus.
   - **Budget fit** — price within `[budget_min, budget_max]` → bonus; outside → small penalty. (No-op if budget unset.)
   - **Category affinity** — category seen in purchase history or wishlist → small bonus.
3. **Discovery mixing** (business rule "mix styles to encourage discovery): after sorting by score, take ~70% from the top, then fill the remaining ~30% with a *discovery slice* — products from tags/brands the user has not interacted with, chosen by a stable round-robin over brand (no randomness, so tests are reproducible).
4. Out-of-stock already excluded in step 1 ("avoid showing out-of-stock items").

Ties broken by a stable key (e.g. `created_at DESC, id`) so the ordering is deterministic.

### 4.3 Cold start (alt-seq 2a — no profile and no history)
Return **trending** products (ranked by recent sales/order count, fallback wishlist count) with `source:"trending"` and `onboarding_prompt:true` so the FE nudges the UC31 quiz.

### 4.4 Freshness / caching ("update recommendations daily")
Redis key `rec:feed:{user_id}:{yyyymmdd}` (TTL = end of day). First call of the day computes and caches the ordered product-id list; later calls read it. The cache is **invalidated** when the user updates their style profile (UC31 `PUT`) or places a new order.

### 4.5 API (`/api/v1/me/recommendations`, auth + role=customer)

| Method | Path | Query | Response |
|--------|------|-------|----------|
| GET | `/me/recommendations` | `limit` (default 20, max 50) | `200` `{items:[ProductCard], source:"personalized"\|"trending", onboarding_prompt:bool}` |

Retrieval reuses the existing catalog read-path (no duplicate SQL). `ProductCard` mirrors the catalog card shape used by UC28: `{id, slug, name, brand:{slug,name}, primary_image_url, price_vnd}`.

---

## 5. UC30 — View Smart Wardrobe (`internal/wardrobe`)

Gemini-composed outfits from the customer's owned items, plus complementary products to buy. **No order minimum** — useful even with an empty closet.

### 5.1 Digital closet
Distinct products from the user's completed/delivered order items (dedup by product). May be empty.

### 5.2 Outfit generation (Gemini via shared `llm` port)
Send the closet (each item's name + category + style_tags) to Gemini, which returns outfit JSON `[{title, note, owned_product_ids[]}]`. The system prompt forbids inventing items not in the closet.

**Text metadata only — no product images are sent to the model.** Gemini reasons over name/category/style_tags (text), not visual color/pattern. `primary_image_url` is returned in the response cards for the FE to display, but is never fed to the model. (Multimodal/vision was considered and deferred — see §8.) Then, per outfit, the **retriever** picks 1–2 complementary in-stock products → `to_buy[]`.

Each returned outfit has two parts:
- `owned[]` — pieces the user already has (may be empty),
- `to_buy[]` — complementary products to purchase.

**Closet states:**
- **Has items:** mix owned pieces + complementary `to_buy[]`.
- **Empty/sparse closet:** outfits are composed entirely of products to buy, grounded in the **style profile (UC31)** + retriever (matching preferred tags/budget). `owned[]` is empty.
- **Extreme cold start** (empty closet *and* no style profile): seed outfits from trending/popular products + set `onboarding_prompt:true` to nudge the UC31 quiz (same pattern as UC29).

### 5.3 Persistence & "update after each order"

`wardrobe_outfits` — snapshot, regenerated when the wardrobe inputs change:

| Column | Type | Notes |
|--------|------|-------|
| id | uuid PK | |
| user_id | uuid | FK → users(id), ON DELETE CASCADE; indexed |
| title | text | |
| note | text | Gemini styling note |
| owned_product_ids | uuid[] | pieces from the closet used (empty when buying full outfit) |
| suggested_product_ids | uuid[] | complementary products to buy |
| model | text NULL | e.g. `gemini-2.0-flash` |
| tokens_in / tokens_out | int | per-generation token accounting (consistent with UC28) |
| generated_at | timestamptz | |

A per-user marker `closet_signature` (e.g. latest delivered order id + distinct purchased-product count) records what the current snapshot was generated from.

**Regenerate when:** `closet_signature` changed (new order) **OR** style profile changed (because the empty-closet path depends on it) **OR** no snapshot exists. For an empty closet, also apply a daily TTL so opening the page repeatedly does not call Gemini each time. Otherwise serve the cached snapshot — **no token spend on a normal open**.

### 5.4 API (`/api/v1/me/wardrobe`, auth + role=customer)

| Method | Path | Response |
|--------|------|----------|
| GET | `/me/wardrobe` | `200` `{closet:[ProductCard], outfits:[{title, note, owned:[ProductCard], to_buy:[ProductCard]}], outfits_status:"ready"\|"unavailable", onboarding_prompt:bool}` |
| POST | `/me/wardrobe/regenerate` | `200` forces recomputation (the "refresh" button) |

### 5.5 Provider failure (graceful degrade)
If Gemini times out / errors / safety-blocks, still return `closet` (DB data) with `outfits:[]` and `outfits_status:"unavailable"` — never fail the whole endpoint. The closet is always viewable.

### 5.6 Error codes

| Code | HTTP | When |
|------|------|------|
| `VALIDATION_FAILED` | 400 | bad query/body |
| (degrade, not an error) | 200 | provider failure → `outfits_status:"unavailable"` |

---

## 6. Shared Dependencies

- **`internal/shared/llm`** port (`Client` interface + `gemini`/`mock` adapters, `AI_PROVIDER` selector) — created by whichever of UC28/UC30 lands first; the other imports it. Default `AI_PROVIDER=mock` keeps dev/test/CI offline.
- **Catalog retriever / read-path** — reused by UC29 and UC30; do not duplicate SQL.
- **`style_tags`** table — shared by products and UC31.
- **brand-follow (UC35)**, **order items**, **wishlist** — read-only signals for UC29/UC30.
- **`ProductCard` DTO** — same shape as the catalog card and UC28.

### Config (env) — additive

| Key | Default | Meaning |
|-----|---------|---------|
| `AI_PROVIDER` | `mock` | `gemini` \| `mock` (shared with UC28) |
| `GEMINI_API_KEY` / `GEMINI_MODEL` | — / `gemini-2.0-flash` | shared with UC28 |
| `AI_REQUEST_TIMEOUT` | `15s` | per Gemini call (shared) |
| `REC_FEED_DEFAULT_LIMIT` | `20` | UC29 default page size |
| `REC_FEED_MAX_LIMIT` | `50` | UC29 cap |
| `WARDROBE_MAX_OUTFITS` | `5` | outfits generated per snapshot |
| `WARDROBE_EMPTY_CLOSET_TTL` | `24h` | regen throttle for empty-closet snapshots |

---

## 7. Testing Strategy

- **UC31:** upsert idempotency, unknown tag rejection, budget validation, IDOR (only the caller's profile), `LoadProfile` getter, cache invalidation on `PUT`.
- **UC29:** scoring (tag overlap / brand affinity / budget fit / category affinity), purchased-product exclusion, deterministic discovery mixing (fixed input → fixed order), cold-start trending + `onboarding_prompt`, daily-cache hit/miss + invalidation on profile/order change.
- **UC30:** closet build + dedup, owned-vs-to_buy composition with items / empty closet / extreme cold start, regenerate-on-signature-change vs cached serve, provider-failure degrade (`outfits_status:"unavailable"`, closet still returned), token accounting persisted. Uses `llm/mock` for deterministic offline runs.
- **E2E** (`AI_PROVIDER=mock`): set profile → recommendations reflect it → wardrobe returns outfits → new order invalidates rec cache + flips wardrobe signature.

## 8. Out-of-Scope / Follow-ups (file as issues)

- Multimodal/vision outfit composition (feeding product images to Gemini for color/pattern-aware pairing) — deferred for token cost, latency, and image-handling complexity; UC30 uses text metadata only.
- Browsing/clickstream tracking to enrich UC29.
- LLM-ranked recommendations / embeddings.
- Per-user daily quota on wardrobe regeneration (currently gated only by staleness + TTL).
- Admin analytics over `wardrobe_outfits` token usage.
- Streaming; chat/cart actions from recommendations or wardrobe.
