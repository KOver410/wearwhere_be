# Goship Shipping Spec B — Fulfillment Lifecycle — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let brands confirm and ship their sub-orders (creating a real Goship shipment), drive delivery status from the Goship webhook, auto-complete orders, settle COD on delivery, and surface tracking to customers — with a safe mock/dev path.

**Architecture:** New brand fulfillment endpoints under `/api/v1/brand/me/orders`; a `fulfillment_service` that re-quotes Goship at ship time and calls a new `goship.Shipper.CreateShipment`; a `shipping_webhook_service` mirroring the PayOS idempotent webhook (FOR UPDATE + signature verify) that maps Goship statuses to sub-order transitions, commits COD stock + marks COD paid + completes the order on delivery. Goship gains a `Shipper` interface (kept separate from the existing `Client` so existing stubs don't break).

**Tech Stack:** Go, gin, pgx v5, golang-migrate, testify-style tests.

**Spec:** `docs/superpowers/specs/2026-06-04-goship-fulfillment-design.md`
**Builds on:** Spec A (DONE) — sub-orders already carry `shipping_carrier` + locked `shipping_fee_vnd`; order `shipping_address` JSONB snapshot already includes `city_code/district_code/ward_code`; `goship.Client` has Rates/locations; mock mode wired.

**Conventions:**
- Module `github.com/wearwhere/wearwhere_be`. Commit messages: NO `Co-Authored-By` trailer.
- Migrations in `db/migrations/`, current max `000030`.
- DB (dev): `postgres://wearwhere:wearwhere@localhost:5432/wearwhere?sslmode=disable`; test: `...wearwhere_test...`. `migrate` CLI present; `make` NOT installed; docker postgres running.
- Tests: run WITHOUT `-race` (CGO off). Plain `go test ./...` works now (WDAC unblocked). Integration: `TEST_DATABASE_URL="postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable" go test -tags integration -p 1 ./...`.
- A real Goship **production** token lives in the gitignored `.env` (`GOSHIP_TOKEN`, `GOSHIP_CLIENT_SECRET`, `GOSHIP_BASE_URL=https://api.goship.io/api/v2`). **Never create a real shipment in tests.**

---

## File Structure

```
db/migrations/000031_add_fulfillment_fields_to_sub_orders.{up,down}.sql   NEW

internal/shipping/goship/
  client.go        MODIFY  add Shipper + Service interfaces; ShipmentReq/ShipmentAddress/ShipmentResp; WebhookPayload; ErrCreateShipment
  client_http.go   MODIFY  CreateShipment (POST /shipments); VerifyWebhookSignature (HMAC over raw body); clientSecret field
  client_mock.go   MODIFY  CreateShipment (fake); VerifyWebhookSignature -> nil
  factory.go       MODIFY  Config.ClientSecret; NewFromConfig returns Service; pass secret to HTTPClient
  status.go        NEW     DeliveryCategory + MapStatus(...)
  status_test.go   NEW

internal/order/domain/
  order.go         MODIFY  SubOrder: +ShippingCostVND *int64, GoshipShipmentCode *string, TrackingURL *string, ShippingStatusText *string
  dto.go           MODIFY  SubOrderResp tracking fields; BrandSubOrderListItem/Resp; ShipReq; BrandOrderListResp
  errors.go        MODIFY  ErrSubOrderNotFound, ErrNotBrandOwner, ErrInvalidTransition, ErrShipmentCreateFailed
  fulfillment.go   NEW     CanConfirm/CanShip guards
  fulfillment_test.go NEW

internal/order/repo/
  repo.go          MODIFY  extend SubOrderRepo + OrderRepo interfaces
  sub_order_pg.go  MODIFY  GetByID, GetByTrackingNoForUpdate, ListByBrand, UpdateConfirmed, UpdateShipped, UpdateDelivered, UpdateShippingStatus, AllDelivered; cols+scan for new fields
  order_pg.go      MODIFY  UpdateStatusOnComplete

internal/brand/repo/address_pg.go   MODIFY  PrimaryAddress(ctx, brandID) (*domain.BrandAddress, error)
internal/brand/repo/repo.go         MODIFY  add PrimaryAddress to interface (if interface exists)

internal/order/service/
  fulfillment_service.go        NEW  ListBrandOrders, BrandOrderDetail, Confirm, Ship
  fulfillment_service_test.go   NEW  (integration)
  shipping_webhook_service.go   NEW  HandleGoshipWebhook (idempotent)
  shipping_webhook_service_test.go NEW (integration)

internal/order/handler/
  brand_fulfillment_handler.go  NEW  List/Detail/Confirm/Ship
  shipping_webhook_handler.go   NEW  GoshipWebhook + SimulateWebhook
  routes.go                     MODIFY  MountBrand + MountShippingPublic + MountShippingDev

internal/config/config.go       MODIFY  GoshipConfig.ClientSecret
.env.example                    MODIFY  GOSHIP_CLIENT_SECRET
cmd/api/main.go                 MODIFY  wire fulfillment + webhook + brand routes; secret into factory
```

---

## Phase 1 — Schema, Domain, Config

### Task 1: Migration 000031 — fulfillment fields on sub_orders

**Files:** Create `db/migrations/000031_add_fulfillment_fields_to_sub_orders.up.sql` / `.down.sql`

- [ ] **Step 1: confirm next number** — `ls db/migrations | sort | tail -3` → highest is `000030`. Use `000031`.

- [ ] **Step 2: write `.up.sql`**
```sql
ALTER TABLE sub_orders
  ADD COLUMN shipping_cost_vnd    BIGINT CHECK (shipping_cost_vnd IS NULL OR shipping_cost_vnd >= 0),
  ADD COLUMN goship_shipment_code TEXT,
  ADD COLUMN tracking_url         TEXT,
  ADD COLUMN shipping_status_text TEXT;
```

- [ ] **Step 3: write `.down.sql`**
```sql
ALTER TABLE sub_orders
  DROP COLUMN IF EXISTS shipping_cost_vnd,
  DROP COLUMN IF EXISTS goship_shipment_code,
  DROP COLUMN IF EXISTS tracking_url,
  DROP COLUMN IF EXISTS shipping_status_text;
```

- [ ] **Step 4: apply to dev + test DB**
```
migrate -path db/migrations -database "postgres://wearwhere:wearwhere@localhost:5432/wearwhere?sslmode=disable" up
migrate -path db/migrations -database "postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable" up
```
Verify: `docker exec wearwhere_postgres psql -U wearwhere -d wearwhere -c "\d sub_orders"` shows the 4 columns. Confirm reversibility: `migrate ... down 1` then `up` on the dev DB once.

- [ ] **Step 5: commit**
```bash
git add db/migrations/000031_*
git commit -m "feat(db): fulfillment fields on sub_orders (cost, goship_shipment_code, tracking_url, status_text)"
```

---

### Task 2: SubOrder struct + repo cols/scan + config secret + .env

**Files:** Modify `internal/order/domain/order.go`, `internal/order/repo/sub_order_pg.go`, `internal/config/config.go`, `.env.example`

- [ ] **Step 1: add fields to `SubOrder`** in `order.go` (after `ShippingProvider *string`):
```go
	ShippingCostVND    *int64
	GoshipShipmentCode *string
	TrackingURL        *string
	ShippingStatusText *string
```

- [ ] **Step 2: update `subOrderCols` + both `scanSubOrder` branches** in `sub_order_pg.go`.
Change the const to include the new columns at the end of the non-join group:
```go
const subOrderCols = `id, order_id, brand_id, subtotal_vnd, shipping_fee_vnd, total_vnd,
                      status, tracking_no, shipping_carrier, shipping_provider,
                      confirmed_at, shipped_at, delivered_at, cancelled_at,
                      shipping_cost_vnd, goship_shipment_code, tracking_url, shipping_status_text,
                      created_at, updated_at`
```
In BOTH branches of `scanSubOrder`, add the four destinations in the SAME position (right before `&s.CreatedAt, &s.UpdatedAt`):
```go
		&s.ShippingCostVND, &s.GoshipShipmentCode, &s.TrackingURL, &s.ShippingStatusText,
```
(The brand-join branch then ends with `&s.CreatedAt, &s.UpdatedAt, &s.BrandSlug, &s.BrandName`.)
Update the `ListByOrderID` inline SELECT to add `s.shipping_cost_vnd, s.goship_shipment_code, s.tracking_url, s.shipping_status_text` in the same position (before `s.created_at, s.updated_at`).

- [ ] **Step 3: add `ClientSecret` to `GoshipConfig`** in `config.go` (struct + loader):
```go
// in GoshipConfig struct, after Token:
	ClientSecret string
// in the cfg.Goship = GoshipConfig{...} block, after Token:
		ClientSecret: getEnv("GOSHIP_CLIENT_SECRET", ""),
```

- [ ] **Step 4: `.env.example`** — add under the Goship section:
```bash
GOSHIP_CLIENT_SECRET=                # required to verify Goship status webhooks (production)
```

- [ ] **Step 5: build**
Run: `go build ./...`  → success (no behavior change yet).

- [ ] **Step 6: commit**
```bash
git add internal/order/domain/order.go internal/order/repo/sub_order_pg.go internal/config .env.example
git commit -m "feat(order): SubOrder fulfillment fields wired through repo; GoshipConfig.ClientSecret"
```

---

## Phase 2 — Goship Shipper Client + Status Mapping

### Task 3: Goship Shipper interface + CreateShipment + webhook verify (mock + http + factory)

**Files:** Modify `internal/shipping/goship/client.go`, `client_http.go`, `client_mock.go`, `factory.go`

> **Interface segregation (important):** Do NOT add methods to the existing `Client` interface — `provider` and `location` tests define stubs that implement `Client`, and adding methods would break them. Add a separate `Shipper` interface and a `Service` that combines both.

- [ ] **Step 1: add DTOs + interfaces to `client.go`** (append):
```go
var ErrCreateShipment = errors.New("goship: failed to create shipment")

// ShipmentAddress is a sender/recipient for shipment creation.
type ShipmentAddress struct {
	Name         string
	Phone        string
	Street       string
	WardCode     string
	DistrictCode string
	CityCode     string
}

type ShipmentReq struct {
	RateID   string          // fresh rate id from a re-quote at ship time
	From     ShipmentAddress // brand pickup
	To       ShipmentAddress // recipient (order snapshot)
	Parcel   Parcel          // weight/dims + cod/amount
	OrderRef string          // our sub_order id; echoed back by the webhook as order_id
}

type ShipmentResp struct {
	TrackingCode string // Goship "code"
	GoshipCode   string // Goship "gcode"
	LabelURL     string
	FeeVND       int64
}

// WebhookPayload is the Goship shipment-status callback.
type WebhookPayload struct {
	GCode        string `json:"gcode"`
	Code         string `json:"code"`
	OrderID      string `json:"order_id"`
	Status       string `json:"status"`
	StatusText   string `json:"status_text"`
	Message      string `json:"message"`
	TrackingURL  string `json:"tracking_url"`
	IsReturn     int    `json:"is_return"`
	IsLost       int    `json:"is_lost"`
	CarrierShort string `json:"carrier_short_name"`
	UpdateTime   int64  `json:"update_time"`
}

// Shipper covers shipment creation + webhook verification (separate from Client
// so existing Client stubs in other packages keep compiling).
type Shipper interface {
	CreateShipment(ctx context.Context, r ShipmentReq) (*ShipmentResp, error)
	VerifyWebhookSignature(rawBody []byte, signature string) error
}

// Service is the full Goship capability (rates/locations + shipment/webhook).
type Service interface {
	Client
	Shipper
}
```

- [ ] **Step 2: extend the mock** in `client_mock.go` (append methods; add `sync/atomic` import + a counter field on MockClient):
Change `type MockClient struct{}` to `type MockClient struct{ seq atomic.Int64 }` and add:
```go
func (m *MockClient) CreateShipment(_ context.Context, r ShipmentReq) (*ShipmentResp, error) {
	n := m.seq.Add(1)
	fee := r.Parcel.AmountVND // any deterministic value; tests assert via stored cost
	if fee == 0 {
		fee = 20000
	}
	return &ShipmentResp{
		TrackingCode: fmt.Sprintf("MOCK-TRK-%d", n),
		GoshipCode:   fmt.Sprintf("MOCK-GS-%d", n),
		LabelURL:     "https://mock.goship.local/label",
		FeeVND:       fee,
	}, nil
}

// Mock accepts any signature.
func (m *MockClient) VerifyWebhookSignature(_ []byte, _ string) error { return nil }
```
Add `"sync/atomic"` to the mock's imports.

- [ ] **Step 3: extend the HTTP client** in `client_http.go`. Add a `clientSecret` field to `HTTPClient` and a setter via the constructor. Change `NewHTTPClient` to accept the secret:
```go
type HTTPClient struct {
	token        string
	clientSecret string
	baseURL      string
	httpClient   *http.Client
}

func NewHTTPClient(token, clientSecret, baseURL string) *HTTPClient {
	return &HTTPClient{
		token:        token,
		clientSecret: clientSecret,
		baseURL:      baseURL,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}
```
Add the methods:
```go
func (c *HTTPClient) CreateShipment(ctx context.Context, r ShipmentReq) (*ShipmentResp, error) {
	body := map[string]any{
		"shipment": map[string]any{
			"rate": r.RateID,
			"address_from": map[string]any{
				"name": r.From.Name, "phone": r.From.Phone, "street": r.From.Street,
				"ward": r.From.WardCode, "district": r.From.DistrictCode, "city": r.From.CityCode,
			},
			"address_to": map[string]any{
				"name": r.To.Name, "phone": r.To.Phone, "street": r.To.Street,
				"ward": r.To.WardCode, "district": r.To.DistrictCode, "city": r.To.CityCode,
			},
			"parcel": map[string]any{
				"cod": r.Parcel.CODVND, "amount": r.Parcel.AmountVND,
				"weight": r.Parcel.WeightG, "length": r.Parcel.LengthCM,
				"width": r.Parcel.WidthCM, "height": r.Parcel.HeightCM,
			},
			"order_id": r.OrderRef,
		},
	}
	var env struct {
		Data struct {
			Code     string `json:"code"`
			GCode    string `json:"gcode"`
			Label    string `json:"label"`
			TotalFee int64  `json:"total_fee"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodPost, "/shipments", body, &env); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCreateShipment, err)
	}
	return &ShipmentResp{
		TrackingCode: env.Data.Code,
		GoshipCode:   env.Data.GCode,
		LabelURL:     env.Data.Label,
		FeeVND:       env.Data.TotalFee,
	}, nil
}

