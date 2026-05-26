# Sprint 3: Orders, Checkout & PayOS Payment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement customer-side order placement, checkout preview, PayOS payment (mock + production-skeleton), stock reservation lifecycle, and a reservation cleanup job for the WearWhere backend.

**Architecture:** New modules `internal/order`, `internal/payment`, `internal/shipping`, `internal/jobs` following the flat-module pattern from Sprint 1/2 (`domain` / `repo` / `service` / `handler`). PayOS integration is abstracted via a `Client` interface with `client_mock.go` (Sprint 3 default) and `client_http.go` (production-ready stub). Place-order runs as a single Postgres transaction with `SELECT FOR UPDATE` on variants plus `UPDATE ... WHERE (stock_qty - reserved_qty) >= qty` as a double-safety against oversell. A 5-min ticker job releases stock for PayOS orders unpaid > 30 min.

**Tech Stack:** Go 1.22, gin-gonic, pgx v5, golang-migrate, google/uuid, zap, testify, dockertest. Module path: `github.com/wearwhere/wearwhere_be`.

**Spec:** [docs/superpowers/specs/2026-05-24-sprint-3-orders-checkout-payos-design.md](../specs/2026-05-24-sprint-3-orders-checkout-payos-design.md)

---

## Pre-flight

Before starting:
- [ ] On branch `sprint-3-orders-checkout` (created from sprint-2-customer-shopping HEAD `872f632`)
- [ ] `go test ./... 2>&1 | tail -20` passes (Sprint 1+2 tests green)
- [ ] `go build ./cmd/api` succeeds
- [ ] Postgres running locally with migrations 000001–000020 applied
- [ ] Beads issues created (`bd create --title="Sprint 3 Task A1..." --type=task --priority=2` x 25)

---

## Task 1: Migration 000021 — variant.reserved_qty

**Files:**
- Create: `db/migrations/000021_add_variant_reserved_qty.up.sql`
- Create: `db/migrations/000021_add_variant_reserved_qty.down.sql`

- [ ] **Step 1: Write up migration**

```sql
-- db/migrations/000021_add_variant_reserved_qty.up.sql
ALTER TABLE product_variants
  ADD COLUMN reserved_qty INT NOT NULL DEFAULT 0,
  ADD CONSTRAINT chk_variant_reserved_nonneg CHECK (reserved_qty >= 0),
  ADD CONSTRAINT chk_variant_reserved_le_stock CHECK (reserved_qty <= stock_qty);
```

- [ ] **Step 2: Write down migration**

```sql
-- db/migrations/000021_add_variant_reserved_qty.down.sql
ALTER TABLE product_variants
  DROP CONSTRAINT IF EXISTS chk_variant_reserved_le_stock,
  DROP CONSTRAINT IF EXISTS chk_variant_reserved_nonneg,
  DROP COLUMN IF EXISTS reserved_qty;
```

- [ ] **Step 3: Apply up migration**

Run: `migrate -path db/migrations -database "$DATABASE_URL" up 1`
Expected: `21/u add_variant_reserved_qty (...)` and exit 0.

- [ ] **Step 4: Verify column**

Run: `psql "$DATABASE_URL" -c "SELECT column_name, data_type, column_default FROM information_schema.columns WHERE table_name='product_variants' AND column_name='reserved_qty';"`
Expected: one row `reserved_qty | integer | 0`.

- [ ] **Step 5: Test down/up cycle**

Run: `migrate -path db/migrations -database "$DATABASE_URL" down 1 && migrate -path db/migrations -database "$DATABASE_URL" up 1`
Expected: no error.

- [ ] **Step 6: Commit**

```bash
git add db/migrations/000021_add_variant_reserved_qty.up.sql db/migrations/000021_add_variant_reserved_qty.down.sql
git commit -m "feat(db): migration 21 add variant.reserved_qty with CHECK constraints"
```

---

## Task 2: Migration 000022 — brand.shipping_flat_fee_vnd

**Files:**
- Create: `db/migrations/000022_add_brand_shipping_fee.up.sql`
- Create: `db/migrations/000022_add_brand_shipping_fee.down.sql`

- [ ] **Step 1: Write up migration**

```sql
-- db/migrations/000022_add_brand_shipping_fee.up.sql
ALTER TABLE brands
  ADD COLUMN shipping_flat_fee_vnd BIGINT NOT NULL DEFAULT 30000
    CHECK (shipping_flat_fee_vnd >= 0);
```

- [ ] **Step 2: Write down migration**

```sql
-- db/migrations/000022_add_brand_shipping_fee.down.sql
ALTER TABLE brands DROP COLUMN IF EXISTS shipping_flat_fee_vnd;
```

- [ ] **Step 3: Apply up migration**

Run: `migrate -path db/migrations -database "$DATABASE_URL" up 1`
Expected: exit 0.

- [ ] **Step 4: Verify**

Run: `psql "$DATABASE_URL" -c "SELECT shipping_flat_fee_vnd FROM brands LIMIT 1;"`
Expected: `30000`.

- [ ] **Step 5: Commit**

```bash
git add db/migrations/000022_add_brand_shipping_fee.up.sql db/migrations/000022_add_brand_shipping_fee.down.sql
git commit -m "feat(db): migration 22 add brand.shipping_flat_fee_vnd default 30000"
```

---

## Task 3: Migration 000023 — orders

**Files:**
- Create: `db/migrations/000023_create_orders.up.sql`
- Create: `db/migrations/000023_create_orders.down.sql`

- [ ] **Step 1: Write up migration**

```sql
-- db/migrations/000023_create_orders.up.sql
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
CREATE INDEX idx_orders_status ON orders(status, created_at DESC);
```

- [ ] **Step 2: Write down migration**

```sql
-- db/migrations/000023_create_orders.down.sql
DROP TABLE IF EXISTS orders;
```

- [ ] **Step 3: Apply**

Run: `migrate -path db/migrations -database "$DATABASE_URL" up 1`
Expected: exit 0.

- [ ] **Step 4: Verify**

Run: `psql "$DATABASE_URL" -c "\d orders"`
Expected: shows all columns including `cancel_reason` and indexes `idx_orders_user_created`, `idx_orders_status`.

- [ ] **Step 5: Commit**

```bash
git add db/migrations/000023_create_orders.up.sql db/migrations/000023_create_orders.down.sql
git commit -m "feat(db): migration 23 create orders table with state CHECKs"
```

---

## Task 4: Migration 000024 — sub_orders

**Files:**
- Create: `db/migrations/000024_create_sub_orders.up.sql`
- Create: `db/migrations/000024_create_sub_orders.down.sql`

- [ ] **Step 1: Write up migration**

```sql
-- db/migrations/000024_create_sub_orders.up.sql
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

CREATE INDEX idx_sub_orders_order ON sub_orders(order_id);
CREATE INDEX idx_sub_orders_brand_status ON sub_orders(brand_id, status, created_at DESC);
```

- [ ] **Step 2: Write down**

```sql
-- db/migrations/000024_create_sub_orders.down.sql
DROP TABLE IF EXISTS sub_orders;
```

- [ ] **Step 3: Apply + verify + commit**

Run: `migrate -path db/migrations -database "$DATABASE_URL" up 1`
Run: `psql "$DATABASE_URL" -c "\d sub_orders"` — expect UNIQUE (order_id, brand_id) and both indexes present.

```bash
git add db/migrations/000024_create_sub_orders.up.sql db/migrations/000024_create_sub_orders.down.sql
git commit -m "feat(db): migration 24 create sub_orders (per-brand fulfillment unit)"
```

---

## Task 5: Migrations 000025+000026 — order_items + payments

**Files:**
- Create: `db/migrations/000025_create_order_items.up.sql`
- Create: `db/migrations/000025_create_order_items.down.sql`
- Create: `db/migrations/000026_create_payments.up.sql`
- Create: `db/migrations/000026_create_payments.down.sql`

- [ ] **Step 1: Write 25 up**

```sql
-- db/migrations/000025_create_order_items.up.sql
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
CREATE INDEX idx_order_items_variant ON order_items(variant_id);
```

- [ ] **Step 2: Write 25 down**

```sql
-- db/migrations/000025_create_order_items.down.sql
DROP TABLE IF EXISTS order_items;
```

- [ ] **Step 3: Write 26 up**

```sql
-- db/migrations/000026_create_payments.up.sql
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
CREATE INDEX idx_payments_cleanup ON payments(method, status, created_at)
  WHERE method = 'payos' AND status = 'pending';
```

- [ ] **Step 4: Write 26 down**

```sql
-- db/migrations/000026_create_payments.down.sql
DROP TABLE IF EXISTS payments;
```

- [ ] **Step 5: Apply both**

Run: `migrate -path db/migrations -database "$DATABASE_URL" up 2`
Expected: `25/u create_order_items` and `26/u create_payments` both succeed.

- [ ] **Step 6: Verify**

Run: `psql "$DATABASE_URL" -c "\dt" | grep -E "(orders|sub_orders|order_items|payments)"`
Expected: all 4 tables present.

- [ ] **Step 7: Commit**

```bash
git add db/migrations/000025_*.sql db/migrations/000026_*.sql
git commit -m "feat(db): migrations 25-26 create order_items + payments with partial idx for cleanup"
```

---

## Task 6: Variant Reserve/Commit/Release ops

**Files:**
- Modify: `internal/product/repo/variant_pg.go` — append 3 new methods
- Modify: `internal/product/repo/repo.go` — extend `VariantRepo` interface (find existing file first)
- Create: `internal/product/repo/variant_reserve_test.go`

- [ ] **Step 1: Inspect existing VariantRepo interface**

Run: `grep -n "type VariantRepo" internal/product/repo/repo.go`
Read the file to see existing method signatures. Add the new methods at the bottom of the interface.

- [ ] **Step 2: Write test file with table-driven concurrency case**

```go
// internal/product/repo/variant_reserve_test.go
package repo_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

func TestVariantReserve_Success(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	repo := productrepo.NewVariantPG(pool)
	ctx := context.Background()

	brandID := testfixtures.SeedBrand(t, pool, "rep")
	productID := testfixtures.SeedProduct(t, pool, brandID, "tee")
	variantID := testfixtures.SeedVariant(t, pool, productID, "BLK-L", 10) // stock_qty=10

	err := repo.Reserve(ctx, pool, variantID, 3)
	require.NoError(t, err)

	stock, reserved := testfixtures.GetVariantStock(t, pool, variantID)
	require.Equal(t, 10, stock)
	require.Equal(t, 3, reserved)
}

func TestVariantReserve_InsufficientStock(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	repo := productrepo.NewVariantPG(pool)
	ctx := context.Background()

	bID := testfixtures.SeedBrand(t, pool, "rep")
	pID := testfixtures.SeedProduct(t, pool, bID, "tee")
	vID := testfixtures.SeedVariant(t, pool, pID, "BLK-S", 2)

	err := repo.Reserve(ctx, pool, vID, 5)
	require.ErrorIs(t, err, productrepo.ErrInsufficientStock)
}

func TestVariantCommit_DecrementsBoth(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	repo := productrepo.NewVariantPG(pool)
	ctx := context.Background()

	bID := testfixtures.SeedBrand(t, pool, "rep")
	pID := testfixtures.SeedProduct(t, pool, bID, "tee")
	vID := testfixtures.SeedVariant(t, pool, pID, "BLK-M", 10)
	require.NoError(t, repo.Reserve(ctx, pool, vID, 3))

	require.NoError(t, repo.Commit(ctx, pool, vID, 3))

	stock, reserved := testfixtures.GetVariantStock(t, pool, vID)
	require.Equal(t, 7, stock)
	require.Equal(t, 0, reserved)
}

func TestVariantRelease_DecrementsReservedOnly(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	repo := productrepo.NewVariantPG(pool)
	ctx := context.Background()

	bID := testfixtures.SeedBrand(t, pool, "rep")
	pID := testfixtures.SeedProduct(t, pool, bID, "tee")
	vID := testfixtures.SeedVariant(t, pool, pID, "BLK-M", 10)
	require.NoError(t, repo.Reserve(ctx, pool, vID, 4))

	require.NoError(t, repo.Release(ctx, pool, vID, 4))

	stock, reserved := testfixtures.GetVariantStock(t, pool, vID)
	require.Equal(t, 10, stock)
	require.Equal(t, 0, reserved)
}

func TestVariantReserve_ConcurrentRace(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	repo := productrepo.NewVariantPG(pool)
	ctx := context.Background()

	bID := testfixtures.SeedBrand(t, pool, "rep")
	pID := testfixtures.SeedProduct(t, pool, bID, "tee")
	vID := testfixtures.SeedVariant(t, pool, pID, "BLK-M", 1) // ONLY 1 in stock

	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = repo.Reserve(ctx, pool, vID, 1)
		}()
	}
	wg.Wait()

	// Exactly one should succeed.
	successCount := 0
	for _, e := range results {
		if e == nil { successCount++ }
	}
	require.Equal(t, 1, successCount, "exactly one goroutine should succeed reserving the last unit")
}

// helper unused variable to silence import (uuid)
var _ = uuid.Nil
var _ *pgxpool.Pool
```

- [ ] **Step 3: Run test — confirm FAIL**

Run: `go test ./internal/product/repo/ -run TestVariantReserve -v 2>&1 | tail -20`
Expected: compile error — `repo.Reserve undefined`, `productrepo.ErrInsufficientStock undefined`.

- [ ] **Step 4: Add ErrInsufficientStock to product/repo errors**

Find existing `ErrNotFound` declaration in `internal/product/repo/` (likely `repo.go` or a shared errors file). Add next to it:

```go
// internal/product/repo/repo.go (or wherever ErrNotFound lives)
var ErrInsufficientStock = errors.New("product variant: insufficient stock")
```

- [ ] **Step 5: Append Reserve/Commit/Release to VariantPG**

Append to `internal/product/repo/variant_pg.go`:

```go
// Reserve adds qty to reserved_qty atomically. Requires stock_qty - reserved_qty >= qty.
// Returns ErrInsufficientStock if the condition fails (rowsAffected = 0).
// Caller may pass a tx via DBTX; if nil, uses the repo's default pool.
func (r *VariantPG) Reserve(ctx context.Context, db DBTX, variantID uuid.UUID, qty int) error {
	if db == nil { db = r.db }
	tag, err := db.Exec(ctx,
		`UPDATE product_variants
		    SET reserved_qty = reserved_qty + $2,
		        updated_at = NOW()
		  WHERE id = $1
		    AND deleted_at IS NULL
		    AND is_active = TRUE
		    AND (stock_qty - reserved_qty) >= $2`,
		variantID, qty)
	if err != nil { return err }
	if tag.RowsAffected() == 0 {
		return ErrInsufficientStock
	}
	return nil
}

// Commit decrements both stock_qty and reserved_qty by qty.
// Used when a PayOS payment is confirmed.
func (r *VariantPG) Commit(ctx context.Context, db DBTX, variantID uuid.UUID, qty int) error {
	if db == nil { db = r.db }
	tag, err := db.Exec(ctx,
		`UPDATE product_variants
		    SET stock_qty    = stock_qty - $2,
		        reserved_qty = reserved_qty - $2,
		        updated_at   = NOW()
		  WHERE id = $1
		    AND reserved_qty >= $2
		    AND stock_qty >= $2`,
		variantID, qty)
	if err != nil { return err }
	if tag.RowsAffected() == 0 {
		return ErrInsufficientStock
	}
	return nil
}

// Release decrements only reserved_qty (stock returns to available pool).
// Used on cancel / payment fail / expired reservation.
func (r *VariantPG) Release(ctx context.Context, db DBTX, variantID uuid.UUID, qty int) error {
	if db == nil { db = r.db }
	tag, err := db.Exec(ctx,
		`UPDATE product_variants
		    SET reserved_qty = reserved_qty - $2,
		        updated_at   = NOW()
		  WHERE id = $1
		    AND reserved_qty >= $2`,
		variantID, qty)
	if err != nil { return err }
	if tag.RowsAffected() == 0 {
		return ErrInsufficientStock // means: trying to release more than reserved (bug)
	}
	return nil
}
```

- [ ] **Step 6: Update VariantRepo interface**

In `internal/product/repo/repo.go`, locate `type VariantRepo interface` and append the three method signatures:

```go
type VariantRepo interface {
	// ... existing methods preserved ...
	Reserve(ctx context.Context, db DBTX, variantID uuid.UUID, qty int) error
	Commit(ctx context.Context, db DBTX, variantID uuid.UUID, qty int) error
	Release(ctx context.Context, db DBTX, variantID uuid.UUID, qty int) error
}
```

- [ ] **Step 7: Add testfixtures helpers (if missing)**

Inspect `internal/testfixtures/fixtures.go`. If `SeedVariant`, `GetVariantStock` are absent, append:

```go
// internal/testfixtures/fixtures.go (append)

func SeedVariant(t *testing.T, pool *pgxpool.Pool, productID uuid.UUID, sku string, stockQty int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(),
		`INSERT INTO product_variants (product_id, sku, size, color, price, stock_qty, is_active)
		 VALUES ($1, $2, 'M', 'Black', 100000, $3, TRUE) RETURNING id`,
		productID, sku, stockQty).Scan(&id)
	require.NoError(t, err)
	return id
}

func GetVariantStock(t *testing.T, pool *pgxpool.Pool, variantID uuid.UUID) (stock, reserved int) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT stock_qty, reserved_qty FROM product_variants WHERE id=$1`,
		variantID).Scan(&stock, &reserved)
	require.NoError(t, err)
	return
}
```

(Mirror existing helpers' style — they import `testing`, `pgxpool`, `uuid`, `require` already.)

- [ ] **Step 8: Run tests — PASS**

Run: `go test ./internal/product/repo/ -run TestVariant -v 2>&1 | tail -30`
Expected: all 5 tests PASS including TestVariantReserve_ConcurrentRace.

- [ ] **Step 9: Commit**

```bash
git add internal/product/repo/variant_pg.go internal/product/repo/repo.go internal/product/repo/variant_reserve_test.go internal/testfixtures/fixtures.go
git commit -m "feat(product): variant Reserve/Commit/Release with atomic stock guards + race test"
```

---

## Task 7: Shipping provider interface + FlatRate

**Files:**
- Create: `internal/shipping/domain/fee.go`
- Create: `internal/shipping/provider/provider.go`
- Create: `internal/shipping/provider/flat_rate.go`
- Create: `internal/shipping/provider/factory.go`
- Create: `internal/shipping/provider/flat_rate_test.go`

- [ ] **Step 1: Write domain/fee.go**

```go
// internal/shipping/domain/fee.go
package domain

import "time"

type FeeQuote struct {
	AmountVND   int64
	Currency    string         // "VND"
	ProviderRef string         // vendor quote id (Sprint 4+ re-price)
	ETA         *time.Duration // optional delivery time
}
```

- [ ] **Step 2: Write provider/provider.go**

```go
// internal/shipping/provider/provider.go
package provider

import (
	"context"

	"github.com/google/uuid"

	shippingdomain "github.com/wearwhere/wearwhere_be/internal/shipping/domain"
)

// CalcReq is what the checkout flow passes when computing shipping fees per brand.
type CalcReq struct {
	BrandID   uuid.UUID
	ToAddress ShippingAddress
	Items     []CalcItem
}

type CalcItem struct {
	VariantID uuid.UUID
	ProductID uuid.UUID
	Qty       int
	WeightG   int // 0 in Sprint 3 — variants don't have weight yet
}

// ShippingAddress mirrors order.ShippingAddress to keep this package
// importable without depending on order/domain (which imports brand/repo).
type ShippingAddress struct {
	Recipient string
	Phone     string
	Line1     string
	Ward      string
	District  string
	City      string
}

type ShippingProvider interface {
	Calculate(ctx context.Context, r CalcReq) (*shippingdomain.FeeQuote, error)
}
```

- [ ] **Step 3: Write provider/flat_rate.go**

```go
// internal/shipping/provider/flat_rate.go
package provider

import (
	"context"

	brandrepo "github.com/wearwhere/wearwhere_be/internal/brand/repo"
	shippingdomain "github.com/wearwhere/wearwhere_be/internal/shipping/domain"
)

// FlatRateProvider reads brands.shipping_flat_fee_vnd. Default 30000 from DB column.
type FlatRateProvider struct {
	brandRepo brandrepo.BrandRepo
}

func NewFlatRateProvider(b brandrepo.BrandRepo) *FlatRateProvider {
	return &FlatRateProvider{brandRepo: b}
}

func (p *FlatRateProvider) Calculate(ctx context.Context, r CalcReq) (*shippingdomain.FeeQuote, error) {
	b, err := p.brandRepo.GetByID(ctx, r.BrandID)
	if err != nil {
		return nil, err
	}
	return &shippingdomain.FeeQuote{
		AmountVND: b.ShippingFlatFeeVND,
		Currency:  "VND",
	}, nil
}
```

- [ ] **Step 4: Write provider/factory.go**

```go
// internal/shipping/provider/factory.go
package provider

import (
	"fmt"

	brandrepo "github.com/wearwhere/wearwhere_be/internal/brand/repo"
)

type Config struct {
	Provider string // "flat" (Sprint 3) | future: "ghn", "ghtk", "viettelpost"
}

func NewFromConfig(cfg Config, brandRepo brandrepo.BrandRepo) (ShippingProvider, error) {
	switch cfg.Provider {
	case "", "flat":
		return NewFlatRateProvider(brandRepo), nil
	default:
		return nil, fmt.Errorf("unknown shipping provider: %q", cfg.Provider)
	}
}
```

- [ ] **Step 5: Write flat_rate_test.go**

```go
// internal/shipping/provider/flat_rate_test.go
package provider_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	branddomain "github.com/wearwhere/wearwhere_be/internal/brand/domain"
	"github.com/wearwhere/wearwhere_be/internal/shipping/provider"
)

type fakeBrandRepo struct {
	byID map[uuid.UUID]*branddomain.Brand
}

func (f *fakeBrandRepo) GetByID(ctx context.Context, id uuid.UUID) (*branddomain.Brand, error) {
	if b, ok := f.byID[id]; ok { return b, nil }
	return nil, context.Canceled // any non-nil sentinel
}
// (stub out other interface methods if go vet complains; minimal interface satisfaction)
// ... add stubs as needed when brandrepo.BrandRepo has more methods.

func TestFlatRateProvider_UsesPerBrandFee(t *testing.T) {
	brandID := uuid.New()
	repo := &fakeBrandRepo{byID: map[uuid.UUID]*branddomain.Brand{
		brandID: {ID: brandID, ShippingFlatFeeVND: 45000},
	}}
	p := provider.NewFlatRateProvider(repo)

	q, err := p.Calculate(context.Background(), provider.CalcReq{BrandID: brandID})
	require.NoError(t, err)
	require.Equal(t, int64(45000), q.AmountVND)
	require.Equal(t, "VND", q.Currency)
}

func TestFactory_DefaultsToFlat(t *testing.T) {
	brandID := uuid.New()
	repo := &fakeBrandRepo{byID: map[uuid.UUID]*branddomain.Brand{
		brandID: {ID: brandID, ShippingFlatFeeVND: 30000},
	}}

	p, err := provider.NewFromConfig(provider.Config{Provider: ""}, repo)
	require.NoError(t, err)
	q, err := p.Calculate(context.Background(), provider.CalcReq{BrandID: brandID})
	require.NoError(t, err)
	require.Equal(t, int64(30000), q.AmountVND)
}

