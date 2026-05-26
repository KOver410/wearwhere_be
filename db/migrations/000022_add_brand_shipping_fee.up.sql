ALTER TABLE brands
  ADD COLUMN shipping_flat_fee_vnd BIGINT NOT NULL DEFAULT 30000
    CHECK (shipping_flat_fee_vnd >= 0);
