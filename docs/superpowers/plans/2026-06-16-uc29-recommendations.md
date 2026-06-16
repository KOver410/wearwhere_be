# UC29 Get AI Recommendations — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A deterministic, heuristic "For You" product feed for logged-in customers, scored against their style profile (UC31) plus brand-follow / purchase / wishlist signals, with a Redis daily cache and a cold-start trending path.

**Architecture:** A new `internal/recommendation` module (domain → repo → service → handler), mounted on `/api/v1/me`. No LLM. The service loads a bounded candidate pool of active in-stock products (with their style tags) once, loads the caller's signals, computes a deterministic score, applies a deterministic discovery mix, and caches the assembled response per user per day in Redis. New users with no profile and no history get a trending feed plus an onboarding-prompt flag. A small invalidation seam is added to the UC31 style-profile service so saving a profile busts the cache.

**Tech Stack:** Go 1.23, gin, pgx/v5 (PostgreSQL), go-redis/v9, testify. Pure scoring/mixing logic is unit-tested with fakes; repo + cache use `//go:build integration` against `TEST_DATABASE_URL` (Postgres) and a local Redis.

**Spec:** `docs/superpowers/specs/2026-06-16-ai-personalization-design.md` (§4). UC31 (style profile) is already merged to `main` and provides `styleprofileservice.Service.LoadProfile(ctx, userID) (*styleprofiledomain.StyleProfileView, error)` (returns `nil, nil` when no profile).

**Conventions (verified):**
- Errors: handlers use `pkg/httpx` (`OK`, `Error`, `ErrorFromApp`). Caller id via `authmw.UserID(c)`.
- Repos: `DBTX` interface, `New<Thing>PG(db)`, `pgxpool.Pool` satisfies DBTX. Brand-follow / order / wishlist already exist but lack the bare-ID queries this module needs, so we add focused read methods in this module's own repo (querying the existing tables directly) rather than widening other modules' repos.
- Catalog facts: active product = `p.deleted_at IS NULL AND p.status='active'` and brand `b.deleted_at IS NULL AND b.status='active'`; in-stock = `bool_or(stock_qty>0)` over `product_variants` where `deleted_at IS NULL AND is_active`; min price = `MIN(price)` over the same; primary image = `product_images` ordered by `sort_order ASC LIMIT 1`; product↔tags via `product_style_tags`; delivered purchase = `sub_orders.status='delivered'`.

**File structure:**
```
internal/recommendation/
  domain/
    dto.go        RecProductCard, RecommendationsResponse, Candidate, UserSignals
  repo/
    repo.go       DBTX, CandidateRepo, SignalRepo interfaces
    candidate_pg.go   Candidates(ctx, limit) []Candidate
    signal_pg.go      FollowedBrandIDs / PurchasedProductIDs / AffinityCategoryIDs
    recommendation_pg_test.go  (integration)
  service/
    scorer.go     pure ScoreCandidate + Rank (deterministic discovery mix)
    scorer_test.go (unit)
    cache.go      Cache interface + RedisCache (daily key, JSON value)
    cache_redis_test.go (integration, skippable)
    service.go    Service.Recommend orchestration + ProfileLoader/Cache interfaces
    service_test.go (unit, fakes)
  handler/
    handler.go    GET /recommendations
    routes.go     Mount
    handler_test.go (unit, fake service-deps)
internal/config/config.go            MODIFY  add Recommendation config
internal/styleprofile/service/service.go  MODIFY  add optional onSaved invalidation hook
cmd/api/main.go                      MODIFY  construct + mount; wire cache invalidation into styleprofile
```

---

## Task 1: Config — recommendation tunables

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: Read the current config**

Open `internal/config/config.go`. Note the `Config` struct, the `Load()` function, and the `getEnv`/`getInt` helpers (used by `RedisConfig`). You will mirror that style.

- [ ] **Step 2: Add a `RecommendationConfig` sub-struct and field**

Add the struct near the other config sub-structs:

```go
type RecommendationConfig struct {
	DefaultLimit  int // env REC_FEED_DEFAULT_LIMIT, default 20
	MaxLimit      int // env REC_FEED_MAX_LIMIT, default 50
	CandidatePool int // env REC_CANDIDATE_POOL, default 300 — max products scored per request
}
```

Add a field to `Config`:

```go
	Recommendation RecommendationConfig
```

In `Load()`, populate it alongside the other sub-structs (use the existing `getInt(key, default)` helper — match its actual name/signature in the file):

```go
	cfg.Recommendation = RecommendationConfig{
		DefaultLimit:  getInt("REC_FEED_DEFAULT_LIMIT", 20),
		MaxLimit:      getInt("REC_FEED_MAX_LIMIT", 50),
		CandidatePool: getInt("REC_CANDIDATE_POOL", 300),
	}
```

- [ ] **Step 3: Build**

Run: `go build ./internal/config/...` then `go build ./...`
Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go
git commit -m "feat(recommendation): config tunables (feed limits, candidate pool)"
```

---

## Task 2: Domain types

**Files:**
- Create: `internal/recommendation/domain/dto.go`

- [ ] **Step 1: Write `dto.go`**

```go
package domain

import (
	"time"

	"github.com/google/uuid"
)

// RecProductCard is the public product card in the feed. Mirrors the catalog
// summary fields used elsewhere (id/slug/name/brand/price/image).
type RecProductCard struct {
	ID           string  `json:"id"`
	Slug         string  `json:"slug"`
	Name         string  `json:"name"`
	BrandSlug    string  `json:"brand_slug"`
	BrandName    string  `json:"brand_name"`
	Currency     string  `json:"currency"`
	MinPrice     float64 `json:"min_price"`
	PrimaryImage *string `json:"primary_image,omitempty"`
}

// RecommendationsResponse is the GET /me/recommendations body.
type RecommendationsResponse struct {
	Items            []RecProductCard `json:"items"`
	Source           string           `json:"source"` // "personalized" | "trending"
	OnboardingPrompt bool             `json:"onboarding_prompt"`
}

// Candidate is an internal scoring row: a catalog product plus the attributes
// the scorer needs. Not serialized to clients directly.
type Candidate struct {
	ProductID    uuid.UUID
	BrandID      uuid.UUID
	CategoryID   uuid.UUID
	Slug         string
	Name         string
	BrandSlug    string
	BrandName    string
	Currency     string
	MinPrice     float64
	PrimaryImage *string
	SoldCount    int
	CreatedAt    time.Time
	StyleTagIDs  []uuid.UUID
}

