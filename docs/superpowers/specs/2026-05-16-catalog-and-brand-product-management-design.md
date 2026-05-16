# Sprint 1 — Catalog & Brand Product Management

**Date:** 2026-05-16
**Author:** AnhND184 (brainstormed with Claude)
**Status:** Approved design, ready for implementation plan

## 1. Purpose & Scope

Build the foundation of the WearWhere shopping experience: brands own products, customers browse them. This is **Sprint 1** of a 3-sprint decomposition of SRS shopping features (UC10–UC22, UC42–UC44).

### In scope

| SRS UC | Description |
|---|---|
| UC10 | Search products by name / brand / style / category |
| UC11 | Filter products by price, style, brand, size, color |
| UC12 | View product detail (images, variants, brand summary) |
| UC13 | View brand profile (story, addresses, products) |
| UC42 | Brand: manage product catalog (CRUD) |
| UC43 | Brand: upload product images |
| UC44 | Brand: set product pricing & inventory (variants) |
| UC51 (partial) | Brand: manage store addresses (1 brand → N addresses) |

### Out of scope (deferred to later sprints)

- **Sprint 2** — Cart, Wishlist, customer Address book
- **Sprint 3** — Orders, Checkout (without payment gateway), Track / Cancel order
- **Payment integration** — Momo / VNPay (deferred indefinitely)
- **Admin module** — Verify brand (UC53), Suspend brand (UC78), Manage categories / style_tags (UC70/UC71), Brand self-registration flow
- **Social / Reviews** — OOTD, comments, follow brand (UC32–UC38)
- **AI features** — Recommendations, Smart Wardrobe (UC28–UC31)
- **Stock reservation** — UC14 "stock reserved 30 min" is Sprint 2 cart logic
- **Brand self-registration** — Sprint 1 seeds brands via SQL migration only

### Scope boundary

Sprint 1 delivers a system where a developer can: seed a brand via SQL, log in as that brand's owner, create a product end-to-end through the API (product → variants → images → publish), and verify the product appears in the public catalog with working search and filters. No customer-side shopping state (cart, wishlist) exists yet.

---

## 2. Module Structure

Follow the existing `internal/auth/{domain,repo,service,middleware,handler}` layout. Two new modules + one new shared package:

```
internal/
├── auth/              (existing, untouched)
├── brand/
│   ├── domain/        — Brand, BrandAddress, errors, DTOs, status enums
│   ├── repo/          — brand_pg.go, address_pg.go
│   ├── service/       — brand_service.go (own-profile read/update, address CRUD)
│   ├── middleware/    — brand_context.go (resolves brand from JWT user_id)
│   └── handler/       — brand_handler.go, address_handler.go, routes.go
├── product/
│   ├── domain/        — Product, Variant, Image, Category, StyleTag, errors, DTOs
│   ├── repo/          — product_pg.go, variant_pg.go, image_pg.go,
│   │                    category_pg.go, style_tag_pg.go
│   ├── service/       — product_service.go (brand-side write),
│   │                    catalog_service.go (public-side read + search)
│   └── handler/       — brand_product_handler.go, catalog_handler.go, routes.go
├── shared/
│   ├── storage/       (new) — interface + local.go + gcs.go (stub) + factory.go
│   └── ... (mailer, jwt, postgres, redis, validator — unchanged)
├── testfixtures/      (new) — seedBrand, seedProduct, seedCategory, seedUser helpers
└── config/
    └── config.go      (extended with StorageConfig)
```

### Why separate `brand` and `product`?

- The brand aggregate is independent (can exist with zero products); products always belong to one brand.
- Sprint 2 cart will depend only on `product` domain — it shouldn't pull in brand-write code.
- Brand profile updates (UC13 read, brand-write) and product catalog (UC10–12 read, UC42–44 write) are logically distinct concerns with their own services.

### Route mounting (in `cmd/api/main.go`)

```go
// Public (OptionalAuth + RateLimit already applied to /api/v1)
catalogHandler.Mount(v1)
//   GET /products, GET /products/by-id/:id,
//   GET /brands/:brand_slug/products/:product_slug,
//   GET /brands, GET /brands/:slug,
//   GET /categories, GET /style-tags

// Brand-only (RequireAuth + RequireRole("brand") + BrandContext)
brandRoutes := v1.Group("/brand/me",
    middleware.RequireAuth(jwtIssuer),
    middleware.RequireRole(domain.RoleBrand),
    brandmw.BrandContext(brandRepo),
)
brandHandler.Mount(brandRoutes)
brandProductHandler.Mount(brandRoutes)
```

---

## 3. Data Model (ERD)

### Entity relationships

```
users (existing)
  ↑ owner_user_id
brands ───── 1:N ───── brand_addresses
  │
  │ brand_id
  ↓
products ──── 1:N ──── product_variants
  │   │                 (image_id → product_images.id, nullable)
  │   │
  │   └──── 1:N ──── product_images (ON DELETE CASCADE)
  │
  ├── N:1 ── categories
  └── M:N ── style_tags  (via product_style_tags)
```

