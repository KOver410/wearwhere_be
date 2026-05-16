CREATE TABLE refresh_sessions (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token_hash  TEXT NOT NULL UNIQUE,
    user_agent          TEXT,
    ip                  INET,
    expires_at          TIMESTAMPTZ NOT NULL,
    revoked_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_refresh_sessions_user ON refresh_sessions(user_id) WHERE revoked_at IS NULL;
CREATE INDEX idx_refresh_sessions_expires ON refresh_sessions(expires_at) WHERE revoked_at IS NULL;