func (c *HTTPClient) VerifyWebhookSignature(rawBody []byte, signature string) error {
	mac := hmac.New(sha256.New, []byte(c.clientSecret))
	mac.Write(rawBody)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return ErrSignatureInvalid
	}
	return nil
}
```
Add `ErrSignatureInvalid` to `client.go` errors var block:
```go
	ErrSignatureInvalid = errors.New("goship: invalid webhook signature")
```
Add imports to `client_http.go`: `"crypto/hmac"`, `"crypto/sha256"`, `"encoding/base64"`.
> The `/shipments` request/response field names are the assumed shape; Task 13 confirms against the live API. Keep them isolated here so only this file changes if they differ.

- [ ] **Step 4: factory** in `factory.go` — add `ClientSecret` to `Config`, change return type to `Service`, pass the secret:
```go
type Config struct {
	Mode         string
	Token        string
	ClientSecret string
	BaseURL      string
}

func NewFromConfig(cfg Config) (Service, error) {
	switch cfg.Mode {
	case "mock", "":
		return NewMockClient(), nil
	case "sandbox", "production":
		if cfg.Token == "" {
			return nil, fmt.Errorf("goship: %s mode requires GOSHIP_TOKEN", cfg.Mode)
		}
		return NewHTTPClient(cfg.Token, cfg.ClientSecret, cfg.BaseURL), nil
	default:
		return nil, fmt.Errorf("goship: unknown mode %q (want mock|sandbox|production)", cfg.Mode)
	}
}
```
> `NewFromConfig` now returns `Service` (superset of `Client`). The existing callers in `cmd/api/main.go` (`location.NewService(goshipClient)`, `provider.GoshipDeps{Client: goshipClient}`) accept a `Client` and still compile because `Service` satisfies `Client`. The `NewHTTPClient` signature changed (added `clientSecret`) — its only caller is the factory; the `goship_real` test calls `NewHTTPClient(tok, base)` and MUST be updated to `NewHTTPClient(tok, "", base)`.

- [ ] **Step 5: fix the `goship_real` test constructor call** in `client_http_real_test.go`: change `NewHTTPClient(tok, base)` → `NewHTTPClient(tok, "", base)`.

- [ ] **Step 6: build + run goship unit tests**
Run: `go build ./...` then `go test ./internal/shipping/...`
Expected: PASS (existing mock/provider/location/weight tests still green; new methods compile).

- [ ] **Step 7: commit**
```bash
git add internal/shipping/goship/
git commit -m "feat(goship): Shipper interface (CreateShipment + webhook HMAC verify); mock + http + factory"
```

---

### Task 4: Goship status mapping

**Files:** Create `internal/shipping/goship/status.go`, `status_test.go`

- [ ] **Step 1: write the failing test** `status_test.go`:
```go
package goship

