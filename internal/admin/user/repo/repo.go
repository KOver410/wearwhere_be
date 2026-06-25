// Package repo defines the read-only persistence port for admin user listing.
package repo

import (
	"context"

	"github.com/wearwhere/wearwhere_be/internal/admin/user/domain"
)

// ReadRepo loads users for the admin list endpoint.
type ReadRepo interface {
	ListUsers(ctx context.Context, f domain.ListUsersFilter) (items []domain.AdminUserRow, total int, err error)
}
