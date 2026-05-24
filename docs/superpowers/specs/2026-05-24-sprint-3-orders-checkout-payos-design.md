# Sprint 3 — Orders, Checkout & PayOS Payment Design

**Date:** 2026-05-24
**Branch (target):** `sprint-3-orders-checkout`
**Status:** APPROVED FOR PLANNING
**Scope:** Customer-side order placement + checkout + PayOS payment + reservation lifecycle.
Brand fulfillment (UC45/46/47) and paid-order cancellation defer to Sprint 4.

---

## 1. Goals & SRS Use Case Coverage

| UC | Title | Sprint 3 coverage |
|----|-------|-------------------|
| UC17 | Checkout | Full — preview endpoint with multi-brand grouping & shipping per brand |
| UC18 | Select Payment Method | Full — `cod` and `payos` (replaces SRS-listed Momo/VNPay) |
| UC19 | Place Order | Full — atomic place with stock reservation, PayOS link creation |
| UC20 | Track Order | Partial — status field surfaced; per-brand status stays `pending` until Sprint 4 brand actions |
| UC21 | View Order History | Full — paginated list + detail |
| UC22 | Cancel Order | Partial — COD anytime (pre-confirm) and PayOS unpaid only. Paid cancel defer Sprint 4 |
| UC45/46/47 | Brand fulfillment | **Deferred to Sprint 4** |

**Non-goals (Sprint 3):**
- Brand-side endpoints (`/api/v1/brand/me/orders/*`)
- Real PayOS production calls (skeleton only; `PAYOS_MODE=mock` for now)
- Stock reservation timeout at cart-add time (SRS rule "30 min after adding to cart" — we reserve at place-order, not cart-add)
- Refund execution (cancel flow does not call PayOS refund API in Sprint 3 because no paid cancel allowed)
- Multi-currency (VND only)

## 2. Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Multi-brand model | Sub-order per brand | Brand fulfillment in Sprint 4 maps cleanly; each brand ships independently |
| Stock timing | Reserve at place_order, commit at paid, release at cancel/expire | PayOS payments can take minutes (banking transfer); reservation prevents oversell |
| Payment gateway | PayOS (replaces SRS Momo/VNPay) | User-requested; aggregator wraps banking + e-wallet + QR under one API |
| PayOS env | Skeleton + mock (`PAYOS_MODE=mock|production`) | No production creds available yet; mock unblocks E2E testing |
| Reservation timeout | 30 min via background cleanup job (every 5 min) | Matches PayOS link expiration; auto-release stock for abandoned orders |
| Shipping | Pluggable `ShippingProvider` interface; Sprint 3 impl reads `brands.shipping_flat_fee_vnd` | Future GHN/GHTK/ViettelPost integration without touching checkout code |
| Cancel rules (Sprint 3) | COD anytime pre-confirm; PayOS only when unpaid | Paid cancel needs brand approval (SRS BR for UC22) — defer with refund |
| Order ID format | `WW-YYYYMMDD-{6-char nanoid}` (alphabet excludes I/O/0/1) | Human-friendly, URL-safe, low collision |

## 3. Architecture & Package Layout

```
internal/order/                          # NEW
  domain/
    order.go              Order, SubOrder, OrderItem, ShippingAddress
    payment_status.go     PaymentMethod, PaymentStatus, OrderStatus, SubOrderStatus
    errors.go
    dto.go                CheckoutPreviewResp, PlaceOrderReq/Resp, ListResp, ...
    order_no.go           GenerateOrderNo()
    order_test.go         CanCustomerCancel state matrix
  repo/
    repo.go               OrderRepo, SubOrderRepo, OrderItemRepo interfaces
    order_pg.go           CRUD + tx-aware updates
    sub_order_pg.go
    order_item_pg.go
    *_test.go
  service/
    checkout_service.go   Preview (read-only, no reservation)
    order_service.go      PlaceOrder (tx), Cancel, List, Detail
    *_test.go
  handler/
    routes.go             RegisterMeRoutes + RegisterPublicRoutes + RegisterDevRoutes
    checkout_handler.go
    order_handler.go
    payment_handler.go    PayOS webhook + dev endpoints
    *_test.go

internal/payment/                        # NEW
  domain/
    payment.go            Payment struct
    errors.go
  repo/
    repo.go               PaymentRepo interface
    payment_pg.go
    payment_pg_test.go
  payos/
    client.go             interface Client + DTOs (CreateLinkReq, WebhookPayload, ...)
    client_http.go        Real HTTP impl (compile-ready, untested vs real PayOS)
    client_mock.go        Used when PAYOS_MODE=mock
    signature.go          HMAC-SHA256 sign + verify (shared by http + webhook handler)
    signature_test.go
    factory.go            NewFromConfig(cfg) -> Client

internal/shipping/                       # NEW
  domain/
    fee.go                FeeQuote
  provider/
    provider.go           interface ShippingProvider + CalcReq/CalcItem
    flat_rate.go          FlatRateProvider (Sprint 3)
    factory.go            NewFromConfig (Sprint 3 returns FlatRate; future: ghn, ghtk)
    flat_rate_test.go

internal/jobs/                           # NEW
  reservation_cleanup.go  Job struct + Run loop + cleanupOnce
  reservation_cleanup_test.go
```

