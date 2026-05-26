// internal/payment/repo/payment_pg.go
package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/payment/domain"
)

type PaymentPG struct{ db DBTX }

func NewPaymentPG(db DBTX) *PaymentPG { return &PaymentPG{db: db} }

const paymentCols = `id, order_id, amount_vnd, method, status,
                     payos_order_code, payos_payment_link_id, payos_checkout_url, payos_qr_code,
                     expired_at, paid_at, failure_reason, raw_webhook_payload,
                     created_at, updated_at`

func scanPayment(row pgx.Row) (*domain.Payment, error) {
	var p domain.Payment
	err := row.Scan(
		&p.ID, &p.OrderID, &p.AmountVND, &p.Method, &p.Status,
		&p.PayosOrderCode, &p.PayosPaymentLinkID, &p.PayosCheckoutURL, &p.PayosQRCode,
		&p.ExpiredAt, &p.PaidAt, &p.FailureReason, &p.RawWebhookPayload,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrPaymentNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (r *PaymentPG) Create(ctx context.Context, db DBTX, p *domain.Payment) error {
	if db == nil {
		db = r.db
	}
	row := db.QueryRow(ctx,
		`INSERT INTO payments
		   (order_id, amount_vnd, method, status, payos_order_code, expired_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at, updated_at`,
		p.OrderID, p.AmountVND, p.Method, p.Status, p.PayosOrderCode, p.ExpiredAt)
	return row.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (r *PaymentPG) GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	return scanPayment(r.db.QueryRow(ctx,
		`SELECT `+paymentCols+` FROM payments WHERE id=$1`, id))
}

func (r *PaymentPG) GetByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Payment, error) {
	return scanPayment(r.db.QueryRow(ctx,
		`SELECT `+paymentCols+` FROM payments WHERE order_id=$1 ORDER BY created_at DESC LIMIT 1`,
		orderID))
}

func (r *PaymentPG) GetByPayosOrderCode(ctx context.Context, code int64) (*domain.Payment, error) {
	return scanPayment(r.db.QueryRow(ctx,
		`SELECT `+paymentCols+` FROM payments WHERE payos_order_code=$1`, code))
}

func (r *PaymentPG) GetByPayosOrderCodeForUpdate(ctx context.Context, db DBTX, code int64) (*domain.Payment, error) {
	if db == nil {
		db = r.db
	}
	return scanPayment(db.QueryRow(ctx,
		`SELECT `+paymentCols+` FROM payments WHERE payos_order_code=$1 FOR UPDATE`, code))
}

func (r *PaymentPG) UpdatePayosLink(ctx context.Context, db DBTX, id uuid.UUID, linkID, checkoutURL, qrCode string) error {
	if db == nil {
		db = r.db
	}
	tag, err := db.Exec(ctx,
		`UPDATE payments
		    SET payos_payment_link_id=$2, payos_checkout_url=$3, payos_qr_code=$4, updated_at=NOW()
		  WHERE id=$1`,
		id, linkID, checkoutURL, qrCode)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrPaymentNotFound
	}
	return nil
}

func (r *PaymentPG) UpdateOnPaid(ctx context.Context, db DBTX, id uuid.UUID, raw []byte) error {
	if db == nil {
		db = r.db
	}
	tag, err := db.Exec(ctx,
		`UPDATE payments
		    SET status='paid', paid_at=NOW(), raw_webhook_payload=$2, updated_at=NOW()
		  WHERE id=$1 AND status='pending'`,
		id, raw)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrIdempotent
	}
	return nil
}

func (r *PaymentPG) UpdateOnFailed(ctx context.Context, db DBTX, id uuid.UUID, reason string, raw []byte) error {
	if db == nil {
		db = r.db
	}
	tag, err := db.Exec(ctx,
		`UPDATE payments
		    SET status='failed', failure_reason=$2, raw_webhook_payload=$3, updated_at=NOW()
		  WHERE id=$1 AND status='pending'`,
		id, reason, raw)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrIdempotent
	}
	return nil
}

func (r *PaymentPG) UpdateOnCancelled(ctx context.Context, db DBTX, id uuid.UUID) error {
	if db == nil {
		db = r.db
	}
	tag, err := db.Exec(ctx,
		`UPDATE payments SET status='cancelled', updated_at=NOW()
		  WHERE id=$1 AND status='pending'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrIdempotent
	}
	return nil
}

func (r *PaymentPG) UpdateOnExpired(ctx context.Context, db DBTX, id uuid.UUID) error {
	if db == nil {
		db = r.db
	}
	tag, err := db.Exec(ctx,
		`UPDATE payments SET status='expired', updated_at=NOW()
		  WHERE id=$1 AND status='pending'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrIdempotent
	}
	return nil
}