// ToCard projects a Candidate to its public card.
func (c Candidate) ToCard() RecProductCard {
	return RecProductCard{
		ID:           c.ProductID.String(),
		Slug:         c.Slug,
		Name:         c.Name,
		BrandSlug:    c.BrandSlug,
		BrandName:    c.BrandName,
		Currency:     c.Currency,
		MinPrice:     c.MinPrice,
		PrimaryImage: c.PrimaryImage,
	}
}

// UserSignals is the assembled per-user signal set the scorer reads.
// Maps are used for O(1) membership. HasProfile/HasHistory drive warm-vs-cold.
type UserSignals struct {
	StyleTagIDs         map[uuid.UUID]bool
	BudgetMin           *int
	BudgetMax           *int
	FollowedBrandIDs    map[uuid.UUID]bool
	PurchasedProductIDs map[uuid.UUID]bool
	AffinityCategoryIDs map[uuid.UUID]bool
}

// HasProfile is true when the user set any style tags or a budget.
func (s UserSignals) HasProfile() bool {
	return len(s.StyleTagIDs) > 0 || s.BudgetMin != nil || s.BudgetMax != nil
}

// HasHistory is true when the user has any behavioral signal.
func (s UserSignals) HasHistory() bool {
	return len(s.FollowedBrandIDs) > 0 || len(s.PurchasedProductIDs) > 0 || len(s.AffinityCategoryIDs) > 0
}
```

- [ ] **Step 2: Build**

Run: `go build ./internal/recommendation/...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/recommendation/domain
git commit -m "feat(recommendation): domain DTOs, Candidate, UserSignals"
```

---

## Task 3: Repository (candidates + signals + integration tests)

**Files:**
- Create: `internal/recommendation/repo/repo.go`
- Create: `internal/recommendation/repo/candidate_pg.go`
- Create: `internal/recommendation/repo/signal_pg.go`
- Test: `internal/recommendation/repo/recommendation_pg_test.go`

- [ ] **Step 1: Write `repo.go`**

```go
package repo

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
)

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// CandidateRepo loads the scorable product pool.
type CandidateRepo interface {
	// Candidates returns up to `limit` active, in-stock products ordered by
	// sold_count DESC, created_at DESC, id ASC (stable), each with its style tag ids.
	Candidates(ctx context.Context, limit int) ([]domain.Candidate, error)
}

// SignalRepo loads per-user behavioral signals.
type SignalRepo interface {
	FollowedBrandIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	PurchasedProductIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	AffinityCategoryIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
}
```

- [ ] **Step 2: Write `candidate_pg.go`**

```go
package repo

import (
	"context"

	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
)

type CandidatePG struct{ db DBTX }

func NewCandidatePG(db DBTX) *CandidatePG { return &CandidatePG{db: db} }

func (r *CandidatePG) Candidates(ctx context.Context, limit int) ([]domain.Candidate, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id, p.brand_id, p.category_id, p.slug, p.name,
		       b.slug, b.name, p.currency,
		       vp.min_price, p.sold_count, p.created_at,
		       (SELECT url FROM product_images
		          WHERE product_id = p.id ORDER BY sort_order ASC LIMIT 1) AS primary_image,
		       COALESCE(
		         (SELECT array_agg(pst.style_tag_id)
		            FROM product_style_tags pst WHERE pst.product_id = p.id),
		         '{}'::uuid[]) AS style_tag_ids
		  FROM products p
		  JOIN brands b ON b.id = p.brand_id
		  JOIN LATERAL (
		    SELECT MIN(price) AS min_price, bool_or(stock_qty > 0) AS in_stock
		      FROM product_variants
		     WHERE product_id = p.id AND deleted_at IS NULL AND is_active
		  ) vp ON true
		 WHERE p.deleted_at IS NULL AND p.status = 'active'
		   AND b.deleted_at IS NULL AND b.status = 'active'
		   AND vp.in_stock
		 ORDER BY p.sold_count DESC, p.created_at DESC, p.id ASC
		 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Candidate
	for rows.Next() {
		var c domain.Candidate
		if err := rows.Scan(
			&c.ProductID, &c.BrandID, &c.CategoryID, &c.Slug, &c.Name,
			&c.BrandSlug, &c.BrandName, &c.Currency,
			&c.MinPrice, &c.SoldCount, &c.CreatedAt,
			&c.PrimaryImage, &c.StyleTagIDs,
		); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
```

> **Note on `MinPrice` scan:** `vp.min_price` can be NULL only if a product has zero active variants — but `vp.in_stock` would then be false and the row is filtered out, so `MIN(price)` is always non-NULL for returned rows. Scanning into `float64` is safe. If a future change makes it nullable, switch the field to `*float64`.

- [ ] **Step 3: Write `signal_pg.go`**

```go
package repo

import (
	"context"

	"github.com/google/uuid"
)

type SignalPG struct{ db DBTX }

func NewSignalPG(db DBTX) *SignalPG { return &SignalPG{db: db} }

func scanUUIDs(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}) ([]uuid.UUID, error) {
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *SignalPG) FollowedBrandIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Query(ctx,
		`SELECT brand_id FROM brand_follows WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	return scanUUIDs(rows)
}

func (r *SignalPG) PurchasedProductIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT oi.product_id
		  FROM order_items oi
		  JOIN sub_orders so ON so.id = oi.sub_order_id
		  JOIN orders o      ON o.id = so.order_id
		 WHERE o.user_id = $1 AND so.status = 'delivered'`, userID)
	if err != nil {
		return nil, err
	}
	return scanUUIDs(rows)
}

func (r *SignalPG) AffinityCategoryIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT p.category_id FROM (
		    SELECT oi.product_id
		      FROM order_items oi
		      JOIN sub_orders so ON so.id = oi.sub_order_id
		      JOIN orders o      ON o.id = so.order_id
		     WHERE o.user_id = $1 AND so.status = 'delivered'
		    UNION
		    SELECT wi.product_id FROM wishlist_items wi WHERE wi.user_id = $1
		) src
		JOIN products p ON p.id = src.product_id`, userID)
	if err != nil {
		return nil, err
	}
	return scanUUIDs(rows)
}
```

> **Verify before finalizing:** confirm `pgx.Rows` satisfies the small interface used by `scanUUIDs` (it has `Next() bool`, `Scan(...any) error`, `Err() error`, `Close()`). It does. If the compiler complains, replace the anonymous interface with `pgx.Rows` directly in the three call sites.

