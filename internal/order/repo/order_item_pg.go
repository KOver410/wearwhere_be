// internal/order/repo/order_item_pg.go
package repo

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
)

type OrderItemPG struct{ db DBTX }

func NewOrderItemPG(db DBTX) *OrderItemPG { return &OrderItemPG{db: db} }

const orderItemCols = `id, sub_order_id, variant_id, product_id, product_name, variant_label,
                       image_url, qty, unit_price_vnd, line_total_vnd, created_at`

func scanOrderItem(row pgx.Row) (*domain.OrderItem, error) {
	var it domain.OrderItem
	err := row.Scan(
		&it.ID, &it.SubOrderID, &it.VariantID, &it.ProductID,
		&it.ProductName, &it.VariantLabel, &it.ImageURL,
		&it.Qty, &it.UnitPriceVND, &it.LineTotalVND, &it.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &it, nil
}

func (r *OrderItemPG) Create(ctx context.Context, db DBTX, it *domain.OrderItem) error {
	if db == nil {
		db = r.db
	}
	row := db.QueryRow(ctx,
		`INSERT INTO order_items
		   (sub_order_id, variant_id, product_id, product_name, variant_label, image_url,
		    qty, unit_price_vnd, line_total_vnd)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, created_at`,
		it.SubOrderID, it.VariantID, it.ProductID, it.ProductName, it.VariantLabel, it.ImageURL,
		it.Qty, it.UnitPriceVND, it.LineTotalVND)
	return row.Scan(&it.ID, &it.CreatedAt)
}

func (r *OrderItemPG) ListBySubOrderID(ctx context.Context, subOrderID uuid.UUID) ([]*domain.OrderItem, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+orderItemCols+` FROM order_items WHERE sub_order_id = $1 ORDER BY created_at ASC`,
		subOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.OrderItem
	for rows.Next() {
		it, err := scanOrderItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *OrderItemPG) ListByOrderID(ctx context.Context, orderID uuid.UUID) ([]*domain.OrderItem, error) {
	rows, err := r.db.Query(ctx,
		`SELECT oi.id, oi.sub_order_id, oi.variant_id, oi.product_id,
		        oi.product_name, oi.variant_label, oi.image_url,
		        oi.qty, oi.unit_price_vnd, oi.line_total_vnd, oi.created_at
		   FROM order_items oi
		   JOIN sub_orders so ON so.id = oi.sub_order_id
		  WHERE so.order_id = $1
		  ORDER BY oi.created_at ASC`,
		orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.OrderItem
	for rows.Next() {
		it, err := scanOrderItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
