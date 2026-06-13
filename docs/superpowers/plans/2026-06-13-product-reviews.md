# Product Reviews Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build customer product reviews (SRS UC37/UC38): verified purchasers post a 1–5 star rating + text + optional size/fit; anyone reads a product's reviews; the product's average rating and review count are denormalized and shown.

**Architecture:** New module `internal/review/` (domain → repo → service → handler). The repo owns transactions: each write (create/update/soft-delete) runs the row change + an aggregate recompute on `products` in one transaction. Service holds the repo interface (no pool) so it stays unit-testable; validation, verified-purchase gating, and ownership live in the service. Endpoints: public read + customer-authed write.

**Tech Stack:** Go, Gin, pgx/v5, PostgreSQL, golang-migrate. Patterns copied from `internal/wishlist`, `internal/order` (transactions), `internal/store` (repo/integration-test layout).

**Spec:** `docs/superpowers/specs/2026-06-13-product-reviews-design.md`

**Branch:** `feature/product-reviews` (already created off `main`).

**No photos this iteration** (deferred). Reviews are text-only.

---

## File Structure

**New module `internal/review/`:**
- `domain/review.go` — `Review` entity, `Fit` constants
- `domain/dto.go` — request/response DTOs + `ListReviewsQuery` + converters
- `domain/errors.go` — sentinel `*httpx.AppError`
- `repo/repo.go` — `Repo` interface, `DBTX`, `ErrDuplicate`, `ErrNotFound`
- `repo/review_pg.go` — Postgres impl (pool-owned, transactional writes + recompute)
- `repo/review_pg_test.go` — integration tests (`//go:build integration`)
- `service/service.go` — validation, verified-purchase gate, ownership, orchestration
- `service/service_test.go` — unit tests with a fake repo
- `handler/handler.go` — Gin handlers
- `handler/routes.go` — `MountReviewsPublic` + `MountReviewsAuthed`
- `handler/handler_test.go` — handler tests

**Modified:**
- `db/migrations/000034_create_product_reviews.{up,down}.sql`
- `db/migrations/000035_add_review_aggregates_to_products.{up,down}.sql`
- `cmd/api/main.go`, `cmd/api/main_test.go` — wire + mount
- `internal/testfixtures/fixtures.go` — add a delivered-order-line seed helper
- `internal/product/...` — surface `avg_rating`/`review_count` on product detail (final task)

> **Migration numbers:** `main` is at `000033` (merged store-discovery). Next free: `000034`, `000035`. The parked `ai-stylist-chatbot` branch also uses `000033/000034` — same eventual-integration renumbering already flagged.

---

## Task 1: Migrations

**Files:**
- Create: `db/migrations/000034_create_product_reviews.up.sql`
- Create: `db/migrations/000034_create_product_reviews.down.sql`
- Create: `db/migrations/000035_add_review_aggregates_to_products.up.sql`
- Create: `db/migrations/000035_add_review_aggregates_to_products.down.sql`

- [ ] **Step 1: Write `000034_create_product_reviews.up.sql`**

```sql
CREATE TABLE product_reviews (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    product_id  UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating      SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    body        TEXT NOT NULL,
    fit         TEXT CHECK (fit IN ('small','true','large')),
    status      TEXT NOT NULL DEFAULT 'published' CHECK (status IN ('published','hidden')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_reviews_user_product
    ON product_reviews (product_id, user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_reviews_product
    ON product_reviews (product_id) WHERE deleted_at IS NULL AND status = 'published';
```

- [ ] **Step 2: Write `000034_create_product_reviews.down.sql`**

```sql
DROP TABLE IF EXISTS product_reviews;
```

- [ ] **Step 3: Write `000035_add_review_aggregates_to_products.up.sql`**

```sql
ALTER TABLE products
  ADD COLUMN avg_rating   NUMERIC(2,1) NOT NULL DEFAULT 0,
  ADD COLUMN review_count INT          NOT NULL DEFAULT 0;
```

- [ ] **Step 4: Write `000035_add_review_aggregates_to_products.down.sql`**

```sql
ALTER TABLE products
  DROP COLUMN IF EXISTS avg_rating,
  DROP COLUMN IF EXISTS review_count;
```

- [ ] **Step 5: Apply and verify on the test DB**

Run:
```
migrate -path db/migrations -database "postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable" up
docker exec wearwhere_postgres psql -U wearwhere -d wearwhere_test -c "\d product_reviews"
docker exec wearwhere_postgres psql -U wearwhere -d wearwhere_test -c "\d products" | grep -E "avg_rating|review_count"
```
Expected: applies `34` and `35`; `product_reviews` table + both new `products` columns present. Then verify down+up of the last two:
```
migrate -path db/migrations -database "postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable" down 2
migrate -path db/migrations -database "postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable" up
```
Leave the DB at version 35.

- [ ] **Step 6: Commit**

```bash
git add db/migrations/000034_create_product_reviews.up.sql db/migrations/000034_create_product_reviews.down.sql db/migrations/000035_add_review_aggregates_to_products.up.sql db/migrations/000035_add_review_aggregates_to_products.down.sql
git commit -m "feat(db): product_reviews table + review aggregates on products"
```

---

## Task 2: Domain — entity, DTOs, errors

**Files:**
- Create: `internal/review/domain/review.go`
- Create: `internal/review/domain/dto.go`
- Create: `internal/review/domain/errors.go`

- [ ] **Step 1: Write the entity** — `internal/review/domain/review.go`

```go
// Package domain holds product-review entities and DTOs.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Fit is the optional size/fit feedback on a review.
const (
	FitSmall = "small"
	FitTrue  = "true"
	FitLarge = "large"
)

// Review is a customer's review of a product. Every review is a verified
// purchase, so there is no separate "verified" flag.
type Review struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	UserID    uuid.UUID
	Rating    int
	Body      string
	Fit       *string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ReviewView is a Review joined with the reviewer's display name (for listing).
type ReviewView struct {
	Review
	ReviewerName string
}
```

