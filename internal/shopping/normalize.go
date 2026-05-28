package shopping

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// minPrefixLen is the threshold below which token prefix matching falls back to
// exact token match. Three is short enough for RU/FI stems ("гриб", "sien") and
// long enough that two-letter prepositions ("or", "на") don't blow up.
const minPrefixLen = 3

// stripMarks removes Unicode combining marks (Mn) so e.g. "crème" → "creme".
// Cyrillic Й and Ё are precomposed letters, not combining marks, so they
// survive intact.
var stripMarks = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// Normalize lowercases s, drops combining diacritics, and replaces any rune that
// is not a letter or digit with a single space, then collapses whitespace runs.
// The result is suitable for token-based comparison of ingredient names.
func Normalize(s string) string {
	folded, _, err := transform.String(stripMarks, strings.ToLower(s))
	if err != nil {
		folded = strings.ToLower(s)
	}
	var b strings.Builder
	b.Grow(len(folded))
	prevSpace := true
	for _, r := range folded {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
			continue
		}
		if !prevSpace {
			b.WriteByte(' ')
			prevSpace = true
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// ContainsTerm reports whether the (normalized) haystack mentions the
// (normalized) needle, accounting for case, diacritics, and a useful slice of
// RU/FI/EN inflection. Each needle token must independently find a haystack
// match (per-token AND); empty inputs never match.
//
// Matching of a single needle token against the haystack tokens:
//   - Tokens shorter than minPrefixLen require an exact token match.
//   - Otherwise three checks run in sequence; the first hit wins:
//     1. Forward prefix: a haystack token starts with the needle ("mushroom"
//     in "mushrooms", "гриб" in "грибной").
//     2. Reverse prefix: the needle starts with a haystack token (also
//     ≥ minPrefixLen). Catches a user-typed plural against a recipe singular
//     ("mushrooms" disliked → "Mushroom soup" caught).
//     3. Stem drop: drop the last rune of the needle (still ≥ minPrefixLen)
//     and forward-prefix again. Bridges stem-mutating inflections like
//     RU яйцо/яйца and FI sieni/sienet at the cost of slightly looser
//     matching at the tail.
//
// Known limit: very long inflected needles whose stem mutates two or more runes
// from the recipe form (e.g. needle "sienet" vs hay "sieni") still miss. The
// generation service retries with prompt accent on miss and fails closed if the
// constraint cannot be honored — this matcher is the defense-in-depth layer,
// not the primary guard.
func ContainsTerm(haystack, needle string) bool {
	needleTokens := strings.Fields(Normalize(needle))
	if len(needleTokens) == 0 {
		return false
	}
	hayTokens := strings.Fields(Normalize(haystack))
	if len(hayTokens) == 0 {
		return false
	}
	for _, nt := range needleTokens {
		if !matchAny(hayTokens, nt) {
			return false
		}
	}
	return true
}

func matchAny(hay []string, needle string) bool {
	needleRunes := []rune(needle)
	n := len(needleRunes)
	if n < minPrefixLen {
		for _, h := range hay {
			if h == needle {
				return true
			}
		}
		return false
	}
	for _, h := range hay {
		if strings.HasPrefix(h, needle) {
			return true
		}
	}
	for _, h := range hay {
		if len([]rune(h)) < minPrefixLen {
			continue
		}
		if strings.HasPrefix(needle, h) {
			return true
		}
	}
	if n > minPrefixLen {
		stem := string(needleRunes[:n-1])
		for _, h := range hay {
			if strings.HasPrefix(h, stem) {
				return true
			}
		}
	}
	return false
}
