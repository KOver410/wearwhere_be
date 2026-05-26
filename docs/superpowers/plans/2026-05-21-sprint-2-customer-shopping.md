# Sprint 2 — Customer Shopping Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement Sprint 2 of the WearWhere shopping system: customer-side cart (UC14/15), wishlist (UC16), and shipping address book. Order/checkout deferred to Sprint 3.

**Architecture:** Three new flat domain modules — `internal/cart`, `internal/wishlist`, `internal/customeraddr` — mirroring Sprint 1's `brand/`, `product/` layout. All endpoints under `/api/v1/me/*`, gated by `RequireAuth + RequireRole(customer)`. Cart uses Postgres UPSERT with `ON CONFLICT (user_id, variant_id)` for idempotent add+increment. Wishlist uses composite PK `(user_id, product_id)`. Customer addresses follow Sprint 1's brand_addresses pattern with `is_default` swap inside a transaction. Stock reservation and wishlist notifications are explicitly deferred.

**Tech Stack:** Go 1.x, Gin, pgx/v5, go-playground/validator v10, golang-migrate, Postgres 16, Redis 7.

**Spec reference:** [docs/superpowers/specs/2026-05-21-sprint-2-customer-shopping-design.md](../specs/2026-05-21-sprint-2-customer-shopping-design.md)

---

## File Structure

### Created

```
db/migrations/
  000018_create_cart_items.{up,down}.sql
  000019_create_wishlist_items.{up,down}.sql
  000020_create_customer_addresses.{up,down}.sql

internal/customeraddr/
  domain/address.go     — CustomerAddress struct
  domain/errors.go      — AppErrors: ErrAddressNotFound, ErrInvalidPhone
  domain/dto.go         — CreateAddressRequest, UpdateAddressRequest, response shapes
  repo/repo.go          — AddressRepo interface + ErrNotFound + DBTX
  repo/customer_address_pg.go
  repo/customer_address_pg_test.go    (integration)
  service/service.go    — CustomerAddressService
  service/service_test.go             (unit)
  handler/handler.go    — HTTP handler
  handler/handler_test.go             (httptest)
  handler/routes.go     — Mount(group, h)

internal/wishlist/
  domain/wishlist.go    — WishlistItem struct + denormalized ListItem (with product/brand)
  domain/errors.go
  domain/dto.go
  repo/repo.go          — WishlistRepo + ErrNotFound + DBTX
  repo/wishlist_pg.go
  repo/wishlist_pg_test.go            (integration)
  service/service.go
  service/service_test.go             (unit)
  handler/handler.go
  handler/handler_test.go             (httptest)
  handler/routes.go

internal/cart/
  domain/cart.go        — CartItem struct + denormalized CartItemView (with variant/product/brand)
  domain/errors.go      — ErrCartItemNotFound, ErrVariantUnavailable, ErrOutOfStock, ErrQtyExceedsMax
  domain/dto.go         — AddToCartRequest, UpdateCartItemRequest, CartResponse, CartSummary
  repo/repo.go          — CartRepo + ErrNotFound + DBTX
  repo/cart_pg.go
  repo/cart_pg_test.go                (integration)
  service/service.go
  service/service_test.go             (unit)
  handler/handler.go
  handler/handler_test.go             (httptest)
  handler/routes.go
```

### Modified

```
internal/product/repo/repo.go         — add VariantRepo.FindForPurchase(ctx, id) signature
internal/product/repo/variant_pg.go   — implement FindForPurchase joining product
internal/testfixtures/fixtures.go     — add SeedCustomer, SeedCartItem, SeedWishlistItem, SeedCustomerAddress
cmd/api/main.go                       — wire 3 new repos/services/handlers + mount /me routes
cmd/api/main_test.go                  — append Sprint 2 customer scenario to existing E2E
```

---

## Conventions used in this plan

- **Repo pattern**: each repo struct holds `db DBTX` where `DBTX` is satisfied by both `*pgxpool.Pool` and `pgx.Tx`. Each repo package declares its own `DBTX` interface (matches Sprint 1).
- **Error pattern**: domain-level errors are `*httpx.AppError` variables defined in `internal/<module>/domain/errors.go` (matches `internal/product/domain/errors.go`). Repo-level "not found" is `ErrNotFound` in the repo package; service translates to a domain `AppError`.
- **Commit cadence**: one commit per task. Commit message format: `feat(<module>): <summary>`, `chore(db): <summary>`, `test(<module>): <summary>`, `fix(<module>): <summary>`.
- **Build tags**: integration tests use `//go:build integration` so `go test ./...` runs unit-only by default.
- **Go imports**: group standard → third-party → project, separated by blank line.
- **Context propagation**: every repo/service method takes `ctx context.Context` as the first argument.
- **IDOR safety**: every UPDATE/DELETE filters by `user_id` in the same SQL statement; `RowsAffected() == 0` → return `ErrNotFound`.

---

## Phase A — Foundation: Migrations + Customer Address Book (Tasks 1–10)

### Task 1: Migration 000018 — cart_items

**Files:**
- Create: `db/migrations/000018_create_cart_items.up.sql`
- Create: `db/migrations/000018_create_cart_items.down.sql`

- [ ] **Step 1: Create up migration**

```sql
-- db/migrations/000018_create_cart_items.up.sql
CREATE TABLE cart_items (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    variant_id          UUID NOT NULL REFERENCES product_variants(id) ON DELETE CASCADE,
    qty                 INT  NOT NULL CHECK (qty BETWEEN 1 AND 10),
    price_snapshot      NUMERIC(12,2) NOT NULL CHECK (price_snapshot > 0),
    currency_snapshot   CHAR(3) NOT NULL DEFAULT 'VND',
    added_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX cart_items_user_variant_uniq
    ON cart_items (user_id, variant_id);

CREATE INDEX cart_items_user_idx
    ON cart_items (user_id);
```

- [ ] **Step 2: Create down migration**

```sql
-- db/migrations/000018_create_cart_items.down.sql
DROP INDEX IF EXISTS cart_items_user_idx;
DROP INDEX IF EXISTS cart_items_user_variant_uniq;
DROP TABLE IF EXISTS cart_items;
```

- [ ] **Step 3: Run migration on dev DB**

Run: `make migrate-up`
Expected: `... migration: 18/u create_cart_items (X ms)` with no errors.

- [ ] **Step 4: Verify schema**

Run: `docker compose exec -T postgres psql -U wearwhere -d wearwhere -c "\d cart_items"`
Expected: table with 8 columns, 2 indexes (cart_items_user_idx + cart_items_user_variant_uniq).

- [ ] **Step 5: Commit**

```bash
git add db/migrations/000018_create_cart_items.up.sql db/migrations/000018_create_cart_items.down.sql
git commit -m "chore(db): add cart_items migration"
```

---

### Task 2: Migration 000019 — wishlist_items

**Files:**
- Create: `db/migrations/000019_create_wishlist_items.up.sql`
- Create: `db/migrations/000019_create_wishlist_items.down.sql`

- [ ] **Step 1: Create up migration**

```sql
-- db/migrations/000019_create_wishlist_items.up.sql
CREATE TABLE wishlist_items (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    added_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, product_id)
);

CREATE INDEX wishlist_items_user_added_idx
    ON wishlist_items (user_id, added_at DESC);
```

- [ ] **Step 2: Create down migration**

```sql
-- db/migrations/000019_create_wishlist_items.down.sql
DROP INDEX IF EXISTS wishlist_items_user_added_idx;
DROP TABLE IF EXISTS wishlist_items;
```

- [ ] **Step 3: Run migration**

Run: `make migrate-up`
Expected: `19/u create_wishlist_items` applied.

- [ ] **Step 4: Verify**

Run: `docker compose exec -T postgres psql -U wearwhere -d wearwhere -c "\d wishlist_items"`
Expected: PK on (user_id, product_id) + the added_at index.

- [ ] **Step 5: Commit**

```bash
git add db/migrations/000019_create_wishlist_items.up.sql db/migrations/000019_create_wishlist_items.down.sql
git commit -m "chore(db): add wishlist_items migration"
```

---

### Task 3: Migration 000020 — customer_addresses

**Files:**
- Create: `db/migrations/000020_create_customer_addresses.up.sql`
- Create: `db/migrations/000020_create_customer_addresses.down.sql`

- [ ] **Step 1: Create up migration**

```sql
-- db/migrations/000020_create_customer_addresses.up.sql
CREATE TABLE customer_addresses (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label           VARCHAR(40)  NOT NULL,
    recipient_name  VARCHAR(120) NOT NULL,
    recipient_phone VARCHAR(20)  NOT NULL,
    address_line    VARCHAR(255) NOT NULL,
    ward            VARCHAR(80)  NOT NULL,
    district        VARCHAR(80)  NOT NULL,
    city            VARCHAR(80)  NOT NULL,
    country         CHAR(2)      NOT NULL DEFAULT 'VN',
    postal_code     VARCHAR(20),
    note            VARCHAR(255),
    is_default      BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX customer_addresses_user_default_uniq
    ON customer_addresses (user_id)
    WHERE is_default AND deleted_at IS NULL;

CREATE INDEX customer_addresses_user_idx
    ON customer_addresses (user_id, deleted_at);
```

- [ ] **Step 2: Create down migration**

```sql
-- db/migrations/000020_create_customer_addresses.down.sql
DROP INDEX IF EXISTS customer_addresses_user_idx;
DROP INDEX IF EXISTS customer_addresses_user_default_uniq;
DROP TABLE IF EXISTS customer_addresses;
```

- [ ] **Step 3: Run migration**

Run: `make migrate-up`
Expected: `20/u create_customer_addresses` applied.

- [ ] **Step 4: Verify partial unique index**

Run:
```bash
docker compose exec -T postgres psql -U wearwhere -d wearwhere -c "\d customer_addresses" \
 && docker compose exec -T postgres psql -U wearwhere -d wearwhere -c "SELECT indexdef FROM pg_indexes WHERE tablename='customer_addresses';"
```
Expected: `customer_addresses_user_default_uniq` shows `WHERE is_default AND (deleted_at IS NULL)`.

- [ ] **Step 5: Commit**

```bash
git add db/migrations/000020_create_customer_addresses.up.sql db/migrations/000020_create_customer_addresses.down.sql
git commit -m "chore(db): add customer_addresses migration"
```

---

### Task 4: Reset test DB and confirm full migration chain

**Files:** none (operational task)

- [ ] **Step 1: Drop and recreate test DB**

Run: `make test-db-reset`
Expected: `wearwhere_test` recreated, all 20 migrations applied (18–20 are new). No errors.

- [ ] **Step 2: Confirm migrations table**

Run: `docker compose exec -T postgres psql -U wearwhere -d wearwhere_test -c "SELECT version, dirty FROM schema_migrations;"`
Expected: `version=20`, `dirty=f`.

- [ ] **Step 3: No commit needed** (this task verifies state, no files changed).

---

### Task 5: Extend testfixtures with customer/cart/wishlist/address seeds

**Files:**
- Modify: `internal/testfixtures/fixtures.go`

- [ ] **Step 1: Add SeedCustomer + cart/wishlist/address fixtures**

Append to `internal/testfixtures/fixtures.go` (after the existing `SeedVariant` function — line ~161):

```go
// SeedCustomer is a thin wrapper around SeedUser with role="customer" for readability.
func SeedCustomer(t *testing.T, db DBTX) SeededUser {
    t.Helper()
    return SeedUser(t, db, "customer")
}

// SeededCartItem is the minimal info callers need after seeding a cart row.
type SeededCartItem struct {
    ID            uuid.UUID
    UserID        uuid.UUID
    VariantID     uuid.UUID
    Qty           int
    PriceSnapshot float64
}

// SeedCartItem inserts a cart_items row. priceSnapshot must equal the variant price
// the caller passed to SeedVariant to mimic real add-to-cart flow.
func SeedCartItem(t *testing.T, db DBTX, userID, variantID uuid.UUID, qty int, priceSnapshot float64) SeededCartItem {
    t.Helper()
    id := uuid.New()
    _, err := db.Exec(context.Background(),
        `INSERT INTO cart_items (id, user_id, variant_id, qty, price_snapshot, currency_snapshot)
         VALUES ($1, $2, $3, $4, $5, 'VND')`,
        id, userID, variantID, qty, priceSnapshot)
    if err != nil {
        t.Fatalf("seed cart_item: %v", err)
    }
    return SeededCartItem{ID: id, UserID: userID, VariantID: variantID, Qty: qty, PriceSnapshot: priceSnapshot}
}

// SeedWishlistItem inserts a wishlist_items row.
func SeedWishlistItem(t *testing.T, db DBTX, userID, productID uuid.UUID) {
    t.Helper()
    _, err := db.Exec(context.Background(),
        `INSERT INTO wishlist_items (user_id, product_id) VALUES ($1, $2)`,
        userID, productID)
    if err != nil {
        t.Fatalf("seed wishlist_item: %v", err)
    }
}

// CustomerAddressOpts overrides defaults for SeedCustomerAddress.
type CustomerAddressOpts struct {
    Label          string
    RecipientName  string
    RecipientPhone string
    IsDefault      bool
}

type SeededCustomerAddress struct {
    ID        uuid.UUID
    UserID    uuid.UUID
    IsDefault bool
}

// SeedCustomerAddress inserts a customer_addresses row with sane Vietnam defaults.
func SeedCustomerAddress(t *testing.T, db DBTX, userID uuid.UUID, opts CustomerAddressOpts) SeededCustomerAddress {
    t.Helper()
    if opts.Label == "" {
        opts.Label = "Nhà"
    }
    if opts.RecipientName == "" {
        opts.RecipientName = "Người Nhận"
    }
    if opts.RecipientPhone == "" {
        opts.RecipientPhone = "+84901234567"
    }
    id := uuid.New()
    _, err := db.Exec(context.Background(),
        `INSERT INTO customer_addresses
           (id, user_id, label, recipient_name, recipient_phone,
            address_line, ward, district, city, country, is_default)
         VALUES ($1,$2,$3,$4,$5,'123 Lê Lợi','Bến Nghé','Quận 1','TP HCM','VN',$6)`,
        id, userID, opts.Label, opts.RecipientName, opts.RecipientPhone, opts.IsDefault)
    if err != nil {
        t.Fatalf("seed customer_address: %v", err)
    }
    return SeededCustomerAddress{ID: id, UserID: userID, IsDefault: opts.IsDefault}
}
```