**Modified existing code:**
- `internal/product/repo/variant_pg.go` — add `Reserve(ctx, variantID, qty, tx)`, `Commit(ctx, variantID, qty, tx)`, `Release(ctx, variantID, qty, tx)`
- `internal/brand/domain/brand.go` — add `ShippingFlatFeeVND int64` field
- `internal/brand/repo/brand_pg.go` — surface `shipping_flat_fee_vnd` in SELECT
- `internal/cart/service/` — no surface change; PlaceOrder reads cart via repo directly with FOR UPDATE
- `cmd/api/main.go` — wire 4 new modules (order, payment, shipping, jobs) + start cleanup job + add `cfg.Payos.*` and `cfg.Shipping.*`

## 4. Database Schema (Migrations 21–26)

### 000021 — variant.reserved_qty

```sql
ALTER TABLE product_variants
  ADD COLUMN reserved_qty INT NOT NULL DEFAULT 0,
  ADD CONSTRAINT chk_variant_reserved_nonneg CHECK (reserved_qty >= 0),
  ADD CONSTRAINT chk_variant_reserved_le_stock CHECK (reserved_qty <= stock_qty);
```

### 000022 — brand.shipping_flat_fee_vnd

```sql
ALTER TABLE brands
  ADD COLUMN shipping_flat_fee_vnd BIGINT NOT NULL DEFAULT 30000
    CHECK (shipping_flat_fee_vnd >= 0);
```

### 000023 — orders

```sql
CREATE TABLE orders (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  order_no TEXT NOT NULL UNIQUE,
  subtotal_vnd BIGINT NOT NULL CHECK (subtotal_vnd >= 0),
  shipping_total_vnd BIGINT NOT NULL CHECK (shipping_total_vnd >= 0),
  grand_total_vnd BIGINT NOT NULL CHECK (grand_total_vnd >= 0),
  payment_method TEXT NOT NULL CHECK (payment_method IN ('cod','payos')),
  payment_status TEXT NOT NULL DEFAULT 'pending'
    CHECK (payment_status IN ('pending','paid','failed','cancelled')),
  status TEXT NOT NULL DEFAULT 'pending_payment'
    CHECK (status IN ('pending_payment','processing','cancelled','completed')),
  shipping_address JSONB NOT NULL,
  notes TEXT,
  cancel_reason TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  paid_at TIMESTAMPTZ,
  cancelled_at TIMESTAMPTZ
);
CREATE INDEX idx_orders_user_created ON orders(user_id, created_at DESC);
```

### 000024 — sub_orders

```sql
CREATE TABLE sub_orders (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id UUID NOT NULL REFERENCES orders(id),
  brand_id UUID NOT NULL REFERENCES brands(id),
  subtotal_vnd BIGINT NOT NULL CHECK (subtotal_vnd >= 0),
  shipping_fee_vnd BIGINT NOT NULL CHECK (shipping_fee_vnd >= 0),
  total_vnd BIGINT NOT NULL CHECK (total_vnd >= 0),
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','confirmed','preparing','shipped','delivered','cancelled')),
  tracking_no TEXT,
  shipping_provider TEXT,
  confirmed_at TIMESTAMPTZ,
  shipped_at TIMESTAMPTZ,
  delivered_at TIMESTAMPTZ,
  cancelled_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (order_id, brand_id)
);
CREATE INDEX idx_sub_orders_brand_status ON sub_orders(brand_id, status, created_at DESC);
```

