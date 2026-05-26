CREATE TYPE product_status AS ENUM ('draft', 'active', 'archived');

CREATE TABLE products (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    brand_id        UUID NOT NULL REFERENCES brands(id) ON DELETE RESTRICT,
    category_id    UUID NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
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

-- All indexes are partial (WHERE deleted_at IS NULL) per codebase convention.
CREATE UNIQUE INDEX idx_products_brand_slug
    ON products (brand_id, slug) WHERE deleted_at IS NULL;
CREATE INDEX idx_products_brand_status
    ON products (brand_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_products_category_status
    ON products (category_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_products_status_created
    ON products (status, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_products_popular
    ON products (sold_count DESC, view_count DESC)
    WHERE deleted_at IS NULL AND status = 'active';
CREATE INDEX idx_products_search_trgm
    ON products USING GIN (search_text gin_trgm_ops)
    WHERE deleted_at IS NULL AND status = 'active';

-- Trigger: recompute search_text on every INSERT/UPDATE of products.
-- The recompute pulls brand.name fresh each time so brand renames propagate.
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

CREATE TRIGGER trg_product_search_text
    BEFORE INSERT OR UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION force_recompute_product_search_text();

-- Trigger: when brand.name changes, touch updated_at on its products so the
-- BEFORE UPDATE trigger above re-fires and recomputes their search_text.
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
