// internal/payment/repo/repo.go
package repo

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/payment/domain"
)

// DBTX abstracts over *pgxpool.Pool and pgx.Tx.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type PaymentRepo interface {
	Create(ctx context.Context, db DBTX, p *domain.Payment) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	GetByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Payment, error)
	GetByPayosOrderCode(ctx context.Context, code int64) (*domain.Payment, error)
	GetByPayosOrderCodeForUpdate(ctx context.Context, db DBTX, code int64) (*domain.Payment, error)
	UpdatePayosLink(ctx context.Context, db DBTX, id uuid.UUID, paymentLinkID, checkoutURL, qrCode string) error
	UpdateOnPaid(ctx context.Context, db DBTX, id uuid.UUID, rawPayload []byte) error
	UpdateOnFailed(ctx context.Context, db DBTX, id uuid.UUID, reason string, rawPayload []byte) error
	UpdateOnCancelled(ctx context.Context, db DBTX, id uuid.UUID) error
	UpdateOnExpired(ctx context.Context, db DBTX, id uuid.UUID) error
}
