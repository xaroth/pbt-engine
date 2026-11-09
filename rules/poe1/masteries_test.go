package poe1_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/resolve"
)

// lifeMasteries are n allocated Life Masteries, each counting itself, and on the first the
// "10% more Maximum Life if you have at least 6 Life Masteries allocated" effect.
func lifeMasteries(n int) []resolve.Bundle {
	var out []resolve.Bundle
	for i := range n {
		id := uint32(100 + i)
		out = append(out, bundle(resolve.NodeSource{ID: id}, resolve.Global, resolve.StatValue{Stat: "number_of_life_masteries", Value: 1}))
	}
	return append(out, bundle(resolve.NodeSource{ID: 100, Effect: 64381}, resolve.Global,
		resolve.StatValue{Stat: "life_mastery_count_maximum_life_+%_final", Value: 10}))
}

func TestLifeMasteryCount(t *testing.T) {
	t.Parallel()
	// level 90: 1134 life
	for n, want := range map[int]float64{5: 1134, 6: 1247, 7: 1247} {
		sh := open(t, resolved(append(endgame(), lifeMasteries(n)...)...))
		if got := value(t, sh, "maximum_life"); got != want {
			t.Errorf("%d life masteries: life %v, want %v", n, got, want)
		}
		if got := value(t, sh, "number_of_life_masteries"); got != float64(n) {
			t.Errorf("%d life masteries counted as %v", n, got)
		}
		for _, note := range sh.Notes() {
			if note.Stat == "life_mastery_count_maximum_life_+%_final" || note.Stat == "number_of_life_masteries" {
				t.Errorf("note: %s", note)
			}
		}
	}
}