### Postgres extensions to enable

```sql
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS unaccent;
-- uuid-ossp and citext already enabled by auth migrations
```

### Tables

#### `brands`

| Column | Type | Notes |
|---|---|---|
| `id` | `UUID PK DEFAULT uuid_generate_v4()` | |
| `slug` | `CITEXT UNIQUE NOT NULL` | URL `/brands/{slug}`, globally unique |
| `name` | `VARCHAR(120) NOT NULL` | |
| `owner_user_id` | `UUID NOT NULL REFERENCES users(id)` | Each user can own at most 1 brand for now |
| `story` | `TEXT` | Long-form brand story / about |
| `logo_url` | `TEXT` | |
| `banner_url` | `TEXT` | |
| `website_url` | `TEXT` | |
| `status` | `brand_status NOT NULL DEFAULT 'active'` | Sprint 1: seed `active`. Future: `pending` / `suspended` |
| `verified_at` | `TIMESTAMPTZ` | Filled by future admin verify endpoint |
| `created_at, updated_at, deleted_at` | `TIMESTAMPTZ` | Soft delete |

Enum: `CREATE TYPE brand_status AS ENUM ('pending', 'active', 'suspended');`

Indexes:
- `UNIQUE (owner_user_id) WHERE deleted_at IS NULL` — enforce one-brand-per-user
- `(status, deleted_at)` — filter active brands
- GIN trigram on `name` for brand-name search (used by `GET /brands?q=`)

#### `brand_addresses`

| Column | Type | Notes |
|---|---|---|
| `id` | `UUID PK` | |
| `brand_id` | `UUID NOT NULL REFERENCES brands(id) ON DELETE CASCADE` | |
| `label` | `VARCHAR(80) NOT NULL` | "HQ", "Showroom HCM" |
| `address_line` | `VARCHAR(255) NOT NULL` | |
| `ward` | `VARCHAR(80) NOT NULL` | Phường/xã |
| `district` | `VARCHAR(80) NOT NULL` | Quận/huyện |
| `city` | `VARCHAR(80) NOT NULL` | Tỉnh/thành |
| `country` | `CHAR(2) NOT NULL DEFAULT 'VN'` | ISO 3166-1 alpha-2 |
| `postal_code` | `VARCHAR(20)` | |
| `phone` | `VARCHAR(20)` | E.164 |
| `latitude` | `NUMERIC(10,7)` | Nullable; consumed by Location module later |
| `longitude` | `NUMERIC(10,7)` | |
| `is_primary` | `BOOL NOT NULL DEFAULT false` | At most one per brand |
| `is_public` | `BOOL NOT NULL DEFAULT true` | Hide HQ/warehouse from customer view |
| `created_at, updated_at, deleted_at` | `TIMESTAMPTZ` | |

Indexes:
- `UNIQUE (brand_id) WHERE is_primary AND deleted_at IS NULL`
- `(brand_id, is_public, deleted_at)`
- `(latitude, longitude)` — pre-seeded for future Location module

#### `categories`

| Column | Type | Notes |
|---|---|---|
| `id` | `UUID PK` | |
| `slug` | `CITEXT UNIQUE NOT NULL` | |
| `name` | `VARCHAR(80) NOT NULL` | |
| `display_order` | `INT NOT NULL DEFAULT 0` | |
| `created_at, updated_at` | `TIMESTAMPTZ` | Hard delete (admin-managed) |

#### `style_tags`

| Column | Type | Notes |
|---|---|---|
| `id` | `UUID PK` | |
| `slug` | `CITEXT UNIQUE NOT NULL` | |
| `name` | `VARCHAR(80) NOT NULL` | |
| `created_at, updated_at` | `TIMESTAMPTZ` | Hard delete |

#### `products`

| Column | Type | Notes |
|---|---|---|
| `id` | `UUID PK` | |
| `brand_id` | `UUID NOT NULL REFERENCES brands(id)` | |
| `category_id` | `UUID NOT NULL REFERENCES categories(id)` | Required |
| `slug` | `CITEXT NOT NULL` | Unique within brand |
| `name` | `VARCHAR(200) NOT NULL` | |
| `description` | `TEXT` | |
| `status` | `product_status NOT NULL DEFAULT 'draft'` | |
| `currency` | `CHAR(3) NOT NULL DEFAULT 'VND'` | |
| `search_text` | `TEXT` | Denormalized, unaccented, lowercased: `name + description + brand.name`. Maintained by trigger. |
| `sold_count` | `INT NOT NULL DEFAULT 0` | Updated when orders place (Sprint 3) |
| `view_count` | `INT NOT NULL DEFAULT 0` | Fire-and-forget incremented in catalog detail handler |
| `created_at, updated_at, deleted_at` | `TIMESTAMPTZ` | Soft delete |

