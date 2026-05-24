ALTER TABLE product_variants
  DROP CONSTRAINT IF EXISTS chk_variant_reserved_le_stock,
  DROP CONSTRAINT IF EXISTS chk_variant_reserved_nonneg,
  DROP COLUMN IF EXISTS reserved_qty;
