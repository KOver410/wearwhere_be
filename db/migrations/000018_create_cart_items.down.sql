-- db/migrations/000018_create_cart_items.down.sql
DROP INDEX IF EXISTS cart_items_user_idx;
DROP INDEX IF EXISTS cart_items_user_variant_uniq;
DROP TABLE IF EXISTS cart_items;
