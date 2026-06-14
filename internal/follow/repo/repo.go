// Package repo defines persistence for follows.
package repo

import (
	"context"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/follow/domain"
)

type Repo interface {
	UserExists(ctx context.Context, id uuid.UUID) (bool, error)
	BrandExists(ctx context.Context, id uuid.UUID) (bool, error)
	// FollowUser inserts (idempotent) and returns the followee's updated follower_count.
	FollowUser(ctx context.Context, follower, followee uuid.UUID) (int, error)
	UnfollowUser(ctx context.Context, follower, followee uuid.UUID) (int, error)
	FollowBrand(ctx context.Context, userID, brandID uuid.UUID) (int, error)
	UnfollowBrand(ctx context.Context, userID, brandID uuid.UUID) (int, error)
	ListFollowingUsers(ctx context.Context, follower uuid.UUID, limit, offset int) ([]domain.FollowingUserItem, int, error)
	ListFollowingBrands(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.FollowingBrandItem, int, error)
}