- [ ] **Step 4: Write the integration test `recommendation_pg_test.go`**

```go
//go:build integration

package repo_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/recommendation/repo"
	"github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		panic("TEST_DATABASE_URL required")
	}
	var err error
	pool, err = pgxpool.New(context.Background(), dsn)
	if err != nil {
		panic(err)
	}
	defer pool.Close()
	os.Exit(m.Run())
}

func TestCandidatePG_ReturnsActiveInStockWithTags(t *testing.T) {
	ctx := context.Background()
	tx := testfixtures.BeginTx(t, pool)
	brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
	cat := testfixtures.SeedCategory(t, tx)
	tag := testfixtures.SeedStyleTag(t, tx)

	inStock := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
	testfixtures.SeedVariant(t, tx, inStock.ID, "M", "red", 200000, 5)
	_, err := tx.Exec(ctx, `INSERT INTO product_style_tags (product_id, style_tag_id) VALUES ($1,$2)`, inStock.ID, tag.ID)
	require.NoError(t, err)

	// out-of-stock product must be excluded
	oos := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
	testfixtures.SeedVariant(t, tx, oos.ID, "M", "blue", 150000, 0)

	r := repo.NewCandidatePG(tx)
	cands, err := r.Candidates(ctx, 100)
	require.NoError(t, err)

	var found bool
	for _, c := range cands {
		require.NotEqual(t, oos.ID, c.ProductID, "out-of-stock product must be excluded")
		if c.ProductID == inStock.ID {
			found = true
			require.Equal(t, []uuid.UUID{tag.ID}, c.StyleTagIDs)
			require.Equal(t, float64(200000), c.MinPrice)
			require.Equal(t, brand.Slug, c.BrandSlug)
		}
	}
	require.True(t, found, "in-stock tagged product must be returned")
}

func TestSignalPG_FollowedBrands_Purchases_Affinity(t *testing.T) {
	ctx := context.Background()
	tx := testfixtures.BeginTx(t, pool)
	user := testfixtures.SeedCustomer(t, tx)
	brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
	cat := testfixtures.SeedCategory(t, tx)
	prod := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
	variant := testfixtures.SeedVariant(t, tx, prod.ID, "M", "red", 200000, 5)

	_, err := tx.Exec(ctx, `INSERT INTO brand_follows (user_id, brand_id) VALUES ($1,$2)`, user.ID, brand.ID)
	require.NoError(t, err)
	testfixtures.SeedDeliveredOrderItem(t, tx, user.ID, brand.ID, prod.ID, variant)

	sr := repo.NewSignalPG(tx)

	brands, err := sr.FollowedBrandIDs(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{brand.ID}, brands)

	purchased, err := sr.PurchasedProductIDs(ctx, user.ID)
	require.NoError(t, err)
	require.Contains(t, purchased, prod.ID)

	cats, err := sr.AffinityCategoryIDs(ctx, user.ID)
	require.NoError(t, err)
	require.Contains(t, cats, cat.ID)
}
```

- [ ] **Step 5: Verify build + integration compile, then run if a DB is available**

Run: `go build ./...` and `go vet -tags=integration ./internal/recommendation/repo/...` (must succeed).
If `TEST_DATABASE_URL` + a migrated DB are available:
```bash
TEST_DATABASE_URL="postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable" go test -tags=integration -p 1 ./internal/recommendation/repo/... -v
```
Expected: PASS. If no DB, report that the integration run is deferred.

- [ ] **Step 6: Commit**

```bash
git add internal/recommendation/repo
git commit -m "feat(recommendation): candidate + signal repositories"
```

---

## Task 4: Scorer (pure, deterministic) — the heart

**Files:**
- Create: `internal/recommendation/service/scorer.go`
- Test: `internal/recommendation/service/scorer_test.go`

- [ ] **Step 1: Write the failing test `scorer_test.go`**

```go
package service_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
	"github.com/wearwhere/wearwhere_be/internal/recommendation/service"
)

func intp(v int) *int { return &v }

func cand(id, brand, cat uuid.UUID, price float64, sold int, tags ...uuid.UUID) domain.Candidate {
	return domain.Candidate{
		ProductID: id, BrandID: brand, CategoryID: cat,
		MinPrice: price, SoldCount: sold, CreatedAt: time.Unix(int64(sold), 0),
		StyleTagIDs: tags,
	}
}

func TestScoreCandidate_AllSignals(t *testing.T) {
	tag := uuid.New()
	brand := uuid.New()
	cat := uuid.New()
	sig := domain.UserSignals{
		StyleTagIDs:         map[uuid.UUID]bool{tag: true},
		BudgetMin:           intp(100000),
		BudgetMax:           intp(300000),
		FollowedBrandIDs:    map[uuid.UUID]bool{brand: true},
		AffinityCategoryIDs: map[uuid.UUID]bool{cat: true},
		PurchasedProductIDs: map[uuid.UUID]bool{},
	}
	c := cand(uuid.New(), brand, cat, 200000, 1, tag)
	// tag(10) + brand(8) + budget-in(5) + category(3) = 26
	require.Equal(t, 26, service.ScoreCandidate(c, sig))
}

func TestScoreCandidate_BudgetOutPenalty(t *testing.T) {
	sig := domain.UserSignals{
		StyleTagIDs: map[uuid.UUID]bool{}, FollowedBrandIDs: map[uuid.UUID]bool{},
		AffinityCategoryIDs: map[uuid.UUID]bool{}, PurchasedProductIDs: map[uuid.UUID]bool{},
		BudgetMin: intp(100000), BudgetMax: intp(200000),
	}
	c := cand(uuid.New(), uuid.New(), uuid.New(), 500000, 1)
	require.Equal(t, -3, service.ScoreCandidate(c, sig)) // out of budget
}

func TestScoreCandidate_NoBudgetNoBudgetScore(t *testing.T) {
	sig := domain.UserSignals{
		StyleTagIDs: map[uuid.UUID]bool{}, FollowedBrandIDs: map[uuid.UUID]bool{},
		AffinityCategoryIDs: map[uuid.UUID]bool{}, PurchasedProductIDs: map[uuid.UUID]bool{},
	}
	c := cand(uuid.New(), uuid.New(), uuid.New(), 500000, 1)
	require.Equal(t, 0, service.ScoreCandidate(c, sig))
}

func TestRank_ExcludesPurchasedAndIsDeterministic(t *testing.T) {
	tag := uuid.New()
	brand := uuid.New()
	cat := uuid.New()
	bought := uuid.New()
	sig := domain.UserSignals{
		StyleTagIDs:         map[uuid.UUID]bool{tag: true},
		FollowedBrandIDs:    map[uuid.UUID]bool{brand: true},
		AffinityCategoryIDs: map[uuid.UUID]bool{},
		PurchasedProductIDs: map[uuid.UUID]bool{bought: true},
	}
	matching := cand(uuid.New(), brand, cat, 100000, 5, tag) // high score
	purchased := cand(bought, brand, cat, 100000, 9, tag)    // excluded
	cands := []domain.Candidate{purchased, matching}

	out := service.Rank(cands, sig, 10)
	ids := map[uuid.UUID]bool{}
	for _, c := range out {
		ids[c.ProductID] = true
	}
	require.False(t, ids[bought], "purchased product must be excluded")
	require.True(t, ids[matching.ProductID])

	// Determinism: same input → identical order.
	out2 := service.Rank(cands, sig, 10)
	require.Equal(t, out, out2)
}

func TestRank_IncludesDiscoverySlice(t *testing.T) {
	tag := uuid.New()
	brand := uuid.New()
	cat := uuid.New()
	sig := domain.UserSignals{
		StyleTagIDs:         map[uuid.UUID]bool{tag: true},
		FollowedBrandIDs:    map[uuid.UUID]bool{brand: true},
		AffinityCategoryIDs: map[uuid.UUID]bool{},
		PurchasedProductIDs: map[uuid.UUID]bool{},
	}
	// 8 matching (explored) + 4 unexplored (different brand, no matching tag)
	var cands []domain.Candidate
	for i := 0; i < 8; i++ {
		cands = append(cands, cand(uuid.New(), brand, cat, 100000, 100-i, tag))
	}
	otherBrand := uuid.New()
	for i := 0; i < 4; i++ {
		cands = append(cands, cand(uuid.New(), otherBrand, cat, 100000, 50-i))
	}
	out := service.Rank(cands, sig, 10)
	require.Len(t, out, 10)
	// With limit 10, discovery target = 3 (30%). At least one unexplored
	// (otherBrand) item must appear in the result.
	var discovery int
	for _, c := range out {
		if c.BrandID == otherBrand {
			discovery++
		}
	}
	require.GreaterOrEqual(t, discovery, 1, "discovery slice must surface unexplored items")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/recommendation/service/...`
