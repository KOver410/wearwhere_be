// Package domain holds the read DTOs and query filter for the admin
// user-listing endpoint (UC54 — list).
package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100

	SortCreatedAt = "created_at"
	SortLastLogin = "last_login_at"
	OrderAsc      = "asc"
	OrderDesc     = "desc"
)

// ListUsersFilter holds the query parameters for GET /admin/users.
type ListUsersFilter struct {
	Q        string
	Sort     string
	Order    string
	Page     int
	PageSize int
}

// Normalized returns a copy with defaults applied and values clamped to the
// allowed ranges/whitelists, safe to hand to the repo.
func (f ListUsersFilter) Normalized() ListUsersFilter {
	n := f
	n.Q = strings.TrimSpace(n.Q)
	switch n.Sort {
	case SortCreatedAt, SortLastLogin:
	default:
		n.Sort = SortCreatedAt
	}
	switch n.Order {
	case OrderAsc, OrderDesc:
	default:
		n.Order = OrderDesc
	}
	if n.Page < 1 {
		n.Page = 1
	}
	if n.PageSize < 1 {
		n.PageSize = defaultPageSize
	}
	if n.PageSize > maxPageSize {
		n.PageSize = maxPageSize
	}
	return n
}

// AdminUserRow is the repo-level read model scanned from the users table.
type AdminUserRow struct {
	ID              uuid.UUID
	Email           *string
	Phone           *string
	Name            string
	Role            string
	Status          string
	EmailVerifiedAt *time.Time
	PhoneVerifiedAt *time.Time
	AvatarURL       *string
	LastLoginAt     *time.Time
	CreatedAt       time.Time
}

// AdminUserResp is the wire representation of a user in the admin list.
type AdminUserResp struct {
	ID            string     `json:"id"`
	Email         *string    `json:"email"`
	Phone         *string    `json:"phone"`
	Name          string     `json:"name"`
	Role          string     `json:"role"`
	Status        string     `json:"status"`
	EmailVerified bool       `json:"email_verified"`
	PhoneVerified bool       `json:"phone_verified"`
	AvatarURL     *string    `json:"avatar_url"`
	LastLoginAt   *time.Time `json:"last_login_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

// AdminUserListResp is a paginated list of users.
type AdminUserListResp struct {
	Data       []AdminUserResp `json:"data"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	Total      int             `json:"total"`
	TotalPages int             `json:"total_pages"`
}

// ToResp maps a repo row to its wire DTO.
func ToResp(r AdminUserRow) AdminUserResp {
	return AdminUserResp{
		ID:            r.ID.String(),
		Email:         r.Email,
		Phone:         r.Phone,
		Name:          r.Name,
		Role:          r.Role,
		Status:        r.Status,
		EmailVerified: r.EmailVerifiedAt != nil,
		PhoneVerified: r.PhoneVerifiedAt != nil,
		AvatarURL:     r.AvatarURL,
		LastLoginAt:   r.LastLoginAt,
		CreatedAt:     r.CreatedAt,
	}
}