- [ ] **Step 2: Verify compiles**

Run: `go build ./internal/testfixtures/...`
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/testfixtures/fixtures.go
git commit -m "test(fixtures): add customer/cart/wishlist/address seeds"
```

---

### Task 6: customeraddr domain — types, errors, DTOs

**Files:**
- Create: `internal/customeraddr/domain/address.go`
- Create: `internal/customeraddr/domain/errors.go`
- Create: `internal/customeraddr/domain/dto.go`

- [ ] **Step 1: Write `domain/address.go`**

```go
// Package domain defines the customer-address aggregate.
package domain

import (
    "time"

    "github.com/google/uuid"
)

type CustomerAddress struct {
    ID             uuid.UUID
    UserID         uuid.UUID
    Label          string
    RecipientName  string
    RecipientPhone string
    AddressLine    string
    Ward           string
    District       string
    City           string
    Country        string
    PostalCode     *string
    Note           *string
    IsDefault      bool
    CreatedAt      time.Time
    UpdatedAt      time.Time
    DeletedAt      *time.Time
}
```

- [ ] **Step 2: Write `domain/errors.go`**

```go
package domain

import (
    "net/http"

    "github.com/wearwhere/wearwhere_be/pkg/httpx"
)

var (
    ErrAddressNotFound = &httpx.AppError{
        Status:  http.StatusNotFound,
        Code:    "ADDRESS_NOT_FOUND",
        Message: "Address not found",
    }
    ErrInvalidPhone = &httpx.AppError{
        Status:  http.StatusBadRequest,
        Code:    "INVALID_PHONE",
        Message: "Phone must be in E.164 format",
    }
)
```

- [ ] **Step 3: Write `domain/dto.go`**

```go
package domain

type CreateAddressRequest struct {
    Label          string  `json:"label"           binding:"required,max=40"`
    RecipientName  string  `json:"recipient_name"  binding:"required,min=2,max=120"`
    RecipientPhone string  `json:"recipient_phone" binding:"required,e164"`
    AddressLine    string  `json:"address_line"    binding:"required,max=255"`
    Ward           string  `json:"ward"            binding:"required,max=80"`
    District       string  `json:"district"        binding:"required,max=80"`
    City           string  `json:"city"            binding:"required,max=80"`
    Country        string  `json:"country"         binding:"omitempty,iso3166_1_alpha2"`
    PostalCode     *string `json:"postal_code"     binding:"omitempty,max=20"`
    Note           *string `json:"note"            binding:"omitempty,max=255"`
    IsDefault      bool    `json:"is_default"`
}

type UpdateAddressRequest struct {
    Label          *string `json:"label"           binding:"omitempty,max=40"`
    RecipientName  *string `json:"recipient_name"  binding:"omitempty,min=2,max=120"`
    RecipientPhone *string `json:"recipient_phone" binding:"omitempty,e164"`
    AddressLine    *string `json:"address_line"    binding:"omitempty,max=255"`
    Ward           *string `json:"ward"            binding:"omitempty,max=80"`
    District       *string `json:"district"        binding:"omitempty,max=80"`
    City           *string `json:"city"            binding:"omitempty,max=80"`
    Country        *string `json:"country"         binding:"omitempty,iso3166_1_alpha2"`
    PostalCode     *string `json:"postal_code"     binding:"omitempty,max=20"`
    Note           *string `json:"note"            binding:"omitempty,max=255"`
    IsDefault      *bool   `json:"is_default"`
}

type AddressResponse struct {
    ID             string  `json:"id"`
    Label          string  `json:"label"`
    RecipientName  string  `json:"recipient_name"`
    RecipientPhone string  `json:"recipient_phone"`
    AddressLine    string  `json:"address_line"`
    Ward           string  `json:"ward"`
    District       string  `json:"district"`
    City           string  `json:"city"`
    Country        string  `json:"country"`
    PostalCode     *string `json:"postal_code,omitempty"`
    Note           *string `json:"note,omitempty"`
    IsDefault      bool    `json:"is_default"`
    CreatedAt      string  `json:"created_at"`
    UpdatedAt      string  `json:"updated_at"`
}

func ToAddressResponse(a *CustomerAddress) AddressResponse {
    return AddressResponse{
        ID:             a.ID.String(),
        Label:          a.Label,
        RecipientName:  a.RecipientName,
        RecipientPhone: a.RecipientPhone,
        AddressLine:    a.AddressLine,
        Ward:           a.Ward,
        District:       a.District,
        City:           a.City,
        Country:        a.Country,
        PostalCode:     a.PostalCode,
        Note:           a.Note,
        IsDefault:      a.IsDefault,
        CreatedAt:      a.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
        UpdatedAt:      a.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
    }
}
```

- [ ] **Step 4: Verify compiles**

Run: `go build ./internal/customeraddr/...`
Expected: no output (success).

- [ ] **Step 5: Commit**

```bash
git add internal/customeraddr/domain/
git commit -m "feat(customeraddr): domain types, errors, DTOs"
```

---

### Task 7: customeraddr repo — PG implementation + integration tests

**Files:**
- Create: `internal/customeraddr/repo/repo.go`
- Create: `internal/customeraddr/repo/customer_address_pg.go`
- Create: `internal/customeraddr/repo/customer_address_pg_test.go`

- [ ] **Step 1: Write `repo/repo.go` (interface + DBTX + ErrNotFound)**

```go
// Package repo provides persistence for customer addresses.
package repo

import (
    "context"
    "errors"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgconn"

    "github.com/wearwhere/wearwhere_be/internal/customeraddr/domain"
)

var ErrNotFound = errors.New("customeraddr: not found")

type DBTX interface {
    Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
    Begin(ctx context.Context) (pgx.Tx, error)
}

type AddressRepo interface {
    List(ctx context.Context, userID uuid.UUID) ([]*domain.CustomerAddress, error)
    FindByID(ctx context.Context, id, userID uuid.UUID) (*domain.CustomerAddress, error)
    Create(ctx context.Context, userID uuid.UUID, req *domain.CreateAddressRequest) (*domain.CustomerAddress, error)
    Update(ctx context.Context, id, userID uuid.UUID, req *domain.UpdateAddressRequest) (*domain.CustomerAddress, error)
    SoftDelete(ctx context.Context, id, userID uuid.UUID) error
}
```

- [ ] **Step 2: Write `repo/customer_address_pg.go`**

```go
package repo

import (
    "context"
    "errors"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"

    "github.com/wearwhere/wearwhere_be/internal/customeraddr/domain"
)

type AddressPG struct{ db DBTX }

func NewAddressPG(db DBTX) *AddressPG { return &AddressPG{db: db} }

const addrCols = `id, user_id, label, recipient_name, recipient_phone,
                  address_line, ward, district, city, country, postal_code, note,
                  is_default, created_at, updated_at, deleted_at`

func scanAddress(row pgx.Row) (*domain.CustomerAddress, error) {
    var a domain.CustomerAddress
    err := row.Scan(
        &a.ID, &a.UserID, &a.Label, &a.RecipientName, &a.RecipientPhone,
        &a.AddressLine, &a.Ward, &a.District, &a.City, &a.Country, &a.PostalCode, &a.Note,
        &a.IsDefault, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
    )
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, ErrNotFound
        }
        return nil, err
    }
    return &a, nil
}

func (r *AddressPG) List(ctx context.Context, userID uuid.UUID) ([]*domain.CustomerAddress, error) {
    rows, err := r.db.Query(ctx,
        `SELECT `+addrCols+` FROM customer_addresses
         WHERE user_id=$1 AND deleted_at IS NULL
         ORDER BY is_default DESC, created_at ASC`, userID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []*domain.CustomerAddress
    for rows.Next() {
        a, err := scanAddress(rows)
        if err != nil {
            return nil, err
        }
        out = append(out, a)
    }
    return out, rows.Err()
}

func (r *AddressPG) FindByID(ctx context.Context, id, userID uuid.UUID) (*domain.CustomerAddress, error) {
    return scanAddress(r.db.QueryRow(ctx,
        `SELECT `+addrCols+` FROM customer_addresses
         WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL`, id, userID))
}

func (r *AddressPG) Create(ctx context.Context, userID uuid.UUID, req *domain.CreateAddressRequest) (*domain.CustomerAddress, error) {
    country := req.Country
    if country == "" {
        country = "VN"
    }

    tx, err := r.db.Begin(ctx)
    if err != nil {
        return nil, err
    }
    defer tx.Rollback(ctx)

    // First-address auto-default OR explicit IsDefault → unset siblings.
    var hasExisting bool
    if err := tx.QueryRow(ctx,
        `SELECT EXISTS(SELECT 1 FROM customer_addresses
         WHERE user_id=$1 AND deleted_at IS NULL)`, userID).Scan(&hasExisting); err != nil {
        return nil, err
    }
    isDefault := req.IsDefault || !hasExisting
    if isDefault && hasExisting {
        if _, err := tx.Exec(ctx,
            `UPDATE customer_addresses SET is_default=FALSE, updated_at=NOW()
             WHERE user_id=$1 AND is_default AND deleted_at IS NULL`, userID); err != nil {
            return nil, err
        }
    }

    a, err := scanAddress(tx.QueryRow(ctx,
        `INSERT INTO customer_addresses
           (user_id, label, recipient_name, recipient_phone, address_line,
            ward, district, city, country, postal_code, note, is_default)
         VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
         RETURNING `+addrCols,
        userID, req.Label, req.RecipientName, req.RecipientPhone, req.AddressLine,
        req.Ward, req.District, req.City, country, req.PostalCode, req.Note, isDefault))
    if err != nil {
        return nil, err
    }
    if err := tx.Commit(ctx); err != nil {
        return nil, err
    }
    return a, nil
}

func (r *AddressPG) Update(ctx context.Context, id, userID uuid.UUID, req *domain.UpdateAddressRequest) (*domain.CustomerAddress, error) {
    tx, err := r.db.Begin(ctx)
    if err != nil {
        return nil, err
    }
    defer tx.Rollback(ctx)

    if req.IsDefault != nil && *req.IsDefault {
        if _, err := tx.Exec(ctx,
            `UPDATE customer_addresses SET is_default=FALSE, updated_at=NOW()
             WHERE user_id=$1 AND id<>$2 AND is_default AND deleted_at IS NULL`,
            userID, id); err != nil {
            return nil, err
        }
    }

    a, err := scanAddress(tx.QueryRow(ctx,
        `UPDATE customer_addresses SET
            label           = COALESCE($3, label),
            recipient_name  = COALESCE($4, recipient_name),
            recipient_phone = COALESCE($5, recipient_phone),
            address_line    = COALESCE($6, address_line),
            ward            = COALESCE($7, ward),
            district        = COALESCE($8, district),
            city            = COALESCE($9, city),
            country         = COALESCE($10, country),
            postal_code     = COALESCE($11, postal_code),
            note            = COALESCE($12, note),
            is_default      = COALESCE($13, is_default),
            updated_at      = NOW()
         WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL
         RETURNING `+addrCols,
        id, userID, req.Label, req.RecipientName, req.RecipientPhone, req.AddressLine,
        req.Ward, req.District, req.City, req.Country, req.PostalCode, req.Note, req.IsDefault))
    if err != nil {
        return nil, err
    }
    if err := tx.Commit(ctx); err != nil {
        return nil, err
    }
    return a, nil
}

// SoftDelete marks the row deleted. If the deleted row was the default, promotes
// the oldest remaining live address to default (atomic in one tx). We use
// SELECT ... FOR UPDATE first to capture is_default before zeroing it (so the
// promotion branch knows whether to fire).
func (r *AddressPG) SoftDelete(ctx context.Context, id, userID uuid.UUID) error {
    tx, err := r.db.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)

    var wasDefault bool
    err = tx.QueryRow(ctx,
        `SELECT is_default FROM customer_addresses
         WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL
         FOR UPDATE`, id, userID).Scan(&wasDefault)
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return ErrNotFound
        }
        return err
    }
    if _, err := tx.Exec(ctx,
        `UPDATE customer_addresses
            SET deleted_at=NOW(), is_default=FALSE, updated_at=NOW()
          WHERE id=$1 AND user_id=$2`, id, userID); err != nil {
        return err
    }
    if wasDefault {
        if _, err := tx.Exec(ctx,
            `UPDATE customer_addresses SET is_default=TRUE, updated_at=NOW()
             WHERE id = (
               SELECT id FROM customer_addresses
               WHERE user_id=$1 AND deleted_at IS NULL
               ORDER BY created_at ASC LIMIT 1
             )`, userID); err != nil {
            return err
        }
    }
    return tx.Commit(ctx)
}
```

Use the two-step version. Delete the broken draft.

- [ ] **Step 3: Write integration tests `repo/customer_address_pg_test.go`**

```go
//go:build integration

package repo_test

import (
    "context"
    "os"
    "testing"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/stretchr/testify/require"

    "github.com/wearwhere/wearwhere_be/internal/customeraddr/domain"
    "github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
    "github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
    dsn := os.Getenv("TEST_DATABASE_URL")
    if dsn == "" {
        panic("TEST_DATABASE_URL required for integration tests")
    }
    var err error
    pool, err = pgxpool.New(context.Background(), dsn)
    if err != nil {
        panic(err)
    }
    defer pool.Close()
    os.Exit(m.Run())
}

func TestAddressPG_Create_FirstAddressIsDefault(t *testing.T) {
    tx := testfixtures.BeginTx(t, pool)
    user := testfixtures.SeedCustomer(t, tx)
    r := repo.NewAddressPG(tx)

    addr, err := r.Create(context.Background(), user.ID, &domain.CreateAddressRequest{
        Label: "Nhà", RecipientName: "X", RecipientPhone: "+84901234567",
        AddressLine: "1 A", Ward: "Phường 1", District: "Quận 1", City: "TP HCM",
        IsDefault: false, // explicitly false; should still become default
    })
    require.NoError(t, err)
    require.True(t, addr.IsDefault)
}