Enum: `CREATE TYPE product_status AS ENUM ('draft', 'active', 'archived');`

Indexes:
- `UNIQUE (brand_id, slug) WHERE deleted_at IS NULL`
- `(brand_id, status, deleted_at)`
- `(category_id, status, deleted_at)`
- `(status, deleted_at, created_at DESC)` — newest sort
- `(sold_count DESC, view_count DESC) WHERE deleted_at IS NULL AND status = 'active'` — popular sort
- `USING GIN (search_text gin_trgm_ops) WHERE deleted_at IS NULL AND status = 'active'` — fuzzy search

#### `product_variants`

| Column | Type | Notes |
|---|---|---|
| `id` | `UUID PK` | |
| `product_id` | `UUID NOT NULL REFERENCES products(id)` | |
| `sku` | `VARCHAR(64) NOT NULL` | Unique per product |
| `size` | `VARCHAR(20) NOT NULL` | Free-form (S/M/L, or 38/39/40) |
| `color` | `VARCHAR(50) NOT NULL` | Display name |
| `color_hex` | `CHAR(7)` | "#RRGGBB" for swatches |
| `price` | `NUMERIC(12,2) NOT NULL CHECK (price > 0)` | Per-variant price |
| `stock_qty` | `INT NOT NULL DEFAULT 0 CHECK (stock_qty >= 0)` | |
| `is_active` | `BOOL NOT NULL DEFAULT true` | Brand can disable without delete |
| `image_id` | `UUID REFERENCES product_images(id) ON DELETE SET NULL` | Nullable; variant-specific image (typically per-color) |
| `created_at, updated_at, deleted_at` | `TIMESTAMPTZ` | Soft delete |

Indexes / constraints:
- `UNIQUE (product_id, size, color) WHERE deleted_at IS NULL`
- `UNIQUE (product_id, sku) WHERE deleted_at IS NULL`
- `(product_id, is_active, deleted_at)`

#### `product_images`

| Column | Type | Notes |
|---|---|---|
| `id` | `UUID PK` | |
| `product_id` | `UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE` | |
| `url` | `TEXT NOT NULL` | Resolved via storage interface |
| `storage_key` | `TEXT NOT NULL` | Key for storage backend (for delete) |
| `alt_text` | `VARCHAR(200)` | |
| `sort_order` | `INT NOT NULL DEFAULT 0` | |
| `is_primary` | `BOOL NOT NULL DEFAULT false` | Card thumbnail; exactly one per product when product has ≥1 image |
| `created_at` | `TIMESTAMPTZ` | No soft delete |

Indexes:
- `(product_id, sort_order)`
- `UNIQUE (product_id) WHERE is_primary` — enforce single primary

#### `product_style_tags` (M:N)

| Column | Type | Notes |
|---|---|---|
| `product_id` | `UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE` | |
| `style_tag_id` | `UUID NOT NULL REFERENCES style_tags(id) ON DELETE CASCADE` | |
| `PRIMARY KEY (product_id, style_tag_id)` | | |

Index: `(style_tag_id, product_id)` for filter-by-tag queries.

### Triggers

**`update_product_search_text`** — runs `BEFORE INSERT OR UPDATE OF name, description, brand_id` on `products`:

```sql
NEW.search_text := unaccent(lower(
  coalesce(NEW.name, '') || ' ' ||
  coalesce(NEW.description, '') || ' ' ||
  coalesce((SELECT name FROM brands WHERE id = NEW.brand_id), '')
));
```

**`resync_brand_products_search`** — runs `AFTER UPDATE OF name` on `brands`:

```sql
UPDATE products SET updated_at = NOW()
WHERE brand_id = NEW.id AND deleted_at IS NULL;
-- The BEFORE UPDATE trigger on products re-computes search_text
```

### Migrations

Numbered continuing from existing auth migrations (last is `000005`):

| # | File | Purpose |
|---|---|---|
| 000006 | `create_extensions_search.up.sql` | Enable `pg_trgm`, `unaccent` |
| 000007 | `create_brands.up.sql` | `brand_status` enum + `brands` table + indexes |
| 000008 | `create_categories.up.sql` | `categories` table |
| 000009 | `create_style_tags.up.sql` | `style_tags` table |
| 000010 | `create_brand_addresses.up.sql` | `brand_addresses` table + indexes |
| 000011 | `create_products.up.sql` | `product_status` enum + `products` table + search trigger |
| 000012 | `create_product_images.up.sql` | `product_images` table (must precede variants — variants FK image_id) |
| 000013 | `create_product_variants.up.sql` | `product_variants` table |
| 000014 | `create_product_style_tags.up.sql` | M:N join table |
| 000015 | `seed_taxonomy.up.sql` | ~10 categories (T-shirt, Dress, Pants, Shoes, …) + ~10 style_tags (Streetwear, Minimalist, Y2K, Vintage, …). Idempotent (`ON CONFLICT DO NOTHING`). |
| 000016 | `seed_dev_brands.up.sql` | 2 demo users (role=brand) + 2 demo brands + 1-2 addresses each. Idempotent. Run only when `APP_ENV=development` — gate by checking env in Makefile target, not in SQL. |