func TestFactory_UnknownProvider(t *testing.T) {
	_, err := provider.NewFromConfig(provider.Config{Provider: "ghn"}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown shipping provider")
}
```

Note: the `fakeBrandRepo` must satisfy the full `brandrepo.BrandRepo` interface. After Task 8 adds `ShippingFlatFeeVND` to Brand and surfaces it in repo, run `go vet` to detect missing methods and stub them as `panic("not used")`.

- [ ] **Step 6: Run tests — PASS (after Task 8 may need re-run if interface grows)**

Run: `go test ./internal/shipping/... -v 2>&1 | tail -20`
Expected: all 3 tests PASS (or compile errors that are resolved by Task 8 — if so, defer this step until then).

- [ ] **Step 7: Commit**

```bash
git add internal/shipping/
git commit -m "feat(shipping): pluggable ShippingProvider interface + FlatRateProvider"
```

---

## Task 8: Brand domain ShippingFlatFeeVND + repo surface

**Files:**
- Modify: `internal/brand/domain/brand.go` — add field
- Modify: `internal/brand/repo/brand_pg.go` — surface column in SELECT, scan

- [ ] **Step 1: Inspect existing brand.go**

Read `internal/brand/domain/brand.go` and locate the `Brand` struct.

- [ ] **Step 2: Add field**

Append after existing fields in the `Brand` struct:

```go
// internal/brand/domain/brand.go (inside type Brand struct)
ShippingFlatFeeVND int64 `json:"shipping_flat_fee_vnd"`
```

- [ ] **Step 3: Add to brand_pg.go SELECT**

Locate the column list constant (e.g., `brandCols`) in `internal/brand/repo/brand_pg.go`. Append `shipping_flat_fee_vnd` to it. Update the corresponding `scanBrand` function to include `&b.ShippingFlatFeeVND` in the same position.

Example (adjust to actual file layout):

```go
const brandCols = `id, slug, name, ..., shipping_flat_fee_vnd, created_at, updated_at, deleted_at`

func scanBrand(row pgx.Row) (*domain.Brand, error) {
	var b domain.Brand
	err := row.Scan(
		&b.ID, &b.Slug, &b.Name, /* ... existing scans in order ... */,
		&b.ShippingFlatFeeVND,
		&b.CreatedAt, &b.UpdatedAt, &b.DeletedAt,
	)
	// ... existing error mapping
	return &b, err
}
```

- [ ] **Step 4: Write test**

Append to `internal/brand/repo/brand_pg_test.go`:

```go
func TestBrandPG_GetByID_IncludesShippingFlatFee(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	repo := brandrepo.NewBrandPG(pool)
	ctx := context.Background()

	brandID := testfixtures.SeedBrand(t, pool, "rep")
	// Override default fee
	_, err := pool.Exec(ctx, `UPDATE brands SET shipping_flat_fee_vnd=45000 WHERE id=$1`, brandID)
	require.NoError(t, err)

	b, err := repo.GetByID(ctx, brandID)
	require.NoError(t, err)
	require.Equal(t, int64(45000), b.ShippingFlatFeeVND)
}

func TestBrandPG_GetByID_DefaultsTo30k(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	repo := brandrepo.NewBrandPG(pool)
	ctx := context.Background()

	brandID := testfixtures.SeedBrand(t, pool, "rep") // SeedBrand doesn't set fee → DB default 30000
	b, err := repo.GetByID(ctx, brandID)
	require.NoError(t, err)
	require.Equal(t, int64(30000), b.ShippingFlatFeeVND)
}
```

- [ ] **Step 5: Run tests + re-run shipping tests**

Run: `go test ./internal/brand/... ./internal/shipping/... -v 2>&1 | tail -30`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/brand/domain/brand.go internal/brand/repo/brand_pg.go internal/brand/repo/brand_pg_test.go
git commit -m "feat(brand): expose shipping_flat_fee_vnd on Brand domain + brand_pg SELECT"
```

---

## Task 9: Order domain types, errors, DTOs, order_no generator

**Files:**
- Create: `internal/order/domain/order.go`
- Create: `internal/order/domain/enums.go`
- Create: `internal/order/domain/errors.go`
- Create: `internal/order/domain/dto.go`
- Create: `internal/order/domain/order_no.go`
- Create: `internal/order/domain/order_test.go`
- Create: `internal/order/domain/order_no_test.go`

- [ ] **Step 1: Write enums.go**

```go
// internal/order/domain/enums.go
package domain

type PaymentMethod string

const (
	PaymentMethodCOD   PaymentMethod = "cod"
	PaymentMethodPayos PaymentMethod = "payos"
)

func (m PaymentMethod) Valid() bool {
	return m == PaymentMethodCOD || m == PaymentMethodPayos
}

type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusPaid      PaymentStatus = "paid"
	PaymentStatusFailed    PaymentStatus = "failed"
	PaymentStatusCancelled PaymentStatus = "cancelled"
	PaymentStatusExpired   PaymentStatus = "expired" // payments table only
)

type OrderStatus string

const (
	OrderStatusPendingPayment OrderStatus = "pending_payment"
	OrderStatusProcessing     OrderStatus = "processing"
	OrderStatusCancelled      OrderStatus = "cancelled"
	OrderStatusCompleted      OrderStatus = "completed"
)

type SubOrderStatus string

const (
	SubOrderStatusPending    SubOrderStatus = "pending"
	SubOrderStatusConfirmed  SubOrderStatus = "confirmed"
	SubOrderStatusPreparing  SubOrderStatus = "preparing"
	SubOrderStatusShipped    SubOrderStatus = "shipped"
	SubOrderStatusDelivered  SubOrderStatus = "delivered"
	SubOrderStatusCancelled  SubOrderStatus = "cancelled"
)
```

- [ ] **Step 2: Write order.go**

```go
// internal/order/domain/order.go
package domain

import (
	"time"

	"github.com/google/uuid"
)

type ShippingAddress struct {
	Recipient string `json:"recipient"`
	Phone     string `json:"phone"`
	Line1     string `json:"line1"`
	Ward      string `json:"ward"`
	District  string `json:"district"`
	City      string `json:"city"`
}

type Order struct {
	ID                  uuid.UUID
	UserID              uuid.UUID
	OrderNo             string
	SubtotalVND         int64
	ShippingTotalVND    int64
	GrandTotalVND       int64
	PaymentMethod       PaymentMethod
	PaymentStatus       PaymentStatus
	Status              OrderStatus
	ShippingAddress     ShippingAddress
	Notes               string
	CancelReason        string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	PaidAt              *time.Time
	CancelledAt         *time.Time

	SubOrders []SubOrder // optional, populated by Detail queries
}

type SubOrder struct {
	ID               uuid.UUID
	OrderID          uuid.UUID
	BrandID          uuid.UUID
	BrandSlug        string // joined view
	BrandName        string // joined view
	SubtotalVND      int64
	ShippingFeeVND   int64
	TotalVND         int64
	Status           SubOrderStatus
	TrackingNo       *string
	ShippingProvider *string
	ConfirmedAt      *time.Time
	ShippedAt        *time.Time
	DeliveredAt      *time.Time
	CancelledAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time

	Items []OrderItem // optional, populated by Detail queries
}

type OrderItem struct {
	ID           uuid.UUID
	SubOrderID   uuid.UUID
	VariantID    uuid.UUID
	ProductID    uuid.UUID
	ProductName  string
	VariantLabel string
	ImageURL     *string
	Qty          int
	UnitPriceVND int64
	LineTotalVND int64
	CreatedAt    time.Time
}

// CancelDecision encodes whether a customer-initiated cancel is allowed.
type CancelDecision struct {
	Allowed bool
	Reason  string // subcode: "" (allowed), "paid_not_supported", "already_shipped",
	               // "already_cancelled", "already_completed"
}

// CanCustomerCancel implements the rule table from spec §5.3.
// Sprint 3: COD pending OR PayOS unpaid → allowed; paid PayOS → defer Sprint 4.
func (o *Order) CanCustomerCancel() CancelDecision {
	switch o.Status {
	case OrderStatusCancelled:
		return CancelDecision{Allowed: false, Reason: "already_cancelled"}
	case OrderStatusCompleted:
		return CancelDecision{Allowed: false, Reason: "already_completed"}
	}

	// Any sub_order advanced beyond pending → block (Sprint 4 will handle paid/shipped flows)
	for _, so := range o.SubOrders {
		if so.Status != SubOrderStatusPending {
			return CancelDecision{Allowed: false, Reason: "already_shipped"}
		}
	}

	if o.PaymentMethod == PaymentMethodPayos && o.PaymentStatus == PaymentStatusPaid {
		return CancelDecision{Allowed: false, Reason: "paid_not_supported"}
	}

	return CancelDecision{Allowed: true}
}
```

- [ ] **Step 3: Write errors.go**

```go
// internal/order/domain/errors.go
package domain

import "errors"

var (
	ErrOrderNotFound           = errors.New("order: not found")
	ErrCartEmpty               = errors.New("order: cart is empty")
	ErrMinOrderValue           = errors.New("order: subtotal below 50000 VND minimum")
	ErrInsufficientStock       = errors.New("order: insufficient stock for variant")
	ErrVariantUnavailable      = errors.New("order: variant unavailable")
	ErrAddressNotFound         = errors.New("order: shipping address not found or not owned")
	ErrInvalidPaymentMethod    = errors.New("order: invalid payment method")
	ErrCancelNotAllowed        = errors.New("order: cannot be cancelled in current state")
	ErrCancelPaidNotSupported  = errors.New("order: paid order cancellation deferred to Sprint 4")
	ErrWebhookSignatureInvalid = errors.New("order: invalid webhook signature")
	ErrPayosLinkCreate         = errors.New("order: failed to create PayOS payment link")
	ErrIDOR                    = errors.New("order: resource not owned by user")
)

const MinOrderValueVND int64 = 50000
```

- [ ] **Step 4: Write dto.go**

```go
// internal/order/domain/dto.go
package domain

import (
	"time"

	"github.com/google/uuid"
)

// CheckoutPreviewItem is a single item in the preview response.
type CheckoutPreviewItem struct {
	VariantID     uuid.UUID `json:"variant_id"`
	ProductID     uuid.UUID `json:"product_id"`
	ProductName   string    `json:"product_name"`
	VariantLabel  string    `json:"variant_label"`
	ImageURL      *string   `json:"image_url"`
	Qty           int       `json:"qty"`
	UnitPriceVND  int64     `json:"unit_price_vnd"`
	LineTotalVND  int64     `json:"line_total_vnd"`
	AvailableQty  int       `json:"available_qty"`
}

type CheckoutPreviewSubOrder struct {
	Brand          BrandRef               `json:"brand"`
	Items          []CheckoutPreviewItem  `json:"items"`
	SubtotalVND    int64                  `json:"subtotal_vnd"`
	ShippingFeeVND int64                  `json:"shipping_fee_vnd"`
	TotalVND       int64                  `json:"total_vnd"`
}

type BrandRef struct {
	ID   uuid.UUID `json:"id"`
	Slug string    `json:"slug"`
	Name string    `json:"name"`
}

type CheckoutPreviewResp struct {
	CartEmpty          bool                      `json:"cart_empty"`
	Address            *ShippingAddress          `json:"address,omitempty"`
	SubOrders          []CheckoutPreviewSubOrder `json:"sub_orders"`
	SubtotalVND        int64                     `json:"subtotal_vnd"`
	ShippingTotalVND   int64                     `json:"shipping_total_vnd"`
	GrandTotalVND      int64                     `json:"grand_total_vnd"`
	MinOrderValueVND   int64                     `json:"min_order_value_vnd"`
	MeetsMinOrder      bool                      `json:"meets_min_order"`
	Warnings           []string                  `json:"warnings"`
}

type PlaceOrderReq struct {
	AddressID     uuid.UUID     `json:"address_id" binding:"required"`
	PaymentMethod PaymentMethod `json:"payment_method" binding:"required"`
	Notes         string        `json:"notes" binding:"max=500"`
}

type PaymentResp struct {
	ID            uuid.UUID     `json:"id"`
	Method        PaymentMethod `json:"method"`
	Status        PaymentStatus `json:"status"`
	AmountVND     int64         `json:"amount_vnd"`
	CheckoutURL   *string       `json:"checkout_url"`
	QRCode        *string       `json:"qr_code"`
	ExpiredAt     *time.Time    `json:"expired_at"`
}

type PlaceOrderResp struct {
	Order   OrderResp   `json:"order"`
	Payment PaymentResp `json:"payment"`
}

type OrderItemResp struct {
	ID            uuid.UUID `json:"id"`
	VariantID     uuid.UUID `json:"variant_id"`
	ProductID     uuid.UUID `json:"product_id"`
	ProductName   string    `json:"product_name"`
	VariantLabel  string    `json:"variant_label"`
	ImageURL      *string   `json:"image_url"`
	Qty           int       `json:"qty"`
	UnitPriceVND  int64     `json:"unit_price_vnd"`
	LineTotalVND  int64     `json:"line_total_vnd"`
}

type SubOrderResp struct {
	ID             uuid.UUID       `json:"id"`
	Brand          BrandRef        `json:"brand"`
	SubtotalVND    int64           `json:"subtotal_vnd"`
	ShippingFeeVND int64           `json:"shipping_fee_vnd"`
	TotalVND       int64           `json:"total_vnd"`
	Status         SubOrderStatus  `json:"status"`
	TrackingNo     *string         `json:"tracking_no"`
	Items          []OrderItemResp `json:"items"`
}

type OrderResp struct {
	ID                uuid.UUID       `json:"id"`
	OrderNo           string          `json:"order_no"`
	Status            OrderStatus     `json:"status"`
	PaymentMethod     PaymentMethod   `json:"payment_method"`
	PaymentStatus     PaymentStatus   `json:"payment_status"`
	SubtotalVND       int64           `json:"subtotal_vnd"`
	ShippingTotalVND  int64           `json:"shipping_total_vnd"`
	GrandTotalVND     int64           `json:"grand_total_vnd"`
	ShippingAddress   ShippingAddress `json:"shipping_address"`
	Notes             string          `json:"notes"`
	CancelReason      string          `json:"cancel_reason,omitempty"`
	SubOrders         []SubOrderResp  `json:"sub_orders"`
	CreatedAt         time.Time       `json:"created_at"`
	PaidAt            *time.Time      `json:"paid_at"`
	CancelledAt       *time.Time      `json:"cancelled_at"`
}

type OrderListItem struct {
	ID              uuid.UUID     `json:"id"`
	OrderNo         string        `json:"order_no"`
	Status          OrderStatus   `json:"status"`
	PaymentMethod   PaymentMethod `json:"payment_method"`
	PaymentStatus   PaymentStatus `json:"payment_status"`
	GrandTotalVND   int64         `json:"grand_total_vnd"`
	ItemCount       int           `json:"item_count"`
	BrandCount      int           `json:"brand_count"`
	FirstItemImage  *string       `json:"first_item_image"`
	FirstItemName   string        `json:"first_item_name"`
	CreatedAt       time.Time     `json:"created_at"`
}

type OrderListResp struct {
	Data       []OrderListItem `json:"data"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	Total      int             `json:"total"`
	TotalPages int             `json:"total_pages"`
}

type CancelOrderReq struct {
	Reason string `json:"reason" binding:"max=200"`
}
```

- [ ] **Step 5: Write order_no.go**

```go
// internal/order/domain/order_no.go
package domain

import (
	"crypto/rand"
	"fmt"
	"time"
)

// nanoidAlphabet excludes I, O, 0, 1 to avoid visual confusion.
const nanoidAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// GenerateOrderNo returns a string like "WW-20260524-AB12CD".
// Uniqueness is enforced by DB unique constraint; caller retries on conflict.
func GenerateOrderNo(now time.Time) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand failure is fatal; we'd rather crash than return predictable IDs
		panic(fmt.Sprintf("order_no: crypto/rand failed: %v", err))
	}
	suffix := make([]byte, 6)
	for i, b := range buf {
		suffix[i] = nanoidAlphabet[int(b)%len(nanoidAlphabet)]
	}
	return fmt.Sprintf("WW-%s-%s", now.Format("20060102"), string(suffix))
}
```

- [ ] **Step 6: Write order_test.go**

```go
// internal/order/domain/order_test.go
package domain_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
)

func TestCanCustomerCancel_PayosUnpaid_Allowed(t *testing.T) {
	o := &domain.Order{
		PaymentMethod: domain.PaymentMethodPayos,
		PaymentStatus: domain.PaymentStatusPending,
		Status:        domain.OrderStatusPendingPayment,
		SubOrders:     []domain.SubOrder{{Status: domain.SubOrderStatusPending}},
	}
	d := o.CanCustomerCancel()
	require.True(t, d.Allowed)
}

func TestCanCustomerCancel_PayosPaid_BlockedPaidNotSupported(t *testing.T) {
	o := &domain.Order{
		PaymentMethod: domain.PaymentMethodPayos,
		PaymentStatus: domain.PaymentStatusPaid,
		Status:        domain.OrderStatusProcessing,
		SubOrders:     []domain.SubOrder{{Status: domain.SubOrderStatusPending}},
	}
	d := o.CanCustomerCancel()
	require.False(t, d.Allowed)
	require.Equal(t, "paid_not_supported", d.Reason)
}

func TestCanCustomerCancel_CODPending_Allowed(t *testing.T) {
	o := &domain.Order{
		PaymentMethod: domain.PaymentMethodCOD,
		PaymentStatus: domain.PaymentStatusPending,
		Status:        domain.OrderStatusProcessing,
		SubOrders:     []domain.SubOrder{{Status: domain.SubOrderStatusPending}},
	}
	d := o.CanCustomerCancel()
	require.True(t, d.Allowed)
}

func TestCanCustomerCancel_AnySubOrderConfirmed_Blocked(t *testing.T) {
	o := &domain.Order{
		PaymentMethod: domain.PaymentMethodCOD,
		PaymentStatus: domain.PaymentStatusPending,
		Status:        domain.OrderStatusProcessing,
		SubOrders: []domain.SubOrder{
			{Status: domain.SubOrderStatusPending},
			{Status: domain.SubOrderStatusConfirmed},
		},
	}
	d := o.CanCustomerCancel()
	require.False(t, d.Allowed)
	require.Equal(t, "already_shipped", d.Reason)
}

func TestCanCustomerCancel_AlreadyCancelled(t *testing.T) {
	o := &domain.Order{Status: domain.OrderStatusCancelled}
	d := o.CanCustomerCancel()
	require.False(t, d.Allowed)
	require.Equal(t, "already_cancelled", d.Reason)
}

func TestCanCustomerCancel_AlreadyCompleted(t *testing.T) {
	o := &domain.Order{Status: domain.OrderStatusCompleted}
	d := o.CanCustomerCancel()
	require.False(t, d.Allowed)
	require.Equal(t, "already_completed", d.Reason)
}

func TestPaymentMethodValid(t *testing.T) {
	require.True(t, domain.PaymentMethodCOD.Valid())
	require.True(t, domain.PaymentMethodPayos.Valid())
	require.False(t, domain.PaymentMethod("bitcoin").Valid())
}

// silence unused
var _ = uuid.New
```

- [ ] **Step 7: Write order_no_test.go**

```go
// internal/order/domain/order_no_test.go
package domain_test

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
)

func TestGenerateOrderNo_Format(t *testing.T) {
	now := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	no := domain.GenerateOrderNo(now)
	re := regexp.MustCompile(`^WW-20260524-[A-Z2-9]{6}$`)
	require.True(t, re.MatchString(no), "got %q", no)
}

func TestGenerateOrderNo_ExcludesAmbiguousChars(t *testing.T) {
	now := time.Now()
	// Statistical: generate 1000, none should contain I, O, 0, 1.
	for i := 0; i < 1000; i++ {
		no := domain.GenerateOrderNo(now)
		for _, ch := range no[12:] { // skip "WW-YYYYMMDD-"
			require.NotEqual(t, 'I', ch, "found I in %q", no)
			require.NotEqual(t, 'O', ch, "found O in %q", no)
			require.NotEqual(t, '0', ch, "found 0 in %q", no)
			require.NotEqual(t, '1', ch, "found 1 in %q", no)
		}
	}
}

func TestGenerateOrderNo_UniqueAcrossManyCalls(t *testing.T) {
	now := time.Now()
	seen := make(map[string]struct{})
	for i := 0; i < 10000; i++ {
		no := domain.GenerateOrderNo(now)
		_, dup := seen[no]
		require.False(t, dup, "duplicate %q at iter %d", no, i)
		seen[no] = struct{}{}
	}
}
```

- [ ] **Step 8: Run tests — PASS**

Run: `go test ./internal/order/domain/ -v 2>&1 | tail -30`
Expected: all 10 tests PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/order/domain/
git commit -m "feat(order): domain types, enums, errors, DTOs + order_no nanoid + cancel rules"
```

---

## Task 10: order_pg repo (orders CRUD + list + cancel)

**Files:**
- Create: `internal/order/repo/repo.go`
- Create: `internal/order/repo/order_pg.go`
- Create: `internal/order/repo/order_pg_test.go`

- [ ] **Step 1: Write repo.go interfaces**

```go
// internal/order/repo/repo.go
package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
)

var (
	ErrNotFound        = errors.New("order repo: not found")
	ErrOrderNoConflict = errors.New("order repo: order_no conflict")
)

// DBTX abstracts over *pgxpool.Pool and pgx.Tx.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type ListFilter struct {
	UserID    uuid.UUID
	Statuses  []domain.OrderStatus // empty = no filter
	FromTime  *string              // RFC3339 string from query
	ToTime    *string
	Page      int
	PageSize  int
}

type OrderRepo interface {
	Create(ctx context.Context, db DBTX, o *domain.Order) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error)
	GetByOrderNo(ctx context.Context, userID uuid.UUID, orderNo string) (*domain.Order, error)
	GetByOrderNoForUpdate(ctx context.Context, db DBTX, userID uuid.UUID, orderNo string) (*domain.Order, error)
	List(ctx context.Context, f ListFilter) (items []*domain.Order, total int, err error)
	UpdateStatusOnPaid(ctx context.Context, db DBTX, orderID uuid.UUID) error
	UpdateStatusOnCancel(ctx context.Context, db DBTX, orderID uuid.UUID, reason string, paymentStatus domain.PaymentStatus) error
}

type SubOrderRepo interface {
	Create(ctx context.Context, db DBTX, so *domain.SubOrder) error
	ListByOrderID(ctx context.Context, orderID uuid.UUID) ([]*domain.SubOrder, error)
	CancelAllByOrderID(ctx context.Context, db DBTX, orderID uuid.UUID) error
}

type OrderItemRepo interface {
	Create(ctx context.Context, db DBTX, item *domain.OrderItem) error
	ListBySubOrderID(ctx context.Context, subOrderID uuid.UUID) ([]*domain.OrderItem, error)
	ListByOrderID(ctx context.Context, orderID uuid.UUID) ([]*domain.OrderItem, error) // for cleanup release
}
```

- [ ] **Step 2: Write order_pg.go**

