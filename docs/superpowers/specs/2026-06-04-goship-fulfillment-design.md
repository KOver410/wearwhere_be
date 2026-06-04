# Goship Shipping — Spec B: Fulfillment Lifecycle (brand confirm/ship + tracking webhook)

**Date:** 2026-06-04
**Branch (target):** `sprint-3-orders-checkout` (continuation) — or a dedicated `goship-fulfillment` branch
**Status:** APPROVED FOR PLANNING
**Scope:** Brand-side fulfillment — confirm a sub-order, ship it (create a real Goship shipment + store tracking), and drive delivery status from the Goship webhook. Plus: order auto-completion, COD-paid-on-delivery, customer tracking visibility, and a dev mock/simulate path.

This is **Spec B of 2**. Spec A (`2026-06-03-goship-shipping-rates-design.md`, DONE) delivered location codes, the Goship client (rates/locations), carrier-selectable checkout, and the chosen carrier + fee locked on each sub-order.

---

## 1. Goals

| Goal | Spec B coverage |
|------|-----------------|
| Brand sees and fulfills its sub-orders | Full — list/detail + confirm + ship endpoints under `/api/v1/brand/me` |
| Real Goship shipment creation at ship | Full — `POST /shipments` with a fresh rate, store tracking/label/cost |
| Delivery status from carrier | Full — `x-goship-hmac-sha256` webhook → sub-order status (idempotent) |
| Order completion | Full — all sub-orders `delivered` → order `completed` |
| COD settlement | Full — COD payment marked `paid` + stock committed on delivery |
| Customer tracking | Full — tracking_no/carrier/status/url/status_text surfaced in order detail |
| Safe dev/testing | Full — mock shipment + `POST /dev/goship/simulate` (never hit real Goship) |

**Non-goals (deferred to a later spec):**
- Order **cancellation** (brand cancel, customer paid-cancel) and PayOS **refund**
- Goship `CancelShipment`
- **Return/lost** handling (restock, refunds) — webhook records the status text but does not transition the enum or restock
- `preparing` sub-status (flow skips straight confirmed → shipped)

## 2. Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Fulfillment flow | `pending → confirmed → shipped → delivered` (no `preparing`) | Fewer endpoints/states; matches the agreed scope |
| Shipment creation point | At **ship** (not confirm) | Confirm is a lightweight acknowledgement; the carrier order is created only when the brand is actually dispatching |
| Customer shipping fee | **Locked at checkout** (`shipping_fee_vnd` from Spec A) — never re-charged | E-commerce standard; the merchant absorbs carrier price drift between checkout and ship |
| Actual carrier cost | Re-quoted at ship, stored in new `shipping_cost_vnd` | Margin/reconciliation; separate from the customer-charged fee |
| Rate at ship | Re-quote Goship by the stored `shipping_carrier` to get a fresh `rate_id`; body may override carrier if it dropped | Goship `rate_id`s are short-lived; cannot reuse the checkout-time id |
| Delivery status source | Goship **webhook** is authoritative (no manual brand override) | Single source of truth; mirrors how PayOS payment status works |
| Webhook correlation | Match sub-order by Goship tracking `code` (stored as `tracking_no`) FOR UPDATE | Deterministic; we set `tracking_no = code` at ship |
| Webhook security | HMAC-SHA256(rawBody, `GOSHIP_CLIENT_SECRET`) base64 == `x-goship-hmac-sha256` | Per Goship docs; mirrors PayOS signature pattern |
| COD settlement | On `delivered`: mark COD payment `paid` + commit reserved stock | Closes the COD inventory/payment loop (Spec A left COD stock reserved indefinitely) |
| Shipment creation dependency | Fulfillment service depends on `goship.Client` directly (not the rate `ShippingProvider`) | Shipment creation is Goship-specific; the client is always wired (mock or http) regardless of `SHIPPING_PROVIDER` |
| Dev safety | Mock client implements `CreateShipment`/`VerifyWebhookSignature`; `POST /dev/goship/simulate` fires fake webhooks | Production token must never create real shipments during dev/test |

## 3. Architecture & Package Layout

