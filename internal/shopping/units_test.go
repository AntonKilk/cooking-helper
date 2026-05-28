package shopping

import "testing"

func TestClassifyUnit(t *testing.T) {
	cases := []struct {
		name   string
		unit   string
		class  string
		factor float64
		ok     bool
	}{
		{"grams", "g", classMass, 1, true},
		{"kilograms", "kg", classMass, 1000, true},
		{"russian grams", "г", classMass, 1, true},
		{"russian kilograms", "кг", classMass, 1000, true},
		{"millilitres", "ml", classVolume, 1, true},
		{"litres", "l", classVolume, 1000, true},
		{"decilitres", "dl", classVolume, 100, true},
		{"finnish tablespoon", "rkl", classVolume, 15, true},
		{"finnish teaspoon", "tl", classVolume, 5, true},
		{"russian tablespoon dotted", "ст.л.", classVolume, 15, true},
		{"russian teaspoon dotted", "ч.л.", classVolume, 5, true},
		{"russian teaspoon spaced", "ч. л.", classVolume, 5, true},
		{"finnish pieces", "kpl", classCount, 1, true},
		{"russian pieces", "шт", classCount, 1, true},
		{"english pieces", "pcs", classCount, 1, true},
		{"uppercase normalizes", "KG", classMass, 1000, true},
		{"empty is opaque", "", "", 0, false},
		{"unknown is opaque", "pinch", "", 0, false},
		{"clove is opaque", "clove", "", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			class, factor, ok := classifyUnit(c.unit)
			if class != c.class || factor != c.factor || ok != c.ok {
				t.Fatalf("classifyUnit(%q) = (%q, %v, %v), want (%q, %v, %v)",
					c.unit, class, factor, ok, c.class, c.factor, c.ok)
			}
		})
	}
}

func TestDisplayAmount(t *testing.T) {
	cases := []struct {
		name      string
		class     string
		canonical float64
		amount    float64
		unit      string
	}{
		{"grams below threshold", classMass, 350, 350, "g"},
		{"grams just below kilo", classMass, 999, 999, "g"},
		{"exactly one kilo", classMass, 1000, 1, "kg"},
		{"one and a half kilo", classMass, 1500, 1.5, "kg"},
		{"millilitres", classVolume, 250, 250, "ml"},
		{"one litre", classVolume, 1000, 1, "l"},
		{"count stays pcs", classCount, 3, 3, "pcs"},
		{"rounds noise", classMass, 350.004, 350, "g"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			amount, unit := displayAmount(c.class, c.canonical)
			if amount != c.amount || unit != c.unit {
				t.Fatalf("displayAmount(%q, %v) = (%v, %q), want (%v, %q)",
					c.class, c.canonical, amount, unit, c.amount, c.unit)
			}
		})
	}
}
