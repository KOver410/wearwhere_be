CREATE TABLE payments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id UUID NOT NULL REFERENCES orders(id),
  amount_vnd BIGINT NOT NULL CHECK (amount_vnd > 0),
  method TEXT NOT NULL CHECK (method IN ('cod','payos')),
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','paid','failed','cancelled','expired')),
  payos_order_code BIGINT UNIQUE,
  payos_payment_link_id TEXT,
  payos_checkout_url TEXT,
  payos_qr_code TEXT,
  expired_at TIMESTAMPTZ,
  paid_at TIMESTAMPTZ,
  failure_reason TEXT,
  raw_webhook_payload JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_payments_order_status ON payments(order_id, status);
CREATE INDEX idx_payments_cleanup ON payments(method, status, created_at)
  WHERE method = 'payos' AND status = 'pending';