- [ ] **Step 2: Write DTOs + converters** — `internal/review/domain/dto.go`

```go
package domain

import "github.com/google/uuid"

// WriteReviewRequest is the body for create (POST) and update (PATCH).
// Gin binding enforces the SRS rules: rating 1-5, body >= 20 chars,
// fit (optional) one of small|true|large.
type WriteReviewRequest struct {
	Rating int    `json:"rating" binding:"required,min=1,max=5"`
	Body   string `json:"body"   binding:"required,min=20,max=5000"`
	Fit    string `json:"fit"    binding:"omitempty,oneof=small true large"`
}

// ListReviewsQuery is the query string for GET /products/:id/reviews.
type ListReviewsQuery struct {
	Rating int    `form:"rating" binding:"omitempty,min=1,max=5"`
	Fit    string `form:"fit"    binding:"omitempty,oneof=small true large"`
	Sort   string `form:"sort"   binding:"omitempty,oneof=newest rating_high rating_low"`
	Page   int    `form:"page,default=1"   binding:"min=1"`
	Limit  int    `form:"limit,default=20" binding:"min=1,max=50"`
}

type ReviewResponse struct {
	ID           string  `json:"id"`
	Rating       int     `json:"rating"`
	Body         string  `json:"body"`
	Fit          *string `json:"fit,omitempty"`
	Verified     bool    `json:"verified"` // always true
	ReviewerName string  `json:"reviewer_name"`
	CreatedAt    string  `json:"created_at"`
}

type ListReviewsResponse struct {
	Items       []ReviewResponse `json:"items"`
	AvgRating   float64          `json:"avg_rating"`
	ReviewCount int              `json:"review_count"`
	Pagination  Pagination       `json:"pagination"`
}

type Pagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

func NewPagination(page, limit, total int) Pagination {
	tp := 0
	if limit > 0 {
		tp = (total + limit - 1) / limit
	}
	return Pagination{Page: page, Limit: limit, Total: total, TotalPages: tp}
}

func ToReviewResponse(v *ReviewView) ReviewResponse {
	return ReviewResponse{
		ID:           v.ID.String(),
		Rating:       v.Rating,
		Body:         v.Body,
		Fit:          v.Fit,
		Verified:     true,
		ReviewerName: v.ReviewerName,
		CreatedAt:    v.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// fitPtr converts a possibly-empty fit string to a *string for storage.
func FitPtr(fit string) *uuid.UUID { return nil } // placeholder removed below
```

> Remove the stray `FitPtr` line above — it was a typo. The correct helper lives in the service (it converts `""` to `nil`); do not add it to the DTO file. Final `dto.go` ends at the `ToReviewResponse` function.

- [ ] **Step 3: Write errors** — `internal/review/domain/errors.go`

```go
package domain

import (
	"net/http"

	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

func ErrProductNotFound() *httpx.AppError {
	return httpx.NewAppError(http.StatusNotFound, "PRODUCT_NOT_FOUND", "Product not found")
}

func ErrNotVerifiedPurchase() *httpx.AppError {
	return httpx.NewAppError(http.StatusForbidden, "NOT_VERIFIED_PURCHASE", "You can only review a product you have received")
}

func ErrReviewExists() *httpx.AppError {
	return httpx.NewAppError(http.StatusConflict, "REVIEW_EXISTS", "You have already reviewed this product")
}

func ErrReviewNotFound() *httpx.AppError {
	return httpx.NewAppError(http.StatusNotFound, "REVIEW_NOT_FOUND", "Review not found")
}

func ErrForbidden() *httpx.AppError {
	return httpx.NewAppError(http.StatusForbidden, "FORBIDDEN", "You can only modify your own review")
}
```

- [ ] **Step 4: Verify build**

Run: `go build ./internal/review/...`
Expected: builds. (If the stray `FitPtr` line was left in, remove it now — the file must compile with no unused imports.)

- [ ] **Step 5: Commit**

```bash
git add internal/review/domain/
git commit -m "feat(review): domain entity, DTOs, sentinel errors"
```

---

## Task 3: testfixtures helper for delivered order lines

**Files:**
- Modify: `internal/testfixtures/fixtures.go`

The repo integration tests need a delivered purchase (order → sub_order delivered → order_item) to exercise the verified-purchase query. Add a helper. First read `internal/testfixtures/fixtures.go` to match existing helper style and confirm the `orders`/`sub_orders`/`order_items` column names used here.

- [ ] **Step 1: Add the helper to `internal/testfixtures/fixtures.go`**

```go
// SeedDeliveredOrderItem creates a minimal order → sub_order(delivered) → order_item
// chain so the buyer (userID) counts as a verified purchaser of productID.
// variantID must belong to productID. Returns nothing; presence is the signal.
func SeedDeliveredOrderItem(t *testing.T, db DBTX, userID, brandID, productID, variantID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	orderID := uuid.New()
	_, err := db.Exec(ctx,
		`INSERT INTO orders (id, user_id, status, order_no, subtotal_vnd, shipping_fee_vnd, total_vnd, payment_method, payment_status)
		 VALUES ($1,$2,'completed',$3,100000,0,100000,'cod','paid')`,
		orderID, userID, "ORD-"+orderID.String()[:8])
	if err != nil {
		t.Fatalf("seed order: %v", err)
	}
	subID := uuid.New()
	_, err = db.Exec(ctx,
		`INSERT INTO sub_orders (id, order_id, brand_id, status, subtotal_vnd, shipping_fee_vnd, total_vnd, delivered_at)
		 VALUES ($1,$2,$3,'delivered',100000,0,100000,NOW())`,
		subID, orderID, brandID)
	if err != nil {
		t.Fatalf("seed sub_order: %v", err)
	}
	_, err = db.Exec(ctx,
		`INSERT INTO order_items (id, sub_order_id, variant_id, product_id, product_name, variant_label, qty, unit_price_vnd, line_total_vnd)
		 VALUES ($1,$2,$3,$4,'P','V',1,100000,100000)`,
		uuid.New(), subID, variantID, productID)
	if err != nil {
		t.Fatalf("seed order_item: %v", err)
	}
}
```

