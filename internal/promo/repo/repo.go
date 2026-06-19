// internal/promo/repo/repo.go
package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/promo/domain"
)

var (
	// ErrNotFound is returned when no promo row matches.
	ErrNotFound = errors.New("promo repo: not found")
	// ErrCodeConflict is returned by Create on a duplicate code.
	ErrCodeConflict = errors.New("promo repo: code conflict")
	// ErrAlreadyRedeemed is returned by InsertRedemption on (promo,user) conflict.
	ErrAlreadyRedeemed = errors.New("promo repo: already redeemed")
)

// DBTX abstracts over *pgxpool.Pool and pgx.Tx so validation + redemption can
// run inside the order-placement transaction.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// PromoRepo is the persistence port for promo codes and redemptions.
type PromoRepo interface {
	// GetActiveByCode loads an active code (no lock). db nil → pool.
	GetActiveByCode(ctx context.Context, db DBTX, code string) (*domain.PromoCode, error)
	// GetActiveByCodeForUpdate loads + row-locks an active code (FOR UPDATE).
	GetActiveByCodeForUpdate(ctx context.Context, db DBTX, code string) (*domain.PromoCode, error)
	// HasRedeemed reports whether the user already redeemed the code.
	HasRedeemed(ctx context.Context, db DBTX, promoID, userID uuid.UUID) (bool, error)
	// InsertRedemption records a redemption; ErrAlreadyRedeemed on (promo,user) conflict.
	InsertRedemption(ctx context.Context, db DBTX, promoID, userID, orderID uuid.UUID, discountVND int64) error

	// Admin operations (use pool).
	Create(ctx context.Context, p *domain.PromoCode) error
	Update(ctx context.Context, p *domain.PromoCode) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.PromoCode, error)
	List(ctx context.Context, page, pageSize int, activeOnly bool) (items []*domain.PromoCode, total int, err error)
}