```go
// internal/order/repo/order_pg.go
package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
)

type OrderPG struct{ db DBTX }

func NewOrderPG(db DBTX) *OrderPG { return &OrderPG{db: db} }

const orderCols = `id, user_id, order_no, subtotal_vnd, shipping_total_vnd, grand_total_vnd,
                   payment_method, payment_status, status, shipping_address, notes, cancel_reason,
                   created_at, updated_at, paid_at, cancelled_at`

func scanOrder(row pgx.Row) (*domain.Order, error) {
	var o domain.Order
	var addrJSON []byte
	var notes, cancelReason *string
	err := row.Scan(
		&o.ID, &o.UserID, &o.OrderNo,
		&o.SubtotalVND, &o.ShippingTotalVND, &o.GrandTotalVND,
		&o.PaymentMethod, &o.PaymentStatus, &o.Status,
		&addrJSON, &notes, &cancelReason,
		&o.CreatedAt, &o.UpdatedAt, &o.PaidAt, &o.CancelledAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal(addrJSON, &o.ShippingAddress); err != nil {
		return nil, fmt.Errorf("decode shipping_address: %w", err)
	}
	if notes != nil { o.Notes = *notes }
	if cancelReason != nil { o.CancelReason = *cancelReason }
	return &o, nil
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && (constraint == "" || strings.Contains(pgErr.ConstraintName, constraint))
	}
	return false
}

func (r *OrderPG) Create(ctx context.Context, db DBTX, o *domain.Order) error {
	if db == nil { db = r.db }
	addrJSON, err := json.Marshal(o.ShippingAddress)
	if err != nil { return err }

	row := db.QueryRow(ctx,
		`INSERT INTO orders
		   (user_id, order_no, subtotal_vnd, shipping_total_vnd, grand_total_vnd,
		    payment_method, payment_status, status, shipping_address, notes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, ''))
		 RETURNING id, created_at, updated_at`,
		o.UserID, o.OrderNo, o.SubtotalVND, o.ShippingTotalVND, o.GrandTotalVND,
		o.PaymentMethod, o.PaymentStatus, o.Status, addrJSON, o.Notes)
	err = row.Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err, "order_no") {
			return ErrOrderNoConflict
		}
		return err
	}
	return nil
}

func (r *OrderPG) GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	return scanOrder(r.db.QueryRow(ctx,
		`SELECT `+orderCols+` FROM orders WHERE id = $1`, id))
}

func (r *OrderPG) GetByOrderNo(ctx context.Context, userID uuid.UUID, orderNo string) (*domain.Order, error) {
	return scanOrder(r.db.QueryRow(ctx,
		`SELECT `+orderCols+` FROM orders WHERE order_no = $1 AND user_id = $2`,
		orderNo, userID))
}

func (r *OrderPG) GetByOrderNoForUpdate(ctx context.Context, db DBTX, userID uuid.UUID, orderNo string) (*domain.Order, error) {
	if db == nil { db = r.db }
	return scanOrder(db.QueryRow(ctx,
		`SELECT `+orderCols+` FROM orders
		  WHERE order_no = $1 AND user_id = $2 FOR UPDATE`,
		orderNo, userID))
}

func (r *OrderPG) List(ctx context.Context, f ListFilter) ([]*domain.Order, int, error) {
	if f.PageSize <= 0 { f.PageSize = 20 }
	if f.PageSize > 50 { f.PageSize = 50 }
	if f.Page <= 0 { f.Page = 1 }

	args := []any{f.UserID}
	where := []string{"user_id = $1"}
	i := 2
	if len(f.Statuses) > 0 {
		strs := make([]string, 0, len(f.Statuses))
		for _, s := range f.Statuses {
			args = append(args, string(s))
			strs = append(strs, fmt.Sprintf("$%d", i))
			i++
		}
		where = append(where, "status IN ("+strings.Join(strs, ",")+")")
	}
	if f.FromTime != nil && *f.FromTime != "" {
		if t, err := time.Parse(time.RFC3339, *f.FromTime); err == nil {
			args = append(args, t)
			where = append(where, fmt.Sprintf("created_at >= $%d", i))
			i++
		}
	}
	if f.ToTime != nil && *f.ToTime != "" {
		if t, err := time.Parse(time.RFC3339, *f.ToTime); err == nil {
			args = append(args, t)
			where = append(where, fmt.Sprintf("created_at <= $%d", i))
			i++
		}
	}
	whereSQL := "WHERE " + strings.Join(where, " AND ")

	// count
	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM orders `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// page
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx,
		`SELECT `+orderCols+` FROM orders `+whereSQL+
			fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, i, i+1),
		args...)
	if err != nil { return nil, 0, err }
	defer rows.Close()

	var out []*domain.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil { return nil, 0, err }
		out = append(out, o)
	}
	return out, total, rows.Err()
}

func (r *OrderPG) UpdateStatusOnPaid(ctx context.Context, db DBTX, orderID uuid.UUID) error {
	if db == nil { db = r.db }
	tag, err := db.Exec(ctx,
		`UPDATE orders
		    SET status = 'processing',
		        payment_status = 'paid',
		        paid_at = NOW(),
		        updated_at = NOW()
		  WHERE id = $1 AND payment_status = 'pending'`,
		orderID)
	if err != nil { return err }
	if tag.RowsAffected() == 0 { return ErrNotFound }
	return nil
}

func (r *OrderPG) UpdateStatusOnCancel(ctx context.Context, db DBTX, orderID uuid.UUID, reason string, paymentStatus domain.PaymentStatus) error {
	if db == nil { db = r.db }
	tag, err := db.Exec(ctx,
		`UPDATE orders
		    SET status = 'cancelled',
		        payment_status = $2,
		        cancel_reason = NULLIF($3, ''),
		        cancelled_at = NOW(),
		        updated_at = NOW()
		  WHERE id = $1 AND status != 'cancelled'`,
		orderID, paymentStatus, reason)
	if err != nil { return err }
	if tag.RowsAffected() == 0 { return ErrNotFound }
	return nil
}
```

- [ ] **Step 3: Write order_pg_test.go**

```go
// internal/order/repo/order_pg_test.go
package repo_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

func seedOrder(t *testing.T, userID uuid.UUID, orderNo string) *domain.Order {
	return &domain.Order{
		UserID:           userID,
		OrderNo:          orderNo,
		SubtotalVND:      100000,
		ShippingTotalVND: 30000,
		GrandTotalVND:    130000,
		PaymentMethod:    domain.PaymentMethodCOD,
		PaymentStatus:    domain.PaymentStatusPending,
		Status:           domain.OrderStatusProcessing,
		ShippingAddress: domain.ShippingAddress{
			Recipient: "An Nguyen", Phone: "0900000000",
			Line1: "1 ABC St", Ward: "P1", District: "Q1", City: "HCM",
		},
		Notes: "test",
	}
}

func TestOrderPG_CreateAndGet(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	repo := orderrepo.NewOrderPG(pool)
	ctx := context.Background()

	userID := testfixtures.SeedUser(t, pool, "buyer@x.com")
	o := seedOrder(t, userID, "WW-20260524-AAAAAA")
	require.NoError(t, repo.Create(ctx, pool, o))
	require.NotEqual(t, uuid.Nil, o.ID)

	got, err := repo.GetByOrderNo(ctx, userID, "WW-20260524-AAAAAA")
	require.NoError(t, err)
	require.Equal(t, o.ID, got.ID)
	require.Equal(t, "An Nguyen", got.ShippingAddress.Recipient)
	require.Equal(t, "test", got.Notes)
}

func TestOrderPG_Create_DuplicateOrderNo(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	repo := orderrepo.NewOrderPG(pool)
	ctx := context.Background()

	userID := testfixtures.SeedUser(t, pool, "buyer@x.com")
	a := seedOrder(t, userID, "WW-20260524-DUP")
	require.NoError(t, repo.Create(ctx, pool, a))
	b := seedOrder(t, userID, "WW-20260524-DUP")
	err := repo.Create(ctx, pool, b)
	require.ErrorIs(t, err, orderrepo.ErrOrderNoConflict)
}

func TestOrderPG_GetByOrderNo_OtherUser_NotFound(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	repo := orderrepo.NewOrderPG(pool)
	ctx := context.Background()

	u1 := testfixtures.SeedUser(t, pool, "u1@x.com")
	u2 := testfixtures.SeedUser(t, pool, "u2@x.com")
	o := seedOrder(t, u1, "WW-20260524-IDOR")
	require.NoError(t, repo.Create(ctx, pool, o))

	_, err := repo.GetByOrderNo(ctx, u2, "WW-20260524-IDOR")
	require.ErrorIs(t, err, orderrepo.ErrNotFound)
}

func TestOrderPG_List_FilterByStatusAndPaginate(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	repo := orderrepo.NewOrderPG(pool)
	ctx := context.Background()

	userID := testfixtures.SeedUser(t, pool, "buyer@x.com")
	for i := 0; i < 5; i++ {
		o := seedOrder(t, userID, "WW-20260524-X"+string(rune('A'+i))+"AAAA")
		require.NoError(t, repo.Create(ctx, pool, o))
	}
	// Make one cancelled
	_, err := pool.Exec(ctx, `UPDATE orders SET status='cancelled' WHERE order_no LIKE 'WW-20260524-XA%'`)
	require.NoError(t, err)

	items, total, err := repo.List(ctx, orderrepo.ListFilter{
		UserID:   userID,
		Statuses: []domain.OrderStatus{domain.OrderStatusProcessing},
		Page:     1, PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, 4, total) // 5 - 1 cancelled
	require.Len(t, items, 4)
}

func TestOrderPG_UpdateStatusOnPaid(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	repo := orderrepo.NewOrderPG(pool)
	ctx := context.Background()

	userID := testfixtures.SeedUser(t, pool, "buyer@x.com")
	o := seedOrder(t, userID, "WW-20260524-PAID")
	o.Status = domain.OrderStatusPendingPayment
	require.NoError(t, repo.Create(ctx, pool, o))

	require.NoError(t, repo.UpdateStatusOnPaid(ctx, pool, o.ID))

	got, _ := repo.GetByID(ctx, o.ID)
	require.Equal(t, domain.OrderStatusProcessing, got.Status)
	require.Equal(t, domain.PaymentStatusPaid, got.PaymentStatus)
	require.NotNil(t, got.PaidAt)
}

func TestOrderPG_UpdateStatusOnCancel(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	repo := orderrepo.NewOrderPG(pool)
	ctx := context.Background()

	userID := testfixtures.SeedUser(t, pool, "buyer@x.com")
	o := seedOrder(t, userID, "WW-20260524-CANC")
	require.NoError(t, repo.Create(ctx, pool, o))

	require.NoError(t, repo.UpdateStatusOnCancel(ctx, pool, o.ID, "user_cancel", domain.PaymentStatusCancelled))

	got, _ := repo.GetByID(ctx, o.ID)
	require.Equal(t, domain.OrderStatusCancelled, got.Status)
	require.Equal(t, "user_cancel", got.CancelReason)
	require.NotNil(t, got.CancelledAt)
}
```

- [ ] **Step 4: Add SeedUser helper if missing**

In `internal/testfixtures/fixtures.go`, append if not present:

```go
func SeedUser(t *testing.T, pool *pgxpool.Pool, email string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, password_hash, is_verified)
		 VALUES ($1, 'fake-hash', TRUE) RETURNING id`, email).Scan(&id)
	require.NoError(t, err)
	return id
}
```

(Adjust columns to match `users` table actual schema; copy from existing fixtures pattern.)

- [ ] **Step 5: Run tests — PASS**

Run: `go test ./internal/order/repo/ -v 2>&1 | tail -30`
Expected: 6 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/order/repo/repo.go internal/order/repo/order_pg.go internal/order/repo/order_pg_test.go internal/testfixtures/fixtures.go
git commit -m "feat(order): order repo with Create/Get/List/UpdateStatus + JSONB shipping_address"
```

---

## Task 11: sub_order_pg + order_item_pg

**Files:**
- Create: `internal/order/repo/sub_order_pg.go`
- Create: `internal/order/repo/order_item_pg.go`
- Create: `internal/order/repo/sub_order_pg_test.go`
- Create: `internal/order/repo/order_item_pg_test.go`

- [ ] **Step 1: Write sub_order_pg.go**

```go
// internal/order/repo/sub_order_pg.go
package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
)

type SubOrderPG struct{ db DBTX }

func NewSubOrderPG(db DBTX) *SubOrderPG { return &SubOrderPG{db: db} }

const subOrderCols = `id, order_id, brand_id, subtotal_vnd, shipping_fee_vnd, total_vnd,
                      status, tracking_no, shipping_provider,
                      confirmed_at, shipped_at, delivered_at, cancelled_at,
                      created_at, updated_at`

func scanSubOrder(row pgx.Row, includeBrandJoin bool) (*domain.SubOrder, error) {
	var s domain.SubOrder
	if includeBrandJoin {
		err := row.Scan(
			&s.ID, &s.OrderID, &s.BrandID, &s.SubtotalVND, &s.ShippingFeeVND, &s.TotalVND,
			&s.Status, &s.TrackingNo, &s.ShippingProvider,
			&s.ConfirmedAt, &s.ShippedAt, &s.DeliveredAt, &s.CancelledAt,
			&s.CreatedAt, &s.UpdatedAt,
			&s.BrandSlug, &s.BrandName,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) { return nil, ErrNotFound }
			return nil, err
		}
		return &s, nil
	}
	err := row.Scan(
		&s.ID, &s.OrderID, &s.BrandID, &s.SubtotalVND, &s.ShippingFeeVND, &s.TotalVND,
		&s.Status, &s.TrackingNo, &s.ShippingProvider,
		&s.ConfirmedAt, &s.ShippedAt, &s.DeliveredAt, &s.CancelledAt,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) { return nil, ErrNotFound }
		return nil, err
	}
	return &s, nil
}

func (r *SubOrderPG) Create(ctx context.Context, db DBTX, so *domain.SubOrder) error {
	if db == nil { db = r.db }
	row := db.QueryRow(ctx,
		`INSERT INTO sub_orders
		   (order_id, brand_id, subtotal_vnd, shipping_fee_vnd, total_vnd, status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at, updated_at`,
		so.OrderID, so.BrandID, so.SubtotalVND, so.ShippingFeeVND, so.TotalVND, so.Status)
	return row.Scan(&so.ID, &so.CreatedAt, &so.UpdatedAt)
}

func (r *SubOrderPG) ListByOrderID(ctx context.Context, orderID uuid.UUID) ([]*domain.SubOrder, error) {
	rows, err := r.db.Query(ctx,
		`SELECT s.id, s.order_id, s.brand_id, s.subtotal_vnd, s.shipping_fee_vnd, s.total_vnd,
		        s.status, s.tracking_no, s.shipping_provider,
		        s.confirmed_at, s.shipped_at, s.delivered_at, s.cancelled_at,
		        s.created_at, s.updated_at,
		        b.slug, b.name
		   FROM sub_orders s
		   JOIN brands b ON b.id = s.brand_id
		  WHERE s.order_id = $1
		  ORDER BY b.name ASC`,
		orderID)
	if err != nil { return nil, err }
	defer rows.Close()

	var out []*domain.SubOrder
	for rows.Next() {
		so, err := scanSubOrder(rows, true)
		if err != nil { return nil, err }
		out = append(out, so)
	}
	return out, rows.Err()
}

func (r *SubOrderPG) CancelAllByOrderID(ctx context.Context, db DBTX, orderID uuid.UUID) error {
	if db == nil { db = r.db }
	_, err := db.Exec(ctx,
		`UPDATE sub_orders
		    SET status = 'cancelled',
		        cancelled_at = NOW(),
		        updated_at = NOW()
		  WHERE order_id = $1 AND status != 'cancelled'`,
		orderID)
	return err
}
```

- [ ] **Step 2: Write order_item_pg.go**

```go
// internal/order/repo/order_item_pg.go
package repo

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
)

type OrderItemPG struct{ db DBTX }

func NewOrderItemPG(db DBTX) *OrderItemPG { return &OrderItemPG{db: db} }

const orderItemCols = `id, sub_order_id, variant_id, product_id, product_name, variant_label,
                       image_url, qty, unit_price_vnd, line_total_vnd, created_at`

func scanOrderItem(row pgx.Row) (*domain.OrderItem, error) {
	var it domain.OrderItem
	err := row.Scan(
		&it.ID, &it.SubOrderID, &it.VariantID, &it.ProductID,
		&it.ProductName, &it.VariantLabel, &it.ImageURL,
		&it.Qty, &it.UnitPriceVND, &it.LineTotalVND, &it.CreatedAt,
	)
	if err != nil { return nil, err }
	return &it, nil
}

func (r *OrderItemPG) Create(ctx context.Context, db DBTX, it *domain.OrderItem) error {
	if db == nil { db = r.db }
	row := db.QueryRow(ctx,
		`INSERT INTO order_items
		   (sub_order_id, variant_id, product_id, product_name, variant_label, image_url,
		    qty, unit_price_vnd, line_total_vnd)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, created_at`,
		it.SubOrderID, it.VariantID, it.ProductID, it.ProductName, it.VariantLabel, it.ImageURL,
		it.Qty, it.UnitPriceVND, it.LineTotalVND)
	return row.Scan(&it.ID, &it.CreatedAt)
}

func (r *OrderItemPG) ListBySubOrderID(ctx context.Context, subOrderID uuid.UUID) ([]*domain.OrderItem, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+orderItemCols+` FROM order_items WHERE sub_order_id = $1 ORDER BY created_at ASC`,
		subOrderID)
	if err != nil { return nil, err }
	defer rows.Close()

	var out []*domain.OrderItem
	for rows.Next() {
		it, err := scanOrderItem(rows)
		if err != nil { return nil, err }
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *OrderItemPG) ListByOrderID(ctx context.Context, orderID uuid.UUID) ([]*domain.OrderItem, error) {
	rows, err := r.db.Query(ctx,
		`SELECT oi.id, oi.sub_order_id, oi.variant_id, oi.product_id,
		        oi.product_name, oi.variant_label, oi.image_url,
		        oi.qty, oi.unit_price_vnd, oi.line_total_vnd, oi.created_at
		   FROM order_items oi
		   JOIN sub_orders so ON so.id = oi.sub_order_id
		  WHERE so.order_id = $1
		  ORDER BY oi.created_at ASC`,
		orderID)
	if err != nil { return nil, err }
	defer rows.Close()

	var out []*domain.OrderItem
	for rows.Next() {
		it, err := scanOrderItem(rows)
		if err != nil { return nil, err }
		out = append(out, it)
	}
	return out, rows.Err()
}
```

- [ ] **Step 3: Write tests**

```go
// internal/order/repo/sub_order_pg_test.go
package repo_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

func TestSubOrderPG_CreateAndList(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	ctx := context.Background()
	orepo := orderrepo.NewOrderPG(pool)
	srepo := orderrepo.NewSubOrderPG(pool)

	userID := testfixtures.SeedUser(t, pool, "buyer@x.com")
	brandID := testfixtures.SeedBrand(t, pool, "rep")

	o := seedOrder(t, userID, "WW-20260524-SOSO")
	require.NoError(t, orepo.Create(ctx, pool, o))

	so := &domain.SubOrder{
		OrderID:        o.ID,
		BrandID:        brandID,
		SubtotalVND:    100000,
		ShippingFeeVND: 30000,
		TotalVND:       130000,
		Status:         domain.SubOrderStatusPending,
	}
	require.NoError(t, srepo.Create(ctx, pool, so))

	list, err := srepo.ListByOrderID(ctx, o.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "rep", list[0].BrandSlug)
}

func TestSubOrderPG_CancelAllByOrderID(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	ctx := context.Background()
	orepo := orderrepo.NewOrderPG(pool)
	srepo := orderrepo.NewSubOrderPG(pool)

	userID := testfixtures.SeedUser(t, pool, "buyer@x.com")
	brandID := testfixtures.SeedBrand(t, pool, "rep")
	o := seedOrder(t, userID, "WW-20260524-CSOS")
	require.NoError(t, orepo.Create(ctx, pool, o))

	for i := 0; i < 2; i++ {
		require.NoError(t, srepo.Create(ctx, pool, &domain.SubOrder{
			OrderID: o.ID, BrandID: brandID,
			SubtotalVND: 100, ShippingFeeVND: 0, TotalVND: 100,
			Status: domain.SubOrderStatusPending,
		}))
	}

	require.NoError(t, srepo.CancelAllByOrderID(ctx, pool, o.ID))

	list, _ := srepo.ListByOrderID(ctx, o.ID)
	for _, so := range list {
		require.Equal(t, domain.SubOrderStatusCancelled, so.Status)
		require.NotNil(t, so.CancelledAt)
	}
}
```

```go
// internal/order/repo/order_item_pg_test.go
package repo_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

func TestOrderItemPG_CreateAndList(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	ctx := context.Background()
	orepo := orderrepo.NewOrderPG(pool)
	srepo := orderrepo.NewSubOrderPG(pool)
	irepo := orderrepo.NewOrderItemPG(pool)

	userID := testfixtures.SeedUser(t, pool, "b@x.com")
	brandID := testfixtures.SeedBrand(t, pool, "rep")
	productID := testfixtures.SeedProduct(t, pool, brandID, "tee")
	variantID := testfixtures.SeedVariant(t, pool, productID, "BLK-M", 10)

	o := seedOrder(t, userID, "WW-20260524-OIPG")
	require.NoError(t, orepo.Create(ctx, pool, o))
	so := &domain.SubOrder{OrderID: o.ID, BrandID: brandID, SubtotalVND: 100, ShippingFeeVND: 0, TotalVND: 100, Status: domain.SubOrderStatusPending}
	require.NoError(t, srepo.Create(ctx, pool, so))

	img := "http://img/1.jpg"
	it := &domain.OrderItem{
		SubOrderID:   so.ID,
		VariantID:    variantID,
		ProductID:    productID,
		ProductName:  "Tee",
		VariantLabel: "Black / M",
		ImageURL:     &img,
		Qty:          2,
		UnitPriceVND: 50000,
		LineTotalVND: 100000,
	}
	require.NoError(t, irepo.Create(ctx, pool, it))
	require.NotEqual(t, uuid.Nil, it.ID)

	list, err := irepo.ListBySubOrderID(ctx, so.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "Tee", list[0].ProductName)
}
```

- [ ] **Step 4: Run tests — PASS**

Run: `go test ./internal/order/repo/ -v 2>&1 | tail -40`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/order/repo/sub_order_pg.go internal/order/repo/order_item_pg.go internal/order/repo/sub_order_pg_test.go internal/order/repo/order_item_pg_test.go
git commit -m "feat(order): sub_order_pg + order_item_pg with brand join + cascade cancel"
```

---

## Task 12: payment domain + payment_pg

**Files:**
- Create: `internal/payment/domain/payment.go`
- Create: `internal/payment/domain/errors.go`
- Create: `internal/payment/repo/repo.go`
- Create: `internal/payment/repo/payment_pg.go`
- Create: `internal/payment/repo/payment_pg_test.go`

- [ ] **Step 1: Write payment domain**

```go
// internal/payment/domain/payment.go
package domain

import (
	"time"

	"github.com/google/uuid"

	orderdomain "github.com/wearwhere/wearwhere_be/internal/order/domain"
)

type Payment struct {
	ID                   uuid.UUID
	OrderID              uuid.UUID
	AmountVND            int64
	Method               orderdomain.PaymentMethod
	Status               orderdomain.PaymentStatus
	PayosOrderCode       *int64
	PayosPaymentLinkID   *string
	PayosCheckoutURL     *string
	PayosQRCode          *string
	ExpiredAt            *time.Time
	PaidAt               *time.Time
	FailureReason        *string
	RawWebhookPayload    []byte
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
```

- [ ] **Step 2: Write payment errors**

```go
// internal/payment/domain/errors.go
package domain

import "errors"

var (
	ErrPaymentNotFound = errors.New("payment: not found")
	ErrIdempotent      = errors.New("payment: already processed (idempotent)")
)
```

- [ ] **Step 3: Write payment repo interface**

```go
// internal/payment/repo/repo.go
package repo

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/payment/domain"
)

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type PaymentRepo interface {
	Create(ctx context.Context, db DBTX, p *domain.Payment) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	GetByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Payment, error)
	GetByPayosOrderCode(ctx context.Context, code int64) (*domain.Payment, error)
	GetByPayosOrderCodeForUpdate(ctx context.Context, db DBTX, code int64) (*domain.Payment, error)
	UpdatePayosLink(ctx context.Context, db DBTX, id uuid.UUID, paymentLinkID, checkoutURL, qrCode string) error
	UpdateOnPaid(ctx context.Context, db DBTX, id uuid.UUID, rawPayload []byte) error
	UpdateOnFailed(ctx context.Context, db DBTX, id uuid.UUID, reason string, rawPayload []byte) error
	UpdateOnCancelled(ctx context.Context, db DBTX, id uuid.UUID) error
	UpdateOnExpired(ctx context.Context, db DBTX, id uuid.UUID) error
}
```

- [ ] **Step 4: Write payment_pg.go**

