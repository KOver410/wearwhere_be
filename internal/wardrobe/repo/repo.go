package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
)

var ErrNoSnapshot = errors.New("wardrobe: no snapshot")

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type ClosetRepo interface {
	ClosetItems(ctx context.Context, userID uuid.UUID) ([]domain.ClosetItem, error)
}

type Snapshot struct {
	Signature string
	Outfits   []domain.Outfit
}

type SnapshotRepo interface {
	Load(ctx context.Context, userID uuid.UUID) (*Snapshot, error)
	Upsert(ctx context.Context, userID uuid.UUID, sig string, outfits []domain.Outfit, model string, tokensIn, tokensOut int) error
}