Expected: FAIL — `service.ScoreCandidate` / `service.Rank` undefined.

- [ ] **Step 3: Write `scorer.go`**

```go
package service

import (
	"sort"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
)

const (
	weightStyleTag  = 10
	weightBrand     = 8
	weightBudgetIn  = 5
	weightBudgetOut = -3
	weightCategory  = 3
)

// ScoreCandidate computes the heuristic score of one candidate for one user.
func ScoreCandidate(c domain.Candidate, sig domain.UserSignals) int {
	score := 0
	for _, tid := range c.StyleTagIDs {
		if sig.StyleTagIDs[tid] {
			score += weightStyleTag
		}
	}
	if sig.FollowedBrandIDs[c.BrandID] {
		score += weightBrand
	}
	if sig.BudgetMin != nil || sig.BudgetMax != nil {
		if inBudget(c.MinPrice, sig.BudgetMin, sig.BudgetMax) {
			score += weightBudgetIn
		} else {
			score += weightBudgetOut
		}
	}
	if sig.AffinityCategoryIDs[c.CategoryID] {
		score += weightCategory
	}
	return score
}

func inBudget(price float64, min, max *int) bool {
	if min != nil && price < float64(*min) {
		return false
	}
	if max != nil && price > float64(*max) {
		return false
	}
	return true
}

// explored reports whether the user already has affinity for this candidate
// (a matching style tag or a followed brand). Unexplored candidates feed the
// discovery slice.
func explored(c domain.Candidate, sig domain.UserSignals) bool {
	if sig.FollowedBrandIDs[c.BrandID] {
		return true
	}
	for _, tid := range c.StyleTagIDs {
		if sig.StyleTagIDs[tid] {
			return true
		}
	}
	return false
}

// Rank excludes purchased products, scores the rest, and returns up to `limit`
// products: ~70% by score with a ~30% discovery slice of unexplored items
// (round-robin by brand for diversity). Fully deterministic.
func Rank(cands []domain.Candidate, sig domain.UserSignals, limit int) []domain.Candidate {
	// 1. Filter out purchased.
	pool := make([]domain.Candidate, 0, len(cands))
	for _, c := range cands {
		if !sig.PurchasedProductIDs[c.ProductID] {
			pool = append(pool, c)
		}
	}

	// 2. Stable sort by score DESC, sold_count DESC, created_at DESC, id ASC.
	scores := make(map[uuid.UUID]int, len(pool))
	for _, c := range pool {
		scores[c.ProductID] = ScoreCandidate(c, sig)
	}
	sort.SliceStable(pool, func(i, j int) bool {
		a, b := pool[i], pool[j]
		if scores[a.ProductID] != scores[b.ProductID] {
			return scores[a.ProductID] > scores[b.ProductID]
		}
		if a.SoldCount != b.SoldCount {
			return a.SoldCount > b.SoldCount
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.After(b.CreatedAt)
		}
		return a.ProductID.String() < b.ProductID.String()
	})

	if limit <= 0 || len(pool) <= limit {
		return pool
	}

	// 3. Split into top slice + discovery slice.
	discoveryTarget := limit * 3 / 10 // ~30%, integer floor
	topTarget := limit - discoveryTarget

	chosen := make([]domain.Candidate, 0, limit)
	used := make(map[uuid.UUID]bool, limit)
	for _, c := range pool {
		if len(chosen) >= topTarget {
			break
		}
		chosen = append(chosen, c)
		used[c.ProductID] = true
	}

	// 4. Discovery: unexplored items not yet used, round-robin by brand.
	if discoveryTarget > 0 {
		var disc []domain.Candidate
		for _, c := range pool {
			if !used[c.ProductID] && !explored(c, sig) {
				disc = append(disc, c)
			}
		}
		for _, c := range roundRobinByBrand(disc, discoveryTarget) {
			chosen = append(chosen, c)
			used[c.ProductID] = true
		}
	}

	// 5. Backfill from the remaining scored pool if short.
	for _, c := range pool {
		if len(chosen) >= limit {
			break
		}
		if !used[c.ProductID] {
			chosen = append(chosen, c)
			used[c.ProductID] = true
		}
	}
	return chosen
}

// roundRobinByBrand returns up to n items, taking at most one item per brand
// per pass (in the input order, which is already score-sorted) so the slice is
// brand-diverse and deterministic.
func roundRobinByBrand(in []domain.Candidate, n int) []domain.Candidate {
	if n <= 0 || len(in) == 0 {
		return nil
	}
	out := make([]domain.Candidate, 0, n)
	taken := make(map[uuid.UUID]bool, len(in))
	for len(out) < n {
		seenBrand := make(map[uuid.UUID]bool)
		progressed := false
		for _, c := range in {
			if taken[c.ProductID] || seenBrand[c.BrandID] {
				continue
			}
			out = append(out, c)
			taken[c.ProductID] = true
			seenBrand[c.BrandID] = true
			progressed = true
			if len(out) >= n {
				return out
			}
		}
		if !progressed {
			break
		}
	}
	return out
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/recommendation/service/... -v -run 'Score|Rank'`
Expected: PASS for all 5 tests.

