-- Sample opening hours (Mon-Sun 09:00-21:00) for every public, geocoded store.
-- weekday: 0=Sunday .. 6=Saturday.
INSERT INTO store_hours (brand_address_id, weekday, open_time, close_time)
SELECT ba.id, wd, TIME '09:00', TIME '21:00'
FROM brand_addresses ba
CROSS JOIN generate_series(0, 6) AS wd
WHERE ba.is_public = TRUE
  AND ba.deleted_at IS NULL
  AND ba.latitude IS NOT NULL
  AND ba.longitude IS NOT NULL
ON CONFLICT DO NOTHING;