func TestAddressPG_Create_DefaultSwapsExisting(t *testing.T) {
    tx := testfixtures.BeginTx(t, pool)
    user := testfixtures.SeedCustomer(t, tx)
    first := testfixtures.SeedCustomerAddress(t, tx, user.ID, testfixtures.CustomerAddressOpts{IsDefault: true})
    r := repo.NewAddressPG(tx)

    second, err := r.Create(context.Background(), user.ID, &domain.CreateAddressRequest{
        Label: "Office", RecipientName: "X", RecipientPhone: "+84901234567",
        AddressLine: "2 B", Ward: "P 2", District: "Q 2", City: "TP HCM",
        IsDefault: true,
    })
    require.NoError(t, err)
    require.True(t, second.IsDefault)

    refetchedFirst, err := r.FindByID(context.Background(), first.ID, user.ID)
    require.NoError(t, err)
    require.False(t, refetchedFirst.IsDefault)
}

func TestAddressPG_SoftDelete_PromotesOldestRemaining(t *testing.T) {
    tx := testfixtures.BeginTx(t, pool)
    user := testfixtures.SeedCustomer(t, tx)
    older := testfixtures.SeedCustomerAddress(t, tx, user.ID, testfixtures.CustomerAddressOpts{IsDefault: false})
    defaultAddr := testfixtures.SeedCustomerAddress(t, tx, user.ID, testfixtures.CustomerAddressOpts{IsDefault: true})
    r := repo.NewAddressPG(tx)

    require.NoError(t, r.SoftDelete(context.Background(), defaultAddr.ID, user.ID))

    promoted, err := r.FindByID(context.Background(), older.ID, user.ID)
    require.NoError(t, err)
    require.True(t, promoted.IsDefault)
}

func TestAddressPG_IDOR_OtherUserCannotFind(t *testing.T) {
    tx := testfixtures.BeginTx(t, pool)
    owner := testfixtures.SeedCustomer(t, tx)
    intruder := testfixtures.SeedCustomer(t, tx)
    seeded := testfixtures.SeedCustomerAddress(t, tx, owner.ID, testfixtures.CustomerAddressOpts{})
    r := repo.NewAddressPG(tx)

    _, err := r.FindByID(context.Background(), seeded.ID, intruder.ID)
    require.ErrorIs(t, err, repo.ErrNotFound)
}
```

- [ ] **Step 4: Run integration tests**

Run: `make test-integration -- -run TestAddressPG`
Or: `TEST_DATABASE_URL="postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable" go test -tags=integration -v ./internal/customeraddr/repo/...`
Expected: 4 PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/customeraddr/repo/
git commit -m "feat(customeraddr): pg repo with default swap + soft-delete promotion"
```

---

### Task 8: customeraddr service + unit tests

**Files:**
- Create: `internal/customeraddr/service/service.go`
- Create: `internal/customeraddr/service/service_test.go`

- [ ] **Step 1: Write service**

```go
// Package service implements customer-address business rules.
package service

import (
    "context"
    "errors"

    "github.com/google/uuid"

    "github.com/wearwhere/wearwhere_be/internal/customeraddr/domain"
    "github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
)

type CustomerAddressService struct {
    repo repo.AddressRepo
}

func New(r repo.AddressRepo) *CustomerAddressService { return &CustomerAddressService{repo: r} }

func (s *CustomerAddressService) List(ctx context.Context, userID uuid.UUID) ([]*domain.CustomerAddress, error) {
    return s.repo.List(ctx, userID)
}

func (s *CustomerAddressService) Get(ctx context.Context, id, userID uuid.UUID) (*domain.CustomerAddress, error) {
    a, err := s.repo.FindByID(ctx, id, userID)
    if errors.Is(err, repo.ErrNotFound) {
        return nil, domain.ErrAddressNotFound
    }
    return a, err
}

func (s *CustomerAddressService) Create(ctx context.Context, userID uuid.UUID, req *domain.CreateAddressRequest) (*domain.CustomerAddress, error) {
    return s.repo.Create(ctx, userID, req)
}

func (s *CustomerAddressService) Update(ctx context.Context, id, userID uuid.UUID, req *domain.UpdateAddressRequest) (*domain.CustomerAddress, error) {
    a, err := s.repo.Update(ctx, id, userID, req)
    if errors.Is(err, repo.ErrNotFound) {
        return nil, domain.ErrAddressNotFound
    }
    return a, err
}

func (s *CustomerAddressService) Delete(ctx context.Context, id, userID uuid.UUID) error {
    if err := s.repo.SoftDelete(ctx, id, userID); err != nil {
        if errors.Is(err, repo.ErrNotFound) {
            return domain.ErrAddressNotFound
        }
        return err
    }
    return nil
}
```

- [ ] **Step 2: Write unit tests with mock repo**

```go
package service_test

import (
    "context"
    "testing"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    "github.com/wearwhere/wearwhere_be/internal/customeraddr/domain"
    "github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
    "github.com/wearwhere/wearwhere_be/internal/customeraddr/service"
)

type mockRepo struct {
    findErr     error
    deleteErr   error
    list        []*domain.CustomerAddress
    findReturns *domain.CustomerAddress
}

func (m *mockRepo) List(_ context.Context, _ uuid.UUID) ([]*domain.CustomerAddress, error) {
    return m.list, nil
}
func (m *mockRepo) FindByID(_ context.Context, _, _ uuid.UUID) (*domain.CustomerAddress, error) {
    return m.findReturns, m.findErr
}
func (m *mockRepo) Create(_ context.Context, _ uuid.UUID, _ *domain.CreateAddressRequest) (*domain.CustomerAddress, error) {
    return &domain.CustomerAddress{IsDefault: true}, nil
}
func (m *mockRepo) Update(_ context.Context, _, _ uuid.UUID, _ *domain.UpdateAddressRequest) (*domain.CustomerAddress, error) {
    return m.findReturns, m.findErr
}
func (m *mockRepo) SoftDelete(_ context.Context, _, _ uuid.UUID) error { return m.deleteErr }

func TestGet_NotFoundMapsToDomainError(t *testing.T) {
    s := service.New(&mockRepo{findErr: repo.ErrNotFound})
    _, err := s.Get(context.Background(), uuid.New(), uuid.New())
    require.ErrorIs(t, err, domain.ErrAddressNotFound)
}

func TestDelete_NotFoundMapsToDomainError(t *testing.T) {
    s := service.New(&mockRepo{deleteErr: repo.ErrNotFound})
    err := s.Delete(context.Background(), uuid.New(), uuid.New())
    require.ErrorIs(t, err, domain.ErrAddressNotFound)
}

func TestCreate_PassesThroughRepo(t *testing.T) {
    s := service.New(&mockRepo{})
    a, err := s.Create(context.Background(), uuid.New(), &domain.CreateAddressRequest{Label: "Nhà"})
    require.NoError(t, err)
    require.True(t, a.IsDefault)
}
```

- [ ] **Step 3: Run unit tests**

Run: `go test -v ./internal/customeraddr/service/...`
Expected: 3 PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/customeraddr/service/
git commit -m "feat(customeraddr): service with repo-error mapping"
```

---

### Task 9: customeraddr handler + routes + httptest

**Files:**
- Create: `internal/customeraddr/handler/handler.go`
- Create: `internal/customeraddr/handler/routes.go`
- Create: `internal/customeraddr/handler/handler_test.go`

- [ ] **Step 1: Write `handler/handler.go`**

```go
package handler

import (
    "errors"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
    "github.com/wearwhere/wearwhere_be/internal/customeraddr/domain"
    "github.com/wearwhere/wearwhere_be/internal/customeraddr/service"
    "github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type Handler struct {
    svc *service.CustomerAddressService
}

func New(s *service.CustomerAddressService) *Handler { return &Handler{svc: s} }

func (h *Handler) userID(c *gin.Context) (uuid.UUID, bool) {
    return authmw.UserID(c)
}

func (h *Handler) List(c *gin.Context) {
    uid, ok := h.userID(c)
    if !ok {
        httpx.ErrorFromApp(c, errors.New("missing user"))
        return
    }
    items, err := h.svc.List(c.Request.Context(), uid)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    out := make([]domain.AddressResponse, 0, len(items))
    for _, a := range items {
        out = append(out, domain.ToAddressResponse(a))
    }
    c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *Handler) Get(c *gin.Context) {
    uid, _ := h.userID(c)
    id, err := uuid.Parse(c.Param("id"))
    if err != nil {
        httpx.ErrorFromApp(c, domain.ErrAddressNotFound)
        return
    }
    a, err := h.svc.Get(c.Request.Context(), id, uid)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    c.JSON(http.StatusOK, domain.ToAddressResponse(a))
}

func (h *Handler) Create(c *gin.Context) {
    uid, _ := h.userID(c)
    var req domain.CreateAddressRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        httpx.ValidationError(c, err)
        return
    }
    a, err := h.svc.Create(c.Request.Context(), uid, &req)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    c.JSON(http.StatusCreated, domain.ToAddressResponse(a))
}

func (h *Handler) Update(c *gin.Context) {
    uid, _ := h.userID(c)
    id, err := uuid.Parse(c.Param("id"))
    if err != nil {
        httpx.ErrorFromApp(c, domain.ErrAddressNotFound)
        return
    }
    var req domain.UpdateAddressRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        httpx.ValidationError(c, err)
        return
    }
    a, err := h.svc.Update(c.Request.Context(), id, uid, &req)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    c.JSON(http.StatusOK, domain.ToAddressResponse(a))
}

func (h *Handler) Delete(c *gin.Context) {
    uid, _ := h.userID(c)
    id, err := uuid.Parse(c.Param("id"))
    if err != nil {
        httpx.ErrorFromApp(c, domain.ErrAddressNotFound)
        return
    }
    if err := h.svc.Delete(c.Request.Context(), id, uid); err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    c.Status(http.StatusNoContent)
}
```

Verify `httpx.ValidationError` exists in `pkg/httpx/response.go`. If not, use `httpx.ErrorFromApp(c, err)` instead with a wrapped validation error. Check via:
`grep -n "func ValidationError" pkg/httpx/response.go`. If absent, replace `httpx.ValidationError(c, err)` with `httpx.Error(c, 400, "VALIDATION_ERROR", err.Error())` matching whatever helper the Sprint 1 handlers used.

- [ ] **Step 2: Write `handler/routes.go`**

```go
package handler

import "github.com/gin-gonic/gin"

// Mount registers customer-address routes onto a group that already has
// RequireAuth + RequireRole(customer) applied (e.g., /me).
func Mount(rg *gin.RouterGroup, h *Handler) {
    rg.GET("/addresses", h.List)
    rg.POST("/addresses", h.Create)
    rg.GET("/addresses/:id", h.Get)
    rg.PATCH("/addresses/:id", h.Update)
    rg.DELETE("/addresses/:id", h.Delete)
}
```

- [ ] **Step 3: Write `handler/handler_test.go` (httptest)**

```go
package handler_test

import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
    "github.com/wearwhere/wearwhere_be/internal/customeraddr/domain"
    "github.com/wearwhere/wearwhere_be/internal/customeraddr/handler"
    "github.com/wearwhere/wearwhere_be/internal/customeraddr/service"
)

// stubRepo implements repo.AddressRepo for handler tests.
type stubRepo struct {
    listReturns []*domain.CustomerAddress
}

func (s *stubRepo) List(_ context.Context, _ uuid.UUID) ([]*domain.CustomerAddress, error) {
    return s.listReturns, nil
}
func (s *stubRepo) FindByID(_ context.Context, id, _ uuid.UUID) (*domain.CustomerAddress, error) {
    return &domain.CustomerAddress{ID: id, Label: "Nhà"}, nil
}
func (s *stubRepo) Create(_ context.Context, _ uuid.UUID, req *domain.CreateAddressRequest) (*domain.CustomerAddress, error) {
    return &domain.CustomerAddress{ID: uuid.New(), Label: req.Label, IsDefault: true}, nil
}
func (s *stubRepo) Update(_ context.Context, id, _ uuid.UUID, _ *domain.UpdateAddressRequest) (*domain.CustomerAddress, error) {
    return &domain.CustomerAddress{ID: id, Label: "Updated"}, nil
}
func (s *stubRepo) SoftDelete(_ context.Context, _, _ uuid.UUID) error { return nil }

func setupRouter(t *testing.T) (*gin.Engine, uuid.UUID) {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    h := handler.New(service.New(&stubRepo{}))
    uid := uuid.New()
    rg := r.Group("/me", func(c *gin.Context) { authmw.SetUserIDForTest(c, uid); c.Next() })
    handler.Mount(rg, h)
    return r, uid
}

