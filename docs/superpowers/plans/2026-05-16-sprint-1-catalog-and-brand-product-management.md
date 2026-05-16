# Sprint 1 — Catalog & Brand Product Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement Sprint 1 of the WearWhere shopping system: brand product management (UC42-44, UC51) and the public catalog (UC10-13). Customer-side cart/orders deferred to later sprints.

**Architecture:** Two new domain modules (`internal/brand`, `internal/product`) plus a new `internal/shared/storage` package. Postgres + Redis already provisioned by auth module. Search uses `pg_trgm` + `unaccent` against a denormalized `search_text` column maintained by trigger. Image upload via `Storage` interface (Local FS for dev, GCS stub for prod).

**Tech Stack:** Go 1.x, Gin, pgx/v5, go-playground/validator, golang-migrate, Postgres 16, Redis 7.

**Spec reference:** [docs/superpowers/specs/2026-05-16-catalog-and-brand-product-management-design.md](../specs/2026-05-16-catalog-and-brand-product-management-design.md)

---

## File Structure

### Created

```
db/migrations/
  000006_create_extensions_search.{up,down}.sql
  000007_create_brands.{up,down}.sql
  000008_create_categories.{up,down}.sql
  000009_create_style_tags.{up,down}.sql
  000010_create_brand_addresses.{up,down}.sql
  000011_create_products.{up,down}.sql
  000012_create_product_images.{up,down}.sql
  000013_create_product_variants.{up,down}.sql
  000014_create_product_style_tags.{up,down}.sql
  000015_seed_taxonomy.{up,down}.sql
  000016_seed_dev_brands.{up,down}.sql

internal/shared/storage/
  storage.go          — Storage interface, Object struct, ErrNotImplemented
  local.go            — LocalStorage impl
  gcs.go              — GCSStorage stub
  factory.go          — New(cfg) → Storage
  local_test.go

internal/shared/slug/
  slug.go             — Slugify(s string) string, IsValid(s string) bool
  slug_test.go

internal/testfixtures/
  fixtures.go         — SeedUser, SeedBrand, SeedCategory, SeedStyleTag, SeedProduct,
                        SeedVariant, SeedImage helpers + BeginTx

internal/brand/
  domain/brand.go     — Brand, BrandAddress, status enums
  domain/errors.go    — AppErrors: ErrBrandNotFound, ErrNoBrandOwned, etc.
  domain/dto.go       — UpdateBrandRequest, Create/UpdateAddressRequest, responses
  repo/repo.go        — BrandRepo, AddressRepo interfaces + ErrNotFound
  repo/brand_pg.go
  repo/address_pg.go
  repo/brand_pg_test.go      (integration)
  repo/address_pg_test.go    (integration)
  service/brand_service.go
  service/brand_service_test.go (unit)
  middleware/brand_context.go
  middleware/brand_context_test.go
  handler/brand_handler.go
  handler/address_handler.go
  handler/brand_handler_test.go
  handler/routes.go

internal/product/
  domain/product.go   — Product, Variant, Image, Category, StyleTag, enums
  domain/errors.go
  domain/dto.go
  repo/repo.go        — ProductRepo, VariantRepo, ImageRepo, CategoryRepo, StyleTagRepo + ErrNotFound
  repo/product_pg.go        (brand-side writes)
  repo/variant_pg.go
  repo/image_pg.go
  repo/category_pg.go
  repo/style_tag_pg.go
  repo/catalog_query.go     (customer-side list with search/filter/sort)
  repo/product_pg_test.go   (integration)
  repo/variant_pg_test.go   (integration)
  repo/catalog_query_test.go (integration, the most important test file)
  service/product_service.go     (brand-side write)
  service/catalog_service.go     (public read)
  service/product_service_test.go (unit)
  service/catalog_service_test.go (unit)
  handler/brand_product_handler.go
  handler/catalog_handler.go
  handler/brand_product_handler_test.go
  handler/catalog_handler_test.go
  handler/routes.go

cmd/api/main_test.go     — single E2E happy path
```

### Modified

```
internal/config/config.go            — add StorageConfig
internal/shared/validator/validator.go — register `slug` validator
cmd/api/main.go                      — wire new repos/services/handlers + mount routes
Makefile                             — add test-db-up, test-unit, test-integration targets
.env.example                         — add STORAGE_* vars (create if missing)
docker-compose.yml                   — no change; reuse postgres container
```

---

## Conventions used in this plan

- **Repo pattern**: each repo struct holds `db DBTX` where `DBTX` is an interface satisfied by both `*pgxpool.Pool` and `pgx.Tx`. This lets integration tests pass a tx and roll back.
- **Error pattern**: domain-level errors as `*httpx.AppError` variables (matches `internal/auth/domain/errors.go`). Repo-level "not found" as `ErrNotFound` in the repo package; service translates to domain AppError.
- **Commit cadence**: one commit per task. Commit message format `feat(<module>): <summary>` or `chore(db): <summary>` or `test(<module>): <summary>`.
- **Build tags**: integration tests use `//go:build integration` so `go test ./...` runs unit-only by default.
- **Go imports**: group standard → third-party → project, separated by blank line (matches existing files).

---

## Phase A — Foundation (Tasks 1-7)

### Task 1: Migration 000006 — extensions + 000007 — brands

**Files:**
- Create: `db/migrations/000006_create_extensions_search.up.sql`
- Create: `db/migrations/000006_create_extensions_search.down.sql`
- Create: `db/migrations/000007_create_brands.up.sql`
- Create: `db/migrations/000007_create_brands.down.sql`

- [ ] **Step 1: Write 000006 up**

`db/migrations/000006_create_extensions_search.up.sql`:
```sql
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS unaccent;
```

- [ ] **Step 2: Write 000006 down**

`db/migrations/000006_create_extensions_search.down.sql`:
```sql
DROP EXTENSION IF EXISTS unaccent;
DROP EXTENSION IF EXISTS pg_trgm;
```

- [ ] **Step 3: Write 000007 up**

`db/migrations/000007_create_brands.up.sql`:
```sql
CREATE TYPE brand_status AS ENUM ('pending', 'active', 'suspended');

CREATE TABLE brands (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    slug            CITEXT NOT NULL UNIQUE,
    name            VARCHAR(120) NOT NULL,
    owner_user_id   UUID NOT NULL REFERENCES users(id),
    story           TEXT,
    logo_url        TEXT,
    banner_url      TEXT,
    website_url     TEXT,
    status          brand_status NOT NULL DEFAULT 'active',
    verified_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_brands_owner_unique
    ON brands (owner_user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_brands_status
    ON brands (status, deleted_at);
CREATE INDEX idx_brands_name_trgm
    ON brands USING GIN (name gin_trgm_ops);
```

- [ ] **Step 4: Write 000007 down**

`db/migrations/000007_create_brands.down.sql`:
```sql
DROP TABLE IF EXISTS brands;
DROP TYPE IF EXISTS brand_status;
```

- [ ] **Step 5: Run migrations**

Run: `make migrate-up`
Expected: applies 000006 and 000007 cleanly.

Verify with `psql` or any client:
```sql
SELECT extname FROM pg_extension WHERE extname IN ('pg_trgm','unaccent');
-- expects 2 rows
\d brands
-- expects table and indexes
```

- [ ] **Step 6: Run down then up to verify reversibility**

Run:
```bash
make migrate-down  # 000007 down
make migrate-down  # 000006 down
make migrate-up    # both up again
```
Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add db/migrations/000006_create_extensions_search.up.sql \
        db/migrations/000006_create_extensions_search.down.sql \
        db/migrations/000007_create_brands.up.sql \
        db/migrations/000007_create_brands.down.sql