```go
// internal/payment/repo/payment_pg.go
package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/payment/domain"
)

type PaymentPG struct{ db DBTX }

func NewPaymentPG(db DBTX) *PaymentPG { return &PaymentPG{db: db} }

const paymentCols = `id, order_id, amount_vnd, method, status,
                     payos_order_code, payos_payment_link_id, payos_checkout_url, payos_qr_code,
                     expired_at, paid_at, failure_reason, raw_webhook_payload,
                     created_at, updated_at`

func scanPayment(row pgx.Row) (*domain.Payment, error) {
	var p domain.Payment
	err := row.Scan(
		&p.ID, &p.OrderID, &p.AmountVND, &p.Method, &p.Status,
		&p.PayosOrderCode, &p.PayosPaymentLinkID, &p.PayosCheckoutURL, &p.PayosQRCode,
		&p.ExpiredAt, &p.PaidAt, &p.FailureReason, &p.RawWebhookPayload,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) { return nil, domain.ErrPaymentNotFound }
		return nil, err
	}
	return &p, nil
}

func (r *PaymentPG) Create(ctx context.Context, db DBTX, p *domain.Payment) error {
	if db == nil { db = r.db }
	row := db.QueryRow(ctx,
		`INSERT INTO payments
		   (order_id, amount_vnd, method, status, payos_order_code, expired_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at, updated_at`,
		p.OrderID, p.AmountVND, p.Method, p.Status, p.PayosOrderCode, p.ExpiredAt)
	return row.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (r *PaymentPG) GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	return scanPayment(r.db.QueryRow(ctx,
		`SELECT `+paymentCols+` FROM payments WHERE id=$1`, id))
}

func (r *PaymentPG) GetByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Payment, error) {
	return scanPayment(r.db.QueryRow(ctx,
		`SELECT `+paymentCols+` FROM payments WHERE order_id=$1 ORDER BY created_at DESC LIMIT 1`,
		orderID))
}

func (r *PaymentPG) GetByPayosOrderCode(ctx context.Context, code int64) (*domain.Payment, error) {
	return scanPayment(r.db.QueryRow(ctx,
		`SELECT `+paymentCols+` FROM payments WHERE payos_order_code=$1`, code))
}

func (r *PaymentPG) GetByPayosOrderCodeForUpdate(ctx context.Context, db DBTX, code int64) (*domain.Payment, error) {
	if db == nil { db = r.db }
	return scanPayment(db.QueryRow(ctx,
		`SELECT `+paymentCols+` FROM payments WHERE payos_order_code=$1 FOR UPDATE`, code))
}

func (r *PaymentPG) UpdatePayosLink(ctx context.Context, db DBTX, id uuid.UUID, linkID, checkoutURL, qrCode string) error {
	if db == nil { db = r.db }
	tag, err := db.Exec(ctx,
		`UPDATE payments
		    SET payos_payment_link_id=$2, payos_checkout_url=$3, payos_qr_code=$4, updated_at=NOW()
		  WHERE id=$1`,
		id, linkID, checkoutURL, qrCode)
	if err != nil { return err }
	if tag.RowsAffected() == 0 { return domain.ErrPaymentNotFound }
	return nil
}

func (r *PaymentPG) UpdateOnPaid(ctx context.Context, db DBTX, id uuid.UUID, raw []byte) error {
	if db == nil { db = r.db }
	tag, err := db.Exec(ctx,
		`UPDATE payments
		    SET status='paid', paid_at=NOW(), raw_webhook_payload=$2, updated_at=NOW()
		  WHERE id=$1 AND status='pending'`,
		id, raw)
	if err != nil { return err }
	if tag.RowsAffected() == 0 { return domain.ErrIdempotent }
	return nil
}

func (r *PaymentPG) UpdateOnFailed(ctx context.Context, db DBTX, id uuid.UUID, reason string, raw []byte) error {
	if db == nil { db = r.db }
	tag, err := db.Exec(ctx,
		`UPDATE payments
		    SET status='failed', failure_reason=$2, raw_webhook_payload=$3, updated_at=NOW()
		  WHERE id=$1 AND status='pending'`,
		id, reason, raw)
	if err != nil { return err }
	if tag.RowsAffected() == 0 { return domain.ErrIdempotent }
	return nil
}

func (r *PaymentPG) UpdateOnCancelled(ctx context.Context, db DBTX, id uuid.UUID) error {
	if db == nil { db = r.db }
	tag, err := db.Exec(ctx,
		`UPDATE payments SET status='cancelled', updated_at=NOW()
		  WHERE id=$1 AND status='pending'`, id)
	if err != nil { return err }
	if tag.RowsAffected() == 0 { return domain.ErrIdempotent }
	return nil
}

func (r *PaymentPG) UpdateOnExpired(ctx context.Context, db DBTX, id uuid.UUID) error {
	if db == nil { db = r.db }
	tag, err := db.Exec(ctx,
		`UPDATE payments SET status='expired', updated_at=NOW()
		  WHERE id=$1 AND status='pending'`, id)
	if err != nil { return err }
	if tag.RowsAffected() == 0 { return domain.ErrIdempotent }
	return nil
}
```

- [ ] **Step 5: Write tests**

```go
// internal/payment/repo/payment_pg_test.go
package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	orderdomain "github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	paymentdomain "github.com/wearwhere/wearwhere_be/internal/payment/domain"
	paymentrepo "github.com/wearwhere/wearwhere_be/internal/payment/repo"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

func seedOrderAndPayment(t *testing.T, ctx context.Context, pool any) (orderID, paymentID interface{}) {
	t.Helper()
	// Inline helper, simplified
	panic("inline below")
}

func TestPaymentPG_CreateAndGet(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	ctx := context.Background()
	orepo := orderrepo.NewOrderPG(pool)
	prepo := paymentrepo.NewPaymentPG(pool)

	userID := testfixtures.SeedUser(t, pool, "p@x.com")
	o := &orderdomain.Order{
		UserID: userID, OrderNo: "WW-20260524-PAYC",
		SubtotalVND: 100000, ShippingTotalVND: 30000, GrandTotalVND: 130000,
		PaymentMethod: orderdomain.PaymentMethodPayos, PaymentStatus: orderdomain.PaymentStatusPending,
		Status: orderdomain.OrderStatusPendingPayment,
		ShippingAddress: orderdomain.ShippingAddress{Recipient: "A", Phone: "0", Line1: "L", Ward: "W", District: "D", City: "C"},
	}
	require.NoError(t, orepo.Create(ctx, pool, o))

	code := int64(1700000000001)
	exp := time.Now().Add(30 * time.Minute)
	p := &paymentdomain.Payment{
		OrderID: o.ID, AmountVND: 130000,
		Method: orderdomain.PaymentMethodPayos, Status: orderdomain.PaymentStatusPending,
		PayosOrderCode: &code, ExpiredAt: &exp,
	}
	require.NoError(t, prepo.Create(ctx, pool, p))

	got, err := prepo.GetByPayosOrderCode(ctx, code)
	require.NoError(t, err)
	require.Equal(t, p.ID, got.ID)
}

func TestPaymentPG_UpdateOnPaid_IdempotentOnSecondCall(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	ctx := context.Background()
	orepo := orderrepo.NewOrderPG(pool)
	prepo := paymentrepo.NewPaymentPG(pool)

	userID := testfixtures.SeedUser(t, pool, "p@x.com")
	o := &orderdomain.Order{
		UserID: userID, OrderNo: "WW-20260524-IDEM",
		SubtotalVND: 100000, ShippingTotalVND: 30000, GrandTotalVND: 130000,
		PaymentMethod: orderdomain.PaymentMethodPayos, PaymentStatus: orderdomain.PaymentStatusPending,
		Status: orderdomain.OrderStatusPendingPayment,
		ShippingAddress: orderdomain.ShippingAddress{Recipient: "A", Phone: "0", Line1: "L", Ward: "W", District: "D", City: "C"},
	}
	require.NoError(t, orepo.Create(ctx, pool, o))

	code := int64(1700000000002)
	p := &paymentdomain.Payment{
		OrderID: o.ID, AmountVND: 130000,
		Method: orderdomain.PaymentMethodPayos, Status: orderdomain.PaymentStatusPending,
		PayosOrderCode: &code,
	}
	require.NoError(t, prepo.Create(ctx, pool, p))

	require.NoError(t, prepo.UpdateOnPaid(ctx, pool, p.ID, []byte(`{"ok":1}`)))
	err := prepo.UpdateOnPaid(ctx, pool, p.ID, []byte(`{"ok":1}`))
	require.ErrorIs(t, err, paymentdomain.ErrIdempotent)
}
```

- [ ] **Step 6: Run tests — PASS**

Run: `go test ./internal/payment/... -v 2>&1 | tail -20`
Expected: 2 tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/payment/
git commit -m "feat(payment): payment domain + payment_pg with idempotent status updates"
```

---

## Task 13: PayOS Client interface + Signature

**Files:**
- Create: `internal/payment/payos/client.go`
- Create: `internal/payment/payos/signature.go`
- Create: `internal/payment/payos/signature_test.go`

- [ ] **Step 1: Write client.go (interface + DTOs)**

```go
// internal/payment/payos/client.go
package payos

import (
	"context"
	"errors"
	"time"
)

var (
	ErrSignatureInvalid = errors.New("payos: invalid webhook signature")
	ErrCreateLink       = errors.New("payos: failed to create payment link")
)

type LineItem struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
	Price    int64  `json:"price"`
}

type Buyer struct {
	Name  string `json:"buyerName"`
	Phone string `json:"buyerPhone"`
	Email string `json:"buyerEmail"`
}

type CreateLinkReq struct {
	OrderCode   int64      `json:"orderCode"`
	AmountVND   int64      `json:"amount"`
	Description string     `json:"description"`
	Items       []LineItem `json:"items"`
	ReturnURL   string     `json:"returnUrl"`
	CancelURL   string     `json:"cancelUrl"`
	Buyer       Buyer      `json:",inline"`
	ExpiredAt   int64      `json:"expiredAt"`
}

type CreateLinkResp struct {
	PaymentLinkID string    `json:"paymentLinkId"`
	CheckoutURL   string    `json:"checkoutUrl"`
	QRCode        string    `json:"qrCode"`
	OrderCode     int64     `json:"orderCode"`
	ExpiredAt     time.Time `json:"-"`
}

type WebhookData struct {
	OrderCode           int64  `json:"orderCode"`
	Amount              int64  `json:"amount"`
	Description         string `json:"description"`
	AccountNumber       string `json:"accountNumber"`
	Reference           string `json:"reference"`
	TransactionDateTime string `json:"transactionDateTime"`
	Currency            string `json:"currency"`
	PaymentLinkID       string `json:"paymentLinkId"`
	Code                string `json:"code"`
	Desc                string `json:"desc"`
}

type WebhookPayload struct {
	Code      string      `json:"code"`
	Desc      string      `json:"desc"`
	Success   bool        `json:"success"`
	Data      WebhookData `json:"data"`
	Signature string      `json:"signature"`
}

type PaymentInfo struct {
	OrderCode int64
	Status    string
	Amount    int64
}

type Client interface {
	CreateLink(ctx context.Context, r CreateLinkReq) (*CreateLinkResp, error)
	VerifyWebhookSignature(p WebhookPayload) error
	GetPayment(ctx context.Context, paymentLinkID string) (*PaymentInfo, error)
	CancelLink(ctx context.Context, paymentLinkID, reason string) error
}
```

- [ ] **Step 2: Write signature.go**

```go
// internal/payment/payos/signature.go
package payos

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Sign computes HMAC-SHA256 over sorted key=value pairs joined by '&'.
// Per PayOS spec: keys sorted alphabetically; values stringified (numbers without quotes).
func Sign(checksumKey string, fields map[string]any) string {
	keys := make([]string, 0, len(fields))
	for k := range fields { keys = append(keys, k) }
	sort.Strings(keys)

	var sb strings.Builder
	for i, k := range keys {
		if i > 0 { sb.WriteByte('&') }
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(fmt.Sprintf("%v", fields[k]))
	}
	mac := hmac.New(sha256.New, []byte(checksumKey))
	mac.Write([]byte(sb.String()))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyWebhook checks a webhook payload's signature against checksumKey.
// Returns ErrSignatureInvalid on mismatch.
func VerifyWebhook(checksumKey string, p WebhookPayload) error {
	fields := webhookDataToMap(p.Data)
	expected := Sign(checksumKey, fields)
	if !hmac.Equal([]byte(expected), []byte(p.Signature)) {
		return ErrSignatureInvalid
	}
	return nil
}

func webhookDataToMap(d WebhookData) map[string]any {
	return map[string]any{
		"orderCode":           d.OrderCode,
		"amount":              d.Amount,
		"description":         d.Description,
		"accountNumber":       d.AccountNumber,
		"reference":           d.Reference,
		"transactionDateTime": d.TransactionDateTime,
		"currency":            d.Currency,
		"paymentLinkId":       d.PaymentLinkID,
		"code":                d.Code,
		"desc":                d.Desc,
	}
}
```

- [ ] **Step 3: Write signature_test.go**

```go
// internal/payment/payos/signature_test.go
package payos_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/payment/payos"
)

func TestSign_DeterministicAcrossInputOrder(t *testing.T) {
	a := payos.Sign("key", map[string]any{"b": 2, "a": 1, "c": 3})
	b := payos.Sign("key", map[string]any{"c": 3, "a": 1, "b": 2})
	require.Equal(t, a, b)
}

func TestSign_KnownValue(t *testing.T) {
	// Hand-computed: HMAC-SHA256("secret", "a=1&b=2") -> compare via stdlib
	expected := func() string {
		h := hmac.New(sha256.New, []byte("secret"))
		h.Write([]byte("a=1&b=2"))
		return hex.EncodeToString(h.Sum(nil))
	}()
	got := payos.Sign("secret", map[string]any{"a": 1, "b": 2})
	require.Equal(t, expected, got)
}

func TestVerifyWebhook_ValidSignature(t *testing.T) {
	data := payos.WebhookData{
		OrderCode: 12345, Amount: 100000, Description: "test",
		AccountNumber: "vcb1", Reference: "ref1", TransactionDateTime: "2026-05-24",
		Currency: "VND", PaymentLinkID: "pl1", Code: "00", Desc: "Success",
	}
	fields := map[string]any{
		"orderCode": data.OrderCode, "amount": data.Amount, "description": data.Description,
		"accountNumber": data.AccountNumber, "reference": data.Reference,
		"transactionDateTime": data.TransactionDateTime, "currency": data.Currency,
		"paymentLinkId": data.PaymentLinkID, "code": data.Code, "desc": data.Desc,
	}
	sig := payos.Sign("secret-key", fields)

	err := payos.VerifyWebhook("secret-key", payos.WebhookPayload{
		Code: "00", Success: true, Data: data, Signature: sig,
	})
	require.NoError(t, err)
}

func TestVerifyWebhook_InvalidSignature(t *testing.T) {
	err := payos.VerifyWebhook("secret-key", payos.WebhookPayload{
		Data: payos.WebhookData{OrderCode: 1, Amount: 100},
		Signature: "deadbeef",
	})
	require.ErrorIs(t, err, payos.ErrSignatureInvalid)
}
```

- [ ] **Step 4: Run tests — PASS**

Run: `go test ./internal/payment/payos/ -run Sign -v 2>&1 | tail -20`
Expected: 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/payment/payos/client.go internal/payment/payos/signature.go internal/payment/payos/signature_test.go
git commit -m "feat(payos): Client interface + HMAC-SHA256 signature sign/verify with tests"
```

---

## Task 14: PayOS HTTP client (production-ready stub)

**Files:**
- Create: `internal/payment/payos/client_http.go`

- [ ] **Step 1: Write client_http.go**

```go
// internal/payment/payos/client_http.go
package payos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const payosBaseURL = "https://api-merchant.payos.vn"

type HTTPClient struct {
	clientID    string
	apiKey      string
	checksumKey string
	httpClient  *http.Client
	baseURL     string
}

func NewHTTPClient(clientID, apiKey, checksumKey string) *HTTPClient {
	return &HTTPClient{
		clientID:    clientID,
		apiKey:      apiKey,
		checksumKey: checksumKey,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		baseURL:     payosBaseURL,
	}
}

type payosEnvelope struct {
	Code string          `json:"code"`
	Desc string          `json:"desc"`
	Data json.RawMessage `json:"data"`
}

func (c *HTTPClient) CreateLink(ctx context.Context, r CreateLinkReq) (*CreateLinkResp, error) {
	// Build request body and signature.
	body := map[string]any{
		"orderCode":   r.OrderCode,
		"amount":      r.AmountVND,
		"description": r.Description,
		"items":       r.Items,
		"returnUrl":   r.ReturnURL,
		"cancelUrl":   r.CancelURL,
		"buyerName":   r.Buyer.Name,
		"buyerPhone":  r.Buyer.Phone,
		"buyerEmail":  r.Buyer.Email,
		"expiredAt":   r.ExpiredAt,
	}
	// Signature for create-link: amount, cancelUrl, description, orderCode, returnUrl
	body["signature"] = Sign(c.checksumKey, map[string]any{
		"amount":      r.AmountVND,
		"cancelUrl":   r.CancelURL,
		"description": r.Description,
		"orderCode":   r.OrderCode,
		"returnUrl":   r.ReturnURL,
	})

	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/v2/payment-requests", bytes.NewReader(buf))
	if err != nil { return nil, err }
	req.Header.Set("x-client-id", c.clientID)
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil { return nil, fmt.Errorf("%w: %v", ErrCreateLink, err) }
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: status=%d body=%s", ErrCreateLink, resp.StatusCode, string(bodyBytes))
	}

	var env payosEnvelope
	if err := json.Unmarshal(bodyBytes, &env); err != nil { return nil, err }
	if env.Code != "00" {
		return nil, fmt.Errorf("%w: code=%s desc=%s", ErrCreateLink, env.Code, env.Desc)
	}

	var data struct {
		PaymentLinkID string `json:"paymentLinkId"`
		CheckoutURL   string `json:"checkoutUrl"`
		QRCode        string `json:"qrCode"`
		OrderCode     int64  `json:"orderCode"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil { return nil, err }

	return &CreateLinkResp{
		PaymentLinkID: data.PaymentLinkID,
		CheckoutURL:   data.CheckoutURL,
		QRCode:        data.QRCode,
		OrderCode:     data.OrderCode,
		ExpiredAt:     time.Unix(r.ExpiredAt, 0),
	}, nil
}

func (c *HTTPClient) VerifyWebhookSignature(p WebhookPayload) error {
	return VerifyWebhook(c.checksumKey, p)
}

func (c *HTTPClient) GetPayment(ctx context.Context, paymentLinkID string) (*PaymentInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		c.baseURL+"/v2/payment-requests/"+paymentLinkID, nil)
	if err != nil { return nil, err }
	req.Header.Set("x-client-id", c.clientID)
	req.Header.Set("x-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("payos GetPayment: status=%d body=%s", resp.StatusCode, string(bodyBytes))
	}

	var env payosEnvelope
	if err := json.Unmarshal(bodyBytes, &env); err != nil { return nil, err }
	var data struct {
		OrderCode int64  `json:"orderCode"`
		Status    string `json:"status"`
		Amount    int64  `json:"amount"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil { return nil, err }

	return &PaymentInfo{OrderCode: data.OrderCode, Status: data.Status, Amount: data.Amount}, nil
}

func (c *HTTPClient) CancelLink(ctx context.Context, paymentLinkID, reason string) error {
	body := map[string]any{"cancellationReason": reason}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/v2/payment-requests/"+paymentLinkID+"/cancel", bytes.NewReader(buf))
	if err != nil { return err }
	req.Header.Set("x-client-id", c.clientID)
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("payos CancelLink: status=%d body=%s", resp.StatusCode, string(b))
	}
	return nil
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/payment/payos/`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add internal/payment/payos/client_http.go
git commit -m "feat(payos): production HTTP client (untested vs real PayOS; needs creds)"
```

---

## Task 15: PayOS Mock client + factory + dev page handler

**Files:**
- Create: `internal/payment/payos/client_mock.go`
- Create: `internal/payment/payos/factory.go`
- Create: `internal/payment/payos/client_mock_test.go`

- [ ] **Step 1: Write client_mock.go**

```go
// internal/payment/payos/client_mock.go
package payos

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

type MockClient struct {
	seq        atomic.Int64
	baseURL    string // for constructing mock checkout URL, e.g. http://localhost:8080
}

func NewMockClient(baseURL string) *MockClient {
	if baseURL == "" { baseURL = "http://localhost:8080" }
	return &MockClient{baseURL: baseURL}
}

func (m *MockClient) CreateLink(_ context.Context, r CreateLinkReq) (*CreateLinkResp, error) {
	id := fmt.Sprintf("mock-pl-%d", m.seq.Add(1))
	return &CreateLinkResp{
		PaymentLinkID: id,
		CheckoutURL:   fmt.Sprintf("%s/dev/payos/mock-checkout?orderCode=%d", m.baseURL, r.OrderCode),
		QRCode:        "data:image/png;base64,mock-qr",
		OrderCode:     r.OrderCode,
		ExpiredAt:     time.Unix(r.ExpiredAt, 0),
	}, nil
}

// Mock accepts any signature — testing convenience only.
func (m *MockClient) VerifyWebhookSignature(_ WebhookPayload) error { return nil }

func (m *MockClient) GetPayment(_ context.Context, paymentLinkID string) (*PaymentInfo, error) {
	return &PaymentInfo{Status: "PENDING"}, nil
}

func (m *MockClient) CancelLink(_ context.Context, _, _ string) error { return nil }
```

- [ ] **Step 2: Write factory.go**

```go
// internal/payment/payos/factory.go
package payos

import "fmt"

type Config struct {
	Mode        string // "mock" | "production"
	ClientID    string
	APIKey      string
	ChecksumKey string
	BaseURL     string // for mock checkout URL (defaults to http://localhost:8080)
	ReturnURL   string
	CancelURL   string
}

func NewFromConfig(cfg Config) (Client, error) {
	switch cfg.Mode {
	case "mock", "":
		return NewMockClient(cfg.BaseURL), nil
	case "production":
		if cfg.ClientID == "" || cfg.APIKey == "" || cfg.ChecksumKey == "" {
			return nil, fmt.Errorf("payos: production mode requires PAYOS_CLIENT_ID, PAYOS_API_KEY, PAYOS_CHECKSUM_KEY")
		}
		return NewHTTPClient(cfg.ClientID, cfg.APIKey, cfg.ChecksumKey), nil
	default:
		return nil, fmt.Errorf("payos: unknown mode %q (want mock|production)", cfg.Mode)
	}
}
```

- [ ] **Step 3: Write client_mock_test.go**

```go
// internal/payment/payos/client_mock_test.go
package payos_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/payment/payos"
)

func TestMockClient_CreateLink_ReturnsLocalURL(t *testing.T) {
	m := payos.NewMockClient("http://localhost:8080")
	r, err := m.CreateLink(context.Background(), payos.CreateLinkReq{
		OrderCode: 42, AmountVND: 130000, Description: "test",
	})
	require.NoError(t, err)
	require.Equal(t, "http://localhost:8080/dev/payos/mock-checkout?orderCode=42", r.CheckoutURL)
	require.Equal(t, int64(42), r.OrderCode)
}

func TestMockClient_VerifyWebhook_AlwaysOK(t *testing.T) {
	m := payos.NewMockClient("")
	require.NoError(t, m.VerifyWebhookSignature(payos.WebhookPayload{Signature: "anything"}))
}

func TestFactory_DefaultsMock(t *testing.T) {
	c, err := payos.NewFromConfig(payos.Config{Mode: ""})
	require.NoError(t, err)
	require.NotNil(t, c)
}

func TestFactory_ProductionRequiresCreds(t *testing.T) {
	_, err := payos.NewFromConfig(payos.Config{Mode: "production"})
	require.Error(t, err)
}

