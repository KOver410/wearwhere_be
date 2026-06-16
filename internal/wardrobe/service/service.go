package service

import (
	"context"
	"errors"
	"strconv"

	"github.com/google/uuid"

	"github.com/wearwhere/wearwhere_be/internal/shared/llm"
	spdomain "github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
	wdomain "github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
	wrepo "github.com/wearwhere/wearwhere_be/internal/wardrobe/repo"
)

// Config holds wardrobe tunables.
type Config struct {
	MaxOutfits     int
	ToBuyPerOutfit int
	DayStamp       string // UTC yyyymmdd; injected so the service stays testable
}

// ProfileLoader reads the style profile (styleprofile/service.Service).
type ProfileLoader interface {
	LoadProfile(ctx context.Context, userID uuid.UUID) (*spdomain.StyleProfileView, error)
}

type Service struct {
	closet    wrepo.ClosetRepo
	snapshots wrepo.SnapshotRepo
	profiles  ProfileLoader
	retriever Retriever
	llm       llm.Client
	cfg       Config
}

func New(c wrepo.ClosetRepo, s wrepo.SnapshotRepo, p ProfileLoader, r Retriever, l llm.Client, cfg Config) *Service {
	return &Service{closet: c, snapshots: s, profiles: p, retriever: r, llm: l, cfg: cfg}
}

// Get returns the wardrobe, regenerating only when the signature changed.
func (s *Service) Get(ctx context.Context, userID uuid.UUID) (*wdomain.WardrobeResponse, error) {
	return s.build(ctx, userID, false)
}

// Regenerate forces a fresh generation regardless of signature.
func (s *Service) Regenerate(ctx context.Context, userID uuid.UUID) (*wdomain.WardrobeResponse, error) {
	return s.build(ctx, userID, true)
}

func (s *Service) build(ctx context.Context, userID uuid.UUID, force bool) (*wdomain.WardrobeResponse, error) {
	closet, err := s.closet.ClosetItems(ctx, userID)
	if err != nil {
		return nil, err
	}
	profile, err := s.profiles.LoadProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	sig := ComputeSignature(closet, profile, s.cfg.DayStamp)
	onboarding := profile == nil && len(closet) == 0

	closetCards := closetToCards(closet)

	// Serve cached snapshot if fresh (and not forced).
	if !force {
		if snap, err := s.snapshots.Load(ctx, userID); err == nil && snap.Signature == sig {
			return &wdomain.WardrobeResponse{
				Closet: closetCards, Outfits: snap.Outfits,
				OutfitsStatus: "ready", OnboardingPrompt: onboarding,
			}, nil
		} else if err != nil && !errors.Is(err, wrepo.ErrNoSnapshot) {
			return nil, err
		}
	}

	outfits, model, tin, tout, genErr := s.generate(ctx, closet, profile)
	if genErr != nil {
		// Graceful degrade: closet still viewable.
		return &wdomain.WardrobeResponse{
			Closet: closetCards, Outfits: []wdomain.Outfit{},
			OutfitsStatus: "unavailable", OnboardingPrompt: onboarding,
		}, nil
	}

	if err := s.snapshots.Upsert(ctx, userID, sig, outfits, model, tin, tout); err != nil {
		return nil, err
	}
	return &wdomain.WardrobeResponse{
		Closet: closetCards, Outfits: outfits,
		OutfitsStatus: "ready", OnboardingPrompt: onboarding,
	}, nil
}

// generate runs the LLM grouping and assembles outfits. Two modes:
//   - closet non-empty: feed owned items → owned[]; add to_buy via retriever.
//   - closet empty: feed retrieved candidates → to_buy[]; owned stays empty.
func (s *Service) generate(ctx context.Context, closet []wdomain.ClosetItem, profile *spdomain.StyleProfileView) ([]wdomain.Outfit, string, int, int, error) {
	var budgetMin, budgetMax *int
	var profileStyles []string
	if profile != nil {
		budgetMin, budgetMax = profile.BudgetMin, profile.BudgetMax
		for _, t := range profile.StyleTags {
			profileStyles = append(profileStyles, t.Slug)
		}
	}

	if len(closet) > 0 {
		return s.generateFromCloset(ctx, closet, profileStyles, budgetMin, budgetMax)
	}
	return s.generateToBuy(ctx, profileStyles, budgetMin, budgetMax)
}

