# Goship Shipping — Spec A: Foundation + Carrier-Selectable Rates at Checkout

**Date:** 2026-06-03
**Branch (target):** `sprint-3-orders-checkout` (continuation) — or a dedicated `goship-rates` branch
**Status:** APPROVED FOR PLANNING
**Scope:** Replace flat-rate shipping with real Goship rate quoting; customer selects a carrier per brand at checkout. Location-code foundation (city/district/ward) + chargeable-weight model for variants.

This is **Spec A of 2**. Spec B (separate doc/plan) covers the fulfillment lifecycle: brand confirm/ship/deliver endpoints, real Goship shipment creation, tracking, status webhooks, and shipment cancellation.

---

## 1. Goals

| Goal | Spec A coverage |
|------|-----------------|
| Real shipping fees from Goship (aggregator over GHN/GHTK/VTP/...) | Full — `Rates` API per brand sub-order |
| Customer chooses carrier per brand | Full — preview returns options; place re-quotes by chosen carrier |
| Structured Vietnamese addresses (province/district/ward codes) | Full — code columns + location proxy endpoints |
| E-commerce-standard fee accuracy | Full — chargeable weight = max(actual, volumetric) |

**Non-goals (Spec A — deferred to Spec B):**
- Brand fulfillment endpoints (`/api/v1/brand/me/orders/*` confirm/ship/deliver)
- Real Goship **shipment** creation (`POST /shipments`), tracking number / label storage
- Goship status **webhook** receiver (`x-goship-hmac-sha256`) and sub-order status transitions
- Shipment cancellation / refund-on-cancel
- Automatic free-text → code normalization is **optional/nice-to-have**, not a blocker (see §6)

## 2. Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Provider integration shape | Mirror PayOS: `client.go` (interface+DTOs) + `client_http.go` + `client_mock.go` + `factory.go`; `goship_real` build-tag integration tests | Consistent with existing codebase; mock unblocks dev/CI without network; sandbox token already available for real tests |
| Mode switch | `GOSHIP_MODE=mock\|sandbox\|production` | Same pattern as `PAYOS_MODE`; sandbox token present now |
| Carrier selection | Customer chooses per brand; server **re-quotes** by chosen carrier at place-order | Never trust client-supplied fee; rate IDs expire fast so we key on carrier code, not rate ID |
| Rate ID persistence | **Not stored** in Spec A | Goship rate IDs are short-lived (minutes); Spec B re-quotes at brand-confirm time. Store only `shipping_carrier` + `shipping_fee_vnd` (the charged fee) |
| Weight model | We send **actual aggregated weight** (`Σ item.qty × weight_g`) **+ representative dimensions**; **Goship applies volumetric server-side** (divisor ~6000) and returns the chargeable fee | Confirmed from doc.goship.io: the `/rates` parcel takes `weight` + `width/height/length` and Goship computes `max(actual, volumetric)` itself — pre-computing client-side would double-apply and used the wrong divisor (5000 vs 6000) |
| Missing weight/dims | Fallback `GOSHIP_DEFAULT_ITEM_WEIGHT_G` + default dims in config | Variants may not have weight yet; never block checkout on missing data |
| Location code type | **TEXT** (`*string` in Go), e.g. district `"100100"`, city `"100000"` | Confirmed from doc: Goship location codes are numeric **strings**, not integers — TEXT preserves exact format |
| Address validity | Codes required to quote Goship; incomplete address → `address_incomplete` warning + **block place-order** | Marketplaces enforce structured addresses; never silently mis-charge |
| Legacy addresses (no codes) | Prompt customer to re-select via dropdowns at checkout; flat-rate is a configured fallback only, not a silent escape | Best practice: gate checkout on valid address |
| `ShippingProvider` interface | Change `Calculate → Quote` returning `[]ShippingOption` | Multi-carrier choice needs multiple options; flat-rate adapts to a single synthetic option |
| Location data | Proxy `cities/districts/wards` through our API with in-memory TTL cache (~24h) | Lists are near-static; FE drives cascading dropdowns; avoids hammering Goship |

## 3. Architecture & Package Layout