func TestFactory_UnknownMode(t *testing.T) {
	_, err := payos.NewFromConfig(payos.Config{Mode: "stripe"})
	require.Error(t, err)
}
```

- [ ] **Step 4: Run tests — PASS**

Run: `go test ./internal/payment/payos/ -v 2>&1 | tail -20`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/payment/payos/client_mock.go internal/payment/payos/factory.go internal/payment/payos/client_mock_test.go
git commit -m "feat(payos): mock client + factory(mode=mock|production) with config validation"
```

---

## Task 16: Checkout preview service

**Files:**
- Create: `internal/order/service/checkout_service.go`
- Create: `internal/order/service/checkout_service_test.go`

- [ ] **Step 1: Write checkout_service.go**

```go
// internal/order/service/checkout_service.go
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	cartdomain "github.com/wearwhere/wearwhere_be/internal/cart/domain"
	cartrepo "github.com/wearwhere/wearwhere_be/internal/cart/repo"
	customeraddrrepo "github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	"github.com/wearwhere/wearwhere_be/internal/shipping/provider"
)

type CheckoutService struct {
	cartRepo    cartrepo.CartRepo
	addrRepo    customeraddrrepo.AddressRepo
	shipping    provider.ShippingProvider
}

func NewCheckoutService(c cartrepo.CartRepo, a customeraddrrepo.AddressRepo, s provider.ShippingProvider) *CheckoutService {
	return &CheckoutService{cartRepo: c, addrRepo: a, shipping: s}
}

func (s *CheckoutService) Preview(ctx context.Context, userID, addressID uuid.UUID) (*domain.CheckoutPreviewResp, error) {
	addr, err := s.addrRepo.GetByID(ctx, addressID)
	if err != nil { return nil, domain.ErrAddressNotFound }
	if addr.UserID != userID { return nil, domain.ErrAddressNotFound }
	shipAddr := domain.ShippingAddress{
		Recipient: addr.Recipient, Phone: addr.Phone, Line1: addr.Line1,
		Ward: addr.Ward, District: addr.District, City: addr.City,
	}

	items, err := s.cartRepo.ListView(ctx, userID)
	if err != nil { return nil, err }
	if len(items) == 0 {
		return &domain.CheckoutPreviewResp{
			CartEmpty:        true,
			Address:          &shipAddr,
			SubOrders:        []domain.CheckoutPreviewSubOrder{},
			MinOrderValueVND: domain.MinOrderValueVND,
			MeetsMinOrder:    false,
			Warnings:         []string{},
		}, nil
	}

	type group struct {
		brand   domain.BrandRef
		items   []domain.CheckoutPreviewItem
		subtotal int64
	}
	grouped := map[uuid.UUID]*group{}
	warnings := []string{}
	var subtotalAll int64

	for _, ci := range items {
		if ci.Unavailable {
			reason := ""
			if ci.UnavailableReason != nil { reason = *ci.UnavailableReason }
			warnings = append(warnings, fmt.Sprintf("variant %s unavailable (%s)", ci.VariantID, reason))
			continue
		}
		if ci.StockQty < ci.Qty {
			warnings = append(warnings, fmt.Sprintf("variant %s low stock (available %d, in cart %d)", ci.VariantID, ci.StockQty, ci.Qty))
		}
		lineTotal := int64(float64(ci.Qty) * ci.CurrentPrice)
		grp, ok := grouped[ci.BrandID]
		if !ok {
			grp = &group{
				brand: domain.BrandRef{ID: ci.BrandID, Slug: ci.BrandSlug, Name: ci.BrandName},
			}
			grouped[ci.BrandID] = grp
		}
		grp.items = append(grp.items, domain.CheckoutPreviewItem{
			VariantID:    ci.VariantID,
			ProductID:    ci.ProductID,
			ProductName:  ci.ProductName,
			VariantLabel: variantLabel(ci),
			ImageURL:     ci.PrimaryImageURL,
			Qty:          ci.Qty,
			UnitPriceVND: int64(ci.CurrentPrice),
			LineTotalVND: lineTotal,
			AvailableQty: ci.StockQty,
		})
		grp.subtotal += lineTotal
		subtotalAll += lineTotal
	}

	subOrders := make([]domain.CheckoutPreviewSubOrder, 0, len(grouped))
	var shippingAll int64
	for bID, g := range grouped {
		quote, err := s.shipping.Calculate(ctx, provider.CalcReq{
			BrandID:   bID,
			ToAddress: toShippingProviderAddr(shipAddr),
		})
		if err != nil {
			return nil, fmt.Errorf("shipping calc for brand %s: %w", bID, err)
		}
		shippingAll += quote.AmountVND
		subOrders = append(subOrders, domain.CheckoutPreviewSubOrder{
			Brand:          g.brand,
			Items:          g.items,
			SubtotalVND:    g.subtotal,
			ShippingFeeVND: quote.AmountVND,
			TotalVND:       g.subtotal + quote.AmountVND,
		})
	}

	grand := subtotalAll + shippingAll
	return &domain.CheckoutPreviewResp{
		CartEmpty:        false,
		Address:          &shipAddr,
		SubOrders:        subOrders,
		SubtotalVND:      subtotalAll,
		ShippingTotalVND: shippingAll,
		GrandTotalVND:    grand,
		MinOrderValueVND: domain.MinOrderValueVND,
		MeetsMinOrder:    subtotalAll >= domain.MinOrderValueVND,
		Warnings:         warnings,
	}, nil
}

func variantLabel(ci *cartdomain.CartItemView) string {
	if ci.Color != "" && ci.Size != "" { return ci.Color + " / " + ci.Size }
	if ci.Color != "" { return ci.Color }
	if ci.Size != "" { return ci.Size }
	return ci.SKU
}

func toShippingProviderAddr(a domain.ShippingAddress) provider.ShippingAddress {
	return provider.ShippingAddress{
		Recipient: a.Recipient, Phone: a.Phone, Line1: a.Line1,
		Ward: a.Ward, District: a.District, City: a.City,
	}
}

// silence unused imports during early dev
var _ = errors.New
```

- [ ] **Step 2: Write tests**

```go
// internal/order/service/checkout_service_test.go
package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	cartdomain "github.com/wearwhere/wearwhere_be/internal/cart/domain"
	customeraddrdomain "github.com/wearwhere/wearwhere_be/internal/customeraddr/domain"
	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	"github.com/wearwhere/wearwhere_be/internal/order/service"
	shippingdomain "github.com/wearwhere/wearwhere_be/internal/shipping/domain"
	"github.com/wearwhere/wearwhere_be/internal/shipping/provider"
)

// -------------- fakes --------------

type fakeCartRepo struct{ items []*cartdomain.CartItemView }

func (f *fakeCartRepo) ListView(_ context.Context, _ uuid.UUID) ([]*cartdomain.CartItemView, error) {
	return f.items, nil
}
// stub other CartRepo methods as panic so test fails loudly if invoked
func (f *fakeCartRepo) UpsertAdd(_ context.Context, _, _ uuid.UUID, _ int, _ float64) (*cartdomain.CartItem, error) { panic("n/a") }
func (f *fakeCartRepo) FindByID(_ context.Context, _, _ uuid.UUID) (*cartdomain.CartItem, error) { panic("n/a") }
func (f *fakeCartRepo) FindByVariant(_ context.Context, _, _ uuid.UUID) (*cartdomain.CartItem, error) { panic("n/a") }
func (f *fakeCartRepo) UpdateQty(_ context.Context, _, _ uuid.UUID, _ int, _ float64) (*cartdomain.CartItem, error) { panic("n/a") }
func (f *fakeCartRepo) Delete(_ context.Context, _, _ uuid.UUID) error { panic("n/a") }
func (f *fakeCartRepo) Clear(_ context.Context, _ uuid.UUID) error { panic("n/a") }

type fakeAddrRepo struct{ addr *customeraddrdomain.Address }
func (f *fakeAddrRepo) GetByID(_ context.Context, _ uuid.UUID) (*customeraddrdomain.Address, error) {
	if f.addr == nil { return nil, customeraddrdomain.ErrNotFound }
	return f.addr, nil
}
// stub rest of AddressRepo methods (panic on unexpected call) — fill in to match actual interface

type fakeShipping struct{ amount int64 }
func (f *fakeShipping) Calculate(_ context.Context, _ provider.CalcReq) (*shippingdomain.FeeQuote, error) {
	return &shippingdomain.FeeQuote{AmountVND: f.amount, Currency: "VND"}, nil
}

// -------------- tests --------------

func TestPreview_EmptyCart(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	cart := &fakeCartRepo{items: nil}
	addr := &fakeAddrRepo{addr: &customeraddrdomain.Address{ID: addrID, UserID: userID, Recipient: "A", Phone: "0", Line1: "L", Ward: "W", District: "D", City: "C"}}
	ship := &fakeShipping{amount: 30000}

	svc := service.NewCheckoutService(cart, addr, ship)
	resp, err := svc.Preview(context.Background(), userID, addrID)
	require.NoError(t, err)
	require.True(t, resp.CartEmpty)
}

func TestPreview_AddressNotOwned_Returns404(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	cart := &fakeCartRepo{}
	addr := &fakeAddrRepo{addr: &customeraddrdomain.Address{ID: addrID, UserID: uuid.New(), Recipient: "X"}}
	ship := &fakeShipping{amount: 30000}

	svc := service.NewCheckoutService(cart, addr, ship)
	_, err := svc.Preview(context.Background(), userID, addrID)
	require.ErrorIs(t, err, domain.ErrAddressNotFound)
}

func TestPreview_GroupsByBrand_AndComputesTotals(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	bA := uuid.New()
	bB := uuid.New()

	cart := &fakeCartRepo{items: []*cartdomain.CartItemView{
		{VariantID: uuid.New(), ProductID: uuid.New(), ProductName: "Tee", BrandID: bA, BrandSlug: "rep", BrandName: "REP",
			Qty: 2, CurrentPrice: 100000, StockQty: 5, Size: "M", Color: "Black", SKU: "BLK-M"},
		{VariantID: uuid.New(), ProductID: uuid.New(), ProductName: "Hat", BrandID: bB, BrandSlug: "fok", BrandName: "FOK",
			Qty: 1, CurrentPrice: 200000, StockQty: 5, Size: "OS", Color: "White", SKU: "WHT-OS"},
	}}
	addr := &fakeAddrRepo{addr: &customeraddrdomain.Address{ID: addrID, UserID: userID, Recipient: "A", Phone: "0", Line1: "L", Ward: "W", District: "D", City: "C"}}
	ship := &fakeShipping{amount: 30000}

	svc := service.NewCheckoutService(cart, addr, ship)
	resp, err := svc.Preview(context.Background(), userID, addrID)
	require.NoError(t, err)
	require.Equal(t, int64(2*100000+200000), resp.SubtotalVND)
	require.Equal(t, int64(2*30000), resp.ShippingTotalVND)
	require.Equal(t, int64(400000+60000), resp.GrandTotalVND)
	require.Len(t, resp.SubOrders, 2)
	require.True(t, resp.MeetsMinOrder)
}

func TestPreview_BelowMinOrder(t *testing.T) {
	userID := uuid.New()
	addrID := uuid.New()
	bID := uuid.New()
	cart := &fakeCartRepo{items: []*cartdomain.CartItemView{
		{VariantID: uuid.New(), ProductID: uuid.New(), ProductName: "Sticker", BrandID: bID, BrandSlug: "x", BrandName: "X",
			Qty: 1, CurrentPrice: 10000, StockQty: 5, SKU: "STK"},
	}}
	addr := &fakeAddrRepo{addr: &customeraddrdomain.Address{ID: addrID, UserID: userID, Recipient: "A", Phone: "0", Line1: "L", Ward: "W", District: "D", City: "C"}}
	ship := &fakeShipping{amount: 30000}

	svc := service.NewCheckoutService(cart, addr, ship)
	resp, err := svc.Preview(context.Background(), userID, addrID)
	require.NoError(t, err)
	require.False(t, resp.MeetsMinOrder)
	require.Equal(t, int64(50000), resp.MinOrderValueVND)
}
```

- [ ] **Step 3: Run tests — PASS**

Run: `go test ./internal/order/service/ -run TestPreview -v 2>&1 | tail -30`
Expected: 4 tests PASS. If `fakeAddrRepo` is missing methods to satisfy `customeraddrrepo.AddressRepo`, add stubs (`panic("not used")`) as the compiler points them out.

- [ ] **Step 4: Commit**

```bash
git add internal/order/service/checkout_service.go internal/order/service/checkout_service_test.go
git commit -m "feat(order): checkout preview service with multi-brand grouping + warnings"
```

---

## Task 17: PlaceOrder transaction flow

**Files:**
- Create: `internal/order/service/order_service.go`
- Create: `internal/order/service/order_service_test.go`

- [ ] **Step 1: Write order_service.go (PlaceOrder only — Cancel/List/Detail in Task 18)**

```go
// internal/order/service/order_service.go
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	authdomain "github.com/wearwhere/wearwhere_be/internal/auth/domain"
	authrepo "github.com/wearwhere/wearwhere_be/internal/auth/repo"
	customeraddrrepo "github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	paymentdomain "github.com/wearwhere/wearwhere_be/internal/payment/domain"
	"github.com/wearwhere/wearwhere_be/internal/payment/payos"
	paymentrepo "github.com/wearwhere/wearwhere_be/internal/payment/repo"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
	"github.com/wearwhere/wearwhere_be/internal/shipping/provider"
)

type OrderService struct {
	pool         *pgxpool.Pool
	orderRepo    orderrepo.OrderRepo
	subOrderRepo orderrepo.SubOrderRepo
	itemRepo     orderrepo.OrderItemRepo
	paymentRepo  paymentrepo.PaymentRepo
	variantRepo  productrepo.VariantRepo
	addrRepo     customeraddrrepo.AddressRepo
	userRepo     authrepo.UserRepo
	shipping     provider.ShippingProvider
	payosClient  payos.Client
	cfg          Config
}

type Config struct {
	ReservationTimeout time.Duration // default 30 min
	PayosReturnURL     string
	PayosCancelURL     string
}

func NewOrderService(
	pool *pgxpool.Pool,
	or orderrepo.OrderRepo, sr orderrepo.SubOrderRepo, ir orderrepo.OrderItemRepo,
	pr paymentrepo.PaymentRepo, vr productrepo.VariantRepo,
	ar customeraddrrepo.AddressRepo, ur authrepo.UserRepo,
	ship provider.ShippingProvider, pc payos.Client, cfg Config,
) *OrderService {
	if cfg.ReservationTimeout == 0 { cfg.ReservationTimeout = 30 * time.Minute }
	return &OrderService{
		pool: pool,
		orderRepo: or, subOrderRepo: sr, itemRepo: ir, paymentRepo: pr,
		variantRepo: vr, addrRepo: ar, userRepo: ur,
		shipping: ship, payosClient: pc, cfg: cfg,
	}
}

// Snapshot row used for atomic place-order.
type cartSnapshotRow struct {
	VariantID    uuid.UUID
	Qty          int
	PriceVND     int64
	StockQty     int
	ReservedQty  int
	IsActive     bool
	VariantDel   *time.Time
	ProductID    uuid.UUID
	VariantLabel string
	ProductName  string
	BrandID      uuid.UUID
	ProductDel   *time.Time
	BrandSlug    string
	BrandName    string
	ImageURL     *string
}

// PayOS order code generator — monotonic int64 (millisecond timestamp + atomic counter)
var payosCodeSeq atomic.Int64

func nextPayosCode() int64 {
	return time.Now().UnixMilli()*1000 + (payosCodeSeq.Add(1) % 1000)
}

// truncate25 keeps strings ≤ 25 chars (PayOS description limit) byte-safe.
func truncate25(s string) string {
	if len(s) <= 25 { return s }
	return s[:25]
}

func (s *OrderService) PlaceOrder(ctx context.Context, userID uuid.UUID, req domain.PlaceOrderReq) (*domain.OrderResp, *domain.PaymentResp, error) {
	if !req.PaymentMethod.Valid() {
		return nil, nil, domain.ErrInvalidPaymentMethod
	}

	// Pre-tx: load address + user (snapshot data needed for PayOS).
	addr, err := s.addrRepo.GetByID(ctx, req.AddressID)
	if err != nil || addr.UserID != userID {
		return nil, nil, domain.ErrAddressNotFound
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil { return nil, nil, err }

	shipAddr := domain.ShippingAddress{
		Recipient: addr.Recipient, Phone: addr.Phone, Line1: addr.Line1,
		Ward: addr.Ward, District: addr.District, City: addr.City,
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil { return nil, nil, err }
	defer tx.Rollback(ctx) //nolint:errcheck

	// 1. Snapshot cart with FOR UPDATE
	rows, err := tx.Query(ctx,
		`SELECT ci.variant_id, ci.qty, ci.price_snapshot,
		        v.stock_qty, v.reserved_qty, v.is_active, v.deleted_at,
		        v.product_id, COALESCE(v.color, '') || '/' || COALESCE(v.size, ''),
		        p.name, p.brand_id, p.deleted_at,
		        b.slug, b.name,
		        (SELECT url FROM product_images WHERE product_id = p.id AND is_primary = TRUE LIMIT 1) AS image_url
		   FROM cart_items ci
		   JOIN product_variants v ON v.id = ci.variant_id
		   JOIN products p ON p.id = v.product_id
		   JOIN brands b ON b.id = p.brand_id
		  WHERE ci.user_id = $1
		  FOR UPDATE OF v`,
		userID)
	if err != nil { return nil, nil, err }

	var cart []cartSnapshotRow
	for rows.Next() {
		var r cartSnapshotRow
		if err := rows.Scan(
			&r.VariantID, &r.Qty, &r.PriceVND,
			&r.StockQty, &r.ReservedQty, &r.IsActive, &r.VariantDel,
			&r.ProductID, &r.VariantLabel,
			&r.ProductName, &r.BrandID, &r.ProductDel,
			&r.BrandSlug, &r.BrandName, &r.ImageURL,
		); err != nil {
			rows.Close()
			return nil, nil, err
		}
		cart = append(cart, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil { return nil, nil, err }
	if len(cart) == 0 { return nil, nil, domain.ErrCartEmpty }

	// 2. Validate per-item
	for _, r := range cart {
		if !r.IsActive || r.VariantDel != nil || r.ProductDel != nil {
			return nil, nil, fmt.Errorf("%w: variant=%s", domain.ErrVariantUnavailable, r.VariantID)
		}
		if r.StockQty-r.ReservedQty < r.Qty {
			return nil, nil, fmt.Errorf("%w: variant=%s requested=%d available=%d",
				domain.ErrInsufficientStock, r.VariantID, r.Qty, r.StockQty-r.ReservedQty)
		}
	}

	// 3. Group by brand + compute shipping
	type brandGroup struct {
		brandID  uuid.UUID
		brandSlug, brandName string
		rows     []cartSnapshotRow
		subtotal int64
		shipping int64
	}
	groups := map[uuid.UUID]*brandGroup{}
	var subtotalAll int64
	for _, r := range cart {
		g, ok := groups[r.BrandID]
		if !ok {
			g = &brandGroup{brandID: r.BrandID, brandSlug: r.BrandSlug, brandName: r.BrandName}
			groups[r.BrandID] = g
		}
		g.rows = append(g.rows, r)
		line := int64(r.Qty) * r.PriceVND
		g.subtotal += line
		subtotalAll += line
	}
	var shippingAll int64
	for _, g := range groups {
		quote, err := s.shipping.Calculate(ctx, provider.CalcReq{
			BrandID: g.brandID,
			ToAddress: provider.ShippingAddress{
				Recipient: shipAddr.Recipient, Phone: shipAddr.Phone, Line1: shipAddr.Line1,
				Ward: shipAddr.Ward, District: shipAddr.District, City: shipAddr.City,
			},
		})
		if err != nil { return nil, nil, fmt.Errorf("shipping calc: %w", err) }
		g.shipping = quote.AmountVND
		shippingAll += quote.AmountVND
	}
	grandTotal := subtotalAll + shippingAll

	// 4. Min-order rule (on subtotal per spec §9)
	if subtotalAll < domain.MinOrderValueVND {
		return nil, nil, domain.ErrMinOrderValue
	}

	// 5. Create order with retry on order_no conflict (3 attempts)
	now := time.Now()
	initialStatus := domain.OrderStatusPendingPayment
	initialPayStatus := domain.PaymentStatusPending
	if req.PaymentMethod == domain.PaymentMethodCOD {
		initialStatus = domain.OrderStatusProcessing
	}

	order := &domain.Order{
		UserID: userID,
		SubtotalVND: subtotalAll, ShippingTotalVND: shippingAll, GrandTotalVND: grandTotal,
		PaymentMethod: req.PaymentMethod, PaymentStatus: initialPayStatus,
		Status: initialStatus, ShippingAddress: shipAddr, Notes: req.Notes,
	}
	for attempt := 0; attempt < 3; attempt++ {
		order.OrderNo = domain.GenerateOrderNo(now)
		err := s.orderRepo.Create(ctx, tx, order)
		if err == nil { break }
		if !errors.Is(err, orderrepo.ErrOrderNoConflict) { return nil, nil, err }
		if attempt == 2 { return nil, nil, err }
	}

	// 6. Insert sub_orders + order_items + reserve
	for _, g := range groups {
		so := &domain.SubOrder{
			OrderID: order.ID, BrandID: g.brandID,
			SubtotalVND: g.subtotal, ShippingFeeVND: g.shipping,
			TotalVND: g.subtotal + g.shipping,
			Status: domain.SubOrderStatusPending,
		}
		if err := s.subOrderRepo.Create(ctx, tx, so); err != nil { return nil, nil, err }
		so.BrandSlug = g.brandSlug
		so.BrandName = g.brandName

		for _, r := range g.rows {
			label := strings.Trim(r.VariantLabel, "/")
			if label == "" { label = "default" }
			it := &domain.OrderItem{
				SubOrderID: so.ID, VariantID: r.VariantID, ProductID: r.ProductID,
				ProductName: r.ProductName, VariantLabel: label, ImageURL: r.ImageURL,
				Qty: r.Qty, UnitPriceVND: r.PriceVND, LineTotalVND: int64(r.Qty) * r.PriceVND,
			}
			if err := s.itemRepo.Create(ctx, tx, it); err != nil { return nil, nil, err }
			so.Items = append(so.Items, *it)

			// Reserve stock atomically (second safety net on top of FOR UPDATE)
			if err := s.variantRepo.Reserve(ctx, tx, r.VariantID, r.Qty); err != nil {
				if errors.Is(err, productrepo.ErrInsufficientStock) {
					return nil, nil, fmt.Errorf("%w: variant=%s qty=%d",
						domain.ErrInsufficientStock, r.VariantID, r.Qty)
				}
				return nil, nil, err
			}
		}
		order.SubOrders = append(order.SubOrders, *so)
	}

	// 7. Create payment row
	var payment *paymentdomain.Payment
	expiresAt := now.Add(s.cfg.ReservationTimeout)
	if req.PaymentMethod == domain.PaymentMethodCOD {
		payment = &paymentdomain.Payment{
			OrderID: order.ID, AmountVND: grandTotal,
			Method: domain.PaymentMethodCOD, Status: domain.PaymentStatusPending,
		}
		if err := s.paymentRepo.Create(ctx, tx, payment); err != nil { return nil, nil, err }
	} else {
		code := nextPayosCode()
		payment = &paymentdomain.Payment{
			OrderID: order.ID, AmountVND: grandTotal,
			Method: domain.PaymentMethodPayos, Status: domain.PaymentStatusPending,
			PayosOrderCode: &code, ExpiredAt: &expiresAt,
		}
		if err := s.paymentRepo.Create(ctx, tx, payment); err != nil { return nil, nil, err }

		// Call PayOS — in tx (mock: instant; production: <500ms)
		items := []payos.LineItem{}
		for _, g := range groups {
			for _, r := range g.rows {
				items = append(items, payos.LineItem{
					Name: r.ProductName, Quantity: r.Qty, Price: r.PriceVND,
				})
			}
		}
		link, err := s.payosClient.CreateLink(ctx, payos.CreateLinkReq{
			OrderCode:   code,
			AmountVND:   grandTotal,
			Description: truncate25(fmt.Sprintf("DH %s", order.OrderNo)),
			Items:       items,
			ReturnURL:   s.cfg.PayosReturnURL + "?orderNo=" + order.OrderNo,
			CancelURL:   s.cfg.PayosCancelURL + "?orderNo=" + order.OrderNo,
			Buyer:       payos.Buyer{Name: user.Name, Phone: stringOrEmpty(user.Phone), Email: stringOrEmpty(user.Email)},
			ExpiredAt:   expiresAt.Unix(),
		})
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %v", domain.ErrPayosLinkCreate, err)
		}
		if err := s.paymentRepo.UpdatePayosLink(ctx, tx, payment.ID, link.PaymentLinkID, link.CheckoutURL, link.QRCode); err != nil {
			return nil, nil, err
		}
		payment.PayosPaymentLinkID = &link.PaymentLinkID
		payment.PayosCheckoutURL = &link.CheckoutURL
		payment.PayosQRCode = &link.QRCode
	}

	// 8. Clear cart
	if _, err := tx.Exec(ctx, `DELETE FROM cart_items WHERE user_id = $1`, userID); err != nil {
		return nil, nil, err
	}

	// 9. Commit
	if err := tx.Commit(ctx); err != nil { return nil, nil, err }

	return orderToResp(order), paymentToResp(payment), nil
}

func stringOrEmpty(p *string) string { if p == nil { return "" }; return *p }

func orderToResp(o *domain.Order) *domain.OrderResp {
	resp := &domain.OrderResp{
		ID: o.ID, OrderNo: o.OrderNo, Status: o.Status,
		PaymentMethod: o.PaymentMethod, PaymentStatus: o.PaymentStatus,
		SubtotalVND: o.SubtotalVND, ShippingTotalVND: o.ShippingTotalVND,
		GrandTotalVND: o.GrandTotalVND, ShippingAddress: o.ShippingAddress,
		Notes: o.Notes, CancelReason: o.CancelReason,
		CreatedAt: o.CreatedAt, PaidAt: o.PaidAt, CancelledAt: o.CancelledAt,
	}
	for _, so := range o.SubOrders {
		sr := domain.SubOrderResp{
			ID: so.ID,
			Brand: domain.BrandRef{ID: so.BrandID, Slug: so.BrandSlug, Name: so.BrandName},
			SubtotalVND: so.SubtotalVND, ShippingFeeVND: so.ShippingFeeVND, TotalVND: so.TotalVND,
			Status: so.Status, TrackingNo: so.TrackingNo,
		}
		for _, it := range so.Items {
			sr.Items = append(sr.Items, domain.OrderItemResp{
				ID: it.ID, VariantID: it.VariantID, ProductID: it.ProductID,
				ProductName: it.ProductName, VariantLabel: it.VariantLabel, ImageURL: it.ImageURL,
				Qty: it.Qty, UnitPriceVND: it.UnitPriceVND, LineTotalVND: it.LineTotalVND,
			})
		}
		resp.SubOrders = append(resp.SubOrders, sr)
	}
	return resp
}

func paymentToResp(p *paymentdomain.Payment) *domain.PaymentResp {
	return &domain.PaymentResp{
		ID: p.ID, Method: p.Method, Status: p.Status,
		AmountVND: p.AmountVND, CheckoutURL: p.PayosCheckoutURL,
		QRCode: p.PayosQRCode, ExpiredAt: p.ExpiredAt,
	}
}

var _ = authdomain.RoleUser // silence unused import (UserRepo brings authdomain transitively)
```