func TestCreate_201AndDefault(t *testing.T) {
    r, _ := setupRouter(t)
    body, _ := json.Marshal(domain.CreateAddressRequest{
        Label: "Nhà", RecipientName: "X", RecipientPhone: "+84901234567",
        AddressLine: "1 A", Ward: "P 1", District: "Q 1", City: "TP HCM",
    })
    w := httptest.NewRecorder()
    req, _ := http.NewRequest("POST", "/me/addresses", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    r.ServeHTTP(w, req)
    require.Equal(t, http.StatusCreated, w.Code)
    var resp domain.AddressResponse
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
    require.True(t, resp.IsDefault)
}

func TestDelete_204(t *testing.T) {
    r, _ := setupRouter(t)
    w := httptest.NewRecorder()
    req, _ := http.NewRequest("DELETE", "/me/addresses/"+uuid.New().String(), nil)
    r.ServeHTTP(w, req)
    require.Equal(t, http.StatusNoContent, w.Code)
}
```

- [ ] **Step 4: Run handler tests**

Run: `go test -v ./internal/customeraddr/handler/...`
Expected: 2 PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/customeraddr/handler/
git commit -m "feat(customeraddr): handler + routes with httptest coverage"
```

---

### Task 10: Wire customeraddr into `cmd/api/main.go`

**Files:**
- Modify: `cmd/api/main.go`

- [ ] **Step 1: Add imports**

After the existing `producthandler` import block (around line 33), append:

```go
customeraddrhandler "github.com/wearwhere/wearwhere_be/internal/customeraddr/handler"
customeraddrrepo "github.com/wearwhere/wearwhere_be/internal/customeraddr/repo"
customeraddrservice "github.com/wearwhere/wearwhere_be/internal/customeraddr/service"
```

- [ ] **Step 2: Construct repo/service/handler**

After the existing `styleTagRepo := ...` (around line 79), add:

```go
customerAddrRepo := customeraddrrepo.NewAddressPG(pgPool)
```

After existing service block (around line 109), add:

```go
customerAddrSvc := customeraddrservice.New(customerAddrRepo)
```

After existing handler block (around line 127), add:

```go
customerAddrHandler := customeraddrhandler.New(customerAddrSvc)
```

- [ ] **Step 3: Mount the `/me` route group**

After the `producthandler.MountCatalog(v1, catalogHandler)` line (around 163), append:

```go
customerGroup := v1.Group("/me",
    middleware.RequireAuth(jwtIssuer),
    middleware.RequireRole(authdomain.RoleCustomer),
)
customeraddrhandler.Mount(customerGroup, customerAddrHandler)
```

- [ ] **Step 4: Build**

Run: `go build ./cmd/api`
Expected: success, no errors.

- [ ] **Step 5: Manual smoke (optional, document for human)**

Document in commit message: "Manual smoke: register customer → login → POST /api/v1/me/addresses with valid body → 201". Skip if no live DB.

- [ ] **Step 6: Commit**

```bash
git add cmd/api/main.go
git commit -m "feat(customeraddr): wire customer address routes under /me"
```

---

## Phase B — Wishlist (Tasks 11–15)

### Task 11: wishlist domain + DTOs + errors

**Files:**
- Create: `internal/wishlist/domain/wishlist.go`
- Create: `internal/wishlist/domain/errors.go`
- Create: `internal/wishlist/domain/dto.go`

- [ ] **Step 1: Write `domain/wishlist.go`**

```go
package domain

import (
    "time"

    "github.com/google/uuid"
)

type WishlistItem struct {
    UserID    uuid.UUID
    ProductID uuid.UUID
    AddedAt   time.Time
}

// WishlistItemView is the denormalized row returned by GET /me/wishlist.
type WishlistItemView struct {
    ProductID       uuid.UUID
    ProductSlug     string
    ProductName     string
    PrimaryImageURL *string
    MinPrice        *float64
    BrandID         uuid.UUID
    BrandSlug       string
    BrandName       string
    AddedAt         time.Time
}
```

- [ ] **Step 2: Write `domain/errors.go`**

```go
package domain

import (
    "net/http"

    "github.com/wearwhere/wearwhere_be/pkg/httpx"
)

// ErrProductNotFound mirrors the product module's not-found semantic so
// wishlist add against an unknown/inactive product returns the same code.
var ErrProductNotFound = &httpx.AppError{
    Status:  http.StatusNotFound,
    Code:    "PRODUCT_NOT_FOUND",
    Message: "Product not found or unavailable",
}
```

- [ ] **Step 3: Write `domain/dto.go`**

```go
package domain

type WishlistItemResponse struct {
    ProductID       string   `json:"product_id"`
    ProductSlug     string   `json:"product_slug"`
    ProductName     string   `json:"product_name"`
    PrimaryImageURL *string  `json:"primary_image_url,omitempty"`
    MinPrice        *float64 `json:"min_price,omitempty"`
    Brand           struct {
        ID   string `json:"id"`
        Slug string `json:"slug"`
        Name string `json:"name"`
    } `json:"brand"`
    AddedAt string `json:"added_at"`
}

type WishlistListResponse struct {
    Items      []WishlistItemResponse `json:"items"`
    Pagination struct {
        Page       int  `json:"page"`
        Limit      int  `json:"limit"`
        Total      int  `json:"total"`
        TotalPages int  `json:"total_pages"`
        HasMore    bool `json:"has_more"`
    } `json:"pagination"`
}

type WishlistContainsResponse struct {
    InWishlist map[string]bool `json:"in_wishlist"`
}

type WishlistContainsQuery struct {
    ProductIDs []string `form:"product_ids" binding:"required,min=1,max=60,dive,uuid"`
}

type WishlistListQuery struct {
    Page  int `form:"page,default=1"   binding:"min=1"`
    Limit int `form:"limit,default=24" binding:"min=1,max=60"`
}
```

- [ ] **Step 4: Verify compiles**

Run: `go build ./internal/wishlist/...`
Expected: success.

- [ ] **Step 5: Commit**

```bash
git add internal/wishlist/domain/
git commit -m "feat(wishlist): domain types, errors, DTOs"
```

---

### Task 12: wishlist repo (PG) + integration tests

**Files:**
- Create: `internal/wishlist/repo/repo.go`
- Create: `internal/wishlist/repo/wishlist_pg.go`
- Create: `internal/wishlist/repo/wishlist_pg_test.go`

- [ ] **Step 1: Write `repo/repo.go`**

```go
package repo

import (
    "context"
    "errors"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgconn"

    "github.com/wearwhere/wearwhere_be/internal/wishlist/domain"
)

var ErrNotFound = errors.New("wishlist: not found")

type DBTX interface {
    Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type WishlistRepo interface {
    Add(ctx context.Context, userID, productID uuid.UUID) error
    Remove(ctx context.Context, userID, productID uuid.UUID) error
    List(ctx context.Context, userID uuid.UUID, limit, offset int) (items []*domain.WishlistItemView, total int, err error)
    Contains(ctx context.Context, userID uuid.UUID, productIDs []uuid.UUID) (map[uuid.UUID]bool, error)
}
```

- [ ] **Step 2: Write `repo/wishlist_pg.go`**

```go
package repo

import (
    "context"

    "github.com/google/uuid"

    "github.com/wearwhere/wearwhere_be/internal/wishlist/domain"
)

type WishlistPG struct{ db DBTX }

func NewWishlistPG(db DBTX) *WishlistPG { return &WishlistPG{db: db} }

// Add inserts (user_id, product_id); ON CONFLICT DO NOTHING makes it idempotent.
// Caller is responsible for verifying the product is active before calling.
func (r *WishlistPG) Add(ctx context.Context, userID, productID uuid.UUID) error {
    _, err := r.db.Exec(ctx,
        `INSERT INTO wishlist_items (user_id, product_id)
         VALUES ($1, $2)
         ON CONFLICT (user_id, product_id) DO NOTHING`,
        userID, productID)
    return err
}

// Remove deletes (user_id, product_id); idempotent — no error if absent.
func (r *WishlistPG) Remove(ctx context.Context, userID, productID uuid.UUID) error {
    _, err := r.db.Exec(ctx,
        `DELETE FROM wishlist_items WHERE user_id=$1 AND product_id=$2`,
        userID, productID)
    return err
}

// List joins to products + brands + product_images (primary) + variant min-price
// subquery. Filters out products that are not active or are soft-deleted.
func (r *WishlistPG) List(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.WishlistItemView, int, error) {
    const baseFrom = `
      FROM wishlist_items wi
      JOIN products p ON p.id = wi.product_id
                     AND p.status='active' AND p.deleted_at IS NULL
      JOIN brands b ON b.id = p.brand_id AND b.deleted_at IS NULL
     WHERE wi.user_id = $1`

    var total int
    if err := r.db.QueryRow(ctx, `SELECT COUNT(*) `+baseFrom, userID).Scan(&total); err != nil {
        return nil, 0, err
    }

    rows, err := r.db.Query(ctx, `
      SELECT
        p.id, p.slug, p.name,
        (SELECT url FROM product_images
           WHERE product_id=p.id AND is_primary
           ORDER BY sort_order ASC LIMIT 1) AS primary_image_url,
        (SELECT MIN(price) FROM product_variants
           WHERE product_id=p.id AND deleted_at IS NULL AND is_active) AS min_price,
        b.id, b.slug, b.name,
        wi.added_at
      `+baseFrom+`
      ORDER BY wi.added_at DESC
      LIMIT $2 OFFSET $3`, userID, limit, offset)
    if err != nil {
        return nil, 0, err
    }
    defer rows.Close()

    var out []*domain.WishlistItemView
    for rows.Next() {
        v := &domain.WishlistItemView{}
        if err := rows.Scan(
            &v.ProductID, &v.ProductSlug, &v.ProductName,
            &v.PrimaryImageURL, &v.MinPrice,
            &v.BrandID, &v.BrandSlug, &v.BrandName,
            &v.AddedAt,
        ); err != nil {
            return nil, 0, err
        }
        out = append(out, v)
    }
    return out, total, rows.Err()
}

// Contains returns map[productID]true for IDs the user has wishlisted; missing IDs
// map to false in the caller (service layer fills zeros).
func (r *WishlistPG) Contains(ctx context.Context, userID uuid.UUID, productIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
    out := make(map[uuid.UUID]bool, len(productIDs))
    if len(productIDs) == 0 {
        return out, nil
    }
    rows, err := r.db.Query(ctx,
        `SELECT product_id FROM wishlist_items
         WHERE user_id=$1 AND product_id = ANY($2)`,
        userID, productIDs)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    for rows.Next() {
        var pid uuid.UUID
        if err := rows.Scan(&pid); err != nil {
            return nil, err
        }
        out[pid] = true
    }
    return out, rows.Err()
}
```

- [ ] **Step 3: Write integration test `repo/wishlist_pg_test.go`**

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

    "github.com/wearwhere/wearwhere_be/internal/testfixtures"
    "github.com/wearwhere/wearwhere_be/internal/wishlist/repo"
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

func TestWishlistPG_AddRemove_Idempotent(t *testing.T) {
    tx := testfixtures.BeginTx(t, pool)
    user := testfixtures.SeedCustomer(t, tx)
    brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
    cat := testfixtures.SeedCategory(t, tx)
    prod := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
    r := repo.NewWishlistPG(tx)

    require.NoError(t, r.Add(context.Background(), user.ID, prod.ID))
    // Second add must not error.
    require.NoError(t, r.Add(context.Background(), user.ID, prod.ID))

    require.NoError(t, r.Remove(context.Background(), user.ID, prod.ID))
    // Second remove must not error.
    require.NoError(t, r.Remove(context.Background(), user.ID, prod.ID))
}

func TestWishlistPG_List_ExcludesInactiveProducts(t *testing.T) {
    tx := testfixtures.BeginTx(t, pool)
    user := testfixtures.SeedCustomer(t, tx)
    brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
    cat := testfixtures.SeedCategory(t, tx)
    active := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
    draft := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "draft")
    testfixtures.SeedWishlistItem(t, tx, user.ID, active.ID)
    testfixtures.SeedWishlistItem(t, tx, user.ID, draft.ID)
    r := repo.NewWishlistPG(tx)

    items, total, err := r.List(context.Background(), user.ID, 20, 0)
    require.NoError(t, err)
    require.Equal(t, 1, total)
    require.Len(t, items, 1)
    require.Equal(t, active.ID, items[0].ProductID)
}

func TestWishlistPG_Contains_MixedHitMiss(t *testing.T) {
    tx := testfixtures.BeginTx(t, pool)
    user := testfixtures.SeedCustomer(t, tx)
    brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
    cat := testfixtures.SeedCategory(t, tx)
    a := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
    b := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
    testfixtures.SeedWishlistItem(t, tx, user.ID, a.ID)
    r := repo.NewWishlistPG(tx)

    res, err := r.Contains(context.Background(), user.ID, []uuid.UUID{a.ID, b.ID})
    require.NoError(t, err)
    require.True(t, res[a.ID])
    require.False(t, res[b.ID]) // absent → false (zero value)
}
```

- [ ] **Step 4: Run integration tests**

Run: `TEST_DATABASE_URL=postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable go test -tags=integration -v ./internal/wishlist/repo/...`
Expected: 3 PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/wishlist/repo/
git commit -m "feat(wishlist): pg repo with idempotent add/remove + list join"
```

---

### Task 13: wishlist service + unit tests

**Files:**
- Create: `internal/wishlist/service/service.go`
- Create: `internal/wishlist/service/service_test.go`

- [ ] **Step 1: Write `service/service.go`**

```go
package service

import (
    "context"

    "github.com/google/uuid"

    productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
    "github.com/wearwhere/wearwhere_be/internal/wishlist/domain"
    "github.com/wearwhere/wearwhere_be/internal/wishlist/repo"
)

type Service struct {
    wishlist    repo.WishlistRepo
    productRepo productrepo.ProductRepo
}

func New(w repo.WishlistRepo, p productrepo.ProductRepo) *Service {
    return &Service{wishlist: w, productRepo: p}
}

// Add gates on product existence + active + not soft-deleted. All three
// failure modes collapse to ErrProductNotFound — the caller (frontend) does
// not need to disambiguate.
func (s *Service) Add(ctx context.Context, userID, productID uuid.UUID) error {
    p, err := s.productRepo.FindByID(ctx, productID)
    if err != nil || p == nil {
        return domain.ErrProductNotFound
    }
    if string(p.Status) != "active" || p.DeletedAt != nil {
        return domain.ErrProductNotFound
    }
    return s.wishlist.Add(ctx, userID, productID)
}

// Remove is idempotent — never errors on missing row.
func (s *Service) Remove(ctx context.Context, userID, productID uuid.UUID) error {
    return s.wishlist.Remove(ctx, userID, productID)
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, page, limit int) ([]*domain.WishlistItemView, int, error) {
    return s.wishlist.List(ctx, userID, limit, (page-1)*limit)
}

// Contains backfills false for IDs the user does not have wishlisted so the
// HTTP response includes every requested ID as a key.
func (s *Service) Contains(ctx context.Context, userID uuid.UUID, productIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
    hits, err := s.wishlist.Contains(ctx, userID, productIDs)
    if err != nil {
        return nil, err
    }
    out := make(map[uuid.UUID]bool, len(productIDs))
    for _, id := range productIDs {
        out[id] = hits[id]
    }
    return out, nil
}
```

- [ ] **Step 2: Write unit tests with mock repos**