```
db/migrations/
  000031_add_fulfillment_fields_to_sub_orders.{up,down}.sql   NEW

internal/shipping/goship/
  client.go            MODIFY  add CreateShipment + VerifyWebhookSignature + DTOs (ShipmentReq/Resp, WebhookPayload)
  client_http.go       MODIFY  POST /shipments; HMAC verify with client secret
  client_mock.go       MODIFY  fake CreateShipment (mock tracking/cost); verify -> nil
  factory.go           MODIFY  Config gains ClientSecret; thread into HTTPClient
  status.go            NEW     MapStatus(goshipStatus, isReturn, isLost) -> (SubOrderStatus-ish category)
  status_test.go       NEW

internal/order/domain/
  order.go             MODIFY  SubOrder: add ShippingCostVND *int64, GoshipShipmentCode *string, TrackingURL *string, ShippingStatusText *string
  dto.go               MODIFY  SubOrderResp add tracking/url/status_text; BrandOrderListItem/Resp; ShipReq
  errors.go            MODIFY  ErrSubOrderNotFound, ErrNotBrandOwner, ErrInvalidTransition, ErrShipmentCreateFailed
  fulfillment.go       NEW     transition guards (CanConfirm, CanShip) + small helpers
  fulfillment_test.go  NEW

internal/order/repo/
  sub_order_pg.go      MODIFY  GetByID, GetByTrackingNoForUpdate, ListByBrand, UpdateConfirmed, UpdateShipped, UpdateDelivered, UpdateShippingStatus
  order_pg.go          MODIFY  UpdateStatusOnComplete; AllSubOrdersDelivered (or a count helper)

internal/order/service/
  fulfillment_service.go        NEW  ListBrandOrders, BrandOrderDetail, Confirm, Ship (+ completion check)
  fulfillment_service_test.go   NEW  (integration)
  shipping_webhook_service.go   NEW  HandleGoshipWebhook (idempotent, FOR UPDATE) — mirrors payment/webhook_service.go
  shipping_webhook_service_test.go NEW (integration)

internal/order/handler/
  brand_fulfillment_handler.go  NEW  list/detail/confirm/ship handlers
  shipping_webhook_handler.go   NEW  POST /shipping/goship/webhook + POST /dev/goship/simulate
  routes.go                     MODIFY  MountBrand(brandGroup) + MountShippingPublic/Dev

internal/config/config.go       MODIFY  GoshipConfig.ClientSecret (GOSHIP_CLIENT_SECRET)
cmd/api/main.go                 MODIFY  wire fulfillment service + brand routes + webhook routes; pass client secret to goship factory
.env.example                    MODIFY  GOSHIP_CLIENT_SECRET
```

## 4. Data Model Changes

### 4.1 Migration `000031_add_fulfillment_fields_to_sub_orders`
```sql
ALTER TABLE sub_orders
  ADD COLUMN shipping_cost_vnd     BIGINT,
  ADD COLUMN goship_shipment_code  TEXT,
  ADD COLUMN tracking_url          TEXT,
  ADD COLUMN shipping_status_text  TEXT;
-- down: DROP COLUMN IF EXISTS (each), matching repo convention
```
All nullable. `tracking_no`, `shipping_carrier`, `shipping_provider`, `confirmed_at`, `shipped_at`, `delivered_at`, `cancelled_at` already exist (migrations 000024/000030 + Spec A).

### 4.2 `SubOrder` struct additions
`ShippingCostVND *int64`, `GoshipShipmentCode *string`, `TrackingURL *string`, `ShippingStatusText *string`.

### 4.3 `SubOrderResp` (customer + brand)
Adds `tracking_no`, `shipping_carrier`, `tracking_url`, `shipping_status_text`, plus the existing `status`. Customer order-detail surfaces these so the buyer can track.

## 5. Goship Client Additions (contract confirmed where possible; `POST /shipments` pinned at impl)

- **CreateShipment** — `POST /shipments`:
  ```json
  {"shipment":{"rate":"<rate_id>",
    "address_from":{...sender/brand pickup, name, phone, district, city...},
    "address_to":{...recipient snapshot...},
    "parcel":{"cod":<int>,"amount":<int>,"weight":<g>,"length":<cm>,"width":<cm>,"height":<cm>},
    "order_id":"<our sub_order reference>"}}
  ```
  Returns (shape pinned at impl) the tracking `code`, internal `gcode`, `label`/sorting url, and the fee. We store `tracking_no = code`, `goship_shipment_code = gcode`, `shipping_cost_vnd = fee`, `tracking_url`.
  > `rate` is a fresh `rate_id` from a re-quote at ship time (Goship requires a rates call first — confirmed in Spec A §5). Exact `/shipments` request/response field names are confirmed against the live API during implementation (Task: real check), behind the `goship_real` build tag, and **without sending a real create** unless explicitly run.