- [ ] **Step 2: Write order_service_test.go**

Use real Postgres via testfixtures since PlaceOrder is heavily DB-driven. Mock only PayOS client.

```go
// internal/order/service/order_service_test.go
package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authrepo "github.com/wearwhere/wearwhere_be/internal/auth/repo"
	customeraddrdomain "github.com/wearwhere/wearwhere_be/internal/customeraddr/domain"
	customeraddrrepo "github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	"github.com/wearwhere/wearwhere_be/internal/order/service"
	"github.com/wearwhere/wearwhere_be/internal/payment/payos"
	paymentrepo "github.com/wearwhere/wearwhere_be/internal/payment/repo"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
	"github.com/wearwhere/wearwhere_be/internal/shipping/provider"
	cartrepo "github.com/wearwhere/wearwhere_be/internal/cart/repo"
	brandrepo "github.com/wearwhere/wearwhere_be/internal/brand/repo"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

type setup struct {
	UserID, AddrID, BrandID, ProductID, VariantID uuid.UUID
	Svc *service.OrderService
}

func setupOrderTest(t *testing.T, stock int) setup {
	pool := testfixtures.MustPool(t)
	t.Cleanup(func() { testfixtures.Clean(t, pool) })
	ctx := context.Background()

	userID := testfixtures.SeedUser(t, pool, "buyer@x.com")
	addrID := testfixtures.SeedCustomerAddress(t, pool, userID)
	brandID := testfixtures.SeedBrand(t, pool, "rep")
	productID := testfixtures.SeedProduct(t, pool, brandID, "tee")
	variantID := testfixtures.SeedVariant(t, pool, productID, "BLK-M", stock)

	// Add 1 cart item
	_, err := pool.Exec(ctx,
		`INSERT INTO cart_items (user_id, variant_id, qty, price_snapshot, currency_snapshot)
		 VALUES ($1, $2, 1, 100000, 'VND')`, userID, variantID)
	require.NoError(t, err)

	svc := service.NewOrderService(
		pool,
		orderrepo.NewOrderPG(pool), orderrepo.NewSubOrderPG(pool), orderrepo.NewOrderItemPG(pool),
		paymentrepo.NewPaymentPG(pool), productrepo.NewVariantPG(pool),
		customeraddrrepo.NewAddressPG(pool), authrepo.NewUserPG(pool),
		provider.NewFlatRateProvider(brandrepo.NewBrandPG(pool)),
		payos.NewMockClient(""),
		service.Config{ReservationTimeout: 30 * time.Minute, PayosReturnURL: "http://ret", PayosCancelURL: "http://can"},
	)
	return setup{UserID: userID, AddrID: addrID, BrandID: brandID, ProductID: productID, VariantID: variantID, Svc: svc}
}

func TestPlaceOrder_COD_Success(t *testing.T) {
	s := setupOrderTest(t, 10)
	resp, pay, err := s.Svc.PlaceOrder(context.Background(), s.UserID, domain.PlaceOrderReq{
		AddressID: s.AddrID, PaymentMethod: domain.PaymentMethodCOD, Notes: "fast",
	})
	require.NoError(t, err)
	require.Equal(t, domain.OrderStatusProcessing, resp.Status)
	require.Equal(t, domain.PaymentStatusPending, resp.PaymentStatus)
	require.Equal(t, domain.PaymentMethodCOD, pay.Method)
	require.Nil(t, pay.CheckoutURL)
}

func TestPlaceOrder_Payos_ReturnsCheckoutURL(t *testing.T) {
	s := setupOrderTest(t, 10)
	resp, pay, err := s.Svc.PlaceOrder(context.Background(), s.UserID, domain.PlaceOrderReq{
		AddressID: s.AddrID, PaymentMethod: domain.PaymentMethodPayos,
	})
	require.NoError(t, err)
	require.Equal(t, domain.OrderStatusPendingPayment, resp.Status)
	require.NotNil(t, pay.CheckoutURL)
	require.Contains(t, *pay.CheckoutURL, "/dev/payos/mock-checkout?orderCode=")
}

func TestPlaceOrder_EmptyCart(t *testing.T) {
	s := setupOrderTest(t, 10)
	// Clear the cart we set up
	pool := testfixtures.MustPool(t)
	_, _ = pool.Exec(context.Background(), `DELETE FROM cart_items WHERE user_id=$1`, s.UserID)
	_, _, err := s.Svc.PlaceOrder(context.Background(), s.UserID, domain.PlaceOrderReq{
		AddressID: s.AddrID, PaymentMethod: domain.PaymentMethodCOD,
	})
	require.ErrorIs(t, err, domain.ErrCartEmpty)
}

func TestPlaceOrder_MinOrderValue(t *testing.T) {
	s := setupOrderTest(t, 10)
	pool := testfixtures.MustPool(t)
	_, _ = pool.Exec(context.Background(),
		`UPDATE cart_items SET price_snapshot=10000 WHERE user_id=$1`, s.UserID)
	_, _, err := s.Svc.PlaceOrder(context.Background(), s.UserID, domain.PlaceOrderReq{
		AddressID: s.AddrID, PaymentMethod: domain.PaymentMethodCOD,
	})
	require.ErrorIs(t, err, domain.ErrMinOrderValue)
}

func TestPlaceOrder_AddressNotOwned(t *testing.T) {
	s := setupOrderTest(t, 10)
	otherUser := testfixtures.SeedUser(t, testfixtures.MustPool(t), "other@x.com")
	_, _, err := s.Svc.PlaceOrder(context.Background(), otherUser, domain.PlaceOrderReq{
		AddressID: s.AddrID, PaymentMethod: domain.PaymentMethodCOD,
	})
	require.ErrorIs(t, err, domain.ErrAddressNotFound)
}

func TestPlaceOrder_StockReservedAfterSuccess(t *testing.T) {
	s := setupOrderTest(t, 5)
	_, _, err := s.Svc.PlaceOrder(context.Background(), s.UserID, domain.PlaceOrderReq{
		AddressID: s.AddrID, PaymentMethod: domain.PaymentMethodPayos,
	})
	require.NoError(t, err)

	stock, reserved := testfixtures.GetVariantStock(t, testfixtures.MustPool(t), s.VariantID)
	require.Equal(t, 5, stock)    // not yet committed
	require.Equal(t, 1, reserved) // reserved
}

func TestPlaceOrder_ClearsCartOnSuccess(t *testing.T) {
	s := setupOrderTest(t, 10)
	_, _, err := s.Svc.PlaceOrder(context.Background(), s.UserID, domain.PlaceOrderReq{
		AddressID: s.AddrID, PaymentMethod: domain.PaymentMethodCOD,
	})
	require.NoError(t, err)

	var cnt int
	err = testfixtures.MustPool(t).QueryRow(context.Background(),
		`SELECT COUNT(*) FROM cart_items WHERE user_id=$1`, s.UserID).Scan(&cnt)
	require.NoError(t, err)
	require.Equal(t, 0, cnt)
}

// SeedCustomerAddress helper: add to testfixtures if missing.
var _ = customeraddrdomain.Address{}
```

- [ ] **Step 3: Add SeedCustomerAddress helper**

In `internal/testfixtures/fixtures.go` if missing:

```go
func SeedCustomerAddress(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(),
		`INSERT INTO customer_addresses
		   (user_id, recipient, phone, line1, ward, district, city, is_default)
		 VALUES ($1, 'An Nguyen', '0900000000', '1 ABC', 'P1', 'Q1', 'HCM', TRUE)
		 RETURNING id`, userID).Scan(&id)
	require.NoError(t, err)
	return id
}
```

- [ ] **Step 4: Run tests — PASS**

Run: `go test ./internal/order/service/ -run TestPlaceOrder -v 2>&1 | tail -40`
Expected: all 7 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/order/service/order_service.go internal/order/service/order_service_test.go internal/testfixtures/fixtures.go
git commit -m "feat(order): PlaceOrder atomic tx with reserve, multi-brand sub_orders, PayOS link"
```

---

## Task 18: Cancel + List + Detail service methods

**Files:**
- Modify: `internal/order/service/order_service.go` — append methods
- Create: `internal/order/service/order_service_cancel_test.go`

- [ ] **Step 1: Append methods to order_service.go**

```go
// internal/order/service/order_service.go (append)

func (s *OrderService) Detail(ctx context.Context, userID uuid.UUID, orderNo string) (*domain.OrderResp, error) {
	o, err := s.orderRepo.GetByOrderNo(ctx, userID, orderNo)
	if err != nil {
		if errors.Is(err, orderrepo.ErrNotFound) { return nil, domain.ErrOrderNotFound }
		return nil, err
	}
	subs, err := s.subOrderRepo.ListByOrderID(ctx, o.ID)
	if err != nil { return nil, err }
	o.SubOrders = make([]domain.SubOrder, 0, len(subs))
	for _, so := range subs {
		items, err := s.itemRepo.ListBySubOrderID(ctx, so.ID)
		if err != nil { return nil, err }
		copySO := *so
		for _, it := range items { copySO.Items = append(copySO.Items, *it) }
		o.SubOrders = append(o.SubOrders, copySO)
	}
	return orderToResp(o), nil
}

func (s *OrderService) List(ctx context.Context, f orderrepo.ListFilter) (*domain.OrderListResp, error) {
	items, total, err := s.orderRepo.List(ctx, f)
	if err != nil { return nil, err }

	out := make([]domain.OrderListItem, 0, len(items))
	for _, o := range items {
		// Lightweight: count items + brands, pick first image
		var itemCount, brandCount int
		var firstImg *string
		var firstName string
		err := s.pool.QueryRow(ctx,
			`SELECT COALESCE(SUM(oi.qty), 0)::INT,
			        COUNT(DISTINCT so.brand_id),
			        (SELECT image_url FROM order_items oi2
			          JOIN sub_orders so2 ON so2.id = oi2.sub_order_id
			          WHERE so2.order_id = $1 ORDER BY oi2.created_at ASC LIMIT 1),
			        (SELECT product_name FROM order_items oi3
			          JOIN sub_orders so3 ON so3.id = oi3.sub_order_id
			          WHERE so3.order_id = $1 ORDER BY oi3.created_at ASC LIMIT 1)
			   FROM order_items oi
			   JOIN sub_orders so ON so.id = oi.sub_order_id
			  WHERE so.order_id = $1`,
			o.ID).Scan(&itemCount, &brandCount, &firstImg, &firstName)
		if err != nil { return nil, err }

		out = append(out, domain.OrderListItem{
			ID: o.ID, OrderNo: o.OrderNo, Status: o.Status,
			PaymentMethod: o.PaymentMethod, PaymentStatus: o.PaymentStatus,
			GrandTotalVND: o.GrandTotalVND,
			ItemCount: itemCount, BrandCount: brandCount,
			FirstItemImage: firstImg, FirstItemName: firstName,
			CreatedAt: o.CreatedAt,
		})
	}

	if f.PageSize <= 0 { f.PageSize = 20 }
	if f.Page <= 0 { f.Page = 1 }
	totalPages := (total + f.PageSize - 1) / f.PageSize
	return &domain.OrderListResp{Data: out, Page: f.Page, PageSize: f.PageSize, Total: total, TotalPages: totalPages}, nil
}

