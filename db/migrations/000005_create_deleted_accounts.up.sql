-- Audit trail for GDPR account deletion (90-day retention)
CREATE TABLE deleted_accounts (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL,
    email_hash      TEXT,         -- hashed for anonymisation
    phone_hash      TEXT,
    deleted_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    purge_after     TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_deleted_accounts_purge ON deleted_accounts(purge_after);
