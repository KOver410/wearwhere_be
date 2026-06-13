// Package service holds product-review business logic: validation, the
// verified-purchase gate, ownership checks, and write orchestration.
package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/review/domain"
	"github.com/wearwhere/wearwhere_be/internal/review/repo"
)

type Service struct {
	repo repo.Repo
}

func NewWithRepo(r repo.Repo) *Service { return &Service{repo: r} }

func fitPtr(fit string) *string {
	if strings.TrimSpace(fit) == "" {
		return nil
	}
	return &fit
}

func (s *Service) Create(ctx context.Context, userID, productID uuid.UUID, req *domain.WriteReviewRequest) (*domain.Review, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	exists, err := s.repo.ProductExists(ctx, productID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, domain.ErrProductNotFound()
	}
	ok, err := s.repo.HasDeliveredPurchase(ctx, userID, productID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrNotVerifiedPurchase()
	}
	rv := &domain.Review{ProductID: productID, UserID: userID, Rating: req.Rating, Body: req.Body, Fit: fitPtr(req.Fit)}
	if err := s.repo.Create(ctx, rv); err != nil {
		if errors.Is(err, repo.ErrDuplicate) {
			return nil, domain.ErrReviewExists()
		}
		return nil, err
	}
	return rv, nil
}

func (s *Service) List(ctx context.Context, productID uuid.UUID, q *domain.ListReviewsQuery) (*domain.ListReviewsResponse, error) {
	exists, err := s.repo.ProductExists(ctx, productID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, domain.ErrProductNotFound()
	}
	f := repo.ListFilter{Rating: q.Rating, Fit: q.Fit, Sort: q.Sort, Limit: q.Limit, Offset: (q.Page - 1) * q.Limit}
	views, total, err := s.repo.ListByProduct(ctx, productID, f)
	if err != nil {
		return nil, err
	}
	agg, err := s.repo.Aggregate(ctx, productID)
	if err != nil {
		return nil, err
	}
	items := make([]domain.ReviewResponse, 0, len(views))
	for _, v := range views {
		items = append(items, domain.ToReviewResponse(v))
	}
	return &domain.ListReviewsResponse{
		Items:       items,
		AvgRating:   agg.AvgRating,
		ReviewCount: agg.ReviewCount,
		Pagination:  domain.NewPagination(q.Page, q.Limit, total),
	}, nil
}

func (s *Service) Update(ctx context.Context, userID, reviewID uuid.UUID, req *domain.WriteReviewRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}
	rv, err := s.repo.GetByID(ctx, reviewID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.ErrReviewNotFound()
		}
		return err
	}
	if rv.UserID != userID {
		return domain.ErrForbidden()
	}
	if err := s.repo.Update(ctx, reviewID, req.Rating, req.Body, fitPtr(req.Fit)); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.ErrReviewNotFound()
		}
		return err
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, userID, reviewID uuid.UUID) error {
	rv, err := s.repo.GetByID(ctx, reviewID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.ErrReviewNotFound()
		}
		return err
	}
	if rv.UserID != userID {
		return domain.ErrForbidden()
	}
	if err := s.repo.SoftDelete(ctx, reviewID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.ErrReviewNotFound()
		}
		return err
	}
	return nil
}
