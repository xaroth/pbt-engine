package poe1_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/resolve"
	rules "github.com/xaroth/pbt-engine/rules/poe1"
)

// masteries are n allocated Life Masteries and one Fire Mastery, each counting itself, and on
// the first Life Mastery the "10% more Maximum Life if you have at least 6 Life Masteries
// allocated" effect.
func masteries(n int) *resolve.Resolved {
	bundles := endgame()
	for i := range n {
		bundles = append(bundles, bundle(resolve.NodeSource{ID: uint32(100 + i)}, resolve.Global, resolve.StatValue{Stat: "number_of_life_masteries", Value: 1}))
	}
	bundles = append(bundles,
		bundle(resolve.NodeSource{ID: 200}, resolve.Global, resolve.StatValue{Stat: "number_of_fire_masteries", Value: 1}),
		bundle(resolve.NodeSource{ID: 100, Effect: 64381}, resolve.Global, resolve.StatValue{Stat: "life_mastery_count_maximum_life_+%_final", Value: 10}))
	r := resolved(bundles...)
	r.Counters = []string{"number_of_fire_masteries", "number_of_life_masteries"}
	return r
}

func TestMasteryCount(t *testing.T) {
	t.Parallel()
	// level 90: 1134 life
	for n, want := range map[int]float64{5: 1134, 6: 1247, 7: 1247} {
		sh := open(t, masteries(n))
		if got := value(t, sh, "maximum_life"); got != want {
			t.Errorf("%d life masteries: life %v, want %v", n, got, want)
		}
		if got := value(t, sh, "number_of_life_masteries"); got != float64(n) {
			t.Errorf("%d life masteries counted as %v", n, got)
		}
		if got := value(t, sh, "number_of_fire_masteries"); got != 1 {
			t.Errorf("fire masteries counted as %v", got)
		}
		for _, note := range sh.Notes() {
			t.Errorf("note: %s", note)
		}
	}
}

// TestMasteryCountWithoutCounter: on a tree without the counter the effect never holds.
func TestMasteryCountWithoutCounter(t *testing.T) {
	t.Parallel()
	r := masteries(6)
	r.Counters = []string{"number_of_fire_masteries"}
	sh := open(t, r)
	if got := value(t, sh, "maximum_life"); got != 1134 {
		t.Errorf("life %v, want 1134", got)
	}
}

// TestLifeIfNoLifeOnBodyArmour: "15% increased maximum Life if there are no Life Modifiers on
// Equipped Body Armour" applies unconditionally, since builds hold no items and an empty body
// armour slot meets the condition. Level 90: 1134 life, 15% increased gives 1304.1.
func TestLifeIfNoLifeOnBodyArmour(t *testing.T) {
	t.Parallel()
	bundles := append(endgame(),
		bundle(resolve.NodeSource{ID: 100, Effect: 34242}, resolve.Global, resolve.StatValue{Stat: "maximum_life_+%_if_no_life_tags_on_body_armour", Value: 15}))
	sh := open(t, resolved(bundles...))
	if got := value(t, sh, "maximum_life"); got != 1304 {
		t.Errorf("life: got %v, want 1304", got)
	}
	notes := sh.Notes()
	if len(notes) != 1 || notes[0].Kind != rules.Partial || notes[0].Stat != "maximum_life_+%_if_no_life_tags_on_body_armour" {
		t.Errorf("notes: got %v, want one partial note for the body armour condition", notes)
	}
}
