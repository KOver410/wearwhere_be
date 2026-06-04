// internal/order/repo/sub_order_pg.go
package repo

import (
	"context"
	"errors"
	"strconv"

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

func (r *SubOrderPG) GetByID(ctx context.Context, id uuid.UUID) (*domain.SubOrder, error) {
	row := r.db.QueryRow(ctx,
		`SELECT s.id, s.order_id, s.brand_id, s.subtotal_vnd, s.shipping_fee_vnd, s.total_vnd,
		        s.status, s.tracking_no, s.shipping_carrier, s.shipping_provider,
		        s.confirmed_at, s.shipped_at, s.delivered_at, s.cancelled_at,
		        s.shipping_cost_vnd, s.goship_shipment_code, s.tracking_url, s.shipping_status_text,
		        s.created_at, s.updated_at, b.slug, b.name
		   FROM sub_orders s JOIN brands b ON b.id = s.brand_id
		  WHERE s.id = $1`, id)
	return scanSubOrder(row, true)
}

func (r *SubOrderPG) GetByIDForUpdate(ctx context.Context, db DBTX, id uuid.UUID) (*domain.SubOrder, error) {
	if db == nil {
		db = r.db
	}
	row := db.QueryRow(ctx,
		`SELECT `+subOrderCols+` FROM sub_orders WHERE id = $1 FOR UPDATE`, id)
	return scanSubOrder(row, false)
}

func (r *SubOrderPG) GetByTrackingNoForUpdate(ctx context.Context, db DBTX, trackingNo string) (*domain.SubOrder, error) {
	if db == nil {
		db = r.db
	}
	row := db.QueryRow(ctx,
		`SELECT `+subOrderCols+` FROM sub_orders WHERE tracking_no = $1 FOR UPDATE`, trackingNo)
	return scanSubOrder(row, false)
}

func (r *SubOrderPG) ListByBrand(ctx context.Context, brandID uuid.UUID, statuses []domain.SubOrderStatus, page, pageSize int) ([]*domain.SubOrder, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	args := []any{brandID}
	where := "s.brand_id = $1"
	if len(statuses) > 0 {
		ss := make([]string, len(statuses))
		for i, st := range statuses {
			ss[i] = string(st)
		}
		args = append(args, ss)
		where += " AND s.status = ANY($2)"
	}
	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM sub_orders s WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.Query(ctx,
		`SELECT s.id, s.order_id, s.brand_id, s.subtotal_vnd, s.shipping_fee_vnd, s.total_vnd,
		        s.status, s.tracking_no, s.shipping_carrier, s.shipping_provider,
		        s.confirmed_at, s.shipped_at, s.delivered_at, s.cancelled_at,
		        s.shipping_cost_vnd, s.goship_shipment_code, s.tracking_url, s.shipping_status_text,
		        s.created_at, s.updated_at, b.slug, b.name
		   FROM sub_orders s JOIN brands b ON b.id = s.brand_id
		  WHERE `+where+`
		  ORDER BY s.created_at DESC
		  LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.SubOrder
	for rows.Next() {
		so, err := scanSubOrder(rows, true)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, so)
	}
	return out, total, rows.Err()
}

func (r *SubOrderPG) UpdateConfirmed(ctx context.Context, db DBTX, id uuid.UUID) error {
	if db == nil {
		db = r.db
	}
	_, err := db.Exec(ctx,
		`UPDATE sub_orders SET status='confirmed', confirmed_at=NOW(), updated_at=NOW()
		  WHERE id=$1 AND status='pending'`, id)
	return err
}

func (r *SubOrderPG) UpdateShipped(ctx context.Context, db DBTX, id uuid.UUID, trackingNo, goshipCode, carrier string, costVND int64, trackingURL string) error {
	if db == nil {
		db = r.db
	}
	_, err := db.Exec(ctx,
		`UPDATE sub_orders
		    SET status='shipped', shipped_at=NOW(), updated_at=NOW(),
		        tracking_no=$2, goship_shipment_code=$3, shipping_carrier=$4,
		        shipping_cost_vnd=$5, tracking_url=$6
		  WHERE id=$1 AND status='confirmed'`,
		id, trackingNo, goshipCode, carrier, costVND, trackingURL)
	return err
}

func (r *SubOrderPG) UpdateDelivered(ctx context.Context, db DBTX, id uuid.UUID, statusText, trackingURL string) error {
	if db == nil {
		db = r.db
	}
	_, err := db.Exec(ctx,
		`UPDATE sub_orders
		    SET status='delivered', delivered_at=NOW(), updated_at=NOW(),
		        shipping_status_text=$2, tracking_url=COALESCE(NULLIF($3,''), tracking_url)
		  WHERE id=$1 AND status <> 'delivered'`,
		id, statusText, trackingURL)
	return err
}

func (r *SubOrderPG) UpdateShippingStatus(ctx context.Context, db DBTX, id uuid.UUID, statusText, trackingURL string) error {
	if db == nil {
		db = r.db
	}
	_, err := db.Exec(ctx,
		`UPDATE sub_orders
		    SET shipping_status_text=$2, tracking_url=COALESCE(NULLIF($3,''), tracking_url),
		        status=CASE WHEN status='confirmed' THEN 'shipped' ELSE status END,
		        shipped_at=CASE WHEN status='confirmed' AND shipped_at IS NULL THEN NOW() ELSE shipped_at END,
		        updated_at=NOW()
		  WHERE id=$1 AND status NOT IN ('delivered','cancelled')`,
		id, statusText, trackingURL)
	return err
}

func (r *SubOrderPG) AllDelivered(ctx context.Context, db DBTX, orderID uuid.UUID) (bool, error) {
	if db == nil {
		db = r.db
	}
	var notDelivered int
	err := db.QueryRow(ctx,
		`SELECT COUNT(*) FROM sub_orders WHERE order_id=$1 AND status NOT IN ('delivered','cancelled')`, orderID).Scan(&notDelivered)
	if err != nil {
		return false, err
	}
	return notDelivered == 0, nil
}
