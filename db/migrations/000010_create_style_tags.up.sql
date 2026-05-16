CREATE TABLE style_tags (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    slug        CITEXT NOT NULL UNIQUE,
    name        VARCHAR(80) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
