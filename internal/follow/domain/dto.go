// Package domain holds follow DTOs and errors.
package domain

// FollowStatusResponse is returned by follow/unfollow endpoints.
type FollowStatusResponse struct {
	Following     bool `json:"following"`
	FollowerCount int  `json:"follower_count"`
}

type FollowingUserItem struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	AvatarURL     *string `json:"avatar_url,omitempty"`
	FollowerCount int     `json:"follower_count"`
}

type FollowingBrandItem struct {
	ID            string  `json:"id"`
	Slug          string  `json:"slug"`
	Name          string  `json:"name"`
	LogoURL       *string `json:"logo_url,omitempty"`
	FollowerCount int     `json:"follower_count"`
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
