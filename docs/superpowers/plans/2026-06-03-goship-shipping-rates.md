# Goship Shipping Spec A — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace flat-rate shipping with real Goship multi-carrier rate quoting where the customer picks a carrier per brand at checkout, on a foundation of structured location codes (string codes) and per-variant weight/dimensions (Goship applies the volumetric adjustment).

**Architecture:** A new `internal/shipping/goship` client (interface + mock + HTTP + factory) mirrors the existing `internal/payment/payos` package. The `ShippingProvider` interface changes from single-fee `Calculate` to multi-option `Quote`. Checkout preview returns carrier options per brand; place-order re-quotes by the chosen carrier and stores the authoritative fee. Address tables gain `city_code/district_code/ward_code`; variants gain weight + dimensions.

**Tech Stack:** Go, gin, pgx v5, golang-migrate SQL migrations, testify-style table tests (match existing repo conventions).

**Spec:** `docs/superpowers/specs/2026-06-03-goship-shipping-rates-design.md`

**Conventions to follow:**
- Module path: `github.com/wearwhere/wearwhere_be`
- Run all tests: `go test ./...`  |  Integration: `go test -tags integration -p 1 ./...`  |  Real sandbox: `go test -tags goship_real ./internal/shipping/goship/...`
- Commit messages: NO `Co-Authored-By` trailer (project rule).
- Migrations live in `db/migrations/`, numbered sequentially; current max is `000026`.

---

## File Structure

```
db/migrations/
  000027_add_location_codes_to_customer_addresses.{up,down}.sql   NEW
  000028_add_location_codes_to_brand_addresses.{up,down}.sql      NEW
  000029_add_dimensions_to_variants.{up,down}.sql                 NEW
  000030_add_shipping_carrier_to_sub_orders.{up,down}.sql         NEW

internal/config/config.go                                         MODIFY  GoshipConfig + ShippingConfig.Provider
.env.example                                                      MODIFY  Goship vars

internal/shipping/goship/                                         NEW (mirror internal/payment/payos)
  client.go            interface + DTOs (Location, RateReq, Rate, Address, Parcel) + errors
  client_mock.go       deterministic mock
  client_mock_test.go
  client_http.go       real HTTP client (Bearer token)
  client_http_real_test.go   build tag: goship_real
  factory.go           NewFromConfig(mode)

internal/shipping/weight/                                         NEW
  weight.go            Aggregate(items, defaults) -> Parcel (weight+dims; Goship applies volumetric)
  weight_test.go

internal/shipping/domain/option.go                                NEW  ShippingOption
internal/shipping/provider/provider.go                            MODIFY  Quote interface + CalcReq codes/weight
internal/shipping/provider/flat_rate.go                           MODIFY  single-option Quote
internal/shipping/provider/goship_provider.go                     NEW  Quote via goship.Client
internal/shipping/provider/goship_provider_test.go                NEW
internal/shipping/provider/factory.go                             MODIFY  "goship" branch

internal/shipping/location/                                        NEW
  service.go           Cities/Districts/Wards + TTL cache
  service_test.go
  handler.go           gin handlers
  routes.go            RegisterRoutes

internal/order/domain/order.go                                    MODIFY  ShippingAddress codes
internal/order/domain/dto.go                                      MODIFY  preview options, PlaceOrderReq.ShippingSelections
internal/order/domain/errors.go                                   MODIFY  ErrAddressIncomplete, ErrCarrierUnavailable
internal/order/service/checkout_service.go                        MODIFY  Quote + address-incomplete
internal/order/service/order_service.go                           MODIFY  re-quote by carrier, store shipping_carrier

internal/customeraddr/...                                          MODIFY  accept+validate codes (create/update)
internal/brand/... (brand address write path)                     MODIFY  accept+validate codes

cmd/api/main.go                                                   MODIFY  wire goship client, provider, location routes
```

---

## Phase 1 — Schema & Domain Foundation

### Task 1: Migrations for location codes, variant dimensions, sub-order carrier ✅ DONE (78837a8)

**Files:**
- Create: `db/migrations/000027_add_location_codes_to_customer_addresses.up.sql` / `.down.sql`
- Create: `db/migrations/000028_add_location_codes_to_brand_addresses.up.sql` / `.down.sql`
- Create: `db/migrations/000029_add_dimensions_to_variants.up.sql` / `.down.sql`
- Create: `db/migrations/000030_add_shipping_carrier_to_sub_orders.up.sql` / `.down.sql`

- [ ] **Step 1: Confirm the next free migration number**

Run: `ls db/migrations | sort | tail -5`
Expected: highest existing is `000026_*`. If higher numbers exist, renumber the four new files to continue the sequence and update this task accordingly.

- [ ] **Step 2: Write `000027_add_location_codes_to_customer_addresses.up.sql`**

Goship location codes are numeric **strings** (e.g. district `"100100"`, city `"100000"`) — use `TEXT`.
```sql
ALTER TABLE customer_addresses
  ADD COLUMN city_code     TEXT,
  ADD COLUMN district_code TEXT,
  ADD COLUMN ward_code     TEXT;
```

`.down.sql`:
```sql
ALTER TABLE customer_addresses
  DROP COLUMN city_code,
  DROP COLUMN district_code,
  DROP COLUMN ward_code;
```

- [ ] **Step 3: Write `000028_add_location_codes_to_brand_addresses.up.sql`**

```sql
ALTER TABLE brand_addresses
  ADD COLUMN city_code     TEXT,
  ADD COLUMN district_code TEXT,
  ADD COLUMN ward_code     TEXT;
```

`.down.sql`:
```sql
ALTER TABLE brand_addresses
  DROP COLUMN city_code,
  DROP COLUMN district_code,
  DROP COLUMN ward_code;
```

- [ ] **Step 4: Write `000029_add_dimensions_to_variants.up.sql`**

The variants table is `product_variants` (confirmed in `order_service.go` JOIN `product_variants v`).
```sql
ALTER TABLE product_variants
  ADD COLUMN weight_g  INT CHECK (weight_g  IS NULL OR weight_g  > 0),
  ADD COLUMN length_cm INT CHECK (length_cm IS NULL OR length_cm > 0),
  ADD COLUMN width_cm  INT CHECK (width_cm  IS NULL OR width_cm  > 0),
  ADD COLUMN height_cm INT CHECK (height_cm IS NULL OR height_cm > 0);
```

`.down.sql`:
```sql
ALTER TABLE product_variants
  DROP COLUMN weight_g,
  DROP COLUMN length_cm,
  DROP COLUMN width_cm,
  DROP COLUMN height_cm;
```

- [ ] **Step 5: Write `000030_add_shipping_carrier_to_sub_orders.up.sql`**

```sql
ALTER TABLE sub_orders ADD COLUMN shipping_carrier TEXT;
```

`.down.sql`:
```sql
ALTER TABLE sub_orders DROP COLUMN shipping_carrier;
```

- [ ] **Step 6: Apply migrations against the dev DB**

Run the project's migrate command (check `Makefile`/README for the exact target, e.g. `make migrate-up` or `migrate -path db/migrations -database "$DATABASE_URL" up`).
Expected: `000030` applied, no errors. Verify with `\d product_variants` and `\d customer_addresses` in psql that the columns exist.

- [ ] **Step 7: Commit**

```bash
git add db/migrations/000027_* db/migrations/000028_* db/migrations/000029_* db/migrations/000030_*
git commit -m "feat(db): location codes on addresses, dimensions on variants, shipping_carrier on sub_orders"
```

---

### Task 2: Add code/dimension fields to domain structs + repo read/write ✅ DONE (6d54ecb)

> Review notes carried forward: (a) address repo `Update` uses `COALESCE($n, col)` with nil codes — Task 15 must pass the request codes through `Update` (and may switch to direct assignment since codes are required); (b) `ShippingAddress` construction sites in checkout_service.go / order_service.go must copy the 3 codes — done in Tasks 13/14.

**Files:**
- Modify: `internal/customeraddr/domain/address.go` (add 3 code fields)
- Modify: `internal/brand/domain/brand.go` (`BrandAddress` add 3 code fields)
- Modify: `internal/order/domain/order.go` (`ShippingAddress` add 3 code fields)
- Modify: the variant domain struct (search: `grep -rn "WeightG\|stock_qty" internal/product internal/catalog`) to add `WeightG, LengthCM, WidthCM, HeightCM *int`
- Modify: the corresponding repos' INSERT/SELECT/scan for customer_addresses, brand_addresses, product_variants

- [ ] **Step 1: Add fields to `CustomerAddress`**

In `internal/customeraddr/domain/address.go`, add after `City string` (Goship codes are strings):
```go
	CityCode     *string
	DistrictCode *string
	WardCode     *string
```