- [ ] **Step 5: Commit**

```bash
git add internal/recommendation/service/scorer.go internal/recommendation/service/scorer_test.go
git commit -m "feat(recommendation): deterministic scorer + discovery mixing"
```

---

## Task 5: Cache (Redis daily) + interface

**Files:**
- Create: `internal/recommendation/service/cache.go`
- Test: `internal/recommendation/service/cache_redis_test.go`

- [ ] **Step 1: Write `cache.go`**

```go
package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
)

// Cache stores the assembled feed per user per day. Implementations must treat
// a miss as (nil, false, nil) — not an error.
type Cache interface {
	Get(ctx context.Context, userID uuid.UUID) (*domain.RecommendationsResponse, bool, error)
	Set(ctx context.Context, userID uuid.UUID, resp *domain.RecommendationsResponse) error
	Invalidate(ctx context.Context, userID uuid.UUID) error
}

// RedisCache is the production Cache. Key: rec:feed:{user}:{yyyymmdd}, TTL 24h.
// Value: the JSON-encoded RecommendationsResponse.
type RedisCache struct{ rdb *redis.Client }

func NewRedisCache(rdb *redis.Client) *RedisCache { return &RedisCache{rdb: rdb} }

func dayKey(userID uuid.UUID, now time.Time) string {
	return "rec:feed:" + userID.String() + ":" + now.UTC().Format("20060102")
}

func (c *RedisCache) Get(ctx context.Context, userID uuid.UUID) (*domain.RecommendationsResponse, bool, error) {
	raw, err := c.rdb.Get(ctx, dayKey(userID, time.Now())).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var resp domain.RecommendationsResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		// Corrupt cache entry — treat as a miss so we recompute.
		return nil, false, nil
	}
	return &resp, true, nil
}

func (c *RedisCache) Set(ctx context.Context, userID uuid.UUID, resp *domain.RecommendationsResponse) error {
	raw, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, dayKey(userID, time.Now()), raw, 24*time.Hour).Err()
}

func (c *RedisCache) Invalidate(ctx context.Context, userID uuid.UUID) error {
	return c.rdb.Del(ctx, dayKey(userID, time.Now())).Err()
}
```

- [ ] **Step 2: Write the integration test `cache_redis_test.go`** (skips when no Redis)

```go
//go:build integration

package service_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
	"github.com/wearwhere/wearwhere_be/internal/recommendation/service"
)

func redisAddr() string {
	if a := os.Getenv("REDIS_ADDR"); a != "" {
		return a
	}
	return "localhost:6379"
}

func TestRedisCache_RoundTripAndInvalidate(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr()})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("redis not reachable at %s: %v", redisAddr(), err)
	}
	defer rdb.Close()

	c := service.NewRedisCache(rdb)
	uid := uuid.New()

	_, ok, err := c.Get(ctx, uid)
	require.NoError(t, err)
	require.False(t, ok)

	resp := &domain.RecommendationsResponse{
		Items:  []domain.RecProductCard{{ID: uuid.New().String(), Name: "X"}},
		Source: "personalized",
	}
	require.NoError(t, c.Set(ctx, uid, resp))

	got, ok, err := c.Get(ctx, uid)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "personalized", got.Source)
	require.Len(t, got.Items, 1)

	require.NoError(t, c.Invalidate(ctx, uid))
	_, ok, err = c.Get(ctx, uid)
	require.NoError(t, err)
	require.False(t, ok)
}
```

- [ ] **Step 3: Verify**

Run: `go build ./...`, `go vet -tags=integration ./internal/recommendation/service/...`.
If Redis is up: `go test -tags=integration ./internal/recommendation/service/... -run RedisCache -v` → PASS. Otherwise it skips.

- [ ] **Step 4: Commit**

```bash
git add internal/recommendation/service/cache.go internal/recommendation/service/cache_redis_test.go
git commit -m "feat(recommendation): redis daily cache"
```

---

## Task 6: Service orchestration

**Files:**
- Create: `internal/recommendation/service/service.go`
- Test: `internal/recommendation/service/service_test.go`

- [ ] **Step 1: Write the failing test `service_test.go`**