> **IMPORTANT:** the exact column lists for `orders` and `sub_orders` must match the real schema. Before finalizing, read `db/migrations/000023_create_orders.up.sql` and `db/migrations/000024_create_sub_orders.up.sql` and adjust the INSERT column lists/NOT-NULL values to match (the values above are placeholders for the required NOT NULL columns — fix any mismatch). `order_items` columns are confirmed: `(id, sub_order_id, variant_id, product_id, product_name, variant_label, qty, unit_price_vnd, line_total_vnd)`.

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/testfixtures/`
Expected: builds.

- [ ] **Step 3: Commit**

```bash
git add internal/testfixtures/fixtures.go
git commit -m "test(fixtures): SeedDeliveredOrderItem helper for verified-purchase tests"
```

---

## Task 4: Repo — interface + Postgres (transactional writes + recompute)

**Files:**
- Create: `internal/review/repo/repo.go`
- Create: `internal/review/repo/review_pg.go`
- Test: `internal/review/repo/review_pg_test.go`

- [ ] **Step 1: Write the interface** — `internal/review/repo/repo.go`

```go
// Package repo defines persistence for product reviews.
package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/review/domain"
)

var (
	ErrNotFound  = errors.New("review: not found")
	ErrDuplicate = errors.New("review: already exists for this user+product")
)

// ListFilter is the normalized query for ListByProduct.
type ListFilter struct {
	Rating int    // 0 = no filter
	Fit    string // "" = no filter
	Sort   string // newest | rating_high | rating_low
	Limit  int
	Offset int
}

// Aggregate is the denormalized rating summary stored on products.
type Aggregate struct {
	AvgRating   float64
	ReviewCount int
}

type Repo interface {
	ProductExists(ctx context.Context, productID uuid.UUID) (bool, error)
	HasDeliveredPurchase(ctx context.Context, userID, productID uuid.UUID) (bool, error)
	// Create inserts the review and recomputes the product aggregate in one tx.
	// Returns ErrDuplicate if an active review already exists for (product,user).
	Create(ctx context.Context, r *domain.Review) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Review, error)
	// Update changes rating/body/fit of an existing review and recomputes the
	// aggregate in one tx.
	Update(ctx context.Context, id uuid.UUID, rating int, body string, fit *string) error
	// SoftDelete sets deleted_at and recomputes the aggregate in one tx.
	SoftDelete(ctx context.Context, id uuid.UUID) error
	// ListByProduct returns published, non-deleted reviews + total count.
	ListByProduct(ctx context.Context, productID uuid.UUID, f ListFilter) ([]*domain.ReviewView, int, error)
	// Aggregate reads the denormalized avg_rating/review_count from products.
	Aggregate(ctx context.Context, productID uuid.UUID) (Aggregate, error)
}
```

- [ ] **Step 2: Write the Postgres impl** — `internal/review/repo/review_pg.go`

```go
package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wearwhere/wearwhere_be/internal/review/domain"
)

type ReviewPG struct{ pool *pgxpool.Pool }

func NewReviewPG(pool *pgxpool.Pool) *ReviewPG { return &ReviewPG{pool: pool} }

// querier is the subset shared by *pgxpool.Pool and pgx.Tx.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func recompute(ctx context.Context, q querier, productID uuid.UUID) error {
	_, err := q.Exec(ctx,
		`UPDATE products p SET
		   avg_rating = COALESCE((SELECT AVG(rating) FROM product_reviews
		                          WHERE product_id = p.id AND deleted_at IS NULL AND status='published'), 0),
		   review_count = (SELECT COUNT(*) FROM product_reviews
		                   WHERE product_id = p.id AND deleted_at IS NULL AND status='published')
		 WHERE p.id = $1`, productID)
	return err
}

func (r *ReviewPG) ProductExists(ctx context.Context, productID uuid.UUID) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM products WHERE id=$1 AND deleted_at IS NULL)`, productID).Scan(&ok)
	return ok, err
}

func (r *ReviewPG) HasDeliveredPurchase(ctx context.Context, userID, productID uuid.UUID) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM order_items oi
		   JOIN sub_orders so ON so.id = oi.sub_order_id
		   JOIN orders o      ON o.id  = so.order_id
		   WHERE oi.product_id = $1 AND o.user_id = $2 AND so.status = 'delivered')`,
		productID, userID).Scan(&ok)
	return ok, err
}