Each migration has a corresponding `.down.sql` that drops in reverse order.

---

## 4. API Surface

All under `/api/v1` (already wrapped with `OptionalAuth + RateLimit`).

### Public endpoints (no auth required)

| Method | Path | UC | Notes |
|---|---|---|---|
| `GET` | `/products` | UC10, UC11 | Search/filter/sort/paginate |
| `GET` | `/products/by-id/:id` | UC12 | Lookup by UUID (deep link) |
| `GET` | `/brands/:brand_slug/products/:product_slug` | UC12 | Lookup by nested slug |
| `GET` | `/brands` | — | List brands (paginated, searchable) |
| `GET` | `/brands/:slug` | UC13 | Brand profile + public addresses + first page of brand products |
| `GET` | `/categories` | — | All categories (for filter UI) |
| `GET` | `/style-tags` | — | All style tags (for filter UI) |

### Brand-side endpoints (`RequireAuth + RequireRole("brand") + BrandContext`)

| Method | Path | UC |
|---|---|---|
| `GET` | `/brand/me` | — |
| `PATCH` | `/brand/me` | — |
| `GET` | `/brand/me/addresses` | UC51 |
| `POST` | `/brand/me/addresses` | UC51 |
| `PATCH` | `/brand/me/addresses/:id` | UC51 |
| `DELETE` | `/brand/me/addresses/:id` | UC51 |
| `GET` | `/brand/me/products` | UC42 |
| `POST` | `/brand/me/products` | UC42 |
| `GET` | `/brand/me/products/:id` | UC42 |
| `PATCH` | `/brand/me/products/:id` | UC42 |
| `DELETE` | `/brand/me/products/:id` | UC42 |
| `POST` | `/brand/me/products/:id/images` | UC43 |
| `PATCH` | `/brand/me/products/:id/images/:image_id` | UC43 |
| `DELETE` | `/brand/me/products/:id/images/:image_id` | UC43 |
| `POST` | `/brand/me/products/:id/variants` | UC44 |
| `PATCH` | `/brand/me/products/:id/variants/:variant_id` | UC44 |
| `DELETE` | `/brand/me/products/:id/variants/:variant_id` | UC44 |

### Query parameters for `GET /products`

| Param | Type | Notes |
|---|---|---|
| `q` | string ≤100 chars | Search query (fuzzy via pg_trgm) |
| `category` | slug | Single category slug |
| `style` | slug, repeatable | Multiple → **OR** within style filter |
| `brand` | slug | Single brand slug |
| `size` | string, repeatable | Multiple → OR (variant must have size AND stock > 0) |
| `color` | string, repeatable | Multiple → OR |
| `price_min`, `price_max` | numeric ≥0 | VND. Applied to min variant price |
| `sort` | enum | `relevance` (default if `q`) / `newest` (default if no `q`) / `popular` / `price_asc` / `price_desc` |
| `page` | int ≥1, default 1 | |
| `limit` | int 1–60, default 24 | |

Different filter groups combine with **AND**. Within a filter group (e.g. `style[]`), values combine with **OR**.

### Pagination envelope (all list endpoints)

```json
{
  "items": [ ... ],
  "pagination": {
    "page": 1,
    "limit": 24,
    "total": 156,
    "total_pages": 7,
    "has_more": true
  }
}
```

### Error envelope (reused from `pkg/httpx`)

```json
{ "error": { "code": "PRODUCT_NOT_FOUND", "message": "...", "details": { } } }
```

### Standardized error codes (Sprint 1 additions)

| Code | HTTP | When |
|---|---|---|
| `NO_BRAND_OWNED` | 403 | User has role=brand but no brand record |
| `BRAND_SUSPENDED` | 403 | Brand status is `suspended` |
| `PRODUCT_NOT_FOUND` | 404 | Product missing or not owned by caller's brand |
| `VARIANT_NOT_FOUND` | 404 | |
| `IMAGE_NOT_FOUND` | 404 | |
| `ADDRESS_NOT_FOUND` | 404 | |
| `SLUG_TAKEN` | 409 | Brand or product slug conflict |
| `VARIANT_CONFLICT` | 409 | Duplicate (product_id, size, color) |
| `INVALID_MIME` | 400 | Unsupported image type |
| `FILE_TOO_LARGE` | 413 | >5 MB |
| `TOO_MANY_FILES` | 400 | >10 files per request |
| `STORAGE_ERROR` | 502 | Storage backend failure |

### Notable endpoint behaviors