git commit -m "chore(db): add pg_trgm/unaccent extensions and brands table"
```

---

### Task 2: Migrations 000008 categories, 000009 style_tags, 000010 brand_addresses

**Files:**
- Create: `db/migrations/000008_create_categories.{up,down}.sql`
- Create: `db/migrations/000009_create_style_tags.{up,down}.sql`
- Create: `db/migrations/000010_create_brand_addresses.{up,down}.sql`

- [ ] **Step 1: Write 000008 up**

```sql
CREATE TABLE categories (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    slug            CITEXT NOT NULL UNIQUE,
    name            VARCHAR(80) NOT NULL,
    display_order   INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_categories_display ON categories (display_order, name);
```

- [ ] **Step 2: Write 000008 down**

```sql
DROP TABLE IF EXISTS categories;
```

- [ ] **Step 3: Write 000009 up**

```sql
CREATE TABLE style_tags (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    slug        CITEXT NOT NULL UNIQUE,
    name        VARCHAR(80) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

- [ ] **Step 4: Write 000009 down**

```sql
DROP TABLE IF EXISTS style_tags;
```

- [ ] **Step 5: Write 000010 up**

```sql
CREATE TABLE brand_addresses (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    brand_id        UUID NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    label           VARCHAR(80) NOT NULL,
    address_line    VARCHAR(255) NOT NULL,
    ward            VARCHAR(80) NOT NULL,
    district        VARCHAR(80) NOT NULL,
    city            VARCHAR(80) NOT NULL,
    country         CHAR(2) NOT NULL DEFAULT 'VN',
    postal_code     VARCHAR(20),
    phone           VARCHAR(20),
    latitude        NUMERIC(10,7),
    longitude       NUMERIC(10,7),
    is_primary      BOOLEAN NOT NULL DEFAULT FALSE,
    is_public       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_brand_addr_primary_unique
    ON brand_addresses (brand_id) WHERE is_primary AND deleted_at IS NULL;
CREATE INDEX idx_brand_addr_brand_public
    ON brand_addresses (brand_id, is_public, deleted_at);
CREATE INDEX idx_brand_addr_geo
    ON brand_addresses (latitude, longitude)
    WHERE latitude IS NOT NULL AND longitude IS NOT NULL;
```

- [ ] **Step 6: Write 000010 down**

```sql
DROP TABLE IF EXISTS brand_addresses;
```

- [ ] **Step 7: Apply and verify**

```bash
make migrate-up
```

Verify in psql:
```sql
\d categories
\d style_tags
\d brand_addresses
```

- [ ] **Step 8: Commit**

```bash
git add db/migrations/000008_*.sql db/migrations/000009_*.sql db/migrations/000010_*.sql
git commit -m "chore(db): add categories, style_tags, brand_addresses tables"
```

---

### Task 3: Migration 000011 — products + search trigger

**Files:**
- Create: `db/migrations/000011_create_products.{up,down}.sql`

- [ ] **Step 1: Write 000011 up**

```sql
CREATE TYPE product_status AS ENUM ('draft', 'active', 'archived');

CREATE TABLE products (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    brand_id        UUID NOT NULL REFERENCES brands(id),
    category_id     UUID NOT NULL REFERENCES categories(id),
    slug            CITEXT NOT NULL,
    name            VARCHAR(200) NOT NULL,
    description     TEXT,
    status          product_status NOT NULL DEFAULT 'draft',
    currency        CHAR(3) NOT NULL DEFAULT 'VND',
    search_text     TEXT,
    sold_count      INT NOT NULL DEFAULT 0,
    view_count      INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_products_brand_slug
    ON products (brand_id, slug) WHERE deleted_at IS NULL;
CREATE INDEX idx_products_brand_status
    ON products (brand_id, status, deleted_at);
CREATE INDEX idx_products_category_status
    ON products (category_id, status, deleted_at);
CREATE INDEX idx_products_status_created
    ON products (status, deleted_at, created_at DESC);
CREATE INDEX idx_products_popular
    ON products (sold_count DESC, view_count DESC)
    WHERE deleted_at IS NULL AND status = 'active';
CREATE INDEX idx_products_search_trgm
    ON products USING GIN (search_text gin_trgm_ops)
    WHERE deleted_at IS NULL AND status = 'active';

-- Trigger: keep search_text in sync with name/description/brand.name
CREATE OR REPLACE FUNCTION update_product_search_text()
RETURNS TRIGGER AS $$
BEGIN
    NEW.search_text := unaccent(lower(
        coalesce(NEW.name, '') || ' ' ||
        coalesce(NEW.description, '') || ' ' ||
        coalesce((SELECT name FROM brands WHERE id = NEW.brand_id), '')
    ));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_product_search_text
    BEFORE INSERT OR UPDATE OF name, description, brand_id ON products
    FOR EACH ROW EXECUTE FUNCTION update_product_search_text();

-- Trigger: when brand.name changes, touch products to retrigger search_text update
CREATE OR REPLACE FUNCTION resync_brand_products_search()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE products
       SET updated_at = NOW()
     WHERE brand_id = NEW.id AND deleted_at IS NULL;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_brand_name_resync
    AFTER UPDATE OF name ON brands
    FOR EACH ROW
    WHEN (OLD.name IS DISTINCT FROM NEW.name)
    EXECUTE FUNCTION resync_brand_products_search();

-- Need an additional trigger on products to recompute search_text when
-- only updated_at changes (the resync trigger above touches updated_at,
-- but the BEFORE UPDATE OF name,description,brand_id trigger does not fire
-- if those columns aren't in the SET clause). Add a separate trigger that
-- fires on any UPDATE to be safe — it short-circuits if values haven't changed.
CREATE OR REPLACE FUNCTION force_recompute_product_search_text()
RETURNS TRIGGER AS $$
BEGIN
    NEW.search_text := unaccent(lower(
        coalesce(NEW.name, '') || ' ' ||
        coalesce(NEW.description, '') || ' ' ||
        coalesce((SELECT name FROM brands WHERE id = NEW.brand_id), '')
    ));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- This fires only when the resync trigger updated updated_at without
-- touching name/description/brand_id. We can't easily distinguish, so we
-- just always recompute on UPDATE; cheap because the brand SELECT is indexed.
DROP TRIGGER trg_product_search_text ON products;
CREATE TRIGGER trg_product_search_text
    BEFORE INSERT OR UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION force_recompute_product_search_text();
```

- [ ] **Step 2: Write 000011 down**

```sql
DROP TRIGGER IF EXISTS trg_brand_name_resync ON brands;
DROP FUNCTION IF EXISTS resync_brand_products_search();
DROP TRIGGER IF EXISTS trg_product_search_text ON products;
DROP FUNCTION IF EXISTS force_recompute_product_search_text();
DROP FUNCTION IF EXISTS update_product_search_text();
DROP TABLE IF EXISTS products;
DROP TYPE IF EXISTS product_status;
```

- [ ] **Step 3: Apply and verify trigger**

```bash
make migrate-up
```

Manual smoke (psql):
```sql
INSERT INTO categories (slug, name) VALUES ('test-cat', 'Test');
INSERT INTO users (id, name, role) VALUES (uuid_generate_v4(), 'Test Owner', 'brand')
  RETURNING id \gset
INSERT INTO brands (slug, name, owner_user_id) VALUES ('test-brand', 'Test Brand', :'id')
  RETURNING id \gset
INSERT INTO products (brand_id, category_id, slug, name, description)
  VALUES (:'id'::uuid, (SELECT id FROM categories WHERE slug='test-cat'),
          'p1', 'Áo Thun Trắng', 'Cotton mềm mại')
  RETURNING search_text;
-- expect search_text contains 'ao thun trang cotton mem mai test brand' (unaccented)

UPDATE brands SET name = 'Brand New Name' WHERE slug = 'test-brand';
SELECT search_text FROM products WHERE slug = 'p1';
-- expect search_text now contains 'brand new name' instead of 'test brand'

-- cleanup
DELETE FROM products WHERE slug='p1';
DELETE FROM brands WHERE slug='test-brand';
DELETE FROM users WHERE name='Test Owner';
DELETE FROM categories WHERE slug='test-cat';
```

- [ ] **Step 4: Commit**

```bash
git add db/migrations/000011_*.sql
git commit -m "chore(db): add products table with search_text trigger"
```

---

### Task 4: Migrations 000012 product_images, 000013 product_variants, 000014 product_style_tags

**Files:**
- Create: `db/migrations/000012_create_product_images.{up,down}.sql`
- Create: `db/migrations/000013_create_product_variants.{up,down}.sql`
- Create: `db/migrations/000014_create_product_style_tags.{up,down}.sql`

- [ ] **Step 1: Write 000012 up (images BEFORE variants)**

```sql
CREATE TABLE product_images (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    product_id      UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    url             TEXT NOT NULL,
    storage_key     TEXT NOT NULL,
    alt_text        VARCHAR(200),
    sort_order      INT NOT NULL DEFAULT 0,
    is_primary      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_product_images_product_order
    ON product_images (product_id, sort_order);
CREATE UNIQUE INDEX idx_product_images_primary
    ON product_images (product_id) WHERE is_primary;
```

- [ ] **Step 2: Write 000012 down**

```sql
DROP TABLE IF EXISTS product_images;
```

- [ ] **Step 3: Write 000013 up**

```sql
CREATE TABLE product_variants (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    product_id      UUID NOT NULL REFERENCES products(id),
    sku             VARCHAR(64) NOT NULL,
    size            VARCHAR(20) NOT NULL,
    color           VARCHAR(50) NOT NULL,
    color_hex       CHAR(7),
    price           NUMERIC(12,2) NOT NULL CHECK (price > 0),
    stock_qty       INT NOT NULL DEFAULT 0 CHECK (stock_qty >= 0),
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    image_id        UUID REFERENCES product_images(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_product_variants_size_color
    ON product_variants (product_id, size, color) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_product_variants_sku
    ON product_variants (product_id, sku) WHERE deleted_at IS NULL;
CREATE INDEX idx_product_variants_active
    ON product_variants (product_id, is_active, deleted_at);
```

- [ ] **Step 4: Write 000013 down**

```sql
DROP TABLE IF EXISTS product_variants;
```

- [ ] **Step 5: Write 000014 up**

```sql
CREATE TABLE product_style_tags (
    product_id      UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    style_tag_id    UUID NOT NULL REFERENCES style_tags(id) ON DELETE CASCADE,
    PRIMARY KEY (product_id, style_tag_id)
);

CREATE INDEX idx_pst_tag_product ON product_style_tags (style_tag_id, product_id);
```

- [ ] **Step 6: Write 000014 down**

```sql
DROP TABLE IF EXISTS product_style_tags;
```

- [ ] **Step 7: Apply**

```bash
make migrate-up
```

Verify in psql:
```sql
\d product_images
\d product_variants
\d product_style_tags
```

- [ ] **Step 8: Commit**

```bash
git add db/migrations/000012_*.sql db/migrations/000013_*.sql db/migrations/000014_*.sql
git commit -m "chore(db): add product_images, product_variants, product_style_tags"
```

---

### Task 5: Seed migrations 000015 taxonomy + 000016 dev brands

**Files:**
- Create: `db/migrations/000015_seed_taxonomy.{up,down}.sql`
- Create: `db/migrations/000016_seed_dev_brands.{up,down}.sql`

- [ ] **Step 1: Write 000015 up — categories + style_tags**

```sql
INSERT INTO categories (slug, name, display_order) VALUES
    ('t-shirt',     'T-Shirt',     10),
    ('shirt',       'Shirt',       20),
    ('dress',       'Dress',       30),
    ('pants',       'Pants',       40),
    ('shorts',      'Shorts',      50),
    ('jacket',      'Jacket',      60),
    ('skirt',       'Skirt',       70),
    ('hoodie',      'Hoodie',      80),
    ('shoes',       'Shoes',       90),
    ('accessory',   'Accessory',  100)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO style_tags (slug, name) VALUES
    ('streetwear', 'Streetwear'),
    ('minimalist', 'Minimalist'),
    ('y2k',        'Y2K'),
    ('vintage',    'Vintage'),
    ('casual',     'Casual'),
    ('formal',     'Formal'),
    ('sporty',     'Sporty'),
    ('vietnamese', 'Vietnamese Heritage'),
    ('preppy',     'Preppy'),
    ('grunge',     'Grunge')
ON CONFLICT (slug) DO NOTHING;
```

- [ ] **Step 2: Write 000015 down**

```sql
DELETE FROM style_tags WHERE slug IN
    ('streetwear','minimalist','y2k','vintage','casual','formal',
     'sporty','vietnamese','preppy','grunge');
DELETE FROM categories WHERE slug IN
    ('t-shirt','shirt','dress','pants','shorts','jacket',
     'skirt','hoodie','shoes','accessory');
```

- [ ] **Step 3: Write 000016 up — dev brand owners + brands**

```sql
-- Idempotent: insert demo brand-owner users if missing.
-- Password hash below corresponds to "DevBrand@1234" using bcrypt cost=12.
-- Regenerate via the auth module's hash package if you want different creds.
INSERT INTO users (id, email, password_hash, role, status, name, email_verified_at)
VALUES
    ('11111111-1111-1111-1111-111111111111',
     'owner1@local.test',
     '$2a$12$KIXxPfnK7UvK7vTFvO5/lOQqB.6t9aS8L0iHnxOEKi4n3a6P3Hk9q',
     'brand', 'active', 'Local-X Owner', NOW()),
    ('22222222-2222-2222-2222-222222222222',
     'owner2@local.test',
     '$2a$12$KIXxPfnK7UvK7vTFvO5/lOQqB.6t9aS8L0iHnxOEKi4n3a6P3Hk9q',
     'brand', 'active', 'BadVibes Owner', NOW())
ON CONFLICT (id) DO NOTHING;

INSERT INTO brands (id, slug, name, owner_user_id, story, status)
VALUES
    ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
     'local-x', 'Local-X',
     '11111111-1111-1111-1111-111111111111',
     'Local-X kể câu chuyện streetwear Việt Nam đương đại.',
     'active'),
    ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
     'badvibes', 'BadVibes',
     '22222222-2222-2222-2222-222222222222',
     'Phong cách Y2K hoài niệm cho Gen Z Việt.',
     'active')
ON CONFLICT (id) DO NOTHING;

INSERT INTO brand_addresses (brand_id, label, address_line, ward, district, city, is_primary, is_public)
VALUES
    ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
     'Flagship Hà Nội', '12 Phố Huế', 'Ngô Thì Nhậm', 'Hai Bà Trưng', 'Hà Nội', TRUE, TRUE),
    ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
     'Showroom HCM', '45 Lý Tự Trọng', 'Bến Nghé', 'Quận 1', 'Hồ Chí Minh', FALSE, TRUE),
    ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
     'BadVibes HQ', '8 Nguyễn Huệ', 'Bến Nghé', 'Quận 1', 'Hồ Chí Minh', TRUE, TRUE)
ON CONFLICT DO NOTHING;
```

- [ ] **Step 4: Write 000016 down**

```sql
DELETE FROM brand_addresses
 WHERE brand_id IN ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
                    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb');
DELETE FROM brands
 WHERE id IN ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
              'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb');
DELETE FROM users
 WHERE id IN ('11111111-1111-1111-1111-111111111111',
              '22222222-2222-2222-2222-222222222222');
```

- [ ] **Step 5: Apply and verify**

```bash
make migrate-up
```

Verify in psql:
```sql
SELECT slug, name FROM categories ORDER BY display_order;
-- expect 10 rows
SELECT slug, name FROM style_tags ORDER BY name;
-- expect 10 rows
SELECT slug, name FROM brands;
-- expect 2 rows
SELECT brand_id, label, is_primary FROM brand_addresses;
-- expect 3 rows
```

- [ ] **Step 6: Verify full reverse**

```bash
# Walk all the way back, then forward, ensure clean
for i in 1 2 3 4 5 6 7 8 9 10 11; do make migrate-down; done
make migrate-up
```

Expected: zero errors. (Each `migrate-down` rolls back 1 step.)

- [ ] **Step 7: Commit**

```bash
git add db/migrations/000015_*.sql db/migrations/000016_*.sql
git commit -m "chore(db): seed taxonomy + dev brands"
```

---

### Task 6: Slug helper + `slug` custom validator

**Files:**
- Create: `internal/shared/slug/slug.go`
- Create: `internal/shared/slug/slug_test.go`
- Modify: `internal/shared/validator/validator.go`

- [ ] **Step 1: Write the failing tests**

`internal/shared/slug/slug_test.go`:
```go
package slug

import "testing"

func TestSlugify(t *testing.T) {
    cases := []struct {
        in, want string
    }{
        {"Áo Thun Trắng", "ao-thun-trang"},
        {"  Café   Sữa  ", "cafe-sua"},
        {"T-Shirt v2.0!", "t-shirt-v2-0"},
        {"---leading---trailing---", "leading-trailing"},
        {"", ""},
        {"!!!", ""},
        {"Số 1 Việt Nam", "so-1-viet-nam"},
        {"Multiple   spaces", "multiple-spaces"},
    }
    for _, c := range cases {
        if got := Slugify(c.in); got != c.want {
            t.Errorf("Slugify(%q) = %q, want %q", c.in, got, c.want)
        }
    }
}

func TestIsValid(t *testing.T) {
    valid := []string{"abc", "abc-def", "a", "a1-b2-c3", "t-shirt"}
    invalid := []string{
        "", "-abc", "abc-", "ab--cd", "Abc", "abc_def", "ab cd", "ab.cd",
    }
    for _, s := range valid {
        if !IsValid(s) {
            t.Errorf("IsValid(%q) = false, want true", s)
        }
    }
    for _, s := range invalid {
        if IsValid(s) {
            t.Errorf("IsValid(%q) = true, want false", s)
        }
    }
}
```

- [ ] **Step 2: Run tests — they should fail (package doesn't exist)**

Run: `go test ./internal/shared/slug/...`
Expected: build error "no Go files".

- [ ] **Step 3: Write the implementation**

`internal/shared/slug/slug.go`:
```go
// Package slug provides URL-safe slug generation and validation.
// Slugify removes diacritics, lowercases, and collapses runs of non-alphanumeric
// characters to a single hyphen. IsValid checks a string matches the slug grammar.
package slug

import (
    "regexp"
    "strings"
    "unicode"

    "golang.org/x/text/runes"
    "golang.org/x/text/transform"
    "golang.org/x/text/unicode/norm"
)

var (
    slugRe        = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
    nonAlphaNumRe = regexp.MustCompile(`[^a-z0-9]+`)
)

// Slugify converts s into a URL-safe slug:
//   - NFD-normalize, strip combining marks (Vietnamese tone removal)
//   - lowercase
//   - replace runs of non-alphanumeric with "-"
//   - trim leading/trailing "-"
func Slugify(s string) string {
    t := transform.Chain(
        norm.NFD,
        runes.Remove(runes.In(unicode.Mn)),
        norm.NFC,
    )
    out, _, err := transform.String(t, s)
    if err != nil {
        out = s
    }
    // Replace 'đ' / 'Đ' explicitly — they are not decomposable.
    out = strings.NewReplacer("đ", "d", "Đ", "D").Replace(out)
    out = strings.ToLower(out)
    out = nonAlphaNumRe.ReplaceAllString(out, "-")
    out = strings.Trim(out, "-")
    return out
}

// IsValid reports whether s is a syntactically valid slug.
// Empty strings are not valid.
func IsValid(s string) bool {
    return s != "" && slugRe.MatchString(s)
}
```

- [ ] **Step 4: Add `golang.org/x/text` dependency**

Run:
```bash
go get golang.org/x/text/unicode/norm
go get golang.org/x/text/runes
go get golang.org/x/text/transform
go mod tidy
```

- [ ] **Step 5: Run tests — they should pass**

Run: `go test ./internal/shared/slug/... -v`
Expected: all PASS.

- [ ] **Step 6: Register `slug` validator with Gin**

Modify `internal/shared/validator/validator.go`. Add to both `V` initialization AND `RegisterWithGin`:

Add the import:
```go
import (
    // ... existing imports
    "github.com/wearwhere/wearwhere_be/internal/shared/slug"
    // ...
)
```

In `V`'s initializer, add a line after the existing `RegisterValidation` calls:
```go
_ = v.RegisterValidation("slug", slugValidator)
```

In `RegisterWithGin`, inside the `if v, ok := ...` block, add:
```go
_ = v.RegisterValidation("slug", slugValidator)
```

At the bottom of the file, add the validator function:
```go
func slugValidator(fl validator.FieldLevel) bool {
    return slug.IsValid(fl.Field().String())
}
```

- [ ] **Step 7: Run all existing tests + new test together**

Run: `go test ./internal/shared/...`
Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/shared/slug/ internal/shared/validator/validator.go go.mod go.sum
git commit -m "feat(shared): add slugify helper and slug validator"
```

---

### Task 7: Storage interface + Local backend + GCS stub + config

**Files:**
- Create: `internal/shared/storage/storage.go`
- Create: `internal/shared/storage/local.go`
- Create: `internal/shared/storage/gcs.go`
- Create: `internal/shared/storage/factory.go`
- Create: `internal/shared/storage/local_test.go`
- Modify: `internal/config/config.go`

- [ ] **Step 1: Write the interface**

`internal/shared/storage/storage.go`:
```go
// Package storage provides a pluggable file storage abstraction.
// Local backs onto the host filesystem (dev). GCS backs onto Google Cloud
// Storage (prod, stubbed in Sprint 1).
package storage

import (
    "context"
    "errors"
    "io"
)

var (
    ErrNotImplemented = errors.New("storage: not implemented")
    ErrNotFound       = errors.New("storage: object not found")
)

type Object struct {
    Key         string // path-like, e.g. "products/<uuid>/<file>.jpg"
    ContentType string // "image/jpeg" | "image/png" | "image/webp"
    Size        int64  // bytes
}

type Storage interface {
    // Put writes r to backend storage and returns the public URL.
    Put(ctx context.Context, obj Object, r io.Reader) (url string, err error)
    // Delete removes the object by key. Idempotent (returns nil for not-found).
    Delete(ctx context.Context, key string) error
    // URL returns the public URL for an existing key without I/O.
    URL(key string) string
}
```

- [ ] **Step 2: Write the LocalStorage test**

`internal/shared/storage/local_test.go`:
```go
package storage

import (
    "bytes"
    "context"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/stretchr/testify/require"
)

func TestLocal_PutAndDelete(t *testing.T) {
    dir := t.TempDir()
    s := NewLocal(dir, "http://localhost:8080/uploads")

    payload := bytes.NewBufferString("hello")
    url, err := s.Put(context.Background(),
        Object{Key: "sub/file.txt", ContentType: "text/plain", Size: 5},
        payload)
    require.NoError(t, err)
    require.Equal(t, "http://localhost:8080/uploads/sub/file.txt", url)

    onDisk, err := os.ReadFile(filepath.Join(dir, "sub", "file.txt"))
    require.NoError(t, err)
    require.Equal(t, "hello", string(onDisk))

    require.NoError(t, s.Delete(context.Background(), "sub/file.txt"))
    _, err = os.Stat(filepath.Join(dir, "sub", "file.txt"))
    require.True(t, os.IsNotExist(err))
}

func TestLocal_DeleteMissing_IsNoop(t *testing.T) {
    dir := t.TempDir()
    s := NewLocal(dir, "http://localhost:8080/uploads")
    require.NoError(t, s.Delete(context.Background(), "does/not/exist.txt"))
}

func TestLocal_RejectsPathTraversal(t *testing.T) {
    dir := t.TempDir()
    s := NewLocal(dir, "http://localhost:8080/uploads")

    _, err := s.Put(context.Background(),
        Object{Key: "../escape.txt", ContentType: "text/plain", Size: 1},
        strings.NewReader("x"))
    require.Error(t, err)
}
```

- [ ] **Step 3: Run test — fails (no NewLocal)**

Run: `go test ./internal/shared/storage/... -v`
Expected: build error.

- [ ] **Step 4: Write LocalStorage**

`internal/shared/storage/local.go`:
```go
package storage

import (
    "context"
    "errors"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strings"
)

type LocalStorage struct {
    dir     string
    baseURL string
}

func NewLocal(dir, baseURL string) *LocalStorage {
    return &LocalStorage{
        dir:     filepath.Clean(dir),
        baseURL: strings.TrimRight(baseURL, "/"),
    }
}

func (s *LocalStorage) Put(ctx context.Context, obj Object, r io.Reader) (string, error) {
    if err := safeKey(obj.Key); err != nil {
        return "", err
    }
    target := filepath.Join(s.dir, filepath.FromSlash(obj.Key))
    if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
        return "", fmt.Errorf("storage.local: mkdir: %w", err)
    }
    f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
    if err != nil {
        return "", fmt.Errorf("storage.local: open: %w", err)
    }
    defer f.Close()
    if _, err := io.Copy(f, r); err != nil {
        return "", fmt.Errorf("storage.local: write: %w", err)
    }
    return s.URL(obj.Key), nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
    if err := safeKey(key); err != nil {
        return err
    }
    target := filepath.Join(s.dir, filepath.FromSlash(key))
    err := os.Remove(target)
    if err != nil && !os.IsNotExist(err) {
        return fmt.Errorf("storage.local: delete: %w", err)
    }
    return nil
}

func (s *LocalStorage) URL(key string) string {
    return s.baseURL + "/" + strings.TrimLeft(key, "/")
}

// safeKey rejects keys that would escape the base directory.
func safeKey(key string) error {
    if key == "" {
        return errors.New("storage: empty key")
    }
    clean := filepath.ToSlash(filepath.Clean("/" + key))
    if strings.Contains(clean, "..") || strings.HasPrefix(clean, "/..") {
        return fmt.Errorf("storage: unsafe key %q", key)
    }
    return nil
}
```

- [ ] **Step 5: Write GCS stub**

`internal/shared/storage/gcs.go`:
```go
package storage

import (
    "context"
    "io"
)

// GCSStorage is a stub for Sprint 1. All methods return ErrNotImplemented
// except URL which produces the canonical GCS public URL format so callers
// in dev paths don't crash if they accidentally request a URL.
type GCSStorage struct {
    bucket   string
    credPath string
}

func NewGCS(bucket, credentialsPath string) *GCSStorage {
    return &GCSStorage{bucket: bucket, credPath: credentialsPath}
}

func (s *GCSStorage) Put(ctx context.Context, obj Object, r io.Reader) (string, error) {
    return "", ErrNotImplemented
}

func (s *GCSStorage) Delete(ctx context.Context, key string) error {
    return ErrNotImplemented
}

func (s *GCSStorage) URL(key string) string {
    return "https://storage.googleapis.com/" + s.bucket + "/" + key
}
```

- [ ] **Step 6: Write factory + config**

`internal/shared/storage/factory.go`:
```go
package storage

import "fmt"

type Config struct {
    Driver         string // "local" | "gcs"
    LocalDir       string
    BaseURL        string
    GCSBucket      string
    GCSCredentials string
    MaxFileSize    int64
    AllowedMIMEs   []string
}

func New(cfg Config) (Storage, error) {
    switch cfg.Driver {
    case "local", "":
        return NewLocal(cfg.LocalDir, cfg.BaseURL), nil
    case "gcs":
        return NewGCS(cfg.GCSBucket, cfg.GCSCredentials), nil
    default:
        return nil, fmt.Errorf("storage: unknown driver %q", cfg.Driver)
    }
}
```

- [ ] **Step 7: Add testify dependency if not present**

Run:
```bash
go get github.com/stretchr/testify/require
go mod tidy
```

Run: `go test ./internal/shared/storage/... -v`
Expected: 3 tests PASS.

- [ ] **Step 8: Wire StorageConfig into config.Config**

Modify `internal/config/config.go`. Add a new field to `Config` struct and a new struct type. Insert after `Limit LimitConfig`:

```go
    Storage StorageConfig
```

And add the struct definition (place near other configs):

```go
type StorageConfig struct {
    Driver         string
    LocalDir       string
    BaseURL        string
    GCSBucket      string
    GCSCredentials string
    MaxFileSize    int64
    AllowedMIMEs   []string
}
```

In `Load()`, add at the end of the struct literal (before the closing `}`):
```go
        Storage: StorageConfig{
            Driver:         getEnv("STORAGE_DRIVER", "local"),
            LocalDir:       getEnv("STORAGE_LOCAL_DIR", "./uploads"),
            BaseURL:        getEnv("STORAGE_BASE_URL", "http://localhost:8080/uploads"),
            GCSBucket:      getEnv("STORAGE_GCS_BUCKET", ""),
            GCSCredentials: getEnv("STORAGE_GCS_CREDENTIALS", ""),
            MaxFileSize:    int64(getInt("STORAGE_MAX_FILE_SIZE", 5*1024*1024)),
            AllowedMIMEs:   csvOrSingle("STORAGE_ALLOWED_MIMES", ""),
        },
```

The default MIME list (when env is unset) is filled in `New(cfg)`. Update factory to set defaults:

In `internal/shared/storage/factory.go`, change `New` to set defaults when slice is empty:
```go
func New(cfg Config) (Storage, error) {
    if len(cfg.AllowedMIMEs) == 0 {
        cfg.AllowedMIMEs = []string{"image/jpeg", "image/png", "image/webp"}
    }
    if cfg.MaxFileSize == 0 {
        cfg.MaxFileSize = 5 * 1024 * 1024
    }
    switch cfg.Driver {
    case "local", "":
        return NewLocal(cfg.LocalDir, cfg.BaseURL), nil
    case "gcs":
        return NewGCS(cfg.GCSBucket, cfg.GCSCredentials), nil
    default:
        return nil, fmt.Errorf("storage: unknown driver %q", cfg.Driver)
    }
}
```

- [ ] **Step 9: Build everything**

Run: `go build ./...`
Expected: success.

- [ ] **Step 10: Commit**

```bash
git add internal/shared/storage/ internal/config/config.go go.mod go.sum
git commit -m "feat(shared): add storage interface with local backend and gcs stub"
```

---

### Task 8: Testfixtures package

**Files:**
- Create: `internal/testfixtures/fixtures.go`

These helpers are used by every integration test in later tasks. Each helper takes a `t *testing.T` plus a `DBTX` interface (so a test can pass a `pgx.Tx` and roll back).

- [ ] **Step 1: Write the fixtures package**

`internal/testfixtures/fixtures.go`:
```go
// Package testfixtures provides minimal row-insertion helpers for integration
// tests. Each helper requires a *testing.T (for fatal-on-error) and a DBTX
// (so callers can pass a pgx.Tx that rolls back at test end).
package testfixtures

import (
    "context"
    "fmt"
    "testing"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
)

// DBTX is the read/write subset both *pgxpool.Pool and pgx.Tx satisfy.
type DBTX interface {
    Exec(ctx context.Context, sql string, args ...any) (pgx.CommandTag, error)
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// BeginTx opens a tx that callers MUST rollback in t.Cleanup.
func BeginTx(t *testing.T, pool *pgxpool.Pool) pgx.Tx {
    t.Helper()
    tx, err := pool.Begin(context.Background())
    if err != nil {
        t.Fatalf("begin tx: %v", err)
    }
    t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
    return tx
}

type SeededUser struct {
    ID   uuid.UUID
    Name string
    Role string
}

// SeedUser inserts a user with given role. Email is randomized.
func SeedUser(t *testing.T, db DBTX, role string) SeededUser {
    t.Helper()
    id := uuid.New()
    email := fmt.Sprintf("u-%s@test.local", id.String()[:8])
    _, err := db.Exec(context.Background(),
        `INSERT INTO users (id, email, role, status, name)
         VALUES ($1, $2, $3, 'active', $4)`,
        id, email, role, "Test "+role)
    if err != nil {
        t.Fatalf("seed user: %v", err)
    }
    return SeededUser{ID: id, Name: "Test " + role, Role: role}
}

type SeededBrand struct {
    ID      uuid.UUID
    Slug    string
    Name    string
    OwnerID uuid.UUID
}

// SeedBrand inserts a brand. Creates an owner user if ownerID is zero.
func SeedBrand(t *testing.T, db DBTX, ownerID uuid.UUID) SeededBrand {
    t.Helper()
    if ownerID == uuid.Nil {
        ownerID = SeedUser(t, db, "brand").ID
    }
    id := uuid.New()
    slug := "brand-" + id.String()[:8]
    name := "Brand " + slug
    _, err := db.Exec(context.Background(),
        `INSERT INTO brands (id, slug, name, owner_user_id, status)
         VALUES ($1, $2, $3, $4, 'active')`,
        id, slug, name, ownerID)
    if err != nil {
        t.Fatalf("seed brand: %v", err)
    }
    return SeededBrand{ID: id, Slug: slug, Name: name, OwnerID: ownerID}
}

type SeededCategory struct {
    ID   uuid.UUID
    Slug string
}

func SeedCategory(t *testing.T, db DBTX) SeededCategory {
    t.Helper()
    id := uuid.New()
    slug := "cat-" + id.String()[:8]
    _, err := db.Exec(context.Background(),
        `INSERT INTO categories (id, slug, name) VALUES ($1, $2, $3)`,
        id, slug, "Cat "+slug)
    if err != nil {
        t.Fatalf("seed category: %v", err)
    }
    return SeededCategory{ID: id, Slug: slug}
}

type SeededStyleTag struct {
    ID   uuid.UUID
    Slug string
}

func SeedStyleTag(t *testing.T, db DBTX) SeededStyleTag {
    t.Helper()
    id := uuid.New()
    slug := "tag-" + id.String()[:8]
    _, err := db.Exec(context.Background(),
        `INSERT INTO style_tags (id, slug, name) VALUES ($1, $2, $3)`,
        id, slug, "Tag "+slug)
    if err != nil {
        t.Fatalf("seed style tag: %v", err)
    }
    return SeededStyleTag{ID: id, Slug: slug}
}

type SeededProduct struct {
    ID         uuid.UUID
    BrandID    uuid.UUID
    CategoryID uuid.UUID
    Slug       string
    Name       string
    Status     string
}

// SeedProduct inserts a product. Pass status="active" to make it visible
// in the public catalog; "draft" otherwise.
func SeedProduct(t *testing.T, db DBTX, brandID, categoryID uuid.UUID, status string) SeededProduct {
    t.Helper()
    id := uuid.New()
    slug := "p-" + id.String()[:8]
    name := "Product " + slug
    _, err := db.Exec(context.Background(),
        `INSERT INTO products (id, brand_id, category_id, slug, name, status)
         VALUES ($1, $2, $3, $4, $5, $6)`,
        id, brandID, categoryID, slug, name, status)
    if err != nil {
        t.Fatalf("seed product: %v", err)
    }
    return SeededProduct{
        ID: id, BrandID: brandID, CategoryID: categoryID,
        Slug: slug, Name: name, Status: status,
    }
}

// SeedVariant inserts a product_variants row with sane defaults.
func SeedVariant(t *testing.T, db DBTX, productID uuid.UUID, size, color string, price float64, stockQty int) uuid.UUID {
    t.Helper()
    id := uuid.New()
    sku := fmt.Sprintf("SKU-%s", id.String()[:8])
    _, err := db.Exec(context.Background(),
        `INSERT INTO product_variants
           (id, product_id, sku, size, color, price, stock_qty)
         VALUES ($1, $2, $3, $4, $5, $6, $7)`,
        id, productID, sku, size, color, price, stockQty)
    if err != nil {
        t.Fatalf("seed variant: %v", err)
    }
    return id
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/testfixtures/
git commit -m "test(fixtures): add seed helpers for integration tests"
```

---

## Phase B — Brand module (Tasks 9-13)

### Task 9: Brand domain + repo interfaces

**Files:**
- Create: `internal/brand/domain/brand.go`
- Create: `internal/brand/domain/errors.go`
- Create: `internal/brand/domain/dto.go`
- Create: `internal/brand/repo/repo.go`

- [ ] **Step 1: Write domain types**

`internal/brand/domain/brand.go`:
```go
package domain

import (
    "time"

    "github.com/google/uuid"
)

type BrandStatus string

const (
    BrandStatusPending   BrandStatus = "pending"
    BrandStatusActive    BrandStatus = "active"
    BrandStatusSuspended BrandStatus = "suspended"
)

type Brand struct {
    ID           uuid.UUID
    Slug         string
    Name         string
    OwnerUserID  uuid.UUID
    Story        *string
    LogoURL      *string
    BannerURL    *string
    WebsiteURL   *string
    Status       BrandStatus
    VerifiedAt   *time.Time
    CreatedAt    time.Time
    UpdatedAt    time.Time
    DeletedAt    *time.Time
}

type BrandAddress struct {
    ID          uuid.UUID
    BrandID     uuid.UUID
    Label       string
    AddressLine string
    Ward        string
    District    string
    City        string
    Country     string
    PostalCode  *string
    Phone       *string
    Latitude    *float64
    Longitude   *float64
    IsPrimary   bool
    IsPublic    bool
    CreatedAt   time.Time
    UpdatedAt   time.Time
    DeletedAt   *time.Time
}
```

- [ ] **Step 2: Write errors**

`internal/brand/domain/errors.go`:
```go
package domain

import (
    "net/http"

    "github.com/wearwhere/wearwhere_be/pkg/httpx"
)

var (
    ErrBrandNotFound   = httpx.NewAppError(http.StatusNotFound, "BRAND_NOT_FOUND", "Brand not found")
    ErrAddressNotFound = httpx.NewAppError(http.StatusNotFound, "ADDRESS_NOT_FOUND", "Address not found")
    ErrNoBrandOwned    = httpx.NewAppError(http.StatusForbidden, "NO_BRAND_OWNED", "User does not own a brand")
    ErrBrandSuspended  = httpx.NewAppError(http.StatusForbidden, "BRAND_SUSPENDED", "Brand is suspended")
    ErrSlugTaken       = httpx.NewAppError(http.StatusConflict, "SLUG_TAKEN", "Slug is already in use")
)
```

- [ ] **Step 3: Write DTOs**

`internal/brand/domain/dto.go`:
```go
package domain

import (
    "time"

    "github.com/google/uuid"
)

type UpdateBrandRequest struct {
    Name       *string `json:"name"        binding:"omitempty,min=2,max=120"`
    Slug       *string `json:"slug"        binding:"omitempty,slug,max=120"`
    Story      *string `json:"story"       binding:"omitempty,max=10000"`
    LogoURL    *string `json:"logo_url"    binding:"omitempty,url"`
    BannerURL  *string `json:"banner_url"  binding:"omitempty,url"`
    WebsiteURL *string `json:"website_url" binding:"omitempty,url"`
}

type CreateAddressRequest struct {
    Label       string   `json:"label"        binding:"required,max=80"`
    AddressLine string   `json:"address_line" binding:"required,max=255"`
    Ward        string   `json:"ward"         binding:"required,max=80"`
    District    string   `json:"district"     binding:"required,max=80"`
    City        string   `json:"city"         binding:"required,max=80"`
    Country     string   `json:"country"      binding:"omitempty,len=2"`
    PostalCode  *string  `json:"postal_code"  binding:"omitempty,max=20"`
    Phone       *string  `json:"phone"        binding:"omitempty,e164"`
    Latitude    *float64 `json:"latitude"     binding:"omitempty,latitude"`
    Longitude   *float64 `json:"longitude"    binding:"omitempty,longitude"`
    IsPrimary   bool     `json:"is_primary"`
    IsPublic    *bool    `json:"is_public"`
}

type UpdateAddressRequest struct {
    Label       *string  `json:"label"        binding:"omitempty,max=80"`
    AddressLine *string  `json:"address_line" binding:"omitempty,max=255"`
    Ward        *string  `json:"ward"         binding:"omitempty,max=80"`
    District    *string  `json:"district"     binding:"omitempty,max=80"`
    City        *string  `json:"city"         binding:"omitempty,max=80"`
    Country     *string  `json:"country"      binding:"omitempty,len=2"`
    PostalCode  *string  `json:"postal_code"  binding:"omitempty,max=20"`
    Phone       *string  `json:"phone"        binding:"omitempty,e164"`
    Latitude    *float64 `json:"latitude"     binding:"omitempty,latitude"`
    Longitude   *float64 `json:"longitude"    binding:"omitempty,longitude"`
    IsPrimary   *bool    `json:"is_primary"`
    IsPublic    *bool    `json:"is_public"`
}

type BrandResponse struct {
    ID         string  `json:"id"`
    Slug       string  `json:"slug"`
    Name       string  `json:"name"`
    Story      *string `json:"story,omitempty"`
    LogoURL    *string `json:"logo_url,omitempty"`
    BannerURL  *string `json:"banner_url,omitempty"`
    WebsiteURL *string `json:"website_url,omitempty"`
    Status     string  `json:"status"`
    CreatedAt  string  `json:"created_at"`
}

type AddressResponse struct {
    ID          string   `json:"id"`
    Label       string   `json:"label"`
    AddressLine string   `json:"address_line"`
    Ward        string   `json:"ward"`
    District    string   `json:"district"`
    City        string   `json:"city"`
    Country     string   `json:"country"`
    PostalCode  *string  `json:"postal_code,omitempty"`
    Phone       *string  `json:"phone,omitempty"`
    Latitude    *float64 `json:"latitude,omitempty"`
    Longitude   *float64 `json:"longitude,omitempty"`
    IsPrimary   bool     `json:"is_primary"`
    IsPublic    bool     `json:"is_public"`
}

func ToBrandResponse(b *Brand) BrandResponse {
    return BrandResponse{
        ID:         b.ID.String(),
        Slug:       b.Slug,
        Name:       b.Name,
        Story:      b.Story,
        LogoURL:    b.LogoURL,
        BannerURL:  b.BannerURL,
        WebsiteURL: b.WebsiteURL,
        Status:     string(b.Status),
        CreatedAt:  b.CreatedAt.UTC().Format(time.RFC3339),
    }
}

func ToAddressResponse(a *BrandAddress) AddressResponse {
    return AddressResponse{
        ID:          a.ID.String(),
        Label:       a.Label,
        AddressLine: a.AddressLine,
        Ward:        a.Ward,
        District:    a.District,
        City:        a.City,
        Country:     a.Country,
        PostalCode:  a.PostalCode,
        Phone:       a.Phone,
        Latitude:    a.Latitude,
        Longitude:   a.Longitude,
        IsPrimary:   a.IsPrimary,
        IsPublic:    a.IsPublic,
    }
}

// FindByOwnerResult bundles brand + ID for context middleware.
type FindByOwnerResult struct {
    BrandID uuid.UUID
    Status  BrandStatus
}
```

- [ ] **Step 4: Write repo interfaces**

`internal/brand/repo/repo.go`:
```go
// Package repo defines persistence interfaces for the brand module.
package repo

import (
    "context"
    "errors"

    "github.com/google/uuid"
    "github.com/wearwhere/wearwhere_be/internal/brand/domain"
)

var ErrNotFound = errors.New("brand: not found")

type BrandRepo interface {
    FindByID(ctx context.Context, id uuid.UUID) (*domain.Brand, error)
    FindBySlug(ctx context.Context, slug string) (*domain.Brand, error)
    FindByOwnerUserID(ctx context.Context, userID uuid.UUID) (*domain.Brand, error)

    // Update applies non-nil fields. Returns ErrNotFound if brand id doesn't exist.
    // Returns a sentinel slug-conflict error (handler maps to SLUG_TAKEN).
    Update(ctx context.Context, id uuid.UUID, req *domain.UpdateBrandRequest) error

    // List paginated active brands; q is optional fuzzy match on name.
    List(ctx context.Context, q string, sort string, limit, offset int) ([]*domain.Brand, int, error)
}

type AddressRepo interface {
    List(ctx context.Context, brandID uuid.UUID, includePrivate bool) ([]*domain.BrandAddress, error)
    FindByID(ctx context.Context, id, brandID uuid.UUID) (*domain.BrandAddress, error)
    Create(ctx context.Context, brandID uuid.UUID, req *domain.CreateAddressRequest) (*domain.BrandAddress, error)
    Update(ctx context.Context, id, brandID uuid.UUID, req *domain.UpdateAddressRequest) (*domain.BrandAddress, error)
    SoftDelete(ctx context.Context, id, brandID uuid.UUID) error
}

// ErrSlugTaken is returned by BrandRepo.Update when the new slug collides.
var ErrSlugTaken = errors.New("brand: slug taken")
```

- [ ] **Step 5: Build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 6: Commit**

```bash
git add internal/brand/domain/ internal/brand/repo/repo.go
git commit -m "feat(brand): add domain types, errors, DTOs, repo interfaces"
```

---

### Task 10: Brand PG repo + integration tests

**Files:**
- Create: `internal/brand/repo/brand_pg.go`
- Create: `internal/brand/repo/brand_pg_test.go` (integration)

- [ ] **Step 1: Write the failing integration test**

`internal/brand/repo/brand_pg_test.go`:
```go
//go:build integration

package repo

import (
    "context"
    "os"
    "testing"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/stretchr/testify/require"

    "github.com/wearwhere/wearwhere_be/internal/brand/domain"
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

func TestBrandPG_FindByOwnerUserID(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})

    repo := NewBrandPG(tx)
    b, err := repo.FindByOwnerUserID(context.Background(), sb.OwnerID)
    require.NoError(t, err)
    require.Equal(t, sb.ID, b.ID)
    require.Equal(t, sb.Slug, b.Slug)
    require.Equal(t, domain.BrandStatusActive, b.Status)
}

func TestBrandPG_FindByOwnerUserID_NotFound(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    u := testfixtures.SeedUser(t, tx, "brand") // no brand

    repo := NewBrandPG(tx)
    _, err := repo.FindByOwnerUserID(context.Background(), u.ID)
    require.ErrorIs(t, err, ErrNotFound)
}

func TestBrandPG_Update_Name(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})
    newName := "Updated Name"

    repo := NewBrandPG(tx)
    err := repo.Update(context.Background(), sb.ID,
        &domain.UpdateBrandRequest{Name: &newName})
    require.NoError(t, err)

    b, err := repo.FindByID(context.Background(), sb.ID)
    require.NoError(t, err)
    require.Equal(t, "Updated Name", b.Name)
}

