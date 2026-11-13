package timeless_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/internal/poe1/timeless"
)

// outcome is a Result in comparable form.
type outcome struct {
	Replaced    bool
	Replacement int
	Rolls       [4]int32
	Additions   []timeless.Rolled
}

func view(r timeless.Result) outcome {
	return outcome{Replaced: r.Replaced, Replacement: r.Replacement, Rolls: r.Rolls, Additions: r.Additions()}
}

func newTree(tb testing.TB, t timeless.Tables) *timeless.Tree {
	tb.Helper()
	tree, err := timeless.NewTree(t)
	if err != nil {
		tb.Fatal(err)
	}
	return tree
}

func smallKinds() []timeless.Kind {
	return []timeless.Kind{timeless.SmallAttribute, timeless.SmallNormal}
}

func notableKind() []timeless.Kind { return []timeless.Kind{timeless.Notable} }

// vaalShaped has the shape of the game's Vaal version (Glorious Vanity): every passive replaced,
// 38 small and 36 notable replacements, two of which take 3 to 4 additions, and 38 notable
// additions. Values are made up.
func vaalShaped() timeless.Tables {
	t := timeless.Tables{Version: timeless.Version{
		ID: "Vaal", SmallAttributeReplaced: true, SmallNormalReplaced: true, NotableReplacementChance: 100,
	}}
	for i := range int32(38) {
		t.Replacements = append(t.Replacements, timeless.Replacement{
			Kinds: smallKinds(), Stats: []timeless.Range{{Min: 1 + i%5, Max: 10 + i%7}}, Weight: 100,
		})
	}
	for i := range int32(34) {
		t.Replacements = append(t.Replacements, timeless.Replacement{
			Kinds: notableKind(), Stats: []timeless.Range{{Min: 5, Max: 10 + i}, {Min: 20, Max: 30}}, Weight: 100,
		})
	}
	for range 2 {
		t.Replacements = append(t.Replacements, timeless.Replacement{
			Kinds: notableKind(), Weight: 100, Additions: timeless.Range{Min: 3, Max: 4},
		})
	}
	for c := range uint32(4) {
		t.Replacements = append(t.Replacements, timeless.Replacement{
			Kinds: []timeless.Kind{timeless.Keystone}, Stats: []timeless.Range{{Min: 1, Max: 1}}, Weight: 100,
			Conqueror: timeless.Conqueror{Index: c + 1},
		})
	}
	for i := range int32(38) {
		t.Additions = append(t.Additions, timeless.Addition{
			Kinds: notableKind(), Stats: []timeless.Range{{Min: 2, Max: 6 + i%4}}, Weight: 100,
		})
	}
	return t
}