- **`POST /brand/me/products`**: required body fields are `name` and `category_id`. `slug` auto-generated from name if omitted (slugify → check unique within brand → append `-2`, `-3`, … on collision). Status defaults to `draft`. Variants and images are created via separate endpoints.
- **`PATCH /brand/me/products/:id` with `status` change**: transitioning `draft → active` requires `≥1 variant` AND `≥1 image`. Reject 409 with `PRODUCT_NOT_PUBLISHABLE` otherwise.
- **`GET /products/:id` / `:slug`**: fires-and-forget `UPDATE products SET view_count = view_count + 1` in a goroutine. Response not blocked.
- **`POST /brand/me/products/:id/images`**: multipart, field name `files[]`. Multiple files in one request. Auto-assign incremental `sort_order`. First image of a product becomes `is_primary=true`.
- **`DELETE /brand/me/products/:id/images/:image_id`**: removes DB row and storage object. If the deleted image was primary, promotes the next image (lowest `sort_order`) to primary.
- **`POST /brand/me/products/:id/variants`**: validates uniqueness `(size, color)` within product. Returns 409 `VARIANT_CONFLICT` on duplicate.
- **`PATCH /brand/me/addresses/:id` with `is_primary=true`**: in a transaction, unsets `is_primary` on all sibling addresses then sets it on the target row.

---

## 5. Search & Filter Implementation

### Indexed search via `pg_trgm`

- `products.search_text` is denormalized + unaccented + lowercased.
- Maintained by `update_product_search_text` trigger on insert/update of `name`, `description`, or `brand_id`.
- When `brands.name` changes, `resync_brand_products_search` trigger touches `products.updated_at` on each child product, which re-fires the search_text trigger.
- GIN index `gin_trgm_ops` on `search_text`, filtered `WHERE deleted_at IS NULL AND status = 'active'`.

### Search query pattern

```sql
SELECT ...
  FROM products p
  JOIN brands b ON b.id = p.brand_id AND b.status = 'active' AND b.deleted_at IS NULL
  JOIN LATERAL (
    SELECT MIN(price) AS min_price, MAX(price) AS max_price,
           bool_or(stock_qty > 0) AS in_stock
      FROM product_variants
     WHERE product_id = p.id AND deleted_at IS NULL AND is_active
  ) vp ON true
 WHERE p.deleted_at IS NULL
   AND p.status = 'active'
   AND ($q::text IS NULL OR p.search_text % unaccent(lower($q)))
   AND ($category_id::uuid IS NULL OR p.category_id = $category_id)
   AND ($brand_id::uuid IS NULL OR p.brand_id = $brand_id)
   AND ($price_min::numeric IS NULL OR vp.min_price >= $price_min)
   AND ($price_max::numeric IS NULL OR vp.min_price <= $price_max)
   AND ($style_ids::uuid[] IS NULL OR EXISTS (
         SELECT 1 FROM product_style_tags
          WHERE product_id = p.id AND style_tag_id = ANY($style_ids)))
   AND ($sizes::text[] IS NULL OR EXISTS (
         SELECT 1 FROM product_variants pv
          WHERE pv.product_id = p.id AND pv.size = ANY($sizes)
            AND pv.is_active AND pv.stock_qty > 0))
   AND ($colors::text[] IS NULL OR EXISTS (
         SELECT 1 FROM product_variants pv
          WHERE pv.product_id = p.id AND pv.color = ANY($colors)
            AND pv.is_active AND pv.stock_qty > 0))
 ORDER BY <sort expression>, p.created_at DESC
 LIMIT $limit OFFSET $offset;
```

The `%` operator is the pg_trgm similarity operator (true when similarity ≥ `pg_trgm.similarity_threshold`, default 0.3).

### Sort expressions

| `sort` | ORDER BY clause |
|---|---|
| `relevance` (default when `q` present) | `similarity(p.search_text, unaccent(lower($q))) DESC, p.sold_count DESC` |
| `newest` (default when no `q`) | `p.created_at DESC` |
| `popular` | `p.sold_count DESC, p.view_count DESC` |
| `price_asc` | `vp.min_price ASC NULLS LAST` |
| `price_desc` | `vp.min_price DESC NULLS LAST` |

All sorts append `p.created_at DESC` as final tiebreaker.

### Total count

Each list endpoint runs two queries: SELECT (with LIMIT/OFFSET) + COUNT(*) with the same WHERE clause. Acceptable for Sprint 1 catalog sizes (≤10k products). Optimization (window function `COUNT(*) OVER()` or cursor pagination) deferred.

### Typo suggestion (UC10 alt 2b)

When `q` is non-empty and the main query returns zero items, fire a second query:

```sql
SELECT name, similarity(unaccent(lower(name)), unaccent(lower($q))) AS sim
  FROM products
 WHERE status = 'active' AND deleted_at IS NULL
 ORDER BY sim DESC LIMIT 3;
```

Response: `{ "items": [], "suggestions": ["..."], "pagination": {...} }`.

### Performance ceiling

This design handles up to ~50k active products comfortably with pg_trgm on a single Postgres instance. Beyond that, migrate to Postgres FTS or an external search engine — out of scope.