func TestBrandPG_Update_SlugConflict(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb1 := testfixtures.SeedBrand(t, tx, [16]byte{})
    sb2 := testfixtures.SeedBrand(t, tx, [16]byte{})

    repo := NewBrandPG(tx)
    err := repo.Update(context.Background(), sb2.ID,
        &domain.UpdateBrandRequest{Slug: &sb1.Slug})
    require.ErrorIs(t, err, ErrSlugTaken)
}

func TestBrandPG_FindBySlug(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})

    repo := NewBrandPG(tx)
    b, err := repo.FindBySlug(context.Background(), sb.Slug)
    require.NoError(t, err)
    require.Equal(t, sb.ID, b.ID)
}

func TestBrandPG_List(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    for i := 0; i < 3; i++ {
        testfixtures.SeedBrand(t, tx, [16]byte{})
    }

    repo := NewBrandPG(tx)
    items, total, err := repo.List(context.Background(), "", "newest", 10, 0)
    require.NoError(t, err)
    require.GreaterOrEqual(t, len(items), 3)
    require.GreaterOrEqual(t, total, 3)
}
```

- [ ] **Step 2: Run test — fails (no NewBrandPG)**

Run: `go test -tags=integration ./internal/brand/repo/... -v`
Expected: build error.

- [ ] **Step 3: Write the repo**

`internal/brand/repo/brand_pg.go`:
```go
package repo

import (
    "context"
    "errors"
    "strings"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgconn"

    "github.com/wearwhere/wearwhere_be/internal/brand/domain"
)

// DBTX is the subset both *pgxpool.Pool and pgx.Tx satisfy.
type DBTX interface {
    Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type BrandPG struct{ db DBTX }

func NewBrandPG(db DBTX) *BrandPG { return &BrandPG{db: db} }

const brandCols = `id, slug, name, owner_user_id, story, logo_url, banner_url,
                   website_url, status, verified_at, created_at, updated_at, deleted_at`

func scanBrand(row pgx.Row) (*domain.Brand, error) {
    var b domain.Brand
    var status string
    err := row.Scan(
        &b.ID, &b.Slug, &b.Name, &b.OwnerUserID, &b.Story, &b.LogoURL,
        &b.BannerURL, &b.WebsiteURL, &status, &b.VerifiedAt,
        &b.CreatedAt, &b.UpdatedAt, &b.DeletedAt,
    )
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, ErrNotFound
        }
        return nil, err
    }
    b.Status = domain.BrandStatus(status)
    return &b, nil
}

func (r *BrandPG) FindByID(ctx context.Context, id uuid.UUID) (*domain.Brand, error) {
    return scanBrand(r.db.QueryRow(ctx,
        `SELECT `+brandCols+` FROM brands WHERE id=$1 AND deleted_at IS NULL`, id))
}

func (r *BrandPG) FindBySlug(ctx context.Context, slug string) (*domain.Brand, error) {
    return scanBrand(r.db.QueryRow(ctx,
        `SELECT `+brandCols+` FROM brands WHERE slug=$1 AND deleted_at IS NULL`, slug))
}

func (r *BrandPG) FindByOwnerUserID(ctx context.Context, userID uuid.UUID) (*domain.Brand, error) {
    return scanBrand(r.db.QueryRow(ctx,
        `SELECT `+brandCols+` FROM brands
         WHERE owner_user_id=$1 AND deleted_at IS NULL`, userID))
}

func (r *BrandPG) Update(ctx context.Context, id uuid.UUID, req *domain.UpdateBrandRequest) error {
    const q = `UPDATE brands SET
        name        = COALESCE($2, name),
        slug        = COALESCE($3, slug),
        story       = COALESCE($4, story),
        logo_url    = COALESCE($5, logo_url),
        banner_url  = COALESCE($6, banner_url),
        website_url = COALESCE($7, website_url),
        updated_at  = NOW()
        WHERE id=$1 AND deleted_at IS NULL`
    tag, err := r.db.Exec(ctx, q, id,
        req.Name, req.Slug, req.Story, req.LogoURL, req.BannerURL, req.WebsiteURL)
    if err != nil {
        if isUniqueViolation(err, "brands_slug_key") {
            return ErrSlugTaken
        }
        return err
    }
    if tag.RowsAffected() == 0 {
        return ErrNotFound
    }
    return nil
}

func (r *BrandPG) List(ctx context.Context, q, sort string, limit, offset int) ([]*domain.Brand, int, error) {
    args := []any{limit, offset}
    where := "deleted_at IS NULL AND status = 'active'"
    if q != "" {
        args = append(args, q)
        where += " AND name % $3"
    }

    orderBy := "created_at DESC"
    switch sort {
    case "a-z":
        orderBy = "name ASC"
    case "newest":
        orderBy = "created_at DESC"
    }

    selectSQL := `SELECT ` + brandCols + ` FROM brands WHERE ` + where +
        ` ORDER BY ` + orderBy + ` LIMIT $1 OFFSET $2`
    countSQL := `SELECT COUNT(*) FROM brands WHERE ` + where

    rows, err := r.db.Query(ctx, selectSQL, args...)
    if err != nil {
        return nil, 0, err
    }
    defer rows.Close()
    var items []*domain.Brand
    for rows.Next() {
        b, err := scanBrand(rows)
        if err != nil {
            return nil, 0, err
        }
        items = append(items, b)
    }
    if err := rows.Err(); err != nil {
        return nil, 0, err
    }

    var total int
    countArgs := args[2:] // skip limit, offset
    if err := r.db.QueryRow(ctx, countSQL, countArgs...).Scan(&total); err != nil {
        return nil, 0, err
    }
    return items, total, nil
}

// isUniqueViolation detects Postgres unique_violation (23505) optionally
// constrained to a named index.
func isUniqueViolation(err error, indexName string) bool {
    var pgErr *pgconn.PgError
    if !errors.As(err, &pgErr) {
        return false
    }
    if pgErr.Code != "23505" {
        return false
    }
    if indexName == "" {
        return true
    }
    return strings.Contains(pgErr.ConstraintName, indexName) ||
        strings.Contains(pgErr.Message, indexName)
}
```

- [ ] **Step 4: Run integration tests**

Make sure docker compose is up (`make up`) and migrations applied. Then:

```bash
docker compose exec -T postgres psql -U wearwhere -c "CREATE DATABASE wearwhere_test;" 2>/dev/null || true
TEST_DATABASE_URL="postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable" \
  migrate -path db/migrations -database "$TEST_DATABASE_URL" up

TEST_DATABASE_URL="postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable" \
  go test -tags=integration ./internal/brand/repo/... -v -run TestBrandPG
```

Expected: all 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/brand/repo/brand_pg.go internal/brand/repo/brand_pg_test.go
git commit -m "feat(brand): brand pg repo with slug-conflict detection"
```

---

### Task 11: BrandAddress PG repo + integration tests

**Files:**
- Create: `internal/brand/repo/address_pg.go`
- Create: `internal/brand/repo/address_pg_test.go`

- [ ] **Step 1: Write the failing test**

`internal/brand/repo/address_pg_test.go`:
```go
//go:build integration

package repo

import (
    "context"
    "testing"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    "github.com/wearwhere/wearwhere_be/internal/brand/domain"
    "github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

func TestAddressPG_CreateThenList(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})

    repo := NewAddressPG(tx)
    addr, err := repo.Create(context.Background(), sb.ID, &domain.CreateAddressRequest{
        Label: "HQ", AddressLine: "12 Phố Huế",
        Ward: "Ngô Thì Nhậm", District: "Hai Bà Trưng", City: "Hà Nội",
        IsPrimary: true,
    })
    require.NoError(t, err)
    require.True(t, addr.IsPrimary)
    require.Equal(t, "VN", addr.Country) // default

    items, err := repo.List(context.Background(), sb.ID, true)
    require.NoError(t, err)
    require.Len(t, items, 1)
}

func TestAddressPG_OnlyOnePrimary(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})
    repo := NewAddressPG(tx)
    ctx := context.Background()

    a1, err := repo.Create(ctx, sb.ID, &domain.CreateAddressRequest{
        Label: "First", AddressLine: "x", Ward: "x", District: "x", City: "x",
        IsPrimary: true,
    })
    require.NoError(t, err)
    require.True(t, a1.IsPrimary)

    a2, err := repo.Create(ctx, sb.ID, &domain.CreateAddressRequest{
        Label: "Second", AddressLine: "y", Ward: "y", District: "y", City: "y",
        IsPrimary: true,
    })
    require.NoError(t, err)
    require.True(t, a2.IsPrimary)

    // a1 must have been demoted by the create
    fetched, err := repo.FindByID(ctx, a1.ID, sb.ID)
    require.NoError(t, err)
    require.False(t, fetched.IsPrimary)
}

func TestAddressPG_IDORProtected(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sbA := testfixtures.SeedBrand(t, tx, [16]byte{})
    sbB := testfixtures.SeedBrand(t, tx, [16]byte{})
    repo := NewAddressPG(tx)
    ctx := context.Background()

    addr, err := repo.Create(ctx, sbA.ID, &domain.CreateAddressRequest{
        Label: "x", AddressLine: "x", Ward: "x", District: "x", City: "x",
    })
    require.NoError(t, err)

    // Brand B must not see brand A's address.
    _, err = repo.FindByID(ctx, addr.ID, sbB.ID)
    require.ErrorIs(t, err, ErrNotFound)

    err = repo.SoftDelete(ctx, addr.ID, sbB.ID)
    require.ErrorIs(t, err, ErrNotFound)
}

func TestAddressPG_PublicOnlyFilter(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})
    repo := NewAddressPG(tx)
    ctx := context.Background()

    pubFalse := false
    _, err := repo.Create(ctx, sb.ID, &domain.CreateAddressRequest{
        Label: "Public", AddressLine: "x", Ward: "x", District: "x", City: "x",
    })
    require.NoError(t, err)
    _, err = repo.Create(ctx, sb.ID, &domain.CreateAddressRequest{
        Label: "Private", AddressLine: "y", Ward: "y", District: "y", City: "y",
        IsPublic: &pubFalse,
    })
    require.NoError(t, err)

    publicOnly, err := repo.List(ctx, sb.ID, false)
    require.NoError(t, err)
    require.Len(t, publicOnly, 1)
    require.Equal(t, "Public", publicOnly[0].Label)

    all, err := repo.List(ctx, sb.ID, true)
    require.NoError(t, err)
    require.Len(t, all, 2)
}

// silence unused import if google/uuid trimmed
var _ = uuid.Nil
```

- [ ] **Step 2: Run test — fails**

Run: `go test -tags=integration ./internal/brand/repo/... -v -run TestAddressPG`
Expected: build error (no NewAddressPG).

- [ ] **Step 3: Write the repo**

`internal/brand/repo/address_pg.go`:
```go
package repo

import (
    "context"
    "errors"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"

    "github.com/wearwhere/wearwhere_be/internal/brand/domain"
)

type AddressPG struct{ db DBTX }

func NewAddressPG(db DBTX) *AddressPG { return &AddressPG{db: db} }

const addrCols = `id, brand_id, label, address_line, ward, district, city,
                  country, postal_code, phone, latitude, longitude,
                  is_primary, is_public, created_at, updated_at, deleted_at`

func scanAddress(row pgx.Row) (*domain.BrandAddress, error) {
    var a domain.BrandAddress
    err := row.Scan(
        &a.ID, &a.BrandID, &a.Label, &a.AddressLine, &a.Ward, &a.District, &a.City,
        &a.Country, &a.PostalCode, &a.Phone, &a.Latitude, &a.Longitude,
        &a.IsPrimary, &a.IsPublic, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
    )
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, ErrNotFound
        }
        return nil, err
    }
    return &a, nil
}

func (r *AddressPG) List(ctx context.Context, brandID uuid.UUID, includePrivate bool) ([]*domain.BrandAddress, error) {
    q := `SELECT ` + addrCols + ` FROM brand_addresses
          WHERE brand_id=$1 AND deleted_at IS NULL`
    if !includePrivate {
        q += ` AND is_public = TRUE`
    }
    q += ` ORDER BY is_primary DESC, created_at ASC`

    rows, err := r.db.Query(ctx, q, brandID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var items []*domain.BrandAddress
    for rows.Next() {
        a, err := scanAddress(rows)
        if err != nil {
            return nil, err
        }
        items = append(items, a)
    }
    return items, rows.Err()
}

func (r *AddressPG) FindByID(ctx context.Context, id, brandID uuid.UUID) (*domain.BrandAddress, error) {
    return scanAddress(r.db.QueryRow(ctx,
        `SELECT `+addrCols+` FROM brand_addresses
         WHERE id=$1 AND brand_id=$2 AND deleted_at IS NULL`, id, brandID))
}

func (r *AddressPG) Create(ctx context.Context, brandID uuid.UUID, req *domain.CreateAddressRequest) (*domain.BrandAddress, error) {
    country := req.Country
    if country == "" {
        country = "VN"
    }
    isPublic := true
    if req.IsPublic != nil {
        isPublic = *req.IsPublic
    }

    // Demote existing primary if new one will be primary.
    if req.IsPrimary {
        if _, err := r.db.Exec(ctx,
            `UPDATE brand_addresses SET is_primary = FALSE, updated_at = NOW()
             WHERE brand_id=$1 AND is_primary AND deleted_at IS NULL`,
            brandID); err != nil {
            return nil, err
        }
    }

    row := r.db.QueryRow(ctx,
        `INSERT INTO brand_addresses
         (brand_id, label, address_line, ward, district, city, country,
          postal_code, phone, latitude, longitude, is_primary, is_public)
         VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
         RETURNING `+addrCols,
        brandID, req.Label, req.AddressLine, req.Ward, req.District, req.City,
        country, req.PostalCode, req.Phone, req.Latitude, req.Longitude,
        req.IsPrimary, isPublic)
    return scanAddress(row)
}

func (r *AddressPG) Update(ctx context.Context, id, brandID uuid.UUID, req *domain.UpdateAddressRequest) (*domain.BrandAddress, error) {
    // If becoming primary, demote others first.
    if req.IsPrimary != nil && *req.IsPrimary {
        if _, err := r.db.Exec(ctx,
            `UPDATE brand_addresses SET is_primary = FALSE, updated_at = NOW()
             WHERE brand_id=$1 AND id <> $2 AND is_primary AND deleted_at IS NULL`,
            brandID, id); err != nil {
            return nil, err
        }
    }
    row := r.db.QueryRow(ctx,
        `UPDATE brand_addresses SET
           label        = COALESCE($3, label),
           address_line = COALESCE($4, address_line),
           ward         = COALESCE($5, ward),
           district     = COALESCE($6, district),
           city         = COALESCE($7, city),
           country      = COALESCE($8, country),
           postal_code  = COALESCE($9, postal_code),
           phone        = COALESCE($10, phone),
           latitude     = COALESCE($11, latitude),
           longitude    = COALESCE($12, longitude),
           is_primary   = COALESCE($13, is_primary),
           is_public    = COALESCE($14, is_public),
           updated_at   = NOW()
         WHERE id=$1 AND brand_id=$2 AND deleted_at IS NULL
         RETURNING `+addrCols,
        id, brandID, req.Label, req.AddressLine, req.Ward, req.District, req.City,
        req.Country, req.PostalCode, req.Phone, req.Latitude, req.Longitude,
        req.IsPrimary, req.IsPublic)
    return scanAddress(row)
}

func (r *AddressPG) SoftDelete(ctx context.Context, id, brandID uuid.UUID) error {
    tag, err := r.db.Exec(ctx,
        `UPDATE brand_addresses SET deleted_at = NOW(), updated_at = NOW()
         WHERE id=$1 AND brand_id=$2 AND deleted_at IS NULL`, id, brandID)
    if err != nil {
        return err
    }
    if tag.RowsAffected() == 0 {
        return ErrNotFound
    }
    return nil
}
```

- [ ] **Step 4: Run integration tests**

```bash
TEST_DATABASE_URL="postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable" \
  go test -tags=integration ./internal/brand/repo/... -v -run TestAddressPG
```

Expected: 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/brand/repo/address_pg.go internal/brand/repo/address_pg_test.go
git commit -m "feat(brand): address pg repo with primary-flip and IDOR protection"
```

---

### Task 12: Brand service + unit tests

**Files:**
- Create: `internal/brand/service/brand_service.go`
- Create: `internal/brand/service/brand_service_test.go`

The service composes the two repos. It is thin (mostly delegation) — its real job is to translate `repo.ErrNotFound` and `repo.ErrSlugTaken` into the corresponding `*httpx.AppError` from `domain/errors.go`.

- [ ] **Step 1: Write unit test with fake repos**

`internal/brand/service/brand_service_test.go`:
```go
package service

import (
    "context"
    "errors"
    "testing"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    "github.com/wearwhere/wearwhere_be/internal/brand/domain"
    "github.com/wearwhere/wearwhere_be/internal/brand/repo"
)

// fakeBrandRepo — minimal in-memory impl for unit tests.
type fakeBrandRepo struct {
    byID         map[uuid.UUID]*domain.Brand
    byOwner      map[uuid.UUID]*domain.Brand
    updateErr    error
}

func (f *fakeBrandRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Brand, error) {
    b, ok := f.byID[id]
    if !ok {
        return nil, repo.ErrNotFound
    }
    return b, nil
}
func (f *fakeBrandRepo) FindBySlug(ctx context.Context, slug string) (*domain.Brand, error) {
    for _, b := range f.byID {
        if b.Slug == slug {
            return b, nil
        }
    }
    return nil, repo.ErrNotFound
}
func (f *fakeBrandRepo) FindByOwnerUserID(ctx context.Context, uid uuid.UUID) (*domain.Brand, error) {
    b, ok := f.byOwner[uid]
    if !ok {
        return nil, repo.ErrNotFound
    }
    return b, nil
}
func (f *fakeBrandRepo) Update(ctx context.Context, id uuid.UUID, req *domain.UpdateBrandRequest) error {
    if f.updateErr != nil {
        return f.updateErr
    }
    b, ok := f.byID[id]
    if !ok {
        return repo.ErrNotFound
    }
    if req.Name != nil {
        b.Name = *req.Name
    }
    if req.Slug != nil {
        b.Slug = *req.Slug
    }
    return nil
}
func (f *fakeBrandRepo) List(ctx context.Context, q, sort string, limit, offset int) ([]*domain.Brand, int, error) {
    return nil, 0, nil
}

type fakeAddrRepo struct{}

func (f *fakeAddrRepo) List(ctx context.Context, brandID uuid.UUID, includePrivate bool) ([]*domain.BrandAddress, error) {
    return nil, nil
}
func (f *fakeAddrRepo) FindByID(ctx context.Context, id, brandID uuid.UUID) (*domain.BrandAddress, error) {
    return nil, repo.ErrNotFound
}
func (f *fakeAddrRepo) Create(ctx context.Context, brandID uuid.UUID, req *domain.CreateAddressRequest) (*domain.BrandAddress, error) {
    return nil, nil
}
func (f *fakeAddrRepo) Update(ctx context.Context, id, brandID uuid.UUID, req *domain.UpdateAddressRequest) (*domain.BrandAddress, error) {
    return nil, repo.ErrNotFound
}
func (f *fakeAddrRepo) SoftDelete(ctx context.Context, id, brandID uuid.UUID) error {
    return repo.ErrNotFound
}

func TestService_GetByID_NotFound_Translates(t *testing.T) {
    svc := New(&fakeBrandRepo{byID: map[uuid.UUID]*domain.Brand{}, byOwner: map[uuid.UUID]*domain.Brand{}}, &fakeAddrRepo{})
    _, err := svc.GetByID(context.Background(), uuid.New())
    require.ErrorIs(t, err, domain.ErrBrandNotFound)
}

func TestService_UpdateOwn_SlugConflict_Translates(t *testing.T) {
    id := uuid.New()
    svc := New(
        &fakeBrandRepo{
            byID:      map[uuid.UUID]*domain.Brand{id: {ID: id}},
            byOwner:   map[uuid.UUID]*domain.Brand{},
            updateErr: repo.ErrSlugTaken,
        },
        &fakeAddrRepo{},
    )
    newSlug := "taken"
    err := svc.UpdateOwn(context.Background(), id, &domain.UpdateBrandRequest{Slug: &newSlug})
    require.ErrorIs(t, err, domain.ErrSlugTaken)
}

func TestService_DeleteAddress_NotFound_Translates(t *testing.T) {
    svc := New(&fakeBrandRepo{}, &fakeAddrRepo{})
    err := svc.DeleteAddress(context.Background(), uuid.New(), uuid.New())
    require.ErrorIs(t, err, domain.ErrAddressNotFound)
}

// silence unused
var _ = errors.New
```

- [ ] **Step 2: Run — fails (no New, no methods)**

Run: `go test ./internal/brand/service/... -v`
Expected: build error.

- [ ] **Step 3: Write the service**

`internal/brand/service/brand_service.go`:
```go
package service

import (
    "context"
    "errors"

    "github.com/google/uuid"

    "github.com/wearwhere/wearwhere_be/internal/brand/domain"
    "github.com/wearwhere/wearwhere_be/internal/brand/repo"
)

type Service struct {
    brands    repo.BrandRepo
    addresses repo.AddressRepo
}

func New(b repo.BrandRepo, a repo.AddressRepo) *Service {
    return &Service{brands: b, addresses: a}
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*domain.Brand, error) {
    b, err := s.brands.FindByID(ctx, id)
    if errors.Is(err, repo.ErrNotFound) {
        return nil, domain.ErrBrandNotFound
    }
    return b, err
}

func (s *Service) GetBySlug(ctx context.Context, slug string) (*domain.Brand, error) {
    b, err := s.brands.FindBySlug(ctx, slug)
    if errors.Is(err, repo.ErrNotFound) {
        return nil, domain.ErrBrandNotFound
    }
    return b, err
}

func (s *Service) UpdateOwn(ctx context.Context, id uuid.UUID, req *domain.UpdateBrandRequest) error {
    err := s.brands.Update(ctx, id, req)
    switch {
    case errors.Is(err, repo.ErrNotFound):
        return domain.ErrBrandNotFound
    case errors.Is(err, repo.ErrSlugTaken):
        return domain.ErrSlugTaken
    }
    return err
}

func (s *Service) ListBrands(ctx context.Context, q, sort string, limit, offset int) ([]*domain.Brand, int, error) {
    return s.brands.List(ctx, q, sort, limit, offset)
}

// Address operations
func (s *Service) ListAddresses(ctx context.Context, brandID uuid.UUID, includePrivate bool) ([]*domain.BrandAddress, error) {
    return s.addresses.List(ctx, brandID, includePrivate)
}

func (s *Service) CreateAddress(ctx context.Context, brandID uuid.UUID, req *domain.CreateAddressRequest) (*domain.BrandAddress, error) {
    return s.addresses.Create(ctx, brandID, req)
}

func (s *Service) UpdateAddress(ctx context.Context, id, brandID uuid.UUID, req *domain.UpdateAddressRequest) (*domain.BrandAddress, error) {
    a, err := s.addresses.Update(ctx, id, brandID, req)
    if errors.Is(err, repo.ErrNotFound) {
        return nil, domain.ErrAddressNotFound
    }
    return a, err
}

func (s *Service) DeleteAddress(ctx context.Context, id, brandID uuid.UUID) error {
    err := s.addresses.SoftDelete(ctx, id, brandID)
    if errors.Is(err, repo.ErrNotFound) {
        return domain.ErrAddressNotFound
    }
    return err
}
```

- [ ] **Step 4: Run tests — pass**

Run: `go test ./internal/brand/service/... -v`
Expected: 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/brand/service/
git commit -m "feat(brand): service translates repo errors to domain AppErrors"
```

---

### Task 13: BrandContext middleware + handlers + routes + main wiring

This task is bigger than ideal but the components are tightly coupled and only meaningful together (handlers need middleware which needs main wiring to be exercised).

**Files:**
- Create: `internal/brand/middleware/brand_context.go`
- Create: `internal/brand/middleware/brand_context_test.go`
- Create: `internal/brand/handler/brand_handler.go`
- Create: `internal/brand/handler/address_handler.go`
- Create: `internal/brand/handler/routes.go`
- Modify: `cmd/api/main.go`

- [ ] **Step 1: Write middleware test**

