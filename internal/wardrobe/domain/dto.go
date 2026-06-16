package domain

import "github.com/google/uuid"

// ClosetItem is one owned product (from a delivered purchase) with the
// attributes Gemini needs to reason about pairing.
type ClosetItem struct {
	ProductID    uuid.UUID
	Name         string
	CategorySlug string
	CategoryName string
	StyleSlugs   []string
}

// OutfitCard is a product shown inside an outfit (owned or to-buy).
type OutfitCard struct {
	ID           string  `json:"id"`
	Slug         string  `json:"slug"`
	Name         string  `json:"name"`
	BrandSlug    string  `json:"brand_slug"`
	BrandName    string  `json:"brand_name"`
	Currency     string  `json:"currency"`
	MinPrice     float64 `json:"min_price"`
	PrimaryImage *string `json:"primary_image,omitempty"`
}

// Outfit is one composed look: owned pieces plus complementary buys.
type Outfit struct {
	Title string       `json:"title"`
	Note  string       `json:"note"`
	Owned []OutfitCard `json:"owned"`
	ToBuy []OutfitCard `json:"to_buy"`
}

// WardrobeResponse is the GET /me/wardrobe body.
type WardrobeResponse struct {
	Closet           []OutfitCard `json:"closet"`
	Outfits          []Outfit     `json:"outfits"`
	OutfitsStatus    string       `json:"outfits_status"` // "ready" | "unavailable"
	OnboardingPrompt bool         `json:"onboarding_prompt"`
}

// LLMOutfit / LLMOutfits are the JSON shape the model returns. item_ids are
// indices into the item list we sent (as strings), mapped back to products.
type LLMOutfit struct {
	Title   string   `json:"title"`
	Note    string   `json:"note"`
	ItemIDs []string `json:"item_ids"`
}
type LLMOutfits struct {
	Outfits []LLMOutfit `json:"outfits"`
}