- [ ] **Step 2: Add the same three `*string` fields to `BrandAddress`** in `internal/brand/domain/brand.go` (after `City string`).

- [ ] **Step 3: Add fields to `ShippingAddress`**

In `internal/order/domain/order.go`:
```go
type ShippingAddress struct {
	Recipient    string  `json:"recipient"`
	Phone        string  `json:"phone"`
	Line1        string  `json:"line1"`
	Ward         string  `json:"ward"`
	District     string  `json:"district"`
	City         string  `json:"city"`
	CityCode     *string `json:"city_code,omitempty"`
	DistrictCode *string `json:"district_code,omitempty"`
	WardCode     *string `json:"ward_code,omitempty"`
}
```

- [ ] **Step 4: Add dimension fields to the variant domain struct**

Locate the struct (Step note above). Add:
```go
	WeightG  *int
	LengthCM *int
	WidthCM  *int
	HeightCM *int
```

- [ ] **Step 5: Update repos to persist/read the new columns**

For each repo (customer address, brand address, product variant): add the new columns to INSERT column lists + `$n` placeholders + arg slices, to UPDATE statements, and to every `SELECT ... ` + `rows.Scan(...)` / `row.Scan(...)` that hydrates the struct. Use `&a.CityCode` etc. (pgx scans `TEXT NULL` → `*string` for the address codes; `INT NULL` → `*int` for the variant dimensions).

- [ ] **Step 6: Build to verify scans compile**

Run: `go build ./...`
Expected: success (no behavior change yet; tests come with the consumers).

- [ ] **Step 7: Commit**

```bash
git add internal/customeraddr internal/brand internal/order/domain internal/product
git commit -m "feat(domain): location-code and variant-dimension fields wired through structs+repos"
```

---

## Phase 2 — Config

### Task 3: GoshipConfig + ShippingConfig.Provider + .env.example ✅ DONE (7954255)

**Files:**
- Modify: `internal/config/config.go`
- Modify: `.env.example`
- Test: `internal/config/config_test.go` (create if absent)

- [ ] **Step 1: Write the failing test**

In `internal/config/config_test.go`:
```go
package config

import (
	"os"
	"testing"
)

func TestLoad_GoshipDefaults(t *testing.T) {
	os.Clearenv()
	os.Setenv("DATABASE_URL", "postgres://x") // satisfy any required env; adjust to actual required keys
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Goship.Mode != "mock" {
		t.Errorf("Goship.Mode = %q, want mock", cfg.Goship.Mode)
	}
	if cfg.Goship.DefaultItemWeightG != 500 {
		t.Errorf("DefaultItemWeightG = %d, want 500", cfg.Goship.DefaultItemWeightG)
	}
	if cfg.Shipping.Provider != "flat" {
		t.Errorf("Shipping.Provider default = %q, want flat", cfg.Shipping.Provider)
	}
}
```
> If `Load()` requires other env vars, set them so the test reaches the Goship assertions. Inspect the top of `config.go` for required keys.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoad_GoshipDefaults -v`
Expected: FAIL — `cfg.Goship` undefined.

- [ ] **Step 3: Add the config structs and loading**

In `config.go`, add to the load function (next to the existing `cfg.Shipping = ...` block at ~line 186):
```go
	cfg.Goship = GoshipConfig{
		Mode:               getEnv("GOSHIP_MODE", "mock"),
		Token:              getEnv("GOSHIP_TOKEN", ""),
		BaseURL:            getEnv("GOSHIP_BASE_URL", "https://sandbox.goship.io/api/v2"),
		DefaultItemWeightG: getInt("GOSHIP_DEFAULT_ITEM_WEIGHT_G", 500),
		DefaultLengthCM:    getInt("GOSHIP_DEFAULT_LENGTH_CM", 20),
		DefaultWidthCM:     getInt("GOSHIP_DEFAULT_WIDTH_CM", 15),
		DefaultHeightCM:    getInt("GOSHIP_DEFAULT_HEIGHT_CM", 10),
	}
```
Change the existing shipping default to `goship`-aware (keep `flat` as the documented fallback default for now to avoid breaking dev without a token):
```go
	cfg.Shipping = ShippingConfig{
		Provider: getEnv("SHIPPING_PROVIDER", "flat"),
	}
```
Add the struct + a field on `Config`:
```go
type GoshipConfig struct {
	Mode               string // mock | sandbox | production
	Token              string
	BaseURL            string
	DefaultItemWeightG int
	DefaultLengthCM    int
	DefaultWidthCM     int
	DefaultHeightCM    int
}
```
Add `Goship GoshipConfig` to the `Config` struct (find where `Shipping ShippingConfig` is declared and add alongside).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestLoad_GoshipDefaults -v`
Expected: PASS

- [ ] **Step 5: Update `.env.example`**

Append after the shipping section:
```bash
# Goship shipping
GOSHIP_MODE=mock                    # mock | sandbox | production
GOSHIP_TOKEN=                       # Bearer token (required for sandbox/production)
GOSHIP_BASE_URL=https://sandbox.goship.io/api/v2
GOSHIP_DEFAULT_ITEM_WEIGHT_G=500
GOSHIP_DEFAULT_LENGTH_CM=20
GOSHIP_DEFAULT_WIDTH_CM=15
GOSHIP_DEFAULT_HEIGHT_CM=10
SHIPPING_PROVIDER=flat              # goship | flat
```

- [ ] **Step 6: Commit**

```bash
git add internal/config .env.example
git commit -m "feat(config): GoshipConfig + defaults + .env.example"
```

---

## Phase 3 — Goship Client

### Task 4: Client interface + DTOs ✅ DONE (df37574)

**Files:**
- Create: `internal/shipping/goship/client.go`

- [ ] **Step 1: Write `client.go`**

```go
package goship

import (
	"context"
	"errors"
)

var (
	ErrRates    = errors.New("goship: failed to fetch rates")
	ErrLocation = errors.New("goship: failed to fetch location list")
)

// Location is a city, district, or ward as returned by Goship.
// Goship codes are numeric strings, e.g. "100000".
type Location struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Address is one endpoint of a shipment (sender or receiver).
type Address struct {
	DistrictCode string
	CityCode     string
}

// Parcel describes the package being shipped. We send actual weight + dims;
// Goship applies volumetric weight (divisor ~6000) server-side.
type Parcel struct {
	WeightG   int
	LengthCM  int
	WidthCM   int
	HeightCM  int
	CODVND    int64 // amount the carrier collects on delivery (0 for prepaid/PayOS)
	AmountVND int64 // declared goods value
}

type RateReq struct {
	From   Address
	To     Address
	Parcel Parcel
}

// Rate is one carrier option returned by Goship.
type Rate struct {
	ID          string // Goship rate id (short-lived; not persisted in Spec A)
	Carrier     string // carrier code (e.g. "ghnv3"); falls back to CarrierName if Goship omits a code
	CarrierName string
	Service     string
	FeeVND      int64
	ETA         string // human-readable expected delivery ("expected")
}

type Client interface {
	Cities(ctx context.Context) ([]Location, error)
	Districts(ctx context.Context, cityCode string) ([]Location, error)
	Wards(ctx context.Context, districtCode string) ([]Location, error)
	Rates(ctx context.Context, r RateReq) ([]Rate, error)
}
```

- [ ] **Step 2: Build**

Run: `go build ./internal/shipping/goship/`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/shipping/goship/client.go
git commit -m "feat(goship): client interface + DTOs"
```

---

### Task 5: Mock client ✅ DONE (8d87e51)

**Files:**
- Create: `internal/shipping/goship/client_mock.go`
- Create: `internal/shipping/goship/client_mock_test.go`

- [ ] **Step 1: Write the failing test**

```go
package goship

import (
	"context"
	"testing"
)

func TestMock_Rates_DeterministicByWeight(t *testing.T) {
	m := NewMockClient()
	got, err := m.Rates(context.Background(), RateReq{
		From:   Address{DistrictCode: "100100", CityCode: "100000"},
		To:     Address{DistrictCode: "200100", CityCode: "200000"},
		Parcel: Parcel{WeightG: 1500},
	})
	if err != nil {
		t.Fatalf("Rates: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 carriers, got %d", len(got))
	}
	// ghnv3 base 15000 + 5000/kg * ceil(1.5kg=2) = 25000
	for _, r := range got {
		if r.Carrier == "ghnv3" && r.FeeVND != 25000 {
			t.Errorf("ghnv3 fee = %d, want 25000", r.FeeVND)
		}
		if r.ID == "" || r.CarrierName == "" {
			t.Errorf("rate missing id/name: %+v", r)
		}
	}
}

