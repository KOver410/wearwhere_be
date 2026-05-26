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
