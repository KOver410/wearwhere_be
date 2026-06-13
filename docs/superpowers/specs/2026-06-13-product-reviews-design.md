# Product Reviews — Design

**Date:** 2026-06-13
**Scope:** SRS UC37 (Submit Product Review), UC38 (View Reviews) — first sub-project of the Social & Community group.
**Status:** Approved (design).

## 1. Goal & Context

Implement customer product reviews: verified purchasers rate (1–5 stars) and write a text review with
optional size/fit feedback; anyone can read reviews on a product, and the product's average rating and
review count are shown prominently.

This is the **Go backend** repo. Reviews depend only on existing `orders`/`sub_orders`/`order_items`
and `products`/`users` — no dependency on other Social & Community sub-projects.

### Decomposition

The Social & Community group (UC32–40) is split into four independent sub-projects, built in order:
**1. Reviews (this spec)** → 2. OOTD (post/like/comment) → 3. Follow (brand/user) → 4. Moderation
(report/block). Each gets its own spec → plan → implementation cycle.

### Decisions (from brainstorming)

- **One review per `(user, product)`**, editable and soft-deletable by its author.
- **Average rating denormalized** onto `products` (`avg_rating`, `review_count`), recomputed in the same
  transaction on every create/update/delete.
- **Size/fit feedback** included as an optional `fit` field (`small` | `true` | `large`).
- **Photos are OUT of scope for now** (deferred to a later iteration). Reviews are text-only.
- **Verified purchase required**: a user may review a product only if they have a delivered order line
  for it.
- **Moderation hook only**: reviews publish immediately (`status='published'`); a `status` column lets
  the future Moderation sub-project hide them. No report/takedown logic here.
- **Brand reply (UC48)** is a Brand Partner feature — out of scope.

## 2. Architecture & module

New module `internal/review/` following existing module layout (domain → repo → service → handler),
mirroring patterns in `internal/wishlist`, `internal/cart`, etc.

- `domain/` — `Review` entity, `Fit` enum, request/response DTOs, sentinel `*httpx.AppError` helpers.
- `repo/` — `Repo` interface + Postgres impl: create, update, soft-delete, get-by-id, list-by-product
  (with filter/sort/pagination), `HasDeliveredPurchase(userID, productID)`, and the aggregate
  recompute. A small `DBTX` interface (matching the existing repo pattern) so service can run
  create+recompute in one transaction.
- `service/` — validation (rating range, body length, fit values), verified-purchase gate, ownership
  checks, orchestration of write + aggregate recompute in a transaction.
- `handler/` + `routes.go` — Gin handlers; `MountReviewsPublic` (read) and a customer-authed mount for
  write/edit/delete.

The `products` denormalized columns are also surfaced through the existing `product` catalog DTOs
(small additive change, no new joins).

## 3. Data model & migrations

Two migrations (numbered next-free on this branch off `main`; `main` is at `000033` after the merged
store-discovery feature, so these are `000034` and `000035`).

### New table `product_reviews`

```sql
CREATE TABLE product_reviews (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    product_id  UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating      SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    body        TEXT NOT NULL,                                  -- min 20 chars (enforced in service)
    fit         TEXT CHECK (fit IN ('small','true','large')),  -- size/fit, nullable
    status      TEXT NOT NULL DEFAULT 'published' CHECK (status IN ('published','hidden')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);
-- one active review per (user, product)
CREATE UNIQUE INDEX idx_reviews_user_product
    ON product_reviews (product_id, user_id) WHERE deleted_at IS NULL;
-- public listing path
CREATE INDEX idx_reviews_product
    ON product_reviews (product_id) WHERE deleted_at IS NULL AND status = 'published';
```

- Every review is a verified purchase (enforced at write), so no separate `verified` flag is needed —
  the badge is always on.
- Photos: deferred. When added, either an ALTER adding `photo_urls TEXT[]` or a separate table.

### Denormalized columns on `products`

```sql
ALTER TABLE products
  ADD COLUMN avg_rating   NUMERIC(2,1) NOT NULL DEFAULT 0,   -- 0.0 .. 5.0
  ADD COLUMN review_count INT          NOT NULL DEFAULT 0;
```

