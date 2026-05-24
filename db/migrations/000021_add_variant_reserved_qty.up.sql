ALTER TABLE product_variants
  ADD COLUMN reserved_qty INT NOT NULL DEFAULT 0,
  ADD CONSTRAINT chk_variant_reserved_nonneg CHECK (reserved_qty >= 0),
  ADD CONSTRAINT chk_variant_reserved_le_stock CHECK (reserved_qty <= stock_qty);
