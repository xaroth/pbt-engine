package poe1_test

import (
	"cmp"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/resolve"
	rules "github.com/xaroth/pbt-engine/rules/poe1"
	sempoe1 "github.com/xaroth/pbt-engine/semantics/poe1"
)

// Start nodes added to the fixture tree.
const (
	marauderStart  = 47175
	chieftainStart = 1
)

// fixtureStore is the semantics fixture of version (the real standard tree's nodes with the
// stats the table knows, so it runs without $PBT_DATASETS) plus a Marauder start joined to
// every other node, a Chieftain start joined to the Chieftain nodes and the test constants: the
// fixture has no classes, edges or constants, and the sheets compared here do not depend on them.
func fixtureStore(tb testing.TB, version string) *dataset.Store {
	tb.Helper()
	fx, err := dataset.Open(os.DirFS(filepath.Join("..", "..", "semantics", "poe1", "testdata", "fixture", version)))
	if err != nil {
		tb.Fatal(err)
	}
	d, err := fx.Dataset(dataset.Options{Trees: []string{"standard"}})
	if err != nil {
		tb.Fatal(err)
	}
	tree := d.Trees[0]
	tree.Nodes = slices.Clone(tree.Nodes)
	var main, chief []uint32
	for i := range tree.Nodes {
		n := &tree.Nodes[i]
		switch n.Ascendancy {
		case "":
			main = append(main, n.ID)
			n.Neighbours = []uint32{marauderStart}
		case "Chieftain":
			chief = append(chief, n.ID)
			n.Neighbours = []uint32{chieftainStart}
		}
	}
	tree.Nodes = append(tree.Nodes,
		dataset.Node{ID: marauderStart, Kind: dataset.ClassStart, Neighbours: main},
		dataset.Node{ID: chieftainStart, Kind: dataset.AscendancyStart, Ascendancy: "Chieftain", Neighbours: chief})
	slices.SortFunc(tree.Nodes, func(a, b dataset.Node) int { return cmp.Compare(a.ID, b.ID) })
	tree.Classes = []dataset.Class{{Character: "Metadata/Characters/Str/Str", StartNode: marauderStart, Ascendancies: []dataset.Ascendancy{{ID: "Chieftain"}}}}
	d.Trees = []dataset.Tree{tree}
	for _, c := range constants() {
		d.Constants = append(d.Constants, dataset.Constant{ID: c.ID, Value: c.Value, Per: c.Per})
	}
	d.Manifest = dataset.Manifest{Game: "poe1", Version: version}
	dir := tb.TempDir()
	if _, err := dataset.Write(dir, d); err != nil {
		tb.Fatal(err)
	}
	s, err := dataset.Open(os.DirFS(dir))
	if err != nil {
		tb.Fatal(err)
	}
	return s
}

// chieftain is the level 90 Chieftain of pbt-conformance with the first effect of each of its
// masteries chosen.
func chieftain(tb testing.TB, s *dataset.Store, version string) *build.Build {
	tb.Helper()
	b := &build.Build{Format: 1, Game: "poe1", Version: version, Class: "Metadata/Characters/Str/Str", Ascendancy: "Chieftain",
		Level: 90, Nodes: treeNodes(tb, version), Masteries: map[uint32]uint32{}}
	tree, err := s.Tree("standard")
	if err != nil {
		tb.Fatal(err)
	}
	for _, n := range tree.Nodes {
		if slices.Contains(b.Nodes, n.ID) && n.Kind == dataset.Mastery && len(n.MasteryEffects) > 0 {
			b.Masteries[n.ID] = n.MasteryEffects[0].ID
		}
	}
	return b
}

func checked(tb testing.TB, b *build.Build, s *dataset.Store) *build.Checked {
	tb.Helper()
	c, _, err := build.Check(b, s)
	if err != nil {
		tb.Fatal(err)
	}
	return c
}

func sheetOf(tb testing.TB, c *build.Checked, wholeTree bool, cfg config.Resolved) *rules.Sheet {
	tb.Helper()
	res := resolve.Resolve
	if wholeTree {
		res = resolve.Tree
	}
	r, err := res(c)
	if err != nil {
		tb.Fatal(err)
	}
	tbl, err := sempoe1.Load()
	if err != nil {
		tb.Fatal(err)
	}
	sh, err := rules.Open(r, tbl, cfg)
	if err != nil {
		tb.Fatal(err)
	}
	if wholeTree {
		b := c.Build()
		sh.SetNodes(b.Nodes, b.Masteries)
	}
	return sh
}