`internal/brand/middleware/brand_context_test.go`:
```go
package middleware

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
    "github.com/wearwhere/wearwhere_be/internal/brand/domain"
    "github.com/wearwhere/wearwhere_be/internal/brand/repo"
)

// fakeBrandRepo: just enough to test middleware decisions.
type fakeRepo struct {
    brand *domain.Brand
    err   error
}

func (f *fakeRepo) FindByOwnerUserID(ctx context.Context, id uuid.UUID) (*domain.Brand, error) {
    return f.brand, f.err
}

// other methods unused
func (f *fakeRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Brand, error)        { return nil, nil }
func (f *fakeRepo) FindBySlug(ctx context.Context, slug string) (*domain.Brand, error)        { return nil, nil }
func (f *fakeRepo) Update(ctx context.Context, id uuid.UUID, r *domain.UpdateBrandRequest) error { return nil }
func (f *fakeRepo) List(ctx context.Context, q, sort string, l, o int) ([]*domain.Brand, int, error) {
    return nil, 0, nil
}

func setup(brandRepo repo.BrandRepo, userID uuid.UUID) *gin.Engine {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    r.Use(func(c *gin.Context) {
        // simulate RequireAuth populating user_id
        authmw.SetUserIDForTest(c, userID)
        c.Next()
    })
    r.Use(BrandContext(brandRepo))
    r.GET("/x", func(c *gin.Context) {
        bid, _ := c.Get("brand_id")
        c.JSON(http.StatusOK, gin.H{"brand_id": bid})
    })
    return r
}

func TestBrandContext_NoBrand_403(t *testing.T) {
    r := setup(&fakeRepo{err: repo.ErrNotFound}, uuid.New())
    rec := httptest.NewRecorder()
    req, _ := http.NewRequest("GET", "/x", nil)
    r.ServeHTTP(rec, req)
    require.Equal(t, http.StatusForbidden, rec.Code)
    require.Contains(t, rec.Body.String(), "NO_BRAND_OWNED")
}

func TestBrandContext_Suspended_403(t *testing.T) {
    bid := uuid.New()
    uid := uuid.New()
    r := setup(&fakeRepo{brand: &domain.Brand{ID: bid, OwnerUserID: uid, Status: domain.BrandStatusSuspended}}, uid)
    rec := httptest.NewRecorder()
    req, _ := http.NewRequest("GET", "/x", nil)
    r.ServeHTTP(rec, req)
    require.Equal(t, http.StatusForbidden, rec.Code)
    require.Contains(t, rec.Body.String(), "BRAND_SUSPENDED")
}

func TestBrandContext_Active_PassesThrough(t *testing.T) {
    bid := uuid.New()
    uid := uuid.New()
    r := setup(&fakeRepo{brand: &domain.Brand{ID: bid, OwnerUserID: uid, Status: domain.BrandStatusActive}}, uid)
    rec := httptest.NewRecorder()
    req, _ := http.NewRequest("GET", "/x", nil)
    r.ServeHTTP(rec, req)
    require.Equal(t, http.StatusOK, rec.Code)
    require.Contains(t, rec.Body.String(), bid.String())
}
```

This test relies on a helper `authmw.SetUserIDForTest`. Add it to auth middleware:

Modify `internal/auth/middleware/context.go` — add at the end:
```go
// SetUserIDForTest is exported only for downstream test packages that need
// to simulate the post-RequireAuth state without a real JWT. Not for prod use.
func SetUserIDForTest(c *gin.Context, userID uuid.UUID) {
    c.Set(ctxUserID, userID)
}
```

- [ ] **Step 2: Write the middleware**

`internal/brand/middleware/brand_context.go`:
```go
// Package middleware: BrandContext loads the caller's brand and attaches
// brand_id to the request context. Chain after RequireAuth + RequireRole.
package middleware

import (
    "errors"

    "github.com/gin-gonic/gin"

    authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
    "github.com/wearwhere/wearwhere_be/internal/brand/domain"
    "github.com/wearwhere/wearwhere_be/internal/brand/repo"
    "github.com/wearwhere/wearwhere_be/pkg/httpx"
)

const (
    CtxBrandID = "brand.id"
    CtxBrand   = "brand.entity"
)

func BrandContext(brandRepo repo.BrandRepo) gin.HandlerFunc {
    return func(c *gin.Context) {
        uid, ok := authmw.UserID(c)
        if !ok {
            httpx.Error(c, 401, "UNAUTHORIZED", "Authentication required")
            return
        }
        b, err := brandRepo.FindByOwnerUserID(c.Request.Context(), uid)
        switch {
        case errors.Is(err, repo.ErrNotFound):
            httpx.ErrorFromApp(c, domain.ErrNoBrandOwned)
            return
        case err != nil:
            httpx.ErrorFromApp(c, domain.ErrBrandNotFound)
            return
        case b.Status == domain.BrandStatusSuspended:
            httpx.ErrorFromApp(c, domain.ErrBrandSuspended)
            return
        }
        c.Set(CtxBrandID, b.ID)
        c.Set(CtxBrand, b)
        c.Next()
    }
}
```

- [ ] **Step 3: Run middleware tests**

Run: `go test ./internal/brand/middleware/... -v`
Expected: 3 tests PASS.

- [ ] **Step 4: Write brand handler**

`internal/brand/handler/brand_handler.go`:
```go
package handler

import (
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    brandmw "github.com/wearwhere/wearwhere_be/internal/brand/middleware"
    "github.com/wearwhere/wearwhere_be/internal/brand/domain"
    "github.com/wearwhere/wearwhere_be/internal/brand/service"
    "github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type BrandHandler struct{ svc *service.Service }

func NewBrandHandler(svc *service.Service) *BrandHandler { return &BrandHandler{svc: svc} }

func (h *BrandHandler) Me(c *gin.Context) {
    bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
    b, err := h.svc.GetByID(c.Request.Context(), bid)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    httpx.OK(c, gin.H{"brand": domain.ToBrandResponse(b)})
}

func (h *BrandHandler) UpdateMe(c *gin.Context) {
    bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
    var req domain.UpdateBrandRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
        return
    }
    if err := h.svc.UpdateOwn(c.Request.Context(), bid, &req); err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    b, err := h.svc.GetByID(c.Request.Context(), bid)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    httpx.OK(c, gin.H{"brand": domain.ToBrandResponse(b)})
}
```

- [ ] **Step 5: Write address handler**

`internal/brand/handler/address_handler.go`:
```go
package handler

import (
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    brandmw "github.com/wearwhere/wearwhere_be/internal/brand/middleware"
    "github.com/wearwhere/wearwhere_be/internal/brand/domain"
    "github.com/wearwhere/wearwhere_be/internal/brand/service"
    "github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type AddressHandler struct{ svc *service.Service }

func NewAddressHandler(svc *service.Service) *AddressHandler { return &AddressHandler{svc: svc} }

func (h *AddressHandler) List(c *gin.Context) {
    bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
    items, err := h.svc.ListAddresses(c.Request.Context(), bid, true)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    resp := make([]domain.AddressResponse, 0, len(items))
    for _, a := range items {
        resp = append(resp, domain.ToAddressResponse(a))
    }
    httpx.OK(c, gin.H{"items": resp})
}

func (h *AddressHandler) Create(c *gin.Context) {
    bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
    var req domain.CreateAddressRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
        return
    }
    a, err := h.svc.CreateAddress(c.Request.Context(), bid, &req)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    httpx.Created(c, gin.H{"address": domain.ToAddressResponse(a)})
}

func (h *AddressHandler) Update(c *gin.Context) {
    bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
    id, err := uuid.Parse(c.Param("id"))
    if err != nil {
        httpx.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid address id")
        return
    }
    var req domain.UpdateAddressRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
        return
    }
    a, err := h.svc.UpdateAddress(c.Request.Context(), id, bid, &req)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    httpx.OK(c, gin.H{"address": domain.ToAddressResponse(a)})
}

func (h *AddressHandler) Delete(c *gin.Context) {
    bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
    id, err := uuid.Parse(c.Param("id"))
    if err != nil {
        httpx.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid address id")
        return
    }
    if err := h.svc.DeleteAddress(c.Request.Context(), id, bid); err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    httpx.NoContent(c)
}
```

- [ ] **Step 6: Write routes**

`internal/brand/handler/routes.go`:
```go
package handler

import (
    "github.com/gin-gonic/gin"
)

type Deps struct {
    Brand   *BrandHandler
    Address *AddressHandler
}

// Mount registers /brand/me routes on the given group.
// Caller is responsible for chaining RequireAuth + RequireRole + BrandContext
// onto the group before calling Mount.
func Mount(rg *gin.RouterGroup, d *Deps) {
    rg.GET("",  d.Brand.Me)
    rg.PATCH("", d.Brand.UpdateMe)

    addr := rg.Group("/addresses")
    {
        addr.GET("",       d.Address.List)
        addr.POST("",      d.Address.Create)
        addr.PATCH(":id",  d.Address.Update)
        addr.DELETE(":id", d.Address.Delete)
    }
}
```

- [ ] **Step 7: Wire into main.go**

Modify `cmd/api/main.go`. Add imports:
```go
    brandhandler "github.com/wearwhere/wearwhere_be/internal/brand/handler"
    brandmw "github.com/wearwhere/wearwhere_be/internal/brand/middleware"
    brandrepo "github.com/wearwhere/wearwhere_be/internal/brand/repo"
    brandservice "github.com/wearwhere/wearwhere_be/internal/brand/service"
    authdomain "github.com/wearwhere/wearwhere_be/internal/auth/domain"
```

After the existing `// ── repos ──` block, append:
```go
    brandRepo := brandrepo.NewBrandPG(pgPool)
    addressRepo := brandrepo.NewAddressPG(pgPool)
```

After the existing `// ── services ──` block, append:
```go
    brandSvc := brandservice.New(brandRepo, addressRepo)
```

After `// ── handlers ──` block, append:
```go
    brandDeps := &brandhandler.Deps{
        Brand:   brandhandler.NewBrandHandler(brandSvc),
        Address: brandhandler.NewAddressHandler(brandSvc),
    }
```

After the existing `handler.Mount(v1, deps)` line, add:
```go
    brandGroup := v1.Group("/brand/me",
        middleware.RequireAuth(jwtIssuer),
        middleware.RequireRole(authdomain.RoleBrand),
        brandmw.BrandContext(brandRepo),
    )
    brandhandler.Mount(brandGroup, brandDeps)
```

- [ ] **Step 8: Build + run tests**

```bash
go build ./...
go test ./internal/brand/... -v
```

Expected: build success, all tests pass.

- [ ] **Step 9: Manual smoke**

Bring server up: `make up && make migrate-up && make run` (in another shell).

```bash
# Login as seeded brand owner (password "DevBrand@1234" if bcrypt hash matches; otherwise update seed)
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"owner1@local.test","password":"DevBrand@1234"}'
# extract access_token

TOKEN="<paste>"
curl http://localhost:8080/api/v1/brand/me -H "Authorization: Bearer $TOKEN"
# expect brand "Local-X" payload

curl -X PATCH http://localhost:8080/api/v1/brand/me \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"story":"Updated story"}'
# expect brand with updated story

curl http://localhost:8080/api/v1/brand/me/addresses \
  -H "Authorization: Bearer $TOKEN"
# expect 2 addresses (HQ + Showroom)
```

If the password hash in seed doesn't match `DevBrand@1234`, generate one via the hash package and update migration 000016. Quick approach: register a new user via `/auth/register`, then update its role to 'brand' in DB and assign as owner of a brand.

- [ ] **Step 10: Commit**

```bash
git add internal/brand/ internal/auth/middleware/context.go cmd/api/main.go
git commit -m "feat(brand): BrandContext middleware + brand/address handlers wired into main"
```

---

## Phase C — Product module: write side (Tasks 14-19)

### Task 14: Product domain types, errors, DTOs, repo interfaces

**Files:**
- Create: `internal/product/domain/product.go`
- Create: `internal/product/domain/errors.go`
- Create: `internal/product/domain/dto.go`
- Create: `internal/product/repo/repo.go`

- [ ] **Step 1: Domain types**

`internal/product/domain/product.go`:
```go
package domain

import (
    "time"

    "github.com/google/uuid"
)

type ProductStatus string

const (
    ProductStatusDraft    ProductStatus = "draft"
    ProductStatusActive   ProductStatus = "active"
    ProductStatusArchived ProductStatus = "archived"
)

type Product struct {
    ID          uuid.UUID
    BrandID     uuid.UUID
    CategoryID  uuid.UUID
    Slug        string
    Name        string
    Description *string
    Status      ProductStatus
    Currency    string
    SoldCount   int
    ViewCount   int
    CreatedAt   time.Time
    UpdatedAt   time.Time
    DeletedAt   *time.Time
}

type Variant struct {
    ID         uuid.UUID
    ProductID  uuid.UUID
    SKU        string
    Size       string
    Color      string
    ColorHex   *string
    Price      float64
    StockQty   int
    IsActive   bool
    ImageID    *uuid.UUID
    CreatedAt  time.Time
    UpdatedAt  time.Time
    DeletedAt  *time.Time
}

type Image struct {
    ID         uuid.UUID
    ProductID  uuid.UUID
    URL        string
    StorageKey string
    AltText    *string
    SortOrder  int
    IsPrimary  bool
    CreatedAt  time.Time
}

type Category struct {
    ID           uuid.UUID
    Slug         string
    Name         string
    DisplayOrder int
}

type StyleTag struct {
    ID   uuid.UUID
    Slug string
    Name string
}

// CatalogItem is a denormalized row for the public listing endpoint.
type CatalogItem struct {
    Product
    BrandSlug    string
    BrandName    string
    MinPrice     float64
    MaxPrice     float64
    InStock      bool
    PrimaryImage *string
}
```

- [ ] **Step 2: Errors**

`internal/product/domain/errors.go`:
```go
package domain

import (
    "net/http"

    "github.com/wearwhere/wearwhere_be/pkg/httpx"
)

var (
    ErrProductNotFound     = httpx.NewAppError(http.StatusNotFound, "PRODUCT_NOT_FOUND", "Product not found")
    ErrVariantNotFound     = httpx.NewAppError(http.StatusNotFound, "VARIANT_NOT_FOUND", "Variant not found")
    ErrImageNotFound       = httpx.NewAppError(http.StatusNotFound, "IMAGE_NOT_FOUND", "Image not found")
    ErrCategoryNotFound    = httpx.NewAppError(http.StatusNotFound, "CATEGORY_NOT_FOUND", "Category not found")
    ErrSlugTaken           = httpx.NewAppError(http.StatusConflict, "SLUG_TAKEN", "Slug already in use")
    ErrVariantConflict     = httpx.NewAppError(http.StatusConflict, "VARIANT_CONFLICT", "Variant with this size+color already exists")
    ErrProductNotPublishable = httpx.NewAppError(http.StatusConflict, "PRODUCT_NOT_PUBLISHABLE", "Product needs at least 1 variant and 1 image to publish")
    ErrInvalidMIME         = httpx.NewAppError(http.StatusBadRequest, "INVALID_MIME", "Unsupported file type")
    ErrFileTooLarge        = httpx.NewAppError(http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "File exceeds maximum size")
    ErrTooManyFiles        = httpx.NewAppError(http.StatusBadRequest, "TOO_MANY_FILES", "Too many files in one request")
    ErrStorageError        = httpx.NewAppError(http.StatusBadGateway, "STORAGE_ERROR", "Storage backend failure")
)
```

- [ ] **Step 3: DTOs**

`internal/product/domain/dto.go`:
```go
package domain

import "time"

type CreateProductRequest struct {
    Name        string   `json:"name"          binding:"required,min=2,max=200"`
    Slug        string   `json:"slug"          binding:"omitempty,slug,max=200"`
    Description string   `json:"description"   binding:"omitempty,max=5000"`
    CategoryID  string   `json:"category_id"   binding:"required,uuid"`
    StyleTagIDs []string `json:"style_tag_ids" binding:"omitempty,max=10,dive,uuid"`
}

type UpdateProductRequest struct {
    Name        *string  `json:"name"          binding:"omitempty,min=2,max=200"`
    Slug        *string  `json:"slug"          binding:"omitempty,slug,max=200"`
    Description *string  `json:"description"   binding:"omitempty,max=5000"`
    CategoryID  *string  `json:"category_id"   binding:"omitempty,uuid"`
    Status      *string  `json:"status"        binding:"omitempty,oneof=draft active archived"`
    StyleTagIDs []string `json:"style_tag_ids" binding:"omitempty,max=10,dive,uuid"`
}

type CreateVariantRequest struct {
    SKU      string  `json:"sku"       binding:"required,min=1,max=64"`
    Size     string  `json:"size"      binding:"required,max=20"`
    Color    string  `json:"color"     binding:"required,max=50"`
    ColorHex string  `json:"color_hex" binding:"omitempty,hexcolor"`
    Price    float64 `json:"price"     binding:"required,gt=0"`
    StockQty int     `json:"stock_qty" binding:"min=0"`
    ImageID  string  `json:"image_id"  binding:"omitempty,uuid"`
}

type UpdateVariantRequest struct {
    Size     *string  `json:"size"      binding:"omitempty,max=20"`
    Color    *string  `json:"color"     binding:"omitempty,max=50"`
    ColorHex *string  `json:"color_hex" binding:"omitempty,hexcolor"`
    Price    *float64 `json:"price"     binding:"omitempty,gt=0"`
    StockQty *int     `json:"stock_qty" binding:"omitempty,min=0"`
    IsActive *bool    `json:"is_active"`
    ImageID  *string  `json:"image_id"  binding:"omitempty,uuid"`
}

type UpdateImageRequest struct {
    SortOrder *int    `json:"sort_order" binding:"omitempty,min=0"`
    AltText   *string `json:"alt_text"   binding:"omitempty,max=200"`
    IsPrimary *bool   `json:"is_primary"`
}

type ListProductsQuery struct {
    Q        string   `form:"q"          binding:"omitempty,max=100"`
    Category string   `form:"category"   binding:"omitempty,slug"`
    Brand    string   `form:"brand"      binding:"omitempty,slug"`
    Style    []string `form:"style"      binding:"omitempty,max=10,dive,slug"`
    Size     []string `form:"size"       binding:"omitempty,max=10,dive,max=20"`
    Color    []string `form:"color"      binding:"omitempty,max=10,dive,max=50"`
    PriceMin *float64 `form:"price_min"  binding:"omitempty,gte=0"`
    PriceMax *float64 `form:"price_max"  binding:"omitempty,gtefield=PriceMin"`
    Sort     string   `form:"sort"       binding:"omitempty,oneof=relevance newest popular price_asc price_desc"`
    Page     int      `form:"page,default=1"   binding:"min=1"`
    Limit    int      `form:"limit,default=24" binding:"min=1,max=60"`
}

// ── responses ──
type ProductSummary struct {
    ID           string  `json:"id"`
    Slug         string  `json:"slug"`
    Name         string  `json:"name"`
    BrandSlug    string  `json:"brand_slug"`
    BrandName    string  `json:"brand_name"`
    Currency     string  `json:"currency"`
    MinPrice     float64 `json:"min_price"`
    MaxPrice     float64 `json:"max_price"`
    InStock      bool    `json:"in_stock"`
    PrimaryImage *string `json:"primary_image,omitempty"`
}

type ProductDetail struct {
    ID          string           `json:"id"`
    Slug        string           `json:"slug"`
    Name        string           `json:"name"`
    Description *string          `json:"description,omitempty"`
    Status      string           `json:"status"`
    Currency    string           `json:"currency"`
    Brand       *BrandRef        `json:"brand"`
    Category    *CategoryRef     `json:"category"`
    StyleTags   []StyleTagRef    `json:"style_tags"`
    Variants    []VariantResp    `json:"variants"`
    Images      []ImageResp      `json:"images"`
    CreatedAt   string           `json:"created_at"`
}

type BrandRef struct {
    ID   string `json:"id"`
    Slug string `json:"slug"`
    Name string `json:"name"`
}

type CategoryRef struct {
    ID   string `json:"id"`
    Slug string `json:"slug"`
    Name string `json:"name"`
}

type StyleTagRef struct {
    ID   string `json:"id"`
    Slug string `json:"slug"`
    Name string `json:"name"`
}

type VariantResp struct {
    ID       string  `json:"id"`
    SKU      string  `json:"sku"`
    Size     string  `json:"size"`
    Color    string  `json:"color"`
    ColorHex *string `json:"color_hex,omitempty"`
    Price    float64 `json:"price"`
    StockQty int     `json:"stock_qty"`
    IsActive bool    `json:"is_active"`
    ImageID  *string `json:"image_id,omitempty"`
}

type ImageResp struct {
    ID        string  `json:"id"`
    URL       string  `json:"url"`
    AltText   *string `json:"alt_text,omitempty"`
    SortOrder int     `json:"sort_order"`
    IsPrimary bool    `json:"is_primary"`
}

func ToVariantResp(v *Variant) VariantResp {
    var img *string
    if v.ImageID != nil {
        s := v.ImageID.String()
        img = &s
    }
    return VariantResp{
        ID: v.ID.String(), SKU: v.SKU, Size: v.Size, Color: v.Color,
        ColorHex: v.ColorHex, Price: v.Price, StockQty: v.StockQty,
        IsActive: v.IsActive, ImageID: img,
    }
}

func ToImageResp(i *Image) ImageResp {
    return ImageResp{
        ID: i.ID.String(), URL: i.URL, AltText: i.AltText,
        SortOrder: i.SortOrder, IsPrimary: i.IsPrimary,
    }
}

func ToProductSummary(c *CatalogItem) ProductSummary {
    return ProductSummary{
        ID: c.ID.String(), Slug: c.Slug, Name: c.Name,
        BrandSlug: c.BrandSlug, BrandName: c.BrandName,
        Currency: c.Currency,
        MinPrice: c.MinPrice, MaxPrice: c.MaxPrice, InStock: c.InStock,
        PrimaryImage: c.PrimaryImage,
    }
}

// Format helper for time fields.
func FormatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }
```

- [ ] **Step 4: Repo interfaces**

`internal/product/repo/repo.go`:
```go
// Package repo defines persistence interfaces for the product module.
package repo

import (
    "context"
    "errors"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgconn"

    "github.com/wearwhere/wearwhere_be/internal/product/domain"
)

var (
    ErrNotFound       = errors.New("product: not found")
    ErrSlugTaken      = errors.New("product: slug taken")
    ErrVariantConflict = errors.New("product: variant conflict")
)

type DBTX interface {
    Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
    Begin(ctx context.Context) (pgx.Tx, error)
}

type ProductRepo interface {
    Create(ctx context.Context, brandID uuid.UUID, slug string, req *domain.CreateProductRequest) (*domain.Product, error)
    FindByID(ctx context.Context, id uuid.UUID) (*domain.Product, error)
    FindByBrandSlug(ctx context.Context, brandSlug, productSlug string) (*domain.Product, error)
    Update(ctx context.Context, id, brandID uuid.UUID, req *domain.UpdateProductRequest) error
    SoftDelete(ctx context.Context, id, brandID uuid.UUID) error
    ListByBrand(ctx context.Context, brandID uuid.UUID, limit, offset int) ([]*domain.Product, int, error)
    SlugExists(ctx context.Context, brandID uuid.UUID, slug string) (bool, error)
    IncrementViewCount(ctx context.Context, id uuid.UUID) error
    SetStyleTags(ctx context.Context, productID uuid.UUID, tagIDs []uuid.UUID) error
    GetStyleTags(ctx context.Context, productID uuid.UUID) ([]*domain.StyleTag, error)
}

type VariantRepo interface {
    Create(ctx context.Context, productID uuid.UUID, req *domain.CreateVariantRequest) (*domain.Variant, error)
    FindByID(ctx context.Context, id, productID uuid.UUID) (*domain.Variant, error)
    ListByProduct(ctx context.Context, productID uuid.UUID, onlyActive bool) ([]*domain.Variant, error)
    Update(ctx context.Context, id, productID uuid.UUID, req *domain.UpdateVariantRequest) (*domain.Variant, error)
    SoftDelete(ctx context.Context, id, productID uuid.UUID) error
}

type ImageRepo interface {
    Create(ctx context.Context, productID uuid.UUID, url, storageKey string) (*domain.Image, error)
    FindByID(ctx context.Context, id, productID uuid.UUID) (*domain.Image, error)
    ListByProduct(ctx context.Context, productID uuid.UUID) ([]*domain.Image, error)
    Update(ctx context.Context, id, productID uuid.UUID, req *domain.UpdateImageRequest) (*domain.Image, error)
    Delete(ctx context.Context, id, productID uuid.UUID) (storageKey string, wasPrimary bool, err error)
    PromoteNextPrimary(ctx context.Context, productID uuid.UUID) error
}

type CategoryRepo interface {
    List(ctx context.Context) ([]*domain.Category, error)
    FindByID(ctx context.Context, id uuid.UUID) (*domain.Category, error)
    FindBySlug(ctx context.Context, slug string) (*domain.Category, error)
}

type StyleTagRepo interface {
    List(ctx context.Context) ([]*domain.StyleTag, error)
    FindBySlugs(ctx context.Context, slugs []string) ([]*domain.StyleTag, error)
}

type CatalogRepo interface {
    List(ctx context.Context, q *domain.ListProductsQuery) ([]*domain.CatalogItem, int, error)
    Detail(ctx context.Context, brandSlug, productSlug string) (*domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error)
    DetailByID(ctx context.Context, id uuid.UUID) (*domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error)
    Suggestions(ctx context.Context, q string, limit int) ([]string, error)
}
```

- [ ] **Step 5: Build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 6: Commit**

```bash
git add internal/product/domain/ internal/product/repo/repo.go
git commit -m "feat(product): domain types, errors, DTOs, repo interfaces"
```

---

### Task 15: Category & StyleTag PG repos

Read-only lookups. Trivial but needed by other repos.

**Files:**
- Create: `internal/product/repo/category_pg.go`
- Create: `internal/product/repo/style_tag_pg.go`

- [ ] **Step 1: Write category repo**

