package shopping

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace only", "   \t\n", ""},
		{"lowercases", "Fresh Mushrooms", "fresh mushrooms"},
		{"strips combining diacritics", "Crème Fraîche", "creme fraiche"},
		{"collapses punctuation to spaces", "salt, pepper & oil", "salt pepper oil"},
		{"keeps digits", "500 g flour", "500 g flour"},
		{"keeps cyrillic", "Грибы свежие", "грибы свежие"},
		{"keeps finnish letters", "Sienet ja kerma", "sienet ja kerma"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Normalize(c.in); got != c.want {
				t.Fatalf("Normalize(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestContainsTerm(t *testing.T) {
	cases := []struct {
		name     string
		haystack string
		needle   string
		want     bool
	}{
		{"empty needle", "Fresh Mushrooms", "", false},
		{"whitespace needle", "Fresh Mushrooms", "   ", false},
		{"empty haystack", "", "mushroom", false},
		{"english plural prefix", "Fresh Mushrooms", "mushroom", true},
		{"english exact phrase", "Mushroom Soup", "mushroom", true},
		{"english reverse plural to singular", "Mushroom Soup", "mushrooms", true},
		{"english unrelated word with shared start", "mushy pasta", "mushroom", false},
		{"russian inflection nominative plural", "грибы", "гриб", true},
		{"russian inflection adjective", "грибной соус", "гриб", true},
		{"russian eggs", "яйца куриные", "яйцо", true},
		{"finnish plural", "sienet", "sieni", true},
		{"finnish adessive", "sienillä", "sieni", true},
		{"short needle requires exact token", "orange juice", "or", false},
		{"short needle exact match", "or rye", "or", true},
		{"multi-token needle all must hit", "green beans, chopped", "green bean", true},
		{"multi-token needle missing one token", "green peppers", "green bean", false},
		{"diacritic-insensitive both sides", "Crème Fraîche", "creme fraiche", true},
		{"diacritic-insensitive needle", "creme fraiche", "crème", true},
		{"case-insensitive", "BEEF Stew", "beef", true},
		{"no false positive on substring inside token", "blackberry", "berry", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ContainsTerm(c.haystack, c.needle); got != c.want {
				t.Fatalf("ContainsTerm(%q, %q) = %v, want %v", c.haystack, c.needle, got, c.want)
			}
		})
	}
}