import "testing"

func TestMapStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		text     string
		isReturn int
		isLost   int
		want     DeliveryCategory
	}{
		{"waiting pickup 901", "901", "Chờ lấy hàng", 0, 0, CategoryShipped},
		{"delivered text", "", "Đã giao hàng", 0, 0, CategoryDelivered},
		{"returned flag", "", "Hoàn hàng", 1, 0, CategoryOther},
		{"lost flag", "", "Thất lạc", 0, 1, CategoryOther},
		{"unknown -> shipped", "555", "Đang vận chuyển", 0, 0, CategoryShipped},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MapStatus(tc.status, tc.text, tc.isReturn, tc.isLost); got != tc.want {
				t.Errorf("MapStatus(%q,%q,%d,%d) = %v, want %v", tc.status, tc.text, tc.isReturn, tc.isLost, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: run → FAIL** (`MapStatus`/`DeliveryCategory` undefined): `go test ./internal/shipping/goship/ -run TestMapStatus`

- [ ] **Step 3: write `status.go`**
```go
package goship

import "strings"

// DeliveryCategory is the coarse outcome a webhook maps to.
type DeliveryCategory int

const (
	CategoryShipped   DeliveryCategory = iota // in-transit / picked up / waiting pickup
	CategoryDelivered                         // successfully delivered
	CategoryOther                             // return / lost / unknown-terminal — record text only
)

// deliveredHints / returnHints are matched case-insensitively against status_text
// as a fallback when numeric codes aren't yet catalogued.
var deliveredHints = []string{"đã giao", "giao thành công", "delivered", "thành công"}

// MapStatus maps a Goship webhook status to a coarse category.
// Returned/lost shipments are CategoryOther (recorded, not auto-restocked — out of Spec B scope).
// Confirmed code "901" = Chờ lấy hàng (waiting pickup) -> Shipped. Extend codes here as observed.
func MapStatus(status, statusText string, isReturn, isLost int) DeliveryCategory {
	if isReturn == 1 || isLost == 1 {
		return CategoryOther
	}
	t := strings.ToLower(statusText)
	for _, h := range deliveredHints {
		if strings.Contains(t, h) {
			return CategoryDelivered
		}
	}
	// Everything else that isn't a return/lost is treated as in-progress (shipped).
	return CategoryShipped
}
```

- [ ] **Step 4: run → PASS**: `go test ./internal/shipping/goship/ -run TestMapStatus -v`

- [ ] **Step 5: commit**
```bash
git add internal/shipping/goship/status.go internal/shipping/goship/status_test.go
git commit -m "feat(goship): delivery status -> coarse category mapping"
```

---

## Phase 3 — Repos

### Task 5: sub-order + order repo methods

**Files:** Modify `internal/order/repo/repo.go`, `internal/order/repo/sub_order_pg.go`, `internal/order/repo/order_pg.go`, `internal/brand/repo/address_pg.go` (+ its interface)

- [ ] **Step 1: extend interfaces in `repo.go`**
Add to `SubOrderRepo`:
```go
	GetByID(ctx context.Context, id uuid.UUID) (*domain.SubOrder, error)
	GetByTrackingNoForUpdate(ctx context.Context, db DBTX, trackingNo string) (*domain.SubOrder, error)
	ListByBrand(ctx context.Context, brandID uuid.UUID, statuses []domain.SubOrderStatus, page, pageSize int) (items []*domain.SubOrder, total int, err error)
	UpdateConfirmed(ctx context.Context, db DBTX, id uuid.UUID) error
	UpdateShipped(ctx context.Context, db DBTX, id uuid.UUID, trackingNo, goshipCode, carrier string, costVND int64, trackingURL string) error
	UpdateDelivered(ctx context.Context, db DBTX, id uuid.UUID, statusText, trackingURL string) error
	UpdateShippingStatus(ctx context.Context, db DBTX, id uuid.UUID, statusText, trackingURL string) error
	AllDelivered(ctx context.Context, db DBTX, orderID uuid.UUID) (bool, error)
```
Add to `OrderRepo`:
```go
	UpdateStatusOnComplete(ctx context.Context, db DBTX, orderID uuid.UUID) error
```

- [ ] **Step 2: implement in `sub_order_pg.go`**
```go
func (r *SubOrderPG) GetByID(ctx context.Context, id uuid.UUID) (*domain.SubOrder, error) {
	row := r.db.QueryRow(ctx,
		`SELECT s.id, s.order_id, s.brand_id, s.subtotal_vnd, s.shipping_fee_vnd, s.total_vnd,
		        s.status, s.tracking_no, s.shipping_carrier, s.shipping_provider,
		        s.confirmed_at, s.shipped_at, s.delivered_at, s.cancelled_at,
		        s.shipping_cost_vnd, s.goship_shipment_code, s.tracking_url, s.shipping_status_text,
		        s.created_at, s.updated_at, b.slug, b.name
		   FROM sub_orders s JOIN brands b ON b.id = s.brand_id
		  WHERE s.id = $1`, id)
	return scanSubOrder(row, true)
}

func (r *SubOrderPG) GetByTrackingNoForUpdate(ctx context.Context, db DBTX, trackingNo string) (*domain.SubOrder, error) {
	if db == nil {
		db = r.db
	}
	row := db.QueryRow(ctx,
		`SELECT `+subOrderCols+` FROM sub_orders WHERE tracking_no = $1 FOR UPDATE`, trackingNo)
	return scanSubOrder(row, false)
}

func (r *SubOrderPG) ListByBrand(ctx context.Context, brandID uuid.UUID, statuses []domain.SubOrderStatus, page, pageSize int) ([]*domain.SubOrder, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	args := []any{brandID}
	where := "s.brand_id = $1"
	if len(statuses) > 0 {
		ss := make([]string, len(statuses))
		for i, st := range statuses {
			ss[i] = string(st)
		}
		args = append(args, ss)
		where += " AND s.status = ANY($2)"
	}
	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM sub_orders s WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.Query(ctx,
		`SELECT s.id, s.order_id, s.brand_id, s.subtotal_vnd, s.shipping_fee_vnd, s.total_vnd,
		        s.status, s.tracking_no, s.shipping_carrier, s.shipping_provider,
		        s.confirmed_at, s.shipped_at, s.delivered_at, s.cancelled_at,
		        s.shipping_cost_vnd, s.goship_shipment_code, s.tracking_url, s.shipping_status_text,
		        s.created_at, s.updated_at, b.slug, b.name
		   FROM sub_orders s JOIN brands b ON b.id = s.brand_id
		  WHERE `+where+`
		  ORDER BY s.created_at DESC
		  LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.SubOrder
	for rows.Next() {
		so, err := scanSubOrder(rows, true)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, so)
	}
	return out, total, rows.Err()
}

func (r *SubOrderPG) UpdateConfirmed(ctx context.Context, db DBTX, id uuid.UUID) error {
	if db == nil {
		db = r.db
	}
	_, err := db.Exec(ctx,
		`UPDATE sub_orders SET status='confirmed', confirmed_at=NOW(), updated_at=NOW()
		  WHERE id=$1 AND status='pending'`, id)
	return err
}

func (r *SubOrderPG) UpdateShipped(ctx context.Context, db DBTX, id uuid.UUID, trackingNo, goshipCode, carrier string, costVND int64, trackingURL string) error {
	if db == nil {
		db = r.db
	}
	_, err := db.Exec(ctx,
		`UPDATE sub_orders
		    SET status='shipped', shipped_at=NOW(), updated_at=NOW(),
		        tracking_no=$2, goship_shipment_code=$3, shipping_carrier=$4,
		        shipping_cost_vnd=$5, tracking_url=$6
		  WHERE id=$1 AND status='confirmed'`,
		id, trackingNo, goshipCode, carrier, costVND, trackingURL)
	return err
}

func (r *SubOrderPG) UpdateDelivered(ctx context.Context, db DBTX, id uuid.UUID, statusText, trackingURL string) error {
	if db == nil {
		db = r.db
	}
	_, err := db.Exec(ctx,
		`UPDATE sub_orders
		    SET status='delivered', delivered_at=NOW(), updated_at=NOW(),
		        shipping_status_text=$2, tracking_url=COALESCE(NULLIF($3,''), tracking_url)
		  WHERE id=$1 AND status <> 'delivered'`,
		id, statusText, trackingURL)
	return err
}

func (r *SubOrderPG) UpdateShippingStatus(ctx context.Context, db DBTX, id uuid.UUID, statusText, trackingURL string) error {
	if db == nil {
		db = r.db
	}
	_, err := db.Exec(ctx,
		`UPDATE sub_orders
		    SET shipping_status_text=$2, tracking_url=COALESCE(NULLIF($3,''), tracking_url),
		        status=CASE WHEN status='confirmed' THEN 'shipped' ELSE status END,
		        shipped_at=CASE WHEN status='confirmed' AND shipped_at IS NULL THEN NOW() ELSE shipped_at END,
		        updated_at=NOW()
		  WHERE id=$1 AND status NOT IN ('delivered','cancelled')`,
		id, statusText, trackingURL)
	return err
}

func (r *SubOrderPG) AllDelivered(ctx context.Context, db DBTX, orderID uuid.UUID) (bool, error) {
	if db == nil {
		db = r.db
	}
	var notDelivered int
	err := db.QueryRow(ctx,
		`SELECT COUNT(*) FROM sub_orders WHERE order_id=$1 AND status <> 'delivered'`, orderID).Scan(&notDelivered)
	if err != nil {
		return false, err
	}
	return notDelivered == 0, nil
}
```
Add a tiny helper at the bottom of the file (used for dynamic placeholder indices):
```go
func itoa(n int) string { return strconv.Itoa(n) }
```
Add `"strconv"` to imports.

- [ ] **Step 3: implement `UpdateStatusOnComplete` in `order_pg.go`** (mirror `UpdateStatusOnPaid`'s style):
```go
func (r *OrderPG) UpdateStatusOnComplete(ctx context.Context, db DBTX, orderID uuid.UUID) error {
	if db == nil {
		db = r.db
	}
	_, err := db.Exec(ctx,
		`UPDATE orders SET status='completed', updated_at=NOW()
		  WHERE id=$1 AND status='processing'`, orderID)
	return err
}
```
> Match the actual `OrderPG` receiver/field name (`r.db`/pool) by looking at `UpdateStatusOnPaid` in the same file.

- [ ] **Step 4: add `PrimaryAddress` to the brand address repo** `internal/brand/repo/address_pg.go` (full primary address, mirrors `PrimaryAddressCodes`):
```go
func (r *AddressPG) PrimaryAddress(ctx context.Context, brandID uuid.UUID) (*domain.BrandAddress, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+addrCols+` FROM brand_addresses
		  WHERE brand_id=$1 AND is_primary=TRUE AND deleted_at IS NULL
		  ORDER BY created_at ASC LIMIT 1`, brandID)
	return scanAddress(row)
}
```
> Use the file's actual `addrCols` const + `scanAddress` helper + receiver field (`r.db`). If `scanAddress` needs a `pgx.Row`, `QueryRow` returns one. If brand addresses are accessed via an interface, add `PrimaryAddress` to it.

- [ ] **Step 5: build**
Run: `go build ./...` → success.

- [ ] **Step 6: commit**
```bash
git add internal/order/repo internal/brand/repo
git commit -m "feat(order): sub-order fulfillment repo methods + order complete + brand PrimaryAddress"
```

---

## Phase 4 — Domain guards + Fulfillment service

### Task 6: transition guards + errors

**Files:** Create `internal/order/domain/fulfillment.go`, `fulfillment_test.go`; modify `internal/order/domain/errors.go`

- [ ] **Step 1: add errors to `errors.go`** (in the var block; add `errors` import if needed):
```go
	ErrSubOrderNotFound    = errors.New("sub-order not found")
	ErrNotBrandOwner       = errors.New("sub-order does not belong to this brand")
	ErrInvalidTransition   = errors.New("invalid fulfillment transition")
	ErrShipmentCreateFailed = errors.New("failed to create shipment with carrier")