```go
package service_test

import (
    "context"
    "testing"
    "time"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    productdomain "github.com/wearwhere/wearwhere_be/internal/product/domain"
    "github.com/wearwhere/wearwhere_be/internal/wishlist/domain"
    "github.com/wearwhere/wearwhere_be/internal/wishlist/service"
)

type fakeWishlist struct {
    addErr error
    rmErr  error
    has    map[uuid.UUID]bool
}

func (f *fakeWishlist) Add(_ context.Context, _, _ uuid.UUID) error    { return f.addErr }
func (f *fakeWishlist) Remove(_ context.Context, _, _ uuid.UUID) error { return f.rmErr }
func (f *fakeWishlist) List(_ context.Context, _ uuid.UUID, _, _ int) ([]*domain.WishlistItemView, int, error) {
    return nil, 0, nil
}
func (f *fakeWishlist) Contains(_ context.Context, _ uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
    out := make(map[uuid.UUID]bool, len(ids))
    for _, id := range ids {
        if f.has[id] {
            out[id] = true
        }
    }
    return out, nil
}

type fakeProductRepo struct{ ret *productdomain.Product; err error }

func (f *fakeProductRepo) Create(_ context.Context, _ uuid.UUID, _ string, _ *productdomain.CreateProductRequest) (*productdomain.Product, error) { return nil, nil }
func (f *fakeProductRepo) FindByID(_ context.Context, _ uuid.UUID) (*productdomain.Product, error)                                                      { return f.ret, f.err }
func (f *fakeProductRepo) FindByBrandSlug(_ context.Context, _, _ string) (*productdomain.Product, error)                                              { return nil, nil }
func (f *fakeProductRepo) Update(_ context.Context, _, _ uuid.UUID, _ *productdomain.UpdateProductRequest) error                                       { return nil }
func (f *fakeProductRepo) SoftDelete(_ context.Context, _, _ uuid.UUID) error                                                                          { return nil }
func (f *fakeProductRepo) ListByBrand(_ context.Context, _ uuid.UUID, _, _ int) ([]*productdomain.Product, int, error)                                  { return nil, 0, nil }
func (f *fakeProductRepo) SlugExists(_ context.Context, _ uuid.UUID, _ string) (bool, error)                                                            { return false, nil }
func (f *fakeProductRepo) IncrementViewCount(_ context.Context, _ uuid.UUID) error                                                                      { return nil }
func (f *fakeProductRepo) SetStyleTags(_ context.Context, _ uuid.UUID, _ []uuid.UUID) error                                                              { return nil }
func (f *fakeProductRepo) GetStyleTags(_ context.Context, _ uuid.UUID) ([]*productdomain.StyleTag, error)                                                { return nil, nil }

func TestAdd_InactiveProductReturnsNotFound(t *testing.T) {
    inactive := &productdomain.Product{Status: "draft"}
    s := service.New(&fakeWishlist{}, &fakeProductRepo{ret: inactive})
    err := s.Add(context.Background(), uuid.New(), uuid.New())
    require.ErrorIs(t, err, domain.ErrProductNotFound)
}

func TestAdd_SoftDeletedProductReturnsNotFound(t *testing.T) {
    now := time.Now()
    deleted := &productdomain.Product{Status: "active", DeletedAt: &now}
    s := service.New(&fakeWishlist{}, &fakeProductRepo{ret: deleted})
    err := s.Add(context.Background(), uuid.New(), uuid.New())
    require.ErrorIs(t, err, domain.ErrProductNotFound)
}

func TestContains_AbsentIDsMapToFalse(t *testing.T) {
    a, b := uuid.New(), uuid.New()
    s := service.New(&fakeWishlist{has: map[uuid.UUID]bool{a: true}}, &fakeProductRepo{})
    out, err := s.Contains(context.Background(), uuid.New(), []uuid.UUID{a, b})
    require.NoError(t, err)
    require.True(t, out[a])
    require.False(t, out[b])
}
```

- [ ] **Step 3: Run unit tests**

Run: `go test -v ./internal/wishlist/service/...`
Expected: 3 PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/wishlist/service/
git commit -m "feat(wishlist): service with product-eligibility gate"
```

---

### Task 14: wishlist handler + routes + httptest

**Files:**
- Create: `internal/wishlist/handler/handler.go`
- Create: `internal/wishlist/handler/routes.go`
- Create: `internal/wishlist/handler/handler_test.go`

- [ ] **Step 1: Write `handler/handler.go`**

```go
package handler

import (
    "errors"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
    "github.com/wearwhere/wearwhere_be/internal/wishlist/domain"
    "github.com/wearwhere/wearwhere_be/internal/wishlist/service"
    "github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type Handler struct{ svc *service.Service }

func New(s *service.Service) *Handler { return &Handler{svc: s} }

// itemToResponse converts a domain view to the HTTP response shape.
func itemToResponse(v *domain.WishlistItemView) domain.WishlistItemResponse {
    r := domain.WishlistItemResponse{
        ProductID:       v.ProductID.String(),
        ProductSlug:     v.ProductSlug,
        ProductName:     v.ProductName,
        PrimaryImageURL: v.PrimaryImageURL,
        MinPrice:        v.MinPrice,
        AddedAt:         v.AddedAt.UTC().Format("2006-01-02T15:04:05Z"),
    }
    r.Brand.ID = v.BrandID.String()
    r.Brand.Slug = v.BrandSlug
    r.Brand.Name = v.BrandName
    return r
}

func (h *Handler) Add(c *gin.Context) {
    uid, _ := authmw.UserID(c)
    pid, err := uuid.Parse(c.Param("product_id"))
    if err != nil {
        httpx.ErrorFromApp(c, domain.ErrProductNotFound)
        return
    }
    if err := h.svc.Add(c.Request.Context(), uid, pid); err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    c.JSON(http.StatusOK, gin.H{"in_wishlist": true})
}

func (h *Handler) Remove(c *gin.Context) {
    uid, _ := authmw.UserID(c)
    pid, err := uuid.Parse(c.Param("product_id"))
    if err != nil {
        // Idempotent: bad UUID still 204 since "absent" semantics holds.
        c.Status(http.StatusNoContent)
        return
    }
    if err := h.svc.Remove(c.Request.Context(), uid, pid); err != nil {
        // Only fail for infra errors.
        if !errors.Is(err, nil) {
            httpx.ErrorFromApp(c, err)
            return
        }
    }
    c.Status(http.StatusNoContent)
}

func (h *Handler) List(c *gin.Context) {
    uid, _ := authmw.UserID(c)
    var q domain.WishlistListQuery
    if err := c.ShouldBindQuery(&q); err != nil {
        httpx.ValidationError(c, err)
        return
    }
    items, total, err := h.svc.List(c.Request.Context(), uid, q.Page, q.Limit)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    resp := domain.WishlistListResponse{Items: make([]domain.WishlistItemResponse, 0, len(items))}
    for _, v := range items {
        resp.Items = append(resp.Items, itemToResponse(v))
    }
    resp.Pagination.Page = q.Page
    resp.Pagination.Limit = q.Limit
    resp.Pagination.Total = total
    if q.Limit > 0 {
        resp.Pagination.TotalPages = (total + q.Limit - 1) / q.Limit
    }
    resp.Pagination.HasMore = q.Page*q.Limit < total
    c.JSON(http.StatusOK, resp)
}

func (h *Handler) Contains(c *gin.Context) {
    uid, _ := authmw.UserID(c)
    var q domain.WishlistContainsQuery
    if err := c.ShouldBindQuery(&q); err != nil {
        httpx.ValidationError(c, err)
        return
    }
    ids := make([]uuid.UUID, 0, len(q.ProductIDs))
    for _, s := range q.ProductIDs {
        if id, err := uuid.Parse(s); err == nil {
            ids = append(ids, id)
        }
    }
    res, err := h.svc.Contains(c.Request.Context(), uid, ids)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    out := make(map[string]bool, len(res))
    for k, v := range res {
        out[k.String()] = v
    }
    c.JSON(http.StatusOK, domain.WishlistContainsResponse{InWishlist: out})
}
```

- [ ] **Step 2: Write `handler/routes.go`**

```go
package handler

import "github.com/gin-gonic/gin"

func Mount(rg *gin.RouterGroup, h *Handler) {
    rg.GET("/wishlist", h.List)
    rg.GET("/wishlist/contains", h.Contains)
    rg.POST("/wishlist/:product_id", h.Add)
    rg.DELETE("/wishlist/:product_id", h.Remove)
}
```

- [ ] **Step 3: Write minimal httptest (`handler/handler_test.go`)**

```go
package handler_test

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
    "github.com/wearwhere/wearwhere_be/internal/wishlist/handler"
)

// Stubs are intentionally minimal — the meaningful coverage lives in service_test
// and repo_test. This file only verifies routing + status codes.

func TestWishlistRoutes_RemoveIsIdempotent204(t *testing.T) {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    h := handler.New(nil) // svc nil; we exercise only the bad-uuid branch
    rg := r.Group("/me", func(c *gin.Context) { authmw.SetUserIDForTest(c, uuid.New()); c.Next() })
    handler.Mount(rg, h)

    w := httptest.NewRecorder()
    req, _ := http.NewRequest("DELETE", "/me/wishlist/not-a-uuid", nil)
    r.ServeHTTP(w, req)
    require.Equal(t, http.StatusNoContent, w.Code)
    _ = json.NewDecoder // keep encoding/json import live
}
```

The nil-service test is intentional: the bad-UUID branch in `Remove` short-circuits before touching the service. Tests that exercise valid paths require real service wiring and live in the E2E test (Task 17/18).

- [ ] **Step 4: Run handler tests**

Run: `go test -v ./internal/wishlist/handler/...`
Expected: 1 PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/wishlist/handler/
git commit -m "feat(wishlist): handler with list/contains/add/remove + routes"
```

---

### Task 15: Wire wishlist into `cmd/api/main.go`

**Files:**
- Modify: `cmd/api/main.go`

- [ ] **Step 1: Add imports**

```go
wishlisthandler "github.com/wearwhere/wearwhere_be/internal/wishlist/handler"
wishlistrepo "github.com/wearwhere/wearwhere_be/internal/wishlist/repo"
wishlistservice "github.com/wearwhere/wearwhere_be/internal/wishlist/service"
```

- [ ] **Step 2: Construct dependencies (after customeraddr wiring from Task 10)**

```go
wishlistRepo := wishlistrepo.NewWishlistPG(pgPool)
wishlistSvc := wishlistservice.New(wishlistRepo, productRepo)
wishlistHandler := wishlisthandler.New(wishlistSvc)
```

- [ ] **Step 3: Mount on the existing `customerGroup`**

After the existing `customeraddrhandler.Mount(customerGroup, customerAddrHandler)`:
```go
wishlisthandler.Mount(customerGroup, wishlistHandler)
```

- [ ] **Step 4: Build**

Run: `go build ./cmd/api`
Expected: success.

- [ ] **Step 5: Commit**

```bash
git add cmd/api/main.go
git commit -m "feat(wishlist): wire wishlist routes under /me"
```

---

## Phase C — Cart (Tasks 16–22)

### Task 16: Add `VariantRepo.FindForPurchase` to product module

**Files:**
- Modify: `internal/product/repo/repo.go`
- Modify: `internal/product/repo/variant_pg.go`

- [ ] **Step 1: Extend `VariantRepo` interface in `repo.go`**

Locate the `VariantRepo` interface (around line 41 of `internal/product/repo/repo.go`). Append a method:

```go
type VariantRepo interface {
    // ... existing methods ...

    // FindForPurchase returns the variant and its parent product only if both are
    // active, in-stock-capable, and not soft-deleted. Returns ErrNotFound if the
    // variant is missing, inactive, soft-deleted, or its product is not active.
    FindForPurchase(ctx context.Context, variantID uuid.UUID) (*domain.Variant, *domain.Product, error)
}
```

- [ ] **Step 2: Implement in `variant_pg.go`**

Append to `internal/product/repo/variant_pg.go`:

```go
func (r *VariantPG) FindForPurchase(ctx context.Context, variantID uuid.UUID) (*domain.Variant, *domain.Product, error) {
    var v domain.Variant
    var p domain.Product
    err := r.db.QueryRow(ctx, `
      SELECT
        v.id, v.product_id, v.sku, v.size, v.color, v.color_hex,
        v.price, v.stock_qty, v.is_active, v.image_id,
        v.created_at, v.updated_at, v.deleted_at,
        p.id, p.brand_id, p.category_id, p.slug, p.name, p.description,
        p.status, p.currency, p.sold_count, p.view_count,
        p.created_at, p.updated_at, p.deleted_at
      FROM product_variants v
      JOIN products p ON p.id = v.product_id
      WHERE v.id = $1
        AND v.deleted_at IS NULL AND v.is_active = TRUE
        AND p.deleted_at IS NULL AND p.status = 'active'`,
        variantID,
    ).Scan(
        &v.ID, &v.ProductID, &v.SKU, &v.Size, &v.Color, &v.ColorHex,
        &v.Price, &v.StockQty, &v.IsActive, &v.ImageID,
        &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
        &p.ID, &p.BrandID, &p.CategoryID, &p.Slug, &p.Name, &p.Description,
        &p.Status, &p.Currency, &p.SoldCount, &p.ViewCount,
        &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
    )
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, nil, ErrNotFound
        }
        return nil, nil, err
    }
    return &v, &p, nil
}
```

If `errors` or `pgx` imports are not yet in this file, add them.

- [ ] **Step 3: Add an integration test**

Append to `internal/product/repo/variant_pg_test.go` (existing file):

```go
//go:build integration

func TestVariantPG_FindForPurchase_RejectsInactiveProduct(t *testing.T) {
    tx := testfixtures.BeginTx(t, pool)
    brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
    cat := testfixtures.SeedCategory(t, tx)
    draft := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "draft")
    vid := testfixtures.SeedVariant(t, tx, draft.ID, "M", "Black", 199000, 5)
    r := repo.NewVariantPG(tx)

    _, _, err := r.FindForPurchase(context.Background(), vid)
    require.ErrorIs(t, err, repo.ErrNotFound)
}
```

- [ ] **Step 4: Run test**

Run: `TEST_DATABASE_URL=... go test -tags=integration -v -run TestVariantPG_FindForPurchase ./internal/product/repo/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/product/repo/repo.go internal/product/repo/variant_pg.go internal/product/repo/variant_pg_test.go
git commit -m "feat(product): variant FindForPurchase for cart eligibility"
```

---

### Task 17: cart domain (types, errors, DTOs)

**Files:**
- Create: `internal/cart/domain/cart.go`
- Create: `internal/cart/domain/errors.go`
- Create: `internal/cart/domain/dto.go`

- [ ] **Step 1: Write `domain/cart.go`**