```go
package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
	"github.com/wearwhere/wearwhere_be/internal/recommendation/service"
	spdomain "github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
)

type fakeCandidates struct{ list []domain.Candidate }

func (f *fakeCandidates) Candidates(_ context.Context, _ int) ([]domain.Candidate, error) {
	return f.list, nil
}

type fakeSignals struct {
	brands []uuid.UUID
	bought []uuid.UUID
	cats   []uuid.UUID
}

func (f *fakeSignals) FollowedBrandIDs(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return f.brands, nil
}
func (f *fakeSignals) PurchasedProductIDs(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return f.bought, nil
}
func (f *fakeSignals) AffinityCategoryIDs(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return f.cats, nil
}

type fakeProfile struct{ view *spdomain.StyleProfileView }

func (f *fakeProfile) LoadProfile(_ context.Context, _ uuid.UUID) (*spdomain.StyleProfileView, error) {
	return f.view, nil
}

type fakeCache struct {
	stored *domain.RecommendationsResponse
}

func (f *fakeCache) Get(_ context.Context, _ uuid.UUID) (*domain.RecommendationsResponse, bool, error) {
	return f.stored, f.stored != nil, nil
}
func (f *fakeCache) Set(_ context.Context, _ uuid.UUID, r *domain.RecommendationsResponse) error {
	f.stored = r
	return nil
}
func (f *fakeCache) Invalidate(_ context.Context, _ uuid.UUID) error { f.stored = nil; return nil }

func newCand(brand, cat uuid.UUID, tags ...uuid.UUID) domain.Candidate {
	return domain.Candidate{ProductID: uuid.New(), BrandID: brand, CategoryID: cat, MinPrice: 100000, StyleTagIDs: tags}
}

func cfg() service.Config { return service.Config{DefaultLimit: 20, MaxLimit: 50, CandidatePool: 300} }

func TestRecommend_ColdStartTrending(t *testing.T) {
	cands := []domain.Candidate{newCand(uuid.New(), uuid.New()), newCand(uuid.New(), uuid.New())}
	svc := service.New(&fakeCandidates{list: cands}, &fakeSignals{}, &fakeProfile{}, &fakeCache{}, cfg())

	resp, err := svc.Recommend(context.Background(), uuid.New(), 0)
	require.NoError(t, err)
	require.Equal(t, "trending", resp.Source)
	require.True(t, resp.OnboardingPrompt)
	require.Len(t, resp.Items, 2)
}

func TestRecommend_PersonalizedWhenProfilePresent(t *testing.T) {
	tag := uuid.New()
	brand := uuid.New()
	cat := uuid.New()
	cands := []domain.Candidate{newCand(brand, cat, tag), newCand(uuid.New(), cat)}
	prof := &fakeProfile{view: &spdomain.StyleProfileView{
		StyleTags: []spdomain.StyleTagRef{{ID: tag.String()}},
	}}
	svc := service.New(&fakeCandidates{list: cands}, &fakeSignals{}, prof, &fakeCache{}, cfg())

	resp, err := svc.Recommend(context.Background(), uuid.New(), 0)
	require.NoError(t, err)
	require.Equal(t, "personalized", resp.Source)
	require.False(t, resp.OnboardingPrompt)
	require.NotEmpty(t, resp.Items)
}

func TestRecommend_UsesCacheOnSecondCall(t *testing.T) {
	cache := &fakeCache{}
	cands := []domain.Candidate{newCand(uuid.New(), uuid.New())}
	prof := &fakeProfile{view: &spdomain.StyleProfileView{BudgetMin: nil}}
	// give it history so it's personalized
	sig := &fakeSignals{brands: []uuid.UUID{uuid.New()}}
	svc := service.New(&fakeCandidates{list: cands}, sig, prof, cache, cfg())

	r1, err := svc.Recommend(context.Background(), uuid.New(), 0)
	require.NoError(t, err)
	require.NotNil(t, cache.stored, "first call must populate cache")
	r2, err := svc.Recommend(context.Background(), uuid.New(), 0)
	require.NoError(t, err)
	require.Equal(t, r1.Source, r2.Source)
}

func TestRecommend_ClampsLimit(t *testing.T) {
	svc := service.New(&fakeCandidates{}, &fakeSignals{}, &fakeProfile{}, &fakeCache{}, cfg())
	require.Equal(t, 20, svc.ResolveLimit(0))
	require.Equal(t, 50, svc.ResolveLimit(1000))
	require.Equal(t, 15, svc.ResolveLimit(15))
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/recommendation/service/...`
Expected: FAIL — `service.New`, `service.Config`, `Recommend`, `ResolveLimit` undefined.

- [ ] **Step 3: Write `service.go`**

```go
package service

import (
	"context"
	"log"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
	"github.com/wearwhere/wearwhere_be/internal/recommendation/repo"
	spdomain "github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
)

// Config holds the feed tunables (mirrors config.RecommendationConfig).
type Config struct {
	DefaultLimit  int
	MaxLimit      int
	CandidatePool int
}

// ProfileLoader is the in-process style-profile reader (satisfied by
// styleprofile/service.Service.LoadProfile). Returns nil when no profile.
type ProfileLoader interface {
	LoadProfile(ctx context.Context, userID uuid.UUID) (*spdomain.StyleProfileView, error)
}

type Service struct {
	candidates repo.CandidateRepo
	signals    repo.SignalRepo
	profiles   ProfileLoader
	cache      Cache
	cfg        Config
}

func New(c repo.CandidateRepo, s repo.SignalRepo, p ProfileLoader, cache Cache, cfg Config) *Service {
	return &Service{candidates: c, signals: s, profiles: p, cache: cache, cfg: cfg}
}

// ResolveLimit clamps the requested limit into [1, MaxLimit], defaulting when <= 0.
func (s *Service) ResolveLimit(requested int) int {
	if requested <= 0 {
		return s.cfg.DefaultLimit
	}
	if requested > s.cfg.MaxLimit {
		return s.cfg.MaxLimit
	}
	return requested
}

// Invalidate busts the user's cached feed (called on profile/order change).
func (s *Service) Invalidate(ctx context.Context, userID uuid.UUID) error {
	return s.cache.Invalidate(ctx, userID)
}

func (s *Service) Recommend(ctx context.Context, userID uuid.UUID, requestedLimit int) (*domain.RecommendationsResponse, error) {
	limit := s.ResolveLimit(requestedLimit)

	if cached, ok, err := s.cache.Get(ctx, userID); err == nil && ok {
		return cached, nil
	} else if err != nil {
		log.Printf("recommendation: cache get failed for %s: %v", userID, err)
	}

	sig, err := s.loadSignals(ctx, userID)
	if err != nil {
		return nil, err
	}

	pool, err := s.candidates.Candidates(ctx, s.cfg.CandidatePool)
	if err != nil {
		return nil, err
	}
	if len(pool) == s.cfg.CandidatePool {
		log.Printf("recommendation: candidate pool capped at %d for %s; some products not scored", s.cfg.CandidatePool, userID)
	}

	var resp domain.RecommendationsResponse
	if sig.HasProfile() || sig.HasHistory() {
		ranked := Rank(pool, sig, limit)
		resp = domain.RecommendationsResponse{
			Items:            toCards(ranked),
			Source:           "personalized",
			OnboardingPrompt: false,
		}
	} else {
		// Cold start: trending = top of the (sold_count-ordered) pool, minus purchased.
		trending := topN(pool, sig, limit)
		resp = domain.RecommendationsResponse{
			Items:            toCards(trending),
			Source:           "trending",
			OnboardingPrompt: true,
		}
	}

	if err := s.cache.Set(ctx, userID, &resp); err != nil {
		log.Printf("recommendation: cache set failed for %s: %v", userID, err)
	}
	return &resp, nil
}

func (s *Service) loadSignals(ctx context.Context, userID uuid.UUID) (domain.UserSignals, error) {
	sig := domain.UserSignals{
		StyleTagIDs:         map[uuid.UUID]bool{},
		FollowedBrandIDs:    map[uuid.UUID]bool{},
		PurchasedProductIDs: map[uuid.UUID]bool{},
		AffinityCategoryIDs: map[uuid.UUID]bool{},
	}

	prof, err := s.profiles.LoadProfile(ctx, userID)
	if err != nil {
		return sig, err
	}
	if prof != nil {
		sig.BudgetMin = prof.BudgetMin
		sig.BudgetMax = prof.BudgetMax
		for _, t := range prof.StyleTags {
			if id, err := uuid.Parse(t.ID); err == nil {
				sig.StyleTagIDs[id] = true
			}
		}
	}

	brands, err := s.signals.FollowedBrandIDs(ctx, userID)
	if err != nil {
		return sig, err
	}
	for _, b := range brands {
		sig.FollowedBrandIDs[b] = true
	}

	bought, err := s.signals.PurchasedProductIDs(ctx, userID)
	if err != nil {
		return sig, err
	}
	for _, p := range bought {
		sig.PurchasedProductIDs[p] = true
	}

	cats, err := s.signals.AffinityCategoryIDs(ctx, userID)
	if err != nil {
		return sig, err
	}
	for _, c := range cats {
		sig.AffinityCategoryIDs[c] = true
	}
	return sig, nil
}

// topN returns the first `limit` non-purchased candidates (pool is already
// sold_count-ordered), used for the cold-start trending feed.
func topN(pool []domain.Candidate, sig domain.UserSignals, limit int) []domain.Candidate {
	out := make([]domain.Candidate, 0, limit)
	for _, c := range pool {
		if sig.PurchasedProductIDs[c.ProductID] {
			continue
		}
		out = append(out, c)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func toCards(cands []domain.Candidate) []domain.RecProductCard {
	out := make([]domain.RecProductCard, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.ToCard())
	}
	return out
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/recommendation/service/... -v`
Expected: PASS for the scorer tests AND the 4 service tests.

