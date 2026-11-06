package build_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

const (
	marauder = "Metadata/Characters/Str/Str"
	ranger   = "Metadata/Characters/Dex/Dex"
)

// testStore is a dataset with two classes on one tree: Marauder starts at 1 with
// ascendancy nodes 10 (Juggernaut) and 11 (Berserker); Ranger starts at 2.
func testStore(t *testing.T) *dataset.Store {
	t.Helper()
	d := &dataset.Dataset{
		Manifest:   dataset.Manifest{Game: "poe1", Version: "3.29.3.3"},
		Stats:      []dataset.Stat{{ID: "base_maximum_life"}},
		Characters: []dataset.Character{{ID: marauder}, {ID: ranger}, {ID: "Metadata/Characters/NotInTree"}},
		Trees: []dataset.Tree{{
			Variant: "standard",
			Classes: []dataset.Class{
				{Character: marauder, StartNode: 1, Ascendancies: []dataset.Ascendancy{{ID: "Juggernaut"}, {ID: "Berserker"}}},
				{Character: ranger, StartNode: 2, Ascendancies: []dataset.Ascendancy{{ID: "Deadeye"}}},
			},
			Nodes: []dataset.Node{
				{ID: 1, Kind: dataset.ClassStart, Neighbours: []uint32{3}},
				{ID: 2, Kind: dataset.ClassStart, Neighbours: []uint32{3}},
				{ID: 3, Neighbours: []uint32{1, 2}, Stats: []dataset.StatValue{{Stat: 0, Value: 10}}},
				{ID: 10, Ascendancy: "Juggernaut"},
				{ID: 11, Ascendancy: "Berserker"},
			},
		}},
	}
	dir := t.TempDir()
	if _, err := dataset.Write(dir, d); err != nil {
		t.Fatal(err)
	}
	s, err := dataset.Open(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func juggernaut() *build.Build {
	return &build.Build{
		Format: build.Format, Game: "poe1", Version: "3.29", Variant: "standard",
		Class: marauder, Ascendancy: "Juggernaut", Level: 90, Nodes: []uint32{10, 3, 1},
		Scenario: build.Scenario{Campaign: build.CampaignActs6To10, Bandits: build.BanditsHelpOak},
	}
}

func TestEncodeParse(t *testing.T) {
	t.Parallel()
	b := juggernaut()
	data, err := b.Encode()
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "format": 1,
  "game": "poe1",
  "version": "3.29",
  "variant": "standard",
  "class": "Metadata/Characters/Str/Str",
  "ascendancy": "Juggernaut",
  "level": 90,
  "nodes": [
    1,
    3,
    10
  ],
  "scenario": {
    "campaign": "acts-6-10",
    "bandits": "help-oak"
  }
}
`
	if diff := cmp.Diff(want, string(data)); diff != "" {
		t.Errorf("Encode (-want +got):\n%s", diff)
	}
	got, err := build.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	b.Nodes = []uint32{1, 3, 10}
	if diff := cmp.Diff(b, got); diff != "" {
		t.Errorf("Parse(Encode) (-want +got):\n%s", diff)
	}
}

func TestDefaults(t *testing.T) {
	t.Parallel()
	want := &build.Build{
		Format: 1, Game: "poe1", Version: "3.29", Variant: build.DefaultVariant, Class: "x", Level: 1, Nodes: []uint32{},
		Scenario: build.Scenario{Campaign: build.CampaignComplete},
	}
	parsed, err := build.Parse([]byte(`{"format":1,"game":"poe1","version":"3.29","class":"x","level":1,"nodes":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, parsed); diff != "" {
		t.Errorf("Parse (-want +got):\n%s", diff)
	}
	data, err := (&build.Build{Game: "poe1", Version: "3.29", Class: "x", Level: 1}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := build.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, encoded); diff != "" {
		t.Errorf("Parse(Encode) of a build without defaults (-want +got):\n%s", diff)
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, data, want string
	}{
		{"not json", `{`, "build: unexpected EOF"},
		{"unknown field", `{"format":1,"items":[]}`, `build: json: unknown field "items"`},
		{"other format", `{"format":2}`, "build: format 2, want 1"},
		{"no format", `{"game":"poe1"}`, "build: format 0, want 1"},
		{"trailing data", `{"format":1} {}`, "build: data after the build"},
		{"trailing brace", `{"format":1}}`, "build: data after the build"},
		{"trailing bracket", `{"format":1}]`, "build: data after the build"},
		{"wrong type", `{"format":1,"level":"90"}`, "build: json: cannot unmarshal string into Go struct field Build.level"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := build.Parse([]byte(testCase.data))
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("got %v, want an error containing %q", err, testCase.want)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()
	s := testStore(t)
	b := juggernaut()
	b.Variant, b.Scenario.Campaign = "", ""
	c, err := build.Check(b, s)
	if err != nil {
		t.Fatal(err)
	}
	var ids []uint32
	for _, n := range c.Nodes() {
		ids = append(ids, n.ID)
	}
	if diff := cmp.Diff([]uint32{1, 3, 10}, ids); diff != "" {
		t.Errorf("Nodes (-want +got):\n%s", diff)
	}
	got := []any{c.Class().StartNode, c.Character().ID, c.Tree().Variant, c.Store() == s}
	if diff := cmp.Diff([]any{uint32(1), marauder, "standard", true}, got); diff != "" {
		t.Errorf("class start, character, tree, version (-want +got):\n%s", diff)
	}
	want := *juggernaut()
	want.Nodes = []uint32{1, 3, 10}
	want.Scenario.Campaign = build.CampaignComplete
	if diff := cmp.Diff(want, c.Build()); diff != "" {
		t.Errorf("Build (-want +got):\n%s", diff)
	}
}

func TestCheckProblems(t *testing.T) {
	t.Parallel()
	s := testStore(t)
	type problem struct {
		Kind error
		Node uint32
	}
	testCases := []struct {
		name string
		edit func(b *build.Build)
		want []problem
	}{
		{"game", func(b *build.Build) { b.Game = "poe2" }, []problem{{build.ErrGame, 0}}},
		{"variant", func(b *build.Build) { b.Variant = "ruthless" }, []problem{{build.ErrVariant, 0}}},
		{"class not in tree", func(b *build.Build) { b.Class = "Metadata/Characters/NotInTree"; b.Ascendancy = ""; b.Nodes = nil }, []problem{{build.ErrClass, 0}}},
		{"class unknown", func(b *build.Build) { b.Class = "x"; b.Ascendancy = ""; b.Nodes = nil }, []problem{{build.ErrClass, 0}}},
		{"ascendancy of another class", func(b *build.Build) { b.Ascendancy = "Deadeye"; b.Nodes = nil }, []problem{{build.ErrAscendancy, 0}}},
		{"level 0", func(b *build.Build) { b.Level = 0 }, []problem{{build.ErrLevel, 0}}},
		{"level 101", func(b *build.Build) { b.Level = 101 }, []problem{{build.ErrLevel, 0}}},
		{"campaign", func(b *build.Build) { b.Scenario.Campaign = "act3" }, []problem{{build.ErrCampaign, 0}}},
		{"bandits", func(b *build.Build) { b.Scenario.Bandits = "help-eramir" }, []problem{{build.ErrBandits, 0}}},
		{"unknown node", func(b *build.Build) { b.Nodes = append(b.Nodes, 99) }, []problem{{build.ErrNodeUnknown, 99}}},
		{"node twice", func(b *build.Build) { b.Nodes = append(b.Nodes, 3) }, []problem{{build.ErrNodeRepeated, 3}}},
		{"node of another ascendancy", func(b *build.Build) { b.Nodes = append(b.Nodes, 11) }, []problem{{build.ErrNodeAscendancy, 11}}},
		{"start of another class", func(b *build.Build) { b.Nodes = append(b.Nodes, 2) }, []problem{{build.ErrNodeOtherClassStart, 2}}},
		{"every problem at once", func(b *build.Build) { b.Level = 0; b.Nodes = append(b.Nodes, 99, 2) },
			[]problem{{build.ErrLevel, 0}, {build.ErrNodeOtherClassStart, 2}, {build.ErrNodeUnknown, 99}}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := juggernaut()
			testCase.edit(b)
			_, err := build.Check(b, s)
			ce, ok := errors.AsType[*build.CheckError](err)
			if !ok {
				t.Fatalf("got %v, want a *CheckError", err)
			}
			var got []problem
			for _, p := range ce.Problems {
				got = append(got, problem{p.Kind, p.Node})
			}
			if diff := cmp.Diff(testCase.want, got, cmp.Comparer(func(a, b error) bool { return a == b })); diff != "" {
				t.Errorf("problems (-want +got):\n%s", diff)
			}
			if !errors.Is(err, testCase.want[0].Kind) {
				t.Errorf("errors.Is(%v, %v): got false, want true", err, testCase.want[0].Kind)
			}
		})
	}
}
