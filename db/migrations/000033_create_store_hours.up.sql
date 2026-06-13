CREATE TABLE store_hours (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    brand_address_id  UUID NOT NULL REFERENCES brand_addresses(id) ON DELETE CASCADE,
    weekday           SMALLINT NOT NULL CHECK (weekday BETWEEN 0 AND 6), -- 0=Sunday .. 6=Saturday
    open_time         TIME NOT NULL,
    close_time        TIME NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_store_hours_addr ON store_hours (brand_address_id, weekday);