---

## 6. Image Upload & Storage

### Storage interface (`internal/shared/storage`)

```go
type Object struct {
    Key         string  // "products/{product_id}/{uuid}.jpg"
    ContentType string  // "image/jpeg" | "image/png" | "image/webp"
    Size        int64
}

type Storage interface {
    Put(ctx context.Context, obj Object, r io.Reader) (url string, err error)
    Delete(ctx context.Context, key string) error
    URL(key string) string
}
```

### Implementations

- **`LocalStorage`** (dev): writes to `${LocalDir}/${key}`, serves via `r.Static("/uploads", ...)` in `cmd/api/main.go`. Selected by `STORAGE_DRIVER=local`.
- **`GCSStorage`** (prod, stub for Sprint 1): file exists with interface-conforming methods returning `ErrNotImplemented`. Selected by `STORAGE_DRIVER=gcs`. Fully wired in a later sprint when prod deploy approaches.

Factory in `internal/shared/storage/factory.go` chooses based on `config.StorageConfig.Driver`.

### Config additions (`internal/config/config.go`)

```go
type StorageConfig struct {
    Driver         string        // "local" | "gcs"
    LocalDir       string        // ./uploads
    BaseURL        string        // http://localhost:8080/uploads
    GCSBucket      string
    GCSCredentials string
    MaxFileSize    int64         // 5 * 1024 * 1024
    AllowedMIMEs   []string      // ["image/jpeg", "image/png", "image/webp"]
}
```

Env vars: `STORAGE_DRIVER`, `STORAGE_LOCAL_DIR`, `STORAGE_BASE_URL`, `STORAGE_MAX_FILE_SIZE`, `STORAGE_ALLOWED_MIMES` (comma-separated). All have sensible defaults for local dev.

### Upload flow

```
Handler (POST /brand/me/products/:id/images):
  1. Verify Content-Type starts with "multipart/form-data".
  2. Parse multipart form; collect files[] (max 10, else 400 TOO_MANY_FILES).
  3. Per-file checks (cheap): size ≤ MaxFileSize (else 413 FILE_TOO_LARGE).
  4. Delegate to service.

Service (product.UploadImages):
  1. Verify product exists and product.brand_id = ctx.BrandID (else 404 PRODUCT_NOT_FOUND).
  2. For each file:
     a. Read first 512 bytes, http.DetectContentType to verify true MIME.
        Reject if not in AllowedMIMEs (400 INVALID_MIME).
     b. Build storage key: "products/{product_id}/{uuid}.{ext}".
     c. storage.Put(ctx, obj, file) → returns url.
     d. INSERT INTO product_images (id, product_id, url, storage_key,
        sort_order, is_primary).
        - sort_order = (SELECT COALESCE(MAX(sort_order), -1) + 1
                          FROM product_images WHERE product_id = $1).
        - is_primary = true if no existing images for this product.
  3. If any DB insert fails after a successful Put, call storage.Delete for cleanup.
  4. Return list of created image rows.
```

### Image delete flow

```
1. Verify image_id → product_id → brand_id chain (else 404).
2. Capture image.storage_key before delete.
3. Transaction:
   a. DELETE FROM product_images WHERE id = ?.
   b. If was_primary: UPDATE product_images
        SET is_primary = true
        WHERE product_id = ? AND id IN (
          SELECT id FROM product_images
           WHERE product_id = ? ORDER BY sort_order LIMIT 1
        ).
4. After commit: storage.Delete(ctx, storage_key). Best-effort; log if fails.
```

### Validation rules

| Rule | Value | Rationale |
|---|---|---|
| Allowed MIMEs | `image/jpeg`, `image/png`, `image/webp` | Excludes SVG (XSS risk), HEIC (no server-side decode) |
| Max file size | 5 MB | Matches auth module avatar limit (UC07) |
| Max files / request | 10 | Memory protection; brands batch-upload via multiple requests |
| Filename | Server-generated UUID | Prevents path traversal & collision |
| MIME sniff | First 512 bytes via `http.DetectContentType` | Client-supplied Content-Type is untrusted |

### Cleanup considerations

- **Product soft-delete**: images files are *not* removed (product may be restored). Files become temporarily orphan.
- **Product hard-delete**: `ON DELETE CASCADE` removes `product_images` rows but storage files are orphaned. Background cleanup job is out of scope; storage cost is negligible at Sprint 1 scale.
- **Static `/uploads` route**: enabled only when `STORAGE_DRIVER=local`. Since filenames are server-generated UUIDs and the route is `r.Static` (not `r.StaticFS` with user input), path traversal is not possible.

---

## 7. Authorization & Validation

### Middleware chain summary

| Route group | Chain (left to right) |
|---|---|
| `GET /products*`, `GET /brands*`, `GET /categories`, `GET /style-tags` | `OptionalAuth + RateLimit` (already applied at `/api/v1`) |
| `/brand/me/*` | `OptionalAuth + RateLimit + RequireAuth + RequireRole(domain.RoleBrand) + BrandContext` |

