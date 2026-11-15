package poe1_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/internal/resolve"
)

// Devotion counts in whole steps of 10 for "+2% to all Elemental Resistances per 10 Devotion",
// and "+1% to all maximum Resistances if you have at least 150 Devotion" holds from 150. At the
// Kitava penalty the elemental resistances are -60 and their maximum 75.
func TestDevotion(t *testing.T) {
	t.Parallel()
	for devotion, want := range map[float64][2]float64{
		0:   {-60, 75},
		19:  {-58, 75},
		149: {-32, 75},
		150: {-30, 76},
	} {
		sh := open(t, resolved(endgame(
			resolve.StatValue{Stat: "base_devotion", Value: devotion},
			resolve.StatValue{Stat: "elemental_resistance_%_per_10_devotion", Value: 2},
			resolve.StatValue{Stat: "additional_maximum_all_resistances_%_at_devotion_threshold", Value: 1},
		)...))
		if got := value(t, sh, "fire_resistance"); got != want[0] {
			t.Errorf("devotion %v: fire resistance %v, want %v", devotion, got, want[0])
		}
		if got := value(t, sh, "maximum_fire_resistance"); got != want[1] {
			t.Errorf("devotion %v: maximum fire resistance %v, want %v", devotion, got, want[1])
		}
		for _, note := range sh.Notes() {
			t.Errorf("devotion %v: note: %s", devotion, note)
		}
	}
}
