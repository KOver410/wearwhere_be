DROP TRIGGER IF EXISTS trg_brand_name_resync ON brands;
DROP FUNCTION IF EXISTS resync_brand_products_search();
DROP TRIGGER IF EXISTS trg_product_search_text ON products;
DROP FUNCTION IF EXISTS force_recompute_product_search_text();
DROP TABLE IF EXISTS products;
DROP TYPE IF EXISTS product_status;