```
internal/shipping/
  domain/
    fee.go                 (existing) FeeQuote — kept; add ShippingOption
    option.go        NEW   ShippingOption{Carrier, CarrierName, Service, AmountVND, ETA}
  provider/
    provider.go            CHANGE  ShippingProvider.Quote(ctx, CalcReq) ([]ShippingOption, error)
    flat_rate.go           CHANGE  returns single option (carrier="flat")
    goship_provider.go NEW  wraps goship.Client.Rates; maps cart items -> parcel weight
    factory.go             CHANGE  add "goship" branch
    *_test.go
  goship/                  NEW  (mirrors internal/payment/payos/)
    client.go              Client interface + DTOs (Location, RateReq, Rate, Parcel)
    client_http.go         real HTTP client (Bearer GOSHIP_TOKEN)
    client_mock.go         deterministic mock (ghn/ghtk/vtp by weight)
    factory.go             NewFromConfig(mode)
    client_http_real_test.go   build tag: goship_real
    client_mock_test.go
  location/                NEW
    service.go             Cities/Districts/Wards with TTL cache over goship.Client
    handler.go             GET /api/v1/locations/...
    routes.go
    *_test.go

internal/shipping/weight/  NEW (or a func in goship_provider.go)
    weight.go              ChargeableGrams(items, defaults) — actual vs volumetric
    weight_test.go

internal/config/config.go  CHANGE  GoshipConfig; extend ShippingConfig

db/migrations/             NEW (next free sequence after 000026; confirm at impl)
    000027_add_location_codes_to_customer_addresses.{up,down}.sql
    000028_add_location_codes_to_brand_addresses.{up,down}.sql
    000029_add_dimensions_to_variants.{up,down}.sql
    000030_add_shipping_carrier_to_sub_orders.{up,down}.sql
```

Touched existing packages: `internal/order/service/checkout_service.go` & `order_service.go` (new `Quote` interface, carrier selection), `internal/order/domain/dto.go` (preview options + `PlaceOrderReq.shipping_selections`), `internal/order/domain/order.go` (`ShippingAddress` adds codes), `internal/customeraddr/*` & `internal/brand/*` (address codes accept/validate), `internal/catalog`/variant write paths (optional weight/dims fields).

## 4. Data Model Changes

### 4.1 `customer_addresses` & `brand_addresses`
```sql
ALTER TABLE customer_addresses
  ADD COLUMN city_code     TEXT,
  ADD COLUMN district_code TEXT,
  ADD COLUMN ward_code     TEXT;
-- same three columns on brand_addresses
```
Nullable (legacy rows have none). Codes are Goship numeric **strings** (e.g. `"100000"`). New/updated addresses created via dropdown must supply all three; partial codes rejected at validation.

### 4.2 `variants` — chargeable-weight inputs
```sql
ALTER TABLE variants
  ADD COLUMN weight_g  INT,
  ADD COLUMN length_cm INT,
  ADD COLUMN width_cm  INT,
  ADD COLUMN height_cm INT;
-- all nullable; CHECK (> 0) when present
```

### 4.3 `sub_orders`
```sql
ALTER TABLE sub_orders ADD COLUMN shipping_carrier TEXT;  -- e.g. 'ghn', 'ghtk', 'vtp'
-- existing: shipping_provider ('goship'|'flat'), shipping_fee_vnd
```

### 4.4 `orders.shipping_address` JSONB snapshot
`ShippingAddress` struct gains `city_code`, `district_code`, `ward_code` (all `*string`, snapshotted at place-order so Spec B can create the shipment from frozen data).

## 5. Goship Client Contract (confirmed against doc.goship.io)

- **Base URL:** sandbox `https://sandbox.goship.io/api/v2`; production TBD (confirm with account).
- **Auth:** `Authorization: Bearer <GOSHIP_TOKEN>`. Token obtained via `POST /login` (`{username, password, client_id, client_secret}` → `{access_token, expires_in}`), lifetime ~100 years (treat as static config). `POST /refresh_token` exists but optional. Spec A uses the static token only; `client_secret` is needed in Spec B for webhook HMAC verification.
- **Locations:** `GET /cities`, `GET /cities/{cityCode}/districts`, `GET /districts/{districtCode}/wards` → `{data:[{id (string), name}]}`.
- **Rates:** `POST /rates`:
  ```json
  {"shipment":{"address_from":{"district":"100100","city":"100000"},
   "address_to":{"district":"100100","city":"100000"},
   "parcel":{"cod":500000,"amount":500000,"width":10,"height":10,"length":10,"weight":750}}}
  ```
  Response `{code, status, data:[{id, carrier_name, carrier_logo, service, expected, cod_fee, total_fee, total_amount}]}`. `weight` is grams; `width/height/length` in cm; Goship applies volumetric (≈6000) and returns `total_fee`. `cod` = grand total for COD orders, `0` for PayOS-prepaid; `amount` = declared value (subtotal).
