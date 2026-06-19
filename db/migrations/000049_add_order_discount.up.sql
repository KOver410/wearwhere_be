ALTER TABLE orders ADD COLUMN discount_vnd BIGINT NOT NULL DEFAULT 0 CHECK (discount_vnd >= 0);
ALTER TABLE orders ADD COLUMN promo_code   CITEXT;
