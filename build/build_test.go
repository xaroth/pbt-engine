package build_test

import (
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

// testVersion is a dataset with two classes on one tree: Marauder starts at 1 with
// ascendancy nodes 10 (Juggernaut) and 11 (Berserker); Ranger starts at 2.
func testVersion(t *testing.T, game, version string) *dataset.Version {
	t.Helper()
	d := &dataset.Dataset{
		Manifest:   dataset.Manifest{Game: game, Version: version},
		Stats:      []dataset.Stat{{ID: "base_maximum_life"}},
		Characters: []dataset.Character{{ID: marauder}, {ID: ranger}},
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
	v, err := dataset.Open(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func juggernaut() *build.Build {
	return &build.Build{
		Format: build.Format, Game: "poe1", Version: "3.29", Variant: "standard",
		Class: marauder, Ascendancy: "Juggernaut", Level: 90, Nodes: []uint32{10, 3, 1},
		Scenario: build.Scenario{Bandit: build.Oak, Penalty: build.AfterAct5},
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
    "bandit": "oak",
    "resistancePenalty": "act5"
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

func TestParseDefaults(t *testing.T) {
	t.Parallel()
	got, err := build.Parse([]byte(`{"format":1,"game":"poe1","version":"3.29","class":"x","level":1,"nodes":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := &build.Build{Format: 1, Game: "poe1", Version: "3.29", Variant: build.DefaultVariant, Class: "x", Level: 1, Nodes: []uint32{}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
	data, err := (&build.Build{Game: "poe1"}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"nodes": []`) || strings.Contains(string(data), "scenario") {
		t.Errorf("Encode of empty build: got %s, want empty nodes and no scenario", data)
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, data string
	}{
		{"not json", `{`},
		{"unknown field", `{"format":1,"items":[]}`},
		{"other format", `{"format":2}`},
		{"no format", `{"game":"poe1"}`},
		{"trailing data", `{"format":1} {}`},
		{"wrong type", `{"format":1,"level":"90"}`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := build.Parse([]byte(testCase.data)); err == nil {
				t.Error("got nil error, want error")
			}
		})
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()
	v := testVersion(t, "poe1", "3.29.3.3")
	c, err := build.Check(juggernaut(), v)
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
	if c.Class().StartNode != 1 || c.Character().ID != marauder || c.Level() != 90 || c.Ascendancy() != "Juggernaut" {
		t.Errorf("got class start %d, character %s, level %d, ascendancy %s; want 1, %s, 90, Juggernaut",
			c.Class().StartNode, c.Character().ID, c.Level(), c.Ascendancy(), marauder)
	}
	if c.Tree().Variant != "standard" || c.Version() != v {
		t.Errorf("got tree %s, version %p, want standard, %p", c.Tree().Variant, c.Version(), v)
	}
	if got := c.Scenario(); got != (build.Scenario{Bandit: build.Oak, Penalty: build.AfterAct5}) {
		t.Errorf("Scenario: got %+v, want oak, act5", got)
	}
}

func TestCheckErrors(t *testing.T) {
	t.Parallel()
	v := testVersion(t, "poe1", "3.29.3.3")
	testCases := []struct {
		name string
		edit func(b *build.Build)
		want string
	}{
		{"game", func(b *build.Build) { b.Game = "poe2" }, `game "poe2"`},
		{"variant", func(b *build.Build) { b.Variant = "ruthless" }, "no ruthless tree"},
		{"class", func(b *build.Build) { b.Class = "Metadata/Characters/Nope"; b.Ascendancy = ""; b.Nodes = nil }, "not in the standard tree"},
		{"ascendancy of another class", func(b *build.Build) { b.Ascendancy = "Deadeye"; b.Nodes = nil }, `ascendancy "Deadeye"`},
		{"level 0", func(b *build.Build) { b.Level = 0 }, "level 0"},
		{"level 101", func(b *build.Build) { b.Level = 101 }, "level 101"},
		{"unknown node", func(b *build.Build) { b.Nodes = append(b.Nodes, 99) }, "node 99 not in"},
		{"node twice", func(b *build.Build) { b.Nodes = append(b.Nodes, 3) }, "node 3 allocated twice"},
		{"node of another ascendancy", func(b *build.Build) { b.Nodes = append(b.Nodes, 11) }, "belongs to ascendancy Berserker"},
		{"start of another class", func(b *build.Build) { b.Nodes = append(b.Nodes, 2) }, "start of another class"},
		{"bandit", func(b *build.Build) { b.Scenario.Bandit = "eramir" }, `bandit "eramir"`},
		{"penalty", func(b *build.Build) { b.Scenario.Penalty = "act3" }, `resistance penalty "act3"`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := juggernaut()
			testCase.edit(b)
			_, err := build.Check(b, v)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("got %v, want an error containing %q", err, testCase.want)
			}
		})
	}

	b := juggernaut()
	b.Level, b.Nodes = 0, append(b.Nodes, 99)
	_, err := build.Check(b, v)
	if err == nil || !strings.Contains(err.Error(), "level 0") || !strings.Contains(err.Error(), "node 99") {
		t.Errorf("two problems: got %v, want both reported", err)
	}
}

func TestOpen(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, version := range []string{"3.28.0.16", "3.29.3.3"} {
		d := &dataset.Dataset{
			Manifest:   dataset.Manifest{Game: "poe1", Version: version},
			Characters: []dataset.Character{{ID: marauder}},
			Trees: []dataset.Tree{{Variant: "standard", Classes: []dataset.Class{{Character: marauder, StartNode: 1}},
				Nodes: []dataset.Node{{ID: 1, Kind: dataset.ClassStart}}}},
		}
		dir := root + "/" + version
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := dataset.Write(dir, d); err != nil {
			t.Fatal(err)
		}
	}
	r, err := dataset.OpenRoot(os.DirFS(root))
	if err != nil {
		t.Fatal(err)
	}
	b := &build.Build{Format: 1, Game: "poe1", Version: "3.28", Class: marauder, Level: 1, Nodes: []uint32{1}}
	c, err := build.Open(r, b)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Version().Name(); got != "3.28.0.16" {
		t.Errorf("version: got %s, want 3.28.0.16", got)
	}
	b.Version = "3.30"
	if _, err := build.Open(r, b); err == nil {
		t.Error("unknown version: got nil error, want error")
	}
}