```go
package domain

import (
    "time"

    "github.com/google/uuid"
)

type CartItem struct {
    ID               uuid.UUID
    UserID           uuid.UUID
    VariantID        uuid.UUID
    Qty              int
    PriceSnapshot    float64
    CurrencySnapshot string
    AddedAt          time.Time
    UpdatedAt        time.Time
}

// CartItemView is the denormalized row returned by GET /me/cart.
type CartItemView struct {
    ID                uuid.UUID
    Qty               int
    PriceSnapshot     float64
    CurrentPrice      float64
    CurrencySnapshot  string
    AddedAt           time.Time

    VariantID    uuid.UUID
    SKU          string
    Size         string
    Color        string
    ColorHex     *string
    StockQty     int

    ProductID       uuid.UUID
    ProductSlug     string
    ProductName     string
    PrimaryImageURL *string

    BrandID   uuid.UUID
    BrandSlug string
    BrandName string

    Unavailable       bool
    UnavailableReason *string // "variant_inactive" | "variant_deleted" | "product_unavailable"
}
```

- [ ] **Step 2: Write `domain/errors.go`**

```go
package domain

import (
    "net/http"

    "github.com/wearwhere/wearwhere_be/pkg/httpx"
)

var (
    ErrCartItemNotFound = &httpx.AppError{
        Status:  http.StatusNotFound,
        Code:    "CART_ITEM_NOT_FOUND",
        Message: "Cart item not found",
    }
    ErrVariantUnavailable = &httpx.AppError{
        Status:  http.StatusConflict,
        Code:    "VARIANT_UNAVAILABLE",
        Message: "Variant or product is no longer available",
    }
    ErrOutOfStock = &httpx.AppError{
        Status:  http.StatusConflict,
        Code:    "VARIANT_OUT_OF_STOCK",
        Message: "Insufficient stock for requested quantity",
    }
    ErrQtyExceedsMax = &httpx.AppError{
        Status:  http.StatusBadRequest,
        Code:    "QTY_EXCEEDS_MAX",
        Message: "Quantity exceeds maximum allowed per item (10)",
    }
)
```

- [ ] **Step 3: Write `domain/dto.go`**

```go
package domain

type AddToCartRequest struct {
    VariantID string `json:"variant_id" binding:"required,uuid"`
    Qty       int    `json:"qty"        binding:"required,min=1,max=10"`
}

type UpdateCartItemRequest struct {
    Qty int `json:"qty" binding:"required,min=1,max=10"`
}

type CartItemResponse struct {
    ID                string  `json:"id"`
    Qty               int     `json:"qty"`
    PriceSnapshot     string  `json:"price_snapshot"`
    CurrentPrice      string  `json:"current_price"`
    PriceChanged      bool    `json:"price_changed"`
    SubtotalSnapshot  string  `json:"subtotal_snapshot"`
    SubtotalCurrent   string  `json:"subtotal_current"`
    Currency          string  `json:"currency"`
    Unavailable       bool    `json:"unavailable"`
    UnavailableReason *string `json:"unavailable_reason,omitempty"`
    AddedAt           string  `json:"added_at"`

    Variant struct {
        ID       string  `json:"id"`
        SKU      string  `json:"sku"`
        Size     string  `json:"size"`
        Color    string  `json:"color"`
        ColorHex *string `json:"color_hex,omitempty"`
        StockQty int     `json:"stock_qty"`
    } `json:"variant"`

    Product struct {
        ID              string  `json:"id"`
        Slug            string  `json:"slug"`
        Name            string  `json:"name"`
        PrimaryImageURL *string `json:"primary_image_url,omitempty"`
    } `json:"product"`

    Brand struct {
        ID   string `json:"id"`
        Slug string `json:"slug"`
        Name string `json:"name"`
    } `json:"brand"`
}

type CartSummary struct {
    ItemCount       int    `json:"item_count"`
    TotalQty        int    `json:"total_qty"`
    TotalSnapshot   string `json:"total_snapshot"`
    TotalCurrent    string `json:"total_current"`
    Currency        string `json:"currency"`
    HasPriceChanges bool   `json:"has_price_changes"`
    HasUnavailable  bool   `json:"has_unavailable"`
}

type CartResponse struct {
    Items   []CartItemResponse `json:"items"`
    Summary CartSummary        `json:"summary"`
}
```

- [ ] **Step 4: Verify compiles**

Run: `go build ./internal/cart/...`
Expected: success.

- [ ] **Step 5: Commit**

```bash
git add internal/cart/domain/
git commit -m "feat(cart): domain types, errors, DTOs"
```

---

### Task 18: cart repo (PG) + integration tests

**Files:**
- Create: `internal/cart/repo/repo.go`
- Create: `internal/cart/repo/cart_pg.go`
- Create: `internal/cart/repo/cart_pg_test.go`

- [ ] **Step 1: Write `repo/repo.go`**

```go
package repo

import (
    "context"
    "errors"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgconn"

    "github.com/wearwhere/wearwhere_be/internal/cart/domain"
)

var ErrNotFound = errors.New("cart: not found")

type DBTX interface {
    Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type CartRepo interface {
    // UpsertAdd inserts a cart row for (userID, variantID) or increments qty
    // on conflict. Caller must already have validated qty range, variant
    // availability, and stock. Returns the resulting row.
    UpsertAdd(ctx context.Context, userID, variantID uuid.UUID, qty int, price float64) (*domain.CartItem, error)

    // FindByID returns a cart_items row scoped to userID; ErrNotFound if absent.
    FindByID(ctx context.Context, id, userID uuid.UUID) (*domain.CartItem, error)

    // UpdateQty sets qty + refreshes price_snapshot. Scoped to userID.
    UpdateQty(ctx context.Context, id, userID uuid.UUID, qty int, price float64) (*domain.CartItem, error)

    // Delete removes a single item; ErrNotFound if absent or wrong user.
    Delete(ctx context.Context, id, userID uuid.UUID) error

    // Clear removes all cart_items for the user.
    Clear(ctx context.Context, userID uuid.UUID) error

    // ListView returns denormalized rows joined with variant + product + brand
    // + primary image. Soft-deleted variants/products surface with Unavailable=true.
    ListView(ctx context.Context, userID uuid.UUID) ([]*domain.CartItemView, error)
}
```

- [ ] **Step 2: Write `repo/cart_pg.go`**

```go
package repo

import (
    "context"
    "errors"
    "time"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"

    "github.com/wearwhere/wearwhere_be/internal/cart/domain"
)

type CartPG struct{ db DBTX }

func NewCartPG(db DBTX) *CartPG { return &CartPG{db: db} }

const itemCols = `id, user_id, variant_id, qty, price_snapshot,
                  currency_snapshot, added_at, updated_at`

func scanItem(row pgx.Row) (*domain.CartItem, error) {
    var i domain.CartItem
    err := row.Scan(
        &i.ID, &i.UserID, &i.VariantID, &i.Qty, &i.PriceSnapshot,
        &i.CurrencySnapshot, &i.AddedAt, &i.UpdatedAt,
    )
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, ErrNotFound
        }
        return nil, err
    }
    return &i, nil
}

func (r *CartPG) UpsertAdd(ctx context.Context, userID, variantID uuid.UUID, qty int, price float64) (*domain.CartItem, error) {
    return scanItem(r.db.QueryRow(ctx,
        `INSERT INTO cart_items (user_id, variant_id, qty, price_snapshot, currency_snapshot)
         VALUES ($1, $2, $3, $4, 'VND')
         ON CONFLICT (user_id, variant_id) DO UPDATE
           SET qty            = LEAST(cart_items.qty + EXCLUDED.qty, 10),
               price_snapshot = EXCLUDED.price_snapshot,
               updated_at     = NOW()
         RETURNING `+itemCols,
        userID, variantID, qty, price))
}

func (r *CartPG) FindByID(ctx context.Context, id, userID uuid.UUID) (*domain.CartItem, error) {
    return scanItem(r.db.QueryRow(ctx,
        `SELECT `+itemCols+` FROM cart_items WHERE id=$1 AND user_id=$2`, id, userID))
}

func (r *CartPG) UpdateQty(ctx context.Context, id, userID uuid.UUID, qty int, price float64) (*domain.CartItem, error) {
    return scanItem(r.db.QueryRow(ctx,
        `UPDATE cart_items
            SET qty=$3, price_snapshot=$4, updated_at=NOW()
          WHERE id=$1 AND user_id=$2
        RETURNING `+itemCols,
        id, userID, qty, price))
}

func (r *CartPG) Delete(ctx context.Context, id, userID uuid.UUID) error {
    tag, err := r.db.Exec(ctx,
        `DELETE FROM cart_items WHERE id=$1 AND user_id=$2`, id, userID)
    if err != nil {
        return err
    }
    if tag.RowsAffected() == 0 {
        return ErrNotFound
    }
    return nil
}

func (r *CartPG) Clear(ctx context.Context, userID uuid.UUID) error {
    _, err := r.db.Exec(ctx, `DELETE FROM cart_items WHERE user_id=$1`, userID)
    return err
}

// ListView LEFT JOINs variants/products/brands so soft-deleted variants still
// appear (with Unavailable=true) until the user explicitly removes them.
func (r *CartPG) ListView(ctx context.Context, userID uuid.UUID) ([]*domain.CartItemView, error) {
    rows, err := r.db.Query(ctx, `
      SELECT
        ci.id, ci.qty, ci.price_snapshot, ci.currency_snapshot, ci.added_at,
        v.id, v.sku, v.size, v.color, v.color_hex, v.stock_qty, v.price, v.is_active, v.deleted_at,
        p.id, p.slug, p.name, p.status, p.deleted_at,
        (SELECT url FROM product_images
           WHERE product_id = p.id AND is_primary
           ORDER BY sort_order ASC LIMIT 1) AS primary_image_url,
        b.id, b.slug, b.name
      FROM cart_items ci
      JOIN product_variants v ON v.id = ci.variant_id
      JOIN products p ON p.id = v.product_id
      JOIN brands b ON b.id = p.brand_id
     WHERE ci.user_id = $1
     ORDER BY ci.added_at DESC`, userID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var out []*domain.CartItemView
    for rows.Next() {
        v := &domain.CartItemView{}
        var vIsActive bool
        var vDeletedAt, pDeletedAt *time.Time
        var pStatus string
        if err := rows.Scan(
            &v.ID, &v.Qty, &v.PriceSnapshot, &v.CurrencySnapshot, &v.AddedAt,
            &v.VariantID, &v.SKU, &v.Size, &v.Color, &v.ColorHex, &v.StockQty,
            &v.CurrentPrice, &vIsActive, &vDeletedAt,
            &v.ProductID, &v.ProductSlug, &v.ProductName, &pStatus, &pDeletedAt,
            &v.PrimaryImageURL,
            &v.BrandID, &v.BrandSlug, &v.BrandName,
        ); err != nil {
            return nil, err
        }
        // Compute availability flags. Variant-deleted takes precedence over
        // variant-inactive, which takes precedence over product-unavailable.
        switch {
        case vDeletedAt != nil:
            v.Unavailable = true
            reason := "variant_deleted"
            v.UnavailableReason = &reason
        case !vIsActive:
            v.Unavailable = true
            reason := "variant_inactive"
            v.UnavailableReason = &reason
        case pDeletedAt != nil || pStatus != "active":
            v.Unavailable = true
            reason := "product_unavailable"
            v.UnavailableReason = &reason
        }
        out = append(out, v)
    }
    return out, rows.Err()
}
```

- [ ] **Step 3: Write integration tests `repo/cart_pg_test.go`**

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

    "github.com/wearwhere/wearwhere_be/internal/cart/repo"
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

func TestCartPG_UpsertIncrementsExistingRow(t *testing.T) {
    tx := testfixtures.BeginTx(t, pool)
    user := testfixtures.SeedCustomer(t, tx)
    brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
    cat := testfixtures.SeedCategory(t, tx)
    prod := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
    vid := testfixtures.SeedVariant(t, tx, prod.ID, "M", "Black", 199000, 100)
    r := repo.NewCartPG(tx)

    first, err := r.UpsertAdd(context.Background(), user.ID, vid, 2, 199000)
    require.NoError(t, err)
    require.Equal(t, 2, first.Qty)

    second, err := r.UpsertAdd(context.Background(), user.ID, vid, 3, 199000)
    require.NoError(t, err)
    require.Equal(t, first.ID, second.ID)
    require.Equal(t, 5, second.Qty)
}

func TestCartPG_UpsertClampsToTen(t *testing.T) {
    tx := testfixtures.BeginTx(t, pool)
    user := testfixtures.SeedCustomer(t, tx)
    brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
    cat := testfixtures.SeedCategory(t, tx)
    prod := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
    vid := testfixtures.SeedVariant(t, tx, prod.ID, "M", "Black", 199000, 100)
    r := repo.NewCartPG(tx)

    _, _ = r.UpsertAdd(context.Background(), user.ID, vid, 8, 199000)
    out, err := r.UpsertAdd(context.Background(), user.ID, vid, 5, 199000)
    require.NoError(t, err)
    require.Equal(t, 10, out.Qty) // clamped
}

func TestCartPG_IDOR_DeleteOtherUserItem(t *testing.T) {
    tx := testfixtures.BeginTx(t, pool)
    owner := testfixtures.SeedCustomer(t, tx)
    intruder := testfixtures.SeedCustomer(t, tx)
    brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
    cat := testfixtures.SeedCategory(t, tx)
    prod := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
    vid := testfixtures.SeedVariant(t, tx, prod.ID, "M", "Black", 199000, 5)
    seeded := testfixtures.SeedCartItem(t, tx, owner.ID, vid, 1, 199000)
    r := repo.NewCartPG(tx)

    err := r.Delete(context.Background(), seeded.ID, intruder.ID)
    require.ErrorIs(t, err, repo.ErrNotFound)
}

func TestCartPG_ListView_FlagsSoftDeletedVariant(t *testing.T) {
    tx := testfixtures.BeginTx(t, pool)
    user := testfixtures.SeedCustomer(t, tx)
    brand := testfixtures.SeedBrand(t, tx, uuid.Nil)
    cat := testfixtures.SeedCategory(t, tx)
    prod := testfixtures.SeedProduct(t, tx, brand.ID, cat.ID, "active")
    vid := testfixtures.SeedVariant(t, tx, prod.ID, "M", "Black", 199000, 5)
    testfixtures.SeedCartItem(t, tx, user.ID, vid, 2, 199000)

    // Soft-delete the variant after it was added to cart.
    _, err := tx.Exec(context.Background(),
        `UPDATE product_variants SET deleted_at=NOW() WHERE id=$1`, vid)
    require.NoError(t, err)

    r := repo.NewCartPG(tx)
    items, err := r.ListView(context.Background(), user.ID)
    require.NoError(t, err)
    require.Len(t, items, 1)
    require.True(t, items[0].Unavailable)
    require.NotNil(t, items[0].UnavailableReason)
    require.Equal(t, "variant_deleted", *items[0].UnavailableReason)
}
```

- [ ] **Step 4: Run integration tests**

Run: `TEST_DATABASE_URL=... go test -tags=integration -v ./internal/cart/repo/...`
Expected: 4 PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cart/repo/
git commit -m "feat(cart): pg repo with UPSERT/clamp/IDOR/availability flags"
```

