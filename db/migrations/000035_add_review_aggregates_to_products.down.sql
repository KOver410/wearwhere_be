ALTER TABLE products
  DROP COLUMN IF EXISTS avg_rating,
  DROP COLUMN IF EXISTS review_count;