func TestMock_Cities_NonEmpty(t *testing.T) {
	m := NewMockClient()
	c, err := m.Cities(context.Background())
	if err != nil || len(c) == 0 {
		t.Fatalf("Cities empty/err: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/shipping/goship/ -run TestMock -v`
Expected: FAIL — `NewMockClient` undefined.

- [ ] **Step 3: Write `client_mock.go`**

```go
package goship

import (
	"context"
	"fmt"
	"math"
)

type MockClient struct{}

func NewMockClient() *MockClient { return &MockClient{} }

func (m *MockClient) Cities(_ context.Context) ([]Location, error) {
	return []Location{{Code: "100000", Name: "Hồ Chí Minh"}, {Code: "200000", Name: "Hà Nội"}}, nil
}

func (m *MockClient) Districts(_ context.Context, cityCode string) ([]Location, error) {
	return []Location{{Code: cityCode + "100", Name: "Quận 1"}, {Code: cityCode + "200", Name: "Quận 2"}}, nil
}

func (m *MockClient) Wards(_ context.Context, districtCode string) ([]Location, error) {
	return []Location{{Code: districtCode + "01", Name: "Phường 1"}}, nil
}

func (m *MockClient) Rates(_ context.Context, r RateReq) ([]Rate, error) {
	kg := int(math.Ceil(float64(r.Parcel.WeightG) / 1000.0))
	if kg < 1 {
		kg = 1
	}
	carriers := []struct {
		code, name string
		base, perKg int64
	}{
		{"ghnv3", "Giao Hàng Nhanh", 15000, 5000},
		{"ghtk", "Giao Hàng Tiết Kiệm", 12000, 4000},
		{"vtp", "Viettel Post", 18000, 6000},
	}
	out := make([]Rate, 0, len(carriers))
	for i, c := range carriers {
		out = append(out, Rate{
			ID:          fmt.Sprintf("mock-rate-%s-%d", c.code, i),
			Carrier:     c.code,
			CarrierName: c.name,
			Service:     "standard",
			FeeVND:      c.base + c.perKg*int64(kg),
			ETA:         "2-4 ngày",
		})
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/shipping/goship/ -run TestMock -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/shipping/goship/client_mock.go internal/shipping/goship/client_mock_test.go
git commit -m "feat(goship): deterministic mock client + tests"
```

---

### Task 6: HTTP client + factory + real sandbox test ✅ DONE (1e35db6) — sandbox reconciliation deferred to Task 17

**Files:**
- Create: `internal/shipping/goship/client_http.go`
- Create: `internal/shipping/goship/factory.go`
- Create: `internal/shipping/goship/client_http_real_test.go` (build tag `goship_real`)

- [ ] **Step 1: Write `client_http.go`**

> Endpoint paths/field names below are the assumed Goship v2 shape; the real-sandbox test in Step 3 is what pins them. If the sandbox responds with different JSON keys/paths, adjust the struct tags and URLs here until that test passes.

```go
package goship

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type HTTPClient struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

func NewHTTPClient(token, baseURL string) *HTTPClient {
	return &HTTPClient{
		token:      token,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *HTTPClient) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("goship %s %s: status=%d body=%s", method, path, resp.StatusCode, string(b))
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

// locationEnvelope matches Goship's { "data": [ { "id": "100000", "name": "..." } ] }.
// id may arrive as a JSON string or number depending on the endpoint, so decode
// it as json.Number and stringify.
type locationEnvelope struct {
	Data []struct {
		ID   json.Number `json:"id"`
		Name string      `json:"name"`
	} `json:"data"`
}

func (c *HTTPClient) locations(ctx context.Context, path string) ([]Location, error) {
	var env locationEnvelope
	if err := c.do(ctx, http.MethodGet, path, nil, &env); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLocation, err)
	}
	out := make([]Location, 0, len(env.Data))
	for _, d := range env.Data {
		out = append(out, Location{Code: d.ID.String(), Name: d.Name})
	}
	return out, nil
}

func (c *HTTPClient) Cities(ctx context.Context) ([]Location, error) {
	return c.locations(ctx, "/cities")
}

func (c *HTTPClient) Districts(ctx context.Context, cityCode string) ([]Location, error) {
	return c.locations(ctx, "/cities/"+cityCode+"/districts")
}

func (c *HTTPClient) Wards(ctx context.Context, districtCode string) ([]Location, error) {
	return c.locations(ctx, "/districts/"+districtCode+"/wards")
}

func (c *HTTPClient) Rates(ctx context.Context, r RateReq) ([]Rate, error) {
	body := map[string]any{
		"shipment": map[string]any{
			"address_from": map[string]any{"district": r.From.DistrictCode, "city": r.From.CityCode},
			"address_to":   map[string]any{"district": r.To.DistrictCode, "city": r.To.CityCode},
			"parcel": map[string]any{
				"cod":    r.Parcel.CODVND,
				"amount": r.Parcel.AmountVND,
				"weight": r.Parcel.WeightG,
				"length": r.Parcel.LengthCM,
				"width":  r.Parcel.WidthCM,
				"height": r.Parcel.HeightCM,
			},
		},
	}
	var env struct {
		Data []struct {
			ID          string `json:"id"`
			Carrier     string `json:"carrier"`      // may be absent; see fallback below
			CarrierName string `json:"carrier_name"`
			Service     string `json:"service"`
			TotalFee    int64  `json:"total_fee"`
			Expected    string `json:"expected"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodPost, "/rates", body, &env); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRates, err)
	}
	out := make([]Rate, 0, len(env.Data))
	for _, d := range env.Data {
		carrier := d.Carrier
		if carrier == "" {
			// Goship's rate object may not expose a short code — fall back to the
			// display name so preview and place-order key selection consistently.
			carrier = d.CarrierName
		}
		out = append(out, Rate{
			ID: d.ID, Carrier: carrier, CarrierName: d.CarrierName,
			Service: d.Service, FeeVND: d.TotalFee, ETA: d.Expected,
		})
	}
	return out, nil
}
```

- [ ] **Step 2: Write `factory.go`**

```go
package goship

import "fmt"

type Config struct {
	Mode    string // mock | sandbox | production
	Token   string
	BaseURL string
}

func NewFromConfig(cfg Config) (Client, error) {
	switch cfg.Mode {
	case "mock", "":
		return NewMockClient(), nil
	case "sandbox", "production":
		if cfg.Token == "" {
			return nil, fmt.Errorf("goship: %s mode requires GOSHIP_TOKEN", cfg.Mode)
		}
		return NewHTTPClient(cfg.Token, cfg.BaseURL), nil
	default:
		return nil, fmt.Errorf("goship: unknown mode %q (want mock|sandbox|production)", cfg.Mode)
	}
}
```

- [ ] **Step 3: Write `client_http_real_test.go`** (gated, mirrors `payos_real`)

```go
//go:build goship_real

package goship

import (
	"context"
	"os"
	"testing"
)

func realClient(t *testing.T) *HTTPClient {
	tok := os.Getenv("GOSHIP_TOKEN")
	if tok == "" {
		t.Skip("GOSHIP_TOKEN not set; skipping real Goship test")
	}
	base := os.Getenv("GOSHIP_BASE_URL")
	if base == "" {
		base = "https://sandbox.goship.io/api/v2"
	}
	return NewHTTPClient(tok, base)
}

func TestRealGoship_Cities(t *testing.T) {
	c := realClient(t)
	cities, err := c.Cities(context.Background())
	if err != nil {
		t.Fatalf("Cities: %v", err)
	}
	if len(cities) == 0 {
		t.Fatal("expected at least one city")
	}
	t.Logf("got %d cities; first=%+v", len(cities), cities[0])
}

func TestRealGoship_Rates(t *testing.T) {
	c := realClient(t)
	// Pick two real district/city codes discovered via Cities/Districts in the sandbox.
	from := Address{DistrictCode: os.Getenv("GOSHIP_TEST_FROM_DISTRICT"), CityCode: os.Getenv("GOSHIP_TEST_FROM_CITY")}
	to := Address{DistrictCode: os.Getenv("GOSHIP_TEST_TO_DISTRICT"), CityCode: os.Getenv("GOSHIP_TEST_TO_CITY")}
	if from.DistrictCode == "" || to.DistrictCode == "" {
		t.Skip("set GOSHIP_TEST_FROM_*/TO_* district+city codes to run rates test")
	}
	rates, err := c.Rates(context.Background(), RateReq{From: from, To: to, Parcel: Parcel{WeightG: 1000, LengthCM: 20, WidthCM: 15, HeightCM: 10, AmountVND: 500000}})
	if err != nil {
		t.Fatalf("Rates: %v", err)
	}
	if len(rates) == 0 {
		t.Fatal("expected at least one carrier rate")
	}
	// Logs the carrier code/name list — used to confirm whether a short code is
	// present and to align the mock fixture (Task 17).
	for _, r := range rates {
		t.Logf("rate id=%s carrier=%q name=%q fee=%d eta=%q", r.ID, r.Carrier, r.CarrierName, r.FeeVND, r.ETA)
	}
}
```

- [ ] **Step 4: Build (mock path) and run the gated real test if a token is present**

Run: `go build ./internal/shipping/goship/ && go vet ./internal/shipping/goship/`
Expected: success.
If `GOSHIP_TOKEN` is available, run `go test -tags goship_real ./internal/shipping/goship/ -run TestRealGoship_Cities -v` and **adjust JSON tags/paths in `client_http.go` until it passes.** Record any contract differences in the spec's §11.

- [ ] **Step 5: Commit**

```bash
git add internal/shipping/goship/client_http.go internal/shipping/goship/factory.go internal/shipping/goship/client_http_real_test.go
git commit -m "feat(goship): HTTP client + factory + gated sandbox integration test"
```

---

## Phase 4 — Chargeable Weight

### Task 7: weight.Aggregate (parcel builder) ✅ DONE (486bcc2)

Goship applies volumetric weight server-side from the `weight` + `width/height/length` we send (divisor ≈6000), so this package only **aggregates** a sub-order's items into one parcel — it does NOT pre-compute chargeable weight.

**Files:**
- Create: `internal/shipping/weight/weight.go`
- Create: `internal/shipping/weight/weight_test.go`

- [ ] **Step 1: Write the failing test**

```go
package weight

