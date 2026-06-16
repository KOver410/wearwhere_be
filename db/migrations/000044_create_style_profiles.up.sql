CREATE TABLE style_profiles (
    user_id      UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    budget_min   INTEGER,
    budget_max   INTEGER,
    onboarded_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (budget_min IS NULL OR budget_min >= 0),
    CHECK (budget_max IS NULL OR budget_min IS NULL OR budget_max >= budget_min)
);
