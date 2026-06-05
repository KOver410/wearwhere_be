-- Backfill Goship location codes for the seeded brand pickup (store) addresses.
--
-- Migration 000017 seeded brand_addresses before 000028 added the
-- city_code/district_code/ward_code columns, so the seeded rows have NULL codes.
-- Goship needs at least city_code + district_code on the PICKUP address to quote
-- shipping; without them /me/checkout/preview (and order placement) fails with
-- HTTP 500 "brand pickup address missing city/district code", which blocks
-- checkout for every cart containing those brands.
--
-- Codes verified against the live Goship locations dataset (GET /locations):
--   Hà Nội = 100000, Quận Hai Bà Trưng = 100300
--   Hồ Chí Minh = 700000, Quận 1 = 700100

UPDATE brand_addresses
SET city_code = '100000', district_code = '100300'
WHERE city = 'Hà Nội' AND district = 'Hai Bà Trưng' AND city_code IS NULL;

UPDATE brand_addresses
SET city_code = '700000', district_code = '700100'
WHERE city = 'Hồ Chí Minh' AND district = 'Quận 1' AND city_code IS NULL;