### 000025 — order_items

```sql
CREATE TABLE order_items (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  sub_order_id UUID NOT NULL REFERENCES sub_orders(id),
  variant_id UUID NOT NULL REFERENCES product_variants(id),
  product_id UUID NOT NULL REFERENCES products(id),
  product_name TEXT NOT NULL,
  variant_label TEXT NOT NULL,
  image_url TEXT,
  qty INT NOT NULL CHECK (qty > 0),
  unit_price_vnd BIGINT NOT NULL CHECK (unit_price_vnd >= 0),
  line_total_vnd BIGINT NOT NULL CHECK (line_total_vnd >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_order_items_sub_order ON order_items(sub_order_id);
```

### 000026 — payments

```sql
CREATE TABLE payments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id UUID NOT NULL REFERENCES orders(id),
  amount_vnd BIGINT NOT NULL CHECK (amount_vnd > 0),
  method TEXT NOT NULL CHECK (method IN ('cod','payos')),
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','paid','failed','cancelled','expired')),
  payos_order_code BIGINT UNIQUE,
  payos_payment_link_id TEXT,
  payos_checkout_url TEXT,
  payos_qr_code TEXT,
  expired_at TIMESTAMPTZ,
  paid_at TIMESTAMPTZ,
  failure_reason TEXT,
  raw_webhook_payload JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_payments_order_status ON payments(order_id, status);
```

**Notes:**
- `shipping_address JSONB` is a snapshot — user can delete address row later, order stays intact.
- `order_items` snapshots product_name, variant_label, image_url, unit_price_vnd so historical orders stay coherent if catalog changes.
- 1 payment per order in Sprint 3 — no retry. PayOS fail → user cancels & re-places.
- `orders.payment_status` enum is **coarser** than `payments.status` enum: orders has no `expired` (it maps to `cancelled`). When the cleanup job marks `payment.status='expired'`, it sets `order.payment_status='cancelled'`.

## 5. Domain Types & State Machine

### 5.1 Enums

```go
type PaymentMethod string  // "cod" | "payos"
type PaymentStatus string  // "pending" | "paid" | "failed" | "cancelled" | "expired" (payments only)
type OrderStatus    string // "pending_payment" | "processing" | "cancelled" | "completed"
type SubOrderStatus string // "pending" | "confirmed" | "preparing" | "shipped" | "delivered" | "cancelled"
```

### 5.2 State machine (Sprint 3 transitions)

```
PayOS flow:
  [POST /me/orders]
    → order.status = pending_payment
      order.payment_status = pending
      payment.status = pending
      sub_orders.status = pending
      reserved_qty += qty
    → PayOS webhook PAID:
        order.status = processing
        order.payment_status = paid
        payment.status = paid
        stock_qty -= qty; reserved_qty -= qty   (commit)
    → PayOS webhook FAILED, or expired by cleanup job, or customer cancels:
        order.status = cancelled
        order.payment_status = failed | cancelled
        payment.status = failed | cancelled | expired
        reserved_qty -= qty   (release; stock_qty unchanged)

COD flow:
  [POST /me/orders]
    → order.status = processing      (skip pending_payment)
      order.payment_status = pending
      payment.status = pending
      sub_orders.status = pending
      reserved_qty += qty
    → customer cancels (allowed only if all sub_orders.status = 'pending'):
        order.status = cancelled
        payment.status = cancelled
        reserved_qty -= qty   (release)

Sub-order transitions confirmed → preparing → shipped → delivered are Sprint 4 (brand actions).
```

### 5.3 Cancel rules (`order.CanCustomerCancel()`)

| Method | order.status | payment_status | Any sub_order ≠ pending | Allowed |
|--------|-------------|----------------|-------------------------|---------|
| payos  | pending_payment | pending | no | ✅ |
| payos  | processing | paid | no | ❌ `paid_not_supported` |
| cod    | processing | pending | no | ✅ |
| any    | processing | any | yes | ❌ `already_shipped` |
| any    | cancelled | any | any | ❌ `already_cancelled` |
| any    | completed | any | any | ❌ `already_completed` |