```

- [ ] **Step 2: write the failing test** `fulfillment_test.go`:
```go
package domain

import "testing"

func TestCanConfirm(t *testing.T) {
	if !CanConfirm(SubOrderStatusPending) {
		t.Error("pending should be confirmable")
	}
	if CanConfirm(SubOrderStatusConfirmed) {
		t.Error("confirmed should NOT be re-confirmable")
	}
}

func TestCanShip(t *testing.T) {
	// ship requires sub-order confirmed AND parent order processing
	if !CanShip(SubOrderStatusConfirmed, OrderStatusProcessing) {
		t.Error("confirmed + processing should be shippable")
	}
	if CanShip(SubOrderStatusPending, OrderStatusProcessing) {
		t.Error("pending should not be shippable")
	}
	if CanShip(SubOrderStatusConfirmed, OrderStatusPendingPayment) {
		t.Error("unpaid (pending_payment) order should not be shippable")
	}
}
```

- [ ] **Step 3: run → FAIL**: `go test ./internal/order/domain/ -run "TestCanConfirm|TestCanShip"`

- [ ] **Step 4: write `fulfillment.go`**
```go
package domain

// CanConfirm reports whether a sub-order in the given status may be confirmed.
func CanConfirm(s SubOrderStatus) bool {
	return s == SubOrderStatusPending
}

// CanShip reports whether a confirmed sub-order may be shipped. The parent order
// must be in 'processing' (PayOS paid, or COD which is processing from placement).
func CanShip(s SubOrderStatus, orderStatus OrderStatus) bool {
	return s == SubOrderStatusConfirmed && orderStatus == OrderStatusProcessing
}
```

- [ ] **Step 5: run → PASS**: `go test ./internal/order/domain/ -run "TestCanConfirm|TestCanShip" -v`

- [ ] **Step 6: commit**
```bash
git add internal/order/domain/fulfillment.go internal/order/domain/fulfillment_test.go internal/order/domain/errors.go
git commit -m "feat(order): fulfillment transition guards + errors"
```

---

### Task 7: DTOs for brand fulfillment + customer tracking

**Files:** Modify `internal/order/domain/dto.go`

- [ ] **Step 1: add tracking fields to `SubOrderResp`** (after `TrackingNo`):
```go
	ShippingCarrier    *string `json:"shipping_carrier"`
	TrackingURL        *string `json:"tracking_url"`
	ShippingStatusText *string `json:"shipping_status_text"`
```

- [ ] **Step 2: add brand DTOs + ship request**
```go
type ShipReq struct {
	Carrier string `json:"carrier"` // optional override; empty = use stored shipping_carrier
}

