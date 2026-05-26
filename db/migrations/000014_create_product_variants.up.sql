CREATE TABLE product_variants (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    product_id      UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
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
    ON product_variants (product_id, is_active) WHERE deleted_at IS NULL;
