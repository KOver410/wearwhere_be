CREATE TABLE sub_orders (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id UUID NOT NULL REFERENCES orders(id),
  brand_id UUID NOT NULL REFERENCES brands(id),
  subtotal_vnd BIGINT NOT NULL CHECK (subtotal_vnd >= 0),
  shipping_fee_vnd BIGINT NOT NULL CHECK (shipping_fee_vnd >= 0),
  total_vnd BIGINT NOT NULL CHECK (total_vnd >= 0),
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','confirmed','preparing','shipped','delivered','cancelled')),
  tracking_no TEXT,
  shipping_provider TEXT,
  confirmed_at TIMESTAMPTZ,
  shipped_at TIMESTAMPTZ,
  delivered_at TIMESTAMPTZ,
  cancelled_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (order_id, brand_id)
);

CREATE INDEX idx_sub_orders_order ON sub_orders(order_id);
CREATE INDEX idx_sub_orders_brand_status ON sub_orders(brand_id, status, created_at DESC);