// sameOutputs fails unless every catalogue output of a and b has the same bits.
func sameOutputs(tb testing.TB, what string, a, b *rules.Sheet) {
	tb.Helper()
	for _, o := range rules.Outputs() {
		x, err1 := a.Output(o.Key)
		y, err2 := b.Output(o.Key)
		if err1 != nil || err2 != nil || math.Float64bits(x) != math.Float64bits(y) {
			tb.Fatalf("%s: %s: %v (%v) and %v (%v)", what, o.Key, x, err1, y, err2)
		}
	}
}

func TestWholeTreeSheetEqualsBuildSheet(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"3.28.0.16", "3.29.3.3"} {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			s := fixtureStore(t, version)
			c := checked(t, chieftain(t, s, version), s)
			if len(c.Masteries()) == 0 {
				t.Fatal("no mastery chosen; the test would not cover masteries")
			}
			tree := sheetOf(t, c, true, rules.DefaultConfig())
			sameOutputs(t, "chieftain", sheetOf(t, c, false, rules.DefaultConfig()), tree)
			if len(tree.Notes()) != len(sheetOf(t, c, false, rules.DefaultConfig()).Notes()) {
				t.Errorf("notes: whole tree %d, build %d", len(tree.Notes()), len(sheetOf(t, c, false, rules.DefaultConfig()).Notes()))
			}
		})
	}
}

// Random levels, allocations, mastery choices and config on one whole-tree sheet equal a sheet
// opened fresh for each build.
func TestWholeTreeSheetEdits(t *testing.T) {
	t.Parallel()
	const version = "3.29.3.3"
	s := fixtureStore(t, version)
	base := chieftain(t, s, version)
	tree := sheetOf(t, checked(t, base, s), true, rules.DefaultConfig())
	r := rand.New(rand.NewPCG(9, 9))
	stages := []rules.CampaignStage{rules.CampaignComplete, rules.CampaignActs6To10, rules.CampaignActs1To5}
	for step := range 30 {
		b := base.Clone()
		for _, n := range base.Nodes {
			if r.IntN(4) == 0 {
				b.Nodes = slices.DeleteFunc(b.Nodes, func(x uint32) bool { return x == n })
			}
		}
		for n := range b.Masteries {
			if r.IntN(3) == 0 {
				delete(b.Masteries, n)
			}
		}
		var v config.Values
		if err := config.Set(&v, rules.Campaign, stages[r.IntN(len(stages))]); err != nil {
			t.Fatal(err)
		}
		cfg, _ := config.Resolve(rules.ConfigSpecs(), config.Layer{Source: config.FromBuild, Values: v})
		b.Level = 1 + r.IntN(100)
		c := checked(t, b, s)
		cb := c.Build()
		if err := tree.SetLevel(cb.Level); err != nil {
			t.Fatal(err)
		}
		tree.SetNodes(cb.Nodes, cb.Masteries)
		tree.SetConfig(cfg)
		sameOutputs(t, "step "+string(rune('a'+step%26)), sheetOf(t, c, false, cfg), tree)
	}
}

// BenchmarkOpenSheet opens the level 90 Chieftain's sheet from its build and over the whole
// tree (Q2 trades a slower open for edits that only switch sources), then switches the whole
// tree sheet to the build's nodes.
func BenchmarkOpenSheet(b *testing.B) {
	const version = "3.29.3.3"
	s := fixtureStore(b, version)
	c := checked(b, chieftain(b, s, version), s)
	cfg := rules.DefaultConfig()
	b.Run("build", func(b *testing.B) {
		for b.Loop() {
			sheetOf(b, c, false, cfg)
		}
	})
	b.Run("whole tree", func(b *testing.B) {
		for b.Loop() {
			sheetOf(b, c, true, cfg)
		}
	})
	sh := sheetOf(b, c, true, cfg)
	cb := c.Build()
	b.Run("set nodes and read life", func(b *testing.B) {
		for i := 0; b.Loop(); i++ {
			nodes := cb.Nodes
			if i%2 == 0 {
				nodes = nodes[:len(nodes)-1]
			}
			sh.SetNodes(nodes, cb.Masteries)
			if _, err := sh.Output(rules.Life); err != nil {
				b.Fatal(err)
			}
		}
	})
}
