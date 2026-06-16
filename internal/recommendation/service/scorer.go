package service

import (
	"sort"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/recommendation/domain"
)

const (
	weightStyleTag  = 10
	weightBrand     = 8
	weightBudgetIn  = 5
	weightBudgetOut = -3
	weightCategory  = 3
)

// ScoreCandidate computes the heuristic score of one candidate for one user.
func ScoreCandidate(c domain.Candidate, sig domain.UserSignals) int {
	score := 0
	for _, tid := range c.StyleTagIDs {
		if sig.StyleTagIDs[tid] {
			score += weightStyleTag
		}
	}
	if sig.FollowedBrandIDs[c.BrandID] {
		score += weightBrand
	}
	if sig.BudgetMin != nil || sig.BudgetMax != nil {
		if inBudget(c.MinPrice, sig.BudgetMin, sig.BudgetMax) {
			score += weightBudgetIn
		} else {
			score += weightBudgetOut
		}
	}
	if sig.AffinityCategoryIDs[c.CategoryID] {
		score += weightCategory
	}
	return score
}

func inBudget(price float64, min, max *int) bool {
	if min != nil && price < float64(*min) {
		return false
	}
	if max != nil && price > float64(*max) {
		return false
	}
	return true
}

// explored reports whether the user already has affinity for this candidate
// (a matching style tag or a followed brand). Unexplored candidates feed the
// discovery slice.
func explored(c domain.Candidate, sig domain.UserSignals) bool {
	if sig.FollowedBrandIDs[c.BrandID] {
		return true
	}
	for _, tid := range c.StyleTagIDs {
		if sig.StyleTagIDs[tid] {
			return true
		}
	}
	return false
}

// Rank excludes purchased products, scores the rest, and returns up to `limit`
// products: ~70% by score with a ~30% discovery slice of unexplored items
// (round-robin by brand for diversity). Fully deterministic.
func Rank(cands []domain.Candidate, sig domain.UserSignals, limit int) []domain.Candidate {
	pool := make([]domain.Candidate, 0, len(cands))
	for _, c := range cands {
		if !sig.PurchasedProductIDs[c.ProductID] {
			pool = append(pool, c)
		}
	}

	scores := make(map[uuid.UUID]int, len(pool))
	for _, c := range pool {
		scores[c.ProductID] = ScoreCandidate(c, sig)
	}
	sort.SliceStable(pool, func(i, j int) bool {
		a, b := pool[i], pool[j]
		if scores[a.ProductID] != scores[b.ProductID] {
			return scores[a.ProductID] > scores[b.ProductID]
		}
		if a.SoldCount != b.SoldCount {
			return a.SoldCount > b.SoldCount
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.After(b.CreatedAt)
		}
		return a.ProductID.String() < b.ProductID.String()
	})

	if limit <= 0 || len(pool) <= limit {
		return pool
	}

	discoveryTarget := limit * 3 / 10
	topTarget := limit - discoveryTarget

	chosen := make([]domain.Candidate, 0, limit)
	used := make(map[uuid.UUID]bool, limit)
	for _, c := range pool {
		if len(chosen) >= topTarget {
			break
		}
		chosen = append(chosen, c)
		used[c.ProductID] = true
	}

	if discoveryTarget > 0 {
		var disc []domain.Candidate
		for _, c := range pool {
			if !used[c.ProductID] && !explored(c, sig) {
				disc = append(disc, c)
			}
		}
		for _, c := range roundRobinByBrand(disc, discoveryTarget) {
			chosen = append(chosen, c)
			used[c.ProductID] = true
		}
	}

	for _, c := range pool {
		if len(chosen) >= limit {
			break
		}
		if !used[c.ProductID] {
			chosen = append(chosen, c)
			used[c.ProductID] = true
		}
	}
	return chosen
}

// roundRobinByBrand returns up to n items, taking at most one item per brand
// per pass (in the input order, already score-sorted) so the slice is
// brand-diverse and deterministic.
func roundRobinByBrand(in []domain.Candidate, n int) []domain.Candidate {
	if n <= 0 || len(in) == 0 {
		return nil
	}
	out := make([]domain.Candidate, 0, n)
	taken := make(map[uuid.UUID]bool, len(in))
	for len(out) < n {
		seenBrand := make(map[uuid.UUID]bool)
		progressed := false
		for _, c := range in {
			if taken[c.ProductID] || seenBrand[c.BrandID] {
				continue
			}
			out = append(out, c)
			taken[c.ProductID] = true
			seenBrand[c.BrandID] = true
			progressed = true
			if len(out) >= n {
				return out
			}
		}
		if !progressed {
			break
		}
	}
	return out
}
