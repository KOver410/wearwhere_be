CREATE TABLE brand_addresses (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    brand_id        UUID NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    label           VARCHAR(80) NOT NULL,
    address_line    VARCHAR(255) NOT NULL,
    ward            VARCHAR(80) NOT NULL,
    district        VARCHAR(80) NOT NULL,
    city            VARCHAR(80) NOT NULL,
    country         CHAR(2) NOT NULL DEFAULT 'VN',
    postal_code     VARCHAR(20),
    phone           VARCHAR(20),
    latitude        NUMERIC(10,7),
    longitude       NUMERIC(10,7),
    is_primary      BOOLEAN NOT NULL DEFAULT FALSE,
    is_public       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_brand_addr_primary_unique
    ON brand_addresses (brand_id) WHERE is_primary AND deleted_at IS NULL;
CREATE INDEX idx_brand_addr_brand_public
    ON brand_addresses (brand_id, is_public)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_brand_addr_geo
    ON brand_addresses (latitude, longitude)
    WHERE latitude IS NOT NULL AND longitude IS NOT NULL;
