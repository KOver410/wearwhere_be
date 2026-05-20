# Sprint 2 — Customer Shopping (Cart, Wishlist, Address Book)

**Date:** 2026-05-21
**Author:** AnhND184 (brainstormed with Claude)
**Status:** Approved design, ready for implementation plan

## 1. Purpose & Scope

Build the customer-side "shopping state" of WearWhere, sitting on top of the Sprint 1 catalog. This is **Sprint 2** of the 3-sprint shopping decomposition.

### In scope

| SRS UC | Module | Description |
|---|---|---|
| UC14 | cart | Add product variant to cart (validate stock + size/color) |
| UC15 | cart | Update quantity / remove item / clear cart |
| UC16 | wishlist | Add/remove product from wishlist; bulk-contains check |
| — | customeraddr | Customer address book CRUD (preparation for UC17 checkout) |

### Out of scope (deferred to later sprints)

- **Sprint 3** — Stock reservation 30-min hold (UC14 business rule), Order placement (UC17–UC19), Tracking (UC20), History (UC21), Cancellation (UC22). Reservation logic is bundled with order creation because that is where real stock contention happens.
- **Wishlist notifications** — UC16 business rule "Notify user if wishlist item goes on sale or out of stock" requires a notification subsystem (in-app + email) that does not yet exist. Tracked as follow-up after notification module ships.
- **Payment integration** — Momo / VNPay (deferred indefinitely per Sprint 1).
- **Admin / Social / AI** — same exclusions as Sprint 1.

### Constraint (by design, not deferral)

All cart, wishlist, and address endpoints require authentication. SRS UC14 precondition "User logged in" is enforced as an absolute boundary — there is no anonymous cart, no guest-cart-merge-on-login, and no session-cookie-based cart. Frontend is responsible for redirecting unauthenticated users to login before exposing add-to-cart UI.

### Scope boundary

After Sprint 2, an authenticated customer can:
- Add a variant to cart (UPSERT semantics — adding same variant again increments qty, clamped to 10).
- Update quantity or remove items; see price-change and unavailability warnings.
- Toggle products in/out of wishlist; bulk-check which products in a catalog grid are currently wishlisted.
- Manage a personal address book with one default address for shipping.

No order, no stock deduction, no checkout flow — those land in Sprint 3.

---

## 2. Module Structure

Mirror Sprint 1's flat layout (`brand/`, `product/`). Three new modules:

```
internal/
├── auth/              (existing)
├── brand/             (existing)
├── product/           (existing)
├── cart/
│   ├── domain/        — Cart, CartItem, errors, DTOs
│   ├── repo/          — cart_pg.go
│   ├── service/       — cart_service.go
│   └── handler/       — cart_handler.go, routes.go
├── wishlist/
│   ├── domain/        — WishlistItem, errors, DTOs
│   ├── repo/          — wishlist_pg.go
│   ├── service/       — wishlist_service.go
│   └── handler/       — wishlist_handler.go, routes.go
└── customeraddr/
    ├── domain/        — CustomerAddress, errors, DTOs
    ├── repo/          — customer_address_pg.go
    ├── service/       — customer_address_service.go
    └── handler/       — customer_address_handler.go, routes.go
```

### Dependency direction

- `cart` depends on `product` (read variant + product + brand + primary image; no writes back).
- `wishlist` depends on `product` (verify product exists and is queryable).
- `customeraddr` depends only on `auth` (user_id from JWT).
- Sprint 3 `order` (future) will depend on `cart` (snapshot cart at place-order time) and `customeraddr` (selected shipping address).

### Route mounting (`cmd/api/main.go`)

```go
// /api/v1 already wraps OptionalAuth + RateLimit.
customerRoutes := v1.Group("/me",
    middleware.RequireAuth(jwtIssuer),
    middleware.RequireRole(domain.RoleCustomer),
)
cartHandler.Mount(customerRoutes)            // /me/cart*
wishlistHandler.Mount(customerRoutes)        // /me/wishlist*
customerAddrHandler.Mount(customerRoutes)    // /me/addresses*
```

`RoleCustomer = "customer"` already exists in `internal/auth/domain/role.go`; `RequireRole(domain.RoleCustomer)` and `authmw.UserID(c)` are reused as-is. No `BrandContext`-style middleware is needed — customer routes operate directly on `userID`.

---

## 3. Data Model

Three new tables, no new Postgres extensions, no new triggers.

### `cart_items`