Recomputed in the same transaction as each review write:
```sql
UPDATE products p SET
  avg_rating   = COALESCE((SELECT AVG(rating) FROM product_reviews
                           WHERE product_id = p.id AND deleted_at IS NULL AND status='published'), 0),
  review_count = (SELECT COUNT(*) FROM product_reviews
                  WHERE product_id = p.id AND deleted_at IS NULL AND status='published')
WHERE p.id = $1;
```

## 4. Verified-purchase rule

A user may review product `X` only if a delivered purchase exists:
```sql
SELECT EXISTS (
  SELECT 1 FROM order_items oi
  JOIN sub_orders so ON so.id = oi.sub_order_id
  JOIN orders o      ON o.id  = so.order_id
  WHERE oi.product_id = $1 AND o.user_id = $2 AND so.status = 'delivered'
);
```
Join path confirmed against the schema: `order_items.sub_order_id → sub_orders.id`,
`sub_orders.order_id → orders.id`, `orders.user_id`. `sub_orders.status='delivered'` is the verified
signal.

## 5. API endpoints

| Method & Path | Auth | Behaviour |
|---|---|---|
| `POST /products/:id/reviews` | Customer | Create review (JSON: `rating`, `body`, `fit?`). `403 NOT_VERIFIED_PURCHASE` if no delivered purchase; `409 REVIEW_EXISTS` if the user already has an active review for this product. |
| `GET /products/:id/reviews` | Public | List `published` reviews. Query: `rating` (exact 1–5 filter), `fit` (small/true/large filter), `sort` = `newest` (default) \| `rating_high` \| `rating_low`, plus pagination (`page`/`limit`, mirroring the catalog convention). Response includes `avg_rating`, `review_count`, items, pagination. |
| `PATCH /reviews/:id` | Customer (owner) | Update own review's `rating`/`body`/`fit`. `403` if not the owner, `404` if not found. |
| `DELETE /reviews/:id` | Customer (owner) | Soft-delete own review (`deleted_at`), then recompute aggregate. `403`/`404` as above. |

- All reviews are verified purchases, so "verified first" (UC38) is trivially satisfied; default sort is
  `newest`.
- Auth uses the existing `RequireAuth` middleware; ownership is checked in the service against the
  authenticated user id.

### Surfacing average rating

Add `avg_rating` and `review_count` to the existing `product` catalog detail DTO (and the product
summary DTO used by listing, if that view shows ratings). Reads the denormalized columns directly.

## 6. Error handling, validation & testing

### Validation (service-enforced, SRS business rules)
- `rating` ∈ [1,5] → else `400`.
- `body` length ≥ 20 characters → else `400`.
- `fit` ∈ {small, true, large} or empty → else `400`.

### Error responses (`pkg/httpx` AppError)
- Not a verified purchase → `403 NOT_VERIFIED_PURCHASE`.
- Already reviewed → `409 REVIEW_EXISTS`.
- Validation failures → `400`.
- Review not found → `404 REVIEW_NOT_FOUND`; not owner → `403 FORBIDDEN`.
- Product not found → `404 PRODUCT_NOT_FOUND`.

### Testing
- **Unit (service, fake repo):** rating/body-min-20/fit validation; verified-purchase gate (allow vs
  deny); ownership on update/delete; aggregate recompute is invoked on create/update/delete.
- **Integration (repo, testfixtures, `//go:build integration`):** verified-purchase query (seed
  order → sub_order delivered → order_item); unique-constraint rejection of a duplicate active review;
  list filter (`rating`, `fit`) + sort + pagination; soft-delete updates aggregate; `hidden` reviews
  excluded from list and aggregate.
- testfixtures likely needs a new helper to seed a delivered order line (`SeedDeliveredOrderItem` or
  similar) — add it if absent.

## 7. Out of scope (documented exclusions)
- **Review photos** — deferred to a later iteration.
- **Brand reply to reviews (UC48)** — Brand Partner feature.
- **Report / takedown of reviews (UC39)** — Moderation sub-project.
- **Block affecting review visibility (UC40)** — Moderation sub-project.

## 8. Branch
`feature/product-reviews`, branched off `main`. Migrations `000034`/`000035` are the next free numbers
on `main` (which is at `000033`). Note the parked `ai-stylist-chatbot` branch already uses
`000033`/`000034`; the same eventual-integration renumbering already flagged for store-discovery applies.
