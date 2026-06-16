CREATE TABLE style_profile_tags (
    user_id      UUID NOT NULL REFERENCES style_profiles(user_id) ON DELETE CASCADE,
    style_tag_id UUID NOT NULL REFERENCES style_tags(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, style_tag_id)
);
CREATE INDEX idx_style_profile_tags_user ON style_profile_tags (user_id);
