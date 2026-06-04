ALTER TABLE sub_orders
  DROP COLUMN IF EXISTS shipping_cost_vnd,
  DROP COLUMN IF EXISTS goship_shipment_code,
  DROP COLUMN IF EXISTS tracking_url,
  DROP COLUMN IF EXISTS shipping_status_text;
