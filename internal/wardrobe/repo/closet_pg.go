package repo

import (
	"context"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
)

type ClosetPG struct{ db DBTX }

func NewClosetPG(db DBTX) *ClosetPG { return &ClosetPG{db: db} }

func (r *ClosetPG) ClosetItems(ctx context.Context, userID uuid.UUID) ([]domain.ClosetItem, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id, p.name, c.slug, c.name,
		       COALESCE(array_agg(st.slug::text) FILTER (WHERE st.slug IS NOT NULL), '{}') AS style_slugs
		  FROM order_items oi
		  JOIN sub_orders so ON so.id = oi.sub_order_id
		  JOIN orders o      ON o.id = so.order_id
		  JOIN products p    ON p.id = oi.product_id
		  JOIN categories c  ON c.id = p.category_id
		  LEFT JOIN product_style_tags pst ON pst.product_id = p.id
		  LEFT JOIN style_tags st          ON st.id = pst.style_tag_id
		 WHERE o.user_id = $1 AND so.status = 'delivered' AND p.deleted_at IS NULL
		 GROUP BY p.id, p.name, c.slug, c.name
		 ORDER BY p.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ClosetItem
	for rows.Next() {
		var it domain.ClosetItem
		if err := rows.Scan(&it.ProductID, &it.Name, &it.CategorySlug, &it.CategoryName, &it.StyleSlugs); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
