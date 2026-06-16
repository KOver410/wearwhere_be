package domain

import (
	"time"

	"github.com/google/uuid"
)

// StyleTagRef is the public shape of a style tag (id/slug/name), matching the
// product catalog's StyleTagRef fields.
type StyleTagRef struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// StyleProfileView is the assembled profile returned to callers and consumed
// in-process by the recommendation service. A user with no saved profile is
// represented by a zero-value view (empty StyleTags, nil budgets, nil OnboardedAt).
type StyleProfileView struct {
	UserID      uuid.UUID
	StyleTags   []StyleTagRef
	BudgetMin   *int
	BudgetMax   *int
	OnboardedAt *time.Time
}
