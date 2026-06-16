package service

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	spdomain "github.com/wearwhere/wearwhere_be/internal/styleprofile/domain"
	wdomain "github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
)

// ComputeSignature derives a stable fingerprint of the inputs that should
// trigger regeneration: the closet product set and the style profile. When the
// closet is empty the day stamp is folded in so an empty wardrobe refreshes
// daily. Returns a short hex digest.
func ComputeSignature(closet []wdomain.ClosetItem, profile *spdomain.StyleProfileView, dayStamp string) string {
	ids := make([]string, 0, len(closet))
	for _, c := range closet {
		ids = append(ids, c.ProductID.String())
	}
	sort.Strings(ids)

	var prof []string
	if profile != nil {
		for _, t := range profile.StyleTags {
			prof = append(prof, t.ID)
		}
		sort.Strings(prof)
		if profile.BudgetMin != nil {
			prof = append(prof, "bmin:"+strconv.Itoa(*profile.BudgetMin))
		}
		if profile.BudgetMax != nil {
			prof = append(prof, "bmax:"+strconv.Itoa(*profile.BudgetMax))
		}
	}

	var sb strings.Builder
	sb.WriteString("closet:")
	sb.WriteString(strings.Join(ids, ","))
	sb.WriteString("|profile:")
	sb.WriteString(strings.Join(prof, ","))
	if len(closet) == 0 {
		sb.WriteString("|day:")
		sb.WriteString(dayStamp)
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:8])
}