## 6. PayOS Integration

### 6.1 Client interface

```go
type Client interface {
    CreateLink(ctx context.Context, r CreateLinkReq) (*CreateLinkResp, error)
    VerifyWebhookSignature(p WebhookPayload) error
    GetPayment(ctx context.Context, paymentLinkID string) (*PaymentInfo, error)
    CancelLink(ctx context.Context, paymentLinkID, reason string) error
}

type CreateLinkReq struct {
    OrderCode   int64       // PayOS unique numeric code (we store as payments.payos_order_code)
    AmountVND   int64
    Description string      // PayOS hard-limit 25 chars: fmt "DH %s" with order_no truncated
    Items       []LineItem
    ReturnURL   string
    CancelURL   string
    Buyer       Buyer
    ExpiredAt   int64       // unix seconds
}

type WebhookPayload struct {
    Code      string      // "00" = success
    Desc      string
    Success   bool
    Data      WebhookData
    Signature string
}
type WebhookData struct {
    OrderCode           int64
    Amount              int64
    Description         string
    AccountNumber       string
    Reference           string
    TransactionDateTime string
    PaymentLinkID       string
}
```

### 6.2 Signature algorithm

```go
// PayOS spec: HMAC-SHA256 over key=value pairs of `data` object, sorted alphabetically by key,
// concatenated with '&'.
// Example: "accountNumber=12345&amount=100000&description=foo&orderCode=1&reference=ref1"
func Sign(checksumKey string, fields map[string]any) string {
    keys := sortedKeys(fields)
    var sb strings.Builder
    for i, k := range keys {
        if i > 0 { sb.WriteByte('&') }
        sb.WriteString(k); sb.WriteByte('='); sb.WriteString(fmt.Sprint(fields[k]))
    }
    h := hmac.New(sha256.New, []byte(checksumKey))
    h.Write([]byte(sb.String()))
    return hex.EncodeToString(h.Sum(nil))
}
```

### 6.3 Webhook handler flow

```
POST /api/v1/payments/payos/webhook
  1. parse body → WebhookPayload
  2. payosClient.VerifyWebhookSignature(p)
       err → respond 401, log warning (PayOS won't retry on 401 by docs; still log)
  3. payment := repo.GetByPayosOrderCode(ctx, p.Data.OrderCode)
       err == not found → respond 200, log warn (could be retry after we've migrated DB; accept)
  4. IF payment.Status != 'pending' → respond 200 OK (idempotent — already processed)
  5. BEGIN TX (acquire row locks on payment + order + sub_orders + variants involved):
       IF p.Success && p.Code == "00":
         payment.Status = 'paid'; paid_at = NOW(); raw_webhook_payload = body
         order.Status = 'processing'; order.PaymentStatus = 'paid'; paid_at = NOW()
         FOR EACH order_item:
           UPDATE product_variants
              SET stock_qty = stock_qty - $qty,
                  reserved_qty = reserved_qty - $qty
            WHERE id = $variant_id
              AND reserved_qty >= $qty
              AND stock_qty >= $qty;
           (rowsAffected == 1 expected; if not → ROLLBACK + 5xx so PayOS retries)
       ELSE:
         payment.Status = 'failed'; failure_reason = p.Desc; raw_webhook_payload = body
         order.Status = 'cancelled'; order.PaymentStatus = 'failed'; cancelled_at = NOW()
         order.cancel_reason = 'payos_payment_failed'
         FOR EACH sub_order: status = 'cancelled'; cancelled_at = NOW()
         FOR EACH order_item:
           UPDATE product_variants
              SET reserved_qty = reserved_qty - $qty
            WHERE id = $variant_id
              AND reserved_qty >= $qty;
     COMMIT
  6. respond 200 OK
```

**Idempotency:** the early-out at step 4 means PayOS retries (e.g., due to network timeout on their side) won't double-commit stock.

### 6.4 Mock mode

