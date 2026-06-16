package repo

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
)

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// CandidateRepo loads the scorable product pool.
type CandidateRepo interface {
	Candidates(ctx context.Context, limit int) ([]domain.Candidate, error)
}

// SignalRepo loads per-user behavioral signals.
type SignalRepo interface {
	FollowedBrandIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	PurchasedProductIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	AffinityCategoryIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
}
