// Package service implements promo-code validation, redemption, and admin CRUD.
package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/promo/domain"
	"github.com/wearwhere/wearwhere_be/internal/promo/repo"
)

// Service holds the promo repo and a clock (overridable in tests).
type Service struct {
	repo repo.PromoRepo
	now  func() time.Time
}

func New(r repo.PromoRepo) *Service {
	return &Service{repo: r, now: time.Now}
}

// normalize trims and upper-cases a code (storage is CITEXT, so case-insensitive;
// we still trim whitespace from user input).
func normalize(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// check applies the eligibility rules and returns the discount or an error.
func (s *Service) check(p *domain.PromoCode, subtotalVND int64, redeemed bool) (int64, error) {
	if p == nil || !p.IsActive {
		return 0, domain.ErrPromoNotFound
	}
	now := s.now()
	if now.Before(p.StartsAt) {
		return 0, domain.ErrPromoNotStarted
	}
	if p.EndsAt != nil && now.After(*p.EndsAt) {
		return 0, domain.ErrPromoExpired
	}
	if subtotalVND < p.MinOrderValueVND {
		return 0, domain.ErrPromoMinOrder
	}
	if redeemed {
		return 0, domain.ErrPromoAlreadyUsed
	}
	d := p.ComputeDiscount(subtotalVND)
	if d <= 0 {
		return 0, domain.ErrPromoNotApplicable
	}
	return d, nil
}

func result(p *domain.PromoCode, discount int64) *domain.ValidateResult {
	return &domain.ValidateResult{
		PromoID:       p.ID,
		Code:          p.Code,
		DiscountType:  p.DiscountType,
		DiscountValue: p.DiscountValue,
		DiscountVND:   discount,
	}
}

// Validate is the read-only check used by checkout preview and FE pre-checks.
// An empty code returns (nil, nil) — "no promo applied" is not an error.
func (s *Service) Validate(ctx context.Context, code string, userID uuid.UUID, subtotalVND int64) (*domain.ValidateResult, error) {
	code = normalize(code)
	if code == "" {
		return nil, nil
	}
	p, err := s.repo.GetActiveByCode(ctx, nil, code)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, domain.ErrPromoNotFound
		}
		return nil, err
	}
	redeemed, err := s.repo.HasRedeemed(ctx, nil, p.ID, userID)
	if err != nil {
		return nil, err
	}
	d, err := s.check(p, subtotalVND, redeemed)
	if err != nil {
		return nil, err
	}
	return result(p, d), nil
}

// ValidateTx re-validates inside the order-placement tx. It row-locks the promo
// code (FOR UPDATE) so a concurrent placement cannot redeem it twice. Returns
// (uuid.Nil, 0, nil) for an empty code.
func (s *Service) ValidateTx(ctx context.Context, db repo.DBTX, code string, userID uuid.UUID, subtotalVND int64) (uuid.UUID, int64, error) {
	code = normalize(code)
	if code == "" {
		return uuid.Nil, 0, nil
	}
	p, err := s.repo.GetActiveByCodeForUpdate(ctx, db, code)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return uuid.Nil, 0, domain.ErrPromoNotFound
		}
		return uuid.Nil, 0, err
	}
	redeemed, err := s.repo.HasRedeemed(ctx, db, p.ID, userID)
	if err != nil {
		return uuid.Nil, 0, err
	}
	d, err := s.check(p, subtotalVND, redeemed)
	if err != nil {
		return uuid.Nil, 0, err
	}
	return p.ID, d, nil
}

// RedeemTx records the redemption inside the order-placement tx. A unique
// violation (race: same user, same code, two concurrent orders) surfaces as
// ErrPromoAlreadyUsed.
func (s *Service) RedeemTx(ctx context.Context, db repo.DBTX, promoID, userID, orderID uuid.UUID, discountVND int64) error {
	if promoID == uuid.Nil {
		return nil
	}
	err := s.repo.InsertRedemption(ctx, db, promoID, userID, orderID, discountVND)
	if errors.Is(err, repo.ErrAlreadyRedeemed) {
		return domain.ErrPromoAlreadyUsed
	}
	return err
}

// ---------------------------------------------------------------------------
// Admin operations
// ---------------------------------------------------------------------------

// CreateCode validates input and persists a new promo code.
func (s *Service) CreateCode(ctx context.Context, req domain.CreatePromoReq) (*domain.PromoCode, error) {
	code := normalize(req.Code)
	if code == "" || !req.DiscountType.Valid() || req.DiscountValue <= 0 {
		return nil, domain.ErrInvalidPromo
	}
	if req.DiscountType == domain.DiscountTypePercentage && (req.DiscountValue < 1 || req.DiscountValue > 100) {
		return nil, domain.ErrInvalidPromo
	}
	if req.MaxDiscountVND != nil && *req.MaxDiscountVND <= 0 {
		return nil, domain.ErrInvalidPromo
	}
	if req.MinOrderValueVND < 0 {
		return nil, domain.ErrInvalidPromo
	}

	starts := s.now()
	if req.StartsAt != nil {
		starts = *req.StartsAt
	}
	if req.EndsAt != nil && !req.EndsAt.After(starts) {
		return nil, domain.ErrInvalidPromo
	}
	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}

	p := &domain.PromoCode{
		Code:             code,
		Description:      req.Description,
		DiscountType:     req.DiscountType,
		DiscountValue:    req.DiscountValue,
		MaxDiscountVND:   req.MaxDiscountVND,
		MinOrderValueVND: req.MinOrderValueVND,
		StartsAt:         starts,
		EndsAt:           req.EndsAt,
		IsActive:         active,
	}
	if err := s.repo.Create(ctx, p); err != nil {
		if errors.Is(err, repo.ErrCodeConflict) {
			return nil, domain.ErrPromoCodeExists
		}
		return nil, err
	}
	return p, nil
}

// UpdateCode patches mutable fields of an existing code.
func (s *Service) UpdateCode(ctx context.Context, id uuid.UUID, req domain.UpdatePromoReq) (*domain.PromoCode, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, domain.ErrPromoNotFound
		}
		return nil, err
	}
	if req.Description != nil {
		p.Description = *req.Description
	}
	if req.MaxDiscountVND != nil {
		if *req.MaxDiscountVND <= 0 {
			return nil, domain.ErrInvalidPromo
		}
		p.MaxDiscountVND = req.MaxDiscountVND
	}
	if req.MinOrderValueVND != nil {
		if *req.MinOrderValueVND < 0 {
			return nil, domain.ErrInvalidPromo
		}
		p.MinOrderValueVND = *req.MinOrderValueVND
	}
	if req.StartsAt != nil {
		p.StartsAt = *req.StartsAt
	}
	if req.EndsAt != nil {
		p.EndsAt = req.EndsAt
	}
	if req.EndsAt != nil && !req.EndsAt.After(p.StartsAt) {
		return nil, domain.ErrInvalidPromo
	}
	if req.IsActive != nil {
		p.IsActive = *req.IsActive
	}
	if err := s.repo.Update(ctx, p); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, domain.ErrPromoNotFound
		}
		return nil, err
	}
	return p, nil
}

// GetCode returns one promo code by ID.
func (s *Service) GetCode(ctx context.Context, id uuid.UUID) (*domain.PromoCode, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, domain.ErrPromoNotFound
		}
		return nil, err
	}
	return p, nil
}

// ListCodes returns a paginated list of promo codes.
func (s *Service) ListCodes(ctx context.Context, page, pageSize int, activeOnly bool) ([]*domain.PromoCode, int, error) {
	return s.repo.List(ctx, page, pageSize, activeOnly)
}