When `PAYOS_MODE=mock`:
- `MockClient.CreateLink` returns `CheckoutURL = /dev/payos/mock-checkout?orderCode={N}`, `QRCode = "mock-qr-base64"`, `PaymentLinkID = "mock-pl-{seq}"`.
- `MockClient.VerifyWebhookSignature` accepts any payload.
- Dev endpoints exposed:
  - `GET /dev/payos/mock-checkout?orderCode=N` — HTML page with two buttons (success/fail).
  - `POST /dev/payos/simulate-webhook` — body `{orderCode: int64, success: bool}` — constructs WebhookPayload and invokes the real webhook handler internally; then redirects to FE return/cancel URL.
- Dev routes registered ONLY when `cfg.Payos.Mode == "mock"`.

## 7. Shipping Provider

```go
type ShippingProvider interface {
    Calculate(ctx context.Context, r CalcReq) (*FeeQuote, error)
}

type CalcReq struct {
    BrandID   uuid.UUID
    ToAddress order.ShippingAddress
    Items     []CalcItem
}
type CalcItem struct {
    VariantID, ProductID uuid.UUID
    Qty                  int
    WeightG              int   // Sprint 3 unused — variants don't have weight yet
}
type FeeQuote struct {
    AmountVND   int64
    Currency    string   // "VND"
    ProviderRef string   // vendor-side quote id (Sprint 4+ for re-price)
    ETA         *time.Duration
}

type FlatRateProvider struct {
    brandRepo brand.Repo
}
func (p *FlatRateProvider) Calculate(ctx context.Context, r CalcReq) (*FeeQuote, error) {
    b, err := p.brandRepo.GetByID(ctx, r.BrandID)
    if err != nil { return nil, err }
    return &FeeQuote{AmountVND: b.ShippingFlatFeeVND, Currency: "VND"}, nil
}
```

Factory (`internal/shipping/provider/factory.go`):
```go
func NewFromConfig(cfg config.Shipping, brandRepo brand.Repo) (ShippingProvider, error) {
    switch cfg.Provider {
    case "", "flat": return &FlatRateProvider{brandRepo: brandRepo}, nil
    // future: case "ghn": return ghn.New(cfg.GHN.Token, ...), nil
    default: return nil, fmt.Errorf("unknown shipping provider: %s", cfg.Provider)
    }
}
```

## 8. Place-Order Transaction Flow

```
PlaceOrder(ctx, userID uuid.UUID, req PlaceOrderReq) (Order, *Payment, error):

  1. Validate input:
     - req.AddressID != nil → addrRepo.GetByID, check user_id match (else ErrAddressNotFound)
     - req.PaymentMethod ∈ {cod, payos}
     - req.Notes ≤ 500 chars
     shippingAddress := snapshot(addr)

  2. BEGIN TX (READ COMMITTED)

  3. Snapshot cart with FOR UPDATE on variants:
     SELECT ci.variant_id, ci.qty, ci.price_snapshot,
            v.stock_qty, v.reserved_qty, v.is_active, v.deleted_at,
            v.product_id, v.label,
            p.name, p.brand_id, p.deleted_at,
            b.name AS brand_name, b.slug AS brand_slug
       FROM cart_items ci
       JOIN product_variants v ON v.id = ci.variant_id
       JOIN products p ON p.id = v.product_id
       JOIN brands b ON b.id = p.brand_id
      WHERE ci.user_id = $1
      FOR UPDATE OF v;
     If empty → ErrCartEmpty.

  4. Per-item validate:
     FOR EACH line:
       - v.is_active=true AND v.deleted_at IS NULL AND p.deleted_at IS NULL → else ErrVariantUnavailable
       - (v.stock_qty - v.reserved_qty) >= ci.qty → else ErrInsufficientStock {variant_id, requested, available}

  5. Group by brand, compute totals:
     FOR EACH brand:
       subtotal[brandID] = sum(qty * unit_price_vnd)
       shipping[brandID] = shippingProvider.Calculate(ctx, CalcReq{brandID, shippingAddress, items}).AmountVND
     orderSubtotal      = sum(subtotal[*])
     orderShippingTotal = sum(shipping[*])
     grandTotal         = orderSubtotal + orderShippingTotal

  6. IF orderSubtotal < 50_000 → ErrMinOrderValue

  7. orderNo := GenerateOrderNo()   // retry 3x on unique conflict

  8. INSERT orders → orderID
  9. FOR EACH brand:
       INSERT sub_orders(...) → subOrderID
       FOR EACH item:
         INSERT order_items(snapshot fields)
         // Reserve atomically (double-safety on top of FOR UPDATE)
         UPDATE product_variants
            SET reserved_qty = reserved_qty + $qty
          WHERE id = $variant_id
            AND (stock_qty - reserved_qty) >= $qty
            AND is_active = true AND deleted_at IS NULL;
         IF rowsAffected != 1 → ROLLBACK + ErrInsufficientStock

 10. Create payment row:
     IF method=cod:
       INSERT payments(method='cod', status='pending', amount=grandTotal)
       UPDATE orders SET status='processing'
     IF method=payos:
       payosOrderCode := nextPayosCode()   // timestamp-based int64
       expiresAt := NOW() + cfg.ReservationTimeoutMinutes minutes  // default 30
       INSERT payments(method='payos', status='pending', amount=grandTotal,
                       payos_order_code=payosOrderCode, expired_at=expiresAt)
       // orders.status stays 'pending_payment'

 11. IF method=payos:
       link, err := payosClient.CreateLink(ctx, CreateLinkReq{
         OrderCode: payosOrderCode,
         AmountVND: grandTotal,
         Description: truncate25(fmt.Sprintf("DH %s", orderNo)),  // PayOS hard-limit 25 chars; helper handles short strings safely
         Items: linesForPayos,
         ReturnURL: cfg.Payos.ReturnURL + "?orderNo=" + orderNo,
         CancelURL: cfg.Payos.CancelURL + "?orderNo=" + orderNo,
         Buyer: Buyer{user.Name, user.Phone, user.Email},
         ExpiredAt: time.Now().Add(time.Duration(cfg.ReservationTimeoutMinutes) * time.Minute).Unix(),
       })
       IF err != nil → ROLLBACK + ErrPayosLinkCreate (wrap original error)
       UPDATE payments
          SET payos_payment_link_id=link.PaymentLinkID,
              payos_checkout_url=link.CheckoutURL,
              payos_qr_code=link.QRCode

 12. DELETE FROM cart_items WHERE user_id = $1

 13. COMMIT

 14. Return (order with sub_orders + items loaded, payment)
```