`RequireAuth`, `RequireRole`, and `OptionalAuth` already exist in `internal/auth/middleware`. Reuse.

### `BrandContext` middleware (new — `internal/brand/middleware`)

```go
func BrandContext(brandRepo brand.Repo) gin.HandlerFunc {
    return func(c *gin.Context) {
        userID := authmw.UserID(c)  // set by RequireAuth
        b, err := brandRepo.FindByOwnerUserID(c.Request.Context(), userID)
        switch {
        case errors.Is(err, brand.ErrNotFound):
            httpx.Error(c, 403, "NO_BRAND_OWNED", "User does not own a brand")
            return
        case err != nil:
            httpx.ErrorFromApp(c, err)
            return
        case b.Status == domain.BrandStatusSuspended:
            httpx.Error(c, 403, "BRAND_SUSPENDED", "Brand is suspended")
            return
        }
        c.Set("brand_id", b.ID)
        c.Set("brand", b)
        c.Next()
    }
}
```

Handlers retrieve via `brandID := c.MustGet("brand_id").(uuid.UUID)`.

### IDOR prevention

Every brand-scoped write enforces `WHERE brand_id = $brand_id` in the same statement that targets the resource — never as a separate "check then update" sequence (race condition).

```sql
-- Product update pattern:
UPDATE products SET ...
 WHERE id = $product_id
   AND brand_id = $brand_id
   AND deleted_at IS NULL
RETURNING ...;
-- If RowsAffected = 0 → 404 PRODUCT_NOT_FOUND
```

For nested resources (variants, images), join through products:

```sql
DELETE FROM product_variants
 WHERE id = $variant_id
   AND product_id IN (
     SELECT id FROM products
      WHERE brand_id = $brand_id AND deleted_at IS NULL
   )
RETURNING id;
```

A repo helper `assertProductOwnedByBrand(ctx, productID, brandID) error` covers multi-step flows.

### Input validation

DTO bindings use Gin's `binding:"..."` plus custom validators registered via `internal/shared/validator`. Selected examples:

```go
type CreateProductRequest struct {
    Name        string   `json:"name"          binding:"required,min=2,max=200"`
    Slug        string   `json:"slug"          binding:"omitempty,slug,max=200"`
    Description string   `json:"description"   binding:"omitempty,max=5000"`
    CategoryID  string   `json:"category_id"   binding:"required,uuid"`
    StyleTagIDs []string `json:"style_tag_ids" binding:"omitempty,max=10,dive,uuid"`
}

type CreateVariantRequest struct {
    SKU      string  `json:"sku"       binding:"required,min=1,max=64"`
    Size     string  `json:"size"      binding:"required,max=20"`
    Color    string  `json:"color"     binding:"required,max=50"`
    ColorHex string  `json:"color_hex" binding:"omitempty,hexcolor"`
    Price    float64 `json:"price"     binding:"required,gt=0"`
    StockQty int     `json:"stock_qty" binding:"min=0"`
}

type ListProductsQuery struct {
    Q        string   `form:"q"        binding:"omitempty,max=100"`
    Category string   `form:"category" binding:"omitempty,slug"`
    Brand    string   `form:"brand"    binding:"omitempty,slug"`
    Style    []string `form:"style"    binding:"omitempty,max=10,dive,slug"`
    Size     []string `form:"size"     binding:"omitempty,max=10,dive,max=20"`
    Color    []string `form:"color"    binding:"omitempty,max=10,dive,max=50"`
    PriceMin *float64 `form:"price_min" binding:"omitempty,gte=0"`
    PriceMax *float64 `form:"price_max" binding:"omitempty,gtefield=PriceMin"`
    Sort     string   `form:"sort"     binding:"omitempty,oneof=relevance newest popular price_asc price_desc"`
    Page     int      `form:"page,default=1"   binding:"min=1"`
    Limit    int      `form:"limit,default=24" binding:"min=1,max=60"`
}
```

New custom validator: `slug` (regex `^[a-z0-9](-?[a-z0-9])*$`, max 200 chars).

### Rate limiting

The existing `/api/v1` group applies `RateLimit(rdb, cfg.Limit.RateLimitPerMin)` (default 100 req/min/user). No extra tightening for Sprint 1. If image upload becomes a hotspot, add a per-route 20 req/min limit.

---

## 8. Testing Strategy

The auth module shipped without tests. Sprint 1 establishes the testing pattern that subsequent sprints follow.

### Three-tier structure

- **Unit** — pure logic with mocked repos. Targets services and shared helpers (slugify, search query builder).
- **Integration** — real Postgres via the `make up` container, transaction-rollback isolation per test. Targets repos and the trigger.
- **E2E** — one or two happy-path scenarios through the real HTTP server with a seeded test database.

### Tooling

