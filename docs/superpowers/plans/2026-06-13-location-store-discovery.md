# Location & Store Discovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the customer-facing Location & Store Discovery API (SRS UC23–26): find nearby stores, search stores by area, view store detail, and get directions — backed by the existing `brand_addresses` data and a server-side Goong Maps proxy.

**Architecture:** Two new modules. `internal/maps/goong/` is a Goong adapter mirroring `internal/shipping/goship/` (interface + HTTP client + deterministic mock + factory). `internal/store/` is a read-only public discovery module (domain → repo → service → handler) that treats public `brand_addresses` rows with coordinates as stores, joins brand identity + new `store_hours`, pre-filters nearby candidates with a SQL haversine query, then enriches real road distance/ETA via Goong Distance Matrix and proxies Goong Directions.

**Tech Stack:** Go, Gin, pgx/v5, PostgreSQL, golang-migrate. Patterns copied from existing `goship`/`location`/`catalog` modules.

**Spec:** `docs/superpowers/specs/2026-06-13-location-store-discovery-design.md`

**Branch:** `feature/location-store-discovery` (already created off `main`).

---

## File Structure

**New — Goong adapter (`internal/maps/goong/`):**
- `client.go` — `Client` interface + request/result types + sentinel errors
- `client_mock.go` — deterministic mock client (no network)
- `client_mock_test.go` — mock determinism tests
- `client_http.go` — real Goong HTTP client (reads API key)
- `client_http_real_test.go` — env-gated live test (skips without `GOONG_API_KEY`)
- `factory.go` — `NewFromConfig` selecting `mock|production`
- `factory_test.go`

**New — store module (`internal/store/`):**
- `domain/store.go` — `Store`, `StoreHours`, `OpenStatus`, `Haversine`, `ComputeOpenStatus`
- `domain/store_test.go` — haversine + open/closed unit tests
- `domain/dto.go` — response DTOs + converters
- `domain/errors.go` — sentinel errors
- `repo/repo.go` — `DBTX`, `Repo` interface, `ErrNotFound`
- `repo/store_pg.go` — Postgres queries (nearby, search, detail, hours)
- `repo/store_pg_test.go` — repo tests using existing testfixtures
- `service/service.go` — orchestration (nearby, search, detail, directions, degrade)
- `service/service_test.go` — service tests with mock Goong
- `handler/handler.go` — Gin handlers
- `handler/routes.go` — `MountStoresPublic`
- `handler/handler_test.go` — handler tests

**Modified:**
- `internal/config/config.go` — add `GoongConfig` + load block
- `internal/config/config_test.go` — Goong defaults test
- `cmd/api/main.go` — construct Goong client + store service + mount routes
- `cmd/api/main_test.go` — wire store routes for integration tests
- `db/migrations/000033_create_store_hours.up.sql` / `.down.sql` — new table

> **Migration number:** This branch is off `main` (highest migration `000032`), so `000033` is free here. Note the parked `ai-stylist-chatbot` branch also uses `000033`/`000034`; flag for renumbering at eventual integration (already noted in the spec).

---

## Task 1: `store_hours` migration

**Files:**
- Create: `db/migrations/000033_create_store_hours.up.sql`
- Create: `db/migrations/000033_create_store_hours.down.sql`

- [ ] **Step 1: Write the up migration**

`db/migrations/000033_create_store_hours.up.sql`:
```sql
CREATE TABLE store_hours (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    brand_address_id  UUID NOT NULL REFERENCES brand_addresses(id) ON DELETE CASCADE,
    weekday           SMALLINT NOT NULL CHECK (weekday BETWEEN 0 AND 6), -- 0=Sunday .. 6=Saturday
    open_time         TIME NOT NULL,
    close_time        TIME NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_store_hours_addr ON store_hours (brand_address_id, weekday);
```

- [ ] **Step 2: Write the down migration**

`db/migrations/000033_create_store_hours.down.sql`:
```sql
DROP TABLE IF EXISTS store_hours;
```

- [ ] **Step 3: Run migration up to verify it applies**

