package service_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
	"github.com/wearwhere/wearwhere_be/internal/recommendation/service"
)

func intp(v int) *int { return &v }

func cand(id, brand, cat uuid.UUID, price float64, sold int, tags ...uuid.UUID) domain.Candidate {
	return domain.Candidate{
		ProductID: id, BrandID: brand, CategoryID: cat,
		MinPrice: price, SoldCount: sold, CreatedAt: time.Unix(int64(sold), 0),
		StyleTagIDs: tags,
	}
}

func TestScoreCandidate_AllSignals(t *testing.T) {
	tag := uuid.New()
	brand := uuid.New()
	cat := uuid.New()
	sig := domain.UserSignals{
		StyleTagIDs:         map[uuid.UUID]bool{tag: true},
		BudgetMin:           intp(100000),
		BudgetMax:           intp(300000),
		FollowedBrandIDs:    map[uuid.UUID]bool{brand: true},
		AffinityCategoryIDs: map[uuid.UUID]bool{cat: true},
		PurchasedProductIDs: map[uuid.UUID]bool{},
	}
	c := cand(uuid.New(), brand, cat, 200000, 1, tag)
	require.Equal(t, 26, service.ScoreCandidate(c, sig)) // 10+8+5+3
}

func TestScoreCandidate_BudgetOutPenalty(t *testing.T) {
	sig := domain.UserSignals{
		StyleTagIDs: map[uuid.UUID]bool{}, FollowedBrandIDs: map[uuid.UUID]bool{},
		AffinityCategoryIDs: map[uuid.UUID]bool{}, PurchasedProductIDs: map[uuid.UUID]bool{},
		BudgetMin: intp(100000), BudgetMax: intp(200000),
	}
	c := cand(uuid.New(), uuid.New(), uuid.New(), 500000, 1)
	require.Equal(t, -3, service.ScoreCandidate(c, sig))
}

func TestScoreCandidate_NoBudgetNoBudgetScore(t *testing.T) {
	sig := domain.UserSignals{
		StyleTagIDs: map[uuid.UUID]bool{}, FollowedBrandIDs: map[uuid.UUID]bool{},
		AffinityCategoryIDs: map[uuid.UUID]bool{}, PurchasedProductIDs: map[uuid.UUID]bool{},
	}
	c := cand(uuid.New(), uuid.New(), uuid.New(), 500000, 1)
	require.Equal(t, 0, service.ScoreCandidate(c, sig))
}

func TestRank_ExcludesPurchasedAndIsDeterministic(t *testing.T) {
	tag := uuid.New()
	brand := uuid.New()
	cat := uuid.New()
	bought := uuid.New()
	sig := domain.UserSignals{
		StyleTagIDs:         map[uuid.UUID]bool{tag: true},
		FollowedBrandIDs:    map[uuid.UUID]bool{brand: true},
		AffinityCategoryIDs: map[uuid.UUID]bool{},
		PurchasedProductIDs: map[uuid.UUID]bool{bought: true},
	}
	matching := cand(uuid.New(), brand, cat, 100000, 5, tag)
	purchased := cand(bought, brand, cat, 100000, 9, tag)
	cands := []domain.Candidate{purchased, matching}

	out := service.Rank(cands, sig, 10)
	ids := map[uuid.UUID]bool{}
	for _, c := range out {
		ids[c.ProductID] = true
	}
	require.False(t, ids[bought], "purchased product must be excluded")
	require.True(t, ids[matching.ProductID])

	out2 := service.Rank(cands, sig, 10)
	require.Equal(t, out, out2)
}

func TestRank_IncludesDiscoverySlice(t *testing.T) {
	tag := uuid.New()
	brand := uuid.New()
	cat := uuid.New()
	sig := domain.UserSignals{
		StyleTagIDs:         map[uuid.UUID]bool{tag: true},
		FollowedBrandIDs:    map[uuid.UUID]bool{brand: true},
		AffinityCategoryIDs: map[uuid.UUID]bool{},
		PurchasedProductIDs: map[uuid.UUID]bool{},
	}
	var cands []domain.Candidate
	for i := 0; i < 8; i++ {
		cands = append(cands, cand(uuid.New(), brand, cat, 100000, 100-i, tag))
	}
	otherBrand := uuid.New()
	for i := 0; i < 4; i++ {
		cands = append(cands, cand(uuid.New(), otherBrand, cat, 100000, 50-i))
	}
	out := service.Rank(cands, sig, 10)
	require.Len(t, out, 10)
	var discovery int
	for _, c := range out {
		if c.BrandID == otherBrand {
			discovery++
		}
	}
	require.GreaterOrEqual(t, discovery, 1, "discovery slice must surface unexplored items")
}