`internal/product/repo/category_pg.go`:
```go
package repo

import (
    "context"
    "errors"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"

    "github.com/wearwhere/wearwhere_be/internal/product/domain"
)

type CategoryPG struct{ db DBTX }

func NewCategoryPG(db DBTX) *CategoryPG { return &CategoryPG{db: db} }

func (r *CategoryPG) List(ctx context.Context) ([]*domain.Category, error) {
    rows, err := r.db.Query(ctx,
        `SELECT id, slug, name, display_order FROM categories ORDER BY display_order, name`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []*domain.Category
    for rows.Next() {
        var c domain.Category
        if err := rows.Scan(&c.ID, &c.Slug, &c.Name, &c.DisplayOrder); err != nil {
            return nil, err
        }
        out = append(out, &c)
    }
    return out, rows.Err()
}

func (r *CategoryPG) FindByID(ctx context.Context, id uuid.UUID) (*domain.Category, error) {
    var c domain.Category
    err := r.db.QueryRow(ctx,
        `SELECT id, slug, name, display_order FROM categories WHERE id=$1`, id).
        Scan(&c.ID, &c.Slug, &c.Name, &c.DisplayOrder)
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, ErrNotFound
    }
    return &c, err
}

func (r *CategoryPG) FindBySlug(ctx context.Context, slug string) (*domain.Category, error) {
    var c domain.Category
    err := r.db.QueryRow(ctx,
        `SELECT id, slug, name, display_order FROM categories WHERE slug=$1`, slug).
        Scan(&c.ID, &c.Slug, &c.Name, &c.DisplayOrder)
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, ErrNotFound
    }
    return &c, err
}
```

- [ ] **Step 2: Write style tag repo**

`internal/product/repo/style_tag_pg.go`:
```go
package repo

import (
    "context"

    "github.com/wearwhere/wearwhere_be/internal/product/domain"
)

type StyleTagPG struct{ db DBTX }

func NewStyleTagPG(db DBTX) *StyleTagPG { return &StyleTagPG{db: db} }

func (r *StyleTagPG) List(ctx context.Context) ([]*domain.StyleTag, error) {
    rows, err := r.db.Query(ctx,
        `SELECT id, slug, name FROM style_tags ORDER BY name`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []*domain.StyleTag
    for rows.Next() {
        var s domain.StyleTag
        if err := rows.Scan(&s.ID, &s.Slug, &s.Name); err != nil {
            return nil, err
        }
        out = append(out, &s)
    }
    return out, rows.Err()
}

func (r *StyleTagPG) FindBySlugs(ctx context.Context, slugs []string) ([]*domain.StyleTag, error) {
    if len(slugs) == 0 {
        return nil, nil
    }
    rows, err := r.db.Query(ctx,
        `SELECT id, slug, name FROM style_tags WHERE slug = ANY($1)`, slugs)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []*domain.StyleTag
    for rows.Next() {
        var s domain.StyleTag
        if err := rows.Scan(&s.ID, &s.Slug, &s.Name); err != nil {
            return nil, err
        }
        out = append(out, &s)
    }
    return out, rows.Err()
}
```

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/product/repo/category_pg.go internal/product/repo/style_tag_pg.go
git commit -m "feat(product): category and style_tag pg repos"
```

---

### Task 16: Product PG repo + integration tests (slug collision logic)

**Files:**
- Create: `internal/product/repo/product_pg.go`
- Create: `internal/product/repo/product_pg_test.go`

- [ ] **Step 1: Write integration tests focusing on slug collision + IDOR**

`internal/product/repo/product_pg_test.go`:
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

    "github.com/wearwhere/wearwhere_be/internal/product/domain"
    "github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
    url := os.Getenv("TEST_DATABASE_URL")
    if url == "" {
        panic("TEST_DATABASE_URL not set")
    }
    p, err := pgxpool.New(context.Background(), url)
    if err != nil {
        panic(err)
    }
    testPool = p
    code := m.Run()
    p.Close()
    os.Exit(code)
}

func TestProductPG_Create(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})
    sc := testfixtures.SeedCategory(t, tx)

    repo := NewProductPG(tx)
    p, err := repo.Create(context.Background(), sb.ID, "my-slug",
        &domain.CreateProductRequest{
            Name: "My Product", CategoryID: sc.ID.String(),
        })
    require.NoError(t, err)
    require.Equal(t, "my-slug", p.Slug)
    require.Equal(t, domain.ProductStatusDraft, p.Status)
}

func TestProductPG_SlugExists_ScopedByBrand(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb1 := testfixtures.SeedBrand(t, tx, [16]byte{})
    sb2 := testfixtures.SeedBrand(t, tx, [16]byte{})
    sc := testfixtures.SeedCategory(t, tx)
    ctx := context.Background()

    repo := NewProductPG(tx)
    _, err := repo.Create(ctx, sb1.ID, "shared-slug",
        &domain.CreateProductRequest{Name: "P1", CategoryID: sc.ID.String()})
    require.NoError(t, err)

    // Same slug under a different brand is OK
    _, err = repo.Create(ctx, sb2.ID, "shared-slug",
        &domain.CreateProductRequest{Name: "P2", CategoryID: sc.ID.String()})
    require.NoError(t, err)

    // Within same brand, returns ErrSlugTaken
    _, err = repo.Create(ctx, sb1.ID, "shared-slug",
        &domain.CreateProductRequest{Name: "P3", CategoryID: sc.ID.String()})
    require.ErrorIs(t, err, ErrSlugTaken)
}

func TestProductPG_Update_IDORProtected(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sbA := testfixtures.SeedBrand(t, tx, [16]byte{})
    sbB := testfixtures.SeedBrand(t, tx, [16]byte{})
    sc := testfixtures.SeedCategory(t, tx)
    p := testfixtures.SeedProduct(t, tx, sbA.ID, sc.ID, string(domain.ProductStatusDraft))

    repo := NewProductPG(tx)
    newName := "Hacker rename"
    err := repo.Update(context.Background(), p.ID, sbB.ID,
        &domain.UpdateProductRequest{Name: &newName})
    require.ErrorIs(t, err, ErrNotFound)

    // brand A can still update
    require.NoError(t, repo.Update(context.Background(), p.ID, sbA.ID,
        &domain.UpdateProductRequest{Name: &newName}))
}

func TestProductPG_SearchTriggerUpdatesOnBrandRename(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})
    sc := testfixtures.SeedCategory(t, tx)
    _ = testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, string(domain.ProductStatusActive))

    var initial string
    require.NoError(t, tx.QueryRow(context.Background(),
        `SELECT search_text FROM products WHERE brand_id=$1 LIMIT 1`, sb.ID).Scan(&initial))
    require.Contains(t, initial, "brand")

    // Rename the brand
    _, err := tx.Exec(context.Background(),
        `UPDATE brands SET name = $1 WHERE id = $2`, "Hoàn Toàn Mới", sb.ID)
    require.NoError(t, err)

    var updated string
    require.NoError(t, tx.QueryRow(context.Background(),
        `SELECT search_text FROM products WHERE brand_id=$1 LIMIT 1`, sb.ID).Scan(&updated))
    require.Contains(t, updated, "hoan toan moi") // unaccented
}

func TestProductPG_StyleTagsSync(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})
    sc := testfixtures.SeedCategory(t, tx)
    st1 := testfixtures.SeedStyleTag(t, tx)
    st2 := testfixtures.SeedStyleTag(t, tx)
    p := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")

    repo := NewProductPG(tx)
    ctx := context.Background()

    require.NoError(t, repo.SetStyleTags(ctx, p.ID, []uuid.UUID{st1.ID, st2.ID}))
    tags, err := repo.GetStyleTags(ctx, p.ID)
    require.NoError(t, err)
    require.Len(t, tags, 2)

    require.NoError(t, repo.SetStyleTags(ctx, p.ID, []uuid.UUID{st1.ID}))
    tags, err = repo.GetStyleTags(ctx, p.ID)
    require.NoError(t, err)
    require.Len(t, tags, 1)
}
```

- [ ] **Step 2: Run — fails**

Run: `TEST_DATABASE_URL=<url> go test -tags=integration ./internal/product/repo/... -v`
Expected: build error.

- [ ] **Step 3: Write the product repo**

`internal/product/repo/product_pg.go`:
```go
package repo

import (
    "context"
    "errors"
    "strings"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgconn"

    "github.com/wearwhere/wearwhere_be/internal/product/domain"
)

type ProductPG struct{ db DBTX }

func NewProductPG(db DBTX) *ProductPG { return &ProductPG{db: db} }

const productCols = `id, brand_id, category_id, slug, name, description, status,
                     currency, sold_count, view_count, created_at, updated_at, deleted_at`

func scanProduct(row pgx.Row) (*domain.Product, error) {
    var p domain.Product
    var status string
    err := row.Scan(
        &p.ID, &p.BrandID, &p.CategoryID, &p.Slug, &p.Name, &p.Description, &status,
        &p.Currency, &p.SoldCount, &p.ViewCount, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
    )
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, ErrNotFound
        }
        return nil, err
    }
    p.Status = domain.ProductStatus(status)
    return &p, nil
}

func (r *ProductPG) Create(ctx context.Context, brandID uuid.UUID, slug string, req *domain.CreateProductRequest) (*domain.Product, error) {
    catID, err := uuid.Parse(req.CategoryID)
    if err != nil {
        return nil, err
    }
    var desc *string
    if req.Description != "" {
        desc = &req.Description
    }
    row := r.db.QueryRow(ctx,
        `INSERT INTO products (brand_id, category_id, slug, name, description, status)
         VALUES ($1, $2, $3, $4, $5, 'draft')
         RETURNING `+productCols,
        brandID, catID, slug, req.Name, desc)
    p, err := scanProduct(row)
    if err != nil {
        if isUniqueViol(err, "idx_products_brand_slug") {
            return nil, ErrSlugTaken
        }
        return nil, err
    }
    return p, nil
}

func (r *ProductPG) FindByID(ctx context.Context, id uuid.UUID) (*domain.Product, error) {
    return scanProduct(r.db.QueryRow(ctx,
        `SELECT `+productCols+` FROM products WHERE id=$1 AND deleted_at IS NULL`, id))
}

func (r *ProductPG) FindByBrandSlug(ctx context.Context, brandSlug, productSlug string) (*domain.Product, error) {
    return scanProduct(r.db.QueryRow(ctx,
        `SELECT `+productCols+` FROM products p
         JOIN brands b ON b.id = p.brand_id
         WHERE b.slug=$1 AND p.slug=$2 AND p.deleted_at IS NULL AND b.deleted_at IS NULL`,
        brandSlug, productSlug))
}

func (r *ProductPG) Update(ctx context.Context, id, brandID uuid.UUID, req *domain.UpdateProductRequest) error {
    var catID *uuid.UUID
    if req.CategoryID != nil {
        v, err := uuid.Parse(*req.CategoryID)
        if err != nil {
            return err
        }
        catID = &v
    }
    tag, err := r.db.Exec(ctx,
        `UPDATE products SET
           name        = COALESCE($3, name),
           slug        = COALESCE($4, slug),
           description = COALESCE($5, description),
           category_id = COALESCE($6, category_id),
           status      = COALESCE($7::product_status, status),
           updated_at  = NOW()
         WHERE id=$1 AND brand_id=$2 AND deleted_at IS NULL`,
        id, brandID, req.Name, req.Slug, req.Description, catID, req.Status)
    if err != nil {
        if isUniqueViol(err, "idx_products_brand_slug") {
            return ErrSlugTaken
        }
        return err
    }
    if tag.RowsAffected() == 0 {
        return ErrNotFound
    }
    return nil
}

func (r *ProductPG) SoftDelete(ctx context.Context, id, brandID uuid.UUID) error {
    tag, err := r.db.Exec(ctx,
        `UPDATE products SET deleted_at=NOW(), updated_at=NOW()
         WHERE id=$1 AND brand_id=$2 AND deleted_at IS NULL`, id, brandID)
    if err != nil {
        return err
    }
    if tag.RowsAffected() == 0 {
        return ErrNotFound
    }
    return nil
}

func (r *ProductPG) ListByBrand(ctx context.Context, brandID uuid.UUID, limit, offset int) ([]*domain.Product, int, error) {
    rows, err := r.db.Query(ctx,
        `SELECT `+productCols+` FROM products
         WHERE brand_id=$1 AND deleted_at IS NULL
         ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
        brandID, limit, offset)
    if err != nil {
        return nil, 0, err
    }
    defer rows.Close()
    var items []*domain.Product
    for rows.Next() {
        p, err := scanProduct(rows)
        if err != nil {
            return nil, 0, err
        }
        items = append(items, p)
    }
    if err := rows.Err(); err != nil {
        return nil, 0, err
    }
    var total int
    if err := r.db.QueryRow(ctx,
        `SELECT COUNT(*) FROM products WHERE brand_id=$1 AND deleted_at IS NULL`,
        brandID).Scan(&total); err != nil {
        return nil, 0, err
    }
    return items, total, nil
}

func (r *ProductPG) SlugExists(ctx context.Context, brandID uuid.UUID, slug string) (bool, error) {
    var exists bool
    err := r.db.QueryRow(ctx,
        `SELECT EXISTS(
           SELECT 1 FROM products
           WHERE brand_id=$1 AND slug=$2 AND deleted_at IS NULL)`,
        brandID, slug).Scan(&exists)
    return exists, err
}

func (r *ProductPG) IncrementViewCount(ctx context.Context, id uuid.UUID) error {
    _, err := r.db.Exec(ctx,
        `UPDATE products SET view_count = view_count + 1
         WHERE id=$1 AND deleted_at IS NULL`, id)
    return err
}

func (r *ProductPG) SetStyleTags(ctx context.Context, productID uuid.UUID, tagIDs []uuid.UUID) error {
    if _, err := r.db.Exec(ctx,
        `DELETE FROM product_style_tags WHERE product_id=$1`, productID); err != nil {
        return err
    }
    for _, tid := range tagIDs {
        if _, err := r.db.Exec(ctx,
            `INSERT INTO product_style_tags (product_id, style_tag_id) VALUES ($1, $2)
             ON CONFLICT DO NOTHING`, productID, tid); err != nil {
            return err
        }
    }
    return nil
}

func (r *ProductPG) GetStyleTags(ctx context.Context, productID uuid.UUID) ([]*domain.StyleTag, error) {
    rows, err := r.db.Query(ctx,
        `SELECT s.id, s.slug, s.name
           FROM style_tags s
           JOIN product_style_tags pst ON pst.style_tag_id = s.id
          WHERE pst.product_id = $1
          ORDER BY s.name`, productID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []*domain.StyleTag
    for rows.Next() {
        var s domain.StyleTag
        if err := rows.Scan(&s.ID, &s.Slug, &s.Name); err != nil {
            return nil, err
        }
        out = append(out, &s)
    }
    return out, rows.Err()
}

func isUniqueViol(err error, indexName string) bool {
    var pgErr *pgconn.PgError
    if !errors.As(err, &pgErr) {
        return false
    }
    if pgErr.Code != "23505" {
        return false
    }
    if indexName == "" {
        return true
    }
    return strings.Contains(pgErr.ConstraintName, indexName) ||
        strings.Contains(pgErr.Message, indexName)
}
```

- [ ] **Step 4: Run tests**

Run: `TEST_DATABASE_URL=<url> go test -tags=integration ./internal/product/repo/... -v -run TestProductPG`
Expected: 5 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/product/repo/product_pg.go internal/product/repo/product_pg_test.go
git commit -m "feat(product): product pg repo with slug collision and IDOR protection"
```

---

### Task 17: Variant PG repo + integration tests

**Files:**
- Create: `internal/product/repo/variant_pg.go`
- Create: `internal/product/repo/variant_pg_test.go`

- [ ] **Step 1: Tests focus on uniqueness + IDOR**

`internal/product/repo/variant_pg_test.go`:
```go
//go:build integration

package repo

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"

    "github.com/wearwhere/wearwhere_be/internal/product/domain"
    "github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

func TestVariantPG_Create(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})
    sc := testfixtures.SeedCategory(t, tx)
    p := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")

    repo := NewVariantPG(tx)
    v, err := repo.Create(context.Background(), p.ID, &domain.CreateVariantRequest{
        SKU: "SKU-A", Size: "M", Color: "White", Price: 250000, StockQty: 5,
    })
    require.NoError(t, err)
    require.Equal(t, p.ID, v.ProductID)
    require.True(t, v.IsActive)
}

func TestVariantPG_DuplicateSizeColor_Conflict(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})
    sc := testfixtures.SeedCategory(t, tx)
    p := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")

    repo := NewVariantPG(tx)
    ctx := context.Background()
    _, err := repo.Create(ctx, p.ID, &domain.CreateVariantRequest{
        SKU: "SKU-A", Size: "M", Color: "White", Price: 100, StockQty: 0,
    })
    require.NoError(t, err)
    _, err = repo.Create(ctx, p.ID, &domain.CreateVariantRequest{
        SKU: "SKU-B", Size: "M", Color: "White", Price: 200, StockQty: 0,
    })
    require.ErrorIs(t, err, ErrVariantConflict)
}

func TestVariantPG_IDORProtected(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})
    sc := testfixtures.SeedCategory(t, tx)
    pA := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")
    pB := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")
    vID := testfixtures.SeedVariant(t, tx, pA.ID, "S", "Red", 100, 1)

    repo := NewVariantPG(tx)
    _, err := repo.FindByID(context.Background(), vID, pB.ID)
    require.ErrorIs(t, err, ErrNotFound)
}

func TestVariantPG_Update(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})
    sc := testfixtures.SeedCategory(t, tx)
    p := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")
    vID := testfixtures.SeedVariant(t, tx, p.ID, "M", "White", 100, 5)

    repo := NewVariantPG(tx)
    newPrice := 999.0
    newStock := 50
    v, err := repo.Update(context.Background(), vID, p.ID, &domain.UpdateVariantRequest{
        Price: &newPrice, StockQty: &newStock,
    })
    require.NoError(t, err)
    require.Equal(t, 999.0, v.Price)
    require.Equal(t, 50, v.StockQty)
}
```

- [ ] **Step 2: Run — fails**

Run: `TEST_DATABASE_URL=<url> go test -tags=integration ./internal/product/repo/... -v -run TestVariantPG`
Expected: build error.

- [ ] **Step 3: Write the repo**

`internal/product/repo/variant_pg.go`:
```go
package repo

import (
    "context"
    "errors"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"

    "github.com/wearwhere/wearwhere_be/internal/product/domain"
)

type VariantPG struct{ db DBTX }

func NewVariantPG(db DBTX) *VariantPG { return &VariantPG{db: db} }

const variantCols = `id, product_id, sku, size, color, color_hex, price, stock_qty,
                     is_active, image_id, created_at, updated_at, deleted_at`

func scanVariant(row pgx.Row) (*domain.Variant, error) {
    var v domain.Variant
    err := row.Scan(
        &v.ID, &v.ProductID, &v.SKU, &v.Size, &v.Color, &v.ColorHex,
        &v.Price, &v.StockQty, &v.IsActive, &v.ImageID,
        &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
    )
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, ErrNotFound
        }
        return nil, err
    }
    return &v, nil
}

func (r *VariantPG) Create(ctx context.Context, productID uuid.UUID, req *domain.CreateVariantRequest) (*domain.Variant, error) {
    var imageID *uuid.UUID
    if req.ImageID != "" {
        v, err := uuid.Parse(req.ImageID)
        if err != nil {
            return nil, err
        }
        imageID = &v
    }
    var hex *string
    if req.ColorHex != "" {
        hex = &req.ColorHex
    }
    row := r.db.QueryRow(ctx,
        `INSERT INTO product_variants
         (product_id, sku, size, color, color_hex, price, stock_qty, image_id)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
         RETURNING `+variantCols,
        productID, req.SKU, req.Size, req.Color, hex, req.Price, req.StockQty, imageID)
    v, err := scanVariant(row)
    if err != nil {
        if isUniqueViol(err, "idx_product_variants_size_color") {
            return nil, ErrVariantConflict
        }
        if isUniqueViol(err, "idx_product_variants_sku") {
            return nil, ErrVariantConflict
        }
        return nil, err
    }
    return v, nil
}

func (r *VariantPG) FindByID(ctx context.Context, id, productID uuid.UUID) (*domain.Variant, error) {
    return scanVariant(r.db.QueryRow(ctx,
        `SELECT `+variantCols+` FROM product_variants
         WHERE id=$1 AND product_id=$2 AND deleted_at IS NULL`, id, productID))
}

func (r *VariantPG) ListByProduct(ctx context.Context, productID uuid.UUID, onlyActive bool) ([]*domain.Variant, error) {
    q := `SELECT ` + variantCols + ` FROM product_variants
          WHERE product_id=$1 AND deleted_at IS NULL`
    if onlyActive {
        q += ` AND is_active = TRUE`
    }
    q += ` ORDER BY created_at ASC`
    rows, err := r.db.Query(ctx, q, productID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []*domain.Variant
    for rows.Next() {
        v, err := scanVariant(rows)
        if err != nil {
            return nil, err
        }
        out = append(out, v)
    }
    return out, rows.Err()
}

func (r *VariantPG) Update(ctx context.Context, id, productID uuid.UUID, req *domain.UpdateVariantRequest) (*domain.Variant, error) {
    var imageID *uuid.UUID
    if req.ImageID != nil && *req.ImageID != "" {
        v, err := uuid.Parse(*req.ImageID)
        if err != nil {
            return nil, err
        }
        imageID = &v
    }
    row := r.db.QueryRow(ctx,
        `UPDATE product_variants SET
           size       = COALESCE($3, size),
           color      = COALESCE($4, color),
           color_hex  = COALESCE($5, color_hex),
           price      = COALESCE($6, price),
           stock_qty  = COALESCE($7, stock_qty),
           is_active  = COALESCE($8, is_active),
           image_id   = COALESCE($9, image_id),
           updated_at = NOW()
         WHERE id=$1 AND product_id=$2 AND deleted_at IS NULL
         RETURNING `+variantCols,
        id, productID, req.Size, req.Color, req.ColorHex,
        req.Price, req.StockQty, req.IsActive, imageID)
    v, err := scanVariant(row)
    if err != nil {
        if isUniqueViol(err, "idx_product_variants_size_color") {
            return nil, ErrVariantConflict
        }
        return nil, err
    }
    return v, nil
}

func (r *VariantPG) SoftDelete(ctx context.Context, id, productID uuid.UUID) error {
    tag, err := r.db.Exec(ctx,
        `UPDATE product_variants SET deleted_at=NOW(), updated_at=NOW()
         WHERE id=$1 AND product_id=$2 AND deleted_at IS NULL`, id, productID)
    if err != nil {
        return err
    }
    if tag.RowsAffected() == 0 {
        return ErrNotFound
    }
    return nil
}
```

- [ ] **Step 4: Run tests**

Expected: 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/product/repo/variant_pg.go internal/product/repo/variant_pg_test.go
git commit -m "feat(product): variant pg repo with uniqueness and IDOR enforcement"
```

---

### Task 18: Image PG repo

**Files:**
- Create: `internal/product/repo/image_pg.go`

(No dedicated integration test in this task; image flow is exercised via service tests and E2E. The DB-level behavior here is mostly straightforward CRUD; the interesting `is_primary` promotion is tested via the service layer.)

- [ ] **Step 1: Write repo**

`internal/product/repo/image_pg.go`:
```go
package repo

import (
    "context"
    "errors"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"

    "github.com/wearwhere/wearwhere_be/internal/product/domain"
)

type ImagePG struct{ db DBTX }

func NewImagePG(db DBTX) *ImagePG { return &ImagePG{db: db} }

const imageCols = `id, product_id, url, storage_key, alt_text,
                   sort_order, is_primary, created_at`

func scanImage(row pgx.Row) (*domain.Image, error) {
    var i domain.Image
    err := row.Scan(
        &i.ID, &i.ProductID, &i.URL, &i.StorageKey, &i.AltText,
        &i.SortOrder, &i.IsPrimary, &i.CreatedAt,
    )
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, ErrNotFound
        }
        return nil, err
    }
    return &i, nil
}

func (r *ImagePG) Create(ctx context.Context, productID uuid.UUID, url, storageKey string) (*domain.Image, error) {
    // Compute next sort_order and decide is_primary in one trip.
    var nextOrder int
    var hasAny bool
    if err := r.db.QueryRow(ctx,
        `SELECT COALESCE(MAX(sort_order), -1) + 1, COUNT(*) > 0
           FROM product_images WHERE product_id=$1`, productID).
        Scan(&nextOrder, &hasAny); err != nil {
        return nil, err
    }
    isPrimary := !hasAny

    row := r.db.QueryRow(ctx,
        `INSERT INTO product_images
           (product_id, url, storage_key, sort_order, is_primary)
         VALUES ($1, $2, $3, $4, $5)
         RETURNING `+imageCols,
        productID, url, storageKey, nextOrder, isPrimary)
    return scanImage(row)
}

func (r *ImagePG) FindByID(ctx context.Context, id, productID uuid.UUID) (*domain.Image, error) {
    return scanImage(r.db.QueryRow(ctx,
        `SELECT `+imageCols+` FROM product_images
         WHERE id=$1 AND product_id=$2`, id, productID))
}

