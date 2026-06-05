-- Revert the brand pickup address Goship code backfill.
UPDATE brand_addresses
SET city_code = NULL, district_code = NULL
WHERE (city = 'Hà Nội' AND district = 'Hai Bà Trưng')
   OR (city = 'Hồ Chí Minh' AND district = 'Quận 1');
