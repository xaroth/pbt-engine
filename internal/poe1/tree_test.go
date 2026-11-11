package poe1_test

import (
	"cmp"
	"errors"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// Start nodes added to the fixture tree.
const (
	marauderStart  = 47175
	chieftainStart = 1
)

// standardOf reads the tables of s other than trees and texts, and its standard tree.
func standardOf(tb testing.TB, s *dataset.Store) (*dataset.Dataset, *dataset.Tree) {
	tb.Helper()
	stats, err := s.Stats()
	if err != nil {
		tb.Fatal(err)
	}
	characters, err := s.Characters()
	if err != nil {
		tb.Fatal(err)
	}
	quests, err := s.Quests()
	if err != nil {
		tb.Fatal(err)
	}
	standard, err := s.Tree("standard")
	if err != nil {
		tb.Fatal(err)
	}
	return &dataset.Dataset{Stats: stats, Characters: characters, Quests: quests}, standard
}

// fixtureStore is the semantics fixture of version (the real standard tree's nodes with the
// stats the table knows, so it runs without $PBT_DATASETS) plus a Marauder start joined to
// every other node, a Chieftain start joined to the Chieftain nodes and the test constants: the
// fixture has no classes, edges or constants, and the sheets compared here do not depend on them.
func fixtureStore(tb testing.TB, version string) *dataset.Store {
	tb.Helper()
	fx, err := dataset.Open(os.DirFS(filepath.Join("testdata", "fixture", version)))
	if err != nil {
		tb.Fatal(err)
	}
	d, standard := standardOf(tb, fx)
	tree := *standard
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

func sheetOf(tb testing.TB, c *build.Checked, wholeTree bool, cfg config.Resolved) *poe1.Sheet {
	tb.Helper()
	res := resolve.Resolve
	if wholeTree {
		res = resolve.Tree
	}
	r, err := res(c)
	if err != nil {
		tb.Fatal(err)
	}
	tbl, err := poe1.Load()
	if err != nil {
		tb.Fatal(err)
	}
	sh, err := poe1.Open(r, tbl, cfg)
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
func sameOutputs(tb testing.TB, what string, a, b *poe1.Sheet) {
	tb.Helper()
	for _, o := range poe1.Outputs() {
		x, err1 := a.Output(o.Key)
		y, err2 := b.Output(o.Key)
		if err1 != nil || err2 != nil || math.Float64bits(x) != math.Float64bits(y) {
			tb.Fatalf("%s: %s: %v (%v) and %v (%v)", what, o.Key, x, err1, y, err2)
		}
	}
}

func TestWholeTreeSheetEqualsBuildSheet(t *testing.T) {
	t.Parallel()
	for _, version := range realdata.Versions() {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			s := fixtureStore(t, version)
			c := checked(t, chieftain(t, s, version), s)
			if len(c.Masteries()) == 0 {
				t.Fatal("no mastery chosen; the test would not cover masteries")
			}
			tree := sheetOf(t, c, true, poe1.DefaultConfig())
			sameOutputs(t, "chieftain", sheetOf(t, c, false, poe1.DefaultConfig()), tree)
			if len(tree.Notes()) != len(sheetOf(t, c, false, poe1.DefaultConfig()).Notes()) {
				t.Errorf("notes: whole tree %d, build %d", len(tree.Notes()), len(sheetOf(t, c, false, poe1.DefaultConfig()).Notes()))
			}
		})
	}
}

// Random levels, allocations, mastery choices and config on one whole-tree sheet equal a sheet
// opened fresh for each build.
func TestWholeTreeSheetEdits(t *testing.T) {
	t.Parallel()
	const version = realdata.Current
	s := fixtureStore(t, version)
	base := chieftain(t, s, version)
	tree := sheetOf(t, checked(t, base, s), true, poe1.DefaultConfig())
	r := rand.New(rand.NewPCG(9, 9))
	stages := []poe1.CampaignStage{poe1.CampaignComplete, poe1.CampaignActs6To10, poe1.CampaignActs1To5}
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
		if err := config.Set(&v, poe1.Campaign, stages[r.IntN(len(stages))]); err != nil {
			t.Fatal(err)
		}
		cfg, _ := config.Resolve(poe1.ConfigSpecs(), config.Layer{Source: config.FromBuild, Values: v})
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

// BenchmarkOpenSheet times poe1.Open of the level 90 Chieftain on the fixture tree, from its
// build and over the whole tree, resolved beforehand (target: < 10 ms
// native).
func BenchmarkOpenSheet(b *testing.B) {
	const version = realdata.Current
	s := fixtureStore(b, version)
	c := checked(b, chieftain(b, s, version), s)
	tbl, err := poe1.Load()
	if err != nil {
		b.Fatal(err)
	}
	for name, res := range map[string]func(*build.Checked) (*resolve.Resolved, error){"build": resolve.Resolve, "whole tree": resolve.Tree} {
		r, err := res(c)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := poe1.Open(r, tbl, poe1.DefaultConfig()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkSwitchNode switches one node on a whole-tree sheet in a checkpoint, reads life and
// reverts, as a sweep's fast path does (target per what-if: < 0.2 ms native), and switches
// a whole allocation with SetNodes.
func BenchmarkSwitchNode(b *testing.B) {
	const version = realdata.Current
	s := fixtureStore(b, version)
	c := checked(b, chieftain(b, s, version), s)
	sh := sheetOf(b, c, true, poe1.DefaultConfig())
	cb := c.Build()
	if _, err := sh.Output(poe1.Life); err != nil {
		b.Fatal(err)
	}
	b.Run("one node", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			cp := sh.Mark()
			sh.SetNode(cb.Nodes[len(cb.Nodes)-1], false)
			if _, err := sh.Output(poe1.Life); err != nil {
				b.Fatal(err)
			}
			if err := sh.Revert(cp); err != nil {
				b.Fatal(err)
			}
		}
		sh.Commit()
	})
	b.Run("whole allocation", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; b.Loop(); i++ {
			nodes := cb.Nodes
			if i%2 == 0 {
				nodes = nodes[:len(nodes)-1]
			}
			sh.SetNodes(nodes, cb.Masteries)
			if _, err := sh.Output(poe1.Life); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// outputs reads every catalogue output's bits.
func outputs(tb testing.TB, sh *poe1.Sheet) []uint64 {
	tb.Helper()
	out := make([]uint64, 0, len(poe1.Outputs()))
	for _, o := range poe1.Outputs() {
		v, err := sh.Output(o.Key)
		if err != nil {
			tb.Fatal(err)
		}
		out = append(out, math.Float64bits(v))
	}
	return out
}

// Switching one node or mastery effect in a checkpoint equals SetNodes with it, and Revert
// gives back the bits before.
func TestSetNodeAndRevert(t *testing.T) {
	t.Parallel()
	const version = realdata.Current
	s := fixtureStore(t, version)
	base := checked(t, chieftain(t, s, version), s)
	sh := sheetOf(t, base, true, poe1.DefaultConfig())
	before := outputs(t, sh)
	tree, err := s.Tree("standard")
	if err != nil {
		t.Fatal(err)
	}
	cb := base.Build()
	tried := 0
	for _, n := range tree.Nodes {
		if slices.Contains(cb.Nodes, n.ID) || n.Ascendancy != "" || tried >= 25 {
			continue
		}
		with := cb.Clone()
		with.Nodes = append(with.Nodes, n.ID)
		cp := sh.Mark()
		if n.Kind == dataset.Mastery && len(n.MasteryEffects) > 0 {
			with.Masteries[n.ID] = n.MasteryEffects[0].ID
			sh.SetMasteryEffect(n.ID, n.MasteryEffects[0].ID, true)
		} else if !sh.SetNode(n.ID, true) {
			t.Fatalf("node %d with stats not on the sheet", n.ID)
		}
		want := sheetOf(t, base, true, poe1.DefaultConfig())
		want.SetNodes(with.Nodes, with.Masteries)
		if !slices.Equal(outputs(t, want), outputs(t, sh)) {
			t.Fatalf("node %d: SetNode differs from SetNodes", n.ID)
		}
		if err := sh.Revert(cp); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(before, outputs(t, sh)) {
			t.Fatalf("node %d: revert did not restore the outputs", n.ID)
		}
		tried++
	}
	sh.Commit()
	if sh.SetNode(999999999, true) {
		t.Error("an unknown node reported on the sheet")
	}
}

// A level not seen yet cannot be added inside a checkpoint; the reviewer's sequence (Mark,
// SetLevel(37), Revert, Commit, SetLevel(37)) now gives level 37.
func TestSetLevelInCheckpoint(t *testing.T) {
	t.Parallel()
	const version = realdata.Current
	s := fixtureStore(t, version)
	c := checked(t, chieftain(t, s, version), s)
	sh := sheetOf(t, c, true, poe1.DefaultConfig())
	cp := sh.Mark()
	if err := sh.SetLevel(37); !errors.Is(err, poe1.ErrCheckpointOpen) {
		t.Fatalf("add in checkpoint: %v", err)
	}
	if err := sh.SetLevel(90); err != nil {
		t.Errorf("switch to a known level in checkpoint: %v", err)
	}
	if err := sh.Revert(cp); err != nil {
		t.Fatal(err)
	}
	sh.Commit()
	if err := sh.SetLevel(37); err != nil {
		t.Fatal(err)
	}
	b := c.Build()
	b.Level = 37
	fresh := sheetOf(t, checked(t, &b, s), false, poe1.DefaultConfig())
	if !slices.Equal(outputs(t, fresh), outputs(t, sh)) {
		t.Error("level 37 after the checkpoint differs from a fresh level 37 sheet")
	}
}
