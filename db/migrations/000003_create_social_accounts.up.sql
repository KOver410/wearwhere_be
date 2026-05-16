CREATE TYPE oauth_provider AS ENUM ('google', 'apple');

CREATE TABLE social_accounts (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider            oauth_provider NOT NULL,
    provider_user_id    TEXT NOT NULL,
    email               CITEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, provider_user_id)
);

CREATE INDEX idx_social_accounts_user ON social_accounts(user_id);
