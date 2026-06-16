package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/shared/llm"
	spdomain "github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
	wdomain "github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
	wrepo "github.com/wearwhere/wearwhere_be/internal/wardrobe/repo"
	"github.com/wearwhere/wearwhere_be/internal/wardrobe/service"
)

type fakeCloset struct{ items []wdomain.ClosetItem }

func (f *fakeCloset) ClosetItems(_ context.Context, _ uuid.UUID) ([]wdomain.ClosetItem, error) {
	return f.items, nil
}

type fakeSnap struct {
	snap     *wrepo.Snapshot
	upserted bool
}

func (f *fakeSnap) Load(_ context.Context, _ uuid.UUID) (*wrepo.Snapshot, error) {
	if f.snap == nil {
		return nil, wrepo.ErrNoSnapshot
	}
	return f.snap, nil
}
func (f *fakeSnap) Upsert(_ context.Context, _ uuid.UUID, sig string, outfits []wdomain.Outfit, _ string, _, _ int) error {
	f.snap = &wrepo.Snapshot{Signature: sig, Outfits: outfits}
	f.upserted = true
	return nil
}

type fakeProfile struct{ view *spdomain.StyleProfileView }

func (f *fakeProfile) LoadProfile(_ context.Context, _ uuid.UUID) (*spdomain.StyleProfileView, error) {
	return f.view, nil
}

type fakeRetriever struct{ cards []wdomain.OutfitCard }

func (f *fakeRetriever) Retrieve(_ context.Context, _ []string, _, _ *int, _ int) ([]wdomain.OutfitCard, error) {
	return f.cards, nil
}

func cfg() service.Config { return service.Config{MaxOutfits: 5, ToBuyPerOutfit: 2, DayStamp: "20260616"} }

func TestGet_EmptyClosetSuggestsToBuy(t *testing.T) {
	closet := &fakeCloset{}
	snap := &fakeSnap{}
	prof := &fakeProfile{view: &spdomain.StyleProfileView{StyleTags: []spdomain.StyleTagRef{{ID: uuid.New().String(), Slug: "minimal"}}}}
	buy := uuid.New().String()
	ret := &fakeRetriever{cards: []wdomain.OutfitCard{{ID: buy, Name: "Shirt"}, {ID: uuid.New().String(), Name: "Pant"}}}
	// Mock LLM groups the two retrieved items (ids "1","2") into one outfit.
	mock := llm.NewMockClient()

	svc := service.New(closet, snap, prof, ret, mock, cfg())
	resp, err := svc.Get(context.Background(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, "ready", resp.OutfitsStatus)
	require.NotEmpty(t, resp.Outfits)
	require.Empty(t, resp.Outfits[0].Owned, "empty closet → no owned items")
	require.NotEmpty(t, resp.Outfits[0].ToBuy, "empty closet → all to-buy")
	require.Empty(t, resp.Closet)
	require.False(t, resp.OnboardingPrompt, "profile present → no onboarding prompt")
}

func TestGet_NoProfileNoCloset_OnboardingPrompt(t *testing.T) {
	ret := &fakeRetriever{cards: []wdomain.OutfitCard{{ID: uuid.New().String(), Name: "Trend"}, {ID: uuid.New().String(), Name: "Trend2"}}}
	svc := service.New(&fakeCloset{}, &fakeSnap{}, &fakeProfile{}, ret, llm.NewMockClient(), cfg())
	resp, err := svc.Get(context.Background(), uuid.New())
	require.NoError(t, err)
	require.True(t, resp.OnboardingPrompt)
}

func TestGet_ServesCachedSnapshotWhenSignatureMatches(t *testing.T) {
	closet := &fakeCloset{} // empty closet
	prof := &fakeProfile{}
	// Pre-store a snapshot whose signature matches the empty/no-profile/day inputs.
	sig := service.ComputeSignature(nil, nil, "20260616")
	snap := &fakeSnap{snap: &wrepo.Snapshot{Signature: sig, Outfits: []wdomain.Outfit{{Title: "cached"}}}}
	svc := service.New(closet, snap, prof, &fakeRetriever{}, llm.NewMockClient(), cfg())

	resp, err := svc.Get(context.Background(), uuid.New())
	require.NoError(t, err)
	require.Len(t, resp.Outfits, 1)
	require.Equal(t, "cached", resp.Outfits[0].Title)
	require.False(t, snap.upserted, "matching signature must not regenerate")
}

func TestGet_ProviderFailureDegrades(t *testing.T) {
	// closet has an item so closet is returned even when generation fails.
	closet := &fakeCloset{items: []wdomain.ClosetItem{{ProductID: uuid.New(), Name: "Tee"}}}
	failing := &failLLM{}
	svc := service.New(closet, &fakeSnap{}, &fakeProfile{}, &fakeRetriever{}, failing, cfg())
	resp, err := svc.Get(context.Background(), uuid.New())
	require.NoError(t, err, "degrade, not error")
	require.Equal(t, "unavailable", resp.OutfitsStatus)
	require.Empty(t, resp.Outfits)
	require.Len(t, resp.Closet, 1, "closet still returned")
}

type failLLM struct{}

func (failLLM) Generate(_ context.Context, _ llm.GenerateRequest) (*llm.GenerateResponse, error) {
	return nil, llm.ErrUnavailable
}

func TestRegenerate_ForcesUpsert(t *testing.T) {
	sig := service.ComputeSignature(nil, nil, "20260616")
	snap := &fakeSnap{snap: &wrepo.Snapshot{Signature: sig, Outfits: []wdomain.Outfit{{Title: "old"}}}}
	ret := &fakeRetriever{cards: []wdomain.OutfitCard{{ID: uuid.New().String(), Name: "A"}, {ID: uuid.New().String(), Name: "B"}}}
	svc := service.New(&fakeCloset{}, snap, &fakeProfile{}, ret, llm.NewMockClient(), cfg())

	_, err := svc.Regenerate(context.Background(), uuid.New())
	require.NoError(t, err)
	require.True(t, snap.upserted, "regenerate always rewrites the snapshot")
}
