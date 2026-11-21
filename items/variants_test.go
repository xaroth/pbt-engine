package items_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
)

// leerCast is the unique helmet with its two variants, rolled halfway, as Path of Building
// writes it; selection is the variant lines.
func leerCast(selection string) string {
	return `Rarity: UNIQUE
Leer Cast
Festival Mask
Variant: Pre 3.19.0
Variant: Current
` + selection + `Implicits: 0
{range:0.5}+(20-30) to Dexterity
{variant:1}30% reduced Damage
{variant:2}25% reduced Damage
{variant:1}{range:0.5}+(20-30) to maximum Life
{variant:2}{range:0.5}+(60-100) to maximum Life
{variant:1}{range:0.5}+(20-30) to maximum Mana
{variant:2}{range:0.5}+(60-100) to maximum Mana
{variant:1}You and nearby allies gain 15% increased Damage
{variant:2}You and nearby allies gain 50% increased Damage
`
}

// Parse keeps the lines of a unique's selected variants and sets each range at its roll.
func TestParseUniqueVariants(t *testing.T) {
	t.Parallel()
	r, _ := realReader(t)
	current := []string{
		"+25 to Dexterity", "25% reduced Damage", "+80 to maximum Life", "+80 to maximum Mana",
		"You and nearby allies gain 50% increased Damage",
	}
	tests := []struct {
		name, text string
		want       []string
	}{
		{name: "selected", text: leerCast("Selected Variant: 2\n"), want: current},
		{name: "none selected is the last", text: leerCast(""), want: current},
		{
			name: "first and alt",
			text: leerCast("Selected Variant: 1\nHas Alt Variant: true\nSelected Alt Variant: 2\n"),
			want: []string{
				"+25 to Dexterity", "30% reduced Damage", "25% reduced Damage", "+25 to maximum Life", "+80 to maximum Life",
				"+25 to maximum Mana", "+80 to maximum Mana", "You and nearby allies gain 15% increased Damage",
				"You and nearby allies gain 50% increased Damage",
			},
		},
		{
			name: "of several variants",
			text: `Rarity: UNIQUE
Wake of Destruction
Mesh Boots
Variant: Pre 2.6.0
Variant: Pre 3.11.0
Variant: Current
Selected Variant: 3
Implicits: 0
Adds 1 to 120 Lightning Damage to Attacks
{range:0.5}(20-60)% increased Armour and Energy Shield
{range:0.5}Gain (10-20) Life per Enemy Killed
{variant:2,3}15% increased Movement Speed
{variant:1,2}10% chance to Cause Monsters to Flee
{variant:3}Drops Shocked Ground while moving, lasting 2 seconds
`,
			want: []string{
				"Adds 1 to 120 Lightning Damage to Attacks", "40% increased Armour and Energy Shield",
				"Gain 15 Life per Enemy Killed", "15% increased Movement Speed",
				"Drops Shocked Ground while moving, lasting 2 seconds",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			it, warnings, err := r.Parse(tt.text)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, m := range it.Mods {
				got = append(got, m.Text)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("lines (-want +got):\n%s", diff)
			}
			for _, w := range warnings {
				if w.Code == build.ItemLineUnread {
					t.Errorf("warning %v", w)
				}
			}
		})
	}
}

// The rolled lines of Leer Cast read as the stats of their values.
func TestParseUniqueRolledStats(t *testing.T) {
	t.Parallel()
	r, _ := realReader(t)
	it, _, err := r.Parse(leerCast("Selected Variant: 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int32{}
	for _, m := range it.Mods {
		for _, sv := range m.Stats {
			got[sv.Stat] += sv.Value
		}
	}
	want := map[string]int32{
		"additional_dexterity": 25, "damage_+%": -25, "base_maximum_life": 80, "base_maximum_mana": 80,
		"local_display_aura_damage_+%": 50,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("stats (-want +got):\n%s", diff)
	}
}
