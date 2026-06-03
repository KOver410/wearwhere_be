ALTER TABLE product_variants
  DROP CONSTRAINT IF EXISTS chk_variant_weight_pos,
  DROP CONSTRAINT IF EXISTS chk_variant_length_pos,
  DROP CONSTRAINT IF EXISTS chk_variant_width_pos,
  DROP CONSTRAINT IF EXISTS chk_variant_height_pos,
  DROP COLUMN IF EXISTS weight_g,
  DROP COLUMN IF EXISTS length_cm,
  DROP COLUMN IF EXISTS width_cm,
  DROP COLUMN IF EXISTS height_cm;
