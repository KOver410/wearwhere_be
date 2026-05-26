// Package slug provides URL-safe slug generation and validation.
// Slugify removes diacritics, lowercases, and collapses runs of non-alphanumeric
// characters to a single hyphen. IsValid checks a string matches the slug grammar.
package slug

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var (
	slugRe        = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	nonAlphaNumRe = regexp.MustCompile(`[^a-z0-9]+`)
	dReplacer     = strings.NewReplacer("đ", "d", "Đ", "D")
)

// Slugify converts s into a URL-safe slug:
//   - NFD-normalize, strip combining marks (Vietnamese tone removal)
//   - lowercase
//   - replace runs of non-alphanumeric with "-"
//   - trim leading/trailing "-"
func Slugify(s string) string {
	t := transform.Chain(
		norm.NFD,
		runes.Remove(runes.In(unicode.Mn)),
		norm.NFC,
	)
	out, _, err := transform.String(t, s)
	if err != nil {
		return ""
	}
	// Replace 'đ' / 'Đ' explicitly — they are not decomposable.
	out = dReplacer.Replace(out)
	out = strings.ToLower(out)
	out = nonAlphaNumRe.ReplaceAllString(out, "-")
	out = strings.Trim(out, "-")
	return out
}

// IsValid reports whether s is a syntactically valid slug.
// Empty strings are not valid.
func IsValid(s string) bool {
	return s != "" && slugRe.MatchString(s)
}
