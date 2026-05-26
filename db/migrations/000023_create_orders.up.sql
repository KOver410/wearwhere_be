CREATE TABLE orders (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  order_no TEXT NOT NULL UNIQUE,
  subtotal_vnd BIGINT NOT NULL CHECK (subtotal_vnd >= 0),
  shipping_total_vnd BIGINT NOT NULL CHECK (shipping_total_vnd >= 0),
  grand_total_vnd BIGINT NOT NULL CHECK (grand_total_vnd >= 0),
  payment_method TEXT NOT NULL CHECK (payment_method IN ('cod','payos')),
  payment_status TEXT NOT NULL DEFAULT 'pending'
    CHECK (payment_status IN ('pending','paid','failed','cancelled')),
  status TEXT NOT NULL DEFAULT 'pending_payment'
    CHECK (status IN ('pending_payment','processing','cancelled','completed')),
  shipping_address JSONB NOT NULL,
  notes TEXT,
  cancel_reason TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  paid_at TIMESTAMPTZ,
  cancelled_at TIMESTAMPTZ
);

CREATE INDEX idx_orders_user_created ON orders(user_id, created_at DESC);
CREATE INDEX idx_orders_status ON orders(status, created_at DESC);
