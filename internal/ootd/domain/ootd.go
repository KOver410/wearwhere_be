// Package domain holds OOTD entities and DTOs.
package domain

import (
	"time"

	"github.com/google/uuid"
)

type Post struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	Caption      *string
	PhotoURLs    []string
	Status       string
	LikeCount    int
	CommentCount int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Comment struct {
	ID        uuid.UUID
	PostID    uuid.UUID
	UserID    uuid.UUID
	Body      string
	Status    string
	CreatedAt time.Time
}

// ProductTag is a product linked to a post ("shop the look").
type ProductTag struct {
	ProductID uuid.UUID
	Slug      string
	Name      string
}

// PostView is a Post enriched for responses.
type PostView struct {
	Post
	AuthorName string
	Tags       []ProductTag
	LikedByMe  bool
}

// CommentView is a Comment enriched with the author's display name.
type CommentView struct {
	Comment
	AuthorName string
}
