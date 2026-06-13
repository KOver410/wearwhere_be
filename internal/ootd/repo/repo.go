// Package repo defines persistence for OOTD posts, likes, and comments.
package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/ootd/domain"
)

var ErrNotFound = errors.New("ootd: not found")

type Repo interface {
	// CreatePost inserts the post (post.ID and post.PhotoURLs pre-set) plus its
	// product tags in one tx.
	CreatePost(ctx context.Context, post *domain.Post, productIDs []uuid.UUID) error
	GetPost(ctx context.Context, id uuid.UUID) (*domain.PostView, error)
	FeedList(ctx context.Context, limit, offset int) ([]*domain.PostView, int, error)
	ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.PostView, int, error)
	UpdateCaption(ctx context.Context, postID uuid.UUID, caption *string) error
	SoftDeletePost(ctx context.Context, postID uuid.UUID) error
	// Like inserts a like (idempotent) and increments like_count only if inserted.
	Like(ctx context.Context, postID, userID uuid.UUID) error
	// Unlike deletes a like (idempotent) and decrements like_count only if deleted.
	Unlike(ctx context.Context, postID, userID uuid.UUID) error
	// LikedPostIDs returns the subset of postIDs the user has liked.
	LikedPostIDs(ctx context.Context, userID uuid.UUID, postIDs []uuid.UUID) (map[uuid.UUID]bool, error)
	// TagsForPosts returns product tags grouped by post id.
	TagsForPosts(ctx context.Context, postIDs []uuid.UUID) (map[uuid.UUID][]domain.ProductTag, error)
	AddComment(ctx context.Context, c *domain.Comment) error
	ListComments(ctx context.Context, postID uuid.UUID, limit, offset int) ([]*domain.CommentView, int, error)
	// CommentOwner returns the comment's author id (ErrNotFound if missing/deleted).
	CommentOwner(ctx context.Context, commentID uuid.UUID) (uuid.UUID, error)
	SoftDeleteComment(ctx context.Context, commentID uuid.UUID) error
}
