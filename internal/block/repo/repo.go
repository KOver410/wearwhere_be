// Package repo defines persistence for user blocks.
package repo

import (
	"context"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/block/domain"
)

type Repo interface {
	UserExists(ctx context.Context, id uuid.UUID) (bool, error)
	// Block inserts (blocker, blocked); idempotent via ON CONFLICT DO NOTHING.
	Block(ctx context.Context, blocker, blocked uuid.UUID) error
	// Unblock deletes the row; idempotent (no error if absent).
	Unblock(ctx context.Context, blocker, blocked uuid.UUID) error
	ListBlocked(ctx context.Context, blocker uuid.UUID, limit, offset int) ([]domain.BlockedUserItem, int, error)
}