- **VerifyWebhookSignature(rawBody []byte, signature string) error** — `base64(HMAC_SHA256(rawBody, clientSecret)) == signature`, constant-time compare; `ErrSignatureInvalid` otherwise.
- **WebhookPayload** (confirmed from docs):
  ```json
  {"gcode","code","order_id","status","status_text","message","tracking_url","is_return","is_lost","carrier_short_name","update_time"}
  ```
- **Mock client:** `CreateShipment` returns `{code:"MOCK-TRK-<n>", gcode:"MOCK-GS-<n>", fee: <from req or fixed>, label_url:"https://mock/label"}`; `VerifyWebhookSignature` returns nil.
- **Config:** `GoshipConfig.ClientSecret` (`GOSHIP_CLIENT_SECRET`); factory passes it to `HTTPClient` for webhook verification. `client_id`/secret already present in `.env`.

## 6. Status Mapping (`status.go`)

Goship sends string status codes + `is_return`/`is_lost` flags + human `status_text`. Mapping → coarse sub-order status:
- delivered/“đã giao”/completed → **delivered**
- pickup waiting (`"901"` Chờ lấy hàng) / picked up / in transit / delivering → **shipped**
- `is_return == 1` or `is_lost == 1` or return/lost codes → **record `shipping_status_text` only**; no enum transition, no restock (deferred)
- unknown → treat as **shipped** (in-progress) and store the raw `status_text`

The exact numeric code set is confirmed/extended at implementation (codes only appear on live shipments). `MapStatus` returns a small enum `{Shipped, Delivered, Other}` so the webhook service decides transitions; it is pure and unit-tested.

## 7. Flows

### 7.1 Brand confirm — `POST /brand/me/orders/:sub_order_id/confirm`
Load sub-order (must belong to `ctx brand_id` → else 403 `ErrNotBrandOwner`). Guard `CanConfirm` (status == pending). Set status=confirmed, confirmed_at=now.

### 7.2 Brand ship — `POST /brand/me/orders/:sub_order_id/ship`
Body: `{ "carrier"?: string }` (optional override).
1. Load sub-order (brand-owned) + parent order. Guard `CanShip` (status == confirmed AND order.status == processing). PayOS unpaid orders are not `processing`, so they’re blocked.
2. Determine `from` = brand primary pickup address codes; `to` = order `shipping_address` snapshot codes (Spec A stored them). Parcel = `weight.Aggregate(items, defaults)`. `cod` = (COD ? sub-order subtotal + shipping_fee_vnd : 0); `amount` = subtotal.
3. `Rates(from,to,parcel,cod)` → pick the option whose `Carrier` == (body.carrier ?? stored `shipping_carrier`). If none → `ErrCarrierUnavailable` (brand re-tries with another carrier). Use that option’s `rate_id`.
4. `CreateShipment(rate_id, addresses, parcel, order_id=sub_order.id)` → on success store `tracking_no`, `goship_shipment_code`, `shipping_cost_vnd`, `tracking_url`, `shipping_carrier` (the actually-used carrier), status=shipped, shipped_at=now. On failure → `ErrShipmentCreateFailed` (no state change).
5. `shipping_fee_vnd` (customer charge) is untouched.

### 7.3 Goship webhook — `POST /shipping/goship/webhook`
1. Read raw body; `VerifyWebhookSignature(raw, header)` (skip in mock mode). Invalid → 401.
2. Parse payload. BEGIN tx (ReadCommitted). `GetByTrackingNoForUpdate(code)`. Not found → commit/200 (no-op, tolerant).
3. `MapStatus(...)`:
   - **Delivered** and sub-order not already delivered → set status=delivered, delivered_at, status_text, tracking_url. If parent order payment is COD & pending → mark payment paid (`UpdateOnPaid`) + commit reserved stock for the order’s items. Then if **all** sub-orders of the order are delivered → `UpdateStatusOnComplete` (processing → completed).
   - **Shipped** → if currently pending/confirmed, advance to shipped (+shipped_at if unset); always update `shipping_status_text`/`tracking_url`.
   - **Other** (return/lost/unknown) → update `shipping_status_text` only; log.
