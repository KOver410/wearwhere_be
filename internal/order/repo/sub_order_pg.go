// internal/order/repo/sub_order_pg.go
package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
)

type SubOrderPG struct{ db DBTX }

func NewSubOrderPG(db DBTX) *SubOrderPG { return &SubOrderPG{db: db} }

const subOrderCols = `id, order_id, brand_id, subtotal_vnd, shipping_fee_vnd, total_vnd,
                      status, tracking_no, shipping_carrier, shipping_provider,
                      confirmed_at, shipped_at, delivered_at, cancelled_at,
                      shipping_cost_vnd, goship_shipment_code, tracking_url, shipping_status_text,
                      created_at, updated_at`

func scanSubOrder(row pgx.Row, includeBrandJoin bool) (*domain.SubOrder, error) {
	var s domain.SubOrder
	if includeBrandJoin {
		err := row.Scan(
			&s.ID, &s.OrderID, &s.BrandID, &s.SubtotalVND, &s.ShippingFeeVND, &s.TotalVND,
			&s.Status, &s.TrackingNo, &s.ShippingCarrier, &s.ShippingProvider,
			&s.ConfirmedAt, &s.ShippedAt, &s.DeliveredAt, &s.CancelledAt,
			&s.ShippingCostVND, &s.GoshipShipmentCode, &s.TrackingURL, &s.ShippingStatusText,
			&s.CreatedAt, &s.UpdatedAt,
			&s.BrandSlug, &s.BrandName,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		return &s, nil
	}
	err := row.Scan(
		&s.ID, &s.OrderID, &s.BrandID, &s.SubtotalVND, &s.ShippingFeeVND, &s.TotalVND,
		&s.Status, &s.TrackingNo, &s.ShippingCarrier, &s.ShippingProvider,
		&s.ConfirmedAt, &s.ShippedAt, &s.DeliveredAt, &s.CancelledAt,
		&s.ShippingCostVND, &s.GoshipShipmentCode, &s.TrackingURL, &s.ShippingStatusText,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}

func (r *SubOrderPG) Create(ctx context.Context, db DBTX, so *domain.SubOrder) error {
	if db == nil {
		db = r.db
	}
	row := db.QueryRow(ctx,
		`INSERT INTO sub_orders
		   (order_id, brand_id, subtotal_vnd, shipping_fee_vnd, total_vnd, status,
		    shipping_carrier, shipping_provider)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, created_at, updated_at`,
		so.OrderID, so.BrandID, so.SubtotalVND, so.ShippingFeeVND, so.TotalVND, so.Status,
		so.ShippingCarrier, so.ShippingProvider)
	return row.Scan(&so.ID, &so.CreatedAt, &so.UpdatedAt)
}

func (r *SubOrderPG) ListByOrderID(ctx context.Context, orderID uuid.UUID) ([]*domain.SubOrder, error) {
	rows, err := r.db.Query(ctx,
		`SELECT s.id, s.order_id, s.brand_id, s.subtotal_vnd, s.shipping_fee_vnd, s.total_vnd,
		        s.status, s.tracking_no, s.shipping_carrier, s.shipping_provider,
		        s.confirmed_at, s.shipped_at, s.delivered_at, s.cancelled_at,
		        s.shipping_cost_vnd, s.goship_shipment_code, s.tracking_url, s.shipping_status_text,
		        s.created_at, s.updated_at,
		        b.slug, b.name
		   FROM sub_orders s
		   JOIN brands b ON b.id = s.brand_id
		  WHERE s.order_id = $1
		  ORDER BY b.name ASC`,
		orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.SubOrder
	for rows.Next() {
		so, err := scanSubOrder(rows, true)
		if err != nil {
			return nil, err
		}
		out = append(out, so)
	}
	return out, rows.Err()
}

func (r *SubOrderPG) CancelAllByOrderID(ctx context.Context, db DBTX, orderID uuid.UUID) error {
	if db == nil {
		db = r.db
	}
	_, err := db.Exec(ctx,
		`UPDATE sub_orders
		    SET status = 'cancelled',
		        cancelled_at = NOW(),
		        updated_at = NOW()
		  WHERE order_id = $1 AND status != 'cancelled'`,
		orderID)
	return err
}
