package service

import (
	"encoding/json"
	"fmt"
	"strings"

	wdomain "github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
)

const wardrobeSystemPrompt = `You are a fashion stylist. You will receive a numbered list of clothing items (id, name, category, styles). Group them into 1 to %d cohesive outfits. Each outfit must reference ONLY item ids from the provided list — never invent items. Respond with STRICT JSON: {"outfits":[{"title":string,"note":string,"item_ids":[string,...]}]}. The note is one short sentence of styling advice. Do not include any text outside the JSON.`

// promptItem is one line fed to the model. Index is the stable "id" the model
// echoes back in item_ids.
type promptItem struct {
	ID         string
	Name       string
	Category   string
	StyleSlugs []string
}

// BuildItemsPrompt renders the system + user prompt for a set of items.
func BuildItemsPrompt(items []promptItem, maxOutfits int) (system, user string) {
	system = fmt.Sprintf(wardrobeSystemPrompt, maxOutfits)
	var sb strings.Builder
	sb.WriteString("Items:\n")
	for _, it := range items {
		sb.WriteString(fmt.Sprintf("- id=%s | %s | category=%s | styles=%s\n",
			it.ID, it.Name, it.Category, strings.Join(it.StyleSlugs, ",")))
	}
	return system, sb.String()
}

// ParseOutfits defensively parses the model's JSON. Tolerates code-fence
// wrapping. Returns nil + error on unrecoverable output.
func ParseOutfits(raw string) ([]wdomain.LLMOutfit, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	var out wdomain.LLMOutfits
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("wardrobe: parse outfits: %w", err)
	}
	return out.Outfits, nil
}
