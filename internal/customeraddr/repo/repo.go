// Package repo provides persistence for customer addresses.
package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/customeraddr/domain"
)

var ErrNotFound = errors.New("customeraddr: not found")

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
}

type AddressRepo interface {
	List(ctx context.Context, userID uuid.UUID) ([]*domain.CustomerAddress, error)
	FindByID(ctx context.Context, id, userID uuid.UUID) (*domain.CustomerAddress, error)
	Create(ctx context.Context, userID uuid.UUID, req *domain.CreateAddressRequest) (*domain.CustomerAddress, error)
	Update(ctx context.Context, id, userID uuid.UUID, req *domain.UpdateAddressRequest) (*domain.CustomerAddress, error)
	SoftDelete(ctx context.Context, id, userID uuid.UUID) error
}
