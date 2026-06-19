CREATE TABLE promo_redemptions (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    promo_code_id UUID NOT NULL REFERENCES promo_codes(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    order_id      UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    discount_vnd  BIGINT NOT NULL CHECK (discount_vnd >= 0),
    redeemed_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (promo_code_id, user_id)   -- one redemption per user (MVP)
);

CREATE INDEX idx_promo_redemptions_user ON promo_redemptions (user_id);