---

### Task 19: cart service + unit tests

**Files:**
- Create: `internal/cart/service/service.go`
- Create: `internal/cart/service/service_test.go`

- [ ] **Step 1: Write `service/service.go`**

```go
package service

import (
    "context"
    "errors"

    "github.com/google/uuid"

    "github.com/wearwhere/wearwhere_be/internal/cart/domain"
    "github.com/wearwhere/wearwhere_be/internal/cart/repo"
    productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
)

type Service struct {
    cart    repo.CartRepo
    variant productrepo.VariantRepo
}

func New(c repo.CartRepo, v productrepo.VariantRepo) *Service {
    return &Service{cart: c, variant: v}
}

func (s *Service) Add(ctx context.Context, userID uuid.UUID, variantID uuid.UUID, qty int) (*domain.CartItem, error) {
    if qty < 1 || qty > 10 {
        return nil, domain.ErrQtyExceedsMax
    }
    v, _, err := s.variant.FindForPurchase(ctx, variantID)
    if err != nil {
        if errors.Is(err, productrepo.ErrNotFound) {
            return nil, domain.ErrVariantUnavailable
        }
        return nil, err
    }

    // Check cumulative qty against the 10-max and the live stock.
    existing, findErr := s.cart.FindByVariant(ctx, userID, variantID) // helper added in repo
    cumulative := qty
    if findErr == nil && existing != nil {
        cumulative += existing.Qty
        if cumulative > 10 {
            return nil, domain.ErrQtyExceedsMax
        }
    }
    if v.StockQty < cumulative {
        return nil, domain.ErrOutOfStock
    }

    return s.cart.UpsertAdd(ctx, userID, variantID, qty, v.Price)
}

func (s *Service) UpdateQty(ctx context.Context, id, userID uuid.UUID, qty int) (*domain.CartItem, error) {
    if qty < 1 || qty > 10 {
        return nil, domain.ErrQtyExceedsMax
    }
    item, err := s.cart.FindByID(ctx, id, userID)
    if err != nil {
        if errors.Is(err, repo.ErrNotFound) {
            return nil, domain.ErrCartItemNotFound
        }
        return nil, err
    }
    v, _, err := s.variant.FindForPurchase(ctx, item.VariantID)
    if err != nil {
        return nil, domain.ErrVariantUnavailable
    }
    if v.StockQty < qty {
        return nil, domain.ErrOutOfStock
    }
    return s.cart.UpdateQty(ctx, id, userID, qty, v.Price)
}

func (s *Service) Remove(ctx context.Context, id, userID uuid.UUID) error {
    if err := s.cart.Delete(ctx, id, userID); err != nil {
        if errors.Is(err, repo.ErrNotFound) {
            return domain.ErrCartItemNotFound
        }
        return err
    }
    return nil
}

func (s *Service) Clear(ctx context.Context, userID uuid.UUID) error {
    return s.cart.Clear(ctx, userID)
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]*domain.CartItemView, error) {
    return s.cart.ListView(ctx, userID)
}
```

Add the `FindByVariant` helper to `CartRepo` interface and `CartPG` since the service references it:

```go
// In repo.go interface:
FindByVariant(ctx context.Context, userID, variantID uuid.UUID) (*domain.CartItem, error)
```

```go
// In cart_pg.go:
func (r *CartPG) FindByVariant(ctx context.Context, userID, variantID uuid.UUID) (*domain.CartItem, error) {
    return scanItem(r.db.QueryRow(ctx,
        `SELECT `+itemCols+` FROM cart_items
         WHERE user_id=$1 AND variant_id=$2`, userID, variantID))
}
```

Add it now (this is part of Task 19's first step).

- [ ] **Step 2: Write `service/service_test.go`**

```go
package service_test

import (
    "context"
    "testing"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    "github.com/wearwhere/wearwhere_be/internal/cart/domain"
    "github.com/wearwhere/wearwhere_be/internal/cart/repo"
    "github.com/wearwhere/wearwhere_be/internal/cart/service"
    productdomain "github.com/wearwhere/wearwhere_be/internal/product/domain"
    productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
)

type fakeCart struct {
    upsertReturns *domain.CartItem
    upsertErr     error
    findByVariant *domain.CartItem
    findVarErr    error
    findByID      *domain.CartItem
    findIDErr     error
    updateReturns *domain.CartItem
    deleteErr     error
}

func (f *fakeCart) UpsertAdd(_ context.Context, _, _ uuid.UUID, _ int, _ float64) (*domain.CartItem, error) {
    return f.upsertReturns, f.upsertErr
}
func (f *fakeCart) FindByID(_ context.Context, _, _ uuid.UUID) (*domain.CartItem, error) {
    return f.findByID, f.findIDErr
}
func (f *fakeCart) FindByVariant(_ context.Context, _, _ uuid.UUID) (*domain.CartItem, error) {
    return f.findByVariant, f.findVarErr
}
func (f *fakeCart) UpdateQty(_ context.Context, _, _ uuid.UUID, _ int, _ float64) (*domain.CartItem, error) {
    return f.updateReturns, nil
}
func (f *fakeCart) Delete(_ context.Context, _, _ uuid.UUID) error    { return f.deleteErr }
func (f *fakeCart) Clear(_ context.Context, _ uuid.UUID) error        { return nil }
func (f *fakeCart) ListView(_ context.Context, _ uuid.UUID) ([]*domain.CartItemView, error) {
    return nil, nil
}

type fakeVariant struct {
    v   *productdomain.Variant
    p   *productdomain.Product
    err error
}

func (f *fakeVariant) Create(_ context.Context, _ uuid.UUID, _ *productdomain.CreateVariantRequest) (*productdomain.Variant, error) { return nil, nil }
func (f *fakeVariant) FindByID(_ context.Context, _, _ uuid.UUID) (*productdomain.Variant, error)                                                 { return f.v, f.err }
func (f *fakeVariant) ListByProduct(_ context.Context, _ uuid.UUID, _ bool) ([]*productdomain.Variant, error)                                      { return nil, nil }
func (f *fakeVariant) Update(_ context.Context, _, _ uuid.UUID, _ *productdomain.UpdateVariantRequest) (*productdomain.Variant, error)             { return nil, nil }
func (f *fakeVariant) SoftDelete(_ context.Context, _, _ uuid.UUID) error                                                                           { return nil }
func (f *fakeVariant) FindForPurchase(_ context.Context, _ uuid.UUID) (*productdomain.Variant, *productdomain.Product, error) {
    return f.v, f.p, f.err
}

func TestAdd_QtyExceedsMax(t *testing.T) {
    s := service.New(&fakeCart{}, &fakeVariant{})
    _, err := s.Add(context.Background(), uuid.New(), uuid.New(), 11)
    require.ErrorIs(t, err, domain.ErrQtyExceedsMax)
}

func TestAdd_UnavailableVariant(t *testing.T) {
    s := service.New(&fakeCart{}, &fakeVariant{err: productrepo.ErrNotFound})
    _, err := s.Add(context.Background(), uuid.New(), uuid.New(), 2)
    require.ErrorIs(t, err, domain.ErrVariantUnavailable)
}

func TestAdd_OutOfStock(t *testing.T) {
    v := &productdomain.Variant{StockQty: 1, Price: 100}
    s := service.New(&fakeCart{}, &fakeVariant{v: v})
    _, err := s.Add(context.Background(), uuid.New(), uuid.New(), 2)
    require.ErrorIs(t, err, domain.ErrOutOfStock)
}

func TestAdd_CumulativeOverTen(t *testing.T) {
    v := &productdomain.Variant{StockQty: 100, Price: 100}
    existing := &domain.CartItem{Qty: 8}
    s := service.New(&fakeCart{findByVariant: existing}, &fakeVariant{v: v})
    _, err := s.Add(context.Background(), uuid.New(), uuid.New(), 5)
    require.ErrorIs(t, err, domain.ErrQtyExceedsMax)
}

func TestAdd_HappyPathReturnsUpsertResult(t *testing.T) {
    v := &productdomain.Variant{StockQty: 100, Price: 199000}
    out := &domain.CartItem{Qty: 3, PriceSnapshot: 199000}
    s := service.New(&fakeCart{
        findVarErr:    repo.ErrNotFound, // no existing row
        upsertReturns: out,
    }, &fakeVariant{v: v})
    got, err := s.Add(context.Background(), uuid.New(), uuid.New(), 3)
    require.NoError(t, err)
    require.Equal(t, 3, got.Qty)
}

func TestUpdateQty_RefreshesSnapshot(t *testing.T) {
    v := &productdomain.Variant{StockQty: 50, Price: 189000} // current price differs
    existing := &domain.CartItem{ID: uuid.New(), VariantID: uuid.New(), Qty: 2, PriceSnapshot: 199000}
    out := &domain.CartItem{Qty: 4, PriceSnapshot: 189000}
    s := service.New(&fakeCart{findByID: existing, updateReturns: out}, &fakeVariant{v: v})
    got, err := s.UpdateQty(context.Background(), existing.ID, uuid.New(), 4)
    require.NoError(t, err)
    require.Equal(t, 189000.0, got.PriceSnapshot)
}
```

- [ ] **Step 3: Run unit tests**

Run: `go test -v ./internal/cart/service/...`
Expected: 6 PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/cart/repo/ internal/cart/service/
git commit -m "feat(cart): service with stock/qty/snapshot business rules"
```

---

### Task 20: cart handler + routes + httptest

**Files:**
- Create: `internal/cart/handler/handler.go`
- Create: `internal/cart/handler/routes.go`
- Create: `internal/cart/handler/handler_test.go`

- [ ] **Step 1: Write `handler/handler.go`**

```go
package handler

import (
    "fmt"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
    "github.com/wearwhere/wearwhere_be/internal/cart/domain"
    "github.com/wearwhere/wearwhere_be/internal/cart/service"
    "github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type Handler struct{ svc *service.Service }

func New(s *service.Service) *Handler { return &Handler{svc: s} }

func money(v float64) string { return fmt.Sprintf("%.2f", v) }

func toItemResponse(v *domain.CartItemView) domain.CartItemResponse {
    out := domain.CartItemResponse{
        ID:                v.ID.String(),
        Qty:               v.Qty,
        PriceSnapshot:     money(v.PriceSnapshot),
        CurrentPrice:      money(v.CurrentPrice),
        PriceChanged:      v.PriceSnapshot != v.CurrentPrice,
        SubtotalSnapshot:  money(v.PriceSnapshot * float64(v.Qty)),
        SubtotalCurrent:   money(v.CurrentPrice * float64(v.Qty)),
        Currency:          v.CurrencySnapshot,
        Unavailable:       v.Unavailable,
        UnavailableReason: v.UnavailableReason,
        AddedAt:           v.AddedAt.UTC().Format("2006-01-02T15:04:05Z"),
    }
    out.Variant.ID = v.VariantID.String()
    out.Variant.SKU = v.SKU
    out.Variant.Size = v.Size
    out.Variant.Color = v.Color
    out.Variant.ColorHex = v.ColorHex
    out.Variant.StockQty = v.StockQty
    out.Product.ID = v.ProductID.String()
    out.Product.Slug = v.ProductSlug
    out.Product.Name = v.ProductName
    out.Product.PrimaryImageURL = v.PrimaryImageURL
    out.Brand.ID = v.BrandID.String()
    out.Brand.Slug = v.BrandSlug
    out.Brand.Name = v.BrandName
    return out
}

func summary(items []domain.CartItemResponse, currency string) domain.CartSummary {
    s := domain.CartSummary{Currency: currency, ItemCount: len(items)}
    var snapTotal, curTotal float64
    for _, it := range items {
        s.TotalQty += it.Qty
        var snap, cur float64
        fmt.Sscanf(it.SubtotalSnapshot, "%f", &snap)
        fmt.Sscanf(it.SubtotalCurrent, "%f", &cur)
        snapTotal += snap
        curTotal += cur
        if it.PriceChanged {
            s.HasPriceChanges = true
        }
        if it.Unavailable {
            s.HasUnavailable = true
        }
    }
    s.TotalSnapshot = money(snapTotal)
    s.TotalCurrent = money(curTotal)
    return s
}

func (h *Handler) Get(c *gin.Context) {
    uid, _ := authmw.UserID(c)
    items, err := h.svc.List(c.Request.Context(), uid)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    resp := domain.CartResponse{Items: make([]domain.CartItemResponse, 0, len(items))}
    currency := "VND"
    for _, v := range items {
        if v.CurrencySnapshot != "" {
            currency = v.CurrencySnapshot
        }
        resp.Items = append(resp.Items, toItemResponse(v))
    }
    resp.Summary = summary(resp.Items, currency)
    c.JSON(http.StatusOK, resp)
}

func (h *Handler) Add(c *gin.Context) {
    uid, _ := authmw.UserID(c)
    var req domain.AddToCartRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        httpx.ValidationError(c, err)
        return
    }
    vid, err := uuid.Parse(req.VariantID)
    if err != nil {
        httpx.ErrorFromApp(c, domain.ErrVariantUnavailable)
        return
    }
    item, err := h.svc.Add(c.Request.Context(), uid, vid, req.Qty)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    c.JSON(http.StatusCreated, gin.H{"id": item.ID.String(), "qty": item.Qty})
}

func (h *Handler) Update(c *gin.Context) {
    uid, _ := authmw.UserID(c)
    id, err := uuid.Parse(c.Param("item_id"))
    if err != nil {
        httpx.ErrorFromApp(c, domain.ErrCartItemNotFound)
        return
    }
    var req domain.UpdateCartItemRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        httpx.ValidationError(c, err)
        return
    }
    item, err := h.svc.UpdateQty(c.Request.Context(), id, uid, req.Qty)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    c.JSON(http.StatusOK, gin.H{"id": item.ID.String(), "qty": item.Qty})
}

