package shopping

import (
	"math"
	"strings"
)

// Measurement classes for shopping-list consolidation. Amounts within a class
// are summed in the class's canonical base unit (g, ml, pcs). Units in different
// classes never merge — a "1 pc" and a "100 g" of the same ingredient stay as
// two separate lines.
const (
	classMass   = "mass"
	classVolume = "volume"
	classCount  = "count"
)

// unitConv maps a normalized, space-stripped unit token to its measurement class
// and the factor that converts one of that unit into the class's canonical base
// (gram, millilitre, piece). Finnish volume units follow the standard kitchen
// equivalences: dl = 100 ml, rkl (ruokalusikka) = 15 ml, tl (teelusikka) = 5 ml.
var unitConv = map[string]struct {
	class  string
	factor float64
}{
	// mass — canonical gram
	"g": {classMass, 1}, "gr": {classMass, 1}, "gram": {classMass, 1},
	"grams": {classMass, 1}, "gramma": {classMass, 1}, "г": {classMass, 1},
	"грамм": {classMass, 1},
	"kg":    {classMass, 1000}, "kilo": {classMass, 1000}, "kilogram": {classMass, 1000},
	"кг": {classMass, 1000},

	// volume — canonical millilitre
	"ml": {classVolume, 1}, "milliliter": {classVolume, 1}, "millilitre": {classVolume, 1},
	"мл": {classVolume, 1},
	"l":  {classVolume, 1000}, "liter": {classVolume, 1000}, "litre": {classVolume, 1000},
	"litra": {classVolume, 1000}, "л": {classVolume, 1000},
	"dl": {classVolume, 100}, "desiliter": {classVolume, 100}, "desilitra": {classVolume, 100},
	"дл":  {classVolume, 100},
	"rkl": {classVolume, 15}, "tbsp": {classVolume, 15}, "ruokalusikka": {classVolume, 15},
	"stl": {classVolume, 15}, "tablespoon": {classVolume, 15}, "стл": {classVolume, 15},
	"tl": {classVolume, 5}, "tsp": {classVolume, 5}, "teelusikka": {classVolume, 5},
	"tsl": {classVolume, 5}, "teaspoon": {classVolume, 5}, "чл": {classVolume, 5},

	// count — canonical piece
	"pcs": {classCount, 1}, "pc": {classCount, 1}, "piece": {classCount, 1},
	"pieces": {classCount, 1}, "kpl": {classCount, 1}, "kappale": {classCount, 1},
	"шт": {classCount, 1}, "штука": {classCount, 1}, "штуки": {classCount, 1},
	"штук": {classCount, 1},
}

// classifyUnit returns the measurement class of a recipe unit string and the
// factor that converts one such unit to the class's canonical base. ok is false
// for an unrecognized or empty unit; the caller then treats the unit as an opaque
// family that merges only with an identical unit string.
func classifyUnit(unit string) (class string, factor float64, ok bool) {
	key := normUnit(unit)
	if key == "" {
		return "", 0, false
	}
	if c, found := unitConv[key]; found {
		return c.class, c.factor, true
	}
	return "", 0, false
}

// normUnit folds a unit string for map lookup: it lowercases, drops diacritics
// and punctuation via Normalize, then removes the inter-token spaces so dotted
// abbreviations collapse to a single token ("ст.л." → "стл", "ч. л." → "чл").
func normUnit(u string) string {
	return strings.ReplaceAll(Normalize(u), " ", "")
}

// displayAmount converts a summed canonical quantity back into a human-friendly
// amount and unit: mass shows kg at or above 1000 g else g; volume shows l at or
// above 1000 ml else ml; count shows pcs. The amount is rounded to two decimals.
func displayAmount(class string, canonical float64) (amount float64, unit string) {
	switch class {
	case classMass:
		if canonical >= 1000 {
			return round2(canonical / 1000), "kg"
		}
		return round2(canonical), "g"
	case classVolume:
		if canonical >= 1000 {
			return round2(canonical / 1000), "l"
		}
		return round2(canonical), "ml"
	case classCount:
		return round2(canonical), "pcs"
	default:
		return round2(canonical), ""
	}
}

// round2 rounds to two decimal places, enough precision for kitchen quantities
// while trimming float summation noise.
func round2(f float64) float64 {
	return math.Round(f*100) / 100
}