| Column | Type | Notes |
|---|---|---|
| `id` | `UUID PK DEFAULT uuid_generate_v4()` | |
| `user_id` | `UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE` | |
| `variant_id` | `UUID NOT NULL REFERENCES product_variants(id) ON DELETE CASCADE` | |
| `qty` | `INT NOT NULL CHECK (qty BETWEEN 1 AND 10)` | UC15 business rule: max 10 per item |
| `price_snapshot` | `NUMERIC(12,2) NOT NULL CHECK (price_snapshot > 0)` | Variant.price at the time of add (or last PATCH) |
| `currency_snapshot` | `CHAR(3) NOT NULL DEFAULT 'VND'` | |
| `added_at` | `TIMESTAMPTZ NOT NULL DEFAULT NOW()` | |
| `updated_at` | `TIMESTAMPTZ NOT NULL DEFAULT NOW()` | |

Indexes:
- `UNIQUE (user_id, variant_id)` — at most one row per (user, variant). Re-adding the same variant performs UPSERT-with-increment (UC14 alt 3c).
- `(user_id)` BTREE — list cart for user.

**No soft delete.** Removing an item is a hard `DELETE`.

**Price snapshot rationale:** the user sees a stable price after adding. `GET /me/cart` joins to `product_variants` to fetch `current_price`; if `price_snapshot ≠ current_price`, response sets `price_changed=true`. The snapshot is refreshed on explicit `PATCH /me/cart/items/:id` (the user re-acknowledged the item).

### `wishlist_items`

| Column | Type | Notes |
|---|---|---|
| `user_id` | `UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE` | |
| `product_id` | `UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE` | |
| `added_at` | `TIMESTAMPTZ NOT NULL DEFAULT NOW()` | |
| `PRIMARY KEY (user_id, product_id)` | | |

Index: `(user_id, added_at DESC)` — list newest-first.

Granularity = product, not variant (SRS UC16: "save favorite products"). The user wishlists "this dress", not "the red M variant of this dress".

### `customer_addresses`

| Column | Type | Notes |
|---|---|---|
| `id` | `UUID PK DEFAULT uuid_generate_v4()` | |
| `user_id` | `UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE` | |
| `label` | `VARCHAR(40) NOT NULL` | "Nhà", "Văn phòng", "Khác" |
| `recipient_name` | `VARCHAR(120) NOT NULL` | May differ from account holder (gifting / household) |
| `recipient_phone` | `VARCHAR(20) NOT NULL` | E.164 |
| `address_line` | `VARCHAR(255) NOT NULL` | |
| `ward` | `VARCHAR(80) NOT NULL` | Phường/xã |
| `district` | `VARCHAR(80) NOT NULL` | Quận/huyện |
| `city` | `VARCHAR(80) NOT NULL` | Tỉnh/thành |
| `country` | `CHAR(2) NOT NULL DEFAULT 'VN'` | ISO 3166-1 alpha-2 |
| `postal_code` | `VARCHAR(20)` | |
| `note` | `VARCHAR(255)` | Delivery instructions ("Cổng số 5, gọi trước 5p") |
| `is_default` | `BOOL NOT NULL DEFAULT false` | At most one per user (live rows) |
| `created_at, updated_at, deleted_at` | `TIMESTAMPTZ` | Soft delete (Sprint 3 orders will FK to this) |

Indexes:
- `UNIQUE (user_id) WHERE is_default AND deleted_at IS NULL` — enforces "exactly one default among live rows".
- `(user_id, deleted_at)` — list addresses for user.

**Differences vs `brand_addresses`:** no `latitude/longitude/is_public`; adds `recipient_name/recipient_phone/note`; uses `is_default` (customer-friendly term) instead of `is_primary`.

### Migrations (continue from Sprint 1's 000016)

| # | File | Purpose |
|---|---|---|
| 000017 | `create_cart_items.up.sql` | Table + UNIQUE constraint + (user_id) index |
| 000018 | `create_wishlist_items.up.sql` | Table + (user_id, added_at DESC) index |
| 000019 | `create_customer_addresses.up.sql` | Table + 2 indexes |

Each ships with a matching `.down.sql` that drops in reverse order. No new extensions, no new enums, no new triggers.

### Cascade & filter behaviors

