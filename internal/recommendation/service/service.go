package service

import (
	"context"
	"log"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
	"github.com/wearwhere/wearwhere_be/internal/recommendation/repo"
	spdomain "github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
)

// Config holds the feed tunables (mirrors config.RecommendationConfig).
type Config struct {
	DefaultLimit  int
	MaxLimit      int
	CandidatePool int
}

// ProfileLoader is the in-process style-profile reader (satisfied by
// styleprofile/service.Service.LoadProfile). Returns nil when no profile.
type ProfileLoader interface {
	LoadProfile(ctx context.Context, userID uuid.UUID) (*spdomain.StyleProfileView, error)
}

type Service struct {
	candidates repo.CandidateRepo
	signals    repo.SignalRepo
	profiles   ProfileLoader
	cache      Cache
	cfg        Config
}

func New(c repo.CandidateRepo, s repo.SignalRepo, p ProfileLoader, cache Cache, cfg Config) *Service {
	return &Service{candidates: c, signals: s, profiles: p, cache: cache, cfg: cfg}
}

// ResolveLimit clamps the requested limit into [1, MaxLimit], defaulting when <= 0.
func (s *Service) ResolveLimit(requested int) int {
	if requested <= 0 {
		return s.cfg.DefaultLimit
	}
	if requested > s.cfg.MaxLimit {
		return s.cfg.MaxLimit
	}
	return requested
}

// Invalidate busts the user's cached feed (called on profile/order change).
func (s *Service) Invalidate(ctx context.Context, userID uuid.UUID) error {
	return s.cache.Invalidate(ctx, userID)
}

func (s *Service) Recommend(ctx context.Context, userID uuid.UUID, requestedLimit int) (*domain.RecommendationsResponse, error) {
	limit := s.ResolveLimit(requestedLimit)

	if cached, ok, err := s.cache.Get(ctx, userID); err == nil && ok {
		return cached, nil
	} else if err != nil {
		log.Printf("recommendation: cache get failed for %s: %v", userID, err)
	}

	sig, err := s.loadSignals(ctx, userID)
	if err != nil {
		return nil, err
	}

	pool, err := s.candidates.Candidates(ctx, s.cfg.CandidatePool)
	if err != nil {
		return nil, err
	}
	if len(pool) == s.cfg.CandidatePool {
		log.Printf("recommendation: candidate pool capped at %d for %s; some products not scored", s.cfg.CandidatePool, userID)
	}

	var resp domain.RecommendationsResponse
	if sig.HasProfile() || sig.HasHistory() {
		ranked := Rank(pool, sig, limit)
		resp = domain.RecommendationsResponse{
			Items:            toCards(ranked),
			Source:           "personalized",
			OnboardingPrompt: false,
		}
	} else {
		trending := topN(pool, sig, limit)
		resp = domain.RecommendationsResponse{
			Items:            toCards(trending),
			Source:           "trending",
			OnboardingPrompt: true,
		}
	}

	if err := s.cache.Set(ctx, userID, &resp); err != nil {
		log.Printf("recommendation: cache set failed for %s: %v", userID, err)
	}
	return &resp, nil
}

func (s *Service) loadSignals(ctx context.Context, userID uuid.UUID) (domain.UserSignals, error) {
	sig := domain.UserSignals{
		StyleTagIDs:         map[uuid.UUID]bool{},
		FollowedBrandIDs:    map[uuid.UUID]bool{},
		PurchasedProductIDs: map[uuid.UUID]bool{},
		AffinityCategoryIDs: map[uuid.UUID]bool{},
	}

	prof, err := s.profiles.LoadProfile(ctx, userID)
	if err != nil {
		return sig, err
	}
	if prof != nil {
		sig.BudgetMin = prof.BudgetMin
		sig.BudgetMax = prof.BudgetMax
		for _, t := range prof.StyleTags {
			if id, err := uuid.Parse(t.ID); err == nil {
				sig.StyleTagIDs[id] = true
			}
		}
	}

	brands, err := s.signals.FollowedBrandIDs(ctx, userID)
	if err != nil {
		return sig, err
	}
	for _, b := range brands {
		sig.FollowedBrandIDs[b] = true
	}

	bought, err := s.signals.PurchasedProductIDs(ctx, userID)
	if err != nil {
		return sig, err
	}
	for _, p := range bought {
		sig.PurchasedProductIDs[p] = true
	}

	cats, err := s.signals.AffinityCategoryIDs(ctx, userID)
	if err != nil {
		return sig, err
	}
	for _, c := range cats {
		sig.AffinityCategoryIDs[c] = true
	}
	return sig, nil
}

// topN returns the first `limit` non-purchased candidates (pool is already
// sold_count-ordered), used for the cold-start trending feed.
func topN(pool []domain.Candidate, sig domain.UserSignals, limit int) []domain.Candidate {
	out := make([]domain.Candidate, 0, limit)
	for _, c := range pool {
		if sig.PurchasedProductIDs[c.ProductID] {
			continue
		}
		out = append(out, c)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func toCards(cands []domain.Candidate) []domain.RecProductCard {
	out := make([]domain.RecProductCard, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.ToCard())
	}
	return out
}
