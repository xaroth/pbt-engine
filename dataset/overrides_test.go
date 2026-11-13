package dataset_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
)

func TestTreeOverride(t *testing.T) {
	t.Parallel()
	tree := sample().Trees[0]
	for id, want := range map[string]bool{"Tasalio3": true, "runegraft_8": true, "Tasalio": false, "": false} {
		o, ok := tree.Override(id)
		if ok != want || (ok && o.ID != id) {
			t.Errorf("Override(%q) = %v, %t; want found %t", id, o, ok, want)
		}
	}
}

// TestReadOverrideOfUnknownStat reads a tree whose override names a stat the stats group lacks.
func TestReadOverrideOfUnknownStat(t *testing.T) {
	t.Parallel()
	d := sample()
	d.Stats = append(d.Stats, dataset.Stat{ID: "spells_chance_to_hinder_on_hit_%"})
	d.Trees[0].Overrides[0].Stats = []dataset.StatValue{{Stat: 3, Value: 5}}
	fsys := rewritten(t, written(t, d), dataset.StatsFile, written(t, sample())[dataset.StatsFile].Data)
	s, err := dataset.Open(fsys)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, err = s.Tree("standard")
	checkError(t, err, dataset.ErrInvalid, `tree-standard.fb: unknown stat "spells_chance_to_hinder_on_hit_%"`)
}
