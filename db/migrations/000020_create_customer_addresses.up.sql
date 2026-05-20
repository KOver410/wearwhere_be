CREATE TABLE customer_addresses (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label           VARCHAR(40)  NOT NULL,
    recipient_name  VARCHAR(120) NOT NULL,
    recipient_phone VARCHAR(20)  NOT NULL,
    address_line    VARCHAR(255) NOT NULL,
    ward            VARCHAR(80)  NOT NULL,
    district        VARCHAR(80)  NOT NULL,
    city            VARCHAR(80)  NOT NULL,
    country         CHAR(2)      NOT NULL DEFAULT 'VN',
    postal_code     VARCHAR(20),
    note            VARCHAR(255),
    is_default      BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX customer_addresses_user_default_uniq
    ON customer_addresses (user_id)
    WHERE is_default AND deleted_at IS NULL;

CREATE INDEX customer_addresses_user_idx
    ON customer_addresses (user_id, deleted_at);
