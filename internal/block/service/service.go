// Package service holds block business logic: self-block guard, existence check.
package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/block/domain"
	"github.com/wearwhere/wearwhere_be/internal/block/repo"
)

type Service struct{ repo repo.Repo }

func New(r repo.Repo) *Service { return &Service{repo: r} }

func (s *Service) BlockUser(ctx context.Context, blocker, target uuid.UUID) (*domain.BlockStatusResponse, error) {
	if blocker == target {
		return nil, domain.ErrCannotBlockSelf()
	}
	ok, err := s.repo.UserExists(ctx, target)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrUserNotFound()
	}
	if err := s.repo.Block(ctx, blocker, target); err != nil {
		return nil, err
	}
	return &domain.BlockStatusResponse{Blocked: true}, nil
}

func (s *Service) UnblockUser(ctx context.Context, blocker, target uuid.UUID) (*domain.BlockStatusResponse, error) {
	if err := s.repo.Unblock(ctx, blocker, target); err != nil {
		return nil, err
	}
	return &domain.BlockStatusResponse{Blocked: false}, nil
}

func (s *Service) ListBlocked(ctx context.Context, blocker uuid.UUID, page, limit int) ([]domain.BlockedUserItem, int, error) {
	return s.repo.ListBlocked(ctx, blocker, limit, (page-1)*limit)
}
