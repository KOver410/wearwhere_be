CREATE TYPE brand_status AS ENUM ('pending', 'active', 'suspended');

CREATE TABLE brands (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    slug            CITEXT NOT NULL UNIQUE,
    name            VARCHAR(120) NOT NULL,
    owner_user_id   UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    story           TEXT,
    logo_url        TEXT,
    banner_url      TEXT,
    website_url     TEXT,
    status          brand_status NOT NULL DEFAULT 'active',
    verified_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_brands_owner_unique
    ON brands (owner_user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_brands_status
    ON brands (status) WHERE deleted_at IS NULL;
CREATE INDEX idx_brands_name_trgm
    ON brands USING GIN (name gin_trgm_ops);