func (r *ReviewPG) Create(ctx context.Context, rv *domain.Review) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	err = tx.QueryRow(ctx,
		`INSERT INTO product_reviews (product_id, user_id, rating, body, fit)
		 VALUES ($1,$2,$3,$4,$5)
		 RETURNING id, status, created_at, updated_at`,
		rv.ProductID, rv.UserID, rv.Rating, rv.Body, rv.Fit,
	).Scan(&rv.ID, &rv.Status, &rv.CreatedAt, &rv.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return ErrDuplicate
		}
		return err
	}
	if err := recompute(ctx, tx, rv.ProductID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *ReviewPG) GetByID(ctx context.Context, id uuid.UUID) (*domain.Review, error) {
	var rv domain.Review
	err := r.pool.QueryRow(ctx,
		`SELECT id, product_id, user_id, rating, body, fit, status, created_at, updated_at
		   FROM product_reviews WHERE id=$1 AND deleted_at IS NULL`, id).
		Scan(&rv.ID, &rv.ProductID, &rv.UserID, &rv.Rating, &rv.Body, &rv.Fit, &rv.Status, &rv.CreatedAt, &rv.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &rv, nil
}

func (r *ReviewPG) Update(ctx context.Context, id uuid.UUID, rating int, body string, fit *string) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var productID uuid.UUID
	err = tx.QueryRow(ctx,
		`UPDATE product_reviews SET rating=$2, body=$3, fit=$4, updated_at=NOW()
		 WHERE id=$1 AND deleted_at IS NULL
		 RETURNING product_id`, id, rating, body, fit).Scan(&productID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if err := recompute(ctx, tx, productID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *ReviewPG) SoftDelete(ctx context.Context, id uuid.UUID) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var productID uuid.UUID
	err = tx.QueryRow(ctx,
		`UPDATE product_reviews SET deleted_at=NOW(), updated_at=NOW()
		 WHERE id=$1 AND deleted_at IS NULL
		 RETURNING product_id`, id).Scan(&productID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if err := recompute(ctx, tx, productID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *ReviewPG) ListByProduct(ctx context.Context, productID uuid.UUID, f ListFilter) ([]*domain.ReviewView, int, error) {
	where := "r.product_id = $1 AND r.deleted_at IS NULL AND r.status='published'"
	args := []any{productID}
	if f.Rating != 0 {
		args = append(args, f.Rating)
		where += " AND r.rating = $2"
	}
	if f.Fit != "" {
		args = append(args, f.Fit)
		where += " AND r.fit = $" + itoa(len(args))
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM product_reviews r WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderBy := "r.created_at DESC"
	switch f.Sort {
	case "rating_high":
		orderBy = "r.rating DESC, r.created_at DESC"
	case "rating_low":
		orderBy = "r.rating ASC, r.created_at DESC"
	}

	args = append(args, f.Limit, f.Offset)
	q := `SELECT r.id, r.product_id, r.user_id, r.rating, r.body, r.fit, r.status,
	             r.created_at, r.updated_at, u.name
	        FROM product_reviews r
	        JOIN users u ON u.id = r.user_id
	       WHERE ` + where + `
	       ORDER BY ` + orderBy + `
	       LIMIT $` + itoa(len(args)-1) + ` OFFSET $` + itoa(len(args))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.ReviewView
	for rows.Next() {
		var v domain.ReviewView
		if err := rows.Scan(&v.ID, &v.ProductID, &v.UserID, &v.Rating, &v.Body, &v.Fit,
			&v.Status, &v.CreatedAt, &v.UpdatedAt, &v.ReviewerName); err != nil {
			return nil, 0, err
		}
		out = append(out, &v)
	}
	return out, total, rows.Err()
}

func (r *ReviewPG) Aggregate(ctx context.Context, productID uuid.UUID) (Aggregate, error) {
	var a Aggregate
	err := r.pool.QueryRow(ctx,
		`SELECT avg_rating, review_count FROM products WHERE id=$1`, productID).
		Scan(&a.AvgRating, &a.ReviewCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Aggregate{}, ErrNotFound
		}
		return Aggregate{}, err
	}
	return a, nil
}
```

Add the small int-to-string helper at the bottom of `review_pg.go` (import `strconv` instead if you prefer — either is fine, but be consistent):

```go
import "strconv" // add to the import block

func itoa(n int) string { return strconv.Itoa(n) }
```

> Implementer note: put `strconv` in the import block and define `itoa` once, OR replace every `itoa(...)` call with `strconv.Itoa(...)` and skip the helper. Don't leave an undefined `itoa`.

- [ ] **Step 3: Write integration tests** — `internal/review/repo/review_pg_test.go`

**Transaction decision (locked):** `ReviewPG` holds `*pgxpool.Pool` and opens a `BeginTx` inside each write method (write + `recompute` on the same tx, then `Commit`) — exactly the Step 2 code. Because the repo COMMITS, these integration tests do NOT use the rollback-tx pattern; they seed against `testPool` directly (committed) and clean up with `testfixtures.Clean(t, testPool)` at the start of each test. `Clean` deletes `products` (among others), which cascade-deletes `product_reviews` via the FK, so reviews are removed transitively. Run serially (`-p 1`), as the Makefile already does.

Convention otherwise mirrors `internal/store/repo/store_pg_test.go`: `//go:build integration`, package-level `testPool`, `TestMain` reading `TEST_DATABASE_URL`.

```go
//go:build integration

package repo

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/review/domain"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		panic("TEST_DATABASE_URL not set; run via `make test-integration`")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		panic(err)
	}
	testPool = pool
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// seedProduct creates an active brand + product + variant; returns brand, product, variant ids.
// Seeds against the pool (committed), since the repo under test commits its own writes.
func seedProduct(t *testing.T, db testfixtures.DBTX) (brand, product, variant uuid.UUID) {
	t.Helper()
	sb := testfixtures.SeedBrand(t, db, uuid.Nil)
	cat := testfixtures.SeedCategory(t, db)
	p := testfixtures.SeedProduct(t, db, sb.ID, cat.ID, "active")
	v := testfixtures.SeedVariant(t, db, p.ID, "M", "Black", 100000, 10)
	return sb.ID, p.ID, v
}

func TestReviewPG_Create_RecomputesAggregate(t *testing.T) {
	testfixtures.Clean(t, testPool)
	ctx := context.Background()
	brand, product, variant := seedProduct(t, testPool)
	user := testfixtures.SeedCustomer(t, testPool)
	testfixtures.SeedDeliveredOrderItem(t, testPool, user.ID, brand, product, variant)

	r := NewReviewPG(testPool)
	rv := &domain.Review{ProductID: product, UserID: user.ID, Rating: 4, Body: "Great fit and quality, very happy"}
	require.NoError(t, r.Create(ctx, rv))

	agg, err := r.Aggregate(ctx, product)
	require.NoError(t, err)
	require.Equal(t, 1, agg.ReviewCount)
	require.InDelta(t, 4.0, agg.AvgRating, 0.01)
}
```

Each test starts with `testfixtures.Clean(t, testPool)` and seeds against `testPool`. Add more integration tests in the same file:
- `TestReviewPG_ListByProduct_FilterAndSort` — seed 2 reviews (different users, ratings 5 and 3), assert `rating` filter returns one, `sort=rating_low` orders 3 before 5, total count correct.
- `TestReviewPG_SoftDelete_UpdatesAggregate` — create review (count 1), soft-delete, assert aggregate count 0, avg 0.
- `TestReviewPG_HasDeliveredPurchase` — true after `SeedDeliveredOrderItem`, false for a product with no delivered line.
- `TestReviewPG_Create_DuplicateReturnsErrDuplicate` — second `Create` for same (user,product) returns `ErrDuplicate`.

Write each with the same setup pattern (`testfixtures.Clean(t, testPool)`, `seedProduct(t, testPool)`, `SeedDeliveredOrderItem(..., testPool, ...)`, `NewReviewPG(testPool)`).

- [ ] **Step 4: Run integration tests**

Run:
```
TEST_DATABASE_URL="postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable" go test -tags=integration -p 1 ./internal/review/repo/ -v
```
Expected: all PASS. Also `go build ./internal/review/...` and `go vet ./internal/review/...` clean.

- [ ] **Step 5: Commit**

```bash
git add internal/review/repo/
git commit -m "feat(review): repo (verified-purchase, list/filter, write+recompute)"
```

---

## Task 5: Service — validation, gating, ownership, transaction owner

**Files:**
- Create: `internal/review/service/service.go`
- Test: `internal/review/service/service_test.go`

The repo owns its write transactions (locked decision in Task 4), so the service is plain: it depends only on the `repo.Repo` interface and calls its methods. This keeps the service fully unit-testable with a fake repo (no pool, no tx).

- [ ] **Step 1: Write the failing test** — `internal/review/service/service_test.go`

```go
package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/review/domain"
	"github.com/wearwhere/wearwhere_be/internal/review/repo"
)

type fakeRepo struct {
	productExists bool
	delivered     bool
	createErr     error
	created       *domain.Review
	getByID       *domain.Review
	updateErr     error
	deleteErr     error
}

func (f *fakeRepo) ProductExists(context.Context, uuid.UUID) (bool, error) { return f.productExists, nil }
func (f *fakeRepo) HasDeliveredPurchase(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return f.delivered, nil
}
func (f *fakeRepo) Create(_ context.Context, r *domain.Review) error {
	if f.createErr != nil {
		return f.createErr
	}
	r.ID = uuid.New()
	f.created = r
	return nil
}
func (f *fakeRepo) GetByID(context.Context, uuid.UUID) (*domain.Review, error) {
	if f.getByID == nil {
		return nil, repo.ErrNotFound
	}
	return f.getByID, nil
}
func (f *fakeRepo) Update(context.Context, uuid.UUID, int, string, *string) error { return f.updateErr }
func (f *fakeRepo) SoftDelete(context.Context, uuid.UUID) error                   { return f.deleteErr }
func (f *fakeRepo) ListByProduct(context.Context, uuid.UUID, repo.ListFilter) ([]*domain.ReviewView, int, error) {
	return nil, 0, nil
}
func (f *fakeRepo) Aggregate(context.Context, uuid.UUID) (repo.Aggregate, error) {
	return repo.Aggregate{}, nil
}

func newSvc(f *fakeRepo) *Service { return NewWithRepo(f) }

func TestCreate_RejectsUnverifiedPurchase(t *testing.T) {
	svc := newSvc(&fakeRepo{productExists: true, delivered: false})
	_, err := svc.Create(context.Background(), uuid.New(), uuid.New(),
		&domain.WriteReviewRequest{Rating: 5, Body: "Twenty characters minimum body!!"})
	if err == nil {
		t.Fatal("expected NOT_VERIFIED_PURCHASE error")
	}
}

func TestCreate_RejectsMissingProduct(t *testing.T) {
	svc := newSvc(&fakeRepo{productExists: false})
	_, err := svc.Create(context.Background(), uuid.New(), uuid.New(),
		&domain.WriteReviewRequest{Rating: 5, Body: "Twenty characters minimum body!!"})
	if err == nil {
		t.Fatal("expected PRODUCT_NOT_FOUND error")
	}
}

func TestCreate_DuplicateMapsToReviewExists(t *testing.T) {
	svc := newSvc(&fakeRepo{productExists: true, delivered: true, createErr: repo.ErrDuplicate})
	_, err := svc.Create(context.Background(), uuid.New(), uuid.New(),
		&domain.WriteReviewRequest{Rating: 5, Body: "Twenty characters minimum body!!"})
	if err == nil {
		t.Fatal("expected REVIEW_EXISTS error")
	}
}

func TestCreate_Success(t *testing.T) {
	f := &fakeRepo{productExists: true, delivered: true}
	svc := newSvc(f)
	got, err := svc.Create(context.Background(), uuid.New(), uuid.New(),
		&domain.WriteReviewRequest{Rating: 4, Body: "Twenty characters minimum body!!", Fit: "true"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Rating != 4 || f.created == nil {
		t.Errorf("review not created as expected: %+v", got)
	}
}

func TestUpdate_RejectsNonOwner(t *testing.T) {
	owner := uuid.New()
	other := uuid.New()
	rv := &domain.Review{ID: uuid.New(), UserID: owner, ProductID: uuid.New()}
	svc := newSvc(&fakeRepo{getByID: rv})
	err := svc.Update(context.Background(), other, rv.ID,
		&domain.WriteReviewRequest{Rating: 3, Body: "Twenty characters minimum body!!"})
	if err == nil {
		t.Fatal("expected FORBIDDEN for non-owner update")
	}
}

func TestDelete_RejectsNonOwner(t *testing.T) {
	owner := uuid.New()
	rv := &domain.Review{ID: uuid.New(), UserID: owner, ProductID: uuid.New()}
	svc := newSvc(&fakeRepo{getByID: rv})
	err := svc.Delete(context.Background(), uuid.New(), rv.ID)
	if err == nil {
		t.Fatal("expected FORBIDDEN for non-owner delete")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/review/service/ -v`
Expected: FAIL — `NewWithRepo`/`Service` undefined.

- [ ] **Step 3: Write the service** — `internal/review/service/service.go`

```go
// Package service holds product-review business logic: validation, the
// verified-purchase gate, ownership checks, and write orchestration.
package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/review/domain"
	"github.com/wearwhere/wearwhere_be/internal/review/repo"
)

type Service struct {
	repo repo.Repo
}

// NewWithRepo builds a service over an existing Repo (used by tests and by the
// production constructor once a pool-backed repo is provided).
func NewWithRepo(r repo.Repo) *Service { return &Service{repo: r} }

func fitPtr(fit string) *string {
	if strings.TrimSpace(fit) == "" {
		return nil
	}
	return &fit
}

func (s *Service) Create(ctx context.Context, userID, productID uuid.UUID, req *domain.WriteReviewRequest) (*domain.Review, error) {
	exists, err := s.repo.ProductExists(ctx, productID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, domain.ErrProductNotFound()
	}
	ok, err := s.repo.HasDeliveredPurchase(ctx, userID, productID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrNotVerifiedPurchase()
	}
	rv := &domain.Review{ProductID: productID, UserID: userID, Rating: req.Rating, Body: req.Body, Fit: fitPtr(req.Fit)}
	if err := s.repo.Create(ctx, rv); err != nil {
		if errors.Is(err, repo.ErrDuplicate) {
			return nil, domain.ErrReviewExists()
		}
		return nil, err
	}
	return rv, nil
}

func (s *Service) List(ctx context.Context, productID uuid.UUID, q *domain.ListReviewsQuery) (*domain.ListReviewsResponse, error) {
	exists, err := s.repo.ProductExists(ctx, productID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, domain.ErrProductNotFound()
	}
	f := repo.ListFilter{Rating: q.Rating, Fit: q.Fit, Sort: q.Sort, Limit: q.Limit, Offset: (q.Page - 1) * q.Limit}
	views, total, err := s.repo.ListByProduct(ctx, productID, f)
	if err != nil {
		return nil, err
	}
	agg, err := s.repo.Aggregate(ctx, productID)
	if err != nil {
		return nil, err
	}
	items := make([]domain.ReviewResponse, 0, len(views))
	for _, v := range views {
		items = append(items, domain.ToReviewResponse(v))
	}
	return &domain.ListReviewsResponse{
		Items:       items,
		AvgRating:   agg.AvgRating,
		ReviewCount: agg.ReviewCount,
		Pagination:  domain.NewPagination(q.Page, q.Limit, total),
	}, nil
}

func (s *Service) Update(ctx context.Context, userID, reviewID uuid.UUID, req *domain.WriteReviewRequest) error {
	rv, err := s.repo.GetByID(ctx, reviewID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.ErrReviewNotFound()
		}
		return err
	}
	if rv.UserID != userID {
		return domain.ErrForbidden()
	}
	if err := s.repo.Update(ctx, reviewID, req.Rating, req.Body, fitPtr(req.Fit)); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.ErrReviewNotFound()
		}
		return err
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, userID, reviewID uuid.UUID) error {
	rv, err := s.repo.GetByID(ctx, reviewID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.ErrReviewNotFound()
		}
		return err
	}
	if rv.UserID != userID {
		return domain.ErrForbidden()
	}
	if err := s.repo.SoftDelete(ctx, reviewID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.ErrReviewNotFound()
		}
		return err
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/review/service/ -v`
Expected: PASS (6 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/review/service/
git commit -m "feat(review): service (validation, verified-purchase gate, ownership)"
```

---

## Task 6: Handler + routes

**Files:**
- Create: `internal/review/handler/handler.go`
- Create: `internal/review/handler/routes.go`
- Test: `internal/review/handler/handler_test.go`

- [ ] **Step 1: Write the handler** — `internal/review/handler/handler.go`

```go
// Package handler exposes product-review HTTP endpoints.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/review/domain"
	"github.com/wearwhere/wearwhere_be/internal/review/service"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type Handler struct{ svc *service.Service }

func New(s *service.Service) *Handler { return &Handler{svc: s} }

func (h *Handler) userID(c *gin.Context) uuid.UUID {
	id, _ := authmw.UserID(c)
	return id
}

// Create handles POST /products/:id/reviews (customer-authed).
func (h *Handler) Create(c *gin.Context) {
	pid, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrProductNotFound())
		return
	}
	var req domain.WriteReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	rv, err := h.svc.Create(c.Request.Context(), h.userID(c), pid, &req)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.Created(c, gin.H{"id": rv.ID.String()})
}

// List handles GET /products/:id/reviews (public).
func (h *Handler) List(c *gin.Context) {
	pid, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrProductNotFound())
		return
	}
	var q domain.ListReviewsQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
		return
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.Limit == 0 {
		q.Limit = 20
	}
	resp, err := h.svc.List(c.Request.Context(), pid, &q)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}

