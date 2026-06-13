CREATE TABLE ootd_post_products (
    post_id    UUID NOT NULL REFERENCES ootd_posts(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    PRIMARY KEY (post_id, product_id)
);