func (s *Service) generateFromCloset(ctx context.Context, closet []wdomain.ClosetItem, profileStyles []string, bMin, bMax *int) ([]wdomain.Outfit, string, int, int, error) {
	items := make([]promptItem, len(closet))
	byID := make(map[string]wdomain.ClosetItem, len(closet))
	for i, c := range closet {
		id := strconv.Itoa(i + 1)
		items[i] = promptItem{ID: id, Name: c.Name, Category: c.CategoryName, StyleSlugs: c.StyleSlugs}
		byID[id] = c
	}
	llmOutfits, resp, err := s.callLLM(ctx, items)
	if err != nil {
		return nil, "", 0, 0, err
	}

	// to_buy: retrieve complementary products by profile/closet styles.
	styles := profileStyles
	if len(styles) == 0 {
		styles = closetStyleSlugs(closet)
	}
	toBuy, _ := s.retriever.Retrieve(ctx, styles, bMin, bMax, s.cfg.ToBuyPerOutfit)

	var out []wdomain.Outfit
	for _, lo := range llmOutfits {
		var owned []wdomain.OutfitCard
		for _, id := range lo.ItemIDs {
			if c, ok := byID[id]; ok {
				owned = append(owned, closetItemToCard(c))
			}
		}
		if len(owned) == 0 {
			continue // model referenced no real owned items; skip
		}
		out = append(out, wdomain.Outfit{Title: lo.Title, Note: lo.Note, Owned: owned, ToBuy: toBuy})
	}
	return out, resp.Model, resp.TokensIn, resp.TokensOut, nil
}

func (s *Service) generateToBuy(ctx context.Context, profileStyles []string, bMin, bMax *int) ([]wdomain.Outfit, string, int, int, error) {
	// Empty closet: retrieve a candidate set to compose buy-the-outfit looks.
	// Profile styles drive it; with no profile the retriever falls back to
	// popular products (no style filter).
	cands, err := s.retriever.Retrieve(ctx, profileStyles, bMin, bMax, s.cfg.MaxOutfits*s.cfg.ToBuyPerOutfit)
	if err != nil {
		return nil, "", 0, 0, err
	}
	if len(cands) == 0 {
		return []wdomain.Outfit{}, "none", 0, 0, nil
	}
	items := make([]promptItem, len(cands))
	byID := make(map[string]wdomain.OutfitCard, len(cands))
	for i, c := range cands {
		id := strconv.Itoa(i + 1)
		items[i] = promptItem{ID: id, Name: c.Name}
		byID[id] = c
	}
	llmOutfits, resp, err := s.callLLM(ctx, items)
	if err != nil {
		return nil, "", 0, 0, err
	}
	var out []wdomain.Outfit
	for _, lo := range llmOutfits {
		var toBuy []wdomain.OutfitCard
		for _, id := range lo.ItemIDs {
			if c, ok := byID[id]; ok {
				toBuy = append(toBuy, c)
			}
		}
		if len(toBuy) == 0 {
			continue
		}
		out = append(out, wdomain.Outfit{Title: lo.Title, Note: lo.Note, Owned: []wdomain.OutfitCard{}, ToBuy: toBuy})
	}
	return out, resp.Model, resp.TokensIn, resp.TokensOut, nil
}

func (s *Service) callLLM(ctx context.Context, items []promptItem) ([]wdomain.LLMOutfit, *llm.GenerateResponse, error) {
	system, user := BuildItemsPrompt(items, s.cfg.MaxOutfits)
	resp, err := s.llm.Generate(ctx, llm.GenerateRequest{System: system, Prompt: user})
	if err != nil {
		return nil, nil, err
	}
	parsed, err := ParseOutfits(resp.Text)
	if err != nil {
		return nil, nil, err
	}
	return parsed, resp, nil
}

func closetToCards(items []wdomain.ClosetItem) []wdomain.OutfitCard {
	out := make([]wdomain.OutfitCard, 0, len(items))
	for _, c := range items {
		out = append(out, closetItemToCard(c))
	}
	return out
}

func closetItemToCard(c wdomain.ClosetItem) wdomain.OutfitCard {
	return wdomain.OutfitCard{ID: c.ProductID.String(), Name: c.Name}
}

func closetStyleSlugs(items []wdomain.ClosetItem) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range items {
		for _, s := range c.StyleSlugs {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}