// Update handles PATCH /reviews/:id (owner).
func (h *Handler) Update(c *gin.Context) {
	rid, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrReviewNotFound())
		return
	}
	var req domain.WriteReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if err := h.svc.Update(c.Request.Context(), h.userID(c), rid, &req); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, gin.H{"updated": true})
}

// Delete handles DELETE /reviews/:id (owner).
func (h *Handler) Delete(c *gin.Context) {
	rid, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.ErrorFromApp(c, domain.ErrReviewNotFound())
		return
	}
	if err := h.svc.Delete(c.Request.Context(), h.userID(c), rid); err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.NoContent(c)
}
```

- [ ] **Step 2: Write routes** — `internal/review/handler/routes.go`

```go
package handler

import "github.com/gin-gonic/gin"

// MountReviewsPublic registers the public read route (no auth).
func MountReviewsPublic(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/products/:id/reviews", h.List)
}

// MountReviewsAuthed registers customer-authed write routes. The caller must
// have chained RequireAuth onto rg.
func MountReviewsAuthed(rg *gin.RouterGroup, h *Handler) {
	rg.POST("/products/:id/reviews", h.Create)
	rg.PATCH("/reviews/:id", h.Update)
	rg.DELETE("/reviews/:id", h.Delete)
}
```

- [ ] **Step 3: Write handler tests** — `internal/review/handler/handler_test.go`

```go
package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/review/repo"
	"github.com/wearwhere/wearwhere_be/internal/review/service"
)

