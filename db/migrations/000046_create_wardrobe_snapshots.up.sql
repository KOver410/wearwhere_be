CREATE TABLE wardrobe_snapshots (
    user_id      UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    signature    TEXT NOT NULL,
    outfits      JSONB NOT NULL DEFAULT '[]'::jsonb,
    model        TEXT,
    tokens_in    INTEGER NOT NULL DEFAULT 0,
    tokens_out   INTEGER NOT NULL DEFAULT 0,
    generated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
