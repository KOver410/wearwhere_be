# Location & Store Discovery — Design

**Date:** 2026-06-13
**Scope:** SRS UC23–UC26 (Find Nearby Stores, Search Store by Area, View Store Details, Get Directions)
**Status:** Approved (design); brand-owner store-hours CRUD deferred to a separate feature.

## 1. Goal & Context

Implement the customer-facing **Location & Store Discovery** feature group from the SRS. This
is the only non-AI user-facing capability not yet built. Customers (and guests) can find local
brand stores near them, search stores by area, view store details, and get directions to a store
via Goong Maps.

This is the **Go backend** repo. Map rendering and live position tracking happen entirely on the
client (Flutter mobile / Next.js web) using the Goong Maps SDK. The backend serves store data and
proxies Goong for geocoding, distance, and routing.

### Key decisions (from brainstorming)

- **Goong integration boundary: backend proxies Goong (full).** The Goong API key stays
  server-side. The backend talks to Goong for distance matrix and directions; the client only
  renders. Chosen to maximize accuracy of displayed distance/ETA (real road distance, not
  straight-line) and to keep the key off the client.
- **"Store" = existing `brand_addresses`** rows with `is_public = true` and non-null
  `latitude`/`longitude`. No new store entity. Brand owners already manage these via the existing
  address handler (UC51).
- **Opening hours: new `store_hours` table.**
- **Auth: public read.** Guests and customers can all access these endpoints (consistent with the
  existing public catalog/brand endpoints).
- **Directions level A/B:** backend returns a route (`distance`, `duration`, `polyline`) one-shot,
  designed cheap + cacheable so the client can re-call it when re-routing. Voice turn-by-turn
  realtime navigation (level C) is **not** built; live position tracking is the client's job.

## 2. Accuracy considerations

Feature accuracy is primarily a **data-quality** problem, not an integration-approach problem:

- **Stored coordinates** drive everything. `brand_addresses.latitude/longitude` is nullable; stores
  without coordinates are excluded from discovery. (Improving coordinate quality at brand-address
  save time — e.g. geocoding on save — is a separate brand-owner concern, not in this scope.)
- **Displayed distance/ETA** uses Goong Distance Matrix (real road distance) rather than haversine
  straight-line, per the integration decision. Haversine is used only as a cheap pre-filter and as
  a graceful-degradation fallback when Goong is unavailable.
- **Vietnam administrative codes** (`city_code`/`district_code`/`ward_code`) may be affected by the
  2025 administrative restructuring; search-by-area correctness depends on the codes seeded on
  `brand_addresses`. Flagged as a data-freshness risk to verify against seed data.
- **Ratings** are out of scope (no review module yet) — omitted from responses.

## 3. Architecture & modules

Two new modules, following existing repo patterns (`internal/shipping/goship` provider pattern;
`MountCatalog`/`MountBrandsPublic` public-route pattern).

### A. `internal/maps/goong/` — Goong adapter

Mirrors `internal/shipping/goship/`.

- `client.go` — `Client` interface:
  - `Geocode(ctx, query) ([]GeocodeResult, error)` — `{Lat, Lng, FormattedAddress}`
  - `DistanceMatrix(ctx, origin, []dest) ([]DistanceResult, error)` — `{DistanceM, DurationS}`
  - `Directions(ctx, origin, dest) (Route, error)` — `{DistanceM, DurationS, Polyline}`
- `client_http.go` — real Goong HTTP client; reads `GOONG_API_KEY`; configurable timeout (~5s);
  `%w` error wrapping (per the gemini adapter convention).
- `client_mock.go` — deterministic mock for tests (no network).
- `factory.go` — `NewFromConfig(cfg)` selecting `mock | production` by `GOONG_MODE` (per goship).
  `production` mode without an API key returns an error at startup.

### B. `internal/store/` — store discovery module (read-only, public)

- `domain/` — `Store` view (composed from `brand_addresses` + brand name/logo/banner + `store_hours`
  + open/closed computed at runtime), DTOs, errors.
- `repo/` — Postgres queries: nearby (haversine pre-filter over `brand_addresses` using the existing
  `idx_brand_addr_geo`), search by area, detail + hours.
- `service/` — orchestration: repo fetches candidates → Goong `DistanceMatrix` enriches real
  distance/ETA → sort; computes open/closed in `Asia/Ho_Chi_Minh`.
- `handler/` + `routes.go` — `MountStoresPublic` (no auth).

### Use-case flows

- **UC23 Nearby:** client sends `lat,lng` → repo haversine selects candidates within radius (5km,
  widened to 10km if empty) → Goong `DistanceMatrix` (one call, capped at top-N candidates) computes
  real road distance/ETA → sort nearest→farthest.
- **UC24 Search by Area:** filter by `city_code`/`district_code`/`ward_code` or `q` (address text) in
  the DB; if `lat,lng` is also supplied, include distance. Goong not required.
- **UC25 Detail:** return brand name/logo/banner, address, phone, hours + open/closed status, and a
  link to the brand's products.