func (r *ImagePG) ListByProduct(ctx context.Context, productID uuid.UUID) ([]*domain.Image, error) {
    rows, err := r.db.Query(ctx,
        `SELECT `+imageCols+` FROM product_images
         WHERE product_id=$1 ORDER BY sort_order ASC`, productID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []*domain.Image
    for rows.Next() {
        i, err := scanImage(rows)
        if err != nil {
            return nil, err
        }
        out = append(out, i)
    }
    return out, rows.Err()
}

func (r *ImagePG) Update(ctx context.Context, id, productID uuid.UUID, req *domain.UpdateImageRequest) (*domain.Image, error) {
    // If becoming primary, first demote sibling.
    if req.IsPrimary != nil && *req.IsPrimary {
        if _, err := r.db.Exec(ctx,
            `UPDATE product_images SET is_primary = FALSE
             WHERE product_id=$1 AND id <> $2 AND is_primary`,
            productID, id); err != nil {
            return nil, err
        }
    }
    row := r.db.QueryRow(ctx,
        `UPDATE product_images SET
           sort_order = COALESCE($3, sort_order),
           alt_text   = COALESCE($4, alt_text),
           is_primary = COALESCE($5, is_primary)
         WHERE id=$1 AND product_id=$2
         RETURNING `+imageCols,
        id, productID, req.SortOrder, req.AltText, req.IsPrimary)
    return scanImage(row)
}

func (r *ImagePG) Delete(ctx context.Context, id, productID uuid.UUID) (string, bool, error) {
    var storageKey string
    var wasPrimary bool
    err := r.db.QueryRow(ctx,
        `DELETE FROM product_images WHERE id=$1 AND product_id=$2
         RETURNING storage_key, is_primary`, id, productID).
        Scan(&storageKey, &wasPrimary)
    if errors.Is(err, pgx.ErrNoRows) {
        return "", false, ErrNotFound
    }
    return storageKey, wasPrimary, err
}

func (r *ImagePG) PromoteNextPrimary(ctx context.Context, productID uuid.UUID) error {
    _, err := r.db.Exec(ctx,
        `UPDATE product_images SET is_primary = TRUE
         WHERE id = (
           SELECT id FROM product_images
            WHERE product_id=$1 ORDER BY sort_order ASC LIMIT 1
         )`, productID)
    return err
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/product/repo/image_pg.go
git commit -m "feat(product): image pg repo with auto primary management"
```

---

### Task 19: Product write service + brand product handlers + main wiring

This task bundles the brand-side product service, image upload service, handlers, routes, and wiring. Steps grouped by concern; one commit at the end.

**Files:**
- Create: `internal/product/service/product_service.go`
- Create: `internal/product/service/product_service_test.go`
- Create: `internal/product/handler/brand_product_handler.go`
- Create: `internal/product/handler/routes.go`
- Modify: `cmd/api/main.go`

- [ ] **Step 1: Write unit test for slug generation + publish gate**

`internal/product/service/product_service_test.go`:
```go
package service

import (
    "context"
    "testing"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    "github.com/wearwhere/wearwhere_be/internal/product/domain"
    "github.com/wearwhere/wearwhere_be/internal/product/repo"
)

// fakeProductRepo: just enough for slug tests
type fakeProductRepo struct {
    existingSlugs map[string]bool
    createCalled  bool
    createdSlug   string
}

func (f *fakeProductRepo) Create(ctx context.Context, brandID uuid.UUID, slug string, req *domain.CreateProductRequest) (*domain.Product, error) {
    f.createCalled = true
    f.createdSlug = slug
    return &domain.Product{ID: uuid.New(), BrandID: brandID, Slug: slug, Name: req.Name, Status: domain.ProductStatusDraft}, nil
}
func (f *fakeProductRepo) SlugExists(ctx context.Context, brandID uuid.UUID, slug string) (bool, error) {
    return f.existingSlugs[slug], nil
}
func (f *fakeProductRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Product, error) { return nil, repo.ErrNotFound }
func (f *fakeProductRepo) FindByBrandSlug(ctx context.Context, bs, ps string) (*domain.Product, error) { return nil, repo.ErrNotFound }
func (f *fakeProductRepo) Update(ctx context.Context, id, brandID uuid.UUID, r *domain.UpdateProductRequest) error { return nil }
func (f *fakeProductRepo) SoftDelete(ctx context.Context, id, brandID uuid.UUID) error { return nil }
func (f *fakeProductRepo) ListByBrand(ctx context.Context, brandID uuid.UUID, l, o int) ([]*domain.Product, int, error) { return nil, 0, nil }
func (f *fakeProductRepo) IncrementViewCount(ctx context.Context, id uuid.UUID) error { return nil }
func (f *fakeProductRepo) SetStyleTags(ctx context.Context, p uuid.UUID, ids []uuid.UUID) error { return nil }
func (f *fakeProductRepo) GetStyleTags(ctx context.Context, p uuid.UUID) ([]*domain.StyleTag, error) { return nil, nil }

func TestService_SlugFromName_AppendsSuffixOnCollision(t *testing.T) {
    fr := &fakeProductRepo{existingSlugs: map[string]bool{"ao-thun-trang": true, "ao-thun-trang-2": true}}
    svc := New(fr, nil, nil, nil, nil, nil, nil, nil)
    bid := uuid.New()
    cid := uuid.New().String()
    _, err := svc.CreateProduct(context.Background(), bid, &domain.CreateProductRequest{
        Name: "Áo Thun Trắng", CategoryID: cid,
    })
    require.NoError(t, err)
    require.True(t, fr.createCalled)
    require.Equal(t, "ao-thun-trang-3", fr.createdSlug)
}

func TestService_ExplicitSlug_RejectsConflict(t *testing.T) {
    fr := &fakeProductRepo{existingSlugs: map[string]bool{"taken": true}}
    svc := New(fr, nil, nil, nil, nil, nil, nil, nil)
    _, err := svc.CreateProduct(context.Background(), uuid.New(), &domain.CreateProductRequest{
        Name: "x", Slug: "taken", CategoryID: uuid.New().String(),
    })
    require.ErrorIs(t, err, domain.ErrSlugTaken)
}
```

- [ ] **Step 2: Write the product write service**

`internal/product/service/product_service.go`:
```go
package service

import (
    "bytes"
    "context"
    "errors"
    "fmt"
    "io"
    "mime/multipart"
    "net/http"

    "github.com/google/uuid"

    "github.com/wearwhere/wearwhere_be/internal/product/domain"
    "github.com/wearwhere/wearwhere_be/internal/product/repo"
    "github.com/wearwhere/wearwhere_be/internal/shared/slug"
    "github.com/wearwhere/wearwhere_be/internal/shared/storage"
)

type Service struct {
    products    repo.ProductRepo
    variants    repo.VariantRepo
    images      repo.ImageRepo
    categories  repo.CategoryRepo
    styleTags   repo.StyleTagRepo
    storage     storage.Storage
    maxFileSize int64
    allowedMIMEs map[string]string // mime -> extension
}

func New(
    p repo.ProductRepo, v repo.VariantRepo, i repo.ImageRepo,
    c repo.CategoryRepo, st repo.StyleTagRepo,
    s storage.Storage, allowedMIMEs []string, maxFileSize int64,
) *Service {
    allowed := map[string]string{}
    extMap := map[string]string{
        "image/jpeg": "jpg",
        "image/png":  "png",
        "image/webp": "webp",
    }
    for _, m := range allowedMIMEs {
        if ext, ok := extMap[m]; ok {
            allowed[m] = ext
        }
    }
    if maxFileSize == 0 {
        maxFileSize = 5 * 1024 * 1024
    }
    return &Service{
        products: p, variants: v, images: i,
        categories: c, styleTags: st,
        storage: s, maxFileSize: maxFileSize, allowedMIMEs: allowed,
    }
}

// ── PRODUCT CRUD ──
func (s *Service) CreateProduct(ctx context.Context, brandID uuid.UUID, req *domain.CreateProductRequest) (*domain.Product, error) {
    var theSlug string
    if req.Slug != "" {
        exists, err := s.products.SlugExists(ctx, brandID, req.Slug)
        if err != nil {
            return nil, err
        }
        if exists {
            return nil, domain.ErrSlugTaken
        }
        theSlug = req.Slug
    } else {
        base := slug.Slugify(req.Name)
        if base == "" {
            base = "product"
        }
        theSlug = base
        for i := 2; i < 100; i++ {
            exists, err := s.products.SlugExists(ctx, brandID, theSlug)
            if err != nil {
                return nil, err
            }
            if !exists {
                break
            }
            theSlug = fmt.Sprintf("%s-%d", base, i)
        }
    }

    p, err := s.products.Create(ctx, brandID, theSlug, req)
    if err != nil {
        if errors.Is(err, repo.ErrSlugTaken) {
            return nil, domain.ErrSlugTaken
        }
        return nil, err
    }
    if len(req.StyleTagIDs) > 0 {
        ids, err := parseUUIDs(req.StyleTagIDs)
        if err != nil {
            return nil, err
        }
        if err := s.products.SetStyleTags(ctx, p.ID, ids); err != nil {
            return nil, err
        }
    }
    return p, nil
}

func (s *Service) GetOwnProduct(ctx context.Context, id, brandID uuid.UUID) (*domain.Product, error) {
    p, err := s.products.FindByID(ctx, id)
    if errors.Is(err, repo.ErrNotFound) {
        return nil, domain.ErrProductNotFound
    }
    if err != nil {
        return nil, err
    }
    if p.BrandID != brandID {
        return nil, domain.ErrProductNotFound
    }
    return p, nil
}

func (s *Service) ListOwnProducts(ctx context.Context, brandID uuid.UUID, limit, offset int) ([]*domain.Product, int, error) {
    return s.products.ListByBrand(ctx, brandID, limit, offset)
}

func (s *Service) UpdateProduct(ctx context.Context, id, brandID uuid.UUID, req *domain.UpdateProductRequest) error {
    // If publishing draft→active, require ≥1 variant and ≥1 image.
    if req.Status != nil && *req.Status == string(domain.ProductStatusActive) {
        variants, err := s.variants.ListByProduct(ctx, id, true)
        if err != nil {
            return err
        }
        images, err := s.images.ListByProduct(ctx, id)
        if err != nil {
            return err
        }
        if len(variants) == 0 || len(images) == 0 {
            return domain.ErrProductNotPublishable
        }
    }
    if req.Slug != nil {
        exists, err := s.products.SlugExists(ctx, brandID, *req.Slug)
        if err != nil {
            return err
        }
        if exists {
            // Allow same slug on this product itself
            p, err := s.products.FindByID(ctx, id)
            if err != nil || p.Slug != *req.Slug {
                return domain.ErrSlugTaken
            }
        }
    }
    err := s.products.Update(ctx, id, brandID, req)
    switch {
    case errors.Is(err, repo.ErrNotFound):
        return domain.ErrProductNotFound
    case errors.Is(err, repo.ErrSlugTaken):
        return domain.ErrSlugTaken
    }
    if err == nil && req.StyleTagIDs != nil {
        ids, perr := parseUUIDs(req.StyleTagIDs)
        if perr != nil {
            return perr
        }
        if perr := s.products.SetStyleTags(ctx, id, ids); perr != nil {
            return perr
        }
    }
    return err
}

func (s *Service) DeleteProduct(ctx context.Context, id, brandID uuid.UUID) error {
    err := s.products.SoftDelete(ctx, id, brandID)
    if errors.Is(err, repo.ErrNotFound) {
        return domain.ErrProductNotFound
    }
    return err
}

// ── VARIANT CRUD ──
func (s *Service) verifyProductOwned(ctx context.Context, id, brandID uuid.UUID) error {
    p, err := s.products.FindByID(ctx, id)
    if errors.Is(err, repo.ErrNotFound) || (p != nil && p.BrandID != brandID) {
        return domain.ErrProductNotFound
    }
    return err
}

func (s *Service) CreateVariant(ctx context.Context, productID, brandID uuid.UUID, req *domain.CreateVariantRequest) (*domain.Variant, error) {
    if err := s.verifyProductOwned(ctx, productID, brandID); err != nil {
        return nil, err
    }
    v, err := s.variants.Create(ctx, productID, req)
    if errors.Is(err, repo.ErrVariantConflict) {
        return nil, domain.ErrVariantConflict
    }
    return v, err
}

func (s *Service) UpdateVariant(ctx context.Context, id, productID, brandID uuid.UUID, req *domain.UpdateVariantRequest) (*domain.Variant, error) {
    if err := s.verifyProductOwned(ctx, productID, brandID); err != nil {
        return nil, err
    }
    v, err := s.variants.Update(ctx, id, productID, req)
    switch {
    case errors.Is(err, repo.ErrNotFound):
        return nil, domain.ErrVariantNotFound
    case errors.Is(err, repo.ErrVariantConflict):
        return nil, domain.ErrVariantConflict
    }
    return v, err
}

func (s *Service) DeleteVariant(ctx context.Context, id, productID, brandID uuid.UUID) error {
    if err := s.verifyProductOwned(ctx, productID, brandID); err != nil {
        return err
    }
    err := s.variants.SoftDelete(ctx, id, productID)
    if errors.Is(err, repo.ErrNotFound) {
        return domain.ErrVariantNotFound
    }
    return err
}

// ── IMAGE UPLOAD ──
func (s *Service) UploadImages(ctx context.Context, productID, brandID uuid.UUID, files []*multipart.FileHeader) ([]*domain.Image, error) {
    if err := s.verifyProductOwned(ctx, productID, brandID); err != nil {
        return nil, err
    }
    if len(files) > 10 {
        return nil, domain.ErrTooManyFiles
    }

    var created []*domain.Image
    var keysWritten []string

    rollback := func() {
        for _, k := range keysWritten {
            _ = s.storage.Delete(ctx, k)
        }
    }

    for _, fh := range files {
        if fh.Size > s.maxFileSize {
            rollback()
            return nil, domain.ErrFileTooLarge
        }
        f, err := fh.Open()
        if err != nil {
            rollback()
            return nil, domain.ErrStorageError
        }

        // Sniff first 512 bytes
        sniff := make([]byte, 512)
        n, _ := io.ReadFull(f, sniff)
        sniff = sniff[:n]
        mime := http.DetectContentType(sniff)
        ext, allowed := s.allowedMIMEs[mime]
        if !allowed {
            f.Close()
            rollback()
            return nil, domain.ErrInvalidMIME
        }

        // Reassemble reader: prepend sniffed bytes to remainder.
        body := io.MultiReader(bytes.NewReader(sniff), f)
        key := fmt.Sprintf("products/%s/%s.%s", productID.String(), uuid.New().String(), ext)
        url, err := s.storage.Put(ctx, storage.Object{Key: key, ContentType: mime, Size: fh.Size}, body)
        f.Close()
        if err != nil {
            rollback()
            return nil, domain.ErrStorageError
        }
        keysWritten = append(keysWritten, key)

        img, err := s.images.Create(ctx, productID, url, key)
        if err != nil {
            rollback()
            return nil, err
        }
        created = append(created, img)
    }
    return created, nil
}

func (s *Service) UpdateImage(ctx context.Context, id, productID, brandID uuid.UUID, req *domain.UpdateImageRequest) (*domain.Image, error) {
    if err := s.verifyProductOwned(ctx, productID, brandID); err != nil {
        return nil, err
    }
    img, err := s.images.Update(ctx, id, productID, req)
    if errors.Is(err, repo.ErrNotFound) {
        return nil, domain.ErrImageNotFound
    }
    return img, err
}

func (s *Service) DeleteImage(ctx context.Context, id, productID, brandID uuid.UUID) error {
    if err := s.verifyProductOwned(ctx, productID, brandID); err != nil {
        return err
    }
    storageKey, wasPrimary, err := s.images.Delete(ctx, id, productID)
    if errors.Is(err, repo.ErrNotFound) {
        return domain.ErrImageNotFound
    }
    if err != nil {
        return err
    }
    if wasPrimary {
        if err := s.images.PromoteNextPrimary(ctx, productID); err != nil {
            return err
        }
    }
    if err := s.storage.Delete(ctx, storageKey); err != nil {
        // log only; DB row already gone
    }
    return nil
}

// ── helpers ──
func parseUUIDs(in []string) ([]uuid.UUID, error) {
    out := make([]uuid.UUID, 0, len(in))
    for _, s := range in {
        u, err := uuid.Parse(s)
        if err != nil {
            return nil, err
        }
        out = append(out, u)
    }
    return out, nil
}
```

- [ ] **Step 3: Run service tests**

Run: `go test ./internal/product/service/... -v`
Expected: 2 tests PASS.

- [ ] **Step 4: Write brand product handler**

`internal/product/handler/brand_product_handler.go`:
```go
package handler

import (
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    brandmw "github.com/wearwhere/wearwhere_be/internal/brand/middleware"
    "github.com/wearwhere/wearwhere_be/internal/product/domain"
    "github.com/wearwhere/wearwhere_be/internal/product/service"
    "github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type BrandProductHandler struct{ svc *service.Service }

func NewBrandProductHandler(svc *service.Service) *BrandProductHandler {
    return &BrandProductHandler{svc: svc}
}

func parseIDParam(c *gin.Context, key string) (uuid.UUID, bool) {
    id, err := uuid.Parse(c.Param(key))
    if err != nil {
        httpx.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid "+key)
        return uuid.Nil, false
    }
    return id, true
}

func (h *BrandProductHandler) Create(c *gin.Context) {
    bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
    var req domain.CreateProductRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
        return
    }
    p, err := h.svc.CreateProduct(c.Request.Context(), bid, &req)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    httpx.Created(c, gin.H{"product": gin.H{
        "id": p.ID.String(), "slug": p.Slug, "name": p.Name, "status": string(p.Status),
    }})
}

func (h *BrandProductHandler) List(c *gin.Context) {
    bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
    page := paginInt(c, "page", 1, 1, 1_000_000)
    limit := paginInt(c, "limit", 24, 1, 60)
    items, total, err := h.svc.ListOwnProducts(c.Request.Context(), bid, limit, (page-1)*limit)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    out := make([]gin.H, 0, len(items))
    for _, p := range items {
        out = append(out, gin.H{
            "id": p.ID.String(), "slug": p.Slug, "name": p.Name,
            "status": string(p.Status), "currency": p.Currency,
        })
    }
    httpx.OK(c, gin.H{"items": out, "pagination": paginationEnvelope(page, limit, total)})
}

func (h *BrandProductHandler) Get(c *gin.Context) {
    bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
    id, ok := parseIDParam(c, "id")
    if !ok {
        return
    }
    p, err := h.svc.GetOwnProduct(c.Request.Context(), id, bid)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    httpx.OK(c, gin.H{"product": gin.H{
        "id": p.ID.String(), "slug": p.Slug, "name": p.Name,
        "description": p.Description, "status": string(p.Status),
        "currency": p.Currency,
    }})
}

func (h *BrandProductHandler) Update(c *gin.Context) {
    bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
    id, ok := parseIDParam(c, "id")
    if !ok {
        return
    }
    var req domain.UpdateProductRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
        return
    }
    if err := h.svc.UpdateProduct(c.Request.Context(), id, bid, &req); err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    httpx.NoContent(c)
}

func (h *BrandProductHandler) Delete(c *gin.Context) {
    bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
    id, ok := parseIDParam(c, "id")
    if !ok {
        return
    }
    if err := h.svc.DeleteProduct(c.Request.Context(), id, bid); err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    httpx.NoContent(c)
}

// Variants
func (h *BrandProductHandler) CreateVariant(c *gin.Context) {
    bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
    pid, ok := parseIDParam(c, "id")
    if !ok {
        return
    }
    var req domain.CreateVariantRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
        return
    }
    v, err := h.svc.CreateVariant(c.Request.Context(), pid, bid, &req)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    httpx.Created(c, gin.H{"variant": domain.ToVariantResp(v)})
}

func (h *BrandProductHandler) UpdateVariant(c *gin.Context) {
    bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
    pid, ok := parseIDParam(c, "id")
    if !ok {
        return
    }
    vid, ok := parseIDParam(c, "variant_id")
    if !ok {
        return
    }
    var req domain.UpdateVariantRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
        return
    }
    v, err := h.svc.UpdateVariant(c.Request.Context(), vid, pid, bid, &req)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    httpx.OK(c, gin.H{"variant": domain.ToVariantResp(v)})
}

func (h *BrandProductHandler) DeleteVariant(c *gin.Context) {
    bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
    pid, ok := parseIDParam(c, "id")
    if !ok {
        return
    }
    vid, ok := parseIDParam(c, "variant_id")
    if !ok {
        return
    }
    if err := h.svc.DeleteVariant(c.Request.Context(), vid, pid, bid); err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    httpx.NoContent(c)
}

// Images
func (h *BrandProductHandler) UploadImages(c *gin.Context) {
    bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
    pid, ok := parseIDParam(c, "id")
    if !ok {
        return
    }
    form, err := c.MultipartForm()
    if err != nil {
        httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
        return
    }
    files := form.File["files"]
    if len(files) == 0 {
        files = form.File["files[]"]
    }
    if len(files) == 0 {
        httpx.Error(c, http.StatusBadRequest, "NO_FILES", "No files in request")
        return
    }
    imgs, err := h.svc.UploadImages(c.Request.Context(), pid, bid, files)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    out := make([]domain.ImageResp, 0, len(imgs))
    for _, i := range imgs {
        out = append(out, domain.ToImageResp(i))
    }
    httpx.Created(c, gin.H{"images": out})
}

func (h *BrandProductHandler) UpdateImage(c *gin.Context) {
    bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
    pid, ok := parseIDParam(c, "id")
    if !ok {
        return
    }
    iid, ok := parseIDParam(c, "image_id")
    if !ok {
        return
    }
    var req domain.UpdateImageRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        httpx.Error(c, http.StatusBadRequest, "INVALID_BODY", err.Error())
        return
    }
    img, err := h.svc.UpdateImage(c.Request.Context(), iid, pid, bid, &req)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    httpx.OK(c, gin.H{"image": domain.ToImageResp(img)})
}

func (h *BrandProductHandler) DeleteImage(c *gin.Context) {
    bid := c.MustGet(brandmw.CtxBrandID).(uuid.UUID)
    pid, ok := parseIDParam(c, "id")
    if !ok {
        return
    }
    iid, ok := parseIDParam(c, "image_id")
    if !ok {
        return
    }
    if err := h.svc.DeleteImage(c.Request.Context(), iid, pid, bid); err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    httpx.NoContent(c)
}

// Pagination helpers shared with catalog handler.
func paginInt(c *gin.Context, key string, def, min, max int) int {
    v := c.Query(key)
    if v == "" {
        return def
    }
    var n int
    _, err := fmt.Sscanf(v, "%d", &n)
    if err != nil || n < min {
        return def
    }
    if n > max {
        return max
    }
    return n
}

func paginationEnvelope(page, limit, total int) gin.H {
    totalPages := (total + limit - 1) / limit
    return gin.H{
        "page": page, "limit": limit, "total": total,
        "total_pages": totalPages, "has_more": page < totalPages,
    }
}
```

Add the `fmt` import to the handler file imports.

- [ ] **Step 5: Write routes**

`internal/product/handler/routes.go`:
```go
package handler

import "github.com/gin-gonic/gin"

// MountBrandProducts mounts /brand/me/products under the given group.
// Caller is responsible for applying RequireAuth + RequireRole + BrandContext.
func MountBrandProducts(rg *gin.RouterGroup, h *BrandProductHandler) {
    p := rg.Group("/products")
    {
        p.GET("",        h.List)
        p.POST("",       h.Create)
        p.GET(":id",     h.Get)
        p.PATCH(":id",   h.Update)
        p.DELETE(":id",  h.Delete)

        p.POST(":id/variants",            h.CreateVariant)
        p.PATCH(":id/variants/:variant_id", h.UpdateVariant)
        p.DELETE(":id/variants/:variant_id", h.DeleteVariant)

        p.POST(":id/images",              h.UploadImages)
        p.PATCH(":id/images/:image_id",   h.UpdateImage)
        p.DELETE(":id/images/:image_id",  h.DeleteImage)
    }
}
```

- [ ] **Step 6: Wire main.go**

Add imports:
```go
    productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
    productservice "github.com/wearwhere/wearwhere_be/internal/product/service"
    producthandler "github.com/wearwhere/wearwhere_be/internal/product/handler"
    "github.com/wearwhere/wearwhere_be/internal/shared/storage"
```

After existing repo init, add:
```go
    productRepo := productrepo.NewProductPG(pgPool)
    variantRepo := productrepo.NewVariantPG(pgPool)
    imageRepo := productrepo.NewImagePG(pgPool)
    categoryRepo := productrepo.NewCategoryPG(pgPool)
    styleTagRepo := productrepo.NewStyleTagPG(pgPool)
```

Initialize storage backend (after redis init):
```go
    storageBackend, err := storage.New(storage.Config{
        Driver:         cfg.Storage.Driver,
        LocalDir:       cfg.Storage.LocalDir,
        BaseURL:        cfg.Storage.BaseURL,
        GCSBucket:      cfg.Storage.GCSBucket,
        GCSCredentials: cfg.Storage.GCSCredentials,
        MaxFileSize:    cfg.Storage.MaxFileSize,
        AllowedMIMEs:   cfg.Storage.AllowedMIMEs,
    })
    if err != nil {
        log.Fatalf("storage: %v", err)
    }
```

After services init:
```go
    productSvc := productservice.New(
        productRepo, variantRepo, imageRepo,
        categoryRepo, styleTagRepo,
        storageBackend, cfg.Storage.AllowedMIMEs, cfg.Storage.MaxFileSize,
    )
```

Mount routes under existing brandGroup (still chained with auth + role + brand context):
```go
    brandProductHandler := producthandler.NewBrandProductHandler(productSvc)
    producthandler.MountBrandProducts(brandGroup, brandProductHandler)
```

Add static file serving for local storage (after `r.GET("/healthz", ...)`):
```go
    if cfg.Storage.Driver == "local" || cfg.Storage.Driver == "" {
        r.Static("/uploads", cfg.Storage.LocalDir)
    }
```

- [ ] **Step 7: Build + tests**

```bash
go build ./...
go test ./internal/product/... -v
```

Expected: build success, unit tests pass.

- [ ] **Step 8: Commit**

```bash
git add internal/product/service/ internal/product/handler/ cmd/api/main.go
git commit -m "feat(product): brand-side product/variant/image service, handlers, wiring"
```

---

## Phase D — Catalog read side (Tasks 20-22)

### Task 20: Catalog query builder (list + detail + suggestions) + integration tests

**Files:**
- Create: `internal/product/repo/catalog_query.go`
- Create: `internal/product/repo/catalog_query_test.go`

This is the most complex repo in the codebase. Tests come first because the SQL must handle many combinations.

- [ ] **Step 1: Write integration tests**

`internal/product/repo/catalog_query_test.go`:
```go
//go:build integration

package repo

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"

    "github.com/wearwhere/wearwhere_be/internal/product/domain"
    "github.com/wearwhere/wearwhere_be/internal/testfixtures"
)

func setupCatalogData(t *testing.T) (tx, sbA, sbB, sc, st any) { return nil, nil, nil, nil, nil }

// Helper: create active product with variants.
func mkActiveProduct(t *testing.T, db DBTX, brandID, categoryID [16]byte, name string, price float64, size, color string) (productID [16]byte) {
    ctx := context.Background()
    p := testfixtures.SeedProduct(t, db.(testfixtures.DBTX), brandID, categoryID, "active")
    // Override name
    _, err := db.Exec(ctx, `UPDATE products SET name=$1 WHERE id=$2`, name, p.ID)
    require.NoError(t, err)
    testfixtures.SeedVariant(t, db.(testfixtures.DBTX), p.ID, size, color, price, 10)
    return p.ID
}