- [ ] **Step 5: Commit**

```bash
git add internal/recommendation/service/service.go internal/recommendation/service/service_test.go
git commit -m "feat(recommendation): service orchestration (warm/cold paths, cache, limit clamp)"
```

---

## Task 7: Handler + routes

**Files:**
- Create: `internal/recommendation/handler/handler.go`
- Create: `internal/recommendation/handler/routes.go`
- Test: `internal/recommendation/handler/handler_test.go`

- [ ] **Step 1: Write `handler.go`**

```go
package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

// Recommender is the service capability the handler needs.
type Recommender interface {
	Recommend(ctx interface{ Done() <-chan struct{} }, userID uuid.UUID, limit int) (*domain.RecommendationsResponse, error)
}
```

> **Correction:** do NOT use the odd `ctx interface{...}` above. Define the interface with the real `context.Context`:

```go
package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
	"github.com/wearwhere/wearwhere_be/pkg/httpx"
)

// Recommender is the service capability the handler needs (satisfied by
// recommendation/service.Service).
type Recommender interface {
	Recommend(ctx context.Context, userID uuid.UUID, limit int) (*domain.RecommendationsResponse, error)
}

type Handler struct{ svc Recommender }

func New(svc Recommender) *Handler { return &Handler{svc: svc} }

func (h *Handler) userID(c *gin.Context) uuid.UUID {
	id, _ := authmw.UserID(c)
	return id
}

func (h *Handler) List(c *gin.Context) {
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	resp, err := h.svc.Recommend(c.Request.Context(), h.userID(c), limit)
	if err != nil {
		httpx.ErrorFromApp(c, err)
		return
	}
	httpx.OK(c, resp)
}

var _ = http.StatusOK // keep net/http imported if unused elsewhere; remove if not needed
```

> Remove the trailing `var _ = http.StatusOK` line and the `net/http` import if the compiler reports `net/http` unused (it is only there as a guard). Keep the file clean — final file should import only what it uses.

- [ ] **Step 2: Write `routes.go`**

```go
package handler

import "github.com/gin-gonic/gin"

// Mount registers the recommendation route under a group that already applies
// RequireAuth + RequireRole(customer) (the /me customer group).
func Mount(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/recommendations", h.List)
}
```

- [ ] **Step 3: Write `handler_test.go`**

```go
package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
	"github.com/wearwhere/wearwhere_be/internal/recommendation/handler"
)

type fakeSvc struct{ last int }

func (f *fakeSvc) Recommend(_ context.Context, _ uuid.UUID, limit int) (*domain.RecommendationsResponse, error) {
	f.last = limit
	return &domain.RecommendationsResponse{
		Items:  []domain.RecProductCard{{ID: uuid.New().String(), Name: "X"}},
		Source: "trending", OnboardingPrompt: true,
	}, nil
}

func setup(f *fakeSvc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		authmw.SetUserIDForTest(c, uuid.New()) // mirror wishlist/styleprofile handler tests
		c.Next()
	})
	handler.Mount(r.Group("/me"), handler.New(f))
	return r
}

func TestList_ReturnsFeed(t *testing.T) {
	f := &fakeSvc{}
	r := setup(f)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/me/recommendations?limit=12", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 12, f.last, "limit query must be parsed and forwarded")

	var body domain.RecommendationsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "trending", body.Source)
	require.True(t, body.OnboardingPrompt)
	require.Len(t, body.Items, 1)
}

func TestList_NoLimitForwardsZero(t *testing.T) {
	f := &fakeSvc{last: -1}
	r := setup(f)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/me/recommendations", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 0, f.last, "missing limit forwards 0 (service applies default)")
}
```

