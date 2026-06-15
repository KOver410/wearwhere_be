// Package domain holds block DTOs and errors.
package domain

// BlockStatusResponse is returned by block/unblock endpoints.
type BlockStatusResponse struct {
	Blocked bool `json:"blocked"`
}

// BlockedUserItem is one entry in the "users I've blocked" list.
type BlockedUserItem struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	AvatarURL *string `json:"avatar_url,omitempty"`
}

type Pagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

func NewPagination(page, limit, total int) Pagination {
	tp := 0
	if limit > 0 {
		tp = (total + limit - 1) / limit
	}
	return Pagination{Page: page, Limit: limit, Total: total, TotalPages: tp}
}
