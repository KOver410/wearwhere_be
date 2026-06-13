CREATE TABLE product_reviews (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    product_id  UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating      SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    body        TEXT NOT NULL,
    fit         TEXT CHECK (fit IN ('small','true','large')),
    status      TEXT NOT NULL DEFAULT 'published' CHECK (status IN ('published','hidden')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_reviews_user_product
    ON product_reviews (product_id, user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_reviews_product
    ON product_reviews (product_id) WHERE deleted_at IS NULL AND status = 'published';
