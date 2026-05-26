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
