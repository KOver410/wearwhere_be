CREATE TABLE brand_follows (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    brand_id   UUID NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, brand_id)
);
CREATE INDEX idx_brand_follows_brand ON brand_follows (brand_id);
