ALTER TABLE product_variants
  ADD COLUMN weight_g  INT,
  ADD COLUMN length_cm INT,
  ADD COLUMN width_cm  INT,
  ADD COLUMN height_cm INT,
  ADD CONSTRAINT chk_variant_weight_pos CHECK (weight_g  IS NULL OR weight_g  > 0),
  ADD CONSTRAINT chk_variant_length_pos CHECK (length_cm IS NULL OR length_cm > 0),
  ADD CONSTRAINT chk_variant_width_pos  CHECK (width_cm  IS NULL OR width_cm  > 0),
  ADD CONSTRAINT chk_variant_height_pos CHECK (height_cm IS NULL OR height_cm > 0);