**Concurrency tradeoffs:**
- `FOR UPDATE OF v` locks only variant rows in this cart. Two users buying the same variant serialize correctly.
- The PayOS HTTP call inside the tx holds variant locks for ~500 ms (mock: instant). Accepted because (a) lock scope is narrow, (b) alternative pattern (commit-then-call with compensating tx on failure) adds complexity not warranted in Sprint 3.
- The atomic `UPDATE ... WHERE (stock - reserved) >= qty` is a second safety net in case FOR UPDATE is bypassed.

## 9. API Endpoints

### Auth-required (JWT middleware), under `/api/v1/me/`:

| Method | Path | Use case |
|--------|------|----------|
| GET    | `/me/checkout/preview?address_id=UUID` | UC17 — dry-run with totals + warnings |
| POST   | `/me/orders` | UC19 — place order |
| GET    | `/me/orders?status=...&page=1&page_size=20&from=...&to=...` | UC21 — list |
| GET    | `/me/orders/:order_no` | UC20+UC21 — detail |
| POST   | `/me/orders/:order_no/cancel` | UC22 — cancel (rules in §5.3) |

### Public:

| Method | Path | Notes |
|--------|------|-------|
| POST   | `/api/v1/payments/payos/webhook` | PayOS callback — signature-protected |

### Dev-only (when `PAYOS_MODE=mock`):

| Method | Path | Notes |
|--------|------|-------|
| GET    | `/dev/payos/mock-checkout?orderCode=N` | HTML simulate page |
| POST   | `/dev/payos/simulate-webhook` | Body `{orderCode, success}` → invoke webhook handler |

### Response/error contracts

See Section 6 of design conversation for full JSON shapes. Key error subcodes for `POST /me/orders/:order_no/cancel` 409:
- `paid_not_supported` — PayOS paid, defer Sprint 4
- `already_shipped` — any sub_order.status ≠ pending
- `already_cancelled`
- `already_completed`

For `POST /me/orders` 409 ErrInsufficientStock body:
```json
{ "error": "insufficient_stock", "variant_id": "uuid", "requested": 3, "available": 1 }
```

