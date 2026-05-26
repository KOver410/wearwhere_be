package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/cart/domain"
)

var ErrNotFound = errors.New("cart: not found")

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type CartRepo interface {
	// UpsertAdd inserts or, on conflict (user_id, variant_id), increments qty
	// clamped to <=10. Caller validates qty range, variant availability, and stock first.
	UpsertAdd(ctx context.Context, userID, variantID uuid.UUID, qty int, price float64) (*domain.CartItem, error)
	// FindByID returns a cart_items row scoped to userID.
	FindByID(ctx context.Context, id, userID uuid.UUID) (*domain.CartItem, error)
	// FindByVariant returns the existing cart row for (user, variant), or ErrNotFound.
	FindByVariant(ctx context.Context, userID, variantID uuid.UUID) (*domain.CartItem, error)
	// UpdateQty sets qty + refreshes price_snapshot.
	UpdateQty(ctx context.Context, id, userID uuid.UUID, qty int, price float64) (*domain.CartItem, error)
	// Delete removes a single item; ErrNotFound if absent or wrong user.
	Delete(ctx context.Context, id, userID uuid.UUID) error
	// Clear deletes all cart_items for the user.
	Clear(ctx context.Context, userID uuid.UUID) error
	// ListView returns denormalized rows joined with variant + product + brand
	// + primary image. Soft-deleted variants/products surface with Unavailable=true.
	ListView(ctx context.Context, userID uuid.UUID) ([]*domain.CartItemView, error)
}
