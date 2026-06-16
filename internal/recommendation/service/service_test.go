package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
	"github.com/wearwhere/wearwhere_be/internal/recommendation/service"
	spdomain "github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
)

type fakeCandidates struct{ list []domain.Candidate }

func (f *fakeCandidates) Candidates(_ context.Context, _ int) ([]domain.Candidate, error) {
	return f.list, nil
}

type fakeSignals struct {
	brands []uuid.UUID
	bought []uuid.UUID
	cats   []uuid.UUID
}

func (f *fakeSignals) FollowedBrandIDs(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return f.brands, nil
}
func (f *fakeSignals) PurchasedProductIDs(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return f.bought, nil
}
func (f *fakeSignals) AffinityCategoryIDs(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return f.cats, nil
}

type fakeProfile struct{ view *spdomain.StyleProfileView }

func (f *fakeProfile) LoadProfile(_ context.Context, _ uuid.UUID) (*spdomain.StyleProfileView, error) {
	return f.view, nil
}

type fakeCache struct {
	stored *domain.RecommendationsResponse
}

func (f *fakeCache) Get(_ context.Context, _ uuid.UUID) (*domain.RecommendationsResponse, bool, error) {
	return f.stored, f.stored != nil, nil
}
func (f *fakeCache) Set(_ context.Context, _ uuid.UUID, r *domain.RecommendationsResponse) error {
	f.stored = r
	return nil
}
func (f *fakeCache) Invalidate(_ context.Context, _ uuid.UUID) error { f.stored = nil; return nil }

func newCand(brand, cat uuid.UUID, tags ...uuid.UUID) domain.Candidate {
	return domain.Candidate{ProductID: uuid.New(), BrandID: brand, CategoryID: cat, MinPrice: 100000, StyleTagIDs: tags}
}

func cfg() service.Config { return service.Config{DefaultLimit: 20, MaxLimit: 50, CandidatePool: 300} }

func TestRecommend_ColdStartTrending(t *testing.T) {
	cands := []domain.Candidate{newCand(uuid.New(), uuid.New()), newCand(uuid.New(), uuid.New())}
	svc := service.New(&fakeCandidates{list: cands}, &fakeSignals{}, &fakeProfile{}, &fakeCache{}, cfg())

	resp, err := svc.Recommend(context.Background(), uuid.New(), 0)
	require.NoError(t, err)
	require.Equal(t, "trending", resp.Source)
	require.True(t, resp.OnboardingPrompt)
	require.Len(t, resp.Items, 2)
}

func TestRecommend_PersonalizedWhenProfilePresent(t *testing.T) {
	tag := uuid.New()
	brand := uuid.New()
	cat := uuid.New()
	cands := []domain.Candidate{newCand(brand, cat, tag), newCand(uuid.New(), cat)}
	prof := &fakeProfile{view: &spdomain.StyleProfileView{
		StyleTags: []spdomain.StyleTagRef{{ID: tag.String()}},
	}}
	svc := service.New(&fakeCandidates{list: cands}, &fakeSignals{}, prof, &fakeCache{}, cfg())

	resp, err := svc.Recommend(context.Background(), uuid.New(), 0)
	require.NoError(t, err)
	require.Equal(t, "personalized", resp.Source)
	require.False(t, resp.OnboardingPrompt)
	require.NotEmpty(t, resp.Items)
}

func TestRecommend_UsesCacheOnSecondCall(t *testing.T) {
	cache := &fakeCache{}
	cands := []domain.Candidate{newCand(uuid.New(), uuid.New())}
	prof := &fakeProfile{view: &spdomain.StyleProfileView{}}
	sig := &fakeSignals{brands: []uuid.UUID{uuid.New()}}
	svc := service.New(&fakeCandidates{list: cands}, sig, prof, cache, cfg())

	r1, err := svc.Recommend(context.Background(), uuid.New(), 0)
	require.NoError(t, err)
	require.NotNil(t, cache.stored, "first call must populate cache")
	r2, err := svc.Recommend(context.Background(), uuid.New(), 0)
	require.NoError(t, err)
	require.Equal(t, r1.Source, r2.Source)
}

func TestRecommend_ClampsLimit(t *testing.T) {
	svc := service.New(&fakeCandidates{}, &fakeSignals{}, &fakeProfile{}, &fakeCache{}, cfg())
	require.Equal(t, 20, svc.ResolveLimit(0))
	require.Equal(t, 50, svc.ResolveLimit(1000))
	require.Equal(t, 15, svc.ResolveLimit(15))
}