- **Carrier codes:** `vtp, ems, vnp, ghtk, ghnv3, shopee, best, tikinow`. **GHN is `ghnv3`** (not `ghn`). Webhook payloads use `carrier_short_name` (e.g. `"ghn"`); the rate object's carrier-code field name (vs only `carrier_name`) must be confirmed at impl — if no short code is present, carrier selection keys on `carrier_name`.

Client interface (Spec A subset):
```go
type Client interface {
    Cities(ctx) ([]Location, error)
    Districts(ctx, cityCode string) ([]Location, error)
    Wards(ctx, districtCode string) ([]Location, error)
    Rates(ctx, RateReq) ([]Rate, error)  // RateReq carries string codes + weight/dims + cod/amount
}
```
Mock returns a fixed carrier set with deterministic fees so tests are stable. Per the confirmed contract (§11), the mock mirrors prod: `Carrier == CarrierName == display name` (e.g. `Giao Hàng Nhanh (v3)`, `Vietnam Post`, `Viettel Post`) — no short codes.

## 6. Flows

### 6.1 Address create/update
FE loads cascading dropdowns from `/api/v1/locations/*`, submits names **and** codes. Backend validates the three codes are present and consistent (district belongs to city, ward to district — validated via the cached location lists). Free-text names retained for display.

### 6.2 Checkout preview — `GET /me/checkout/preview`
1. Load cart, group by brand (existing logic).
2. Validate selected address has all three codes. If not → response carries `address_incomplete=true`, no options returned, place-order will be blocked.
3. Per brand sub-order: compute parcel chargeable weight (§7), call `provider.Quote(from=brand pickup address, to=customer address, weight)`.
4. Return per-brand `shipping_options: [{carrier, carrier_name, service, amount_vnd, eta}]`. No fee committed yet.

### 6.3 Place order — `POST /me/orders`
`PlaceOrderReq` adds `shipping_selections: [{brand_id, carrier}]` (one per brand sub-order).
1. Block if address incomplete (`ErrAddressIncomplete`).
2. Inside the existing atomic tx: for each brand, **re-quote** Goship and pick the option matching the submitted `carrier`. Use that authoritative `amount_vnd` as `shipping_fee_vnd`; store `shipping_carrier` + `shipping_provider='goship'`.
3. If the chosen carrier is no longer available → `ErrCarrierUnavailable` (FE re-previews).
4. Remainder (sub-orders, stock reserve, payment, cart clear) unchanged.

### 6.4 Fallback provider
If `SHIPPING_PROVIDER=flat`, `Quote` returns one option `{carrier:"flat", amount: brands.shipping_flat_fee_vnd}`. Used for local dev or as an explicit configured degrade — never an automatic silent fallback when a Goship address is invalid.

## 7. Parcel Weight & Dimensions

Goship applies the volumetric calculation server-side (divisor ≈6000) from the `weight` + `width/height/length` we send, so we do **not** pre-compute chargeable weight. We aggregate the sub-order into one parcel:
```
parcel_weight_g     = Σ item.qty * (variant.weight_g  ?? GOSHIP_DEFAULT_ITEM_WEIGHT_G)
parcel_length_cm    = max(item.length_cm ?? default)      // representative box
parcel_width_cm     = max(item.width_cm  ?? default)
parcel_height_cm    = Σ (item.qty * (item.height_cm ?? default))   // stack height
```
Missing field on a variant → substitute config defaults (`GOSHIP_DEFAULT_*`). The `weight` package only **aggregates** actual weight + picks representative dimensions; the carrier-side volumetric adjustment is Goship's responsibility. Unit-tested in `weight_test.go`.

## 8. Configuration

