package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
	"github.com/wearwhere/wearwhere_be/internal/styleprofile/repo"
)

type Service struct{ repo repo.StyleProfileRepo }

func New(r repo.StyleProfileRepo) *Service { return &Service{repo: r} }

// Get returns the saved profile, or an empty (zero-value) view when the user
// has never set one. GET never 404s on a missing profile.
func (s *Service) Get(ctx context.Context, userID uuid.UUID) (*domain.StyleProfileView, error) {
	v, err := s.repo.Load(ctx, userID)
	if errors.Is(err, repo.ErrNotFound) {
		return &domain.StyleProfileView{UserID: userID}, nil
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}

// LoadProfile is the in-process getter for other services (recommendation,
// stylist). It returns nil when the user has no profile.
func (s *Service) LoadProfile(ctx context.Context, userID uuid.UUID) (*domain.StyleProfileView, error) {
	v, err := s.repo.Load(ctx, userID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, nil
	}
	return v, err
}

// Save validates and upserts the profile. Idempotent: fully overwrites the
// tag set and budget. Forward note: when UC29 lands, invalidate that user's
// recommendation cache here after a successful upsert.
func (s *Service) Save(ctx context.Context, userID uuid.UUID, req domain.UpdateStyleProfileRequest) (*domain.StyleProfileView, error) {
	if req.BudgetMin != nil && req.BudgetMax != nil && *req.BudgetMax < *req.BudgetMin {
		return nil, domain.ErrInvalidBudget
	}

	ids := make([]uuid.UUID, 0, len(req.StyleTagIDs))
	for _, raw := range req.StyleTagIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, &domain.UnknownStyleTagsError{IDs: []string{raw}}
		}
		ids = append(ids, id)
	}

	if len(ids) > 0 {
		unknown, err := s.repo.UnknownTagIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		if len(unknown) > 0 {
			out := make([]string, len(unknown))
			for i, u := range unknown {
				out[i] = u.String()
			}
			return nil, &domain.UnknownStyleTagsError{IDs: out}
		}
	}

	return s.repo.Upsert(ctx, domain.UpsertParams{
		UserID:      userID,
		StyleTagIDs: ids,
		BudgetMin:   req.BudgetMin,
		BudgetMax:   req.BudgetMax,
	})
}