- **UC26 Directions:** `GET /stores/:id/directions?from=lat,lng` → backend calls Goong Directions →
  returns `{distance_m, duration_s, polyline}`. Turn-by-turn rendering and live tracking are client-side.

## 4. Data model & migrations

One new migration. Branching off `main` (highest migration `000032`), the next free number is
`000033`. **Collision risk:** the parked `ai-stylist-chatbot` branch also uses `000033`/`000034`;
if both branches eventually merge, the AI migrations will need renumbering. Flagged for the eventual
integration.

### New table `store_hours`

```sql
CREATE TABLE store_hours (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    brand_address_id  UUID NOT NULL REFERENCES brand_addresses(id) ON DELETE CASCADE,
    weekday           SMALLINT NOT NULL,        -- 0=Sunday .. 6=Saturday
    open_time         TIME NOT NULL,            -- local VN time
    close_time        TIME NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_store_hours_addr ON store_hours (brand_address_id, weekday);
```

- Multiple rows per weekday allowed (e.g. lunch break: 08:00–12:00 and 13:30–21:00).
- No rows for a given weekday = closed that day. No rows at all = "hours not set" → open/closed
  status omitted from the response.
- Open/closed computed at runtime in `Asia/Ho_Chi_Minh`.
- `weekday` convention: **0=Sunday .. 6=Saturday** (matches Go `time.Weekday`).

### Reused, unchanged

- `brand_addresses` — store = `is_public = true AND latitude IS NOT NULL AND longitude IS NOT NULL`.
  Existing `idx_brand_addr_geo` supports the haversine pre-filter.
- Store photos — use `brands.logo_url` + `banner_url` (per SRS UC25 "store photos from brand
  profile"). No new photo table.

### Out of scope (documented exclusions)

- **Ratings** — deferred until the review module exists.
- **Voice turn-by-turn realtime nav (level C)** — client hands off to an external maps app if needed.
- **Brand-owner store-hours CRUD** — a separate Brand Partner feature. This feature only reads
  `store_hours` and seeds sample data for testing.

## 5. API endpoints (public, no auth)

Mounted via `MountStoresPublic` (per `MountCatalog`/`MountBrandsPublic`). All read-only.

| UC | Method & Path | Query/Params | Response |
|----|---------------|--------------|----------|
| 23 | `GET /stores/nearby` | `lat`, `lng` (required), `radius_km` (default 5, auto-widen to 10 if empty) | stores + `distance_m`, `duration_s` (real, from Goong), open/closed, sorted nearest→farthest |
| 24 | `GET /stores` | `city_code`/`district_code`/`ward_code` **or** `q` (address text); optional `lat,lng` to include distance | stores matching area |
| 25 | `GET /stores/:id` | `id` = brand_address id | detail: brand name/logo/banner, address, phone, hours + open/closed, link to brand products |
| 26 | `GET /stores/:id/directions` | `from` = `lat,lng` (required) | `{distance_m, duration_s, polyline}` from Goong Directions |

### Service decisions

- **Nearby:** haversine pre-filter (cheap, coarse) → Goong `DistanceMatrix` once for the candidate
  set, **capped at top-N (e.g. 25)** to bound Goong cost; log when the candidate set is truncated.
- **Search by area:** filter directly in the DB; Goong only used to add distance when `lat,lng` given.
- **Directions:** proxy Goong; cache keyed on `(store_id, from rounded to ~50m)` with a short TTL so
  client re-route calls are cheap (supports the level A/B decision).

## 6. Config, error handling & testing

### Config (mirror `GoshipConfig` in `internal/config/config.go`)

```go
type GoongConfig struct {
    Mode    string // mock | production
    APIKey  string
    BaseURL string // default https://rsapi.goong.io
}
```

Env: `GOONG_MODE` (default `mock`), `GOONG_API_KEY`, `GOONG_BASE_URL`. `production` without a key →
factory error at startup (per goship requiring a token).

### Error handling (`pkg/httpx` AppError)

- Missing/invalid `lat,lng` or `from` → `400` validation error.
- Store id not found / not public / no coordinates → `404`.
- Goong timeout/error:
  - **Nearby:** degrade gracefully — still return stores with **haversine** distance and a
    `distance_approx: true` flag instead of failing the whole request.
  - **Directions:** return `502` (no fallback route); client shows "couldn't get directions, here's
    the address".
- Goong HTTP client: configurable timeout (~5s), `%w` error wrapping.

### Testing (TDD; existing repo patterns)

- `goong/client_mock.go` — deterministic, used by all service/handler tests (no network).
- `goong/client_http_real_test.go` — real Goong call, env-gated (skip without `GOONG_API_KEY`), per
  `goship/client_http_real_test.go`.
- Service tests: haversine math, 5→10km radius widening, sort order, open/closed around hour
  boundaries (VN timezone), graceful degradation when Goong fails.
- Handler tests: validation, 404, response shape.
- Repo tests: nearby/search using existing testfixtures.

## 7. Branch note

This feature is independent of the parked `ai-stylist-chatbot` branch. `main` already has the base
`pkg/httpx` and `internal/shared`, so the feature branches off `main`.