```bash
# Goship
GOSHIP_MODE=mock                    # mock | sandbox | production
GOSHIP_TOKEN=                       # Bearer token (required for sandbox/production)
GOSHIP_BASE_URL=https://sandbox.goship.io/api/v2
GOSHIP_DEFAULT_ITEM_WEIGHT_G=500
GOSHIP_DEFAULT_LENGTH_CM=20
GOSHIP_DEFAULT_WIDTH_CM=15
GOSHIP_DEFAULT_HEIGHT_CM=10

# Shipping selector
SHIPPING_PROVIDER=goship            # goship | flat
```
`GoshipConfig{Mode, Token, BaseURL, DefaultItemWeightG, DefaultDims}` validated at startup (token required unless mock). Wired in `cmd/api/main.go` alongside the existing shipping factory. `GOSHIP_CLIENT_SECRET` (for webhook HMAC) and login/refresh-token support are deferred to Spec B — Spec A uses the static `GOSHIP_TOKEN` only.

## 9. Error Handling

| Condition | Error | Surfaced as |
|-----------|-------|-------------|
| Address missing any code | `ErrAddressIncomplete` | preview `address_incomplete=true`; place 422 |
| Chosen carrier gone at place | `ErrCarrierUnavailable` | place 409 → FE re-previews |
| Goship API down / timeout | `ErrShippingUnavailable` | preview 503; never silently flat-rate |
| Inconsistent codes (ward∉district) | `ErrInvalidLocation` | address create/update 422 |
| Goship returns zero rates | empty options + warning | preview shows "no carrier serves this route" |

Goship HTTP client uses a context timeout and returns typed errors; transient failures bubble up rather than producing a wrong fee.

## 10. Testing

- **Unit:** mock client behavior; `weight.ChargeableGrams` matrix (actual>vol, vol>actual, missing fields→defaults); flat-rate single-option; factory mode switch; location code consistency validation.
- **Integration (`integration` tag):** checkout preview returns options per brand; place-order re-quotes & stores `shipping_carrier`/fee; address-incomplete blocks place; carrier-unavailable path.
- **Real sandbox (`goship_real` tag):** `Cities/Districts/Wards` return data; `Rates` returns ≥1 carrier for a known HCMC→Hanoi route; skips if `GOSHIP_TOKEN` unset (mirrors `payos_real`).

## 11. Confirmed Contract (verified against the live API — Task 17, 2026-06-04)

Verified with a real production token against **`https://api.goship.io/api/v2`** (the sandbox host `sandbox.goship.io` returned 401 for this token — the token is production-scoped). `GET /cities` and `POST /rates` both parse cleanly into the implemented client — **no `client_http.go` changes were required**.

- **Base URL:** production = `https://api.goship.io/api/v2`. (Set per-env via `GOSHIP_BASE_URL`; config default remains the sandbox URL.)
- **Auth:** static `Authorization: Bearer <token>`. The provided token's `exp` is ~2036 (effectively long-lived). `/login` exchange not needed when a token is supplied.
- **`/cities`** → `{code,status,data:[{id (numeric string), name, ...}]}`. 63 cities; e.g. **Hà Nội `id="100000"`**. Our `json.Number`→string handling works.
- **`/cities/{id}/districts`** → `data:[{id, name, city_id, support_carriers:[...]}]`. District `id` is a string (e.g. `"100300"`). Each district lists `support_carriers`, e.g. `["vtp","kerry","ov","supership","ghnv3","vnp","hola","snappy","nhattin","shopee","jnt","ems","jnt2"]`. **GHN = `ghnv3`** confirmed.
- **`/rates`** → `data:[{id, carrier_name, carrier_logo, service, expected, cod_fee, total_fee, total_amount}]`. **There is NO short carrier-code field** — only `carrier_name` (a display name). For a Hà Nội intra-city route the carriers returned were: `Vietnam Post`, `SPX Express`, `Best Express`, `Giao Hàng Nhanh (v3)`, `J&T Express 1`.
  - **Decision:** carrier selection therefore keys on the **display name** (`carrier_name`). The HTTP client sets `Rate.Carrier = carrier_name` (fallback already in place); the **mock now mirrors this** (`Carrier == CarrierName == display name`, no short codes) so dev/test and prod use the same identifier shape. Preview and place-order re-quote use the same client/mode, so the round-trip match is consistent.
- **Volumetric:** applied server-side by Goship from the `weight` + `width/height/length` we send (we do not pre-compute).
- **COD:** `parcel.cod`/`parcel.amount` accepted. Spec A sends `cod = sub-order subtotal` for COD (0 for PayOS) — see tracked limitation (refine in Spec B to include shipping in the collected amount).

Remaining (Spec B): shipment creation (`POST /shipments`) field contract, tracking, webhook `x-goship-hmac-sha256` verification (needs `GOSHIP_CLIENT_SECRET`), cancel.
