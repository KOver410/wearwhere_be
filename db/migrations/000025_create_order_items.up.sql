CREATE TABLE order_items (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  sub_order_id UUID NOT NULL REFERENCES sub_orders(id),
  variant_id UUID NOT NULL REFERENCES product_variants(id),
  product_id UUID NOT NULL REFERENCES products(id),
  product_name TEXT NOT NULL,
  variant_label TEXT NOT NULL,
  image_url TEXT,
  qty INT NOT NULL CHECK (qty > 0),
  unit_price_vnd BIGINT NOT NULL CHECK (unit_price_vnd >= 0),
  line_total_vnd BIGINT NOT NULL CHECK (line_total_vnd >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_order_items_sub_order ON order_items(sub_order_id);
CREATE INDEX idx_order_items_variant ON order_items(variant_id);