// TestConquer pins picks and rolls on the Vaal-shaped tables, as this implementation gives them;
// the implementation agrees with Path of Building's lookup tables on every node and seed they
// hold for the real tables.
func TestConquer(t *testing.T) {
	t.Parallel()
	tree := newTree(t, vaalShaped())
	testCases := []struct {
		name string
		seed uint32
		node timeless.Node
		want outcome
	}{
		{
			name: "small replaced",
			seed: 100,
			node: timeless.Node{ID: 26196, Kind: timeless.SmallAttribute},
			want: outcome{Replaced: true, Replacement: 11, Rolls: [4]int32{6}, Additions: []timeless.Rolled{}},
		},
		{
			name: "notable replaced with two rolls",
			seed: 4321,
			node: timeless.Node{ID: 6230, Kind: timeless.Notable},
			want: outcome{Replaced: true, Replacement: 41, Rolls: [4]int32{7, 22}, Additions: []timeless.Rolled{}},
		},
		{
			name: "notable replaced with additions",
			seed: 129,
			node: timeless.Node{ID: 6230, Kind: timeless.Notable},
			want: outcome{Replaced: true, Replacement: 73, Additions: []timeless.Rolled{
				{Addition: 26, Rolls: [2]int32{3}}, {Addition: 2, Rolls: [2]int32{7}}, {Addition: 18, Rolls: [2]int32{8}},
			}},
		},
		{
			name: "keystone of the conqueror",
			seed: 8000,
			node: timeless.Node{ID: 1, Kind: timeless.Keystone},
			want: outcome{Replaced: true, Replacement: 74, Rolls: [4]int32{1}, Additions: []timeless.Rolled{}},
		},
		{
			name: "jewel socket unchanged",
			seed: 100,
			node: timeless.Node{ID: 1, Kind: timeless.JewelSocket},
			want: outcome{Replacement: -1, Additions: []timeless.Rolled{}},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got := view(tree.Conquer(timeless.Jewel{Seed: testCase.seed, Conqueror: timeless.Conqueror{Index: 1}}, testCase.node))
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				t.Errorf("Conquer (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConquerKeystoneWithoutConqueror(t *testing.T) {
	t.Parallel()
	tree := newTree(t, vaalShaped())
	got := view(tree.Conquer(timeless.Jewel{Seed: 100, Conqueror: timeless.Conqueror{Index: 9}}, timeless.Node{ID: 1, Kind: timeless.Keystone}))
	want := outcome{Replacement: -1, Additions: []timeless.Rolled{}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Conquer (-want +got):\n%s", diff)
	}
}

// addingTables adds to notables and small passives and replaces notables by chance.
func addingTables(chance int32) timeless.Tables {
	return timeless.Tables{
		Version: timeless.Version{
			ID:                       "Templar",
			SmallAttributeAdditions:  timeless.Range{Min: 1, Max: 1},
			SmallNormalAdditions:     timeless.Range{Min: 1, Max: 1},
			NotableAdditions:         timeless.Range{Min: 1, Max: 2},
			NotableReplacementChance: chance,
		},
		Replacements: []timeless.Replacement{{Kinds: notableKind(), Stats: []timeless.Range{{Min: 1, Max: 9}}, Weight: 10}},
		Additions: []timeless.Addition{
			{Kinds: smallKinds(), Stats: []timeless.Range{{Min: 5, Max: 5}}, Weight: 10},
			{Kinds: notableKind(), Stats: []timeless.Range{{Min: 1, Max: 3}, {Min: -4, Max: -2}}, Weight: 10},
			{Kinds: notableKind(), Stats: []timeless.Range{{Min: 7, Max: 7}}, Weight: 30},
		},
	}
}

func TestConquerNotableChance(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		chance   int32
		replaced int
	}{
		{name: "never", chance: 0, replaced: 0},
		{name: "always", chance: 100, replaced: 500},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			tree := newTree(t, addingTables(testCase.chance))
			replaced := 0
			for seed := range uint32(500) {
				res := tree.Conquer(timeless.Jewel{Seed: seed}, timeless.Node{ID: 6230, Kind: timeless.Notable})
				adds := res.Additions()
				if res.Replaced {
					replaced++
				} else if len(adds) < 1 || len(adds) > 2 {
					t.Fatalf("seed %d: got %d additions, want 1 to 2", seed, len(adds))
				}
				for _, a := range adds {
					if a.Addition == 1 && (a.Rolls[1] < -4 || a.Rolls[1] > -2) {
						t.Errorf("seed %d: second stat rolled %d, want -4 to -2", seed, a.Rolls[1])
					}
				}
			}
			if replaced != testCase.replaced {
				t.Errorf("got %d of 500 notables replaced, want %d", replaced, testCase.replaced)
			}
		})
	}
}

func TestConquerSmallAddition(t *testing.T) {
	t.Parallel()
	tree := newTree(t, addingTables(0))
	got := view(tree.Conquer(timeless.Jewel{Seed: 1}, timeless.Node{ID: 6230, Kind: timeless.SmallNormal}))
	want := outcome{Replacement: -1, Additions: []timeless.Rolled{{Addition: 0, Rolls: [2]int32{5}}}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Conquer (-want +got):\n%s", diff)
	}
}

// Elegant Hubris shows seeds 20 times what the game draws from.
func TestConquerEternalSeedStep(t *testing.T) {
	t.Parallel()
	eternal := vaalShaped()
	eternal.Version.ID = "Eternal"
	vaal := newTree(t, vaalShaped())
	hubris := newTree(t, eternal)
	node := timeless.Node{ID: 6230, Kind: timeless.Notable}
	for seed := uint32(100); seed < 110; seed++ {
		want := view(vaal.Conquer(timeless.Jewel{Seed: seed}, node))
		got := view(hubris.Conquer(timeless.Jewel{Seed: seed * 20}, node))
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("seed %d (-want +got):\n%s", seed*20, diff)
		}
	}
}

