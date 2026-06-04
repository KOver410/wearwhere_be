ALTER TABLE brand_addresses
  DROP COLUMN IF EXISTS city_code,
  DROP COLUMN IF EXISTS district_code,
  DROP COLUMN IF EXISTS ward_code;
