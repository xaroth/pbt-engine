package simulator_test

import (
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/simulator"
)

// TestNodeText compares nodes with the game's 3.29.1 tree export
// (github.com/grindinggear/skilltree-export, data.json).
func TestNodeText(t *testing.T) {
	t.Parallel()
	sim, err := simulator.New(realdata.Open(t, realdata.Current))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		what string
		id   uint32
		want simulator.NodeText
	}{
		{"small passive, in description order", 1822, simulator.NodeText{
			Name:  "Life and Energy Shield",
			Lines: []string{"5% increased maximum Energy Shield", "5% increased maximum Life"},
		}},
		{"notable", 2550, simulator.NodeText{
			Name:  "Arsonist",
			Lines: []string{"+10% to Fire Damage over Time Multiplier", "Regenerate 1.2% of Life per second"},
		}},
		{"keystone, one description over two rows", 39085, simulator.NodeText{
			Name: "Elemental Equilibrium",
			Lines: []string{"Hits that deal Elemental Damage remove Exposure to those Elements and inflict Exposure to other Elements\n" +
				"Exposure inflicted this way applies -25% to Resistances"},
		}},
		{"mastery", 44298, simulator.NodeText{
			Name: "Elemental Mastery",
			Masteries: []simulator.MasteryText{
				{Effect: 48385, Lines: []string{"Exposure you inflict applies at least -18% to the affected Resistance"}},
				{Effect: 4119, Lines: []string{"Prevent +60% of Reflected Elemental Damage"}},
				{Effect: 59343, Lines: []string{"50% reduced Effect of Exposure on you"}},
				{Effect: 17942, Lines: []string{"Hits have 15% chance to treat Enemy Monster Elemental Resistance values as inverted"}},
				{Effect: 30502, Lines: []string{"Critical Strikes against you do not inherently inflict Elemental Ailments"}},
				{Effect: 25681, Lines: []string{"3% chance for Hits to deal 300% of Physical Damage as Extra Damage of a random Element"}},
			},
		}},
		{"buff in the aura scope", 17315, simulator.NodeText{
			Name: "Overwhelm",
			Lines: []string{
				"Base Critical Strike Chance for Attacks with Weapons is 8%",
				"+10% to Critical Strike Multiplier per Nearby Enemy, up to +100%",
				"Nearby Enemies have -30% to Critical Strike Multiplier",
			},
		}},
	}
	for _, c := range cases {
		t.Run(c.what, func(t *testing.T) {
			t.Parallel()
			got, err := sim.NodeText(build.DefaultVariant, c.id, "en")
			if err != nil {
				t.Fatal(err)
			}
			if diff := gocmp.Diff(c.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("node %d (-want +got):\n%s", c.id, diff)
			}
		})
	}
}