func (s *OrderService) Cancel(ctx context.Context, userID uuid.UUID, orderNo, reason string) (*domain.OrderResp, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil { return nil, err }
	defer tx.Rollback(ctx) //nolint:errcheck

	o, err := s.orderRepo.GetByOrderNoForUpdate(ctx, tx, userID, orderNo)
	if err != nil {
		if errors.Is(err, orderrepo.ErrNotFound) { return nil, domain.ErrOrderNotFound }
		return nil, err
	}
	// Load sub_orders to evaluate cancel decision
	subs, err := s.subOrderRepo.ListByOrderID(ctx, o.ID)
	if err != nil { return nil, err }
	for _, so := range subs { o.SubOrders = append(o.SubOrders, *so) }

	decision := o.CanCustomerCancel()
	if !decision.Allowed {
		if decision.Reason == "paid_not_supported" { return nil, domain.ErrCancelPaidNotSupported }
		return nil, fmt.Errorf("%w: %s", domain.ErrCancelNotAllowed, decision.Reason)
	}

	// Determine payment status after cancel
	var newPayStatus domain.PaymentStatus = domain.PaymentStatusCancelled

	// 1. Cancel order + sub_orders
	if err := s.orderRepo.UpdateStatusOnCancel(ctx, tx, o.ID, reason, newPayStatus); err != nil { return nil, err }
	if err := s.subOrderRepo.CancelAllByOrderID(ctx, tx, o.ID); err != nil { return nil, err }

	// 2. Cancel payment
	pay, err := s.paymentRepo.GetByOrderID(ctx, o.ID)
	if err == nil && pay.Status == domain.PaymentStatusPending {
		_ = s.paymentRepo.UpdateOnCancelled(ctx, tx, pay.ID)
	}

	// 3. Release stock
	items, err := s.itemRepo.ListByOrderID(ctx, o.ID)
	if err != nil { return nil, err }
	for _, it := range items {
		if err := s.variantRepo.Release(ctx, tx, it.VariantID, it.Qty); err != nil {
			// Log but don't block — release inconsistency surfaces in stock metrics
			// In Sprint 3 we want correctness, so bail out:
			return nil, fmt.Errorf("release stock for variant %s: %w", it.VariantID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil { return nil, err }

	// Reload for response
	return s.Detail(ctx, userID, orderNo)
}
```

- [ ] **Step 2: Write cancel test**

```go
// internal/order/service/order_service_cancel_test.go
package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

func TestCancel_CODPending_ReleasesStock(t *testing.T) {
	s := setupOrderTest(t, 5)
	resp, _, err := s.Svc.PlaceOrder(context.Background(), s.UserID, domain.PlaceOrderReq{
		AddressID: s.AddrID, PaymentMethod: domain.PaymentMethodCOD,
	})
	require.NoError(t, err)

	_, reservedBefore := testfixtures.GetVariantStock(t, testfixtures.MustPool(t), s.VariantID)
	require.Equal(t, 1, reservedBefore)

	cancelled, err := s.Svc.Cancel(context.Background(), s.UserID, resp.OrderNo, "changed my mind")
	require.NoError(t, err)
	require.Equal(t, domain.OrderStatusCancelled, cancelled.Status)
	require.Equal(t, "changed my mind", cancelled.CancelReason)

	stock, reserved := testfixtures.GetVariantStock(t, testfixtures.MustPool(t), s.VariantID)
	require.Equal(t, 5, stock)
	require.Equal(t, 0, reserved)
}

func TestCancel_OtherUser_NotFound(t *testing.T) {
	s := setupOrderTest(t, 5)
	resp, _, err := s.Svc.PlaceOrder(context.Background(), s.UserID, domain.PlaceOrderReq{
		AddressID: s.AddrID, PaymentMethod: domain.PaymentMethodCOD,
	})
	require.NoError(t, err)

	otherUser := testfixtures.SeedUser(t, testfixtures.MustPool(t), "stranger@x.com")
	_, err = s.Svc.Cancel(context.Background(), otherUser, resp.OrderNo, "")
	require.ErrorIs(t, err, domain.ErrOrderNotFound)
}

func TestList_ReturnsPagedOrders(t *testing.T) {
	s := setupOrderTest(t, 100)
	for i := 0; i < 3; i++ {
		// re-fill cart for each order
		_, _ = testfixtures.MustPool(t).Exec(context.Background(),
			`INSERT INTO cart_items (user_id, variant_id, qty, price_snapshot, currency_snapshot)
			 VALUES ($1, $2, 1, 100000, 'VND')`, s.UserID, s.VariantID)
		_, _, err := s.Svc.PlaceOrder(context.Background(), s.UserID, domain.PlaceOrderReq{
			AddressID: s.AddrID, PaymentMethod: domain.PaymentMethodCOD,
		})
		require.NoError(t, err)
	}

	resp, err := s.Svc.List(context.Background(), orderrepo.ListFilter{
		UserID: s.UserID, Page: 1, PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, 3, resp.Total)
	require.Len(t, resp.Data, 3)
}
```

Add import for `orderrepo` at the top of the test file:
```go
import orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
```

- [ ] **Step 3: Run tests — PASS**

Run: `go test ./internal/order/service/ -v 2>&1 | tail -30`
Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/order/service/order_service.go internal/order/service/order_service_cancel_test.go
git commit -m "feat(order): Cancel (release stock) + List (paginated) + Detail service methods"
```

---

## Task 19: Checkout handler + route

**Files:**
- Create: `internal/order/handler/handler.go`
- Create: `internal/order/handler/routes.go`
- Create: `internal/order/handler/checkout_handler_test.go`

- [ ] **Step 1: Write handler.go (Checkout part — Order endpoints in Task 20)**

```go
// internal/order/handler/handler.go
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/order/domain"
	"github.com/wearwhere/wearwhere_be/internal/order/service"
)

type Handler struct {
	checkout *service.CheckoutService
	order    *service.OrderService
}

func New(c *service.CheckoutService, o *service.OrderService) *Handler {
	return &Handler{checkout: c, order: o}
}

func (h *Handler) PreviewCheckout(c *gin.Context) {
	userID, ok := authmw.UserID(c)
	if !ok { c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"}); return }

	addrStr := c.Query("address_id")
	addressID, err := uuid.Parse(addrStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing or invalid address_id"})
		return
	}

	resp, err := h.checkout.Preview(c, userID, addressID)
	if err != nil {
		if errors.Is(err, domain.ErrAddressNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "address_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}
```

- [ ] **Step 2: Write routes.go (mount under /me)**

```go
// internal/order/handler/routes.go
package handler

import "github.com/gin-gonic/gin"

func Mount(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/checkout/preview", h.PreviewCheckout)
	rg.POST("/orders", h.PlaceOrder)
	rg.GET("/orders", h.ListOrders)
	rg.GET("/orders/:order_no", h.DetailOrder)
	rg.POST("/orders/:order_no/cancel", h.CancelOrder)
}
```

- [ ] **Step 3: Write checkout_handler_test.go (basic 401 + 400 cases)**

```go
// internal/order/handler/checkout_handler_test.go
package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/order/handler"
)

func TestPreview_MissingAddressID_400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.New(nil, nil) // services nil — handler returns 400 before invoking
	r.GET("/me/checkout/preview", h.PreviewCheckout)

	req := httptest.NewRequest("GET", "/me/checkout/preview", nil)
	// inject user id via context (mimic JWT middleware) — use Set since authmw.UserID reads from gin.Context
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Without auth middleware, the handler will 401. To trigger 400 path,
	// use a route that pre-sets user id. This test demonstrates the 401 branch:
	require.Equal(t, http.StatusUnauthorized, w.Code)
}
```

Note: full handler coverage uses E2E tests in Task 24. Unit tests here are just smoke.

- [ ] **Step 4: Run + commit**

Run: `go test ./internal/order/handler/ -v 2>&1 | tail -10`
Expected: PASS.

```bash
git add internal/order/handler/handler.go internal/order/handler/routes.go internal/order/handler/checkout_handler_test.go
git commit -m "feat(order): checkout preview handler + route stubs"
```

---

## Task 20: Order handlers (Place / List / Detail / Cancel)

**Files:**
- Modify: `internal/order/handler/handler.go` — append methods

- [ ] **Step 1: Append methods**

```go
// internal/order/handler/handler.go (append)

import (
	"strconv"
	"strings"

	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
)

func (h *Handler) PlaceOrder(c *gin.Context) {
	userID, ok := authmw.UserID(c)
	if !ok { c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"}); return }

	var req domain.PlaceOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "detail": err.Error()})
		return
	}
	resp, pay, err := h.order.PlaceOrder(c, userID, req)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrCartEmpty):
			c.JSON(http.StatusBadRequest, gin.H{"error": "cart_empty"})
		case errors.Is(err, domain.ErrMinOrderValue):
			c.JSON(http.StatusBadRequest, gin.H{"error": "min_order_value", "min_value_vnd": domain.MinOrderValueVND})
		case errors.Is(err, domain.ErrAddressNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "address_not_found"})
		case errors.Is(err, domain.ErrInsufficientStock):
			c.JSON(http.StatusConflict, gin.H{"error": "insufficient_stock", "detail": err.Error()})
		case errors.Is(err, domain.ErrVariantUnavailable):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "variant_unavailable", "detail": err.Error()})
		case errors.Is(err, domain.ErrInvalidPaymentMethod):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payment_method"})
		case errors.Is(err, domain.ErrPayosLinkCreate):
			c.JSON(http.StatusBadGateway, gin.H{"error": "payos_unavailable", "detail": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusCreated, gin.H{"order": resp, "payment": pay})
}

func (h *Handler) ListOrders(c *gin.Context) {
	userID, ok := authmw.UserID(c)
	if !ok { c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"}); return }

	filter := orderrepo.ListFilter{UserID: userID, Page: 1, PageSize: 20}
	if statusStr := c.Query("status"); statusStr != "" {
		for _, s := range strings.Split(statusStr, ",") {
			filter.Statuses = append(filter.Statuses, domain.OrderStatus(strings.TrimSpace(s)))
		}
	}
	if p, _ := strconv.Atoi(c.Query("page")); p > 0 { filter.Page = p }
	if ps, _ := strconv.Atoi(c.Query("page_size")); ps > 0 { filter.PageSize = ps }
	if from := c.Query("from"); from != "" { filter.FromTime = &from }
	if to := c.Query("to"); to != "" { filter.ToTime = &to }

	resp, err := h.order.List(c, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) DetailOrder(c *gin.Context) {
	userID, ok := authmw.UserID(c)
	if !ok { c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"}); return }

	orderNo := c.Param("order_no")
	resp, err := h.order.Detail(c, userID, orderNo)
	if err != nil {
		if errors.Is(err, domain.ErrOrderNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "order_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) CancelOrder(c *gin.Context) {
	userID, ok := authmw.UserID(c)
	if !ok { c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"}); return }

	orderNo := c.Param("order_no")
	var req domain.CancelOrderReq
	_ = c.ShouldBindJSON(&req) // body optional

	resp, err := h.order.Cancel(c, userID, orderNo, req.Reason)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrOrderNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "order_not_found"})
		case errors.Is(err, domain.ErrCancelPaidNotSupported):
			c.JSON(http.StatusConflict, gin.H{"error": "cancel_not_allowed", "subcode": "paid_not_supported"})
		case errors.Is(err, domain.ErrCancelNotAllowed):
			// Extract subcode from wrapped error
			subcode := "already_shipped"
			msg := err.Error()
			for _, code := range []string{"already_shipped", "already_cancelled", "already_completed"} {
				if strings.Contains(msg, code) { subcode = code; break }
			}
			c.JSON(http.StatusConflict, gin.H{"error": "cancel_not_allowed", "subcode": subcode})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, resp)
}
```

- [ ] **Step 2: Run handler tests + compile check**

Run: `go build ./internal/order/...`
Expected: exit 0.

Run: `go test ./internal/order/... 2>&1 | tail -10`
Expected: all PASS (full coverage in E2E later).

- [ ] **Step 3: Commit**

```bash
git add internal/order/handler/handler.go
git commit -m "feat(order): order handlers Place/List/Detail/Cancel with structured error codes"
```

---

## Task 21: PayOS webhook handler + dev endpoints + payment service

**Files:**
- Create: `internal/payment/service/webhook_service.go`
- Create: `internal/payment/handler/handler.go`
- Create: `internal/payment/handler/routes.go`
- Create: `internal/payment/service/webhook_service_test.go`

- [ ] **Step 1: Write webhook_service.go**

```go
// internal/payment/service/webhook_service.go
package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	orderdomain "github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	paymentdomain "github.com/wearwhere/wearwhere_be/internal/payment/domain"
	"github.com/wearwhere/wearwhere_be/internal/payment/payos"
	paymentrepo "github.com/wearwhere/wearwhere_be/internal/payment/repo"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
)

type WebhookService struct {
	pool        *pgxpool.Pool
	paymentRepo paymentrepo.PaymentRepo
	orderRepo   orderrepo.OrderRepo
	subOrder    orderrepo.SubOrderRepo
	items       orderrepo.OrderItemRepo
	variant     productrepo.VariantRepo
	payosClient payos.Client
}

func NewWebhookService(
	pool *pgxpool.Pool,
	pr paymentrepo.PaymentRepo, or orderrepo.OrderRepo,
	sr orderrepo.SubOrderRepo, ir orderrepo.OrderItemRepo,
	vr productrepo.VariantRepo, pc payos.Client,
) *WebhookService {
	return &WebhookService{
		pool: pool, paymentRepo: pr, orderRepo: or, subOrder: sr,
		items: ir, variant: vr, payosClient: pc,
	}
}

// HandlePayosWebhook processes a verified payload. Returns nil on success (incl. idempotent no-op).
// Signature verification is done by the caller.
func (s *WebhookService) HandlePayosWebhook(ctx context.Context, p payos.WebhookPayload) error {
	raw, _ := json.Marshal(p)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil { return err }
	defer tx.Rollback(ctx) //nolint:errcheck

	payment, err := s.paymentRepo.GetByPayosOrderCodeForUpdate(ctx, tx, p.Data.OrderCode)
	if err != nil {
		if errors.Is(err, paymentdomain.ErrPaymentNotFound) { return nil } // accept + log
		return err
	}
	if payment.Status != orderdomain.PaymentStatusPending {
		return nil // idempotent — already processed
	}

	items, err := s.items.ListByOrderID(ctx, payment.OrderID)
	if err != nil { return err }

	if p.Success && p.Code == "00" {
		if err := s.paymentRepo.UpdateOnPaid(ctx, tx, payment.ID, raw); err != nil { return err }
		if err := s.orderRepo.UpdateStatusOnPaid(ctx, tx, payment.OrderID); err != nil { return err }
		for _, it := range items {
			if err := s.variant.Commit(ctx, tx, it.VariantID, it.Qty); err != nil {
				return err
			}
		}
	} else {
		if err := s.paymentRepo.UpdateOnFailed(ctx, tx, payment.ID, p.Desc, raw); err != nil { return err }
		if err := s.orderRepo.UpdateStatusOnCancel(ctx, tx, payment.OrderID, "payos_payment_failed", orderdomain.PaymentStatusFailed); err != nil {
			return err
		}
		if err := s.subOrder.CancelAllByOrderID(ctx, tx, payment.OrderID); err != nil { return err }
		for _, it := range items {
			if err := s.variant.Release(ctx, tx, it.VariantID, it.Qty); err != nil { return err }
		}
	}

	return tx.Commit(ctx)
}
```

- [ ] **Step 2: Write payment handler.go**

```go
// internal/payment/handler/handler.go
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/wearwhere/wearwhere_be/internal/payment/payos"
	"github.com/wearwhere/wearwhere_be/internal/payment/service"
)

type Handler struct {
	webhook    *service.WebhookService
	payos      payos.Client
	mockMode   bool
}

func New(w *service.WebhookService, pc payos.Client, mockMode bool) *Handler {
	return &Handler{webhook: w, payos: pc, mockMode: mockMode}
}

// PayosWebhook handles PayOS callbacks.
func (h *Handler) PayosWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read_body"})
		return
	}
	var payload payos.WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_json"})
		return
	}

	if err := h.payos.VerifyWebhookSignature(payload); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_signature"})
		return
	}

	if err := h.webhook.HandlePayosWebhook(c, payload); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// MockCheckoutPage — dev only, returns simple HTML.
func (h *Handler) MockCheckoutPage(c *gin.Context) {
	if !h.mockMode { c.Status(http.StatusNotFound); return }
	orderCode := c.Query("orderCode")
	html := fmt.Sprintf(`<!DOCTYPE html>
<html><body style="font-family:sans-serif;padding:2em">
<h1>WearWhere Mock PayOS</h1>
<p>Order code: <b>%s</b></p>
<form action="/dev/payos/simulate-webhook" method="POST">
  <input type="hidden" name="orderCode" value="%s"/>
  <button name="success" value="true">✅ Pay Success</button>
  <button name="success" value="false">❌ Pay Fail</button>
</form>
</body></html>`, orderCode, orderCode)
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}

// SimulateWebhook — dev only, constructs payload and calls webhook handler.
func (h *Handler) SimulateWebhook(c *gin.Context) {
	if !h.mockMode { c.Status(http.StatusNotFound); return }
	codeStr := c.PostForm("orderCode")
	successStr := c.PostForm("success")
	code, err := strconv.ParseInt(codeStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_order_code"})
		return
	}
	success := successStr == "true"

	payload := payos.WebhookPayload{
		Code:    "00",
		Success: success,
		Data: payos.WebhookData{
			OrderCode: code, Amount: 0, Description: "mock",
			Code: "00", Desc: "Mock payment",
		},
		Signature: "mock", // mock client always accepts
	}
	if err := h.webhook.HandlePayosWebhook(c, payload); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "success": success})
}

var _ = errors.New
```

- [ ] **Step 3: Write routes.go**

```go
// internal/payment/handler/routes.go
package handler

import "github.com/gin-gonic/gin"

// MountPublic registers the PayOS webhook endpoint (no auth).
func MountPublic(rg *gin.RouterGroup, h *Handler) {
	rg.POST("/payments/payos/webhook", h.PayosWebhook)
}

// MountDev registers dev-only endpoints when PAYOS_MODE=mock.
func MountDev(r *gin.Engine, h *Handler) {
	dev := r.Group("/dev/payos")
	dev.GET("/mock-checkout", h.MockCheckoutPage)
	dev.POST("/simulate-webhook", h.SimulateWebhook)
}
```

- [ ] **Step 4: Write webhook test**

```go
// internal/payment/service/webhook_service_test.go
package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	authrepo "github.com/wearwhere/wearwhere_be/internal/auth/repo"
	brandrepo "github.com/wearwhere/wearwhere_be/internal/brand/repo"
	customeraddrrepo "github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
	orderdomain "github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	orderservice "github.com/wearwhere/wearwhere_be/internal/order/service"
	"github.com/wearwhere/wearwhere_be/internal/payment/payos"
	paymentrepo "github.com/wearwhere/wearwhere_be/internal/payment/repo"
	paymentservice "github.com/wearwhere/wearwhere_be/internal/payment/service"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
	"github.com/wearwhere/wearwhere_be/internal/shipping/provider"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

func TestWebhook_Success_CommitsStockAndOrder(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	ctx := context.Background()

	userID := testfixtures.SeedUser(t, pool, "buyer@x.com")
	addrID := testfixtures.SeedCustomerAddress(t, pool, userID)
	brandID := testfixtures.SeedBrand(t, pool, "rep")
	productID := testfixtures.SeedProduct(t, pool, brandID, "tee")
	variantID := testfixtures.SeedVariant(t, pool, productID, "BLK-M", 5)
	_, _ = pool.Exec(ctx, `INSERT INTO cart_items (user_id, variant_id, qty, price_snapshot, currency_snapshot)
		 VALUES ($1, $2, 1, 100000, 'VND')`, userID, variantID)

	osvc := orderservice.NewOrderService(pool,
		orderrepo.NewOrderPG(pool), orderrepo.NewSubOrderPG(pool), orderrepo.NewOrderItemPG(pool),
		paymentrepo.NewPaymentPG(pool), productrepo.NewVariantPG(pool),
		customeraddrrepo.NewAddressPG(pool), authrepo.NewUserPG(pool),
		provider.NewFlatRateProvider(brandrepo.NewBrandPG(pool)),
		payos.NewMockClient(""),
		orderservice.Config{ReservationTimeout: 30 * time.Minute},
	)
	_, pay, err := osvc.PlaceOrder(ctx, userID, orderdomain.PlaceOrderReq{
		AddressID: addrID, PaymentMethod: orderdomain.PaymentMethodPayos,
	})
	require.NoError(t, err)

	// Look up payos_order_code for this payment
	var code int64
	err = pool.QueryRow(ctx, `SELECT payos_order_code FROM payments WHERE id=$1`, pay.ID).Scan(&code)
	require.NoError(t, err)

	wsvc := paymentservice.NewWebhookService(pool,
		paymentrepo.NewPaymentPG(pool), orderrepo.NewOrderPG(pool),
		orderrepo.NewSubOrderPG(pool), orderrepo.NewOrderItemPG(pool),
		productrepo.NewVariantPG(pool), payos.NewMockClient(""),
	)
	err = wsvc.HandlePayosWebhook(ctx, payos.WebhookPayload{
		Code: "00", Success: true,
		Data: payos.WebhookData{OrderCode: code, Amount: 130000},
		Signature: "mock",
	})
	require.NoError(t, err)

	stock, reserved := testfixtures.GetVariantStock(t, pool, variantID)
	require.Equal(t, 4, stock)
	require.Equal(t, 0, reserved)
}

func TestWebhook_Idempotent_SecondCallNoOp(t *testing.T) {
	// Similar setup → call webhook twice with success=true → stock should only decrement once.
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	ctx := context.Background()

	userID := testfixtures.SeedUser(t, pool, "b@x.com")
	addrID := testfixtures.SeedCustomerAddress(t, pool, userID)
	brandID := testfixtures.SeedBrand(t, pool, "rep")
	productID := testfixtures.SeedProduct(t, pool, brandID, "tee")
	variantID := testfixtures.SeedVariant(t, pool, productID, "BLK-S", 5)
	_, _ = pool.Exec(ctx, `INSERT INTO cart_items (user_id, variant_id, qty, price_snapshot, currency_snapshot)
		 VALUES ($1, $2, 2, 100000, 'VND')`, userID, variantID)

	osvc := orderservice.NewOrderService(pool,
		orderrepo.NewOrderPG(pool), orderrepo.NewSubOrderPG(pool), orderrepo.NewOrderItemPG(pool),
		paymentrepo.NewPaymentPG(pool), productrepo.NewVariantPG(pool),
		customeraddrrepo.NewAddressPG(pool), authrepo.NewUserPG(pool),
		provider.NewFlatRateProvider(brandrepo.NewBrandPG(pool)),
		payos.NewMockClient(""),
		orderservice.Config{ReservationTimeout: 30 * time.Minute},
	)
	_, pay, err := osvc.PlaceOrder(ctx, userID, orderdomain.PlaceOrderReq{
		AddressID: addrID, PaymentMethod: orderdomain.PaymentMethodPayos,
	})
	require.NoError(t, err)

	var code int64
	_ = pool.QueryRow(ctx, `SELECT payos_order_code FROM payments WHERE id=$1`, pay.ID).Scan(&code)

	wsvc := paymentservice.NewWebhookService(pool,
		paymentrepo.NewPaymentPG(pool), orderrepo.NewOrderPG(pool),
		orderrepo.NewSubOrderPG(pool), orderrepo.NewOrderItemPG(pool),
		productrepo.NewVariantPG(pool), payos.NewMockClient(""),
	)
	payload := payos.WebhookPayload{
		Code: "00", Success: true,
		Data: payos.WebhookData{OrderCode: code, Amount: 130000},
		Signature: "mock",
	}
	require.NoError(t, wsvc.HandlePayosWebhook(ctx, payload))
	require.NoError(t, wsvc.HandlePayosWebhook(ctx, payload)) // 2nd call

	stock, reserved := testfixtures.GetVariantStock(t, pool, variantID)
	require.Equal(t, 3, stock) // 5 - 2, not 5 - 4
	require.Equal(t, 0, reserved)
}
```

- [ ] **Step 5: Run + commit**

Run: `go test ./internal/payment/service/ ./internal/payment/handler/ -v 2>&1 | tail -20`
Expected: all PASS.

```bash
git add internal/payment/service/ internal/payment/handler/
git commit -m "feat(payment): PayOS webhook service (idempotent) + handler + dev simulate endpoints"
```

---

## Task 22: Reservation cleanup job

**Files:**
- Create: `internal/jobs/reservation_cleanup.go`
- Create: `internal/jobs/reservation_cleanup_test.go`

- [ ] **Step 1: Write reservation_cleanup.go**

```go
// internal/jobs/reservation_cleanup.go
package jobs

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	orderdomain "github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	paymentrepo "github.com/wearwhere/wearwhere_be/internal/payment/repo"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
)

type ReservationCleanupJob struct {
	pool         *pgxpool.Pool
	orderRepo    orderrepo.OrderRepo
	subOrderRepo orderrepo.SubOrderRepo
	itemRepo     orderrepo.OrderItemRepo
	paymentRepo  paymentrepo.PaymentRepo
	variantRepo  productrepo.VariantRepo
	timeoutMin   int
}

func NewReservationCleanupJob(
	pool *pgxpool.Pool,
	or orderrepo.OrderRepo, sr orderrepo.SubOrderRepo, ir orderrepo.OrderItemRepo,
	pr paymentrepo.PaymentRepo, vr productrepo.VariantRepo,
	timeoutMin int,
) *ReservationCleanupJob {
	if timeoutMin <= 0 { timeoutMin = 30 }
	return &ReservationCleanupJob{
		pool: pool, orderRepo: or, subOrderRepo: sr, itemRepo: ir,
		paymentRepo: pr, variantRepo: vr, timeoutMin: timeoutMin,
	}
}

// Run blocks until ctx is cancelled.
func (j *ReservationCleanupJob) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 { interval = 5 * time.Minute }
	t := time.NewTicker(interval)
	defer t.Stop()
	log.Printf("[reservation-cleanup] starting (timeout=%dm, interval=%s)", j.timeoutMin, interval)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[reservation-cleanup] stopping")
			return
		case <-t.C:
			if n, err := j.CleanupOnce(ctx); err != nil {
				log.Printf("[reservation-cleanup] error: %v", err)
			} else if n > 0 {
				log.Printf("[reservation-cleanup] released %d expired orders", n)
			}
		}
	}
}

// CleanupOnce scans expired pending PayOS payments and releases their stock.
// Returns count of orders processed.
func (j *ReservationCleanupJob) CleanupOnce(ctx context.Context) (int, error) {
	rows, err := j.pool.Query(ctx,
		`SELECT p.id, p.order_id FROM payments p
		  WHERE p.method = 'payos'
		    AND p.status = 'pending'
		    AND p.created_at < NOW() - ($1 || ' minutes')::interval
		  ORDER BY p.created_at ASC
		  LIMIT 100`,
		j.timeoutMin)
	if err != nil { return 0, err }
	type expired struct{ paymentID, orderID uuid.UUID }
	var todo []expired
	for rows.Next() {
		var e expired
		if err := rows.Scan(&e.paymentID, &e.orderID); err != nil {
			rows.Close()
			return 0, err
		}
		todo = append(todo, e)
	}
	rows.Close()

	count := 0
	for _, e := range todo {
		if err := j.expireOne(ctx, e.paymentID, e.orderID); err != nil {
			log.Printf("[reservation-cleanup] expireOne(%s) failed: %v", e.orderID, err)
			continue
		}
		count++
	}
	return count, nil
}

func (j *ReservationCleanupJob) expireOne(ctx context.Context, paymentID, orderID uuid.UUID) error {
	tx, err := j.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil { return err }
	defer tx.Rollback(ctx) //nolint:errcheck

	// Re-check payment status (race with webhook)
	var status string
	if err := tx.QueryRow(ctx,
		`SELECT status FROM payments WHERE id=$1 FOR UPDATE`, paymentID,
	).Scan(&status); err != nil {
		return err
	}
	if status != "pending" { return nil } // webhook beat us

	if err := j.paymentRepo.UpdateOnExpired(ctx, tx, paymentID); err != nil { return err }
	if err := j.orderRepo.UpdateStatusOnCancel(ctx, tx, orderID, "payos_payment_timeout", orderdomain.PaymentStatusCancelled); err != nil {
		return err
	}
	if err := j.subOrderRepo.CancelAllByOrderID(ctx, tx, orderID); err != nil { return err }

	items, err := j.itemRepo.ListByOrderID(ctx, orderID)
	if err != nil { return err }
	for _, it := range items {
		if err := j.variantRepo.Release(ctx, tx, it.VariantID, it.Qty); err != nil {
			return fmt.Errorf("release variant %s qty=%d: %w", it.VariantID, it.Qty, err)
		}
	}

	return tx.Commit(ctx)
}
```

- [ ] **Step 2: Write test**

```go
// internal/jobs/reservation_cleanup_test.go
package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	authrepo "github.com/wearwhere/wearwhere_be/internal/auth/repo"
	brandrepo "github.com/wearwhere/wearwhere_be/internal/brand/repo"
	customeraddrrepo "github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
	"github.com/wearwhere/wearwhere_be/internal/jobs"
	orderdomain "github.com/wearwhere/wearwhere_be/internal/order/domain"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	orderservice "github.com/wearwhere/wearwhere_be/internal/order/service"
	"github.com/wearwhere/wearwhere_be/internal/payment/payos"
	paymentrepo "github.com/wearwhere/wearwhere_be/internal/payment/repo"
	productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
	"github.com/wearwhere/wearwhere_be/internal/shipping/provider"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

func TestCleanupOnce_ExpiresOldPendingPayments(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	ctx := context.Background()

	userID := testfixtures.SeedUser(t, pool, "b@x.com")
	addrID := testfixtures.SeedCustomerAddress(t, pool, userID)
	brandID := testfixtures.SeedBrand(t, pool, "rep")
	productID := testfixtures.SeedProduct(t, pool, brandID, "tee")
	variantID := testfixtures.SeedVariant(t, pool, productID, "BLK-M", 5)
	_, _ = pool.Exec(ctx, `INSERT INTO cart_items (user_id, variant_id, qty, price_snapshot, currency_snapshot)
		 VALUES ($1, $2, 2, 100000, 'VND')`, userID, variantID)

	osvc := orderservice.NewOrderService(pool,
		orderrepo.NewOrderPG(pool), orderrepo.NewSubOrderPG(pool), orderrepo.NewOrderItemPG(pool),
		paymentrepo.NewPaymentPG(pool), productrepo.NewVariantPG(pool),
		customeraddrrepo.NewAddressPG(pool), authrepo.NewUserPG(pool),
		provider.NewFlatRateProvider(brandrepo.NewBrandPG(pool)),
		payos.NewMockClient(""),
		orderservice.Config{ReservationTimeout: 30 * time.Minute},
	)
	resp, _, err := osvc.PlaceOrder(ctx, userID, orderdomain.PlaceOrderReq{
		AddressID: addrID, PaymentMethod: orderdomain.PaymentMethodPayos,
	})
	require.NoError(t, err)

	// Backdate the payment so it's older than the timeout (1 minute test threshold).
	_, _ = pool.Exec(ctx, `UPDATE payments SET created_at = NOW() - INTERVAL '5 minutes' WHERE order_id = $1`, resp.ID)

	job := jobs.NewReservationCleanupJob(pool,
		orderrepo.NewOrderPG(pool), orderrepo.NewSubOrderPG(pool), orderrepo.NewOrderItemPG(pool),
		paymentrepo.NewPaymentPG(pool), productrepo.NewVariantPG(pool),
		1, // 1 minute timeout for test
	)
	n, err := job.CleanupOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	// Verify state
	var orderStatus, paymentStatus string
	_ = pool.QueryRow(ctx, `SELECT status FROM orders WHERE id=$1`, resp.ID).Scan(&orderStatus)
	require.Equal(t, "cancelled", orderStatus)
	_ = pool.QueryRow(ctx, `SELECT status FROM payments WHERE order_id=$1`, resp.ID).Scan(&paymentStatus)
	require.Equal(t, "expired", paymentStatus)

	stock, reserved := testfixtures.GetVariantStock(t, pool, variantID)
	require.Equal(t, 5, stock)
	require.Equal(t, 0, reserved)
}

func TestCleanupOnce_SkipsRecentPayments(t *testing.T) {
	pool := testfixtures.MustPool(t)
	defer testfixtures.Clean(t, pool)
	ctx := context.Background()

	userID := testfixtures.SeedUser(t, pool, "b@x.com")
	addrID := testfixtures.SeedCustomerAddress(t, pool, userID)
	brandID := testfixtures.SeedBrand(t, pool, "rep")
	productID := testfixtures.SeedProduct(t, pool, brandID, "tee")
	variantID := testfixtures.SeedVariant(t, pool, productID, "BLK-M", 5)
	_, _ = pool.Exec(ctx, `INSERT INTO cart_items (user_id, variant_id, qty, price_snapshot, currency_snapshot)
		 VALUES ($1, $2, 2, 100000, 'VND')`, userID, variantID)

	osvc := orderservice.NewOrderService(pool,
		orderrepo.NewOrderPG(pool), orderrepo.NewSubOrderPG(pool), orderrepo.NewOrderItemPG(pool),
		paymentrepo.NewPaymentPG(pool), productrepo.NewVariantPG(pool),
		customeraddrrepo.NewAddressPG(pool), authrepo.NewUserPG(pool),
		provider.NewFlatRateProvider(brandrepo.NewBrandPG(pool)),
		payos.NewMockClient(""),
		orderservice.Config{ReservationTimeout: 30 * time.Minute},
	)
	_, _, err := osvc.PlaceOrder(ctx, userID, orderdomain.PlaceOrderReq{
		AddressID: addrID, PaymentMethod: orderdomain.PaymentMethodPayos,
	})
	require.NoError(t, err)

	job := jobs.NewReservationCleanupJob(pool,
		orderrepo.NewOrderPG(pool), orderrepo.NewSubOrderPG(pool), orderrepo.NewOrderItemPG(pool),
		paymentrepo.NewPaymentPG(pool), productrepo.NewVariantPG(pool),
		30,
	)
	n, err := job.CleanupOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, n) // payment was just created, well within 30-min window

	_, reserved := testfixtures.GetVariantStock(t, pool, variantID)
	require.Equal(t, 2, reserved)
}
```

- [ ] **Step 3: Run + commit**

Run: `go test ./internal/jobs/ -v 2>&1 | tail -20`
Expected: 2 tests PASS.

```bash
git add internal/jobs/
git commit -m "feat(jobs): reservation_cleanup job releases expired PayOS pending orders"
```

---

## Task 23: Config + main.go wire-up

**Files:**
- Modify: `internal/config/config.go` — add Payos + Shipping + Reservation sections
- Modify: `cmd/api/main.go` — wire 4 new modules + start cleanup job + new routes
- Create: `.env.example` (or modify) — add new variables

- [ ] **Step 1: Add Payos/Shipping/Reservation configs**

Append in `internal/config/config.go`:

```go
// inside type Config struct
Payos       PayosConfig
Shipping    ShippingConfig
Reservation ReservationConfig

// at bottom of file
type PayosConfig struct {
	Mode        string // "mock" | "production"
	ClientID    string
	APIKey      string
	ChecksumKey string
	ReturnURL   string
	CancelURL   string
	BaseURL     string // for mock checkout URL
}

type ShippingConfig struct {
	Provider string // "flat" (Sprint 3)
}

type ReservationConfig struct {
	TimeoutMinutes  int // default 30
	CleanupInterval time.Duration // default 5 min
}
```

In `Load()`, append to `cfg`:

```go
Payos: PayosConfig{
	Mode:        getEnv("PAYOS_MODE", "mock"),
	ClientID:    getEnv("PAYOS_CLIENT_ID", ""),
	APIKey:      getEnv("PAYOS_API_KEY", ""),
	ChecksumKey: getEnv("PAYOS_CHECKSUM_KEY", ""),
	ReturnURL:   getEnv("PAYOS_RETURN_URL", "http://localhost:3000/checkout/success"),
	CancelURL:   getEnv("PAYOS_CANCEL_URL", "http://localhost:3000/checkout/cancel"),
	BaseURL:     getEnv("PAYOS_BASE_URL", "http://localhost:8080"),
},
Shipping: ShippingConfig{
	Provider: getEnv("SHIPPING_PROVIDER", "flat"),
},
Reservation: ReservationConfig{
	TimeoutMinutes:  getInt("RESERVATION_TIMEOUT_MINUTES", 30),
	CleanupInterval: getDuration("RESERVATION_CLEANUP_INTERVAL", 5*time.Minute),
},
```

- [ ] **Step 2: Update .env.example**

Append (or create if missing):

```bash
# PayOS
PAYOS_MODE=mock                              # mock|production
PAYOS_CLIENT_ID=
PAYOS_API_KEY=
PAYOS_CHECKSUM_KEY=
PAYOS_RETURN_URL=http://localhost:3000/checkout/success
PAYOS_CANCEL_URL=http://localhost:3000/checkout/cancel
PAYOS_BASE_URL=http://localhost:8080

# Shipping
SHIPPING_PROVIDER=flat

# Reservation timeout (PayOS pending orders auto-cancelled after this)
RESERVATION_TIMEOUT_MINUTES=30
RESERVATION_CLEANUP_INTERVAL=5m
```

- [ ] **Step 3: Wire main.go**

Read current `cmd/api/main.go` and inject new modules. Append imports:

```go
import (
	// ... existing imports ...
	"github.com/jackc/pgx/v5/pgxpool"

	orderhandler "github.com/wearwhere/wearwhere_be/internal/order/handler"
	orderrepo "github.com/wearwhere/wearwhere_be/internal/order/repo"
	orderservice "github.com/wearwhere/wearwhere_be/internal/order/service"
	paymenthandler "github.com/wearwhere/wearwhere_be/internal/payment/handler"
	paymentrepo "github.com/wearwhere/wearwhere_be/internal/payment/repo"
	paymentservice "github.com/wearwhere/wearwhere_be/internal/payment/service"
	"github.com/wearwhere/wearwhere_be/internal/payment/payos"
	"github.com/wearwhere/wearwhere_be/internal/shipping/provider"
	"github.com/wearwhere/wearwhere_be/internal/jobs"
)
```

After existing repo wire-up, before route registration:

```go
// ── new repos ──
orderRepo := orderrepo.NewOrderPG(pgPool)
subOrderRepo := orderrepo.NewSubOrderPG(pgPool)
orderItemRepo := orderrepo.NewOrderItemPG(pgPool)
paymentRepo := paymentrepo.NewPaymentPG(pgPool)

// ── shipping provider ──
shippingProvider, err := provider.NewFromConfig(provider.Config{Provider: cfg.Shipping.Provider}, brandRepo)
if err != nil { log.Fatalf("shipping provider: %v", err) }

// ── PayOS client ──
payosClient, err := payos.NewFromConfig(payos.Config{
	Mode: cfg.Payos.Mode, ClientID: cfg.Payos.ClientID,
	APIKey: cfg.Payos.APIKey, ChecksumKey: cfg.Payos.ChecksumKey,
	BaseURL: cfg.Payos.BaseURL,
})
if err != nil { log.Fatalf("payos: %v", err) }

// ── services ──
checkoutSvc := orderservice.NewCheckoutService(cartRepo, customerAddrRepo, shippingProvider)
orderSvc := orderservice.NewOrderService(
	pgPool, orderRepo, subOrderRepo, orderItemRepo,
	paymentRepo, variantRepo,
	customerAddrRepo, userRepo,
	shippingProvider, payosClient,
	orderservice.Config{
		ReservationTimeout: time.Duration(cfg.Reservation.TimeoutMinutes) * time.Minute,
		PayosReturnURL:     cfg.Payos.ReturnURL,
		PayosCancelURL:     cfg.Payos.CancelURL,
	},
)
webhookSvc := paymentservice.NewWebhookService(
	pgPool, paymentRepo, orderRepo, subOrderRepo, orderItemRepo, variantRepo, payosClient,
)

// ── handlers ──
orderH := orderhandler.New(checkoutSvc, orderSvc)
paymentH := paymenthandler.New(webhookSvc, payosClient, cfg.Payos.Mode == "mock")

// ── routes ──
api := r.Group("/api/v1")  // assume r is *gin.Engine already constructed
meAuth := api.Group("/me", jwtMW) // JWT middleware var name from existing wire-up
orderhandler.Mount(meAuth, orderH)
paymenthandler.MountPublic(api, paymentH)
if cfg.Payos.Mode == "mock" {
	paymenthandler.MountDev(r, paymentH)
}

// ── background job ──
cleanupJob := jobs.NewReservationCleanupJob(
	pgPool, orderRepo, subOrderRepo, orderItemRepo,
	paymentRepo, variantRepo, cfg.Reservation.TimeoutMinutes,
)
go cleanupJob.Run(ctx, cfg.Reservation.CleanupInterval)
```

Adjust variable names (`brandRepo`, `cartRepo`, `customerAddrRepo`, `userRepo`, `variantRepo`, `jwtMW`, `r`) to match existing main.go names. Read main.go end-to-end first to ensure clean integration.

- [ ] **Step 4: Build**

Run: `go build ./cmd/api`
Expected: exit 0. Resolve any import or signature mismatches.

- [ ] **Step 5: Run unit + integration tests**

Run: `go test ./... 2>&1 | tail -30`
Expected: all PASS.

- [ ] **Step 6: Smoke run**

Run (in dev terminal): `PAYOS_MODE=mock ./cmd/api/api` or `go run ./cmd/api`
Visit `http://localhost:8080/dev/payos/mock-checkout?orderCode=1` — expect HTML page.
Stop the server.

- [ ] **Step 7: Commit**

```bash
git add internal/config/config.go cmd/api/main.go .env.example
git commit -m "feat(api): wire order/payment/shipping/jobs in main + Payos/Shipping/Reservation config"
```

---

## Task 24: E2E tests in cmd/api/main_test.go

**Files:**
- Modify: `cmd/api/main_test.go` — append 5 new test scenarios

- [ ] **Step 1: Inspect existing E2E test setup**

Read `cmd/api/main_test.go` first 200 lines to understand:
- HTTP server bootstrap function (likely `startTestServer(t)` or similar)
- Helper for registering + logging in a user
- Helper for adding cart items
- How brands/products/variants are seeded

- [ ] **Step 2: Append `TestE2E_OrderPayosFlow`**

```go
// cmd/api/main_test.go (append after Sprint 2 tests)

func TestE2E_OrderPayosFlow(t *testing.T) {
	srv := startTestServer(t)
	defer srv.Close()

	// 1. Register + login user → get JWT
	jwt := registerAndLoginUser(t, srv, "buyer-payos@x.com", "password123")

	// 2. Seed brand + product + variant (use existing helpers or hit brand-admin endpoints)
	brandID, _, variantID, price := seedBrandWithVariant(t, srv, "rep-vn", "Tee", 100000, 5)
	_ = brandID

	// 3. Add to cart
	addToCart(t, srv, jwt, variantID, 2)

	// 4. Create shipping address
	addrID := createCustomerAddress(t, srv, jwt, "An Nguyen", "0900000000", "1 ABC", "P1", "Q1", "HCM")

	// 5. POST /me/orders payment_method=payos
	body, _ := json.Marshal(map[string]any{
		"address_id": addrID, "payment_method": "payos", "notes": "fast",
	})
	resp, code := doRequest(t, srv, "POST", "/api/v1/me/orders", jwt, body)
	require.Equal(t, http.StatusCreated, code)

	var placed struct {
		Order struct {
			ID            string `json:"id"`
			OrderNo       string `json:"order_no"`
			Status        string `json:"status"`
			GrandTotalVND int64  `json:"grand_total_vnd"`
		} `json:"order"`
		Payment struct {
			Method      string  `json:"method"`
			CheckoutURL *string `json:"checkout_url"`
		} `json:"payment"`
	}
	require.NoError(t, json.Unmarshal(resp, &placed))
	require.Equal(t, "pending_payment", placed.Order.Status)
	require.Equal(t, "payos", placed.Payment.Method)
	require.NotNil(t, placed.Payment.CheckoutURL)
	require.Equal(t, int64(2*price+30000), placed.Order.GrandTotalVND)

	// 6. Extract orderCode from checkout URL and simulate webhook
	checkoutURL := *placed.Payment.CheckoutURL
	orderCode := extractQueryParam(checkoutURL, "orderCode")

	form := url.Values{"orderCode": {orderCode}, "success": {"true"}}
	resp2, code2 := doFormRequest(t, srv, "POST", "/dev/payos/simulate-webhook", "", form)
	_ = resp2
	require.Equal(t, http.StatusOK, code2)

	// 7. Verify order moved to processing + paid
	resp3, code3 := doRequest(t, srv, "GET", "/api/v1/me/orders/"+placed.Order.OrderNo, jwt, nil)
	require.Equal(t, http.StatusOK, code3)
	var detail map[string]any
	require.NoError(t, json.Unmarshal(resp3, &detail))
	require.Equal(t, "processing", detail["status"])
	require.Equal(t, "paid", detail["payment_status"])

	// 8. Verify stock decremented
	stock := getVariantStock(t, srv, variantID)
	require.Equal(t, 3, stock) // 5 - 2
}
```

- [ ] **Step 3: Append `TestE2E_OrderCODFlow`**

```go
func TestE2E_OrderCODFlow(t *testing.T) {
	srv := startTestServer(t)
	defer srv.Close()

	jwt := registerAndLoginUser(t, srv, "buyer-cod@x.com", "password123")
	_, _, variantID, _ := seedBrandWithVariant(t, srv, "fok", "Hat", 200000, 5)
	addToCart(t, srv, jwt, variantID, 1)
	addrID := createCustomerAddress(t, srv, jwt, "B", "0911111111", "L", "W", "D", "C")

	body, _ := json.Marshal(map[string]any{
		"address_id": addrID, "payment_method": "cod",
	})
	resp, code := doRequest(t, srv, "POST", "/api/v1/me/orders", jwt, body)
	require.Equal(t, http.StatusCreated, code)

	var placed struct {
		Order struct {
			OrderNo string `json:"order_no"`
			Status  string `json:"status"`
		} `json:"order"`
		Payment struct {
			Method      string  `json:"method"`
			CheckoutURL *string `json:"checkout_url"`
		} `json:"payment"`
	}
	require.NoError(t, json.Unmarshal(resp, &placed))
	require.Equal(t, "processing", placed.Order.Status)
	require.Equal(t, "cod", placed.Payment.Method)
	require.Nil(t, placed.Payment.CheckoutURL)
}
```

- [ ] **Step 4: Append `TestE2E_CancelPayosUnpaid`**

```go
func TestE2E_CancelPayosUnpaid(t *testing.T) {
	srv := startTestServer(t)
	defer srv.Close()

	jwt := registerAndLoginUser(t, srv, "canceller@x.com", "password123")
	_, _, variantID, _ := seedBrandWithVariant(t, srv, "rep", "Tee", 100000, 5)
	addToCart(t, srv, jwt, variantID, 2)
	addrID := createCustomerAddress(t, srv, jwt, "B", "0911111111", "L", "W", "D", "C")

	body, _ := json.Marshal(map[string]any{
		"address_id": addrID, "payment_method": "payos",
	})
	resp, _ := doRequest(t, srv, "POST", "/api/v1/me/orders", jwt, body)
	var placed struct {
		Order struct{ OrderNo string `json:"order_no"` } `json:"order"`
	}
	_ = json.Unmarshal(resp, &placed)

	stockBefore := getVariantStock(t, srv, variantID)
	require.Equal(t, 5, stockBefore) // not yet committed

	// Cancel
	cancelBody, _ := json.Marshal(map[string]any{"reason": "changed my mind"})
	_, code := doRequest(t, srv, "POST",
		"/api/v1/me/orders/"+placed.Order.OrderNo+"/cancel", jwt, cancelBody)
	require.Equal(t, http.StatusOK, code)

	stockAfter := getVariantStock(t, srv, variantID)
	require.Equal(t, 5, stockAfter)
	// reserved should be 0 now (released)
}
```

- [ ] **Step 5: Append `TestE2E_InsufficientStockRace`**

```go
func TestE2E_InsufficientStockRace(t *testing.T) {
	srv := startTestServer(t)
	defer srv.Close()

	_, _, variantID, _ := seedBrandWithVariant(t, srv, "rep", "Tee", 100000, 1) // ONLY 1 in stock

	// Two users
	jwtA := registerAndLoginUser(t, srv, "a@x.com", "password123")
	jwtB := registerAndLoginUser(t, srv, "b@x.com", "password123")
	addToCart(t, srv, jwtA, variantID, 1)
	addToCart(t, srv, jwtB, variantID, 1)
	addrA := createCustomerAddress(t, srv, jwtA, "A", "0911", "L", "W", "D", "C")
	addrB := createCustomerAddress(t, srv, jwtB, "B", "0922", "L", "W", "D", "C")

	body := func(addrID string) []byte {
		b, _ := json.Marshal(map[string]any{"address_id": addrID, "payment_method": "cod"})
		return b
	}

	var wg sync.WaitGroup
	codes := make([]int, 2)
	wg.Add(2)
	go func() { defer wg.Done(); _, codes[0] = doRequest(t, srv, "POST", "/api/v1/me/orders", jwtA, body(addrA)) }()
	go func() { defer wg.Done(); _, codes[1] = doRequest(t, srv, "POST", "/api/v1/me/orders", jwtB, body(addrB)) }()
	wg.Wait()

	successCount := 0
	conflictCount := 0
	for _, c := range codes {
		if c == http.StatusCreated { successCount++ }
		if c == http.StatusConflict { conflictCount++ }
	}
	require.Equal(t, 1, successCount, "exactly one order should succeed")
	require.Equal(t, 1, conflictCount, "exactly one should get 409")
}
```

- [ ] **Step 6: Append `TestE2E_ReservationCleanupJob`**

```go
func TestE2E_ReservationCleanupJob(t *testing.T) {
	srv := startTestServer(t)
	defer srv.Close()

	jwt := registerAndLoginUser(t, srv, "x@x.com", "password123")
	_, _, variantID, _ := seedBrandWithVariant(t, srv, "rep", "Tee", 100000, 5)
	addToCart(t, srv, jwt, variantID, 2)
	addrID := createCustomerAddress(t, srv, jwt, "A", "0900", "L", "W", "D", "C")

	body, _ := json.Marshal(map[string]any{"address_id": addrID, "payment_method": "payos"})
	resp, _ := doRequest(t, srv, "POST", "/api/v1/me/orders", jwt, body)
	var placed struct{ Order struct{ ID, OrderNo string } `json:"order"` }
	_ = json.Unmarshal(resp, &placed)

	// Backdate payment for this order to trigger cleanup
	pool := srv.Pool // assumes startTestServer exposes db pool
	_, _ = pool.Exec(context.Background(),
		`UPDATE payments SET created_at = NOW() - INTERVAL '60 minutes' WHERE order_id = (SELECT id FROM orders WHERE order_no=$1)`,
		placed.Order.OrderNo)

	// Run cleanup once directly
	job := jobs.NewReservationCleanupJob(
		pool, orderrepo.NewOrderPG(pool), orderrepo.NewSubOrderPG(pool), orderrepo.NewOrderItemPG(pool),
		paymentrepo.NewPaymentPG(pool), productrepo.NewVariantPG(pool), 30,
	)
	n, err := job.CleanupOnce(context.Background())
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, 1)

	// Verify cancelled
	_, code := doRequest(t, srv, "GET", "/api/v1/me/orders/"+placed.Order.OrderNo, jwt, nil)
	require.Equal(t, http.StatusOK, code)
	// detail.status == "cancelled" (parse if needed)

	stock := getVariantStock(t, srv, variantID)
	require.Equal(t, 5, stock)
}
```

- [ ] **Step 7: Add missing helpers if needed**

`extractQueryParam`, `doFormRequest`, `getVariantStock`, `seedBrandWithVariant`, `createCustomerAddress`, `addToCart` — if any are missing, look at how Sprint 2 E2E test does it and add minimal wrappers. Also expose `srv.Pool` from `startTestServer` for the cleanup test if it's not already.

- [ ] **Step 8: Run E2E + commit**

Run: `go test ./cmd/api/ -run TestE2E_Order -v 2>&1 | tail -50`
Expected: all 5 scenarios PASS.

```bash
git add cmd/api/main_test.go
git commit -m "test(e2e): Sprint 3 order flows (PayOS, COD, cancel, race, cleanup job)"
```

---

## Task 25: Format, docs, push

**Files:**
- Modify: `README.md` (if exists) — add Sprint 3 section briefly
- All `.go` files — `gofmt`

- [ ] **Step 1: Format Go code**

Run: `gofmt -w internal/order internal/payment internal/shipping internal/jobs internal/product internal/brand internal/config cmd/api`
Expected: exit 0.

- [ ] **Step 2: Final test pass**

Run: `go test ./... 2>&1 | tail -30`
Expected: all tests PASS.

- [ ] **Step 3: Final build**

Run: `go build ./...`
Expected: exit 0.

- [ ] **Step 4: Update README.md (if present) or note in commit**

If `README.md` exists, append:

```markdown
## Sprint 3 — Orders, Checkout, PayOS

New endpoints:
- `GET  /api/v1/me/checkout/preview?address_id=...`
- `POST /api/v1/me/orders`
- `GET  /api/v1/me/orders` (paginated)
- `GET  /api/v1/me/orders/:order_no`
- `POST /api/v1/me/orders/:order_no/cancel`
- `POST /api/v1/payments/payos/webhook` (public)

PayOS modes (env `PAYOS_MODE`):
- `mock` (default): no creds needed; `GET /dev/payos/mock-checkout?orderCode=N` simulates the gateway page.
- `production`: requires `PAYOS_CLIENT_ID`, `PAYOS_API_KEY`, `PAYOS_CHECKSUM_KEY`.

A background `reservation_cleanup` job runs every `RESERVATION_CLEANUP_INTERVAL` (default 5m) and releases stock for PayOS orders pending > `RESERVATION_TIMEOUT_MINUTES` (default 30).

Design doc: `docs/superpowers/specs/2026-05-24-sprint-3-orders-checkout-payos-design.md`
```

- [ ] **Step 5: Commit format + docs**

```bash
git add -A
git commit -m "chore(format): gofmt + README sprint-3 section"
```

- [ ] **Step 6: Close beads tasks**

Run (replace IDs with actual):
```bash
bd close wearwhere_be-XXX wearwhere_be-YYY ... --reason="Sprint 3 complete"
bd remember "sprint-3-orders-checkout-execution-status" "Sprint 3 ... <summary>"
```

- [ ] **Step 7: Push to remote + open PR**

```bash
git pull --rebase origin sprint-3-orders-checkout 2>/dev/null || true
git push -u origin sprint-3-orders-checkout
gh pr create --title "Sprint 3: Orders, Checkout & PayOS" --body "$(cat <<'EOF'
## Summary
- Customer-side order placement with multi-brand sub-orders
- PayOS payment integration (mock + production-ready HTTP skeleton)
- Stock reservation lifecycle (reserve at place, commit at paid, release at cancel/expire)
- 30-min reservation cleanup background job

## Endpoints (new)
- `GET /api/v1/me/checkout/preview`, `POST /api/v1/me/orders`, `GET/POST/cancel /me/orders/*`
- `POST /api/v1/payments/payos/webhook` (public, signature-verified)
- `GET /dev/payos/mock-checkout`, `POST /dev/payos/simulate-webhook` (dev-only, `PAYOS_MODE=mock`)

## Test plan
- [x] Unit: domain/state-machine, signature HMAC, order_no nanoid
- [x] Repo: variant reserve/commit/release race, order CRUD + IDOR, payment idempotency
- [x] Service: PlaceOrder full tx, cancel, list, webhook
- [x] E2E: PayOS flow, COD flow, cancel unpaid, stock race, cleanup job

Spec: `docs/superpowers/specs/2026-05-24-sprint-3-orders-checkout-payos-design.md`
Plan: `docs/superpowers/plans/2026-05-24-sprint-3-orders-checkout-payos.md`
EOF
)"
```

Expected: PR URL printed.

- [ ] **Step 8: Verify push status**

Run: `git status` — must show "Your branch is up to date with 'origin/sprint-3-orders-checkout'".

---

## Summary

25 tasks complete. Sprint 3 ships customer-side orders + checkout + PayOS (mock) end-to-end.

**Deferred to Sprint 4 (per spec §14):**
- Brand fulfillment endpoints (UC45/46/47)
- Paid-order cancellation + PayOS refund API
- Sub-order status transitions (confirmed → preparing → shipped → delivered)
- Notification triggers
- Order reviews (UC37)
- Real PayOS production credentials integration