4. Idempotent: a webhook that doesn’t advance state just updates status_text and returns 200.
5. Commit. Always 200 unless signature invalid or an unexpected error.

Dev: `POST /dev/goship/simulate` accepts `{tracking_no, status}` and calls the same service with a synthesized payload (mounted only when `GOSHIP_MODE=mock` or app env != production, mirroring the PayOS dev endpoints).

### 7.4 Order completion
`OrderRepo.UpdateStatusOnComplete(orderID)` sets `status='completed', updated_at=now WHERE id=$1 AND status='processing'`. Called after a delivery once `AllSubOrdersDelivered(orderID)` (count where status != 'delivered' == 0) is true. Idempotent via the `WHERE status='processing'` guard.

## 8. Brand Order Listing/Detail
- `GET /brand/me/orders?status=&page=&page_size=` → sub-orders where `brand_id = ctx brand`, newest first, with order_no, customer recipient (from snapshot), totals, status, tracking. Paginated like the customer list.
- `GET /brand/me/orders/:sub_order_id` → sub-order + its items + the shipping address snapshot; 404 `ErrSubOrderNotFound`, 403 if not brand-owned.

## 9. Error Handling

| Condition | Error | HTTP |
|-----------|-------|------|
| Sub-order not found | `ErrSubOrderNotFound` | 404 |
| Sub-order not owned by brand | `ErrNotBrandOwner` | 403 |
| Confirm when not pending / ship when not confirmed / order not paid | `ErrInvalidTransition` | 409 |
| Chosen carrier not available at ship | `ErrCarrierUnavailable` (reused) | 409 |
| Goship CreateShipment fails | `ErrShipmentCreateFailed` | 502 |
| Webhook bad signature | `ErrSignatureInvalid` | 401 |
| Webhook unknown tracking | (none) | 200 no-op |

## 10. Testing
- **Unit:** `MapStatus` table; mock `CreateShipment`; `VerifyWebhookSignature` (valid/tampered); `CanConfirm`/`CanShip` transition guards.
- **Integration (`integration`):** confirm→ship with mock client stores tracking/cost/status; webhook `delivered` for a COD order → payment paid + stock committed + (single-brand) order completed; PayOS order delivered → order completed without touching payment; webhook idempotent (second delivered is a no-op); brand-owner guard (cross-brand → 403); ship blocked when order not `processing` (PayOS unpaid).
- **Real (`goship_real`, gated):** confirm `POST /shipments` request/response field names against the live API — build the request and validate the rate step; do **not** create a real shipment unless an explicit opt-in env (`GOSHIP_ALLOW_REAL_CREATE=1`) is set.

## 11. Contract Status (as of impl 2026-06-04)

**Confirmed / validated:**
- `POST /rates` works against the live `https://api.goship.io/api/v2` (Spec A §11) and is the source of the fresh `rate_id` used at ship time. The gated test `TestRealGoship_CreateShipment_Gated` exercises the rates→(would-create) path live and logs the rate to be used.
- Webhook HMAC verification, status mapping, the full fulfillment lifecycle, COD settlement, and order completion are all covered by integration tests against the MOCK Goship client — no real shipments are created in tests.

**NOT yet confirmed (assumed shape, isolated in `client_http.go`):**
- `POST /shipments` request/response field names (the returned tracking `code` / `gcode` / `label` / `total_fee` keys, and whether `address_from`/`address_to` require fields beyond name/phone/street/district/city). The configured token is a **production** token, so the real `CreateShipment` is gated behind `GOSHIP_ALLOW_REAL_CREATE=1` to avoid booking a real delivery order. **Before production cutover**, run the gated test against a sandbox account (or with an intentional single booking) and reconcile the field mapping in `client_http.go` + this section.
- The full Goship delivery status code set. `status.go` seeds the known `"901"` (waiting pickup) + delivered-text matching; numeric codes are extended as observed on live shipments. Return/lost shipments are recorded (status_text) but not auto-restocked (deferred).
