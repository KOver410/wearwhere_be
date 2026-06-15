// Package service holds follow business logic: self-follow guard, existence checks.
package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/follow/domain"
	"github.com/wearwhere/wearwhere_be/internal/follow/repo"
)

type Service struct{ repo repo.Repo }

func New(r repo.Repo) *Service { return &Service{repo: r} }

func (s *Service) FollowUser(ctx context.Context, follower, followee uuid.UUID) (*domain.FollowStatusResponse, error) {
	if follower == followee {
		return nil, domain.ErrCannotFollowSelf()
	}
	ok, err := s.repo.UserExists(ctx, followee)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrUserNotFound()
	}
	count, err := s.repo.FollowUser(ctx, follower, followee)
	if err != nil {
		return nil, err
	}
	return &domain.FollowStatusResponse{Following: true, FollowerCount: count}, nil
}

func (s *Service) UnfollowUser(ctx context.Context, follower, followee uuid.UUID) (*domain.FollowStatusResponse, error) {
	count, err := s.repo.UnfollowUser(ctx, follower, followee)
	if err != nil {
		return nil, err
	}
	return &domain.FollowStatusResponse{Following: false, FollowerCount: count}, nil
}

func (s *Service) FollowBrand(ctx context.Context, userID, brandID uuid.UUID) (*domain.FollowStatusResponse, error) {
	ok, err := s.repo.BrandExists(ctx, brandID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrBrandNotFound()
	}
	count, err := s.repo.FollowBrand(ctx, userID, brandID)
	if err != nil {
		return nil, err
	}
	return &domain.FollowStatusResponse{Following: true, FollowerCount: count}, nil
}

func (s *Service) UnfollowBrand(ctx context.Context, userID, brandID uuid.UUID) (*domain.FollowStatusResponse, error) {
	count, err := s.repo.UnfollowBrand(ctx, userID, brandID)
	if err != nil {
		return nil, err
	}
	return &domain.FollowStatusResponse{Following: false, FollowerCount: count}, nil
}

func (s *Service) ListFollowingUsers(ctx context.Context, follower uuid.UUID, page, limit int) ([]domain.FollowingUserItem, int, error) {
	return s.repo.ListFollowingUsers(ctx, follower, limit, (page-1)*limit)
}

func (s *Service) ListFollowingBrands(ctx context.Context, userID uuid.UUID, page, limit int) ([]domain.FollowingBrandItem, int, error) {
	return s.repo.ListFollowingBrands(ctx, userID, limit, (page-1)*limit)
}
