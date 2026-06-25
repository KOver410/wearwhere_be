// Package service implements the admin user-listing use case (UC54 — list).
package service

import (
	"context"

	"github.com/wearwhere/wearwhere_be/internal/admin/user/domain"
	"github.com/wearwhere/wearwhere_be/internal/admin/user/repo"
)

type Service struct{ repo repo.ReadRepo }

func New(r repo.ReadRepo) *Service { return &Service{repo: r} }

// ListUsers normalizes the filter, queries the repo, and maps rows to the
// paginated wire response.
func (s *Service) ListUsers(ctx context.Context, raw domain.ListUsersFilter) (domain.AdminUserListResp, error) {
	f := raw.Normalized()
	rows, total, err := s.repo.ListUsers(ctx, f)
	if err != nil {
		return domain.AdminUserListResp{}, err
	}
	data := make([]domain.AdminUserResp, 0, len(rows))
	for _, r := range rows {
		data = append(data, domain.ToResp(r))
	}
	resp := domain.AdminUserListResp{
		Data:     data,
		Page:     f.Page,
		PageSize: f.PageSize,
		Total:    total,
	}
	if f.PageSize > 0 {
		resp.TotalPages = (total + f.PageSize - 1) / f.PageSize
	}
	return resp, nil
}
