package shopping

import "github.com/AntonKilk/cooking-helper/internal/domain"

// categoryRule maps a store category to the multilingual (EN / FI / RU) keywords
// that imply it. Rules are consulted in slice order and the first keyword hit
// wins, so the more specific categories (frozen, meat/fish, dairy) come before
// the broad ones (produce, pantry) to avoid a generic keyword shadowing them.
type categoryRule struct {
	category domain.IngredientCategory
	keywords []string
}

// categoryRules is a deliberately first-pass dictionary: it places the common
// ingredients that dominate the recipe set deterministically and for free. The
// long tail is handled by the service layer's LLM fallback (and cached), so this
// table is tuned for precision on frequent items rather than exhaustive coverage.
var categoryRules = []categoryRule{
	{domain.CategoryFrozen, []string{
		"frozen", "pakaste", "pakastettu", "заморож", "ice cream", "jäätelö", "мороженое",
	}},
	{domain.CategoryMeatFish, []string{
		"chicken", "kana", "кур",
		"turkey", "kalkkuna", "индейк",
		"beef", "nauta", "naudan", "говядин", "steak", "pihvi",
		"pork", "sika", "porsaan", "свин",
		"ham", "kinkku", "ветчин", "bacon", "pekoni", "бекон",
		"lamb", "lammas", "lampaan", "ягнят", "баранин",
		"mince", "jauheliha", "фарш",
		"sausage", "makkara", "колбас",
		"salmon", "lohi", "лосось", "сёмг", "семг",
		"fish", "kala", "рыб",
		"tuna", "tonnikala", "тунец", "cod", "turska", "треск",
		"shrimp", "katkarapu", "креветк", "prawn", "seafood", "äyriäis",
	}},
	{domain.CategoryDairy, []string{
		"milk", "maito", "молоко",
		"cheese", "juusto", "сыр",
		"cream", "kerma", "сливк", "сметан", "smetana", "creme fraiche", "crème fraiche",
		"yogurt", "yoghurt", "jogurtti", "jugurtti", "йогурт",
		"egg", "muna", "яйц",
		"quark", "rahka", "творог", "kefir", "кефир",
		"масло сливочн", "сливочное масло",
	}},
	{domain.CategoryProduce, []string{
		"onion", "sipuli", "лук",
		"carrot", "porkkana", "морков",
		"potato", "peruna", "картоф",
		"tomato", "tomaatti", "помидор", "томат",
		"garlic", "valkosipuli", "чеснок",
		"paprika", "bell pepper",
		"cucumber", "kurkku", "огур",
		"lettuce", "salaatti", "салат",
		"cabbage", "kaali", "капуст",
		"broccoli", "parsakaali", "брокколи",
		"mushroom", "sieni", "гриб",
		"spinach", "pinaatti", "шпинат",
		"apple", "omena", "яблок",
		"banana", "banaani", "банан",
		"lemon", "sitruuna", "лимон", "lime", "лайм",
		"orange", "appelsiini", "апельсин",
		"berry", "marja", "ягод",
		"herb", "yrtti", "basil", "basilika", "базилик",
		"parsley", "persilja", "петрушк", "dill", "tilli", "укроп", "зелень",
		"avocado", "avokado", "авокадо",
		"zucchini", "kesäkurpitsa", "кабач",
		"corn", "maissi", "кукуруз",
		"bean", "papu", "фасол", "pea", "herne", "горош",
	}},
	{domain.CategoryPantry, []string{
		"flour", "jauho", "мука",
		"sugar", "sokeri", "сахар",
		"salt", "suola", "соль",
		"rice", "riisi", "рис",
		"pasta", "макарон", "спагетти", "spaghetti", "noodle", "nuudeli", "лапша",
		"oil", "öljy", "масло раст", "растительное масло", "olive oil", "oliiviöljy",
		"pepper", "pippuri",
		"spice", "mauste", "специ", "припра",
		"stock", "liemi", "бульон", "broth", "bouillon",
		"sauce", "kastike", "соус", "ketchup", "кетчуп",
		"vinegar", "etikka", "уксус",
		"honey", "hunaja", "мёд",
		"jam", "hillo", "варенье",
		"bread", "leipä", "хлеб",
		"oat", "kaura", "овсян", "cereal", "muesli", "мюсли",
		"can", "säilyke", "консерв", "tomato paste", "tomaattipyree", "томатная паста",
		"yeast", "hiiva", "дрожж", "baking", "leivinjauhe", "сода",
		"mustard", "sinappi", "горчиц",
		"soy", "soija", "соев",
		"coconut milk", "kookosmaito",
		"chocolate", "suklaa", "шоколад", "cocoa", "kaakao", "какао",
		"nut", "pähkinä", "орех",
	}},
}

// LookupCategory returns the store category implied by an ingredient name using
// the built-in dictionary, and true when a keyword matched. It returns
// ("", false) when no keyword applies — the caller resolves the miss (cache then
// LLM). Matching is inflection- and diacritic-tolerant via ContainsTerm.
func LookupCategory(name string) (domain.IngredientCategory, bool) {
	for _, rule := range categoryRules {
		for _, kw := range rule.keywords {
			if ContainsTerm(name, kw) {
				return rule.category, true
			}
		}
	}
	return "", false
}
