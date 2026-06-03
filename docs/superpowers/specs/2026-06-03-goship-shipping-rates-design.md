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
| Weight model | Chargeable weight = `max(actual_g, volumetric_g)`, volumetric = `(L×W×H cm)/5000`, summed over `item.qty` | Vietnamese carrier standard (GHN/GHTK divisor 5000); large-marketplace best practice |
| Missing weight/dims | Fallback `GOSHIP_DEFAULT_ITEM_WEIGHT_G` + default dims in config | Variants may not have weight yet; never block checkout on missing data |
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
  ADD COLUMN city_code     INT,
  ADD COLUMN district_code INT,
  ADD COLUMN ward_code     INT;
-- same three columns on brand_addresses
```
Nullable (legacy rows have none). New/updated addresses created via dropdown must supply all three; partial codes rejected at validation.

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
`ShippingAddress` struct gains `city_code`, `district_code`, `ward_code` (snapshotted at place-order so Spec B can create the shipment from frozen data).

## 5. Goship Client Contract (to confirm against sandbox)

> Exact endpoint paths/field names pinned during implementation against the sandbox account — doc.goship.io was unreachable at design time. Shape below reflects Goship v2.

- **Auth:** `Authorization: Bearer <GOSHIP_TOKEN>` (static token from connection settings).
- **Locations:** `GET /cities`, `GET /cities/{cityCode}/districts`, `GET /districts/{districtCode}/wards` → `[]Location{Code, Name}`.
- **Rates:** `POST /rates` with `{address_from:{district,city}, address_to:{district,city}, parcel:{weight_g, L,W,H}}` → `[]Rate{ID, Carrier, CarrierName, Service, TotalFeeVND, ExpectedDeliveryText}`.

Client interface (Spec A subset):
```go
type Client interface {
    Cities(ctx) ([]Location, error)
    Districts(ctx, cityCode int) ([]Location, error)
    Wards(ctx, districtCode int) ([]Location, error)
    Rates(ctx, RateReq) ([]Rate, error)
}
```
Mock returns a fixed carrier set (ghn/ghtk/vtp) with fee = `base + perKg * ceil(weightKg)` so tests are deterministic.

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

## 7. Chargeable Weight

```
volumetric_g(item)  = (L_cm * W_cm * H_cm) / 5000 * 1000
chargeable_g(item)  = max(actual_weight_g, volumetric_g)   // per unit
parcel_g(sub_order) = Σ item.qty * chargeable_g(item)
```
Missing field on a variant → substitute config defaults (`GOSHIP_DEFAULT_ITEM_WEIGHT_G`, default box dims). Divisor 5000 matches GHN/GHTK. Unit-tested in `weight_test.go`.

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
`GoshipConfig{Mode, Token, BaseURL, DefaultItemWeightG, DefaultDims}` validated at startup (token required unless mock). Wired in `cmd/api/main.go` alongside the existing shipping factory.

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

## 11. Open Items Pinned at Implementation
- Confirm exact Goship v2 endpoint paths, request/response field names, and whether token is static vs requires login/refresh (sandbox).
- Confirm carrier code list returned by the sandbox account (drives mock fixture).
- Confirm volumetric divisor per carrier (assume 5000; some use 6000) — make it a per-provider constant if it varies.