## 10. Background Job — Reservation Cleanup

```go
// internal/jobs/reservation_cleanup.go
type ReservationCleanupJob struct {
    paymentRepo payment.Repo
    orderRepo   order.Repo
    db          *pgxpool.Pool
    log         *zap.Logger
}

func (j *ReservationCleanupJob) Run(ctx context.Context, interval time.Duration) {
    t := time.NewTicker(interval)
    defer t.Stop()
    for {
        select {
        case <-ctx.Done(): return
        case <-t.C:
            if err := j.cleanupOnce(ctx); err != nil {
                j.log.Error("reservation cleanup failed", zap.Error(err))
            }
        }
    }
}
```

`cleanupOnce` SQL skeleton (`$1` = `cfg.ReservationTimeoutMinutes`):
```sql
-- find expired pending PayOS payments
SELECT p.id, p.order_id
  FROM payments p
 WHERE p.method = 'payos'
   AND p.status = 'pending'
   AND p.created_at < NOW() - ($1 || ' minutes')::interval
 ORDER BY p.created_at ASC
 LIMIT 100;
```

For each result, in its own tx:
1. `SELECT FOR UPDATE` payment + order
2. Re-check `payment.status = 'pending'` (skip if changed)
3. Set payment.status='expired', order.status='cancelled', order.payment_status='cancelled', order.cancel_reason='payos_payment_timeout', cancelled_at=NOW()
4. Set all sub_orders.status='cancelled', cancelled_at=NOW()
5. Release reserved: FOR EACH order_item, `UPDATE variants SET reserved_qty = reserved_qty - qty WHERE id = $1 AND reserved_qty >= $2`
6. COMMIT

Started in `cmd/api/main.go`:
```go
cleanupCtx, cancelCleanup := context.WithCancel(context.Background())
go cleanupJob.Run(cleanupCtx, 5*time.Minute)
// graceful shutdown handler calls cancelCleanup() on SIGTERM
```

## 11. Configuration (env vars)

Add to `internal/config/config.go` (and `.env.example`):

```
PAYOS_MODE=mock                          # mock|production
PAYOS_CLIENT_ID=                         # required when mode=production
PAYOS_API_KEY=                           # required when mode=production
PAYOS_CHECKSUM_KEY=                      # required when mode=production
PAYOS_RETURN_URL=http://localhost:3000/checkout/success
PAYOS_CANCEL_URL=http://localhost:3000/checkout/cancel

RESERVATION_TIMEOUT_MINUTES=30           # consumed by cleanup job (drives both per-payment expired_at and cleanup query threshold)
RESERVATION_CLEANUP_INTERVAL_MINUTES=5   # how often cleanup job runs

SHIPPING_PROVIDER=flat                   # flat (Sprint 3); future: ghn|ghtk|viettelpost
```

Config struct validation: when `PAYOS_MODE=production`, all three credentials must be non-empty.

## 12. Test Strategy

### Unit tests (no DB, mock dependencies)
- `internal/order/domain/order_test.go` — `CanCustomerCancel` state matrix (8 cases)
- `internal/order/domain/order_no_test.go` — format regex `^WW-\d{8}-[A-Z0-9]{6}$`, exclusion of I/O/0/1
- `internal/payment/payos/signature_test.go` — HMAC match/mismatch, sorted-key concat
- `internal/payment/payos/client_mock_test.go` — mock CreateLink stable, VerifyWebhookSignature always-pass
- `internal/shipping/provider/flat_rate_test.go` — brand lookup, default 30k

### Repo integration tests (real Postgres, testfixtures)
- `internal/order/repo/order_pg_test.go` — Create/Get/List/Update + concurrent reserve race (2 goroutines on stock=1 → one succeeds, one 409)
- `internal/order/repo/sub_order_pg_test.go`, `order_item_pg_test.go` — basic CRUD
- `internal/payment/repo/payment_pg_test.go` — Get by payos_order_code (unique), status updates
- `internal/product/repo/variant_pg_test.go` — Reserve/Commit/Release atomic with CHECK constraint

### Service tests (mocked repos + mock PayOS)
- `internal/order/service/checkout_service_test.go` — Preview totals, multi-brand grouping, warnings
- `internal/order/service/order_service_test.go` — PlaceOrder: happy paths (cod, payos), rollback on PayOS fail, min order rejection, insufficient stock, address IDOR, cancel state machine

