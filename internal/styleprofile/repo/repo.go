package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
)

// ErrNotFound means the user has no saved style profile row.
var ErrNotFound = errors.New("styleprofile: not found")

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type StyleProfileRepo interface {
	Load(ctx context.Context, userID uuid.UUID) (*domain.StyleProfileView, error)
	Upsert(ctx context.Context, p domain.UpsertParams) (*domain.StyleProfileView, error)
	UnknownTagIDs(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error)
}
