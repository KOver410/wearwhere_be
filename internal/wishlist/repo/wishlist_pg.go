package repo

import (
	"context"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/wishlist/domain"
)

type WishlistPG struct{ db DBTX }

func NewWishlistPG(db DBTX) *WishlistPG { return &WishlistPG{db: db} }

// Add is idempotent — ON CONFLICT DO NOTHING avoids errors on re-add.
// Caller verifies product is active before calling.
func (r *WishlistPG) Add(ctx context.Context, userID, productID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO wishlist_items (user_id, product_id)
         VALUES ($1, $2)
         ON CONFLICT (user_id, product_id) DO NOTHING`,
		userID, productID)
	return err
}

// Remove is idempotent — no error if absent.
func (r *WishlistPG) Remove(ctx context.Context, userID, productID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM wishlist_items WHERE user_id=$1 AND product_id=$2`,
		userID, productID)
	return err
}

// List joins to products + brands; filters non-active/soft-deleted products.
// Returns (items, total, err) where total is the count across all pages.
func (r *WishlistPG) List(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.WishlistItemView, int, error) {
	const baseFrom = `
      FROM wishlist_items wi
      JOIN products p ON p.id = wi.product_id
                     AND p.status='active' AND p.deleted_at IS NULL
      JOIN brands b ON b.id = p.brand_id AND b.deleted_at IS NULL
     WHERE wi.user_id = $1`

	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) `+baseFrom, userID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(ctx, `
      SELECT
        p.id, p.slug, p.name,
        (SELECT url FROM product_images
           WHERE product_id=p.id AND is_primary
           ORDER BY sort_order ASC LIMIT 1) AS primary_image_url,
        (SELECT MIN(price) FROM product_variants
           WHERE product_id=p.id AND deleted_at IS NULL AND is_active) AS min_price,
        b.id, b.slug, b.name,
        wi.added_at
      `+baseFrom+`
      ORDER BY wi.added_at DESC
      LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*domain.WishlistItemView
	for rows.Next() {
		v := &domain.WishlistItemView{}
		if err := rows.Scan(
			&v.ProductID, &v.ProductSlug, &v.ProductName,
			&v.PrimaryImageURL, &v.MinPrice,
			&v.BrandID, &v.BrandSlug, &v.BrandName,
			&v.AddedAt,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

// Contains returns map[productID]true for IDs the user has wishlisted.
func (r *WishlistPG) Contains(ctx context.Context, userID uuid.UUID, productIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	out := make(map[uuid.UUID]bool, len(productIDs))
	if len(productIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT product_id FROM wishlist_items
         WHERE user_id=$1 AND product_id = ANY($2)`,
		userID, productIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pid uuid.UUID
		if err := rows.Scan(&pid); err != nil {
			return nil, err
		}
		out[pid] = true
	}
	return out, rows.Err()
}