type BrandSubOrderListItem struct {
	SubOrderID   uuid.UUID      `json:"sub_order_id"`
	OrderNo      string         `json:"order_no"`
	Status       SubOrderStatus `json:"status"`
	Recipient    string         `json:"recipient"`
	TotalVND     int64          `json:"total_vnd"`
	ItemCount    int            `json:"item_count"`
	TrackingNo   *string        `json:"tracking_no"`
	CreatedAt    time.Time      `json:"created_at"`
}

type BrandSubOrderListResp struct {
	Data       []BrandSubOrderListItem `json:"data"`
	Page       int                     `json:"page"`
	PageSize   int                     `json:"page_size"`
	Total      int                     `json:"total"`
	TotalPages int                     `json:"total_pages"`
}

type BrandSubOrderDetailResp struct {
	SubOrderID         uuid.UUID       `json:"sub_order_id"`
	OrderNo            string          `json:"order_no"`
	Status             SubOrderStatus  `json:"status"`
	SubtotalVND        int64           `json:"subtotal_vnd"`
	ShippingFeeVND     int64           `json:"shipping_fee_vnd"`
	TotalVND           int64           `json:"total_vnd"`
	ShippingCarrier    *string         `json:"shipping_carrier"`
	TrackingNo         *string         `json:"tracking_no"`
	TrackingURL        *string         `json:"tracking_url"`
	ShippingStatusText *string         `json:"shipping_status_text"`
	ShippingAddress    ShippingAddress `json:"shipping_address"`
	Items              []OrderItemResp `json:"items"`
	CreatedAt          time.Time       `json:"created_at"`
}
```
(Imports `time` + `uuid` already present in dto.go.)

- [ ] **Step 3: ensure the customer `DetailOrder` maps the new SubOrderResp fields.** Find where `SubOrderResp` is built (in `internal/order/service` Detail mapping or handler) and set `ShippingCarrier`, `TrackingURL`, `ShippingStatusText` from the sub-order. (Search: `grep -rn "SubOrderResp{" internal/order`.)

- [ ] **Step 4: build**
Run: `go build ./...` → success.

- [ ] **Step 5: commit**
```bash
git add internal/order/domain/dto.go internal/order/service internal/order/handler
git commit -m "feat(order): brand fulfillment DTOs + customer tracking fields in order detail"
```

---

### Task 8: Fulfillment service — List, Detail, Confirm, Ship

**Files:** Create `internal/order/service/fulfillment_service.go`

- [ ] **Step 1: write the service** `fulfillment_service.go`:
```go
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	branddomain "github.com/wearwhere/wearwhere_be/internal/brand/domain"
	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
	"github.com/wearwhere/wearwhere_be/internal/shipping/weight"
)

// brandPickupRepo returns a brand's full primary pickup address.
type brandPickupRepo interface {
	PrimaryAddress(ctx context.Context, brandID uuid.UUID) (*branddomain.BrandAddress, error)
}

type FulfillmentService struct {
	pool       *pgxpool.Pool
	orderRepo  orderrepo.OrderRepo
	subOrder   orderrepo.SubOrderRepo
	items      orderrepo.OrderItemRepo
	goship     goship.Service
	brandAddr  brandPickupRepo
	defaults   weight.Defaults
}

func NewFulfillmentService(
	pool *pgxpool.Pool, or orderrepo.OrderRepo, sr orderrepo.SubOrderRepo,
	ir orderrepo.OrderItemRepo, gs goship.Service, ba brandPickupRepo, d weight.Defaults,
) *FulfillmentService {
	return &FulfillmentService{pool: pool, orderRepo: or, subOrder: sr, items: ir, goship: gs, brandAddr: ba, defaults: d}
}

// loadOwned fetches a sub-order and asserts brand ownership.
func (s *FulfillmentService) loadOwned(ctx context.Context, brandID, subOrderID uuid.UUID) (*domain.SubOrder, error) {
	so, err := s.subOrder.GetByID(ctx, subOrderID)
	if err != nil {
		if errors.Is(err, orderrepo.ErrNotFound) {
			return nil, domain.ErrSubOrderNotFound
		}
		return nil, err
	}
	if so.BrandID != brandID {
		return nil, domain.ErrNotBrandOwner
	}
	return so, nil
}

func (s *FulfillmentService) List(ctx context.Context, brandID uuid.UUID, statuses []domain.SubOrderStatus, page, pageSize int) (*domain.BrandSubOrderListResp, error) {
	rows, total, err := s.subOrder.ListByBrand(ctx, brandID, statuses, page, pageSize)
	if err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	out := make([]domain.BrandSubOrderListItem, 0, len(rows))
	for _, so := range rows {
		ord, err := s.orderRepo.GetByID(ctx, so.OrderID)
		if err != nil {
			return nil, err
		}
		its, err := s.items.ListBySubOrderID(ctx, so.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.BrandSubOrderListItem{
			SubOrderID: so.ID, OrderNo: ord.OrderNo, Status: so.Status,
			Recipient: ord.ShippingAddress.Recipient, TotalVND: so.TotalVND,
			ItemCount: len(its), TrackingNo: so.TrackingNo, CreatedAt: so.CreatedAt,
		})
	}
	totalPages := (total + pageSize - 1) / pageSize
	return &domain.BrandSubOrderListResp{Data: out, Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages}, nil
}

func (s *FulfillmentService) Detail(ctx context.Context, brandID, subOrderID uuid.UUID) (*domain.BrandSubOrderDetailResp, error) {
	so, err := s.loadOwned(ctx, brandID, subOrderID)
	if err != nil {
		return nil, err
	}
	ord, err := s.orderRepo.GetByID(ctx, so.OrderID)
	if err != nil {
		return nil, err
	}
	its, err := s.items.ListBySubOrderID(ctx, so.ID)
	if err != nil {
		return nil, err
	}
	itemResps := make([]domain.OrderItemResp, 0, len(its))
	for _, it := range its {
		itemResps = append(itemResps, domain.OrderItemResp{
			ID: it.ID, VariantID: it.VariantID, ProductID: it.ProductID,
			ProductName: it.ProductName, VariantLabel: it.VariantLabel, ImageURL: it.ImageURL,
			Qty: it.Qty, UnitPriceVND: it.UnitPriceVND, LineTotalVND: it.LineTotalVND,
		})
	}
	return &domain.BrandSubOrderDetailResp{
		SubOrderID: so.ID, OrderNo: ord.OrderNo, Status: so.Status,
		SubtotalVND: so.SubtotalVND, ShippingFeeVND: so.ShippingFeeVND, TotalVND: so.TotalVND,
		ShippingCarrier: so.ShippingCarrier, TrackingNo: so.TrackingNo, TrackingURL: so.TrackingURL,
		ShippingStatusText: so.ShippingStatusText, ShippingAddress: ord.ShippingAddress,
		Items: itemResps, CreatedAt: so.CreatedAt,
	}, nil
}

func (s *FulfillmentService) Confirm(ctx context.Context, brandID, subOrderID uuid.UUID) error {
	so, err := s.loadOwned(ctx, brandID, subOrderID)
	if err != nil {
		return err
	}
	if !domain.CanConfirm(so.Status) {
		return domain.ErrInvalidTransition
	}
	return s.subOrder.UpdateConfirmed(ctx, nil, so.ID)
}

