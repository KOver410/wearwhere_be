package repo

import (
	"context"

	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
)

type CandidatePG struct{ db DBTX }

func NewCandidatePG(db DBTX) *CandidatePG { return &CandidatePG{db: db} }

func (r *CandidatePG) Candidates(ctx context.Context, limit int) ([]domain.Candidate, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id, p.brand_id, p.category_id, p.slug, p.name,
		       b.slug, b.name, p.currency,
		       vp.min_price, p.sold_count, p.created_at,
		       (SELECT url FROM product_images
		          WHERE product_id = p.id ORDER BY sort_order ASC LIMIT 1) AS primary_image,
		       COALESCE(
		         (SELECT array_agg(pst.style_tag_id)
		            FROM product_style_tags pst WHERE pst.product_id = p.id),
		         '{}'::uuid[]) AS style_tag_ids
		  FROM products p
		  JOIN brands b ON b.id = p.brand_id
		  JOIN LATERAL (
		    SELECT MIN(price) AS min_price, bool_or(stock_qty > 0) AS in_stock
		      FROM product_variants
		     WHERE product_id = p.id AND deleted_at IS NULL AND is_active
		  ) vp ON true
		 WHERE p.deleted_at IS NULL AND p.status = 'active'
		   AND b.deleted_at IS NULL AND b.status = 'active'
		   AND vp.in_stock
		 ORDER BY p.sold_count DESC, p.created_at DESC, p.id ASC
		 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Candidate
	for rows.Next() {
		var c domain.Candidate
		if err := rows.Scan(
			&c.ProductID, &c.BrandID, &c.CategoryID, &c.Slug, &c.Name,
			&c.BrandSlug, &c.BrandName, &c.Currency,
			&c.MinPrice, &c.SoldCount, &c.CreatedAt,
			&c.PrimaryImage, &c.StyleTagIDs,
		); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
