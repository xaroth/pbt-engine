package items_test

import (
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/items"
)

// Parse sets a line's ranges at its {range:} roll as Path of Building rounds them, and keeps the
// highest with a warning where no roll is given.
func TestParseRoll(t *testing.T) {
	t.Parallel()
	r, _ := realReader(t)
	tests := []struct {
		name, line, want string
		assumed          bool
	}{
		{name: "half up", line: "{range:0.5}+(20-25) to maximum Life", want: "+23 to maximum Life"},
		{name: "whole", line: "{range:0.25}+(20-60) to maximum Life", want: "+30 to maximum Life"},
		{name: "high precision", line: "{range:0.25}Regenerate (1-2)% of Life per second", want: "Regenerate 1.25% of Life per second"},
		{name: "flat regeneration", line: "{range:0.25}Regenerate (1-2) Life per second", want: "Regenerate 1.3 Life per second"},
		{name: "decimals in the line", line: "{range:0.25}(1.5-2.5)% increased Attack Speed", want: "1.8% increased Attack Speed"},
		{name: "negative turns", line: "{range:0}(-10-10)% increased Mana Regeneration Rate", want: "10% reduced Mana Regeneration Rate"},
		{name: "no roll", line: "+(20-30) to maximum Life", want: "+(20-30) to maximum Life", assumed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			it, warnings, err := r.Parse("Rarity: RARE\nDoom Coil\nCoral Ring\nImplicits: 0\n" + tt.line + "\n")
			if err != nil {
				t.Fatal(err)
			}
			if got := it.Mods[0].Text; got != tt.want {
				t.Errorf("line %q, want %q", got, tt.want)
			}
			assumed := slices.ContainsFunc(warnings, func(w build.Warning) bool { return w.Code == items.RollAssumed })
			if assumed != tt.assumed {
				t.Errorf("roll assumed %t, want %t: %v", assumed, tt.assumed, warnings)
			}
		})
	}
}