### Job test
- `internal/jobs/reservation_cleanup_test.go` — expired payment released; webhook-came-first no-op; non-PayOS skipped

### E2E test (extend `cmd/api/main_test.go`)
- `TestE2E_OrderPayosFlow` — register → cart-add (2 brands) → POST /me/orders payos → assert checkout_url → POST /dev/payos/simulate-webhook success → GET /me/orders/:order_no shows status=processing, payment_status=paid → variants.stock_qty decremented
- `TestE2E_OrderCODFlow` — similar but COD → status=processing immediately, payment_status=pending, variants reserved (not committed)
- `TestE2E_CancelPayosUnpaid` — PayOS placed, no webhook → POST cancel → released
- `TestE2E_InsufficientStockRace` — 2 parallel POST /me/orders on stock=1 → exactly one 201, one 409
- `TestE2E_ReservationCleanupJob` — place PayOS, manipulate `created_at` to >30 min ago, run cleanup → order cancelled, reserved released

## 13. Task Sequencing (25 beads tasks)

| Phase | # | Task | Depends |
|-------|---|------|---------|
| **A: Migrations** | A1 | Migration 000021: variant.reserved_qty + CHECK | — |
| | A2 | Migration 000022: brand.shipping_flat_fee_vnd | — |
| | A3 | Migration 000023: orders + cancel_reason | A1 |
| | A4 | Migration 000024: sub_orders | A3 |
| | A5 | Migrations 000025+000026: order_items + payments | A4 |
| **B: Foundation ops** | B1 | variant_pg Reserve/Commit/Release + tests | A1 |
| | B2 | shipping/provider interface + FlatRate + factory + tests | A2 |
| | B3 | brand_pg expose ShippingFlatFeeVND in Get/List | A2 |
| **C: Order data layer** | C1 | order/domain types + errors + DTOs + order_no + tests | A5 |
| | C2 | order_pg (create/get/list/update) + tests | C1 |
| | C3 | sub_order + order_item repo + tests | C1 |
| | C4 | payment/domain + payment_pg + tests | C1 |
| **D: PayOS** | D1 | payos client interface + signature + tests | — |
| | D2 | payos client_http real impl | D1 |
| | D3 | payos client_mock + factory + dev page handler | D1 |
| **E: Service layer** | E1 | checkout_service.Preview + tests | B2, C2 |
| | E2 | order_service.PlaceOrder tx flow + tests | B1, B2, C2-4, D1, D3 |
| | E3 | order_service Cancel/List/Detail + tests | E2 |
| **F: Handlers** | F1 | checkout_handler.Preview + route | E1 |
| | F2 | order_handler Place/List/Detail/Cancel + routes | E2, E3 |
| | F3 | payment_handler PayOS webhook + dev endpoints + routes | E2, D1, D3 |
| **G: Wire-up + job** | G1 | reservation_cleanup job + tests | C2, C4, B1 |
| | G2 | cmd/api/main.go wire order/payment/shipping/jobs + start cleanup + config | F1, F2, F3, G1 |
| **H: E2E + ship** | H1 | E2E tests (5 scenarios) | G2 |
| | H2 | gofmt, .env.example, README, commit + push | H1 |

Critical path: A1 → A3 → A4 → A5 → C1 → C2 → E2 → F2 → G2 → H1 → H2 (11 nodes).

## 14. Open Questions / Future Work (Sprint 4+)

- Brand-side endpoints: list/confirm/preparing/shipped/delivered (UC45/46/47)
- Paid-order cancellation + PayOS refund API call + brand approval flow
- Sub-order status state machine enforcement (currently free-form, Sprint 4 should formalize)
- Notification triggers (brand on new order; customer on status change) — needs notification subsystem
- Order ratings/reviews (UC37) — depends on order delivered
- Multi-currency / commission accounting (BR: 8-12%) — store at sub_order level on Sprint 4
- Variant weight & dimensions for vendor shipping integration (GHN needs `weight`, `length`, `width`, `height`)
- Webhook retry strategy & dead-letter (currently we 5xx and rely on PayOS retry)
- Wishlist → cart conversion analytics; cart abandonment recovery emails