func TestCatalog_List_NoFilters(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})
    sc := testfixtures.SeedCategory(t, tx)
    mkActiveProduct(t, tx, sb.ID, sc.ID, "Áo Thun Trắng", 250000, "M", "White")
    mkActiveProduct(t, tx, sb.ID, sc.ID, "Quần Jeans Xanh", 500000, "L", "Blue")

    repo := NewCatalogPG(tx)
    items, total, err := repo.List(context.Background(), &domain.ListProductsQuery{
        Page: 1, Limit: 10, Sort: "newest",
    })
    require.NoError(t, err)
    require.GreaterOrEqual(t, total, 2)
    require.GreaterOrEqual(t, len(items), 2)
}

func TestCatalog_List_SearchByName(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})
    sc := testfixtures.SeedCategory(t, tx)
    mkActiveProduct(t, tx, sb.ID, sc.ID, "Áo Thun Trắng", 250000, "M", "White")
    mkActiveProduct(t, tx, sb.ID, sc.ID, "Quần Jeans", 500000, "L", "Blue")

    repo := NewCatalogPG(tx)
    // Search with accents stripped — should match "Áo Thun Trắng"
    items, _, err := repo.List(context.Background(), &domain.ListProductsQuery{
        Q: "ao thun", Page: 1, Limit: 10, Sort: "relevance",
    })
    require.NoError(t, err)
    require.GreaterOrEqual(t, len(items), 1)
    found := false
    for _, i := range items {
        if i.Name == "Áo Thun Trắng" {
            found = true
        }
    }
    require.True(t, found, "expected áo thun trắng in results")
}

func TestCatalog_List_FilterByPriceRange(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})
    sc := testfixtures.SeedCategory(t, tx)
    mkActiveProduct(t, tx, sb.ID, sc.ID, "Cheap", 100000, "M", "White")
    mkActiveProduct(t, tx, sb.ID, sc.ID, "Pricey", 1000000, "M", "Black")

    repo := NewCatalogPG(tx)
    min := 50000.0
    max := 200000.0
    items, _, err := repo.List(context.Background(), &domain.ListProductsQuery{
        PriceMin: &min, PriceMax: &max, Page: 1, Limit: 10, Sort: "newest",
    })
    require.NoError(t, err)
    for _, i := range items {
        require.LessOrEqual(t, i.MinPrice, 200000.0)
        require.GreaterOrEqual(t, i.MinPrice, 50000.0)
    }
}

func TestCatalog_List_FilterByBrandSlug(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sbA := testfixtures.SeedBrand(t, tx, [16]byte{})
    sbB := testfixtures.SeedBrand(t, tx, [16]byte{})
    sc := testfixtures.SeedCategory(t, tx)
    mkActiveProduct(t, tx, sbA.ID, sc.ID, "From A", 100, "M", "X")
    mkActiveProduct(t, tx, sbB.ID, sc.ID, "From B", 100, "M", "X")

    repo := NewCatalogPG(tx)
    items, _, err := repo.List(context.Background(), &domain.ListProductsQuery{
        Brand: sbA.Slug, Page: 1, Limit: 10, Sort: "newest",
    })
    require.NoError(t, err)
    for _, i := range items {
        require.Equal(t, sbA.Slug, i.BrandSlug)
    }
}

func TestCatalog_List_SortPriceAsc(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})
    sc := testfixtures.SeedCategory(t, tx)
    mkActiveProduct(t, tx, sb.ID, sc.ID, "P1", 300, "M", "X")
    mkActiveProduct(t, tx, sb.ID, sc.ID, "P2", 100, "M", "Y")
    mkActiveProduct(t, tx, sb.ID, sc.ID, "P3", 200, "M", "Z")

    repo := NewCatalogPG(tx)
    items, _, err := repo.List(context.Background(), &domain.ListProductsQuery{
        Page: 1, Limit: 10, Sort: "price_asc", Brand: sb.Slug,
    })
    require.NoError(t, err)
    require.GreaterOrEqual(t, len(items), 3)
    // Verify non-decreasing min_price for our 3
    var prev float64
    for _, i := range items {
        require.GreaterOrEqual(t, i.MinPrice, prev)
        prev = i.MinPrice
    }
}

func TestCatalog_List_IncludesOnlyActive(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})
    sc := testfixtures.SeedCategory(t, tx)
    pDraft := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "draft")
    testfixtures.SeedVariant(t, tx, pDraft.ID, "M", "X", 100, 10)
    pActive := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "active")
    testfixtures.SeedVariant(t, tx, pActive.ID, "M", "Y", 100, 10)

    repo := NewCatalogPG(tx)
    items, _, err := repo.List(context.Background(), &domain.ListProductsQuery{
        Brand: sb.Slug, Page: 1, Limit: 100, Sort: "newest",
    })
    require.NoError(t, err)
    for _, i := range items {
        require.NotEqual(t, pDraft.ID, i.ID)
    }
}

func TestCatalog_Detail(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})
    sc := testfixtures.SeedCategory(t, tx)
    p := testfixtures.SeedProduct(t, tx, sb.ID, sc.ID, "active")
    testfixtures.SeedVariant(t, tx, p.ID, "S", "Red", 100, 5)
    testfixtures.SeedVariant(t, tx, p.ID, "M", "Red", 100, 3)

    repo := NewCatalogPG(tx)
    prod, cat, variants, _, _, err := repo.Detail(context.Background(), sb.Slug, p.Slug)
    require.NoError(t, err)
    require.Equal(t, p.ID, prod.ID)
    require.Equal(t, sc.ID, cat.ID)
    require.Len(t, variants, 2)
}

func TestCatalog_Suggestions(t *testing.T) {
    tx := testfixtures.BeginTx(t, testPool)
    sb := testfixtures.SeedBrand(t, tx, [16]byte{})
    sc := testfixtures.SeedCategory(t, tx)
    mkActiveProduct(t, tx, sb.ID, sc.ID, "Áo Khoác Bomber", 500000, "M", "X")

    repo := NewCatalogPG(tx)
    sugg, err := repo.Suggestions(context.Background(), "ao khoac bombe", 3)
    require.NoError(t, err)
    require.NotEmpty(t, sugg)
}
```

- [ ] **Step 2: Run — fails**

Run: `TEST_DATABASE_URL=<url> go test -tags=integration ./internal/product/repo/... -v -run TestCatalog`
Expected: build error.

- [ ] **Step 3: Write the catalog query**

`internal/product/repo/catalog_query.go`:
```go
package repo

import (
    "context"
    "errors"
    "fmt"
    "strings"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"

    "github.com/wearwhere/wearwhere_be/internal/product/domain"
)

type CatalogPG struct{ db DBTX }

func NewCatalogPG(db DBTX) *CatalogPG { return &CatalogPG{db: db} }

// List returns active products matching the query filters with pagination.
func (r *CatalogPG) List(ctx context.Context, q *domain.ListProductsQuery) ([]*domain.CatalogItem, int, error) {
    var conds []string
    var args []any
    add := func(cond string, arg any) {
        args = append(args, arg)
        conds = append(conds, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(args))))
    }

    if q.Q != "" {
        add("p.search_text % unaccent(lower(?))", q.Q)
    }
    if q.Category != "" {
        add("c.slug = ?", q.Category)
    }
    if q.Brand != "" {
        add("b.slug = ?", q.Brand)
    }
    if q.PriceMin != nil {
        add("vp.min_price >= ?", *q.PriceMin)
    }
    if q.PriceMax != nil {
        add("vp.min_price <= ?", *q.PriceMax)
    }
    if len(q.Style) > 0 {
        add(`EXISTS (
              SELECT 1 FROM product_style_tags pst
              JOIN style_tags st ON st.id = pst.style_tag_id
              WHERE pst.product_id = p.id AND st.slug = ANY(?))`, q.Style)
    }
    if len(q.Size) > 0 {
        add(`EXISTS (
              SELECT 1 FROM product_variants pv
              WHERE pv.product_id = p.id AND pv.size = ANY(?)
                AND pv.is_active AND pv.stock_qty > 0 AND pv.deleted_at IS NULL)`, q.Size)
    }
    if len(q.Color) > 0 {
        add(`EXISTS (
              SELECT 1 FROM product_variants pv
              WHERE pv.product_id = p.id AND pv.color = ANY(?)
                AND pv.is_active AND pv.stock_qty > 0 AND pv.deleted_at IS NULL)`, q.Color)
    }

    where := "p.deleted_at IS NULL AND p.status = 'active'" +
        " AND b.deleted_at IS NULL AND b.status = 'active'"
    if len(conds) > 0 {
        where += " AND " + strings.Join(conds, " AND ")
    }

    var orderBy string
    switch q.Sort {
    case "price_asc":
        orderBy = "vp.min_price ASC NULLS LAST, p.created_at DESC"
    case "price_desc":
        orderBy = "vp.min_price DESC NULLS LAST, p.created_at DESC"
    case "popular":
        orderBy = "p.sold_count DESC, p.view_count DESC, p.created_at DESC"
    case "relevance":
        if q.Q != "" {
            args = append(args, q.Q)
            relIdx := len(args)
            orderBy = fmt.Sprintf(
                "similarity(p.search_text, unaccent(lower($%d))) DESC, p.sold_count DESC, p.created_at DESC",
                relIdx)
        } else {
            orderBy = "p.created_at DESC"
        }
    case "newest", "":
        orderBy = "p.created_at DESC"
    default:
        orderBy = "p.created_at DESC"
    }

    page := q.Page
    if page < 1 {
        page = 1
    }
    limit := q.Limit
    if limit < 1 || limit > 60 {
        limit = 24
    }
    offset := (page - 1) * limit

    args = append(args, limit, offset)
    limOff := fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))

    selectSQL := `
SELECT p.id, p.brand_id, p.category_id, p.slug, p.name, p.description, p.status::text,
       p.currency, p.sold_count, p.view_count, p.created_at, p.updated_at, p.deleted_at,
       b.slug AS brand_slug, b.name AS brand_name,
       vp.min_price, vp.max_price, vp.in_stock,
       (SELECT url FROM product_images
         WHERE product_id = p.id ORDER BY sort_order ASC LIMIT 1) AS primary_image
  FROM products p
  JOIN brands b      ON b.id = p.brand_id
  JOIN categories c  ON c.id = p.category_id
  JOIN LATERAL (
    SELECT MIN(price) AS min_price, MAX(price) AS max_price,
           bool_or(stock_qty > 0) AS in_stock
      FROM product_variants
     WHERE product_id = p.id AND deleted_at IS NULL AND is_active
  ) vp ON true
 WHERE ` + where + `
 ORDER BY ` + orderBy + limOff

    rows, err := r.db.Query(ctx, selectSQL, args...)
    if err != nil {
        return nil, 0, err
    }
    defer rows.Close()
    var items []*domain.CatalogItem
    for rows.Next() {
        var it domain.CatalogItem
        var status string
        var minP, maxP *float64
        var inStock *bool
        var primary *string
        if err := rows.Scan(
            &it.ID, &it.BrandID, &it.CategoryID, &it.Slug, &it.Name, &it.Description,
            &status, &it.Currency, &it.SoldCount, &it.ViewCount,
            &it.CreatedAt, &it.UpdatedAt, &it.DeletedAt,
            &it.BrandSlug, &it.BrandName,
            &minP, &maxP, &inStock, &primary,
        ); err != nil {
            return nil, 0, err
        }
        it.Status = domain.ProductStatus(status)
        if minP != nil {
            it.MinPrice = *minP
        }
        if maxP != nil {
            it.MaxPrice = *maxP
        }
        if inStock != nil {
            it.InStock = *inStock
        }
        it.PrimaryImage = primary
        items = append(items, &it)
    }
    if err := rows.Err(); err != nil {
        return nil, 0, err
    }

    // Count query — same WHERE, no ORDER/LIMIT.
    countSQL := `
SELECT COUNT(*)
  FROM products p
  JOIN brands b      ON b.id = p.brand_id
  JOIN categories c  ON c.id = p.category_id
  JOIN LATERAL (
    SELECT MIN(price) AS min_price
      FROM product_variants
     WHERE product_id = p.id AND deleted_at IS NULL AND is_active
  ) vp ON true
 WHERE ` + where

    // Count uses only the filter args (not relevance, not limit/offset).
    countArgsLen := len(conds) // each cond added exactly one arg above; but relevance + limit/offset don't appear in conds
    // Re-derive count args precisely. Simpler: count conditions = conds slice length, so first N args.
    countArgs := args[:countArgsLen]
    var total int
    if err := r.db.QueryRow(ctx, countSQL, countArgs...).Scan(&total); err != nil {
        return nil, 0, err
    }
    return items, total, nil
}

// Detail returns product + category + variants + images + style tags.
func (r *CatalogPG) Detail(ctx context.Context, brandSlug, productSlug string) (
    *domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error,
) {
    prodRow := r.db.QueryRow(ctx,
        `SELECT `+productCols+` FROM products p
         JOIN brands b ON b.id = p.brand_id
         WHERE b.slug=$1 AND p.slug=$2
           AND p.deleted_at IS NULL AND b.deleted_at IS NULL`,
        brandSlug, productSlug)
    p, err := scanProduct(prodRow)
    if err != nil {
        if errors.Is(err, ErrNotFound) {
            return nil, nil, nil, nil, nil, err
        }
        return nil, nil, nil, nil, nil, err
    }
    return r.collectDetailParts(ctx, p)
}

func (r *CatalogPG) DetailByID(ctx context.Context, id uuid.UUID) (
    *domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error,
) {
    p, err := scanProduct(r.db.QueryRow(ctx,
        `SELECT `+productCols+` FROM products WHERE id=$1 AND deleted_at IS NULL`, id))
    if err != nil {
        return nil, nil, nil, nil, nil, err
    }
    return r.collectDetailParts(ctx, p)
}

func (r *CatalogPG) collectDetailParts(ctx context.Context, p *domain.Product) (
    *domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error,
) {
    var cat domain.Category
    if err := r.db.QueryRow(ctx,
        `SELECT id, slug, name, display_order FROM categories WHERE id=$1`, p.CategoryID).
        Scan(&cat.ID, &cat.Slug, &cat.Name, &cat.DisplayOrder); err != nil {
        return nil, nil, nil, nil, nil, err
    }

    variantRows, err := r.db.Query(ctx,
        `SELECT `+variantCols+` FROM product_variants
         WHERE product_id=$1 AND deleted_at IS NULL ORDER BY created_at ASC`, p.ID)
    if err != nil {
        return nil, nil, nil, nil, nil, err
    }
    var variants []*domain.Variant
    for variantRows.Next() {
        v, err := scanVariant(variantRows)
        if err != nil {
            variantRows.Close()
            return nil, nil, nil, nil, nil, err
        }
        variants = append(variants, v)
    }
    variantRows.Close()

    imageRows, err := r.db.Query(ctx,
        `SELECT `+imageCols+` FROM product_images
         WHERE product_id=$1 ORDER BY sort_order ASC`, p.ID)
    if err != nil {
        return nil, nil, nil, nil, nil, err
    }
    var images []*domain.Image
    for imageRows.Next() {
        i, err := scanImage(imageRows)
        if err != nil {
            imageRows.Close()
            return nil, nil, nil, nil, nil, err
        }
        images = append(images, i)
    }
    imageRows.Close()

    tagRows, err := r.db.Query(ctx,
        `SELECT s.id, s.slug, s.name
           FROM style_tags s
           JOIN product_style_tags pst ON pst.style_tag_id = s.id
          WHERE pst.product_id = $1 ORDER BY s.name`, p.ID)
    if err != nil {
        return nil, nil, nil, nil, nil, err
    }
    var tags []*domain.StyleTag
    for tagRows.Next() {
        var st domain.StyleTag
        if err := tagRows.Scan(&st.ID, &st.Slug, &st.Name); err != nil {
            tagRows.Close()
            return nil, nil, nil, nil, nil, err
        }
        tags = append(tags, &st)
    }
    tagRows.Close()

    return p, &cat, variants, images, tags, nil
}