// A stat whose range holds one value takes no draw, so the stat after it rolls what it would
// roll in its place.
func TestConquerFixedStatDrawsNothing(t *testing.T) {
	t.Parallel()
	tables := func(stats ...timeless.Range) timeless.Tables {
		return timeless.Tables{
			Version:      timeless.Version{ID: "Vaal", SmallNormalReplaced: true},
			Replacements: []timeless.Replacement{{Kinds: smallKinds(), Stats: stats, Weight: 1}},
		}
	}
	fixed := newTree(t, tables(timeless.Range{Min: 3, Max: 3}, timeless.Range{Min: 1, Max: 1000}))
	alone := newTree(t, tables(timeless.Range{Min: 1, Max: 1000}))
	node := timeless.Node{ID: 6230, Kind: timeless.SmallNormal}
	for seed := range uint32(20) {
		got := fixed.Conquer(timeless.Jewel{Seed: seed}, node).Rolls
		want := alone.Conquer(timeless.Jewel{Seed: seed}, node).Rolls
		if got[0] != 3 || got[1] != want[0] {
			t.Errorf("seed %d: got rolls %v, want [3 %d]", seed, got[:2], want[0])
		}
	}
}

func TestNewTreeRejects(t *testing.T) {
	t.Parallel()
	five := make([]timeless.Range, 5)
	testCases := []struct {
		name   string
		tables timeless.Tables
	}{
		{
			name:   "replacement with five stats",
			tables: timeless.Tables{Replacements: []timeless.Replacement{{Kinds: notableKind(), Stats: five, Weight: 1}}},
		},
		{
			name:   "addition with three stats",
			tables: timeless.Tables{Additions: []timeless.Addition{{Kinds: notableKind(), Stats: five[:3], Weight: 1}}},
		},
		{
			name:   "unknown kind",
			tables: timeless.Tables{Additions: []timeless.Addition{{Kinds: []timeless.Kind{6}, Weight: 1}}},
		},
		{
			name:   "no kind",
			tables: timeless.Tables{Replacements: []timeless.Replacement{{Weight: 1}}},
		},
		{
			name: "additions with nothing to add",
			tables: timeless.Tables{
				Version:   timeless.Version{NotableAdditions: timeless.Range{Min: 1, Max: 1}},
				Additions: []timeless.Addition{{Kinds: smallKinds(), Weight: 1}},
			},
		},
		{
			name: "too many additions",
			tables: timeless.Tables{
				Version:      timeless.Version{NotableAdditions: timeless.Range{Min: 1, Max: 2}},
				Replacements: []timeless.Replacement{{Kinds: notableKind(), Weight: 1, Additions: timeless.Range{Min: 7, Max: 7}}},
				Additions:    []timeless.Addition{{Kinds: notableKind(), Weight: 1}},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := timeless.NewTree(testCase.tables); err == nil {
				t.Error("got no error")
			}
		})
	}
}

func TestSmallKind(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		stats []string
		want  timeless.Kind
	}{
		{stats: []string{"additional_dexterity"}, want: timeless.SmallAttribute},
		{stats: []string{"additional_strength", "additional_intelligence"}, want: timeless.SmallNormal},
		{stats: []string{"maximum_life_+%"}, want: timeless.SmallNormal},
	}
	for _, testCase := range testCases {
		if got := timeless.SmallKind(testCase.stats); got != testCase.want {
			t.Errorf("SmallKind(%v): got %d, want %d", testCase.stats, got, testCase.want)
		}
	}
}

// BenchmarkConquerRadius conquers the passives of the fullest socket's radius for one seed: 74
// small passives, 22 notables and 4 keystones on the Vaal-shaped tables, the costliest version.
// A reverse search scans a version's whole seed range, up to 8001 seeds (target: under 500 ms
// in WebAssembly for the range, so under 60 µs per seed).
func BenchmarkConquerRadius(b *testing.B) {
	tree := newTree(b, vaalShaped())
	nodes := make([]timeless.Node, 0, 100)
	for i := range uint32(100) {
		kind := timeless.SmallNormal
		switch {
		case i < 4:
			kind = timeless.Keystone
		case i < 26:
			kind = timeless.Notable
		case i < 60:
			kind = timeless.SmallAttribute
		}
		nodes = append(nodes, timeless.Node{ID: 1000 + 37*i, Kind: kind})
	}
	jewel := timeless.Jewel{Seed: 100, Conqueror: timeless.Conqueror{Index: 1}}
	for b.Loop() {
		for _, n := range nodes {
			_ = tree.Conquer(jewel, n)
		}
	}
}
