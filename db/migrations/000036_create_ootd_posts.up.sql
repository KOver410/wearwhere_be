CREATE TABLE ootd_posts (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    caption       TEXT,
    photo_urls    TEXT[] NOT NULL CHECK (array_length(photo_urls,1) BETWEEN 1 AND 10),
    status        TEXT NOT NULL DEFAULT 'published' CHECK (status IN ('published','hidden')),
    like_count    INT  NOT NULL DEFAULT 0,
    comment_count INT  NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ
);
CREATE INDEX idx_ootd_posts_feed ON ootd_posts (created_at DESC) WHERE deleted_at IS NULL AND status='published';
CREATE INDEX idx_ootd_posts_user ON ootd_posts (user_id, created_at DESC) WHERE deleted_at IS NULL;
