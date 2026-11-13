package dataset_test

import (
	"os"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/dataset"
)

// withTimeless returns sample with the standard tree's None, Vaal and Karui timeless versions
// and the English names of the Vaal replacements.
func withTimeless() *dataset.Dataset {
	d := sample()
	d.Texts[0].TimelessNames = []dataset.Name{{ID: "vaal_keystone_1", Name: "Divine Flesh"}, {ID: "vaal_notable_random_1", Name: "Tempered by War"}}
	d.Trees[0].TimelessVersions = []dataset.TimelessVersion{
		{ID: "None"},
		{
			ID: "Vaal", SmallAttributeReplaced: true, SmallNormalReplaced: true, NotableReplacementChance: 100,
			Replacements: []dataset.TimelessReplacement{
				{
					ID: "vaal_keystone_1", Kinds: []dataset.TimelessKind{dataset.TimelessKeystone},
					Stats: []dataset.StatRange{{Stat: 2, Min: 1, Max: 1}}, Weight: 100,
					Conqueror: dataset.TimelessConqueror{Index: 1},
				},
				{
					// Row order of stats is kept: 2 before 0.
					ID: "vaal_notable_random_1", Kinds: []dataset.TimelessKind{dataset.TimelessNotable},
					Stats:  []dataset.StatRange{{Stat: 2, Min: 10, Max: 20}, {Stat: 0, Min: -5, Max: 5}},
					Weight: 50, Additions: dataset.ValueRange{Min: 1, Max: 2},
				},
			},
			Additions: []dataset.TimelessAddition{{
				ID: "vaal_small_fire_damage", Kinds: []dataset.TimelessKind{dataset.TimelessSmallAttribute, dataset.TimelessNotable},
				Stats: []dataset.StatRange{{Stat: 1, Min: 7, Max: 12}}, Weight: 100,
			}},
		},
		{
			ID: "Karui", SmallAttributeAdditions: dataset.ValueRange{Min: 1, Max: 1},
			SmallNormalAdditions: dataset.ValueRange{Min: 1, Max: 1}, NotableAdditions: dataset.ValueRange{Min: 1, Max: 1},
		},
	}
	return d
}

func TestTimelessRoundTrip(t *testing.T) {
	t.Parallel()
	d := withTimeless()
	dir := t.TempDir()
	m, err := dataset.Write(dir, d)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := readAll(os.DirFS(dir))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	d.Manifest = m
	if diff := gocmp.Diff(d, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("round trip mismatch (-want +got):\n%s", diff)
	}
}

func TestTimelessWriteErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, want string
		change     func(v []dataset.TimelessVersion)
	}{
		{name: "repeated version", want: `timeless version "Vaal": empty or repeated id`, change: func(v []dataset.TimelessVersion) { v[2].ID = "Vaal" }},
		{name: "replacement stat out of range", want: `version "Vaal" replacement 1: stat 9 out of range`, change: func(v []dataset.TimelessVersion) {
			v[1].Replacements[1].Stats[0].Stat = 9
		}},
		{name: "addition stat out of range", want: `version "Vaal" addition 0: stat 3 out of range`, change: func(v []dataset.TimelessVersion) {
			v[1].Additions[0].Stats[0].Stat = 3
		}},
		{name: "unknown kind", want: `version "Vaal" replacement 0: unknown kind`, change: func(v []dataset.TimelessVersion) {
			v[1].Replacements[0].Kinds = []dataset.TimelessKind{0}
		}},
		{name: "range reversed", want: `version "Vaal" addition 0: range reversed`, change: func(v []dataset.TimelessVersion) {
			v[1].Additions[0].Stats[0].Min = 13
		}},
		{name: "name for unknown passive", want: "name for unknown timeless passive vaal_keystone_1", change: func(v []dataset.TimelessVersion) {
			v[1].Replacements[0].ID = "x0"
		}},
		{name: "too many addition stats", want: `version "Vaal" addition 0: 3 stats, at most 2`, change: func(v []dataset.TimelessVersion) {
			v[1].Additions[0].Stats = []dataset.StatRange{{Stat: 0}, {Stat: 1}, {Stat: 2}}
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			d := withTimeless()
			testCase.change(d.Trees[0].TimelessVersions)
			_, err := dataset.Write(t.TempDir(), d)
			checkError(t, err, dataset.ErrInvalid, testCase.want)
		})
	}
}

// TestTimelessNames reads the timeless names from the names file alone.
func TestTimelessNames(t *testing.T) {
	t.Parallel()
	d := withTimeless()
	full := written(t, d)
	s, err := dataset.Open(without(full, dataset.TextFile("en"), dataset.StatsFile, dataset.CharactersFile))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := s.Names("en")
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	if diff := gocmp.Diff(d.Texts[0].TimelessNames, got.TimelessNames); diff != "" {
		t.Errorf("timeless names (-want +got):\n%s", diff)
	}
}