> **Verify:** `authmw.SetUserIDForTest` exists (the UC31 + wishlist handler tests use it). If the helper name differs, use whatever those tests use.

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/recommendation/handler/... -v` (expect 2 PASS) and `go build ./...`.

- [ ] **Step 5: Commit**

```bash
git add internal/recommendation/handler
git commit -m "feat(recommendation): GET /me/recommendations handler + routes"
```

---

## Task 8: Wire into the API + cache-invalidation seam in UC31

**Files:**
- Modify: `internal/styleprofile/service/service.go`
- Modify: `cmd/api/main.go`

- [ ] **Step 1: Add an optional invalidation hook to the style-profile service**

Open `internal/styleprofile/service/service.go`. Add an optional callback field and setter, and invoke it after a successful `Save`. This is the seam the design's "forward note" reserved.

Change the struct + constructor region to:

```go
type Service struct {
	repo      repo.StyleProfileRepo
	onSaved   func(ctx context.Context, userID uuid.UUID)
}

func New(r repo.StyleProfileRepo) *Service { return &Service{repo: r} }

// SetOnSaved registers a callback invoked after a profile is successfully
// saved (used to invalidate the recommendation cache). Optional; nil-safe.
func (s *Service) SetOnSaved(fn func(ctx context.Context, userID uuid.UUID)) { s.onSaved = fn }
```

At the END of `Save`, replace the final `return s.repo.Upsert(...)` with:

```go
	view, err := s.repo.Upsert(ctx, domain.UpsertParams{
		UserID:      userID,
		StyleTagIDs: ids,
		BudgetMin:   req.BudgetMin,
		BudgetMax:   req.BudgetMax,
	})
	if err != nil {
		return nil, err
	}
	if s.onSaved != nil {
		s.onSaved(ctx, userID)
	}
	return view, nil
```

Confirm `uuid` is imported in this file (it already is, via the parse loop). Run `go test ./internal/styleprofile/...` — existing tests must still pass (callback is nil in those).

- [ ] **Step 2: Construct the recommendation module in `main.go`**

Read `main.go`. Add aliased imports near the other modules:

```go
	recommendationhandler "github.com/wearwhere/wearwhere_be/internal/recommendation/handler"
	recommendationrepo "github.com/wearwhere/wearwhere_be/internal/recommendation/repo"
	recommendationservice "github.com/wearwhere/wearwhere_be/internal/recommendation/service"
```

Where modules are constructed (near `styleProfileSvc`), add — note the redis client variable: find how the existing code builds it (search for `redis.New(` / `redisClient` / the `cfg.Redis` usage) and reuse that variable name (shown here as `redisClient`):

```go
	recSvc := recommendationservice.New(
		recommendationrepo.NewCandidatePG(pgPool),
		recommendationrepo.NewSignalPG(pgPool),
		styleProfileSvc, // satisfies ProfileLoader via LoadProfile
		recommendationservice.NewRedisCache(redisClient),
		recommendationservice.Config{
			DefaultLimit:  cfg.Recommendation.DefaultLimit,
			MaxLimit:      cfg.Recommendation.MaxLimit,
			CandidatePool: cfg.Recommendation.CandidatePool,
		},
	)
	recommendationHandler := recommendationhandler.New(recSvc)

	// Bust the recommendation cache when a user updates their style profile.
	styleProfileSvc.SetOnSaved(func(ctx context.Context, userID uuid.UUID) {
		if err := recSvc.Invalidate(ctx, userID); err != nil {
			log.Printf("recommendation: invalidate after profile save failed for %s: %v", userID, err)
		}
	})
```

> If the redis client variable has a different name in main.go, use the real one. If `context`/`log`/`uuid` are not yet imported in main.go, add them.

- [ ] **Step 3: Mount on the customer group**

In the `customerGroup` block (after `styleprofilehandler.Mount(customerGroup, styleProfileHandler)`), add:

```go
	recommendationhandler.Mount(customerGroup, recommendationHandler)
```

- [ ] **Step 4: Verify**

Run:
- `go build ./cmd/api/...` and `go build ./...` (expect success)
- `go test ./internal/recommendation/... ./internal/styleprofile/...` (unit tests PASS)

- [ ] **Step 5: Commit**

```bash
git add internal/styleprofile/service/service.go cmd/api/main.go
git commit -m "feat(recommendation): wire module + style-profile cache-invalidation seam"
```

---

## Self-Review

**Spec coverage (§4 of the design):**
- `GET /me/recommendations` with `limit` (default 20, max 50) → Task 7 handler + Task 6 `ResolveLimit`. ✓
- Response `{items, source, onboarding_prompt}` → Task 2 DTO. ✓
- Signals: profile (tags+budget), followed brands, purchases, wishlist categories → Task 3 + Task 6 `loadSignals`. ✓
- Scoring: tag overlap (highest), brand affinity, budget fit (+/-), category affinity → Task 4 `ScoreCandidate`. ✓
- Candidate set: active, in-stock, exclude purchased → Task 3 SQL (`status='active'`, `in_stock`) + Task 4 `Rank` purchased filter. ✓
- Discovery mixing, deterministic (no randomness) → Task 4 `Rank` + `roundRobinByBrand`, with a determinism test. ✓
- Avoid out-of-stock → Task 3 `vp.in_stock`. ✓
- Cold start → trending + `onboarding_prompt:true`, `source:"trending"` → Task 6. ✓
- "Update daily" via Redis daily cache, invalidate on profile change → Task 5 + Task 8 seam. ✓
- Order-change invalidation → **documented deviation**: deferred to follow-up; the 24h TTL bounds staleness and purchased items are excluded on the next daily recompute. (Approved scope decision.)
- Reuse catalog read-path, ProductCard shape → Task 3 SQL mirrors the catalog query; `RecProductCard` mirrors `ProductSummary`. ✓

**Placeholder scan:** Task 7 Step 1 intentionally shows a wrong-then-corrected interface to steer the implementer to `context.Context`; the corrected block is the one to use, and the guard line is explicitly flagged for removal. No unfilled TODOs remain.

**Type consistency:** `Candidate`, `UserSignals`, `RecProductCard`, `RecommendationsResponse` (Task 2) are used identically across Tasks 4–7. `CandidateRepo.Candidates`, `SignalRepo.{FollowedBrandIDs,PurchasedProductIDs,AffinityCategoryIDs}` (Task 3) match the fakes in Task 6. `Cache` interface (Task 5) matches `fakeCache` (Task 6) and `RedisCache`. `ProfileLoader.LoadProfile` matches `styleprofileservice.Service.LoadProfile` (returns `*spdomain.StyleProfileView`). `service.New(cand, sig, profile, cache, cfg)` matches the wiring in Task 8. `Recommender.Recommend` (Task 7) matches `Service.Recommend` (Task 6).

**Follow-up to file as an issue:** order-placement cache invalidation (wire `recSvc.Invalidate` into the order-creation success path).