// Ship re-quotes Goship by the chosen carrier, creates the shipment, and persists tracking.
func (s *FulfillmentService) Ship(ctx context.Context, brandID, subOrderID uuid.UUID, carrierOverride string) error {
	so, err := s.loadOwned(ctx, brandID, subOrderID)
	if err != nil {
		return err
	}
	ord, err := s.orderRepo.GetByID(ctx, so.OrderID)
	if err != nil {
		return err
	}
	if !domain.CanShip(so.Status, ord.Status) {
		return domain.ErrInvalidTransition
	}
	to := ord.ShippingAddress
	if to.CityCode == nil || to.DistrictCode == nil {
		return domain.ErrAddressIncomplete
	}
	from, err := s.brandAddr.PrimaryAddress(ctx, brandID)
	if err != nil || from == nil || from.CityCode == nil || from.DistrictCode == nil {
		return fmt.Errorf("%w: brand pickup address incomplete", domain.ErrShipmentCreateFailed)
	}

	its, err := s.items.ListBySubOrderID(ctx, so.ID)
	if err != nil {
		return err
	}
	wItems := make([]weight.Item, 0, len(its))
	for _, it := range its {
		wItems = append(wItems, weight.Item{Qty: it.Qty}) // dims default; variant dims not joined here
	}
	parcel := weight.Aggregate(wItems, s.defaults)

	carrier := derefStr(so.ShippingCarrier) // free func defined in Step 2
	if carrierOverride != "" {
		carrier = carrierOverride
	}
	var cod int64
	if ord.PaymentMethod == domain.PaymentMethodCOD {
		cod = so.SubtotalVND + so.ShippingFeeVND
	}

	// Re-quote to obtain a fresh rate_id for the chosen carrier.
	rates, err := s.goship.Rates(ctx, goship.RateReq{
		From:   goship.Address{CityCode: *from.CityCode, DistrictCode: *from.DistrictCode},
		To:     goship.Address{CityCode: *to.CityCode, DistrictCode: *to.DistrictCode},
		Parcel: goship.Parcel{WeightG: parcel.WeightG, LengthCM: parcel.LengthCM, WidthCM: parcel.WidthCM, HeightCM: parcel.HeightCM, CODVND: cod, AmountVND: so.SubtotalVND},
	})
	if err != nil {
		return fmt.Errorf("%w: re-quote: %v", domain.ErrShipmentCreateFailed, err)
	}
	var rate *goship.Rate
	for i := range rates {
		if rates[i].Carrier == carrier {
			rate = &rates[i]
			break
		}
	}
	if rate == nil {
		return domain.ErrCarrierUnavailable
	}

	resp, err := s.goship.CreateShipment(ctx, goship.ShipmentReq{
		RateID: rate.ID,
		From:   shipAddrFromBrand(from),
		To:     shipAddrFromSnapshot(to),
		Parcel: goship.Parcel{WeightG: parcel.WeightG, LengthCM: parcel.LengthCM, WidthCM: parcel.WidthCM, HeightCM: parcel.HeightCM, CODVND: cod, AmountVND: so.SubtotalVND},
		OrderRef: so.ID.String(),
	})
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrShipmentCreateFailed, err)
	}
	return s.subOrder.UpdateShipped(ctx, nil, so.ID, resp.TrackingCode, resp.GoshipCode, rate.Carrier, resp.FeeVND, resp.LabelURL)
}
```
(The `derefStr` / `shipAddrFromBrand` / `shipAddrFromSnapshot` helpers used above are added in Step 2.)

- [ ] **Step 2: add the free helpers** (place at the bottom of `fulfillment_service.go`):
```go
func derefStr(p *string) string {
	if p != nil {
		return *p
	}
	return ""
}

func shipAddrFromBrand(a *branddomain.BrandAddress) goship.ShipmentAddress {
	return goship.ShipmentAddress{
		Name: a.Label, Phone: derefStr(a.Phone), Street: a.AddressLine,
		WardCode: derefStr(a.WardCode), DistrictCode: derefStr(a.DistrictCode), CityCode: derefStr(a.CityCode),
	}
}

func shipAddrFromSnapshot(a domain.ShippingAddress) goship.ShipmentAddress {
	return goship.ShipmentAddress{
		Name: a.Recipient, Phone: a.Phone, Street: a.Line1,
		WardCode: derefStr(a.WardCode), DistrictCode: derefStr(a.DistrictCode), CityCode: derefStr(a.CityCode),
	}
}
```
> Verify `branddomain.BrandAddress` field names (`Label`, `AddressLine`, `Phone *string`, `WardCode/DistrictCode/CityCode *string`) against `internal/brand/domain/brand.go` — adjust if the brand uses a different field for the sender name (e.g. a brand name from the brand entity). Using `Label` is acceptable; if a real contact name is required, thread the brand name in.

- [ ] **Step 3: build**
Run: `go build ./...` → success.

- [ ] **Step 4: commit** (service is integration-tested in Task 11)
```bash
git add internal/order/service/fulfillment_service.go
git commit -m "feat(order): fulfillment service (list/detail/confirm/ship via Goship)"
```

---

## Phase 5 — Webhook service + handlers + routes

### Task 9: Goship webhook service (idempotent)

**Files:** Create `internal/order/service/shipping_webhook_service.go`

- [ ] **Step 1: write the service** (mirrors `payment/service/webhook_service.go`):
```go
package service

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	paymentdomain "github.com/wearwhere/wearwhere_be/internal/payment/domain"
	paymentrepo "github.com/wearwhere/wearwhere_be/internal/payment/repo"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
)

type ShippingWebhookService struct {
	pool      *pgxpool.Pool
	subOrder  orderrepo.SubOrderRepo
	orderRepo orderrepo.OrderRepo
	items     orderrepo.OrderItemRepo
	payment   paymentrepo.PaymentRepo
	variant   productrepo.VariantRepo
}

func NewShippingWebhookService(
	pool *pgxpool.Pool, sr orderrepo.SubOrderRepo, or orderrepo.OrderRepo,
	ir orderrepo.OrderItemRepo, pr paymentrepo.PaymentRepo, vr productrepo.VariantRepo,
) *ShippingWebhookService {
	return &ShippingWebhookService{pool: pool, subOrder: sr, orderRepo: or, items: ir, payment: pr, variant: vr}
}

// HandleGoshipWebhook is idempotent. Signature must be verified by the caller.
func (s *ShippingWebhookService) HandleGoshipWebhook(ctx context.Context, p goship.WebhookPayload) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	so, err := s.subOrder.GetByTrackingNoForUpdate(ctx, tx, p.Code)
	if err != nil {
		if errors.Is(err, orderrepo.ErrNotFound) {
			return tx.Commit(ctx) // unknown tracking — tolerate (200)
		}
		return err
	}

	switch goship.MapStatus(p.Status, p.StatusText, p.IsReturn, p.IsLost) {
	case goship.CategoryDelivered:
		if so.Status == domain.SubOrderStatusDelivered {
			return tx.Commit(ctx) // idempotent
		}
		if err := s.subOrder.UpdateDelivered(ctx, tx, so.ID, p.StatusText, p.TrackingURL); err != nil {
			return err
		}
		ord, err := s.orderRepo.GetByID(ctx, so.OrderID)
		if err != nil {
			return err
		}
		// COD: commit this sub-order's reserved stock on delivery.
		if ord.PaymentMethod == domain.PaymentMethodCOD {
			its, err := s.items.ListBySubOrderID(ctx, so.ID)
			if err != nil {
				return err
			}
			for _, it := range its {
				if err := s.variant.Commit(ctx, tx, it.VariantID, it.Qty); err != nil {
					return err
				}
			}
		}
		// Order completion: if all sub-orders delivered, complete + settle COD payment.
		allDone, err := s.subOrder.AllDelivered(ctx, tx, so.OrderID)
		if err != nil {
			return err
		}
		if allDone {
			if err := s.orderRepo.UpdateStatusOnComplete(ctx, tx, so.OrderID); err != nil {
				return err
			}
			if ord.PaymentMethod == domain.PaymentMethodCOD {
				pay, err := s.payment.GetByOrderID(ctx, so.OrderID)
				if err == nil && pay.Status == domain.PaymentStatusPending {
					if err := s.payment.UpdateOnPaid(ctx, tx, pay.ID, []byte(`{"source":"cod_delivered"}`)); err != nil {
						return err
					}
				} else if err != nil && !errors.Is(err, paymentdomain.ErrPaymentNotFound) {
					return err
				}
			}
		}
	case goship.CategoryShipped:
		if err := s.subOrder.UpdateShippingStatus(ctx, tx, so.ID, p.StatusText, p.TrackingURL); err != nil {
			return err
		}
	default: // CategoryOther (return/lost/unknown) — record text only
		if err := s.subOrder.UpdateShippingStatus(ctx, tx, so.ID, p.StatusText, p.TrackingURL); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
```
> Verify `paymentRepo.GetByOrderID` + `UpdateOnPaid(ctx, tx, id, raw []byte)` signatures against `internal/payment/repo` and adjust. Verify `paymentdomain.ErrPaymentNotFound` exists.

- [ ] **Step 2: build**
Run: `go build ./...` → success.

- [ ] **Step 3: commit**
```bash
git add internal/order/service/shipping_webhook_service.go
git commit -m "feat(order): idempotent Goship status webhook service (delivered->complete, COD settle)"
```

---

### Task 10: handlers + routes

**Files:** Create `internal/order/handler/brand_fulfillment_handler.go`, `internal/order/handler/shipping_webhook_handler.go`; modify `internal/order/handler/routes.go`

- [ ] **Step 1: brand fulfillment handler** `brand_fulfillment_handler.go`:
```go
package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	brandmw "github.com/wearwhere/wearwhere_be/internal/brand/middleware"
	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	"github.com/wearwhere/wearwhere_be/internal/order/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type BrandFulfillmentHandler struct{ svc *service.FulfillmentService }

func NewBrandFulfillmentHandler(s *service.FulfillmentService) *BrandFulfillmentHandler {
	return &BrandFulfillmentHandler{svc: s}
}

func (h *BrandFulfillmentHandler) brandID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get(brandmw.CtxBrandID)
	if !ok {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

func (h *BrandFulfillmentHandler) List(c *gin.Context) {
	bid, ok := h.brandID(c)
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "NO_BRAND", "no brand context")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	var statuses []domain.SubOrderStatus
	if s := c.Query("status"); s != "" {
		statuses = append(statuses, domain.SubOrderStatus(s))
	}
	resp, err := h.svc.List(c.Request.Context(), bid, statuses, page, pageSize)
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "LIST_FAILED", err.Error())
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *BrandFulfillmentHandler) Detail(c *gin.Context) {
	bid, ok := h.brandID(c)
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "NO_BRAND", "no brand context")
		return
	}
	id, err := uuid.Parse(c.Param("sub_order_id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_ID", "invalid sub_order_id")
		return
	}
	resp, err := h.svc.Detail(c.Request.Context(), bid, id)
	if err != nil {
		writeFulfilErr(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *BrandFulfillmentHandler) Confirm(c *gin.Context) {
	bid, ok := h.brandID(c)
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "NO_BRAND", "no brand context")
		return
	}
	id, err := uuid.Parse(c.Param("sub_order_id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_ID", "invalid sub_order_id")
		return
	}
	if err := h.svc.Confirm(c.Request.Context(), bid, id); err != nil {
		writeFulfilErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "confirmed"})
}

