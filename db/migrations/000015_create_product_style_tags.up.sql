CREATE TABLE product_style_tags (
    product_id      UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    style_tag_id    UUID NOT NULL REFERENCES style_tags(id) ON DELETE CASCADE,
    PRIMARY KEY (product_id, style_tag_id)
);

CREATE INDEX idx_pst_tag_product ON product_style_tags (style_tag_id, product_id);