Run: `migrate -path db/migrations -database "$DATABASE_URL" up` (or the project's `make migrate-up` if present — check the Makefile).
Expected: applies `000033` with no error; `\d store_hours` shows the table.

- [ ] **Step 4: Commit**

```bash
git add db/migrations/000033_create_store_hours.up.sql db/migrations/000033_create_store_hours.down.sql
git commit -m "feat(db): store_hours table for store discovery"
```

---

## Task 2: Goong adapter — interface, types, errors

**Files:**
- Create: `internal/maps/goong/client.go`

- [ ] **Step 1: Write the interface, types, and sentinel errors**

`internal/maps/goong/client.go`:
```go
// Package goong is a server-side adapter for the Goong Maps API
// (geocoding, distance matrix, directions). Mirrors internal/shipping/goship.
package goong

import (
	"context"
	"errors"
)

var (
	ErrGeocode  = errors.New("goong: failed to geocode")
	ErrDistance = errors.New("goong: failed to fetch distance matrix")
	ErrDirections = errors.New("goong: failed to fetch directions")
)

// LatLng is a WGS84 coordinate.
type LatLng struct {
	Lat float64
	Lng float64
}

// GeocodeResult is one candidate for a geocoded query.
type GeocodeResult struct {
	Lat              float64
	Lng              float64
	FormattedAddress string
}

// DistanceResult is the road distance/duration from one origin to one destination.
type DistanceResult struct {
	DistanceM int64 // meters
	DurationS int64 // seconds
}

// Route is a single computed route for directions.
type Route struct {
	DistanceM int64
	DurationS int64
	Polyline  string // encoded polyline for the client to render
}

type Client interface {
	Geocode(ctx context.Context, query string) ([]GeocodeResult, error)
	// DistanceMatrix returns one DistanceResult per destination, in the same
	// order as dests. Used to enrich nearby candidates with real road distance.
	DistanceMatrix(ctx context.Context, origin LatLng, dests []LatLng) ([]DistanceResult, error)
	Directions(ctx context.Context, origin, dest LatLng) (Route, error)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/maps/goong/...`
Expected: builds with no error.

- [ ] **Step 3: Commit**

```bash
git add internal/maps/goong/client.go
git commit -m "feat(goong): client interface, types, sentinel errors"
```

---

## Task 3: Goong mock client

**Files:**
- Create: `internal/maps/goong/client_mock.go`
- Test: `internal/maps/goong/client_mock_test.go`

- [ ] **Step 1: Write the failing test**

`internal/maps/goong/client_mock_test.go`:
```go
package goong

import (
	"context"
	"testing"
)

func TestMockClient_DistanceMatrix_OnePerDest(t *testing.T) {
	m := NewMockClient()
	origin := LatLng{Lat: 10.776, Lng: 106.700}
	dests := []LatLng{{Lat: 10.78, Lng: 106.70}, {Lat: 10.80, Lng: 106.71}}
	got, err := m.DistanceMatrix(context.Background(), origin, dests)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != len(dests) {
		t.Fatalf("got %d results, want %d", len(got), len(dests))
	}
	// Farther destination must have a larger distance (monotonic, deterministic).
	if got[1].DistanceM <= got[0].DistanceM {
		t.Errorf("expected dest1 farther than dest0: %d vs %d", got[1].DistanceM, got[0].DistanceM)
	}
}

func TestMockClient_Geocode_NonEmpty(t *testing.T) {
	m := NewMockClient()
	got, err := m.Geocode(context.Background(), "Quận 1, Hồ Chí Minh")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one geocode result")
	}
}

func TestMockClient_Directions_Positive(t *testing.T) {
	m := NewMockClient()
	r, err := m.Directions(context.Background(), LatLng{10.77, 106.70}, LatLng{10.80, 106.71})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if r.DistanceM <= 0 || r.DurationS <= 0 || r.Polyline == "" {
		t.Errorf("expected positive route with polyline, got %+v", r)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/maps/goong/ -run TestMockClient -v`
Expected: FAIL — `NewMockClient` undefined.

- [ ] **Step 3: Write the mock implementation**

`internal/maps/goong/client_mock.go`:
```go
package goong

import (
	"context"
	"math"
)

// MockClient returns deterministic results derived from haversine distance,
// so tests are stable and ordering is realistic. No network calls.
type MockClient struct{}

func NewMockClient() *MockClient { return &MockClient{} }

// haversineM returns straight-line meters between two coordinates.
func haversineM(a, b LatLng) int64 {
	const earthM = 6371000.0
	la1 := a.Lat * math.Pi / 180
	la2 := b.Lat * math.Pi / 180
	dLa := (b.Lat - a.Lat) * math.Pi / 180
	dLo := (b.Lng - a.Lng) * math.Pi / 180
	h := math.Sin(dLa/2)*math.Sin(dLa/2) +
		math.Cos(la1)*math.Cos(la2)*math.Sin(dLo/2)*math.Sin(dLo/2)
	return int64(2 * earthM * math.Asin(math.Sqrt(h)))
}

func (m *MockClient) Geocode(_ context.Context, query string) ([]GeocodeResult, error) {
	// Deterministic central HCMC point regardless of query text.
	return []GeocodeResult{{Lat: 10.7769, Lng: 106.7009, FormattedAddress: query}}, nil
}

func (m *MockClient) DistanceMatrix(_ context.Context, origin LatLng, dests []LatLng) ([]DistanceResult, error) {
	out := make([]DistanceResult, 0, len(dests))
	for _, d := range dests {
		straight := haversineM(origin, d)
		road := int64(float64(straight) * 1.3) // road factor
		out = append(out, DistanceResult{DistanceM: road, DurationS: road / 8})
	}
	return out, nil
}

func (m *MockClient) Directions(_ context.Context, origin, dest LatLng) (Route, error) {
	straight := haversineM(origin, dest)
	road := int64(float64(straight) * 1.3)
	return Route{DistanceM: road, DurationS: road / 8, Polyline: "mock_polyline"}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/maps/goong/ -run TestMockClient -v`
Expected: PASS (all three tests).

- [ ] **Step 5: Commit**

```bash
git add internal/maps/goong/client_mock.go internal/maps/goong/client_mock_test.go
git commit -m "feat(goong): deterministic mock client"
```

---

## Task 4: Goong HTTP client

**Files:**
- Create: `internal/maps/goong/client_http.go`
- Test: `internal/maps/goong/client_http_real_test.go`

Goong endpoints (REST, key as `api_key` query param, base `https://rsapi.goong.io`):
- Geocode: `GET /Geocode?address=<q>&api_key=<k>` → `{results:[{geometry:{location:{lat,lng}},formatted_address}]}`
- Distance Matrix: `GET /DistanceMatrix?origins=lat,lng&destinations=lat,lng|lat,lng&vehicle=car&api_key=<k>` → `{rows:[{elements:[{distance:{value},duration:{value}}]}]}`
- Directions: `GET /Direction?origin=lat,lng&destination=lat,lng&vehicle=car&api_key=<k>` → `{routes:[{legs:[{distance:{value},duration:{value}}],overview_polyline:{points}}]}`

- [ ] **Step 1: Write the HTTP client**

`internal/maps/goong/client_http.go`:
```go
package goong

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type HTTPClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewHTTPClient(apiKey, baseURL string) *HTTPClient {
	return &HTTPClient{
		apiKey:     apiKey,
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *HTTPClient) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	q.Set("api_key", c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("goong GET %s: status=%d body=%s", path, resp.StatusCode, string(b))
	}
	return json.Unmarshal(b, out)
}

func latLngStr(p LatLng) string {
	return strconv.FormatFloat(p.Lat, 'f', -1, 64) + "," + strconv.FormatFloat(p.Lng, 'f', -1, 64)
}

func (c *HTTPClient) Geocode(ctx context.Context, query string) ([]GeocodeResult, error) {
	var env struct {
		Results []struct {
			FormattedAddress string `json:"formatted_address"`
			Geometry         struct {
				Location struct {
					Lat float64 `json:"lat"`
					Lng float64 `json:"lng"`
				} `json:"location"`
			} `json:"geometry"`
		} `json:"results"`
	}
	q := url.Values{}
	q.Set("address", query)
	if err := c.getJSON(ctx, "/Geocode", q, &env); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrGeocode, err)
	}
	out := make([]GeocodeResult, 0, len(env.Results))
	for _, r := range env.Results {
		out = append(out, GeocodeResult{
			Lat: r.Geometry.Location.Lat, Lng: r.Geometry.Location.Lng,
			FormattedAddress: r.FormattedAddress,
		})
	}
	return out, nil
}

func (c *HTTPClient) DistanceMatrix(ctx context.Context, origin LatLng, dests []LatLng) ([]DistanceResult, error) {
	parts := make([]string, 0, len(dests))
	for _, d := range dests {
		parts = append(parts, latLngStr(d))
	}
	q := url.Values{}
	q.Set("origins", latLngStr(origin))
	q.Set("destinations", strings.Join(parts, "|"))
	q.Set("vehicle", "car")
	var env struct {
		Rows []struct {
			Elements []struct {
				Distance struct {
					Value int64 `json:"value"`
				} `json:"distance"`
				Duration struct {
					Value int64 `json:"value"`
				} `json:"duration"`
			} `json:"elements"`
		} `json:"rows"`
	}
	if err := c.getJSON(ctx, "/DistanceMatrix", q, &env); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDistance, err)
	}
	if len(env.Rows) == 0 {
		return nil, fmt.Errorf("%w: empty rows", ErrDistance)
	}
	els := env.Rows[0].Elements
	out := make([]DistanceResult, 0, len(els))
	for _, e := range els {
		out = append(out, DistanceResult{DistanceM: e.Distance.Value, DurationS: e.Duration.Value})
	}
	return out, nil
}

func (c *HTTPClient) Directions(ctx context.Context, origin, dest LatLng) (Route, error) {
	q := url.Values{}
	q.Set("origin", latLngStr(origin))
	q.Set("destination", latLngStr(dest))
	q.Set("vehicle", "car")
	var env struct {
		Routes []struct {
			Legs []struct {
				Distance struct {
					Value int64 `json:"value"`
				} `json:"distance"`
				Duration struct {
					Value int64 `json:"value"`
				} `json:"duration"`
			} `json:"legs"`
			OverviewPolyline struct {
				Points string `json:"points"`
			} `json:"overview_polyline"`
		} `json:"routes"`
	}
	if err := c.getJSON(ctx, "/Direction", q, &env); err != nil {
		return Route{}, fmt.Errorf("%w: %v", ErrDirections, err)
	}
	if len(env.Routes) == 0 || len(env.Routes[0].Legs) == 0 {
		return Route{}, fmt.Errorf("%w: no route", ErrDirections)
	}
	r := env.Routes[0]
	return Route{
		DistanceM: r.Legs[0].Distance.Value,
		DurationS: r.Legs[0].Duration.Value,
		Polyline:  r.OverviewPolyline.Points,
	}, nil
}
```

- [ ] **Step 2: Write the env-gated real test**

`internal/maps/goong/client_http_real_test.go`:
```go
package goong

import (
	"context"
	"os"
	"testing"
)

// Live test against the real Goong API. Skipped unless GOONG_API_KEY is set,
// mirroring goship/client_http_real_test.go.
func TestHTTPClient_DistanceMatrix_Real(t *testing.T) {
	key := os.Getenv("GOONG_API_KEY")
	if key == "" {
		t.Skip("GOONG_API_KEY not set; skipping live Goong test")
	}
	c := NewHTTPClient(key, "https://rsapi.goong.io")
	origin := LatLng{Lat: 10.7769, Lng: 106.7009}
	dests := []LatLng{{Lat: 10.7800, Lng: 106.7000}}
	got, err := c.DistanceMatrix(context.Background(), origin, dests)
	if err != nil {
		t.Fatalf("DistanceMatrix: %v", err)
	}
	if len(got) != 1 || got[0].DistanceM <= 0 {
		t.Fatalf("unexpected result: %+v", got)
	}
}
```

- [ ] **Step 3: Run the build + gated test**

Run: `go test ./internal/maps/goong/ -run TestHTTPClient -v`
Expected: PASS as SKIP (no `GOONG_API_KEY` in CI). `go build ./internal/maps/goong/...` succeeds.

- [ ] **Step 4: Commit**

```bash
git add internal/maps/goong/client_http.go internal/maps/goong/client_http_real_test.go
git commit -m "feat(goong): HTTP client for geocode, distance matrix, directions"
```

---

## Task 5: Goong factory

**Files:**
- Create: `internal/maps/goong/factory.go`
- Test: `internal/maps/goong/factory_test.go`

- [ ] **Step 1: Write the failing test**

`internal/maps/goong/factory_test.go`:
```go
package goong

import "testing"

func TestNewFromConfig_MockByDefault(t *testing.T) {
	c, err := NewFromConfig(Config{Mode: "mock"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := c.(*MockClient); !ok {
		t.Fatalf("expected *MockClient, got %T", c)
	}
}

func TestNewFromConfig_ProductionRequiresKey(t *testing.T) {
	if _, err := NewFromConfig(Config{Mode: "production"}); err == nil {
		t.Fatal("expected error when production mode has no API key")
	}
}

func TestNewFromConfig_ProductionWithKey(t *testing.T) {
	c, err := NewFromConfig(Config{Mode: "production", APIKey: "k", BaseURL: "https://rsapi.goong.io"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := c.(*HTTPClient); !ok {
		t.Fatalf("expected *HTTPClient, got %T", c)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/maps/goong/ -run TestNewFromConfig -v`
Expected: FAIL — `NewFromConfig`/`Config` undefined.

- [ ] **Step 3: Write the factory**

`internal/maps/goong/factory.go`:
```go
package goong

import "fmt"

type Config struct {
	Mode    string // mock | production
	APIKey  string
	BaseURL string
}

func NewFromConfig(cfg Config) (Client, error) {
	switch cfg.Mode {
	case "mock", "":
		return NewMockClient(), nil
	case "production":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("goong: production mode requires GOONG_API_KEY")
		}
		base := cfg.BaseURL
		if base == "" {
			base = "https://rsapi.goong.io"
		}
		return NewHTTPClient(cfg.APIKey, base), nil
	default:
		return nil, fmt.Errorf("goong: unknown mode %q (want mock|production)", cfg.Mode)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/maps/goong/ -run TestNewFromConfig -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/maps/goong/factory.go internal/maps/goong/factory_test.go
git commit -m "feat(goong): factory selecting mock|production"
```

---

## Task 6: Config — `GoongConfig`

**Files:**
- Modify: `internal/config/config.go` (add struct + load block near the `GoshipConfig` block at lines 191-200 / 225-234)
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go`:
```go
func TestLoad_GoongDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Goong.Mode != "mock" {
		t.Errorf("Goong.Mode = %q, want mock", cfg.Goong.Mode)
	}
	if cfg.Goong.BaseURL != "https://rsapi.goong.io" {
		t.Errorf("Goong.BaseURL = %q, want https://rsapi.goong.io", cfg.Goong.BaseURL)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoad_GoongDefaults -v`
Expected: FAIL — `cfg.Goong` undefined.

- [ ] **Step 3: Add the struct and load block**

In `internal/config/config.go`, add the field to the `Config` struct (next to `Goship GoshipConfig` around line 26):
```go
	Goong       GoongConfig
```

Add the load block right after the `cfg.Goship = GoshipConfig{...}` block (after line 200):
```go
	cfg.Goong = GoongConfig{
		Mode:    getEnv("GOONG_MODE", "mock"),
		APIKey:  getEnv("GOONG_API_KEY", ""),
		BaseURL: getEnv("GOONG_BASE_URL", "https://rsapi.goong.io"),
	}
```

Add the struct definition after the `GoshipConfig` struct (after line 234):
```go
type GoongConfig struct {
	Mode    string // mock | production
	APIKey  string
	BaseURL string
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestLoad_GoongDefaults -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): Goong config block (mode, api key, base url)"
```

---

## Task 7: store domain — entity, hours, haversine, open/closed

**Files:**
- Create: `internal/store/domain/store.go`
- Test: `internal/store/domain/store_test.go`

`weekday` is `0=Sunday..6=Saturday` (matches Go `time.Weekday`). Open/closed is computed in `Asia/Ho_Chi_Minh`.

- [ ] **Step 1: Write the failing test**

`internal/store/domain/store_test.go`:
```go
package domain

import (
	"testing"
	"time"
)

func TestHaversineKm_KnownDistance(t *testing.T) {
	// Ben Thanh Market -> Landmark 81, ~6.5km straight line.
	d := HaversineKm(10.7720, 106.6980, 10.7951, 106.7218)
	if d < 3 || d > 5 {
		t.Errorf("HaversineKm = %.2f, want roughly 3-5km", d)
	}
}

func TestComputeOpenStatus_OpenInsideWindow(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, loc) // Monday 10:00, weekday=1
	hours := []StoreHours{{Weekday: 1, OpenTime: "09:00", CloseTime: "21:00"}}
	st := ComputeOpenStatus(hours, now)
	if st == nil || !st.Open {
		t.Fatalf("expected open, got %+v", st)
	}
}

func TestComputeOpenStatus_ClosedOutsideWindow(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	now := time.Date(2026, 6, 15, 22, 0, 0, 0, loc) // Monday 22:00
	hours := []StoreHours{{Weekday: 1, OpenTime: "09:00", CloseTime: "21:00"}}
	st := ComputeOpenStatus(hours, now)
	if st == nil || st.Open {
		t.Fatalf("expected closed, got %+v", st)
	}
}

func TestComputeOpenStatus_NoHoursReturnsNil(t *testing.T) {
	if ComputeOpenStatus(nil, time.Now()) != nil {
		t.Error("expected nil open status when no hours configured")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/domain/ -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Write the domain code**

`internal/store/domain/store.go`:
```go
// Package domain holds store-discovery entities and pure logic.
package domain

import (
	"math"
	"time"

	"github.com/google/uuid"
)

// Store is a public, geocoded brand address presented as a physical store,
// composed with brand identity and opening hours.
type Store struct {
	AddressID   uuid.UUID
	BrandID     uuid.UUID
	BrandName   string
	BrandSlug   string
	LogoURL     *string
	BannerURL   *string
	Label       string
	AddressLine string
	Ward        string
	District    string
	City        string
	Phone       *string
	Latitude    float64
	Longitude   float64
	Hours       []StoreHours

	// Populated by the service for nearby/search results.
	DistanceM     *int64
	DurationS     *int64
	DistanceApprox bool // true when distance is haversine fallback (Goong unavailable)
}

// StoreHours is one opening window for a given weekday (0=Sunday..6=Saturday).
// Times are "HH:MM" in Asia/Ho_Chi_Minh.
type StoreHours struct {
	Weekday   int
	OpenTime  string
	CloseTime string
}

// OpenStatus is the computed open/closed state at a point in time.
type OpenStatus struct {
	Open bool
}

// HaversineKm returns the straight-line distance in kilometers.
func HaversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const earthKm = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthKm * math.Asin(math.Sqrt(a))
}

// ComputeOpenStatus returns nil when no hours are configured (status unknown).
// Otherwise it reports whether `now` falls inside any window for that weekday.
func ComputeOpenStatus(hours []StoreHours, now time.Time) *OpenStatus {
	if len(hours) == 0 {
		return nil
	}
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err == nil {
		now = now.In(loc)
	}
	wd := int(now.Weekday()) // Sunday=0..Saturday=6
	cur := now.Hour()*60 + now.Minute()
	for _, h := range hours {
		if h.Weekday != wd {
			continue
		}
		open := parseHM(h.OpenTime)
		close := parseHM(h.CloseTime)
		if open <= cur && cur < close {
			return &OpenStatus{Open: true}
		}
	}
	return &OpenStatus{Open: false}
}

// parseHM converts "HH:MM" to minutes since midnight; returns 0 on parse failure.
func parseHM(s string) int {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0
	}
	return t.Hour()*60 + t.Minute()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/domain/ -v`
Expected: PASS (all four tests).

- [ ] **Step 5: Commit**

```bash
git add internal/store/domain/store.go internal/store/domain/store_test.go
git commit -m "feat(store): domain entity, haversine, open/closed logic"
```

---

## Task 8: store domain — DTOs and errors

**Files:**
- Create: `internal/store/domain/dto.go`
- Create: `internal/store/domain/errors.go`

- [ ] **Step 1: Write the DTOs + converter**

`internal/store/domain/dto.go`:
```go
package domain

import "github.com/google/uuid"

// StoreSummary is the list-item shape for nearby/search results.
type StoreSummary struct {
	ID             uuid.UUID `json:"id"`
	BrandName      string    `json:"brand_name"`
	BrandSlug      string    `json:"brand_slug"`
	LogoURL        *string   `json:"logo_url,omitempty"`
	Label          string    `json:"label"`
	AddressLine    string    `json:"address_line"`
	Ward           string    `json:"ward"`
	District       string    `json:"district"`
	City           string    `json:"city"`
	Latitude       float64   `json:"latitude"`
	Longitude      float64   `json:"longitude"`
	DistanceM      *int64    `json:"distance_m,omitempty"`
	DurationS      *int64    `json:"duration_s,omitempty"`
	DistanceApprox bool      `json:"distance_approx,omitempty"`
	Open           *bool     `json:"open,omitempty"`
}

// StoreDetail is the full single-store shape (UC25).
type StoreDetail struct {
	StoreSummary
	BannerURL *string             `json:"banner_url,omitempty"`
	Phone     *string             `json:"phone,omitempty"`
	Hours     []StoreHoursDTO     `json:"hours"`
}

type StoreHoursDTO struct {
	Weekday   int    `json:"weekday"`
	OpenTime  string `json:"open_time"`
	CloseTime string `json:"close_time"`
}

// DirectionsResponse is the UC26 route payload.
type DirectionsResponse struct {
	DistanceM int64  `json:"distance_m"`
	DurationS int64  `json:"duration_s"`
	Polyline  string `json:"polyline"`
}

func openPtr(st *OpenStatus) *bool {
	if st == nil {
		return nil
	}
	return &st.Open
}

func ToStoreSummary(s *Store, open *OpenStatus) StoreSummary {
	return StoreSummary{
		ID: s.AddressID, BrandName: s.BrandName, BrandSlug: s.BrandSlug,
		LogoURL: s.LogoURL, Label: s.Label, AddressLine: s.AddressLine,
		Ward: s.Ward, District: s.District, City: s.City,
		Latitude: s.Latitude, Longitude: s.Longitude,
		DistanceM: s.DistanceM, DurationS: s.DurationS,
		DistanceApprox: s.DistanceApprox, Open: openPtr(open),
	}
}

func ToStoreDetail(s *Store, open *OpenStatus) StoreDetail {
	hrs := make([]StoreHoursDTO, 0, len(s.Hours))
	for _, h := range s.Hours {
		hrs = append(hrs, StoreHoursDTO{Weekday: h.Weekday, OpenTime: h.OpenTime, CloseTime: h.CloseTime})
	}
	return StoreDetail{
		StoreSummary: ToStoreSummary(s, open),
		BannerURL:    s.BannerURL, Phone: s.Phone, Hours: hrs,
	}
}
```

- [ ] **Step 2: Write the errors**

`internal/store/domain/errors.go`:
```go
package domain

import (
	"net/http"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

func ErrStoreNotFound() *httpx.AppError {
	return httpx.NewAppError(http.StatusNotFound, "STORE_NOT_FOUND", "Store not found")
}

func ErrDirectionsUnavailable() *httpx.AppError {
	return httpx.NewAppError(http.StatusBadGateway, "DIRECTIONS_UNAVAILABLE", "Could not compute directions")
}
```

- [ ] **Step 3: Verify build**

Run: `go build ./internal/store/...`
Expected: builds.

- [ ] **Step 4: Commit**

```bash
git add internal/store/domain/dto.go internal/store/domain/errors.go
git commit -m "feat(store): DTOs, converters, sentinel errors"
```

---

## Task 9: store repo — interface + Postgres

**Files:**
- Create: `internal/store/repo/repo.go`
- Create: `internal/store/repo/store_pg.go`
- Test: `internal/store/repo/store_pg_test.go`

The store columns join `brand_addresses` with `brands`. A store is a row where
`ba.is_public AND ba.deleted_at IS NULL AND ba.latitude IS NOT NULL AND ba.longitude IS NOT NULL`
and the brand is active (`b.status='active' AND b.deleted_at IS NULL`).

- [ ] **Step 1: Write the interface + DBTX**

`internal/store/repo/repo.go`:
```go
// Package repo defines persistence for store discovery (read-only over brand_addresses).
package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/store/domain"
)

var ErrNotFound = errors.New("store: not found")

// DBTX is the subset of pgxpool.Pool used by this repo.
type DBTX interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// AreaFilter is the search-by-area criteria (any subset; q is address text).
type AreaFilter struct {
	CityCode     string
	DistrictCode string
	WardCode     string
	Q            string
	Limit        int
}

type Repo interface {
	// Nearby returns public geocoded stores within radiusKm of (lat,lng),
	// already sorted by haversine distance ascending, capped at limit.
	Nearby(ctx context.Context, lat, lng, radiusKm float64, limit int) ([]*domain.Store, error)
	// SearchByArea returns stores matching the filter.
	SearchByArea(ctx context.Context, f AreaFilter) ([]*domain.Store, error)
	// Detail returns one store by brand_address id (ErrNotFound if not a public store).
	Detail(ctx context.Context, addressID uuid.UUID) (*domain.Store, error)
}
```

- [ ] **Step 2: Write the failing repo test**

`internal/store/repo/store_pg_test.go` (uses the existing testfixtures harness — check `internal/testfixtures` for the exact helper name, e.g. `testfixtures.NewDB(t)`; this plan assumes a `func NewTestPool(t *testing.T) *pgxpool.Pool` exists, adapt to the real one):
```go
package repo

import (
	"context"
	"testing"

	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

func TestStorePG_Nearby_OrdersByDistance(t *testing.T) {
	pool := testfixtures.NewTestPool(t) // adapt to the real fixtures helper
	ctx := context.Background()
	r := NewStorePG(pool)

	// testfixtures should seed at least one active brand with a public,
	// geocoded address near central HCMC (10.7769,106.7009). If the fixtures
	// lack store rows, add a seed helper here.
	got, err := r.Nearby(ctx, 10.7769, 106.7009, 10, 25)
	if err != nil {
		t.Fatalf("Nearby: %v", err)
	}
	for i := 1; i < len(got); i++ {
		di := domain.HaversineKm(10.7769, 106.7009, got[i-1].Latitude, got[i-1].Longitude)
		dj := domain.HaversineKm(10.7769, 106.7009, got[i].Latitude, got[i].Longitude)
		if di > dj {
			t.Errorf("results not sorted by distance at %d", i)
		}
	}
}
```
> Note: import `domain "github.com/wearwhere/wearwhere_be/internal/store/domain"` in the test.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/store/repo/ -run TestStorePG_Nearby -v`
Expected: FAIL — `NewStorePG` undefined.

- [ ] **Step 4: Write the Postgres repo**

`internal/store/repo/store_pg.go`:
```go
package repo

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/store/domain"
)

type StorePG struct{ db DBTX }

func NewStorePG(db DBTX) *StorePG { return &StorePG{db: db} }

const storeCols = `ba.id, ba.brand_id, b.name, b.slug, b.logo_url, b.banner_url,
                   ba.label, ba.address_line, ba.ward, ba.district, ba.city,
                   ba.phone, ba.latitude, ba.longitude`

const storeFrom = `FROM brand_addresses ba
                   JOIN brands b ON b.id = ba.brand_id
                   WHERE ba.is_public = TRUE AND ba.deleted_at IS NULL
                     AND ba.latitude IS NOT NULL AND ba.longitude IS NOT NULL
                     AND b.status = 'active' AND b.deleted_at IS NULL`

func scanStore(row pgx.Row) (*domain.Store, error) {
	var s domain.Store
	err := row.Scan(
		&s.AddressID, &s.BrandID, &s.BrandName, &s.BrandSlug, &s.LogoURL, &s.BannerURL,
		&s.Label, &s.AddressLine, &s.Ward, &s.District, &s.City,
		&s.Phone, &s.Latitude, &s.Longitude,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}

func (r *StorePG) collect(rows pgx.Rows) ([]*domain.Store, error) {
	defer rows.Close()
	var out []*domain.Store
	for rows.Next() {
		s, err := scanStore(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Nearby uses a bounding-box pre-filter on the indexed lat/lng, then orders by
// the haversine expression. Cheap and index-friendly; exact ordering done in SQL.
func (r *StorePG) Nearby(ctx context.Context, lat, lng, radiusKm float64, limit int) ([]*domain.Store, error) {
	// ~111km per degree latitude; widen longitude by cos(lat) for the box.
	q := `SELECT ` + storeCols + `,
	      6371 * 2 * ASIN(SQRT(
	        POWER(SIN(RADIANS(ba.latitude - $1)/2), 2) +
	        COS(RADIANS($1)) * COS(RADIANS(ba.latitude)) *
	        POWER(SIN(RADIANS(ba.longitude - $2)/2), 2)
	      )) AS dist_km
	      ` + storeFrom + `
	        AND ba.latitude BETWEEN $1 - ($3/111.0) AND $1 + ($3/111.0)
	        AND ba.longitude BETWEEN $2 - ($3/(111.0*COS(RADIANS($1)))) AND $2 + ($3/(111.0*COS(RADIANS($1))))
	      ORDER BY dist_km ASC
	      LIMIT $4`
	rows, err := r.db.Query(ctx, q, lat, lng, radiusKm, limit)
	if err != nil {
		return nil, err
	}
	// dist_km is selected for ordering only; scanStore ignores it, so re-map.
	defer rows.Close()
	var out []*domain.Store
	for rows.Next() {
		var s domain.Store
		var distKm float64
		if err := rows.Scan(
			&s.AddressID, &s.BrandID, &s.BrandName, &s.BrandSlug, &s.LogoURL, &s.BannerURL,
			&s.Label, &s.AddressLine, &s.Ward, &s.District, &s.City,
			&s.Phone, &s.Latitude, &s.Longitude, &distKm,
		); err != nil {
			return nil, err
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

func (r *StorePG) SearchByArea(ctx context.Context, f AreaFilter) ([]*domain.Store, error) {
	q := `SELECT ` + storeCols + ` ` + storeFrom
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		q += cond + "$" + strconv.Itoa(len(args))
	}
	if f.CityCode != "" {
		add(" AND ba.city_code = ", f.CityCode)
	}
	if f.DistrictCode != "" {
		add(" AND ba.district_code = ", f.DistrictCode)
	}
	if f.WardCode != "" {
		add(" AND ba.ward_code = ", f.WardCode)
	}
	if f.Q != "" {
		args = append(args, "%"+strings.ToLower(f.Q)+"%")
		q += " AND (LOWER(ba.address_line) LIKE $" + strconv.Itoa(len(args)) +
			" OR LOWER(ba.district) LIKE $" + strconv.Itoa(len(args)) +
			" OR LOWER(ba.city) LIKE $" + strconv.Itoa(len(args)) + ")"
	}
	q += " ORDER BY b.name ASC"
	if f.Limit > 0 {
		args = append(args, f.Limit)
		q += " LIMIT $" + strconv.Itoa(len(args))
	}
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return r.collect(rows)
}

func (r *StorePG) Detail(ctx context.Context, addressID uuid.UUID) (*domain.Store, error) {
	q := `SELECT ` + storeCols + ` ` + storeFrom + ` AND ba.id = $1`
	s, err := scanStore(r.db.QueryRow(ctx, q, addressID))
	if err != nil {
		return nil, err
	}
	hours, err := r.hours(ctx, addressID)
	if err != nil {
		return nil, err
	}
	s.Hours = hours
	return s, nil
}

func (r *StorePG) hours(ctx context.Context, addressID uuid.UUID) ([]domain.StoreHours, error) {
	rows, err := r.db.Query(ctx,
		`SELECT weekday, to_char(open_time,'HH24:MI'), to_char(close_time,'HH24:MI')
		   FROM store_hours WHERE brand_address_id = $1
		   ORDER BY weekday, open_time`, addressID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.StoreHours
	for rows.Next() {
		var h domain.StoreHours
		if err := rows.Scan(&h.Weekday, &h.OpenTime, &h.CloseTime); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/store/repo/ -run TestStorePG_Nearby -v`
Expected: PASS (after adapting the testfixtures helper name and seeding a store row).

- [ ] **Step 6: Commit**

```bash
git add internal/store/repo/repo.go internal/store/repo/store_pg.go internal/store/repo/store_pg_test.go
git commit -m "feat(store): repo with nearby (haversine), area search, detail+hours"
```

---

## Task 10: store service — orchestration

**Files:**
- Create: `internal/store/service/service.go`
- Test: `internal/store/service/service_test.go`

Behaviour:
- `Nearby`: repo at `radiusKm=5`; if empty, retry at `10`. Cap candidates to `maxNearby=25`. Call Goong `DistanceMatrix` once to set real `DistanceM`/`DurationS`; on Goong error, fall back to haversine meters and set `DistanceApprox=true`. Sort by `DistanceM` ascending.
- `Detail`: repo detail + `ComputeOpenStatus(now)`.
- `Directions`: Goong `Directions`; on error return `domain.ErrDirectionsUnavailable()`.

- [ ] **Step 1: Write the failing test**

`internal/store/service/service_test.go`:
```go
package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/maps/goong"
	"github.com/wearwhere/wearwhere_be/internal/store/domain"
	"github.com/wearwhere/wearwhere_be/internal/store/repo"
)

// fakeRepo returns canned stores.
type fakeRepo struct {
	nearby []*domain.Store
	detail *domain.Store
}

func (f *fakeRepo) Nearby(_ context.Context, _, _, _ float64, _ int) ([]*domain.Store, error) {
	return f.nearby, nil
}
func (f *fakeRepo) SearchByArea(_ context.Context, _ repo.AreaFilter) ([]*domain.Store, error) {
	return f.nearby, nil
}
func (f *fakeRepo) Detail(_ context.Context, _ uuid.UUID) (*domain.Store, error) {
	if f.detail == nil {
		return nil, repo.ErrNotFound
	}
	return f.detail, nil
}

// failGoong errors on DistanceMatrix to exercise the degrade path.
type failGoong struct{ goong.Client }

func (failGoong) DistanceMatrix(context.Context, goong.LatLng, []goong.LatLng) ([]goong.DistanceResult, error) {
	return nil, errors.New("boom")
}

func TestNearby_EnrichesWithGoongDistance(t *testing.T) {
	stores := []*domain.Store{
		{AddressID: uuid.New(), Latitude: 10.78, Longitude: 106.70},
		{AddressID: uuid.New(), Latitude: 10.80, Longitude: 106.71},
	}
	svc := New(&fakeRepo{nearby: stores}, goong.NewMockClient())
	got, err := svc.Nearby(context.Background(), 10.7769, 106.7009, 0)
	if err != nil {
		t.Fatalf("Nearby: %v", err)
	}
	if got[0].DistanceM == nil {
		t.Fatal("expected DistanceM populated from Goong")
	}
	if *got[0].DistanceM > *got[1].DistanceM {
		t.Error("expected ascending distance order")
	}
}

func TestNearby_DegradesWhenGoongFails(t *testing.T) {
	stores := []*domain.Store{{AddressID: uuid.New(), Latitude: 10.78, Longitude: 106.70}}
	svc := New(&fakeRepo{nearby: stores}, failGoong{goong.NewMockClient()})
	got, err := svc.Nearby(context.Background(), 10.7769, 106.7009, 0)
	if err != nil {
		t.Fatalf("Nearby should degrade, not fail: %v", err)
	}
	if !got[0].DistanceApprox || got[0].DistanceM == nil {
		t.Errorf("expected haversine fallback with DistanceApprox=true, got %+v", got[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/service/ -v`
Expected: FAIL — `New`/`Nearby` undefined.

- [ ] **Step 3: Write the service**

`internal/store/service/service.go`:
```go
// Package service orchestrates store discovery: DB candidates + Goong enrichment.
package service

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/maps/goong"
	"github.com/wearwhere/wearwhere_be/internal/store/domain"
	"github.com/wearwhere/wearwhere_be/internal/store/repo"
)

const (
	defaultRadiusKm = 5.0
	widenRadiusKm   = 10.0
	maxNearby       = 25
)

type Service struct {
	repo  repo.Repo
	goong goong.Client
	now   func() time.Time
}

func New(r repo.Repo, g goong.Client) *Service {
	return &Service{repo: r, goong: g, now: time.Now}
}

func (s *Service) Nearby(ctx context.Context, lat, lng, radiusKm float64) ([]domain.StoreSummary, error) {
	if radiusKm <= 0 {
		radiusKm = defaultRadiusKm
	}
	stores, err := s.repo.Nearby(ctx, lat, lng, radiusKm, maxNearby)
	if err != nil {
		return nil, err
	}
	if len(stores) == 0 && radiusKm < widenRadiusKm {
		stores, err = s.repo.Nearby(ctx, lat, lng, widenRadiusKm, maxNearby)
		if err != nil {
			return nil, err
		}
	}
	s.enrichDistances(ctx, lat, lng, stores)
	sort.SliceStable(stores, func(i, j int) bool {
		return derefM(stores[i].DistanceM) < derefM(stores[j].DistanceM)
	})
	out := make([]domain.StoreSummary, 0, len(stores))
	for _, st := range stores {
		out = append(out, domain.ToStoreSummary(st, domain.ComputeOpenStatus(st.Hours, s.now())))
	}
	return out, nil
}

// enrichDistances sets DistanceM/DurationS from Goong; on failure falls back to
// haversine meters with DistanceApprox=true (graceful degradation per spec).
func (s *Service) enrichDistances(ctx context.Context, lat, lng float64, stores []*domain.Store) {
	if len(stores) == 0 {
		return
	}
	dests := make([]goong.LatLng, len(stores))
	for i, st := range stores {
		dests[i] = goong.LatLng{Lat: st.Latitude, Lng: st.Longitude}
	}
	res, err := s.goong.DistanceMatrix(ctx, goong.LatLng{Lat: lat, Lng: lng}, dests)
	if err != nil || len(res) != len(stores) {
		for _, st := range stores {
			m := int64(domain.HaversineKm(lat, lng, st.Latitude, st.Longitude) * 1000)
			st.DistanceM = &m
			st.DistanceApprox = true
		}
		return
	}
	for i, st := range stores {
		dm, du := res[i].DistanceM, res[i].DurationS
		st.DistanceM, st.DurationS = &dm, &du
	}
}

func (s *Service) SearchByArea(ctx context.Context, f repo.AreaFilter, origin *goong.LatLng) ([]domain.StoreSummary, error) {
	stores, err := s.repo.SearchByArea(ctx, f)
	if err != nil {
		return nil, err
	}
	if origin != nil {
		s.enrichDistances(ctx, origin.Lat, origin.Lng, stores)
	}
	out := make([]domain.StoreSummary, 0, len(stores))
	for _, st := range stores {
		out = append(out, domain.ToStoreSummary(st, domain.ComputeOpenStatus(st.Hours, s.now())))
	}
	return out, nil
}

func (s *Service) Detail(ctx context.Context, id uuid.UUID) (domain.StoreDetail, error) {
	st, err := s.repo.Detail(ctx, id)
	if err != nil {
		if err == repo.ErrNotFound {
			return domain.StoreDetail{}, domain.ErrStoreNotFound()
		}
		return domain.StoreDetail{}, err
	}
	return domain.ToStoreDetail(st, domain.ComputeOpenStatus(st.Hours, s.now())), nil
}

func (s *Service) Directions(ctx context.Context, id uuid.UUID, from goong.LatLng) (domain.DirectionsResponse, error) {
	st, err := s.repo.Detail(ctx, id)
	if err != nil {
		if err == repo.ErrNotFound {
			return domain.DirectionsResponse{}, domain.ErrStoreNotFound()
		}
		return domain.DirectionsResponse{}, err
	}
	route, err := s.goong.Directions(ctx, from, goong.LatLng{Lat: st.Latitude, Lng: st.Longitude})
	if err != nil {
		return domain.DirectionsResponse{}, domain.ErrDirectionsUnavailable()
	}
	return domain.DirectionsResponse{DistanceM: route.DistanceM, DurationS: route.DurationS, Polyline: route.Polyline}, nil
}

func derefM(p *int64) int64 {
	if p == nil {
		return 1<<62 // sort missing distances last
	}
	return *p
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/service/ -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/store/service/service.go internal/store/service/service_test.go
git commit -m "feat(store): service orchestration with Goong enrichment + degrade"
```

---

## Task 11: store handler + routes

**Files:**
- Create: `internal/store/handler/handler.go`
- Create: `internal/store/handler/routes.go`
- Test: `internal/store/handler/handler_test.go`

- [ ] **Step 1: Write the handler**

`internal/store/handler/handler.go`:
```go
// Package handler exposes public store-discovery HTTP endpoints.
package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/maps/goong"
	"github.com/wearwhere/wearwhere_be/internal/store/repo"
	"github.com/wearwhere/wearwhere_be/internal/store/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type Handler struct{ svc *service.Service }

func NewHandler(svc *service.Service) *Handler { return &Handler{svc: svc} }

// parseLatLng parses "lat,lng" or separate values; returns ok=false on failure.
func parseLatLng(lat, lng string) (goong.LatLng, bool) {
	la, err1 := strconv.ParseFloat(strings.TrimSpace(lat), 64)
	ln, err2 := strconv.ParseFloat(strings.TrimSpace(lng), 64)
	if err1 != nil || err2 != nil {
		return goong.LatLng{}, false
	}
	return goong.LatLng{Lat: la, Lng: ln}, true
}

func (h *Handler) Nearby(c *gin.Context) {
	p, ok := parseLatLng(c.Query("lat"), c.Query("lng"))
	if !ok {
		httpx.Error(c, http.StatusBadRequest, "INVALID_QUERY", "lat and lng are required floats")
		return
	}
	radius, _ := strconv.ParseFloat(c.Query("radius_km"), 64)
	items, err := h.svc.Nearby(c.Request.Context(), p.Lat, p.Lng, radius)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) Search(c *gin.Context) {
	f := repo.AreaFilter{
		CityCode:     c.Query("city_code"),
		DistrictCode: c.Query("district_code"),
		WardCode:     c.Query("ward_code"),
		Q:            c.Query("q"),
		Limit:        50,
	}
	var origin *goong.LatLng
	if p, ok := parseLatLng(c.Query("lat"), c.Query("lng")); ok {
		origin = &p
	}
	items, err := h.svc.SearchByArea(c.Request.Context(), f, origin)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) Detail(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_ID", "invalid store id")
		return
	}
	d, err := h.svc.Detail(c.Request.Context(), id)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, d)
}

func (h *Handler) Directions(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_ID", "invalid store id")
		return
	}
	from := c.Query("from")
	parts := strings.Split(from, ",")
	if len(parts) != 2 {
		httpx.Error(c, http.StatusBadRequest, "INVALID_QUERY", "from must be 'lat,lng'")
		return
	}
	p, ok := parseLatLng(parts[0], parts[1])
	if !ok {
		httpx.Error(c, http.StatusBadRequest, "INVALID_QUERY", "from must be 'lat,lng'")
		return
	}
	resp, err := h.svc.Directions(c.Request.Context(), id, p)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}
```

- [ ] **Step 2: Write the routes**

`internal/store/handler/routes.go`:
```go
package handler

import "github.com/gin-gonic/gin"

// MountStoresPublic registers public read-only store discovery routes (no auth).
func MountStoresPublic(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/stores/nearby", h.Nearby)
	rg.GET("/stores", h.Search)
	rg.GET("/stores/:id", h.Detail)
	rg.GET("/stores/:id/directions", h.Directions)
}
```

- [ ] **Step 3: Write the handler test**

`internal/store/handler/handler_test.go`:
```go
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/wearwhere/wearwhere_be/internal/maps/goong"
	storerepo "github.com/wearwhere/wearwhere_be/internal/store/repo"
	"github.com/wearwhere/wearwhere_be/internal/store/service"
)

func newTestRouter(svc *service.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	MountStoresPublic(r.Group("/api/v1"), NewHandler(svc))
	return r
}

func TestNearby_MissingLatLng_400(t *testing.T) {
	// Build a service with a repo that would error if reached; we expect a 400 before that.
	svc := service.New(nil, goong.NewMockClient())
	r := newTestRouter(svc)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/stores/nearby", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if _, ok := body["error"]; !ok {
		t.Errorf("expected error envelope, got %s", w.Body.String())
	}
	_ = storerepo.AreaFilter{} // keep import if unused elsewhere
}
```
> Note: if the unused import bites, drop the `storerepo` import and the trailing no-op line.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/handler/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/handler/handler.go internal/store/handler/routes.go internal/store/handler/handler_test.go
git commit -m "feat(store): public HTTP handlers + routes (UC23-26)"
```

---

## Task 12: Wire into `cmd/api/main.go` and `main_test.go`

**Files:**
- Modify: `cmd/api/main.go` (Goong client near the goship block ~line 120; store svc near the services block ~line 145; mount near `MountCatalog` ~line 286)
- Modify: `cmd/api/main_test.go` (mirror the wiring for integration tests near line 111)

- [ ] **Step 1: Add imports**

In `cmd/api/main.go` import block, add:
```go
	"github.com/wearwhere/wearwhere_be/internal/maps/goong"
	storehandler "github.com/wearwhere/wearwhere_be/internal/store/handler"
	storerepo "github.com/wearwhere/wearwhere_be/internal/store/repo"
	storeservice "github.com/wearwhere/wearwhere_be/internal/store/service"
```

- [ ] **Step 2: Construct the Goong client (after the goship block, ~line 130)**

```go
	// ── maps (Goong) ──
	goongClient, err := goong.NewFromConfig(goong.Config{
		Mode:    cfg.Goong.Mode,
		APIKey:  cfg.Goong.APIKey,
		BaseURL: cfg.Goong.BaseURL,
	})
	if err != nil {
		log.Fatalf("goong client: %v", err)
	}
```

- [ ] **Step 3: Construct the store service (in the services block, ~line 149)**

```go
	storeSvc := storeservice.New(storerepo.NewStorePG(pgPool), goongClient)
```

- [ ] **Step 4: Mount the routes (next to MountCatalog, ~line 286)**

```go
	storehandler.MountStoresPublic(v1, storehandler.NewHandler(storeSvc))
```

- [ ] **Step 5: Mirror wiring in `cmd/api/main_test.go`**

Near the other public mounts (~line 112), add:
```go
	storeSvc := storeservice.New(storerepo.NewStorePG(pgPool), goong.NewMockClient())
	storehandler.MountStoresPublic(v1, storehandler.NewHandler(storeSvc))
```
(Add the same imports to the test file; use `goong.NewMockClient()` so tests never hit the network.)

- [ ] **Step 6: Build + run the full test suite**

Run: `go build ./... && go test ./...`
Expected: build succeeds; all tests pass (Goong real test skipped without key).

- [ ] **Step 7: Commit**

```bash
git add cmd/api/main.go cmd/api/main_test.go
git commit -m "feat(store): wire Goong client + store discovery routes into API"
```

---

## Task 13: Dev seed for `store_hours` (manual verification aid)

**Files:**
- Create: `db/seed_dev_store_hours.sql`

Per the spec, this feature only reads `store_hours` (brand-owner CRUD is a separate feature), so seed sample rows for manual testing.

- [ ] **Step 1: Write the seed**

`db/seed_dev_store_hours.sql`:
```sql
-- Sample opening hours (Mon-Sun 09:00-21:00) for every public, geocoded store.
-- weekday: 0=Sunday .. 6=Saturday.
INSERT INTO store_hours (brand_address_id, weekday, open_time, close_time)
SELECT ba.id, wd, TIME '09:00', TIME '21:00'
FROM brand_addresses ba
CROSS JOIN generate_series(0, 6) AS wd
WHERE ba.is_public = TRUE
  AND ba.deleted_at IS NULL
  AND ba.latitude IS NOT NULL
  AND ba.longitude IS NOT NULL
ON CONFLICT DO NOTHING;
```

- [ ] **Step 2: Apply and smoke-test the endpoints**

```bash
psql "$DATABASE_URL" -f db/seed_dev_store_hours.sql
# with the API running locally:
curl "http://localhost:8080/api/v1/stores/nearby?lat=10.7769&lng=106.7009"
curl "http://localhost:8080/api/v1/stores?city_code=<a real city_code from your seed>"
```
Expected: `nearby` returns stores ordered by distance with `distance_m`, `open` fields; search returns matching stores.

- [ ] **Step 3: Commit**

```bash
git add db/seed_dev_store_hours.sql
git commit -m "chore(store): dev seed for store_hours"
```

---

## Self-Review

**Spec coverage:**
- UC23 Find Nearby → Task 9 (repo `Nearby`) + Task 10 (`Service.Nearby`, radius widen, Goong enrich) + Task 11 (`/stores/nearby`). ✓
- UC24 Search by Area → Task 9 (`SearchByArea`) + Task 10 + Task 11 (`/stores`). ✓
- UC25 View Detail → Task 9 (`Detail` + hours) + Task 7/8 (open/closed, DTO with brand logo/banner/phone) + Task 11 (`/stores/:id`). ✓
- UC26 Directions → Task 4/5 (Goong client) + Task 10 (`Directions`) + Task 11 (`/stores/:id/directions`). ✓
- Goong proxy (geocode/distance/directions) → Tasks 2–5. ✓
- `store_hours` table → Task 1. Open/closed in VN tz → Task 7. ✓
- Public read, no auth → Task 11 `MountStoresPublic`, Task 12 mounts on `v1` without auth middleware. ✓
- Graceful degrade when Goong fails (nearby → haversine + `distance_approx`) → Task 10. ✓
- Config mock|production, prod requires key → Tasks 5, 6. ✓
- Ratings excluded, brand-owner hours CRUD excluded → not implemented (documented). ✓

**Placeholder scan:** One implementer note flagged inline (the testfixtures helper name → adapt to the real `internal/testfixtures` API when implementing Task 9). This is an explicit "use the real symbol" instruction, not a silent TODO. No "add error handling"-style hand-waving.

**Type consistency:** `goong.Client` interface (`Geocode`/`DistanceMatrix`/`Directions`) is implemented by both `MockClient` and `HTTPClient` and consumed by `service.New(repo.Repo, goong.Client)`. `domain.Store` fields (`AddressID`, `DistanceM *int64`, `DistanceApprox`) are used consistently across repo scan, service enrich, and DTO converters. `repo.Repo` methods (`Nearby`/`SearchByArea`/`Detail`) match the fake in the service test and the real `StorePG`. `ComputeOpenStatus(hours, now) *OpenStatus` signature consistent across domain/service.
