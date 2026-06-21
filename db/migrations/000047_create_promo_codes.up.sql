CREATE TYPE promo_discount_type AS ENUM ('percentage', 'fixed');

CREATE TABLE promo_codes (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    code                CITEXT NOT NULL UNIQUE,
    description         TEXT,
    discount_type       promo_discount_type NOT NULL,
    discount_value      BIGINT NOT NULL CHECK (discount_value > 0), -- percent (1..100) OR VND amount
    max_discount_vnd    BIGINT CHECK (max_discount_vnd IS NULL OR max_discount_vnd > 0), -- cap for percentage
    min_order_value_vnd BIGINT NOT NULL DEFAULT 0 CHECK (min_order_value_vnd >= 0),
    starts_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ends_at             TIMESTAMPTZ,
    is_active           BOOLEAN NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (discount_type <> 'percentage' OR discount_value BETWEEN 1 AND 100),
    CHECK (ends_at IS NULL OR ends_at > starts_at)
);

CREATE INDEX idx_promo_codes_active ON promo_codes (is_active, starts_at, ends_at);