- `testing` stdlib + `testify/require` for assertions.
- `pgxpool` against `wearwhere_test` database (created via Makefile target on first run).
- Hand-written mocks for repo interfaces — no `gomock` or `mockery` in Sprint 1.
- `httptest.NewRecorder` + Gin engine for handler tests.
- Test fixtures in `internal/testfixtures/`: `SeedBrand`, `SeedProduct`, `SeedCategory`, `SeedUser` helpers that insert minimal rows and return entities.

### Test database setup

`docker-compose.yml` postgres service already runs. Add a Makefile target:

```makefile
TEST_DB_URL = postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable

test-db-up:
	docker compose exec -T postgres psql -U wearwhere \
	  -c "CREATE DATABASE wearwhere_test;" || true
	migrate -path db/migrations -database "$(TEST_DB_URL)" up

test-integration: test-db-up
	TEST_DATABASE_URL="$(TEST_DB_URL)" go test ./... -tags=integration -v

test-unit:
	go test ./... -v -race
```

Integration tests are gated by `//go:build integration` so `go test ./...` without tag stays fast (unit only).

### Transaction-rollback isolation

Each integration test opens a `pgx.Tx`, passes it (via a `DBTX` interface that pool and tx both satisfy) into the repo under test, and rolls back in `t.Cleanup`. No data pollution between tests; no global truncate needed.

```go
func TestProductRepo_Create(t *testing.T) {
    tx := beginTx(t, pool)
    defer tx.Rollback(context.Background())

    repo := product.NewPG(tx)
    brand := testfixtures.SeedBrand(t, tx)
    p, err := repo.Create(ctx, brand.ID, "Áo thun trắng", ...)

    require.NoError(t, err)
    require.NotEmpty(t, p.Slug)
}
```

### Coverage priorities (not driven by % targets)

| Area | Why mandatory |
|---|---|
| Slug auto-generation + collision retry | Easy-to-miss off-by-one; depends on DB state |
| Search query composition with mixed filter combos (q + category + style + price + size) | Most complex single query in the system |
| IDOR: brand A cannot mutate brand B's products / variants / images | Core security boundary |
| `BrandContext` middleware: no-brand / suspended / OK paths | Determines 403 vs 200 |
| Variant uniqueness `(product_id, size, color)` constraint vs service error | DB raises, service must translate to `VARIANT_CONFLICT` |
| Image upload flow: MIME sniff, size limit, sort_order increment, primary promotion on delete | Many failure modes in multipart |
| `search_text` trigger: updates on brand name change | Trigger logic that no test elsewhere will catch |
| Pagination edges: page=0 (rejected), limit > max, page beyond total | Query-builder bugs cluster here |

### Test layout

```
internal/
├── brand/
│   ├── repo/brand_pg_test.go               // integration
│   ├── repo/address_pg_test.go             // integration
│   ├── service/brand_service_test.go       // unit
│   ├── middleware/brand_context_test.go    // unit + httptest
│   └── handler/brand_handler_test.go       // httptest
├── product/
│   ├── repo/product_pg_test.go             // integration (search query focus)
│   ├── repo/variant_pg_test.go             // integration
│   ├── repo/image_pg_test.go               // integration
│   ├── service/product_service_test.go     // unit
│   ├── service/catalog_service_test.go     // unit
│   └── handler/catalog_handler_test.go     // httptest
├── shared/
│   └── storage/local_test.go               // unit, uses t.TempDir
└── testfixtures/                           // helpers
```

### One end-to-end scenario

`cmd/api/main_test.go` runs against a fresh test DB:

```
1. Seed: user (role=brand), brand, category, style_tag.
2. POST /auth/login → get access token.
3. POST /brand/me/products            (status=draft initially)
4. POST /brand/me/products/:id/variants × 2
5. POST /brand/me/products/:id/images (multipart, 1 file)
6. PATCH /brand/me/products/:id  {"status": "active"}
7. GET /products?q=<query>            (unauthenticated)
     → expect product appears in items
8. GET /brands/{brand_slug}/products/{product_slug}
     → expect detail with both variants and 1 image
```

Runs in ~3–5 seconds. Catches wiring issues between modules.

### Out of scope for Sprint 1 testing

- Load / benchmark tests — run ad-hoc with k6 or vegeta later
- Fuzz testing on search query
- Mutation testing
- CI workflow setup (GitHub Actions) — separate cross-sprint task

---

## 9. Implementation order (preview for plan)

This is a sketch; the formal plan comes in the next document.

1. Migrations (000006–000016) + `make migrate-up` runs cleanly
2. `internal/shared/storage` (interface + Local + GCS stub + factory + config)
3. `internal/brand` (domain → repo → service → middleware → handler → mount)
4. `internal/product` brand-side write: domain → repo → service → handler
5. `internal/product` customer-side read + search → catalog service + handler
6. `internal/testfixtures` helpers
7. Test passes (unit + integration + E2E)
8. Manual smoke: end-to-end via curl, including image upload