func (h *BrandFulfillmentHandler) Ship(c *gin.Context) {
	bid, ok := h.brandID(c)
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "NO_BRAND", "no brand context")
		return
	}
	id, err := uuid.Parse(c.Param("sub_order_id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_ID", "invalid sub_order_id")
		return
	}
	var req domain.ShipReq
	_ = c.ShouldBindJSON(&req) // body optional
	if err := h.svc.Ship(c.Request.Context(), bid, id, req.Carrier); err != nil {
		writeFulfilErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "shipped"})
}

func writeFulfilErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrSubOrderNotFound):
		httpx.Error(c, http.StatusNotFound, "SUB_ORDER_NOT_FOUND", err.Error())
	case errors.Is(err, domain.ErrNotBrandOwner):
		httpx.Error(c, http.StatusForbidden, "NOT_OWNER", err.Error())
	case errors.Is(err, domain.ErrInvalidTransition):
		httpx.Error(c, http.StatusConflict, "INVALID_TRANSITION", err.Error())
	case errors.Is(err, domain.ErrCarrierUnavailable):
		httpx.Error(c, http.StatusConflict, "CARRIER_UNAVAILABLE", err.Error())
	case errors.Is(err, domain.ErrAddressIncomplete):
		httpx.Error(c, http.StatusConflict, "ADDRESS_INCOMPLETE", err.Error())
	case errors.Is(err, domain.ErrShipmentCreateFailed):
		httpx.Error(c, http.StatusBadGateway, "SHIPMENT_FAILED", err.Error())
	default:
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL", err.Error())
	}
}
```
> Confirm `brandmw.CtxBrandID` is exported and is the context key holding a `uuid.UUID` (from `internal/brand/middleware/brand_context.go`). Confirm `httpx.Error(c, status, code, msg)` signature (used by the location handler in Spec A).

- [ ] **Step 2: webhook handler** `shipping_webhook_handler.go`:
```go
package handler

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/wearwhere/wearwhere_be/internal/order/service"
	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
)

type ShippingWebhookHandler struct {
	svc      *service.ShippingWebhookService
	goship   goship.Shipper
	mockMode bool
}

func NewShippingWebhookHandler(s *service.ShippingWebhookService, gs goship.Shipper, mockMode bool) *ShippingWebhookHandler {
	return &ShippingWebhookHandler{svc: s, goship: gs, mockMode: mockMode}
}

// GoshipWebhook — POST /shipping/goship/webhook
func (h *ShippingWebhookHandler) GoshipWebhook(c *gin.Context) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read_body"})
		return
	}
	if !h.mockMode {
		sig := c.GetHeader("x-goship-hmac-sha256")
		if err := h.goship.VerifyWebhookSignature(raw, sig); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_signature"})
			return
		}
	}
	var p goship.WebhookPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}
	if err := h.svc.HandleGoshipWebhook(c.Request.Context(), p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"received": true})
}

// SimulateWebhook — POST /dev/goship/simulate (dev only)
func (h *ShippingWebhookHandler) SimulateWebhook(c *gin.Context) {
	var req struct {
		TrackingNo string `json:"tracking_no" form:"tracking_no"`
		Status     string `json:"status" form:"status"`         // free text, e.g. "Đã giao hàng"
		IsReturn   int    `json:"is_return" form:"is_return"`
		IsLost     int    `json:"is_lost" form:"is_lost"`
	}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	p := goship.WebhookPayload{Code: req.TrackingNo, StatusText: req.Status, IsReturn: req.IsReturn, IsLost: req.IsLost}
	if err := h.svc.HandleGoshipWebhook(c.Request.Context(), p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"simulated": true})
}
```
Add `"encoding/json"` to imports.

- [ ] **Step 3: routes** — append to `internal/order/handler/routes.go`:
```go
// MountBrand registers brand fulfillment routes. Caller chains brand auth onto rg.
func MountBrand(rg *gin.RouterGroup, h *BrandFulfillmentHandler) {
	o := rg.Group("/orders")
	o.GET("", h.List)
	o.GET("/:sub_order_id", h.Detail)
	o.POST("/:sub_order_id/confirm", h.Confirm)
	o.POST("/:sub_order_id/ship", h.Ship)
}

// MountShippingPublic registers the Goship status webhook (no auth).
func MountShippingPublic(rg *gin.RouterGroup, h *ShippingWebhookHandler) {
	rg.POST("/shipping/goship/webhook", h.GoshipWebhook)
}

// MountShippingDev registers dev-only simulate endpoint.
func MountShippingDev(rg *gin.RouterGroup, h *ShippingWebhookHandler) {
	rg.POST("/goship/simulate", h.SimulateWebhook)
}
```

- [ ] **Step 4: build**
Run: `go build ./...` → success.

- [ ] **Step 5: commit**
```bash
git add internal/order/handler/
git commit -m "feat(order): brand fulfillment handlers + Goship webhook/simulate handlers + routes"
```

---

## Phase 6 — Wiring + integration tests + live reconcile

### Task 11: Wire everything in main.go

**Files:** Modify `cmd/api/main.go`

- [ ] **Step 1: pass client secret to the goship factory** — update the existing `goship.NewFromConfig(goship.Config{...})` call to include `ClientSecret: cfg.Goship.ClientSecret`.

- [ ] **Step 2: construct services + handlers + mount routes.** Near the order service wiring:
```go
	fulfillmentSvc := orderservice.NewFulfillmentService(
		pgPool, orderRepo, subOrderRepo, orderItemRepo, goshipClient, addressRepo /* brand AddressPG */, weight.Defaults{
			WeightG: cfg.Goship.DefaultItemWeightG, LengthCM: cfg.Goship.DefaultLengthCM,
			WidthCM: cfg.Goship.DefaultWidthCM, HeightCM: cfg.Goship.DefaultHeightCM,
		},
	)
	shippingWebhookSvc := orderservice.NewShippingWebhookService(
		pgPool, subOrderRepo, orderRepo, orderItemRepo, paymentRepo, variantRepo,
	)
	brandFulfilHandler := orderhandler.NewBrandFulfillmentHandler(fulfillmentSvc)
	shippingWebhookHandler := orderhandler.NewShippingWebhookHandler(shippingWebhookSvc, goshipClient, goshipMockMode)

	orderhandler.MountBrand(brandGroup, brandFulfilHandler)         // brandGroup already has brand auth chain
	orderhandler.MountShippingPublic(v1, shippingWebhookHandler)    // public group (same as payos webhook)
