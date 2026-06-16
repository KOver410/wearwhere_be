package repo

import (
	"context"

	"github.com/google/uuid"
)

type SignalPG struct{ db DBTX }

func NewSignalPG(db DBTX) *SignalPG { return &SignalPG{db: db} }

func (r *SignalPG) FollowedBrandIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Query(ctx, `SELECT brand_id FROM brand_follows WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *SignalPG) PurchasedProductIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT oi.product_id
		  FROM order_items oi
		  JOIN sub_orders so ON so.id = oi.sub_order_id
		  JOIN orders o      ON o.id = so.order_id
		 WHERE o.user_id = $1 AND so.status = 'delivered'`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *SignalPG) AffinityCategoryIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT p.category_id FROM (
		    SELECT oi.product_id
		      FROM order_items oi
		      JOIN sub_orders so ON so.id = oi.sub_order_id
		      JOIN orders o      ON o.id = so.order_id
		     WHERE o.user_id = $1 AND so.status = 'delivered'
		    UNION
		    SELECT wi.product_id FROM wishlist_items wi WHERE wi.user_id = $1
		) src
		JOIN products p ON p.id = src.product_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
