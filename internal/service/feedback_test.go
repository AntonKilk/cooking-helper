package service

import (
	"strings"
	"testing"

	"github.com/AntonKilk/cooking-helper/internal/domain"
)

func TestSerializeRecentEmpty(t *testing.T) {
	if got := serializeRecent(nil); len(got) != 0 {
		t.Fatalf("serializeRecent(nil) = %v, want empty", got)
	}
}

func TestSerializeRecentTags(t *testing.T) {
	cases := []struct {
		name     string
		recipe   domain.Recipe
		wantLine string
	}{
		{
			name:     "no feedback",
			recipe:   domain.Recipe{Title: "Plain Soup"},
			wantLine: "Plain Soup",
		},
		{
			name:     "liked and cook again",
			recipe:   domain.Recipe{Title: "Creamy Pasta", Feedback: &domain.Feedback{Liked: true, CookAgain: true}},
			wantLine: "Creamy Pasta [liked, cook again]",
		},
		{
			name:     "liked only",
			recipe:   domain.Recipe{Title: "Beef Tacos", Feedback: &domain.Feedback{Liked: true}},
			wantLine: "Beef Tacos [liked]",
		},
		{
			name:     "cook again only",
			recipe:   domain.Recipe{Title: "Fish Bowl", Feedback: &domain.Feedback{CookAgain: true}},
			wantLine: "Fish Bowl [cook again]",
		},
		{
			name:     "disliked overrides neutral tag",
			recipe:   domain.Recipe{Title: "Mushroom Stew", Feedback: &domain.Feedback{Disliked: true, Liked: true, CookAgain: true}},
			wantLine: "Mushroom Stew [" + dislikedTag + "]",
		},
		{
			name:     "feedback with no signal set",
			recipe:   domain.Recipe{Title: "Empty Signal", Feedback: &domain.Feedback{}},
			wantLine: "Empty Signal",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := serializeRecent([]domain.Recipe{c.recipe})
			if len(got) != 1 || got[0] != c.wantLine {
				t.Fatalf("serializeRecent = %v, want [%q]", got, c.wantLine)
			}
		})
	}
}

func TestSerializeRecentDislikedNotNeutralTag(t *testing.T) {
	got := serializeRecent([]domain.Recipe{
		{Title: "Bad Dish", Feedback: &domain.Feedback{Disliked: true}},
	})
	if len(got) != 1 {
		t.Fatalf("got %d lines, want 1", len(got))
	}
	if !strings.Contains(got[0], "do not make this again") {
		t.Errorf("disliked line missing do-not-repeat instruction: %q", got[0])
	}
	if strings.Contains(got[0], "[disliked]") {
		t.Errorf("disliked line should not use the bare neutral tag: %q", got[0])
	}
}

func TestSerializeRecentPreservesOrder(t *testing.T) {
	got := serializeRecent([]domain.Recipe{
		{Title: "First"},
		{Title: "Second"},
		{Title: "Third"},
	})
	want := []string{"First", "Second", "Third"}
	if !equalStrings(got, want) {
		t.Fatalf("serializeRecent order = %v, want %v", got, want)
	}
}

func TestSerializeRecentTruncatesLongTitle(t *testing.T) {
	long := strings.Repeat("a", maxFeedbackTitleChars+25)
	got := serializeRecent([]domain.Recipe{{Title: long}})
	if len(got) != 1 {
		t.Fatalf("got %d lines, want 1", len(got))
	}
	line := got[0]
	if !strings.HasSuffix(line, "…") {
		t.Errorf("over-long title not truncated with ellipsis: %q", line)
	}
	// Body before the ellipsis must not exceed the rune budget.
	body := strings.TrimSuffix(line, "…")
	if n := len([]rune(body)); n > maxFeedbackTitleChars {
		t.Errorf("truncated body = %d runes, want <= %d", n, maxFeedbackTitleChars)
	}
}

func TestSerializeRecentTruncationRuneAware(t *testing.T) {
	// Multibyte (Cyrillic) title longer than the budget must not split a rune.
	long := strings.Repeat("щ", maxFeedbackTitleChars+10)
	got := serializeRecent([]domain.Recipe{{Title: long}})
	if len(got) != 1 {
		t.Fatalf("got %d lines, want 1", len(got))
	}
	if !strings.ContainsRune(got[0], '…') {
		t.Errorf("multibyte title not truncated: %q", got[0])
	}
	// Valid UTF-8 with no replacement characters means no rune was split.
	if strings.ContainsRune(got[0], '�') {
		t.Errorf("truncation split a multibyte rune: %q", got[0])
	}
}