import "testing"

func TestAggregate(t *testing.T) {
	d := Defaults{WeightG: 500, LengthCM: 20, WidthCM: 15, HeightCM: 10}
	ip := func(v int) *int { return &v }

	t.Run("explicit fields: weight sums by qty, L/W take max, H stacks", func(t *testing.T) {
		// qty2 weight300 L30 W25 H5  -> weight 600, L 30, W 25, H 10
		got := Aggregate([]Item{{Qty: 2, WeightG: ip(300), LengthCM: ip(30), WidthCM: ip(25), HeightCM: ip(5)}}, d)
		if got.WeightG != 600 || got.LengthCM != 30 || got.WidthCM != 25 || got.HeightCM != 10 {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("missing fields fall back to defaults", func(t *testing.T) {
		// qty3 nil -> weight 1500, L20 W15 H 3*10=30
		got := Aggregate([]Item{{Qty: 3}}, d)
		if got.WeightG != 1500 || got.LengthCM != 20 || got.WidthCM != 15 || got.HeightCM != 30 {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("multiple items: max footprint, summed weight + stacked height", func(t *testing.T) {
		// item A qty1 w200 L10 W10 H4 ; item B qty1 w800 L40 W30 H6
		got := Aggregate([]Item{
			{Qty: 1, WeightG: ip(200), LengthCM: ip(10), WidthCM: ip(10), HeightCM: ip(4)},
			{Qty: 1, WeightG: ip(800), LengthCM: ip(40), WidthCM: ip(30), HeightCM: ip(6)},
		}, d)
		if got.WeightG != 1000 || got.LengthCM != 40 || got.WidthCM != 30 || got.HeightCM != 10 {
			t.Fatalf("got %+v", got)
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/shipping/weight/ -v`
Expected: FAIL — undefined `Aggregate`, `Item`, `Defaults`, `Parcel`.

- [ ] **Step 3: Write `weight.go`**

```go
package weight

type Defaults struct {
	WeightG, LengthCM, WidthCM, HeightCM int
}

// Item is one cart line; nil dimension fields fall back to Defaults.
type Item struct {
	Qty      int
	WeightG  *int
	LengthCM *int
	WidthCM  *int
	HeightCM *int
}

// Parcel is the aggregated package sent to the carrier. Goship applies the
// volumetric adjustment itself, so these are actual aggregated values.
type Parcel struct {
	WeightG  int
	LengthCM int
	WidthCM  int
	HeightCM int
}

func or(p *int, def int) int {
	if p != nil && *p > 0 {
		return *p
	}
	return def
}

// Aggregate combines a sub-order's items into one parcel: weight sums by qty,
// length/width take the max footprint, height stacks (Σ qty*height).
func Aggregate(items []Item, d Defaults) Parcel {
	var p Parcel
	for _, it := range items {
		p.WeightG += it.Qty * or(it.WeightG, d.WeightG)
		if l := or(it.LengthCM, d.LengthCM); l > p.LengthCM {
			p.LengthCM = l
		}
		if w := or(it.WidthCM, d.WidthCM); w > p.WidthCM {
			p.WidthCM = w
		}
		p.HeightCM += it.Qty * or(it.HeightCM, d.HeightCM)
	}
	return p
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/shipping/weight/ -v`
Expected: PASS (all three subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/shipping/weight/
git commit -m "feat(shipping): parcel aggregator (weight/dims); Goship applies volumetric"
```

---

## Phase 5 — Provider Interface & Goship Provider

### Task 8: ShippingOption + Quote interface + flat-rate adapt + factory ✅ DONE (fbb0068) — full build red until Task 14

**Files:**
- Create: `internal/shipping/domain/option.go`
- Modify: `internal/shipping/provider/provider.go`
- Modify: `internal/shipping/provider/flat_rate.go`
- Modify: `internal/shipping/provider/factory.go`
- Test: `internal/shipping/provider/flat_rate_test.go` (update existing)

- [ ] **Step 1: Write `option.go`**

```go
package domain

type ShippingOption struct {
	Carrier     string // "" / "flat" for flat-rate; carrier code for goship
	CarrierName string
	Service     string
	AmountVND   int64
	ETA         string
}
```

- [ ] **Step 2: Change the interface + CalcReq in `provider.go`**

Replace the `CalcItem`, `CalcReq`, and `ShippingProvider` definitions:
```go
type CalcItem struct {
	VariantID uuid.UUID
	ProductID uuid.UUID
	Qty       int
	WeightG   *int
	LengthCM  *int
	WidthCM   *int
	HeightCM  *int
}

type CalcReq struct {
	BrandID    uuid.UUID
	ToAddress  ShippingAddress
	ToCityCode *string
	ToDistrict *string
	CODVND     int64 // carrier-collected amount (grand total for COD, 0 for PayOS)
	AmountVND  int64 // declared goods value (sub-order subtotal)
	Items      []CalcItem
}

type ShippingProvider interface {
	Quote(ctx context.Context, r CalcReq) ([]shippingdomain.ShippingOption, error)
}
```
(Keep the `ShippingAddress` struct as-is.)

- [ ] **Step 3: Update the failing flat-rate test first**

In `flat_rate_test.go`, change the call/assertion to the new shape:
```go
func TestFlatRate_Quote_SingleOption(t *testing.T) {
	// existing fake brand repo returning ShippingFlatFeeVND = 30000
	p := NewFlatRateProvider(fakeBrandRepo{fee: 30000})
	opts, err := p.Quote(context.Background(), CalcReq{BrandID: someBrandID})
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if len(opts) != 1 || opts[0].AmountVND != 30000 || opts[0].Carrier != "flat" {
		t.Fatalf("unexpected options: %+v", opts)
	}
}
```
> Reuse whatever fake/stub `brandRepo` the existing test already defines; only the method name and return shape change.

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/shipping/provider/ -run TestFlatRate -v`
Expected: FAIL — `Quote` undefined / old `Calculate` gone.

- [ ] **Step 5: Rewrite `flat_rate.go` to implement `Quote`**

```go
func (p *FlatRateProvider) Quote(ctx context.Context, r CalcReq) ([]shippingdomain.ShippingOption, error) {
	b, err := p.brandRepo.FindByID(ctx, r.BrandID)
	if err != nil {
		return nil, err
	}
	return []shippingdomain.ShippingOption{{
		Carrier:     "flat",
		CarrierName: "Standard",
		Service:     "standard",
		AmountVND:   b.ShippingFlatFeeVND,
	}}, nil
}
```
(Delete the old `Calculate` method.)

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/shipping/provider/ -run TestFlatRate -v`
Expected: PASS

- [ ] **Step 7: Add the `goship` branch to `factory.go`**

```go
type Config struct {
	Provider string // "flat" | "goship"
}

func NewFromConfig(cfg Config, brandRepo brandrepo.BrandRepo, gp *GoshipDeps) (ShippingProvider, error) {
	switch cfg.Provider {
	case "", "flat":
		return NewFlatRateProvider(brandRepo), nil
	case "goship":
		if gp == nil {
			return nil, fmt.Errorf("shipping: goship provider requires GoshipDeps")
		}
		return NewGoshipProvider(gp.Client, gp.PickupRepo, gp.Defaults), nil
	default:
		return nil, fmt.Errorf("unknown shipping provider: %q", cfg.Provider)
	}
}
```
`GoshipDeps`, `NewGoshipProvider`, and `PickupRepo` are defined in Task 9. (Compilation will be completed in Task 9 — commit Task 8 and Task 9 together if `go build` fails between them.)

- [ ] **Step 8: Commit**

```bash
git add internal/shipping/domain/option.go internal/shipping/provider/provider.go internal/shipping/provider/flat_rate.go internal/shipping/provider/factory.go internal/shipping/provider/flat_rate_test.go
git commit -m "feat(shipping): Quote(multi-option) interface; flat-rate returns single option"
```

---

### Task 9: GoshipProvider ✅ DONE (6221771) — incl. pgx.ErrNoRows→ErrPickupIncomplete fix

**Files:**
- Create: `internal/shipping/provider/goship_provider.go`
- Create: `internal/shipping/provider/goship_provider_test.go`

- [ ] **Step 1: Write the failing test (with a stub goship client + stub pickup repo)**

```go
package provider

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
	"github.com/wearwhere/wearwhere_be/internal/shipping/weight"
)

type stubGoship struct{ rates []goship.Rate }

func (s stubGoship) Cities(context.Context) ([]goship.Location, error) { return nil, nil }
func (s stubGoship) Districts(context.Context, string) ([]goship.Location, error) { return nil, nil }
func (s stubGoship) Wards(context.Context, string) ([]goship.Location, error) { return nil, nil }
func (s stubGoship) Rates(context.Context, goship.RateReq) ([]goship.Rate, error) {
	return s.rates, nil
}

type stubPickup struct{ city, district string; err error }

func (s stubPickup) PrimaryAddressCodes(_ context.Context, _ uuid.UUID) (city, district string, err error) {
	return s.city, s.district, s.err
}

func sp(s string) *string { return &s }

func TestGoshipProvider_Quote_MapsRates(t *testing.T) {
	cli := stubGoship{rates: []goship.Rate{
		{ID: "r1", Carrier: "ghnv3", CarrierName: "GHN", FeeVND: 25000, ETA: "2 ngày"},
		{ID: "r2", Carrier: "ghtk", CarrierName: "GHTK", FeeVND: 20000, ETA: "3 ngày"},
	}}
	d := weight.Defaults{WeightG: 500, LengthCM: 20, WidthCM: 15, HeightCM: 10}
	p := NewGoshipProvider(cli, stubPickup{city: "100000", district: "100100"}, d)

	opts, err := p.Quote(context.Background(), CalcReq{
		BrandID:    uuid.New(),
		ToCityCode: sp("200000"),
		ToDistrict: sp("200100"),
		Items:      []CalcItem{{Qty: 1}},
	})
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if len(opts) != 2 || opts[0].Carrier != "ghnv3" || opts[0].AmountVND != 25000 {
		t.Fatalf("unexpected options: %+v", opts)
	}
}

func TestGoshipProvider_Quote_MissingDestCodes(t *testing.T) {
	p := NewGoshipProvider(stubGoship{}, stubPickup{city: "100000", district: "100100"}, weight.Defaults{})
	_, err := p.Quote(context.Background(), CalcReq{BrandID: uuid.New(), Items: []CalcItem{{Qty: 1}}})
	if err == nil {
		t.Fatal("expected error when destination codes are missing")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/shipping/provider/ -run TestGoshipProvider -v`
Expected: FAIL — undefined `NewGoshipProvider`, `GoshipDeps`, `PickupRepo`.

- [ ] **Step 3: Write `goship_provider.go`**

```go
package provider

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
	shippingdomain "github.com/wearwhere/wearwhere_be/internal/shipping/domain"
	"github.com/wearwhere/wearwhere_be/internal/shipping/weight"
)

// ErrDestinationIncomplete is returned when the customer address lacks codes.
var ErrDestinationIncomplete = errors.New("shipping: destination missing city/district code")

// ErrPickupIncomplete is returned when the brand's pickup address lacks codes.
var ErrPickupIncomplete = errors.New("shipping: brand pickup address missing city/district code")

// PickupRepo returns the brand's primary pickup address location codes (Goship string codes).
type PickupRepo interface {
	PrimaryAddressCodes(ctx context.Context, brandID uuid.UUID) (cityCode, districtCode string, err error)
}

// GoshipDeps groups the goship provider's collaborators for the factory.
type GoshipDeps struct {
	Client     goship.Client
	PickupRepo PickupRepo
	Defaults   weight.Defaults
}

type GoshipProvider struct {
	client   goship.Client
	pickup   PickupRepo
	defaults weight.Defaults
}

func NewGoshipProvider(c goship.Client, p PickupRepo, d weight.Defaults) *GoshipProvider {
	return &GoshipProvider{client: c, pickup: p, defaults: d}
}

func (p *GoshipProvider) Quote(ctx context.Context, r CalcReq) ([]shippingdomain.ShippingOption, error) {
	if r.ToCityCode == nil || r.ToDistrict == nil {
		return nil, ErrDestinationIncomplete
	}
	fromCity, fromDist, err := p.pickup.PrimaryAddressCodes(ctx, r.BrandID)
	if err != nil {
		return nil, err
	}
	if fromCity == "" || fromDist == "" {
		return nil, ErrPickupIncomplete
	}

	wItems := make([]weight.Item, 0, len(r.Items))
	for _, it := range r.Items {
		wItems = append(wItems, weight.Item{
			Qty: it.Qty, WeightG: it.WeightG,
			LengthCM: it.LengthCM, WidthCM: it.WidthCM, HeightCM: it.HeightCM,
		})
	}
	parcel := weight.Aggregate(wItems, p.defaults)

	rates, err := p.client.Rates(ctx, goship.RateReq{
		From: goship.Address{CityCode: fromCity, DistrictCode: fromDist},
		To:   goship.Address{CityCode: *r.ToCityCode, DistrictCode: *r.ToDistrict},
		Parcel: goship.Parcel{
			WeightG: parcel.WeightG, LengthCM: parcel.LengthCM, WidthCM: parcel.WidthCM, HeightCM: parcel.HeightCM,
			CODVND: r.CODVND, AmountVND: r.AmountVND,
		},
	})
	if err != nil {
		return nil, err
	}
	opts := make([]shippingdomain.ShippingOption, 0, len(rates))
	for _, rt := range rates {
		opts = append(opts, shippingdomain.ShippingOption{
			Carrier: rt.Carrier, CarrierName: rt.CarrierName,
			Service: rt.Service, AmountVND: rt.FeeVND, ETA: rt.ETA,
		})
	}
	return opts, nil
}
```

- [ ] **Step 4: Implement `PrimaryAddressCodes` on the brand address repo**

In the brand address repo (search: `grep -rln "brand_addresses" internal/brand`), add:
```go
func (r *BrandAddressPG) PrimaryAddressCodes(ctx context.Context, brandID uuid.UUID) (string, string, error) {
	var city, district *string
	err := r.pool.QueryRow(ctx,
		`SELECT city_code, district_code FROM brand_addresses
		  WHERE brand_id = $1 AND is_primary = TRUE AND deleted_at IS NULL
		  LIMIT 1`, brandID).Scan(&city, &district)
	if err != nil {
		return "", "", err
	}
	c, d := "", ""
	if city != nil { c = *city }
	if district != nil { d = *district }
	return c, d, nil
}
```
> Match the actual repo's struct name and pool field (`r.pool` / `r.db`). If brand addresses live behind a different repo type, attach the method there.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/shipping/... -v`
Expected: PASS (provider, weight, goship mock).

- [ ] **Step 6: Commit**

```bash
git add internal/shipping/provider/goship_provider.go internal/shipping/provider/goship_provider_test.go internal/brand
git commit -m "feat(shipping): GoshipProvider maps rates to options + brand pickup codes lookup"
```

---

## Phase 6 — Location Endpoints

### Task 10: Location service with TTL cache ✅ DONE (d0dc2ff)

**Files:**
- Create: `internal/shipping/location/service.go`
- Create: `internal/shipping/location/service_test.go`

- [ ] **Step 1: Write the failing test**

```go
package location

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
)

type countingClient struct{ calls atomic.Int64 }

func (c *countingClient) Cities(context.Context) ([]goship.Location, error) {
	c.calls.Add(1)
	return []goship.Location{{Code: "100000", Name: "HCM"}}, nil
}
func (c *countingClient) Districts(context.Context, string) ([]goship.Location, error) { return nil, nil }
func (c *countingClient) Wards(context.Context, string) ([]goship.Location, error)     { return nil, nil }
func (c *countingClient) Rates(context.Context, goship.RateReq) ([]goship.Rate, error) { return nil, nil }

func TestService_Cities_CachedWithinTTL(t *testing.T) {
	cc := &countingClient{}
	s := NewService(cc, time.Hour)
	for i := 0; i < 3; i++ {
		if _, err := s.Cities(context.Background()); err != nil {
			t.Fatalf("Cities: %v", err)
		}
	}
	if got := cc.calls.Load(); got != 1 {
		t.Errorf("client called %d times, want 1 (cached)", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/shipping/location/ -v`
Expected: FAIL — undefined `NewService`.

- [ ] **Step 3: Write `service.go`**

```go
package location

import (
	"context"
	"sync"
	"time"

	"github.com/wearwhere/wearwhere_be/internal/shipping/goship"
)

type cacheEntry struct {
	data    []goship.Location
	expires time.Time
}

type Service struct {
	client goship.Client
	ttl    time.Duration
	mu     sync.Mutex
	cache  map[string]cacheEntry
}

func NewService(c goship.Client, ttl time.Duration) *Service {
	return &Service{client: c, ttl: ttl, cache: map[string]cacheEntry{}}
}

func (s *Service) get(ctx context.Context, key string, load func(context.Context) ([]goship.Location, error)) ([]goship.Location, error) {
	s.mu.Lock()
	if e, ok := s.cache[key]; ok && time.Now().Before(e.expires) {
		s.mu.Unlock()
		return e.data, nil
	}
	s.mu.Unlock()

	data, err := load(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cache[key] = cacheEntry{data: data, expires: time.Now().Add(s.ttl)}
	s.mu.Unlock()
	return data, nil
}

func (s *Service) Cities(ctx context.Context) ([]goship.Location, error) {
	return s.get(ctx, "cities", s.client.Cities)
}

func (s *Service) Districts(ctx context.Context, cityCode string) ([]goship.Location, error) {
	return s.get(ctx, "d:"+cityCode, func(c context.Context) ([]goship.Location, error) {
		return s.client.Districts(c, cityCode)
	})
}

func (s *Service) Wards(ctx context.Context, districtCode string) ([]goship.Location, error) {
	return s.get(ctx, "w:"+districtCode, func(c context.Context) ([]goship.Location, error) {
		return s.client.Wards(c, districtCode)
	})
}
```
> `time.Now()` is fine here (production code, not a workflow script).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/shipping/location/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/shipping/location/service.go internal/shipping/location/service_test.go
git commit -m "feat(location): cached cities/districts/wards service over goship client"
```

---

### Task 11: Location HTTP handlers + routes ✅ DONE (fdcaaa1)

**Files:**
- Create: `internal/shipping/location/handler.go`
- Create: `internal/shipping/location/routes.go`

- [ ] **Step 1: Write `handler.go`**

> Match the existing handler conventions (error envelope, gin binding) — open `internal/order/handler/handler.go` to copy the response/error helper style used project-wide. Below uses a plain `gin.H`; replace with the project's standard envelope if one exists.

```go
package location

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct{ svc *Service }

func NewHandler(s *Service) *Handler { return &Handler{svc: s} }

func (h *Handler) Cities(c *gin.Context) {
	out, err := h.svc.Cities(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to load cities"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func (h *Handler) Districts(c *gin.Context) {
	code := c.Param("city_code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing city_code"})
		return
	}
	out, err := h.svc.Districts(c.Request.Context(), code)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to load districts"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func (h *Handler) Wards(c *gin.Context) {
	code := c.Param("district_code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing district_code"})
		return
	}
	out, err := h.svc.Wards(c.Request.Context(), code)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to load wards"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}
```

- [ ] **Step 2: Write `routes.go`**

```go
package location

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts location endpoints under the given authenticated group.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler) {
	loc := rg.Group("/locations")
	loc.GET("/cities", h.Cities)
	loc.GET("/cities/:city_code/districts", h.Districts)
	loc.GET("/districts/:district_code/wards", h.Wards)
}
```
> Mount under the same authenticated `/api/v1` group used by other `/me` routes. Check `cmd/api/main.go` for the exact group variable and auth middleware, and call `location.RegisterRoutes(authedGroup, locHandler)` there (done in Task 16).

- [ ] **Step 3: Build**

Run: `go build ./internal/shipping/location/`
Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/shipping/location/handler.go internal/shipping/location/routes.go
git commit -m "feat(location): GET cities/districts/wards endpoints"
```

---

## Phase 7 — Checkout & Order Integration

### Task 12: DTO + error additions ✅ DONE (59eea10, with Task 13)

**Files:**
- Modify: `internal/order/domain/dto.go`
- Modify: `internal/order/domain/errors.go`

- [ ] **Step 1: Add option/selection DTOs to `dto.go`**

Add a shipping option type and embed options into the preview sub-order:
```go
type ShippingOptionResp struct {
	Carrier     string `json:"carrier"`
	CarrierName string `json:"carrier_name"`
	Service     string `json:"service"`
	AmountVND   int64  `json:"amount_vnd"`
	ETA         string `json:"eta"`
}
```
Add to `CheckoutPreviewSubOrder` (after `TotalVND`):
```go
	ShippingOptions []ShippingOptionResp `json:"shipping_options"`
```
Add to `CheckoutPreviewResp` (after `Warnings`):
```go
	AddressIncomplete bool `json:"address_incomplete"`
```
Add the per-brand carrier selection to `PlaceOrderReq`:
```go
type ShippingSelection struct {
	BrandID uuid.UUID `json:"brand_id" binding:"required"`
	Carrier string    `json:"carrier" binding:"required"`
}

type PlaceOrderReq struct {
	AddressID          uuid.UUID           `json:"address_id" binding:"required"`
	PaymentMethod      PaymentMethod       `json:"payment_method" binding:"required"`
	Notes              string              `json:"notes" binding:"max=500"`
	ShippingSelections []ShippingSelection `json:"shipping_selections" binding:"required,dive"`
}
```

- [ ] **Step 2: Add errors to `errors.go`**

```go
var (
	ErrAddressIncomplete  = errors.New("shipping address is missing city/district/ward code")
	ErrCarrierUnavailable = errors.New("selected carrier is no longer available for this route")
	ErrCarrierNotSelected = errors.New("no shipping carrier selected for one or more brands")
	ErrShippingUnavailable = errors.New("shipping service temporarily unavailable")
)
```
> If `errors` isn't imported in `errors.go`, add it.

- [ ] **Step 3: Build**

Run: `go build ./internal/order/...`
Expected: FAIL — `checkout_service.go`/`order_service.go` still call removed `Calculate`. That's expected; fixed in Tasks 13–14. (Do not commit yet.)

---

### Task 13: Checkout preview uses Quote + address-incomplete gate ✅ DONE (59eea10)

**Files:**
- Modify: `internal/order/service/checkout_service.go`
- Test: `internal/order/service/checkout_service_test.go` (extend existing)

- [ ] **Step 1: Write/extend the failing test**

Add a test that a complete address yields per-brand options and an incomplete one sets `AddressIncomplete`. Use the existing test's fakes for cart/addr repos and inject a stub provider:
```go
type stubProvider struct{ opts []shippingdomain.ShippingOption; err error }
func (s stubProvider) Quote(context.Context, provider.CalcReq) ([]shippingdomain.ShippingOption, error) {
	return s.opts, s.err
}

func TestPreview_ReturnsCarrierOptions(t *testing.T) {
	// address WITH codes (city_code/district_code set on the fake CustomerAddress)
	sp := stubProvider{opts: []shippingdomain.ShippingOption{
		{Carrier: "ghnv3", CarrierName: "GHN", AmountVND: 25000, ETA: "2 ngày"},
		{Carrier: "ghtk", CarrierName: "GHTK", AmountVND: 20000, ETA: "3 ngày"},
	}}
	svc := NewCheckoutService(fakeCart, fakeAddrWithCodes, sp)
	resp, err := svc.Preview(context.Background(), userID, addrID)
	if err != nil { t.Fatal(err) }
	if resp.AddressIncomplete { t.Fatal("should not be incomplete") }
	if len(resp.SubOrders[0].ShippingOptions) != 2 { t.Fatalf("want 2 options") }
}

func TestPreview_AddressIncompleteWhenNoCodes(t *testing.T) {
	svc := NewCheckoutService(fakeCart, fakeAddrNoCodes, stubProvider{})
	resp, err := svc.Preview(context.Background(), userID, addrID)
	if err != nil { t.Fatal(err) }
	if !resp.AddressIncomplete { t.Fatal("want AddressIncomplete=true") }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/order/service/ -run TestPreview -v`
Expected: FAIL — compile error (`Quote` not used yet) / assertions fail.

- [ ] **Step 3: Modify `Preview`**

After loading `addr`, snapshot codes and check completeness:
```go
	shipAddr := domain.ShippingAddress{
		Recipient: addr.RecipientName, Phone: addr.RecipientPhone, Line1: addr.AddressLine,
		Ward: addr.Ward, District: addr.District, City: addr.City,
		CityCode: addr.CityCode, DistrictCode: addr.DistrictCode, WardCode: addr.WardCode,
	}
	addrIncomplete := addr.CityCode == nil || addr.DistrictCode == nil || addr.WardCode == nil
```
In the per-brand loop, when incomplete skip the provider call and leave options empty; otherwise call `Quote`:
```go
		var options []domain.ShippingOptionResp
		var cheapest int64
		if !addrIncomplete {
			opts, err := s.shipping.Quote(ctx, provider.CalcReq{
				BrandID:    bID,
				ToAddress:  toShippingProviderAddr(shipAddr),
				ToCityCode: addr.CityCode,
				ToDistrict: addr.DistrictCode,
				CODVND:     0,          // preview: payment method not chosen yet — estimate as prepaid
				AmountVND:  g.subtotal, // declared goods value
				Items:      toCalcItems(g.items), // map preview items -> provider.CalcItem (qty + variant dims)
			})
			if err != nil {
				return nil, fmt.Errorf("shipping quote for brand %s: %w", bID, err)
			}
			for i, o := range opts {
				options = append(options, domain.ShippingOptionResp{
					Carrier: o.Carrier, CarrierName: o.CarrierName, Service: o.Service,
					AmountVND: o.AmountVND, ETA: o.ETA,
				})
				if i == 0 || o.AmountVND < cheapest {
					cheapest = o.AmountVND
				}
			}
		}
		subOrders = append(subOrders, domain.CheckoutPreviewSubOrder{
			Brand: g.brand, Items: g.items, SubtotalVND: g.subtotal,
			ShippingFeeVND:  cheapest, // indicative (cheapest) until customer chooses
			TotalVND:        g.subtotal + cheapest,
			ShippingOptions: options,
		})
		shippingAll += cheapest
```
Set `AddressIncomplete: addrIncomplete` on the returned `CheckoutPreviewResp` (both the empty-cart and normal returns). Add a `toCalcItems` helper that maps preview items to `provider.CalcItem` (the preview item carries `VariantID`/`Qty`; fetch variant dims via the cart view if present, else leave dimension pointers nil so defaults apply).
> If the cart view doesn't expose variant weight/dims, leave them nil here — the provider's defaults handle it. Real dims flow through in `order_service` Step (Task 14) where the snapshot query can `SELECT v.weight_g, ...`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/order/service/ -run TestPreview -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/order/domain/dto.go internal/order/domain/errors.go internal/order/service/checkout_service.go internal/order/service/checkout_service_test.go
git commit -m "feat(checkout): preview returns per-brand carrier options + address-incomplete gate"
```

---

### Task 14: PlaceOrder re-quotes by chosen carrier and stores shipping_carrier ✅ DONE (52736aa + fix 8b24d54) — full build GREEN; main.go factory wiring also done here

> Note: Task 14 already wired `cmd/api/main.go` GoshipDeps into `provider.NewFromConfig` (needed for build). Task 16 remaining work = location routes mount + inject location.Service into address services.

**Files:**
- Modify: `internal/order/service/order_service.go`
- Modify: `internal/order/domain/order.go` (`SubOrder` add `ShippingCarrier *string`)
- Modify: `internal/order/repo/sub_order_pg.go` (persist `shipping_carrier` + `shipping_provider`)
- Test: `internal/order/service/order_service_test.go` (integration, extend existing)

- [ ] **Step 1: Add `ShippingCarrier` to the `SubOrder` struct and its INSERT**

In `order.go`, add `ShippingCarrier *string` and `ShippingProvider *string` to `SubOrder`. In `sub_order_pg.go`'s `Create`, add `shipping_carrier, shipping_provider` to the column list, placeholders, and args.

- [ ] **Step 2: Write the failing integration test**

In `order_service_test.go` (build tag `integration`), add a case: seed a brand pickup address with codes + a customer address with codes, place an order with `ShippingSelections: [{BrandID, Carrier: "ghnv3"}]` against the **mock** goship provider (the mock returns carriers `ghnv3`/`ghtk`/`vtp`), assert the persisted sub-order has `shipping_carrier = 'ghnv3'` and `shipping_fee_vnd` equal to the mock's ghnv3 fee; and a case where an unknown carrier (e.g. `"ghn"`) yields `ErrCarrierUnavailable`.
```go
func TestPlaceOrder_Goship_StoresChosenCarrierFee(t *testing.T) {
	// ... existing integration harness setup, but construct OrderService with a
	// GoshipProvider backed by goship.NewMockClient() and a pickup repo returning codes.
	req := domain.PlaceOrderReq{
		AddressID:     addrID,
		PaymentMethod: domain.PaymentMethodCOD,
		ShippingSelections: []domain.ShippingSelection{{BrandID: brandID, Carrier: "ghnv3"}},
	}
	orderResp, _, err := svc.PlaceOrder(ctx, userID, req)
	if err != nil { t.Fatalf("PlaceOrder: %v", err) }
	// assert sub_order row shipping_carrier='ghn' and fee matches mock ghn fee
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test -tags integration -p 1 ./internal/order/service/ -run TestPlaceOrder_Goship -v`
Expected: FAIL — selection not honored / column not stored.

- [ ] **Step 4: Modify Step 2 of PlaceOrder to snapshot address codes**

In the `shipAddr := domain.ShippingAddress{...}` block (~line 127), add the three codes (`CityCode: addr.CityCode, ...`). Add an early guard:
```go
	if addr.CityCode == nil || addr.DistrictCode == nil || addr.WardCode == nil {
		return nil, nil, domain.ErrAddressIncomplete
	}
```

- [ ] **Step 5: Add variant dims to the cart snapshot query + row struct**

Extend the Step 5 `SELECT` to include `v.weight_g, v.length_cm, v.width_cm, v.height_cm` and add matching `*int` fields to `cartSnapshotRow` + `rows.Scan(...)`.

- [ ] **Step 6: Replace the Step 7 shipping loop with carrier re-quote**

Build a selection map and replace the `s.shipping.Calculate(...)` loop:
```go
	selByBrand := map[uuid.UUID]string{}
	for _, sel := range req.ShippingSelections {
		selByBrand[sel.BrandID] = sel.Carrier
	}
	var shippingAll int64
	for _, bID := range brandOrder {
		g := groups[bID]
		chosen, ok := selByBrand[g.brandID]
		if !ok {
			return nil, nil, domain.ErrCarrierNotSelected
		}
		items := make([]provider.CalcItem, 0, len(g.rows))
		for _, r := range g.rows {
			items = append(items, provider.CalcItem{
				VariantID: r.VariantID, ProductID: r.ProductID, Qty: r.Qty,
				WeightG: r.WeightG, LengthCM: r.LengthCM, WidthCM: r.WidthCM, HeightCM: r.HeightCM,
			})
		}
		// COD: the carrier collects this brand's goods value on delivery; PayOS is prepaid (0).
		// Using sub-order subtotal (not incl. shipping) avoids a circular dependency on the
		// fee we are about to compute. Refined in Spec B at shipment-creation time.
		var codVND int64
		if req.PaymentMethod == domain.PaymentMethodCOD {
			codVND = g.subtotal
		}
		opts, err := s.shipping.Quote(ctx, provider.CalcReq{
			BrandID:    g.brandID,
			ToAddress:  provider.ShippingAddress{Recipient: shipAddr.Recipient, Phone: shipAddr.Phone, Line1: shipAddr.Line1, Ward: shipAddr.Ward, District: shipAddr.District, City: shipAddr.City},
			ToCityCode: shipAddr.CityCode,
			ToDistrict: shipAddr.DistrictCode,
			CODVND:     codVND,
			AmountVND:  g.subtotal,
			Items:      items,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("%w: brand %s: %v", domain.ErrShippingUnavailable, g.brandID, err)
		}
		var matched *shippingdomain.ShippingOption
		for i := range opts {
			if opts[i].Carrier == chosen {
				matched = &opts[i]
				break
			}
		}
		if matched == nil {
			return nil, nil, domain.ErrCarrierUnavailable
		}
		g.shipping = matched.AmountVND
		g.carrier = matched.Carrier // add `carrier string` to brandGroup struct
		shippingAll += matched.AmountVND
	}
```
Add `carrier string` to the `brandGroup` struct (Step 7 type) and import `shippingdomain "github.com/wearwhere/wearwhere_be/internal/shipping/domain"`.

- [ ] **Step 7: Persist the carrier in the Step 10 sub-order creation**

In the `so := &domain.SubOrder{...}` literal, add:
```go
			ShippingCarrier:  strPtr(g.carrier),
			ShippingProvider: strPtr("goship"),
```
Add a `strPtr` helper if not present: `func strPtr(s string) *string { return &s }`.
> When `SHIPPING_PROVIDER=flat`, `g.carrier` is `"flat"` — store that; the column simply records which provider/carrier produced the fee.

- [ ] **Step 8: Run integration tests**

Run: `go test -tags integration -p 1 ./internal/order/service/ -run TestPlaceOrder -v`
Expected: PASS (new Goship cases + existing place-order cases still green).

- [ ] **Step 9: Commit**

```bash
git add internal/order
git commit -m "feat(order): re-quote chosen carrier at place-order; store shipping_carrier/provider"
```

---

### Task 15: Address create/update accept + validate location codes ✅ DONE (a7e47db) — customer + brand; repo now passes req codes (resolves Task 2 COALESCE note)

**Files:**
- Modify: customer address request DTO + service create/update (search: `grep -rln "RecipientName" internal/customeraddr`)
- Modify: brand address create/update (search: `grep -rln "brand_addresses" internal/brand`)
- Test: customer address service test (extend existing)

- [ ] **Step 1: Add code fields to the create/update request DTOs**

Add to both customer and brand address request structs (Goship codes are strings):
```go
	CityCode     *string `json:"city_code" binding:"required"`
	DistrictCode *string `json:"district_code" binding:"required"`
	WardCode     *string `json:"ward_code" binding:"required"`
```
> Marking them `required` enforces structured addresses on all NEW/updated addresses (best-practice gate). Legacy rows stay null until edited.

- [ ] **Step 2: Write the failing test (consistency validation)**

```go
func TestCreateAddress_RejectsInconsistentCodes(t *testing.T) {
	// district_code not belonging to city_code (validated via cached location lists)
	_, err := svc.Create(ctx, userID, req /* city=1, district=999 */)
	if !errors.Is(err, domain.ErrInvalidLocation) {
		t.Fatalf("want ErrInvalidLocation, got %v", err)
	}
}
```
Add `ErrInvalidLocation = errors.New("invalid location: district/ward does not belong to parent")` to the customer address domain errors.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/customeraddr/... -run TestCreateAddress_RejectsInconsistentCodes -v`
Expected: FAIL.

- [ ] **Step 4: Implement validation in the service**

Inject the `location.Service` into the address service. In `Create`/`Update`, after binding, validate the code hierarchy:
```go
	districts, err := s.loc.Districts(ctx, *req.CityCode)
	if err != nil { return nil, domain.ErrInvalidLocation }
	if !containsCode(districts, *req.DistrictCode) { return nil, domain.ErrInvalidLocation }
	wards, err := s.loc.Wards(ctx, *req.DistrictCode)
	if err != nil { return nil, domain.ErrInvalidLocation }
	if !containsCode(wards, *req.WardCode) { return nil, domain.ErrInvalidLocation }
```
Add `containsCode(list []goship.Location, code string) bool` (compares `l.Code == code`). Persist the three codes through to the repo (already wired in Task 2).

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/customeraddr/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/customeraddr internal/brand
git commit -m "feat(address): require + validate city/district/ward codes on create/update"
```

---

## Phase 8 — Wiring & End-to-End

### Task 16: Wire everything in main.go ✅ DONE (c221ec5 + factory part in 52736aa) — locSvc + routes + address-service injection

**Files:**
- Modify: `cmd/api/main.go`

- [ ] **Step 1: Construct the goship client, provider, and location service**

Near the existing shipping factory wiring (`grep -n "shipping" cmd/api/main.go`):
```go
	goshipClient, err := goship.NewFromConfig(goship.Config{
		Mode: cfg.Goship.Mode, Token: cfg.Goship.Token, BaseURL: cfg.Goship.BaseURL,
	})
	if err != nil { log.Fatalf("goship: %v", err) }

	locSvc := location.NewService(goshipClient, 24*time.Hour)

	shippingProvider, err := provider.NewFromConfig(
		provider.Config{Provider: cfg.Shipping.Provider},
		brandRepo,
		&provider.GoshipDeps{
			Client:     goshipClient,
			PickupRepo: brandAddrRepo, // implements PrimaryAddressCodes (Task 9 Step 4)
			Defaults: weight.Defaults{
				WeightG: cfg.Goship.DefaultItemWeightG, LengthCM: cfg.Goship.DefaultLengthCM,
				WidthCM: cfg.Goship.DefaultWidthCM, HeightCM: cfg.Goship.DefaultHeightCM,
			},
		},
	)
	if err != nil { log.Fatalf("shipping provider: %v", err) }
```
Replace the previous `provider.NewFromConfig(...)` call (which had two args) with the three-arg version above. Inject `locSvc` into the customer + brand address services (Task 15).

- [ ] **Step 2: Mount location routes**

After the authenticated `/api/v1` group is built and other `/me` routes registered:
```go
	location.RegisterRoutes(v1Authed, location.NewHandler(locSvc))
```
> Use the actual authenticated group variable name from main.go.

- [ ] **Step 3: Build the whole binary**

Run: `go build ./...`
Expected: success.

- [ ] **Step 4: Run the full unit suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Run the integration suite**

Run: `go test -tags integration -p 1 ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/api/main.go
git commit -m "feat(wiring): goship client + provider + location routes in main"
```

---

### Task 17: Manual sandbox smoke + spec contract reconciliation ✅ DONE (520f0a8) — verified against live api.goship.io; client_http.go needed NO change; carrier=display name (no short code) → mock now mirrors prod; spec §11 updated with confirmed contract. (Token in gitignored .env; not committed.)

> **Post-implementation review notes (tracked, not blocking Spec A):**
> - **COD amount (Important):** `order_service.go` sends `codVND = g.subtotal` (merchandise only, not incl. shipping) to Goship rates. Deliberate Spec A simplification (circular: need fee to know COD). Refine in Spec B at shipment-creation (two-pass quote, or subtotal+fee). 
> - **Minor:** `containsCode` duplicated in customeraddr/brand services (could extract); `toCalcItems` in checkout preview uses default dims (preview vs place fee may differ slightly for variants with real dims); location cache has a benign cold-miss stampede (24h TTL → startup-only). 
> - Fixed during review: provider-name now persisted accurately (8b24d54); pgx.ErrNoRows→ErrPickupIncomplete (6221771); address codes optional on UPDATE to preserve PATCH semantics (d8b991b).

**Files:**
- Modify (if needed): `internal/shipping/goship/client_http.go` (field/path fixes)
- Modify: `docs/superpowers/specs/2026-06-03-goship-shipping-rates-design.md` §11 (record confirmed contract)

- [ ] **Step 1: Set sandbox env and run the real test**

```bash
export GOSHIP_MODE=sandbox
export GOSHIP_TOKEN=<sandbox token>
go test -tags goship_real ./internal/shipping/goship/ -run TestRealGoship_Cities -v
```
Expected: PASS, logs real city list. If JSON shape differs, fix `client_http.go` tags/paths and re-run.

- [ ] **Step 2: Discover real codes and run the rates test**

From the cities/districts output, pick a HCMC→Hanoi district/city pair, set `GOSHIP_TEST_FROM_*`/`TO_*`, and run `TestRealGoship_Rates`.
Expected: ≥1 carrier returned. The test logs `carrier`/`carrier_name` per rate — confirm whether a short code is present (if not, selection keys on `carrier_name`, already handled in `client_http.go`). Align the mock fixture in `client_mock.go` (`ghnv3`/`ghtk`/`vtp`) with the real carrier codes returned.

- [ ] **Step 3: Record the confirmed contract in the spec §11**

Note: exact endpoint paths, auth scheme (static token vs login), JSON field names, carrier code list, and the volumetric divisor used by the sandbox carriers. Replace the "pinned at implementation" wording with the confirmed values.

- [ ] **Step 4: Commit**

```bash
git add internal/shipping/goship docs/superpowers/specs/2026-06-03-goship-shipping-rates-design.md
git commit -m "fix(goship): reconcile HTTP client with sandbox contract; document confirmed API"
```

---

## Definition of Done (Spec A) — ✅ ALL MET

- [x] Migrations 000027–000030 applied; address tables have codes, variants have dims, sub_orders has `shipping_carrier`.
- [x] `GOSHIP_*` config loads with sane defaults; `SHIPPING_PROVIDER=goship` selects the Goship provider.
- [x] `go test ./...` and `go test -tags integration -p 1 ./...` both green (full module).
- [x] `GET /api/v1/locations/{cities,cities/:c/districts,districts/:d/wards}` return data (cached).
- [x] Checkout preview returns per-brand carrier options; incomplete address sets `address_incomplete` and blocks place-order.
- [x] Place-order re-quotes by chosen carrier, stores authoritative `shipping_fee_vnd` + `shipping_carrier`; unknown carrier → `ErrCarrierUnavailable`.
- [x] Live `Cities`/`Rates` confirmed against api.goship.io; spec §11 updated with the confirmed contract (carrier=display name; no short code).
- [ ] Spec B (fulfillment: brand confirm/ship/deliver, shipment creation, tracking webhook, cancel) remains out of scope and is filed as the next plan.
```