func (r *CatalogPG) Suggestions(ctx context.Context, q string, limit int) ([]string, error) {
    if q == "" {
        return nil, nil
    }
    if limit <= 0 {
        limit = 3
    }
    rows, err := r.db.Query(ctx,
        `SELECT name
           FROM products
          WHERE status = 'active' AND deleted_at IS NULL
          ORDER BY similarity(unaccent(lower(name)), unaccent(lower($1))) DESC
          LIMIT $2`, q, limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []string
    for rows.Next() {
        var n string
        if err := rows.Scan(&n); err != nil {
            return nil, err
        }
        out = append(out, n)
    }
    return out, rows.Err()
}

// Ensure pgx interface in the package by no-op reference for the linter.
var _ = pgx.ErrNoRows
```

The `setupCatalogData` placeholder helper in the test file is not used by the actual tests — delete it. Or replace with a real helper. Adjust the test file to use only `mkActiveProduct`.

Note: in the test file, `mkActiveProduct` signature uses `[16]byte` for ids since `uuid.UUID` is `[16]byte` alias. The test calls inline-cast `db.(testfixtures.DBTX)` — this requires the `tx` to satisfy both `DBTX` (in repo package) and `testfixtures.DBTX`. They are structurally identical. If casting fails, change the helper to accept `testfixtures.DBTX` directly:

```go
func mkActiveProduct(t *testing.T, db testfixtures.DBTX, brandID, categoryID uuid.UUID, name string, price float64, size, color string) uuid.UUID {
    ctx := context.Background()
    p := testfixtures.SeedProduct(t, db, brandID, categoryID, "active")
    _, err := db.Exec(ctx, `UPDATE products SET name=$1 WHERE id=$2`, name, p.ID)
    require.NoError(t, err)
    testfixtures.SeedVariant(t, db, p.ID, size, color, price, 10)
    return p.ID
}
```

And update callers to pass `uuid.UUID` directly (which is what `SeedBrand` etc. return). Drop the `setupCatalogData` stub.

- [ ] **Step 4: Run tests**

Expected: 8 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/product/repo/catalog_query.go internal/product/repo/catalog_query_test.go
git commit -m "feat(product): catalog query with filters, sort, search, detail, suggestions"
```

---

### Task 21: Catalog service + unit tests

**Files:**
- Create: `internal/product/service/catalog_service.go`
- Create: `internal/product/service/catalog_service_test.go`

- [ ] **Step 1: Write unit test for fallback / view-count fire-and-forget**

`internal/product/service/catalog_service_test.go`:
```go
package service

import (
    "context"
    "errors"
    "sync/atomic"
    "testing"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    "github.com/wearwhere/wearwhere_be/internal/product/domain"
    "github.com/wearwhere/wearwhere_be/internal/product/repo"
)

type fakeCatalogRepo struct {
    items []*domain.CatalogItem
    total int
    err   error
    suggestions []string
}

func (f *fakeCatalogRepo) List(ctx context.Context, q *domain.ListProductsQuery) ([]*domain.CatalogItem, int, error) {
    return f.items, f.total, f.err
}
func (f *fakeCatalogRepo) Detail(ctx context.Context, bs, ps string) (*domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error) {
    return nil, nil, nil, nil, nil, repo.ErrNotFound
}
func (f *fakeCatalogRepo) DetailByID(ctx context.Context, id uuid.UUID) (*domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error) {
    return nil, nil, nil, nil, nil, repo.ErrNotFound
}
func (f *fakeCatalogRepo) Suggestions(ctx context.Context, q string, limit int) ([]string, error) {
    return f.suggestions, nil
}

type fakeProductRepoNoOp struct{ viewCount int32 }

func (f *fakeProductRepoNoOp) IncrementViewCount(ctx context.Context, id uuid.UUID) error {
    atomic.AddInt32(&f.viewCount, 1)
    return nil
}
// rest unused — satisfy interface with errors
func (f *fakeProductRepoNoOp) Create(ctx context.Context, brandID uuid.UUID, slug string, req *domain.CreateProductRequest) (*domain.Product, error) { return nil, errors.New("nope") }
func (f *fakeProductRepoNoOp) SlugExists(ctx context.Context, brandID uuid.UUID, slug string) (bool, error) { return false, nil }
func (f *fakeProductRepoNoOp) FindByID(ctx context.Context, id uuid.UUID) (*domain.Product, error) { return nil, repo.ErrNotFound }
func (f *fakeProductRepoNoOp) FindByBrandSlug(ctx context.Context, bs, ps string) (*domain.Product, error) { return nil, repo.ErrNotFound }
func (f *fakeProductRepoNoOp) Update(ctx context.Context, id, brandID uuid.UUID, r *domain.UpdateProductRequest) error { return nil }
func (f *fakeProductRepoNoOp) SoftDelete(ctx context.Context, id, brandID uuid.UUID) error { return nil }
func (f *fakeProductRepoNoOp) ListByBrand(ctx context.Context, brandID uuid.UUID, l, o int) ([]*domain.Product, int, error) { return nil, 0, nil }
func (f *fakeProductRepoNoOp) SetStyleTags(ctx context.Context, p uuid.UUID, ids []uuid.UUID) error { return nil }
func (f *fakeProductRepoNoOp) GetStyleTags(ctx context.Context, p uuid.UUID) ([]*domain.StyleTag, error) { return nil, nil }

func TestCatalog_List_EmptyResults_ReturnsSuggestions(t *testing.T) {
    cr := &fakeCatalogRepo{items: nil, total: 0, suggestions: []string{"Áo Thun"}}
    svc := NewCatalog(cr, &fakeProductRepoNoOp{})
    items, total, suggestions, err := svc.List(context.Background(), &domain.ListProductsQuery{
        Q: "asdfgh", Page: 1, Limit: 10,
    })
    require.NoError(t, err)
    require.Len(t, items, 0)
    require.Equal(t, 0, total)
    require.Equal(t, []string{"Áo Thun"}, suggestions)
}

func TestCatalog_List_NoQuery_NoSuggestions(t *testing.T) {
    cr := &fakeCatalogRepo{items: nil, total: 0, suggestions: []string{"x"}}
    svc := NewCatalog(cr, &fakeProductRepoNoOp{})
    _, _, suggestions, _ := svc.List(context.Background(), &domain.ListProductsQuery{
        Q: "", Page: 1, Limit: 10,
    })
    require.Nil(t, suggestions)
}
```

- [ ] **Step 2: Write the catalog service**

`internal/product/service/catalog_service.go`:
```go
package service

import (
    "context"
    "errors"

    "github.com/google/uuid"

    "github.com/wearwhere/wearwhere_be/internal/product/domain"
    "github.com/wearwhere/wearwhere_be/internal/product/repo"
)

type CatalogService struct {
    catalog repo.CatalogRepo
    products repo.ProductRepo
}

func NewCatalog(c repo.CatalogRepo, p repo.ProductRepo) *CatalogService {
    return &CatalogService{catalog: c, products: p}
}

// List returns (items, total, suggestions, err). Suggestions only when q is
// non-empty AND there were zero results.
func (s *CatalogService) List(ctx context.Context, q *domain.ListProductsQuery) ([]*domain.CatalogItem, int, []string, error) {
    items, total, err := s.catalog.List(ctx, q)
    if err != nil {
        return nil, 0, nil, err
    }
    var suggestions []string
    if total == 0 && q.Q != "" {
        suggestions, _ = s.catalog.Suggestions(ctx, q.Q, 3)
    }
    return items, total, suggestions, nil
}

func (s *CatalogService) Detail(ctx context.Context, brandSlug, productSlug string) (
    *domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error,
) {
    p, cat, vs, imgs, tags, err := s.catalog.Detail(ctx, brandSlug, productSlug)
    if errors.Is(err, repo.ErrNotFound) {
        return nil, nil, nil, nil, nil, domain.ErrProductNotFound
    }
    if err == nil {
        // fire-and-forget view increment
        go func(id uuid.UUID) {
            _ = s.products.IncrementViewCount(context.Background(), id)
        }(p.ID)
    }
    return p, cat, vs, imgs, tags, err
}

func (s *CatalogService) DetailByID(ctx context.Context, id uuid.UUID) (
    *domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error,
) {
    p, cat, vs, imgs, tags, err := s.catalog.DetailByID(ctx, id)
    if errors.Is(err, repo.ErrNotFound) {
        return nil, nil, nil, nil, nil, domain.ErrProductNotFound
    }
    if err == nil {
        go func(id uuid.UUID) {
            _ = s.products.IncrementViewCount(context.Background(), id)
        }(p.ID)
    }
    return p, cat, vs, imgs, tags, err
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/product/service/... -v`
Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/product/service/catalog_service.go internal/product/service/catalog_service_test.go
git commit -m "feat(product): catalog service with empty-result suggestions"
```

---

### Task 22: Catalog handlers + brand list/detail handlers + wire to main

**Files:**
- Create: `internal/product/handler/catalog_handler.go`
- Modify: `internal/brand/handler/brand_handler.go` (add public list/detail)
- Modify: `internal/brand/handler/routes.go` (add public mount)
- Modify: `internal/product/handler/routes.go` (add catalog mount)
- Modify: `cmd/api/main.go`

- [ ] **Step 1: Write catalog handler**

`internal/product/handler/catalog_handler.go`:
```go
package handler

import (
    "fmt"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    "github.com/wearwhere/wearwhere_be/internal/product/domain"
    productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
    "github.com/wearwhere/wearwhere_be/internal/product/service"
    "github.com/wearwhere/wearwhere_be/pkg/httpx"
)

type CatalogHandler struct {
    svc       *service.CatalogService
    categoryR productrepo.CategoryRepo
    styleR    productrepo.StyleTagRepo
}

func NewCatalogHandler(
    svc *service.CatalogService,
    cr productrepo.CategoryRepo,
    sr productrepo.StyleTagRepo,
) *CatalogHandler {
    return &CatalogHandler{svc: svc, categoryR: cr, styleR: sr}
}

func (h *CatalogHandler) List(c *gin.Context) {
    var q domain.ListProductsQuery
    if err := c.ShouldBindQuery(&q); err != nil {
        httpx.Error(c, http.StatusBadRequest, "INVALID_QUERY", err.Error())
        return
    }
    if q.Sort == "" {
        if q.Q != "" {
            q.Sort = "relevance"
        } else {
            q.Sort = "newest"
        }
    }
    items, total, suggestions, err := h.svc.List(c.Request.Context(), &q)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    out := make([]domain.ProductSummary, 0, len(items))
    for _, it := range items {
        out = append(out, domain.ToProductSummary(it))
    }
    resp := gin.H{
        "items":      out,
        "pagination": paginationEnvelope(q.Page, q.Limit, total),
    }
    if len(suggestions) > 0 {
        resp["suggestions"] = suggestions
    }
    httpx.OK(c, resp)
}

func (h *CatalogHandler) Detail(c *gin.Context) {
    bs := c.Param("brand_slug")
    ps := c.Param("product_slug")
    h.respondDetail(c, func() (*domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error) {
        return h.svc.Detail(c.Request.Context(), bs, ps)
    })
}

func (h *CatalogHandler) DetailByID(c *gin.Context) {
    id, err := uuid.Parse(c.Param("id"))
    if err != nil {
        httpx.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid product id")
        return
    }
    h.respondDetail(c, func() (*domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error) {
        return h.svc.DetailByID(c.Request.Context(), id)
    })
}

func (h *CatalogHandler) respondDetail(c *gin.Context, fetch func() (*domain.Product, *domain.Category, []*domain.Variant, []*domain.Image, []*domain.StyleTag, error)) {
    p, cat, variants, images, tags, err := fetch()
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    vresp := make([]domain.VariantResp, 0, len(variants))
    for _, v := range variants {
        vresp = append(vresp, domain.ToVariantResp(v))
    }
    iresp := make([]domain.ImageResp, 0, len(images))
    for _, i := range images {
        iresp = append(iresp, domain.ToImageResp(i))
    }
    tresp := make([]domain.StyleTagRef, 0, len(tags))
    for _, t := range tags {
        tresp = append(tresp, domain.StyleTagRef{
            ID: t.ID.String(), Slug: t.Slug, Name: t.Name,
        })
    }
    // Brand lookup for brand_ref — query inline (cheap, indexed).
    // For Sprint 1 we accept this extra hop; later detail query can return brand fields.
    out := domain.ProductDetail{
        ID: p.ID.String(), Slug: p.Slug, Name: p.Name,
        Description: p.Description, Status: string(p.Status),
        Currency: p.Currency,
        Brand:    &domain.BrandRef{ID: p.BrandID.String()},
        Category: &domain.CategoryRef{
            ID: cat.ID.String(), Slug: cat.Slug, Name: cat.Name,
        },
        StyleTags: tresp,
        Variants:  vresp,
        Images:    iresp,
        CreatedAt: domain.FormatTime(p.CreatedAt),
    }
    httpx.OK(c, gin.H{"product": out})
}

func (h *CatalogHandler) ListCategories(c *gin.Context) {
    items, err := h.categoryR.List(c.Request.Context())
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    out := make([]domain.CategoryRef, 0, len(items))
    for _, x := range items {
        out = append(out, domain.CategoryRef{
            ID: x.ID.String(), Slug: x.Slug, Name: x.Name,
        })
    }
    httpx.OK(c, gin.H{"items": out})
}

func (h *CatalogHandler) ListStyleTags(c *gin.Context) {
    items, err := h.styleR.List(c.Request.Context())
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    out := make([]domain.StyleTagRef, 0, len(items))
    for _, x := range items {
        out = append(out, domain.StyleTagRef{
            ID: x.ID.String(), Slug: x.Slug, Name: x.Name,
        })
    }
    httpx.OK(c, gin.H{"items": out})
}

// Helper for query int parsing reused from brand handler.
func paginationEnvelopeC(page, limit, total int) gin.H {
    totalPages := (total + limit - 1) / limit
    if totalPages == 0 {
        totalPages = 1
    }
    return gin.H{
        "page": page, "limit": limit, "total": total,
        "total_pages": totalPages, "has_more": page < totalPages,
    }
}

// silence unused
var _ = fmt.Sprint
```

The handler reuses `paginationEnvelope` from the brand product handler file. If symbol collision, rename one or move the helper to a small `internal/product/handler/util.go`.

- [ ] **Step 2: Add public mount in routes.go**

`internal/product/handler/routes.go` — append:
```go
func MountCatalog(rg *gin.RouterGroup, h *CatalogHandler) {
    rg.GET("/products",                            h.List)
    rg.GET("/products/by-id/:id",                  h.DetailByID)
    rg.GET("/brands/:brand_slug/products/:product_slug", h.Detail)
    rg.GET("/categories",  h.ListCategories)
    rg.GET("/style-tags",  h.ListStyleTags)
}
```

- [ ] **Step 3: Add public brand handlers**

Append to `internal/brand/handler/brand_handler.go`:
```go
type BrandsPublicHandler struct{ svc *service.Service }

func NewBrandsPublicHandler(svc *service.Service) *BrandsPublicHandler {
    return &BrandsPublicHandler{svc: svc}
}

func (h *BrandsPublicHandler) List(c *gin.Context) {
    q := c.Query("q")
    sort := c.Query("sort")
    page := paginInt(c, "page", 1, 1, 1_000_000)
    limit := paginInt(c, "limit", 24, 1, 60)
    items, total, err := h.svc.ListBrands(c.Request.Context(), q, sort, limit, (page-1)*limit)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    out := make([]domain.BrandResponse, 0, len(items))
    for _, b := range items {
        out = append(out, domain.ToBrandResponse(b))
    }
    httpx.OK(c, gin.H{
        "items": out,
        "pagination": paginEnvelope(page, limit, total),
    })
}

func (h *BrandsPublicHandler) Detail(c *gin.Context) {
    slug := c.Param("slug")
    b, err := h.svc.GetBySlug(c.Request.Context(), slug)
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    addrs, err := h.svc.ListAddresses(c.Request.Context(), b.ID, false) // public only
    if err != nil {
        httpx.ErrorFromApp(c, err)
        return
    }
    addrOut := make([]domain.AddressResponse, 0, len(addrs))
    for _, a := range addrs {
        addrOut = append(addrOut, domain.ToAddressResponse(a))
    }
    httpx.OK(c, gin.H{
        "brand":     domain.ToBrandResponse(b),
        "addresses": addrOut,
    })
}

func paginInt(c *gin.Context, key string, def, min, max int) int {
    v := c.Query(key)
    if v == "" {
        return def
    }
    var n int
    _, err := fmt.Sscanf(v, "%d", &n)
    if err != nil || n < min {
        return def
    }
    if n > max {
        return max
    }
    return n
}

func paginEnvelope(page, limit, total int) gin.H {
    totalPages := (total + limit - 1) / limit
    if totalPages == 0 {
        totalPages = 1
    }
    return gin.H{
        "page": page, "limit": limit, "total": total,
        "total_pages": totalPages, "has_more": page < totalPages,
    }
}
```

Add to imports: `"fmt"`.

- [ ] **Step 4: Update brand routes**

`internal/brand/handler/routes.go`:
```go
package handler

import "github.com/gin-gonic/gin"

type Deps struct {
    Brand   *BrandHandler
    Address *AddressHandler
}

func Mount(rg *gin.RouterGroup, d *Deps) {
    rg.GET("",  d.Brand.Me)
    rg.PATCH("", d.Brand.UpdateMe)

    addr := rg.Group("/addresses")
    {
        addr.GET("",       d.Address.List)
        addr.POST("",      d.Address.Create)
        addr.PATCH(":id",  d.Address.Update)
        addr.DELETE(":id", d.Address.Delete)
    }
}

func MountBrandsPublic(rg *gin.RouterGroup, h *BrandsPublicHandler) {
    rg.GET("/brands",        h.List)
    rg.GET("/brands/:slug",  h.Detail)
}
```

- [ ] **Step 5: Wire main.go**

Add catalog service + handlers after product service init:
```go
    catalogRepo := productrepo.NewCatalogPG(pgPool)
    catalogSvc := productservice.NewCatalog(catalogRepo, productRepo)
    catalogHandler := producthandler.NewCatalogHandler(catalogSvc, categoryRepo, styleTagRepo)
    brandsPublicHandler := brandhandler.NewBrandsPublicHandler(brandSvc)
```

After existing `handler.Mount(v1, deps)` (auth) and `brandhandler.Mount(brandGroup, brandDeps)`:
```go
    producthandler.MountCatalog(v1, catalogHandler)
    brandhandler.MountBrandsPublic(v1, brandsPublicHandler)
```

- [ ] **Step 6: Build + run tests**

```bash
go build ./...
go test ./...
```

Expected: build success, all unit tests pass.

- [ ] **Step 7: Manual smoke**

```bash
make run
# in another shell:
curl 'http://localhost:8080/api/v1/categories'    # expect 10 items
curl 'http://localhost:8080/api/v1/style-tags'    # expect 10 items
curl 'http://localhost:8080/api/v1/brands'        # expect 2 items (Local-X, BadVibes)
curl 'http://localhost:8080/api/v1/brands/local-x'  # expect brand + addresses
curl 'http://localhost:8080/api/v1/products'      # expect [] (no products yet)
```

- [ ] **Step 8: Commit**

```bash
git add internal/product/handler/catalog_handler.go internal/product/handler/routes.go \
        internal/brand/handler/brand_handler.go internal/brand/handler/routes.go \
        cmd/api/main.go
git commit -m "feat(product): public catalog endpoints + brand list/detail wired"
```

---

## Phase E — E2E test, Makefile, smoke (Tasks 23-25)

### Task 23: Test DB setup + Makefile targets

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Append test targets**

Add to `Makefile`:
```makefile
TEST_DB_URL ?= postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable

test-db-up:
	@docker compose exec -T postgres psql -U wearwhere -d wearwhere \
	    -c "CREATE DATABASE wearwhere_test;" 2>/dev/null || true
	migrate -path $(MIGRATIONS_DIR) -database "$(TEST_DB_URL)" up

test-db-reset:
	@docker compose exec -T postgres psql -U wearwhere -d wearwhere \
	    -c "DROP DATABASE IF EXISTS wearwhere_test; CREATE DATABASE wearwhere_test;"
	migrate -path $(MIGRATIONS_DIR) -database "$(TEST_DB_URL)" up

test-unit:
	go test ./... -v -race

test-integration: test-db-up
	TEST_DATABASE_URL="$(TEST_DB_URL)" go test ./... -tags=integration -v -race

test: test-unit test-integration
```

Update `help` target to add new lines.

- [ ] **Step 2: Verify**

```bash
make test-db-up
make test-unit
make test-integration
```

Expected: all tests across modules pass.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "chore(make): add test-db-up, test-unit, test-integration targets"
```

---

### Task 24: E2E happy-path test

**Files:**
- Create: `cmd/api/main_test.go`

This runs the real HTTP server against the test DB and exercises the full flow.

- [ ] **Step 1: Write the test**

`cmd/api/main_test.go`:
```go
//go:build integration

package main

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "mime/multipart"
    "net/http"
    "net/http/httptest"
    "os"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/stretchr/testify/require"

    authhandler "github.com/wearwhere/wearwhere_be/internal/auth/handler"
    authmw "github.com/wearwhere/wearwhere_be/internal/auth/middleware"
    authrepo "github.com/wearwhere/wearwhere_be/internal/auth/repo"
    authservice "github.com/wearwhere/wearwhere_be/internal/auth/service"
    brandhandler "github.com/wearwhere/wearwhere_be/internal/brand/handler"
    brandmw "github.com/wearwhere/wearwhere_be/internal/brand/middleware"
    brandrepo "github.com/wearwhere/wearwhere_be/internal/brand/repo"
    brandservice "github.com/wearwhere/wearwhere_be/internal/brand/service"
    "github.com/wearwhere/wearwhere_be/internal/config"
    authdomain "github.com/wearwhere/wearwhere_be/internal/auth/domain"
    producthandler "github.com/wearwhere/wearwhere_be/internal/product/handler"
    productrepo "github.com/wearwhere/wearwhere_be/internal/product/repo"
    productservice "github.com/wearwhere/wearwhere_be/internal/product/service"
    jwtsvc "github.com/wearwhere/wearwhere_be/internal/shared/jwt"
    "github.com/wearwhere/wearwhere_be/internal/shared/storage"
    authvalidator "github.com/wearwhere/wearwhere_be/internal/shared/validator"
)

func buildTestServer(t *testing.T, pool *pgxpool.Pool, storageBackend storage.Storage) (*httptest.Server, *jwtsvc.Issuer) {
    gin.SetMode(gin.TestMode)
    authvalidator.RegisterWithGin()

    jwtIssuer := jwtsvc.NewIssuer("test-secret", 15*60*60*1000_000_000) // 15h
    userRepo := authrepo.NewUserPG(pool)
    sessionRepo := authrepo.NewSessionPG(pool)

    // Auth (minimal — login flow only)
    tokenSvc := authservice.NewTokenService(jwtIssuer, sessionRepo, 24*60*60*1000_000_000)
    authSvc := authservice.NewAuthService(userRepo, nil, tokenSvc, nil, config.LimitConfig{})

    brandRepo := brandrepo.NewBrandPG(pool)
    addressRepo := brandrepo.NewAddressPG(pool)
    brandSvc := brandservice.New(brandRepo, addressRepo)

    productRepo := productrepo.NewProductPG(pool)
    variantRepo := productrepo.NewVariantPG(pool)
    imageRepo := productrepo.NewImagePG(pool)
    categoryRepo := productrepo.NewCategoryPG(pool)
    styleTagRepo := productrepo.NewStyleTagPG(pool)
    catalogRepo := productrepo.NewCatalogPG(pool)

    productSvc := productservice.New(productRepo, variantRepo, imageRepo,
        categoryRepo, styleTagRepo, storageBackend,
        []string{"image/jpeg", "image/png", "image/webp"}, 5*1024*1024)
    catalogSvc := productservice.NewCatalog(catalogRepo, productRepo)

    r := gin.New()
    r.Use(gin.Recovery())
    v1 := r.Group("/api/v1", authmw.OptionalAuth(jwtIssuer))

    authhandler.Mount(v1, &authhandler.Deps{
        Auth:      authhandler.NewAuthHandler(authSvc),
        JWTIssuer: jwtIssuer,
    })

    brandGroup := v1.Group("/brand/me",
        authmw.RequireAuth(jwtIssuer),
        authmw.RequireRole(authdomain.RoleBrand),
        brandmw.BrandContext(brandRepo),
    )
    brandhandler.Mount(brandGroup, &brandhandler.Deps{
        Brand:   brandhandler.NewBrandHandler(brandSvc),
        Address: brandhandler.NewAddressHandler(brandSvc),
    })
    producthandler.MountBrandProducts(brandGroup, producthandler.NewBrandProductHandler(productSvc))
    producthandler.MountCatalog(v1, producthandler.NewCatalogHandler(catalogSvc, categoryRepo, styleTagRepo))
    brandhandler.MountBrandsPublic(v1, brandhandler.NewBrandsPublicHandler(brandSvc))

    srv := httptest.NewServer(r)
    t.Cleanup(srv.Close)
    return srv, jwtIssuer
}

func issueTokenForOwner(t *testing.T, jwtIssuer *jwtsvc.Issuer, pool *pgxpool.Pool, brandOwnerID string) string {
    tok, _, err := jwtIssuer.Issue(brandOwnerID, string(authdomain.RoleBrand), "owner@e2e.test")
    require.NoError(t, err)
    return tok
}

func TestE2E_BrandCreatesProduct_AppearsInCatalog(t *testing.T) {
    url := os.Getenv("TEST_DATABASE_URL")
    if url == "" {
        t.Skip("TEST_DATABASE_URL not set")
    }
    pool, err := pgxpool.New(context.Background(), url)
    require.NoError(t, err)
    t.Cleanup(pool.Close)

    // Seed: brand owner, brand, category in a tx — but here we need permanent
    // rows (the server uses pool, not tx). Insert directly; clean up at end.
    ctx := context.Background()
    var ownerID, brandID, categoryID string
    require.NoError(t, pool.QueryRow(ctx,
        `INSERT INTO users (email, role, status, name)
         VALUES ('e2e-owner@test.local', 'brand', 'active', 'E2E Owner')
         RETURNING id`).Scan(&ownerID))
    require.NoError(t, pool.QueryRow(ctx,
        `INSERT INTO brands (slug, name, owner_user_id, status)
         VALUES ('e2e-brand', 'E2E Brand', $1, 'active') RETURNING id`,
        ownerID).Scan(&brandID))
    require.NoError(t, pool.QueryRow(ctx,
        `INSERT INTO categories (slug, name) VALUES ('e2e-cat', 'E2E Category') RETURNING id`).
        Scan(&categoryID))

    t.Cleanup(func() {
        pool.Exec(ctx, `DELETE FROM product_images WHERE product_id IN
          (SELECT id FROM products WHERE brand_id=$1)`, brandID)
        pool.Exec(ctx, `DELETE FROM product_variants WHERE product_id IN
          (SELECT id FROM products WHERE brand_id=$1)`, brandID)
        pool.Exec(ctx, `DELETE FROM products WHERE brand_id=$1`, brandID)
        pool.Exec(ctx, `DELETE FROM brand_addresses WHERE brand_id=$1`, brandID)
        pool.Exec(ctx, `DELETE FROM brands WHERE id=$1`, brandID)
        pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, ownerID)
        pool.Exec(ctx, `DELETE FROM categories WHERE id=$1`, categoryID)
    })

    backend := storage.NewLocal(t.TempDir(), "http://test/uploads")
    srv, jwtIssuer := buildTestServer(t, pool, backend)
    token := issueTokenForOwner(t, jwtIssuer, pool, ownerID)

    // 1. Create product
    body := fmt.Sprintf(`{"name":"E2E Áo Thun","category_id":"%s"}`, categoryID)
    productID := postJSON(t, srv.URL+"/api/v1/brand/me/products", token, body, http.StatusCreated)["product"].(map[string]any)["id"].(string)

    // 2. Add a variant
    variantBody := `{"sku":"E2E-001","size":"M","color":"White","price":250000,"stock_qty":10}`
    _ = postJSON(t, srv.URL+"/api/v1/brand/me/products/"+productID+"/variants", token, variantBody, http.StatusCreated)

    // 3. Upload an image (multipart)
    var buf bytes.Buffer
    mw := multipart.NewWriter(&buf)
    fw, _ := mw.CreateFormFile("files", "tiny.jpg")
    // 8x8 white JPEG header — enough for DetectContentType
    fw.Write([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0, 1, 1, 0, 0, 1, 0, 1, 0, 0, 0xff, 0xd9})
    mw.Close()
    req, _ := http.NewRequest("POST", srv.URL+"/api/v1/brand/me/products/"+productID+"/images", &buf)
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", mw.FormDataContentType())
    resp, err := http.DefaultClient.Do(req)
    require.NoError(t, err)
    defer resp.Body.Close()
    require.Equal(t, http.StatusCreated, resp.StatusCode, "image upload should succeed")

    // 4. Publish (status draft → active)
    patchJSON(t, srv.URL+"/api/v1/brand/me/products/"+productID, token, `{"status":"active"}`, http.StatusNoContent)

    // 5. Public list (no auth)
    list := getJSON(t, srv.URL+"/api/v1/products?q=ao+thun", "", http.StatusOK)
    items := list["items"].([]any)
    require.GreaterOrEqual(t, len(items), 1)

    // 6. Public detail
    detail := getJSON(t, srv.URL+"/api/v1/brands/e2e-brand/products/"+items[0].(map[string]any)["slug"].(string), "", http.StatusOK)
    prod := detail["product"].(map[string]any)
    variants := prod["variants"].([]any)
    images := prod["images"].([]any)
    require.Len(t, variants, 1)
    require.Len(t, images, 1)
}

// ── HTTP helpers ──
func postJSON(t *testing.T, url, token, body string, expectStatus int) map[string]any {
    return doJSON(t, "POST", url, token, body, expectStatus)
}
func patchJSON(t *testing.T, url, token, body string, expectStatus int) map[string]any {
    return doJSON(t, "PATCH", url, token, body, expectStatus)
}
func getJSON(t *testing.T, url, token string, expectStatus int) map[string]any {
    return doJSON(t, "GET", url, token, "", expectStatus)
}

func doJSON(t *testing.T, method, url, token, body string, expectStatus int) map[string]any {
    t.Helper()
    var rdr io.Reader
    if body != "" {
        rdr = bytes.NewReader([]byte(body))
    }
    req, err := http.NewRequest(method, url, rdr)
    require.NoError(t, err)
    if token != "" {
        req.Header.Set("Authorization", "Bearer "+token)
    }
    if body != "" {
        req.Header.Set("Content-Type", "application/json")
    }
    resp, err := http.DefaultClient.Do(req)
    require.NoError(t, err)
    defer resp.Body.Close()
    raw, _ := io.ReadAll(resp.Body)
    require.Equal(t, expectStatus, resp.StatusCode, "url=%s body=%s response=%s", url, body, string(raw))
    if len(raw) == 0 {
        return nil
    }
    var out map[string]any
    require.NoError(t, json.Unmarshal(raw, &out))
    return out
}
```

If `jwtIssuer.Issue(...)` signature differs from what's used above, inspect [internal/shared/jwt/jwt.go](internal/shared/jwt/jwt.go) and adapt: pass user_id, role, email. The auth module's existing tests should illustrate signatures; if absent, mimic what `authSvc.Login` does internally.

- [ ] **Step 2: Run E2E**

```bash
make test-integration -- -run TestE2E_BrandCreatesProduct
```

Expected: PASS in 3-5 seconds.

- [ ] **Step 3: Commit**

```bash
git add cmd/api/main_test.go
git commit -m "test(e2e): happy-path brand-creates-product-appears-in-catalog"
```

---

### Task 25: Smoke checklist + final session push

**Files:**
- Modify (optional): `Makefile` (add `seed-dev` convenience), `.env.example`

- [ ] **Step 1: Run full test suite**

```bash
make test
```

Expected: all unit and integration tests pass.

- [ ] **Step 2: Run `go vet` and `gofmt`**

```bash
go vet ./...
gofmt -l . | tee /dev/stderr | wc -l
# expect 0
```

- [ ] **Step 3: Manual smoke via curl**

Start fresh DB, apply migrations, run server:
```bash
make down
make up
make migrate-up
make run &
sleep 2
```

Then exercise:
```bash
# Login as seeded brand owner (requires the password hash in 000016 to match a known plain password — if seed hash is bogus, register a fresh user and manually flip its role to 'brand' via psql, then assign as owner of a new brand row).

# Public — no auth
curl -s http://localhost:8080/api/v1/categories | jq '.items | length'  # 10
curl -s http://localhost:8080/api/v1/style-tags | jq '.items | length'  # 10
curl -s http://localhost:8080/api/v1/brands | jq '.items | length'      # 2
curl -s http://localhost:8080/api/v1/brands/local-x | jq '.addresses | length' # 2

# Empty catalog (no products yet)
curl -s 'http://localhost:8080/api/v1/products' | jq '.items | length'  # 0
```

- [ ] **Step 4: Final push**

```bash
git status
git pull --rebase
git push
git status   # MUST show "up to date with origin"
```

- [ ] **Step 5: Close beads issues**

If any beads issues were created during the sprint, close them:
```bash
bd list --status=in_progress
bd close <id1> <id2> ...
```

- [ ] **Step 6: Commit any final files (Makefile help update, README addendum)**

If you added README/CHANGELOG entries:
```bash
git add Makefile README.md
git commit -m "docs: document sprint-1 endpoints and test commands"
git push
```

---

## Self-review (filled in at plan-author time)

**Spec coverage:** Every spec section maps to one or more tasks:
- Section 1 scope → Task 1-16 (migrations + brand + product write side) + 20-22 (catalog read)
- Section 2 data model → Tasks 1-5
- Section 3 API surface → Tasks 13 (brand), 19 (brand product write), 22 (catalog read)
- Section 4 search → Tasks 3 (trigger), 20 (catalog_query.go)
- Section 5 storage → Task 7
- Section 6 authZ + IDOR → Tasks 10/11/16/17 (repo-level IDOR), Task 13 (BrandContext)
- Section 7 testing → Tasks 8 (fixtures), 23 (Makefile), 24 (E2E), and per-task integration tests

**Placeholder scan:** No "TBD" / "TODO" / "implement later" / "similar to" remain. All code blocks include real implementations.

**Type consistency:** Repo interfaces in Task 9 and Task 14 match what later tasks use (e.g. `BrandRepo.FindByOwnerUserID`, `ProductRepo.SlugExists`, `CatalogRepo.List/Detail/Suggestions`). DTOs in Task 9 (`UpdateBrandRequest`) and Task 14 (`CreateProductRequest`) are the same names used by services and handlers.

**Known gotchas in this plan (read before implementing):**

1. **Password hash in `000016_seed_dev_brands.up.sql`** is a placeholder bcrypt value. It is unlikely to match any real password. After applying migrations, either:
   - Run `auth_service.Register` to create a fresh owner user, then `UPDATE users SET role='brand'` and reassign brand ownership, OR
   - Regenerate the hash with `hash.HashPassword("DevBrand@1234")` from `internal/shared/hash` and replace it in the migration before running.

2. **The `force_recompute_product_search_text` trigger fires on every UPDATE.** This is intentional (to catch the resync-from-brand path) but means every variant/image update touching `products.updated_at` does extra work. Acceptable at Sprint 1 scale (<10k products); revisit if profiling shows it as a hotspot.

3. **`gtefield=PriceMin` validator in `ListProductsQuery`** depends on go-playground/validator's `gtefield` tag for pointer fields. If the tag misbehaves on `*float64`, switch to manual check in the handler.

4. **Static `/uploads` route** is registered only when `STORAGE_DRIVER` is `local` or empty. In prod (`STORAGE_DRIVER=gcs`), the route is absent and URLs returned by the GCS stub will currently 502 because GCS is not implemented. Acceptable for Sprint 1; Sprint 2+ will implement GCS.




