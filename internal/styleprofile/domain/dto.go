package domain

import "github.com/google/uuid"

// UpdateStyleProfileRequest is the PUT body. Budget cross-field validation
// (max >= min) is done in the service, not via binding tags, so nil budgets
// are handled cleanly.
type UpdateStyleProfileRequest struct {
	StyleTagIDs []string `json:"style_tag_ids" binding:"omitempty,max=10,dive,uuid"`
	BudgetMin   *int     `json:"budget_min"    binding:"omitempty,gte=0"`
	BudgetMax   *int     `json:"budget_max"    binding:"omitempty,gte=0"`
}

// StyleProfileResponse is the GET/PUT response body.
type StyleProfileResponse struct {
	StyleTags   []StyleTagRef `json:"style_tags"`
	BudgetMin   *int          `json:"budget_min,omitempty"`
	BudgetMax   *int          `json:"budget_max,omitempty"`
	OnboardedAt *string       `json:"onboarded_at,omitempty"`
}

// UpsertParams is the repo input for a profile write.
type UpsertParams struct {
	UserID      uuid.UUID
	StyleTagIDs []uuid.UUID
	BudgetMin   *int
	BudgetMax   *int
}
