// internal/order/repo/order_pg.go
package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/order/domain"
)

type OrderPG struct{ db DBTX }

func NewOrderPG(db DBTX) *OrderPG { return &OrderPG{db: db} }

const orderCols = `id, user_id, order_no, subtotal_vnd, shipping_total_vnd, grand_total_vnd,
                   payment_method, payment_status, status, shipping_address, notes, cancel_reason,
                   created_at, updated_at, paid_at, cancelled_at`

func scanOrder(row pgx.Row) (*domain.Order, error) {
	var o domain.Order
	var addrJSON []byte
	var notes, cancelReason *string
	err := row.Scan(
		&o.ID, &o.UserID, &o.OrderNo,
		&o.SubtotalVND, &o.ShippingTotalVND, &o.GrandTotalVND,
		&o.PaymentMethod, &o.PaymentStatus, &o.Status,
		&addrJSON, &notes, &cancelReason,
		&o.CreatedAt, &o.UpdatedAt, &o.PaidAt, &o.CancelledAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal(addrJSON, &o.ShippingAddress); err != nil {
		return nil, fmt.Errorf("decode shipping_address: %w", err)
	}
	if notes != nil {
		o.Notes = *notes
	}
	if cancelReason != nil {
		o.CancelReason = *cancelReason
	}
	return &o, nil
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && (constraint == "" || strings.Contains(pgErr.ConstraintName, constraint))
	}
	return false
}

func (r *OrderPG) Create(ctx context.Context, db DBTX, o *domain.Order) error {
	if db == nil {
		db = r.db
	}
	addrJSON, err := json.Marshal(o.ShippingAddress)
	if err != nil {
		return err
	}

	row := db.QueryRow(ctx,
		`INSERT INTO orders
		   (user_id, order_no, subtotal_vnd, shipping_total_vnd, grand_total_vnd,
		    payment_method, payment_status, status, shipping_address, notes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, ''))
		 RETURNING id, created_at, updated_at`,
		o.UserID, o.OrderNo, o.SubtotalVND, o.ShippingTotalVND, o.GrandTotalVND,
		o.PaymentMethod, o.PaymentStatus, o.Status, addrJSON, o.Notes)
	err = row.Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err, "order_no") {
			return ErrOrderNoConflict
		}
		return err
	}
	return nil
}

func (r *OrderPG) GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	return scanOrder(r.db.QueryRow(ctx,
		`SELECT `+orderCols+` FROM orders WHERE id = $1`, id))
}

func (r *OrderPG) GetByOrderNo(ctx context.Context, userID uuid.UUID, orderNo string) (*domain.Order, error) {
	return scanOrder(r.db.QueryRow(ctx,
		`SELECT `+orderCols+` FROM orders WHERE order_no = $1 AND user_id = $2`,
		orderNo, userID))
}

func (r *OrderPG) GetByOrderNoForUpdate(ctx context.Context, db DBTX, userID uuid.UUID, orderNo string) (*domain.Order, error) {
	if db == nil {
		db = r.db
	}
	return scanOrder(db.QueryRow(ctx,
		`SELECT `+orderCols+` FROM orders
		  WHERE order_no = $1 AND user_id = $2 FOR UPDATE`,
		orderNo, userID))
}

func (r *OrderPG) List(ctx context.Context, f ListFilter) ([]*domain.Order, int, error) {
	if f.PageSize <= 0 {
		f.PageSize = 20
	}
	if f.PageSize > 50 {
		f.PageSize = 50
	}
	if f.Page <= 0 {
		f.Page = 1
	}

	args := []any{f.UserID}
	where := []string{"user_id = $1"}
	i := 2
	if len(f.Statuses) > 0 {
		strs := make([]string, 0, len(f.Statuses))
		for _, s := range f.Statuses {
			args = append(args, string(s))
			strs = append(strs, fmt.Sprintf("$%d", i))
			i++
		}
		where = append(where, "status IN ("+strings.Join(strs, ",")+")")
	}
	if f.FromTime != nil && *f.FromTime != "" {
		if t, err := time.Parse(time.RFC3339, *f.FromTime); err == nil {
			args = append(args, t)
			where = append(where, fmt.Sprintf("created_at >= $%d", i))
			i++
		}
	}
	if f.ToTime != nil && *f.ToTime != "" {
		if t, err := time.Parse(time.RFC3339, *f.ToTime); err == nil {
			args = append(args, t)
			where = append(where, fmt.Sprintf("created_at <= $%d", i))
			i++
		}
	}
	whereSQL := "WHERE " + strings.Join(where, " AND ")

	// count
	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM orders `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// page
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.Query(ctx,
		`SELECT `+orderCols+` FROM orders `+whereSQL+
			fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, i, i+1),
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*domain.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, o)
	}
	return out, total, rows.Err()
}

func (r *OrderPG) UpdateStatusOnPaid(ctx context.Context, db DBTX, orderID uuid.UUID) error {
	if db == nil {
		db = r.db
	}
	tag, err := db.Exec(ctx,
		`UPDATE orders
		    SET status = 'processing',
		        payment_status = 'paid',
		        paid_at = NOW(),
		        updated_at = NOW()
		  WHERE id = $1 AND payment_status = 'pending'`,
		orderID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *OrderPG) UpdateStatusOnCancel(ctx context.Context, db DBTX, orderID uuid.UUID, reason string, paymentStatus domain.PaymentStatus) error {
	if db == nil {
		db = r.db
	}
	tag, err := db.Exec(ctx,
		`UPDATE orders
		    SET status = 'cancelled',
		        payment_status = $2,
		        cancel_reason = NULLIF($3, ''),
		        cancelled_at = NOW(),
		        updated_at = NOW()
		  WHERE id = $1 AND status != 'cancelled'`,
		orderID, paymentStatus, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