- **Product/variant soft-deleted (`deleted_at IS NOT NULL`)** — this is the normal path. Cart query LEFT JOINs and emits `unavailable=true` + `unavailable_reason` for the affected item; row is NOT auto-removed (user explicitly removes). Wishlist query filters out soft-deleted products entirely (they no longer exist from the user's perspective).
- **User hard-deleted** — `ON DELETE CASCADE` cleans cart_items, wishlist_items, customer_addresses. This is the documented user-deletion path (UC09).
- **Product/variant hard delete is not an exposed operation in any sprint.** Both are soft-deleted via `deleted_at`. The `ON DELETE CASCADE` on `cart_items.variant_id` and `wishlist_items.product_id` is a safety net for direct DB cleanup, not an operational path. Note: Sprint 1's `product_variants.product_id` has no CASCADE, so a raw `DELETE FROM products` would be FK-blocked by variants — by design.

---

## 4. API Surface

All endpoints under `/api/v1/me/*`, chain `OptionalAuth + RateLimit + RequireAuth + RequireRole(customer)`.

### Cart endpoints

| Method | Path | UC | Notes |
|---|---|---|---|
| `GET` | `/me/cart` | UC14/15 | Full cart with variant + product + brand + primary image |
| `POST` | `/me/cart/items` | UC14 | Add variant; existing → increment qty (clamped ≤10) |
| `PATCH` | `/me/cart/items/:item_id` | UC15 | Update qty (refreshes price_snapshot) |
| `DELETE` | `/me/cart/items/:item_id` | UC15 | Remove single item |
| `DELETE` | `/me/cart` | — | Clear entire cart (used by Sprint 3 after place-order, and customer-facing "empty cart" UX) |

**`POST /me/cart/items` request:**
```json
{ "variant_id": "uuid", "qty": 2 }
```

Service pipeline:
1. Validate variant exists with `is_active=true AND deleted_at IS NULL`, parent product has `status='active' AND deleted_at IS NULL`. Else → 409 `VARIANT_UNAVAILABLE`.
2. Validate `1 ≤ qty ≤ 10`. Else → 400 `QTY_EXCEEDS_MAX` (gin binding catches range; service double-checks for safety).
3. Compute `proposed_qty`:
   - New row: `proposed_qty = req.qty`.
   - Existing row: `proposed_qty = min(existing.qty + req.qty, 10)`. If `existing.qty + req.qty > 10`, return 400 `QTY_EXCEEDS_MAX` with `details: {max_qty: 10, current_qty: existing.qty, max_addable: 10 - existing.qty}`.
4. Stock check: `variant.stock_qty ≥ proposed_qty`. Else → 409 `VARIANT_OUT_OF_STOCK` with `details: {stock_qty: variant.stock_qty}`.
5. UPSERT:
```sql
INSERT INTO cart_items (id, user_id, variant_id, qty, price_snapshot, currency_snapshot)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id, variant_id) DO UPDATE
SET qty = LEAST(cart_items.qty + EXCLUDED.qty, 10),
    price_snapshot = EXCLUDED.price_snapshot,
    updated_at = NOW()
RETURNING *;
```

**`PATCH /me/cart/items/:item_id` request:**
```json
{ "qty": 3 }
```

Pipeline: verify ownership via `WHERE id=$id AND user_id=$user_id` (else 404 `CART_ITEM_NOT_FOUND`) → range check → stock check (else 409 `VARIANT_OUT_OF_STOCK`, UC15 alt 3a) → set `qty + price_snapshot=variant.price + updated_at=NOW()`.

**`GET /me/cart` response:**
```json
{
  "items": [
    {
      "id": "cart-item-uuid",
      "qty": 2,
      "price_snapshot": "199000.00",
      "current_price": "189000.00",
      "price_changed": true,
      "subtotal_snapshot": "398000.00",
      "subtotal_current": "378000.00",
      "unavailable": false,
      "unavailable_reason": null,
      "added_at": "...",
      "variant": {
        "id": "...", "sku": "...",
        "size": "M", "color": "Black", "color_hex": "#000000",
        "stock_qty": 5
      },
      "product": {
        "id": "...", "slug": "...", "name": "...",
        "primary_image_url": "..."
      },
      "brand": { "id": "...", "slug": "...", "name": "..." }
    }
  ],
  "summary": {
    "item_count": 1,
    "total_qty": 2,
    "total_snapshot": "398000.00",
    "total_current": "378000.00",
    "currency": "VND",
    "has_price_changes": true,
    "has_unavailable": false
  }
}
```

`unavailable=true` is set when variant is soft-deleted/inactive or parent product is not in `status='active'`. `unavailable_reason` is one of `"variant_inactive" | "variant_deleted" | "product_unavailable"`. Frontend shows "Sản phẩm không còn bán" with a remove-from-cart CTA.

Cart is not grouped by brand in the response. Frontend groups using `items[].brand.id`. Sprint 3 checkout backend will compute brand groupings server-side when shipping fees are involved.

### Wishlist endpoints

| Method | Path | UC | Notes |
|---|---|---|---|
| `GET` | `/me/wishlist` | UC16 | Paginated; default limit=24, max=60 |
| `POST` | `/me/wishlist/:product_id` | UC16 | Idempotent add — already present → 200 OK |
| `DELETE` | `/me/wishlist/:product_id` | UC16 | Idempotent remove — not present → 204 |
| `GET` | `/me/wishlist/contains?product_ids=uuid1,uuid2,...` | — | Bulk membership check for catalog heart-icon rendering. Max 60 ids per call. |

UC16 alt 2a "Already in wishlist: Remove instead" is a frontend toggle concern. Backend exposes two clean, idempotent verbs.

`GET /me/wishlist` response items mirror catalog product cards: `id, slug, name, primary_image_url, brand{id,slug,name}, min_price (computed from variants), added_at`. Pagination envelope identical to Sprint 1.

`GET /me/wishlist/contains` response:
```json
{ "in_wishlist": { "uuid1": true, "uuid2": false, ... } }
```

### Customer Address endpoints

| Method | Path | UC | Notes |
|---|---|---|---|
| `GET` | `/me/addresses` | — | List all live (no pagination — typical <10 per user) |
| `POST` | `/me/addresses` | — | Create; if it is the first live address, auto-set `is_default=true` regardless of request body |
| `GET` | `/me/addresses/:id` | — | Get single |
| `PATCH` | `/me/addresses/:id` | — | Update; if request includes `is_default=true`, run swap in transaction |
| `DELETE` | `/me/addresses/:id` | — | Soft delete; if deleted row was default, auto-promote oldest live address to default |

**Default swap transaction (`PATCH ... is_default=true`):**
```sql
BEGIN;
UPDATE customer_addresses SET is_default=false, updated_at=NOW()
 WHERE user_id=$user_id AND is_default=true AND deleted_at IS NULL AND id<>$id;
UPDATE customer_addresses SET is_default=true, updated_at=NOW(), ...other fields...
 WHERE id=$id AND user_id=$user_id AND deleted_at IS NULL
RETURNING *;
COMMIT;
```

The partial unique index `UNIQUE (user_id) WHERE is_default AND deleted_at IS NULL` is the safety net; logic bugs surface as constraint violations at the second UPDATE.

**Soft delete + default promotion:**
```sql
BEGIN;
UPDATE customer_addresses SET deleted_at=NOW(), is_default=false
 WHERE id=$id AND user_id=$user_id AND deleted_at IS NULL
RETURNING is_default AS was_default;
-- if was_default:
UPDATE customer_addresses SET is_default=true, updated_at=NOW()
 WHERE id = (
   SELECT id FROM customer_addresses
    WHERE user_id=$user_id AND deleted_at IS NULL
    ORDER BY created_at ASC LIMIT 1
 );
COMMIT;
```

### Error codes (Sprint 2 additions)

| Code | HTTP | When |
|---|---|---|
| `VARIANT_UNAVAILABLE` | 409 | Variant inactive/soft-deleted or parent product not `status='active'` |
| `VARIANT_OUT_OF_STOCK` | 409 | Requested qty exceeds `variant.stock_qty`. `details: {stock_qty: n}` |
| `QTY_EXCEEDS_MAX` | 400 | qty > 10 or cumulative qty > 10. `details: {max_qty: 10, current_qty, max_addable}` |
| `CART_ITEM_NOT_FOUND` | 404 | Item not owned by caller or does not exist |
| `INVALID_PHONE` | 400 | recipient_phone fails E.164 validation |

Reused from Sprint 1: `ADDRESS_NOT_FOUND` (URL `/me/addresses` vs `/brand/me/addresses` disambiguates context), `PRODUCT_NOT_FOUND` (for wishlist add with non-existent product), `VALIDATION_ERROR` (gin binding failures).

### Pagination

`GET /me/wishlist` reuses Sprint 1's envelope:
```json
{
  "items": [ ... ],
  "pagination": { "page":1, "limit":24, "total":N, "total_pages":P, "has_more":bool }
}
```

`GET /me/cart` and `GET /me/addresses` return all items in a single response (cart capped naturally by qty ≤10 per item and typical small basket sizes; addresses capped by user behavior). No pagination needed.

---

## 5. Authorization, Validation & Concurrency

### Auth chain

All endpoints: `OptionalAuth + RateLimit + RequireAuth + RequireRole(domain.RoleCustomer)`. The `/api/v1` group already supplies `OptionalAuth + RateLimit`; the `/me` sub-group adds the rest. No `BrandContext`-style middleware — customer routes use `userID := authmw.UserID(c)` directly.

### IDOR pattern (mirror Sprint 1)

Every write filters `user_id = $user_id` in the same statement that targets the resource — no separate "check then update" sequence.

```sql
-- Cart item update pattern
UPDATE cart_items SET qty=$qty, price_snapshot=$p, updated_at=NOW()
 WHERE id = $item_id AND user_id = $user_id
RETURNING *;
-- RowsAffected = 0 → 404 CART_ITEM_NOT_FOUND
```

Wishlist composite PK `(user_id, product_id)` already encodes ownership. `DELETE FROM wishlist_items WHERE user_id=$u AND product_id=$p` is naturally IDOR-safe.

### Concurrency

- **Cart UPSERT** uses `INSERT ... ON CONFLICT (user_id, variant_id) DO UPDATE`. Atomic at the DB level — concurrent adds of the same variant for the same user serialize correctly.
- **Stock check is read-then-act** (TOCTOU possible). Accepted for Sprint 2: without stock reservation, the worst case is "user adds 5; concurrent customer buys the last 4 before this user checks out, then Sprint 3 checkout reports insufficient stock". This is the same UX failure mode that Sprint 3 reservation will close, so Sprint 2 does not over-engineer.
- **Default address swap** runs inside an explicit `pgx.Tx`. The partial unique index `UNIQUE (user_id) WHERE is_default AND deleted_at IS NULL` is the invariant-of-record — any code path that fails to unset siblings first will be rejected by the DB.

### DTO validation

```go
type AddToCartRequest struct {
    VariantID string `json:"variant_id" binding:"required,uuid"`
    Qty       int    `json:"qty"        binding:"required,min=1,max=10"`
}

type UpdateCartItemRequest struct {
    Qty int `json:"qty" binding:"required,min=1,max=10"`
}

type CreateAddressRequest struct {
    Label          string `json:"label"           binding:"required,max=40"`
    RecipientName  string `json:"recipient_name"  binding:"required,min=2,max=120"`
    RecipientPhone string `json:"recipient_phone" binding:"required,e164"`
    AddressLine    string `json:"address_line"    binding:"required,max=255"`
    Ward           string `json:"ward"            binding:"required,max=80"`
    District       string `json:"district"        binding:"required,max=80"`
    City           string `json:"city"            binding:"required,max=80"`
    Country        string `json:"country"         binding:"omitempty,iso3166_1_alpha2"`
    PostalCode     string `json:"postal_code"     binding:"omitempty,max=20"`
    Note           string `json:"note"            binding:"omitempty,max=255"`
    IsDefault      bool   `json:"is_default"`
}

type UpdateAddressRequest struct { /* all fields omitempty + pointer types for partial update */ }

type WishlistContainsQuery struct {
    ProductIDs []string `form:"product_ids" binding:"required,min=1,max=60,dive,uuid"`
}
```

`e164` and `iso3166_1_alpha2` are built-in gin/validator v10 tags. No new custom validators are needed.

### Rate limiting

The `/api/v1` group's existing `RateLimit(rdb, cfg.Limit.RateLimitPerMin)` (default 100 req/min/user) covers Sprint 2 endpoints. If a hot loop emerges (e.g., catalog page firing many `/me/wishlist/contains` calls), introduce a per-route limit then — not pre-emptively.

---

## 6. Testing Strategy

Mirror Sprint 1's three-tier structure (unit + integration + E2E). Test database, transaction-rollback isolation, hand-written mocks, build tags — all the same.

### Test fixtures (extend `internal/testfixtures/`)

- `SeedCustomer(t, db) → User` — user with `role=customer, status=active, email_verified_at=NOW()`.
- `SeedCartItem(t, db, userID, variantID, qty) → CartItem` — also computes `price_snapshot` from current variant.
- `SeedWishlistItem(t, db, userID, productID)`.
- `SeedCustomerAddress(t, db, userID, opts) → CustomerAddress` — opts with sensible Vietnam defaults; `IsDefault` honored.

Reuse `SeedBrand`, `SeedProduct`, `SeedVariant`, `SeedCategory` from Sprint 1.

### Coverage priorities

| Area | Why mandatory |
|---|---|
| Cart UPSERT increment + clamp ≤10 | Off-by-one easy; both branches (new vs existing) must be exercised |
| Cart stock check → `OUT_OF_STOCK` translation | Service-to-error-code mapping is a security-relevant contract |
| IDOR: user A cannot read/update/delete user B's cart, wishlist, or address | Core security boundary |
| Default address swap inside transaction | Partial unique index is necessary but not sufficient — test the logic that unsets siblings first |
| Soft-deleted product surfaces `unavailable=true` in `GET /me/cart` | Cross-module behavior between Sprint 1 soft-delete and Sprint 2 cart query |
| Wishlist idempotent add (200 on re-add) and remove (204 on re-remove) | UC16 alt 2a expects no error on already-in-state |
| Wishlist contains: empty list, all-true, mixed, IDs not in wishlist | Boundary cases for catalog grid heart-icon |
| Customer address delete-default → promote oldest live | Easy to miss; failing means user is "no default address" after delete |
| Price snapshot refresh on PATCH qty | Without this, snapshot stales indefinitely after price changes |
| Cart cascade: deleting variant removes cart_items; deleting product removes wishlist | Verifies FK ON DELETE CASCADE semantics |

### Test layout

```
internal/
├── cart/
│   ├── repo/cart_pg_test.go             // integration: UPSERT, IDOR, cascade
│   ├── service/cart_service_test.go     // unit: stock/qty validation, error translation
│   └── handler/cart_handler_test.go     // httptest
├── wishlist/
│   ├── repo/wishlist_pg_test.go         // integration
│   ├── service/wishlist_service_test.go // unit
│   └── handler/wishlist_handler_test.go
└── customeraddr/
    ├── repo/customer_address_pg_test.go    // integration: default swap, soft-delete promotion
    ├── service/customer_address_service_test.go
    └── handler/customer_address_handler_test.go
```

Integration tests use the existing `//go:build integration` tag and `TEST_DATABASE_URL` env var from Sprint 1's `make test-integration` target.

### E2E extension (`cmd/api/main_test.go`)

Append a Sprint 2 customer scenario after Sprint 1's brand-creates-product flow:

```
9.  Seed: customer user → POST /auth/login → token2
10. POST /me/addresses (first → auto-default true)
11. POST /me/addresses (second, is_default=true → first auto-unset)
12. GET /me/addresses → 2 items, exactly one with is_default=true
13. POST /me/wishlist/:product_id → GET /me/wishlist (1 item)
14. POST /me/wishlist/:product_id (again) → 200 OK (idempotent)
15. GET /me/wishlist/contains?product_ids=<id>,<other> → mixed map
16. POST /me/cart/items {variant_id, qty:2} → GET /me/cart (1 item, qty 2)
17. POST /me/cart/items {variant_id, qty:3} → GET (qty=5; UPSERT increment)
18. PATCH /me/cart/items/:id {qty:10} → ok
19. PATCH /me/cart/items/:id {qty:11} → 400 QTY_EXCEEDS_MAX
20. DELETE /me/cart/items/:id → GET (empty)
21. DELETE /me/addresses/:default_id → next address promoted to default
```

Runs alongside Sprint 1 scenario in ~5–7 seconds. Catches cross-module wiring breaks.

### Out of scope for Sprint 2 testing

- Load tests
- Fuzz testing
- CI workflow setup
- Concurrent-add race tests (deferred with reservation logic to Sprint 3)

---

## 7. Implementation Order (preview for plan)

Driven by dependency direction (fewest dependencies first):

1. **Migrations 000017–000019** — `make migrate-up` runs cleanly on both `wearwhere` and `wearwhere_test` databases.
2. **`internal/customeraddr/`** — standalone, no `product` dependency. Onboarding-friendly first module.
3. **`internal/wishlist/`** — depends only on `product.Repo.FindByID` for existence check.
4. **`internal/cart/`** — most complex (UPSERT, snapshot, unavailable join, stock check).
5. **`internal/testfixtures/`** — extend with customer/cart/wishlist/address seeds.
6. **Wire in `cmd/api/main.go`** — construct repos/services/handlers, mount under `/me`.
7. **E2E extend** — append Sprint 2 customer scenario to `cmd/api/main_test.go`.
8. **Tests pass** — `make test-unit` + `make test-integration` green.
9. **Manual smoke** — curl flow: login as customer → add address → wishlist → cart UPSERT → patch qty → delete.

Each module follows the same intra-order: `domain → repo → service → handler → wire`. Beads tasks will break this into ~3–4 sub-tasks per module when the implementation plan is written.