// minimal fake repo (reuse the service's interface) for handler wiring tests.
type fakeRepo struct{ productExists, delivered bool }

func (f *fakeRepo) ProductExists(_ interface{ Done() }, _ uuid.UUID) (bool, error) { return false, nil }

// NOTE: implement the repo.Repo interface fully here, mirroring the service test's
// fakeRepo (ProductExists, HasDeliveredPurchase, Create, GetByID, Update, SoftDelete,
// ListByProduct, Aggregate). Omitted for brevity — copy the service test's fakeRepo.

func setup(f repo.Repo, userID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := New(service.NewWithRepo(f))
	v1 := r.Group("/api/v1")
	MountReviewsPublic(v1, h)
	// emulate auth by injecting the user id, then mount authed routes
	authed := v1.Group("", func(c *gin.Context) { authmw.SetUserIDForTest(c, userID); c.Next() })
	MountReviewsAuthed(authed, h)
	return r
}

func TestCreate_InvalidBody_400(t *testing.T) {
	// body too short triggers binding error (min=20)
	f := &reviewFake{productExists: true, delivered: true} // use the full fake (see note)
	r := setup(f, uuid.New())
	w := httptest.NewRecorder()
	payload, _ := json.Marshal(map[string]any{"rating": 5, "body": "too short"})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/products/"+uuid.New().String()+"/reviews", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
```

> **Implementer:** the handler test needs a full `repo.Repo` fake. Rather than duplicate, define the fake in a shared test helper OR copy the service test's `fakeRepo` into the handler test package (it's a different package, so copy it and rename to `reviewFake`). Implement all 8 interface methods returning the configured values. Keep just two focused tests: (1) invalid body → 400; (2) a valid create with `productExists+delivered` → 201. Remove the stray `fakeRepo.ProductExists(interface{ Done() }...)` stub above — that was illustrative; use the real `repo.Repo` signatures.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/review/handler/ -v`
Expected: PASS. Also `go build ./...` clean.

- [ ] **Step 5: Commit**

```bash
git add internal/review/handler/
git commit -m "feat(review): HTTP handlers + routes (UC37/38)"
```

---

## Task 7: Wire into cmd/api

**Files:**
- Modify: `cmd/api/main.go`
- Modify: `cmd/api/main_test.go`

Read `cmd/api/main.go` first. Landmarks: imports block; the repo construction block (`wishlistRepo := wishlistrepo.NewWishlistPG(pgPool)` ~line 108); the services block (`wishlistSvc := ...` ~line 163); handler construction (`wishlistHandler := ...` ~line 263); the public mounts (`producthandler.MountCatalog(v1, ...)`, `storehandler.MountStoresPublic(v1, ...)` ~line 302-304); the customer-authed group (`customerGroup := v1.Group("/me", middleware.RequireAuth(jwtIssuer))` ~line 306).

- [ ] **Step 1: Add imports**

```go
	reviewhandler "github.com/wearwhere/wearwhere_be/internal/review/handler"
	reviewrepo "github.com/wearwhere/wearwhere_be/internal/review/repo"
	reviewservice "github.com/wearwhere/wearwhere_be/internal/review/service"
```

- [ ] **Step 2: Construct repo + service + handler**

Near the other repos/services (after the wishlist trio is fine):
```go
	reviewRepo := reviewrepo.NewReviewPG(pgPool)
	reviewSvc := reviewservice.NewWithRepo(reviewRepo)
	reviewHandler := reviewhandler.New(reviewSvc)
```

- [ ] **Step 3: Mount routes**

Public read, next to `storehandler.MountStoresPublic(v1, ...)`:
```go
	reviewhandler.MountReviewsPublic(v1, reviewHandler)
```
Authed writes — reviews are not under `/me`, so mount on an auth-chained group built on `v1`. Add after the customer group block:
```go
	reviewsAuthed := v1.Group("", middleware.RequireAuth(jwtIssuer))
	reviewhandler.MountReviewsAuthed(reviewsAuthed, reviewHandler)
```

- [ ] **Step 4: Mirror in `cmd/api/main_test.go`**

Read `cmd/api/main_test.go`; near the other public mounts and the auth-emulating setup, construct the review trio with the test pool and mount both public and authed groups (the test harness already has a way to mount authed routes — follow the existing wishlist/cart pattern; if the test uses `RequireAuth` with a real issuer, reuse it; if it injects a user id, mount the authed reviews group the same way). Use `reviewrepo.NewReviewPG(pool)`.

- [ ] **Step 5: Verify**

Run:
```
go build ./...
go test ./...
TEST_DATABASE_URL="postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable" go test -tags=integration -p 1 ./internal/review/... ./cmd/api/...
```
Expected: build clean; unit pass; integration pass.

- [ ] **Step 6: Commit**

```bash
git add cmd/api/main.go cmd/api/main_test.go
git commit -m "feat(review): wire reviews module into the API"
```

---

## Task 8: Surface avg_rating + review_count on product detail

**Files:**
- Modify: `internal/product/domain/dto.go` (ProductDetail struct)
- Modify: `internal/product/repo/catalog_query.go` (Detail query + scan)
- Possibly: `internal/product/service/*` and any `ToProductDetail` converter

This is a focused, additive change so UC38 ("display average rating prominently") is satisfied on the product detail page without a second API call.

- [ ] **Step 1: Read the product detail path**

Read `internal/product/repo/catalog_query.go` (the `Detail` function, its SELECT and `Scan`), `internal/product/domain/dto.go` (the `ProductDetail` struct and any `ToProductDetail`/converter), and the product entity used by Detail. Identify where `products` columns are selected and scanned for the single-product detail response.

- [ ] **Step 2: Add the two columns to the detail query + scan**

In the `Detail` SELECT, add `p.avg_rating, p.review_count` to the selected `products` columns, and add matching scan targets (two new fields on the scanned entity — add `AvgRating float64` and `ReviewCount int` to that entity struct). Follow the exact column ordering/scan pattern already present.

- [ ] **Step 3: Add fields to the response DTO + converter**

Add to `ProductDetail`:
```go
	AvgRating   float64 `json:"avg_rating"`
	ReviewCount int     `json:"review_count"`
```
Populate them in the converter that builds `ProductDetail` from the entity.

- [ ] **Step 4: Verify**

Run: `go build ./... && go test ./...`
And the integration suite for product + cmd:
```
TEST_DATABASE_URL="postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable" go test -tags=integration -p 1 ./internal/product/... ./cmd/api/...
```
Expected: build clean; tests pass. (If an existing product detail test asserts an exact JSON shape, update it to include the two new fields.)

> If this change proves more invasive than adding two columns + two fields (e.g. the detail entity is shared widely and adding fields ripples), STOP and report DONE_WITH_CONCERNS describing the ripple — surfacing on product detail can be deferred since the reviews list endpoint already returns `avg_rating`/`review_count`.

- [ ] **Step 5: Commit**

```bash
git add internal/product/
git commit -m "feat(product): expose avg_rating + review_count on product detail"
```

---

## Self-Review

**Spec coverage:**
- UC37 Submit (verified purchase, 1–5 stars, ≥20 chars, size/fit, "Verified Purchase" badge) → Task 2 (DTO binding + Fit), Task 4 (`HasDeliveredPurchase`, unique index), Task 5 (gating, validation), Task 6 (`POST`). Badge = `Verified: true` always. ✓
- UC38 View (list, filter by rating/fit, sort, avg prominent, verified first) → Task 4 (`ListByProduct` filter/sort), Task 5 (`List` + aggregate), Task 6 (`GET`), Task 8 (avg on product detail). "Verified first" trivially satisfied (all verified). ✓
- One review/(user,product) → unique partial index (Task 1) + `ErrDuplicate`→409 (Tasks 4/5). ✓
- Edit/delete own → Task 5 ownership + Task 6 PATCH/DELETE + soft delete. ✓
- Denormalized avg/count + recompute in tx → Task 1 columns, Task 4 `recompute`. ✓
- Moderation hook (`status` default published, list excludes hidden) → Task 1 column, Task 4 `where ... status='published'`. ✓
- Out of scope (photos, brand reply, report/takedown, block) → not implemented. ✓

**Placeholder scan:** The Task 2 `FitPtr` stray line is explicitly flagged for removal (typo guard, not a real placeholder). Transaction ownership is now LOCKED (repo owns tx via `pool.BeginTx`; integration tests use `testfixtures.Clean` teardown) — no ambiguity. The Task 6 handler-fake note says to copy the service fake — concrete. Task 8 is read-then-modify with exact field/column additions. No "add validation"/"handle errors" hand-waving.

**Type consistency:** `repo.Repo` interface (ProductExists, HasDeliveredPurchase, Create, GetByID, Update, SoftDelete, ListByProduct, Aggregate) is implemented by `ReviewPG` (Task 4) and the fakes (Tasks 5/6), and consumed by `service.Service` (Task 5). `domain.WriteReviewRequest` used by Create + Update. `domain.ReviewView` produced by `ListByProduct`, consumed by `ToReviewResponse`. `repo.Aggregate{AvgRating, ReviewCount}` matches the products columns. `domain.ErrReviewExists/ErrNotVerifiedPurchase/...` are `*httpx.AppError`. Consistent.

**Locked design decision (Task 4):** the write transaction lives in the repo — `ReviewPG` holds `*pgxpool.Pool` and each write method opens `BeginTx`, runs the row change + `recompute`, and `Commit`s. Because writes commit, the repo integration tests seed against `testPool` and clean up with `testfixtures.Clean` (no rollback-tx). The service depends only on `repo.Repo` and stays pool-free / unit-testable.