func (h *Handler) Delete(c *gin.Context) {
    uid, _ := authmw.UserID(c)
    id, err := uuid.Parse(c.Param("item_id"))
    if err != nil {
        httpx.ErrorFromApp(c, domain.ErrCartItemNotFound)
        return
    }
    if err := h.svc.Remove(c.Request.Context(), id, uid); err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    c.Status(http.StatusNoContent)
}

func (h *Handler) Clear(c *gin.Context) {
    uid, _ := authmw.UserID(c)
    if err := h.svc.Clear(c.Request.Context(), uid); err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    c.Status(http.StatusNoContent)
}
```

Note: the `summary` helper uses `fmt.Sscanf` to parse money strings — a hack for plan brevity. Replace with a `money(float64)`-mirroring `parseMoney` helper, or refactor `summary` to take raw floats from the view (cleaner). Implementer's choice.

- [ ] **Step 2: Write `handler/routes.go`**

```go
package handler

import "github.com/gin-gonic/gin"

func Mount(rg *gin.RouterGroup, h *Handler) {
    rg.GET("/cart", h.Get)
    rg.POST("/cart/items", h.Add)
    rg.PATCH("/cart/items/:item_id", h.Update)
    rg.DELETE("/cart/items/:item_id", h.Delete)
    rg.DELETE("/cart", h.Clear)
}
```

- [ ] **Step 3: Write `handler/handler_test.go` (minimal — meaningful coverage in E2E)**

```go
package handler_test

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
    "github.com/wearwhere/wearwhere_be/internal/cart/handler"
)

func TestCartRoutes_DeleteWithBadUUID_404(t *testing.T) {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    h := handler.New(nil)
    rg := r.Group("/me", func(c *gin.Context) { authmw.SetUserIDForTest(c, uuid.New()); c.Next() })
    handler.Mount(rg, h)

    w := httptest.NewRecorder()
    req, _ := http.NewRequest("DELETE", "/me/cart/items/not-a-uuid", nil)
    r.ServeHTTP(w, req)
    require.Equal(t, http.StatusNotFound, w.Code)
}
```

- [ ] **Step 4: Run tests**

Run: `go test -v ./internal/cart/handler/...`
Expected: 1 PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cart/handler/
git commit -m "feat(cart): handler + routes with summary computation"
```

---

### Task 21: Wire cart into `cmd/api/main.go`

**Files:**
- Modify: `cmd/api/main.go`

- [ ] **Step 1: Add imports**

```go
carthandler "github.com/wearwhere/wearwhere_be/internal/cart/handler"
cartrepo "github.com/wearwhere/wearwhere_be/internal/cart/repo"
cartservice "github.com/wearwhere/wearwhere_be/internal/cart/service"
```

- [ ] **Step 2: Construct dependencies**

```go
cartRepo := cartrepo.NewCartPG(pgPool)
cartSvc := cartservice.New(cartRepo, variantRepo)
cartHandler := carthandler.New(cartSvc)
```

- [ ] **Step 3: Mount on `customerGroup`**

```go
carthandler.Mount(customerGroup, cartHandler)
```

- [ ] **Step 4: Build**

Run: `go build ./cmd/api`
Expected: success.

- [ ] **Step 5: Commit**

```bash
git add cmd/api/main.go
git commit -m "feat(cart): wire cart routes under /me"
```

---

### Task 22: Run full unit + integration suite

**Files:** none

- [ ] **Step 1: Reset test DB**

Run: `make test-db-reset`
Expected: all 20 migrations applied cleanly.

- [ ] **Step 2: Unit tests**

Run: `make test-unit`
Expected: all packages PASS, no race-detector warnings.

- [ ] **Step 3: Integration tests**

Run: `make test-integration`
Expected: all integration tests PASS. Cart/wishlist/customeraddr should each contribute 3–5 tests.

- [ ] **Step 4: gofmt + go vet**

Run: `gofmt -w ./... && go vet ./...`
Expected: no output (clean).

- [ ] **Step 5: Commit if formatting changed**

```bash
git status
# If gofmt modified files:
git add -u
git commit -m "chore(format): gofmt -w on sprint-2 codebase"
```

---

## Phase D — End-to-End + Finalize (Tasks 23–25)

### Task 23: Extend `cmd/api/main_test.go` with Sprint 2 customer scenario

**Files:**
- Modify: `cmd/api/main_test.go`

- [ ] **Step 1: Inspect existing E2E**

Run: `grep -n "func Test" cmd/api/main_test.go`
Note the existing test function name(s) and the helper that brings up the test server.

- [ ] **Step 2: Append Sprint 2 sub-test**

Append a new sub-test (or extend the existing happy-path test) after the Sprint 1 product-creation flow:

```go
// Sprint 2 customer flow — runs after Sprint 1 brand creates an active product.
// Re-uses brandSlug/productSlug/variantID from the Sprint 1 portion above.
t.Run("customer shopping flow", func(t *testing.T) {
    // 1. Seed and log in customer.
    customer := seedAndLoginCustomer(t, srv)
    headers := map[string]string{"Authorization": "Bearer " + customer.AccessToken}

    // 2. Create first address — auto-default.
    addr1 := postJSON(t, srv, "/api/v1/me/addresses", headers, map[string]any{
        "label": "Nhà", "recipient_name": "T", "recipient_phone": "+84901234567",
        "address_line": "1 A", "ward": "P 1", "district": "Q 1", "city": "TP HCM",
    })
    require.Equal(t, http.StatusCreated, addr1.Code)
    var addr1Body map[string]any
    require.NoError(t, json.Unmarshal(addr1.Body.Bytes(), &addr1Body))
    require.True(t, addr1Body["is_default"].(bool))

    // 3. Create second address with is_default=true — first unset.
    addr2 := postJSON(t, srv, "/api/v1/me/addresses", headers, map[string]any{
        "label": "Office", "recipient_name": "T", "recipient_phone": "+84901234568",
        "address_line": "2 B", "ward": "P 2", "district": "Q 2", "city": "TP HCM",
        "is_default": true,
    })
    require.Equal(t, http.StatusCreated, addr2.Code)

    list := getJSON(t, srv, "/api/v1/me/addresses", headers)
    var listBody struct {
        Items []map[string]any `json:"items"`
    }
    require.NoError(t, json.Unmarshal(list.Body.Bytes(), &listBody))
    require.Len(t, listBody.Items, 2)
    var defaults int
    for _, it := range listBody.Items {
        if it["is_default"].(bool) {
            defaults++
        }
    }
    require.Equal(t, 1, defaults)

    // 4. Wishlist add (idempotent).
    w := postEmpty(t, srv, "/api/v1/me/wishlist/"+productID, headers)
    require.Equal(t, http.StatusOK, w.Code)
    w2 := postEmpty(t, srv, "/api/v1/me/wishlist/"+productID, headers)
    require.Equal(t, http.StatusOK, w2.Code)

    // 5. Wishlist contains.
    contains := getJSON(t, srv, "/api/v1/me/wishlist/contains?product_ids="+productID, headers)
    require.Equal(t, http.StatusOK, contains.Code)
    var cb domain.WishlistContainsResponse
    require.NoError(t, json.Unmarshal(contains.Body.Bytes(), &cb))
    require.True(t, cb.InWishlist[productID])

    // 6. Cart UPSERT: add 2 then 3 → 5; add 6 more → 400 QTY_EXCEEDS_MAX.
    c1 := postJSON(t, srv, "/api/v1/me/cart/items", headers, map[string]any{
        "variant_id": variantID, "qty": 2,
    })
    require.Equal(t, http.StatusCreated, c1.Code)
    c2 := postJSON(t, srv, "/api/v1/me/cart/items", headers, map[string]any{
        "variant_id": variantID, "qty": 3,
    })
    require.Equal(t, http.StatusCreated, c2.Code)

    cart := getJSON(t, srv, "/api/v1/me/cart", headers)
    require.Equal(t, http.StatusOK, cart.Code)
    var cartBody struct {
        Items []map[string]any `json:"items"`
    }
    require.NoError(t, json.Unmarshal(cart.Body.Bytes(), &cartBody))
    require.Len(t, cartBody.Items, 1)
    require.EqualValues(t, 5, cartBody.Items[0]["qty"])

    // 7. PATCH qty=10 OK, qty=11 rejected by binding.
    itemID := cartBody.Items[0]["id"].(string)
    patchOK := patchJSON(t, srv, "/api/v1/me/cart/items/"+itemID, headers, map[string]any{"qty": 10})
    require.Equal(t, http.StatusOK, patchOK.Code)
    patchBad := patchJSON(t, srv, "/api/v1/me/cart/items/"+itemID, headers, map[string]any{"qty": 11})
    require.Equal(t, http.StatusBadRequest, patchBad.Code)

    // 8. Delete the item → empty cart.
    del := delReq(t, srv, "/api/v1/me/cart/items/"+itemID, headers)
    require.Equal(t, http.StatusNoContent, del.Code)
    cart2 := getJSON(t, srv, "/api/v1/me/cart", headers)
    var c2Body struct {
        Items []any `json:"items"`
    }
    require.NoError(t, json.Unmarshal(cart2.Body.Bytes(), &c2Body))
    require.Len(t, c2Body.Items, 0)

    // 9. Delete default address → promote remaining.
    delAddr := delReq(t, srv, "/api/v1/me/addresses/"+addr2Body["id"].(string), headers)
    require.Equal(t, http.StatusNoContent, delAddr.Code)
    listAfter := getJSON(t, srv, "/api/v1/me/addresses", headers)
    require.NoError(t, json.Unmarshal(listAfter.Body.Bytes(), &listBody))
    require.Len(t, listBody.Items, 1)
    require.True(t, listBody.Items[0]["is_default"].(bool))
})
```

The helpers `seedAndLoginCustomer`, `postJSON`, `getJSON`, `postEmpty`, `patchJSON`, `delReq` either already exist in `main_test.go` from Sprint 1 or need to be added. Reuse Sprint 1's HTTP helpers — extend them only if signatures differ.

`addr2Body` is parsed similarly to `addr1Body` — add the unmarshal if not shown.

- [ ] **Step 3: Run E2E**

Run: `make test-integration`
Expected: the new sub-test PASSes in ~5 seconds.

- [ ] **Step 4: Commit**

```bash
git add cmd/api/main_test.go
git commit -m "test(e2e): customer shopping flow (cart, wishlist, address)"
```

---

### Task 24: Self-review + manual smoke

**Files:** none (operational task)

- [ ] **Step 1: Final full test run**

Run: `make test-unit && make test-integration`
Expected: both green.

- [ ] **Step 2: Manual smoke (optional)**

Document a quick curl sequence in the commit body or PR description:

```bash
TOKEN=$(curl -sX POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"customer@test.local","password":"..."}' | jq -r .access_token)

curl -sX POST http://localhost:8080/api/v1/me/addresses \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"label":"Nhà","recipient_name":"X","recipient_phone":"+84901234567",
       "address_line":"1 A","ward":"P 1","district":"Q 1","city":"TP HCM"}'

curl -sX POST http://localhost:8080/api/v1/me/cart/items \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"variant_id":"<uuid>","qty":2}'

curl -sH "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/me/cart
```

- [ ] **Step 3: No commit** (verification only).

---

### Task 25: Push branch + open PR

**Files:** none

- [ ] **Step 1: Pull-rebase to be safe**

Run: `git pull --rebase`
Expected: no conflicts (branch is solo).

- [ ] **Step 2: Push**

Run: `git push`
Expected: branch updated on origin.

- [ ] **Step 3: Open PR via gh CLI**

Run:
```bash
gh pr create --base main --head sprint-2-customer-shopping \
  --title "Sprint 2: Customer shopping (cart, wishlist, address book)" \
  --body "$(cat <<'EOF'
## Summary
- Cart UPSERT with snapshot + 10-qty clamp + unavailability flagging
- Wishlist idempotent add/remove + paginated list + bulk-contains check
- Customer address book with default-swap and oldest-promotion soft-delete

## Test plan
- [x] `make test-unit` green
- [x] `make test-integration` green (incl. new E2E customer sub-test)
- [x] Manual curl smoke documented in spec

Spec: docs/superpowers/specs/2026-05-21-sprint-2-customer-shopping-design.md
Plan: docs/superpowers/plans/2026-05-21-sprint-2-customer-shopping.md

🤖 Generated with Claude Code
EOF
)"
```
Expected: PR URL printed.

- [ ] **Step 4: No commit needed.** Branch is on origin, PR is open.

---

## Self-Review Notes

This plan covers every section of the spec:

| Spec Section | Plan Coverage |
|---|---|
| 1. Purpose & scope | Phases A–D collectively scope to cart + wishlist + address book; stock reservation and notifications are explicitly excluded throughout. |
| 2. Module structure | Task 6–9 (customeraddr), 11–14 (wishlist), 17–20 (cart) build the three flat modules. Task 10/15/21 wire to main.go. |
| 3. Data model | Tasks 1–3 create migrations 18/19/20 with the exact column lists, indexes, and constraints from the spec. |
| 4. API surface | Tasks 9, 14, 20 implement all endpoints listed in Section 4 of the spec. Error codes (Section 4 table) are defined in `domain/errors.go` in each module. |
| 5. Authorization, validation & concurrency | IDOR pattern enforced in every UPDATE/DELETE (Tasks 7, 12, 18). Cart UPSERT atomic (Task 18). Default-address swap inside tx (Task 7). DTO validation tags identical to spec (Tasks 6, 11, 17). |
| 6. Testing strategy | Fixtures extended in Task 5. Coverage priorities mapped: cart UPSERT/clamp (Task 18), IDOR (Tasks 7, 18), default swap (Task 7), soft-deleted product in cart (Task 18), wishlist idempotency (Task 12), price snapshot refresh (Task 19), wishlist contains (Task 12). E2E covers all the priorities in Task 23. |
| 7. Implementation order | Plan phases A→B→C→D match spec's implementation order (foundation/customeraddr → wishlist → cart → e2e). |

**Known plan-time compromises (called out for the implementer):**
- The `summary` helper in Task 20's handler uses `fmt.Sscanf` to round-trip money strings — a quick way to compose totals from already-formatted strings. The implementer should refactor to operate on raw floats from the view (carry them through `toItemResponse` separately) to avoid the round-trip.