```
Use the EXISTING variable names from main.go (`orderRepo`, `subOrderRepo`, `orderItemRepo`, `paymentRepo`, `variantRepo`, `addressRepo` (brand), `pgPool`, `brandGroup`, `v1`, `goshipClient`). `goshipMockMode` = `cfg.Goship.Mode == "mock" || cfg.Goship.Mode == ""` (compute it; reuse if a similar flag exists).
For dev simulate, mount under the same dev group used by PayOS dev endpoints, only when in mock/dev mode:
```go
	if goshipMockMode {
		orderhandler.MountShippingDev(devGroup, shippingWebhookHandler) // devGroup = the group where payos dev is mounted
	}
```
> Find where `paymenthandler.MountDev` is called to reuse its group + mock-mode condition.

- [ ] **Step 3: build the whole binary**
Run: `go build ./...` → success.

- [ ] **Step 4: full unit suite**
Run: `go test ./...` → PASS.

- [ ] **Step 5: commit**
```bash
git add cmd/api/main.go
git commit -m "feat(wiring): fulfillment service + brand fulfillment routes + Goship webhook routes"
```

---

### Task 12: Integration tests (fulfillment + webhook)

**Files:** Create `internal/order/service/fulfillment_service_test.go`, `internal/order/service/shipping_webhook_service_test.go` (both `//go:build integration`)

- [ ] **Step 1: fulfillment lifecycle test** — using `goship.NewMockClient()` and the testfixtures harness (mirror `order_service_test.go` `setupGoshipOrder`):
  - Seed brand with a primary brand_address that has city/district codes; seed customer address with codes; place a COD order (reuse the Spec A helper that already passes `ShippingSelections`).
  - Construct `FulfillmentService` with the mock goship client + brand `AddressPG` + `weight.Defaults{WeightG:500,...}`.
  - Confirm the sub-order → assert status `confirmed` in DB.
  - Ship with the stored carrier → assert status `shipped`, `tracking_no` non-null (starts `MOCK-TRK-`), `shipping_cost_vnd` set.
  - Cross-brand guard: `Detail`/`Confirm` with a different brandID → `ErrNotBrandOwner`.
  - Ship before confirm → `ErrInvalidTransition`.
```go
//go:build integration

package service_test
// ... construct svc := service.NewFulfillmentService(pool, orderRepo, subOrderRepo, itemRepo, goship.NewMockClient(), brandAddrRepo, weight.Defaults{WeightG:500,LengthCM:20,WidthCM:15,HeightCM:10})
// drive Confirm then Ship, assert DB rows. (Follow the existing integration harness in this package.)
```
> Write concrete assertions following the existing `order_service_test.go` patterns (same package_test, same fixtures). The mock carrier identifier is the display name `"Giao Hàng Nhanh (v3)"` (Spec A reconciliation) — use the carrier stored at placement (the Spec A helper selects it).

- [ ] **Step 2: webhook test** — place + confirm + ship a COD order (mock), then call `ShippingWebhookService.HandleGoshipWebhook` with a delivered payload (`Code = tracking_no`, `StatusText = "Đã giao hàng"`):
  - assert sub-order `delivered`, `delivered_at` set; order `completed`; COD payment `paid`; variant stock committed (reserved decremented).
  - second identical call → no-op (still delivered/completed; no double commit) — assert stock unchanged.
  - PayOS variant: place+pay (reuse payos path)+confirm+ship, deliver → order completed, payment already paid (unchanged), no double stock commit.

- [ ] **Step 3: run integration tests**
Run: `TEST_DATABASE_URL="postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable" go test -tags integration -p 1 ./internal/order/... -run "Fulfil|Goship|Webhook" -v`
Expected: PASS.

- [ ] **Step 4: full integration suite** (catch cross-package regressions like Spec A had)
Run: `TEST_DATABASE_URL="postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable" go test -tags integration -p 1 ./...`
Expected: PASS across ALL packages (including cmd/api E2E). Fix any caller broken by the `NewHTTPClient`/factory signature change.

- [ ] **Step 5: commit**
```bash
git add internal/order/service/fulfillment_service_test.go internal/order/service/shipping_webhook_service_test.go
git commit -m "test(order): fulfillment lifecycle + Goship webhook integration tests"
```

---

### Task 13: Live contract reconciliation (gated, no real shipment)

**Files:** Modify (if needed) `internal/shipping/goship/client_http.go`; modify `docs/superpowers/specs/2026-06-04-goship-fulfillment-design.md` §11

- [ ] **Step 1: confirm the `/shipments` request/response shape WITHOUT creating a shipment.** Add a gated test `internal/shipping/goship/client_http_shipment_real_test.go` (`//go:build goship_real`) that:
  - reads `GOSHIP_TOKEN`/`GOSHIP_BASE_URL` from env (skip if unset),
  - calls `Rates` to obtain a real `rate_id`,
  - ONLY if `GOSHIP_ALLOW_REAL_CREATE=1` calls `CreateShipment` (otherwise logs the would-be request body and skips the POST).
```go
//go:build goship_real

package goship
// realClient(t) reuses the helper from client_http_real_test.go
func TestRealGoship_CreateShipment_Gated(t *testing.T) {
	if os.Getenv("GOSHIP_ALLOW_REAL_CREATE") != "1" {
		t.Skip("set GOSHIP_ALLOW_REAL_CREATE=1 to actually create a real shipment")
	}
	// ... Rates -> pick rate.ID -> CreateShipment with test addresses -> log resp, adjust struct tags if needed
}
```
- [ ] **Step 2:** if run with creds, reconcile `client_http.go` `/shipments` field names to the real response and record the confirmed contract (request keys, response keys, label field, fee field) in spec §11. If NOT run (no opt-in), note in §11 that the `/shipments` contract remains assumed and is to be confirmed before production cutover.

- [ ] **Step 3: commit**
```bash
git add internal/shipping/goship docs/superpowers/specs/2026-06-04-goship-fulfillment-design.md
git commit -m "test(goship): gated real shipment-create check; document /shipments contract status"
```

---

## Definition of Done (Spec B)

- [ ] Migration 000031 applied (dev+test); sub_orders has cost/goship_code/tracking_url/status_text.
- [ ] `go test ./...` and `go test -tags integration -p 1 ./...` both green (full module, incl. cmd/api E2E).
- [ ] Brand can `GET /brand/me/orders`, view detail, `confirm`, and `ship` (own sub-orders only; cross-brand → 403).
- [ ] Ship re-quotes Goship, creates a shipment (mock in dev), stores tracking_no/goship_code/cost/tracking_url; customer fee unchanged.
- [ ] `POST /shipping/goship/webhook` verifies HMAC, idempotently maps status; delivered → sub-order delivered, COD stock committed + COD payment paid at completion, order `completed` when all delivered.
- [ ] `POST /dev/goship/simulate` works in mock mode; no real shipment is created in any test.
- [ ] Customer order detail surfaces tracking_no/carrier/status_text/tracking_url.
- [ ] `/shipments` contract confirmed against the live API, or §11 explicitly flags it as pending production confirmation.
- [ ] Out of scope confirmed deferred: cancellation, refund, return/lost restock.
