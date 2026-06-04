ALTER TABLE sub_orders
  ADD COLUMN shipping_cost_vnd    BIGINT CHECK (shipping_cost_vnd IS NULL OR shipping_cost_vnd >= 0),
  ADD COLUMN goship_shipment_code TEXT,
  ADD COLUMN tracking_url         TEXT,
  ADD COLUMN shipping_status_text TEXT;
