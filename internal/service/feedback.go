package service

import (
	"strings"

	"github.com/AntonKilk/cooking-helper/internal/domain"
)

// maxFeedbackTitleChars bounds a single serialized history line so a pathological
// recipe title cannot inflate the prompt's token footprint. The line count is
// already bounded by the configured recent-history limit; this caps each line's
// width. Titles longer than this are truncated with an ellipsis.
const maxFeedbackTitleChars = 80

// dislikedTag is the bracket emitted for a previously-disliked dish. It is an
// explicit do-not-repeat instruction (stronger than the neutral feedback tag) so
// the model treats a disliked past recipe as something to avoid, not merely to
// note. The generate-week prompt's guidance is phrased to match this wording.
const dislikedTag = "DISLIKED — do not make this again"

// serializeRecent turns recent recipes (newest-first, as RecentRecipes returns
// them) into compact prompt lines tagged with their feedback, e.g.
// "Creamy Pasta [liked, cook again]". A disliked dish overrides the neutral tag
// with an explicit do-not-repeat instruction. Titles are truncated to
// maxFeedbackTitleChars to keep the prompt within a reasonable token budget.
// Input order is preserved.
func serializeRecent(recipes []domain.Recipe) []string {
	lines := make([]string, 0, len(recipes))
	for _, r := range recipes {
		line := truncateTitle(r.Title)
		if tag := feedbackTag(r.Feedback); tag != "" {
			line += " [" + tag + "]"
		}
		lines = append(lines, line)
	}
	return lines
}

// truncateTitle shortens a title to maxFeedbackTitleChars runes, appending an
// ellipsis when it had to cut. Rune-aware so multibyte titles (RU/FI) are not
// split mid-character.
func truncateTitle(title string) string {
	runes := []rune(title)
	if len(runes) <= maxFeedbackTitleChars {
		return title
	}
	return strings.TrimSpace(string(runes[:maxFeedbackTitleChars])) + "…"
}

// feedbackTag renders a recipe's feedback as a compact bracket label. A disliked
// reaction takes precedence and yields the explicit do-not-repeat instruction;
// otherwise the positive signals (liked, cook again) are joined. Returns "" when
// there is no feedback or no signal set.
func feedbackTag(f *domain.Feedback) string {
	if f == nil {
		return ""
	}
	if f.Disliked {
		return dislikedTag
	}
	var parts []string
	if f.Liked {
		parts = append(parts, "liked")
	}
	if f.CookAgain {
		parts = append(parts, "cook again")
	}
	return strings.Join(parts, ", ")
}
