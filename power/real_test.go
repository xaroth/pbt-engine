package power_test

import (
	"cmp"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/power"
	"github.com/xaroth/pbt-engine/simulator"
)

// realChieftain is the level 90 Chieftain of pbt-conformance on the current pinned dataset with its mastery
// choices, and a sim over that dataset.
func realChieftain(tb testing.TB) (*simulator.Sim, *build.Build) {
	tb.Helper()
	sim, err := simulator.New(realdata.Open(tb, realdata.Current))
	if err != nil {
		tb.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "internal", "poe1", "testdata", "explain", "chieftain-L90-3.29.nodes"))
	if err != nil {
		tb.Fatal(err)
	}
	b := &build.Build{Format: 1, Game: "poe1", Version: "3.29", Class: "Metadata/Characters/Str/Str", Ascendancy: "Chieftain", Level: 90,
		Masteries: map[uint32]uint32{4139: 47642, 11505: 36313, 12125: 21468, 12382: 64381, 25011: 31822, 27193: 45653, 27733: 23471, 32241: 34242, 41415: 38454}}
	for f := range strings.SplitSeq(strings.TrimSpace(string(data)), ",") {
		n, err := strconv.ParseUint(f, 10, 32)
		if err != nil {
			tb.Fatal(err)
		}
		b.Nodes = append(b.Nodes, uint32(n))
	}
	return sim, b
}

// Every alone and path result equals simulating the edited build; ranking by life equals
// sorting those brute-force values.
func TestRealChieftainEqualsBruteForce(t *testing.T) {
	t.Parallel()
	sim, base := realChieftain(t)
	nps := nodes(t, sim, base, power.Outputs(poe1.Life))
	t.Logf("%d candidates", len(nps))
	st, err := sim.Simulate(base, simulator.Outputs(poe1.Life))
	if err != nil {
		t.Fatal(err)
	}
	baseLife, _ := st.Get(poe1.Life)
	type ranked struct {
		node, effect uint32
		gain         float64
	}
	got, brute := make([]ranked, 0, len(nps)), make([]ranked, 0, len(nps))
	for _, np := range nps {
		var aloneAfter float64
		for i, tc := range []struct {
			r     simulator.Result
			nodes []uint32
		}{{np.Alone, []uint32{np.Node}}, {np.ViaPath, np.Path}} {
			b := base.Clone()
			b.Nodes = append(b.Nodes, tc.nodes...)
			if np.Effect != 0 {
				b.Masteries[np.Node] = np.Effect
			}
			st, err := sim.Simulate(b, simulator.Outputs(poe1.Life))
			if err != nil {
				t.Fatal(err)
			}
			want, _ := st.Get(poe1.Life)
			c := tc.r.Changes[poe1.Life]
			if tc.r.Err != nil || math.Float64bits(c.After) != math.Float64bits(want) {
				t.Fatalf("node %d effect %d: %+v, want %v", np.Node, np.Effect, c, want)
			}
			if i == 0 {
				aloneAfter = want
			}
		}
		got = append(got, ranked{np.Node, np.Effect, np.Alone.Changes[poe1.Life].Abs})
		brute = append(brute, ranked{np.Node, np.Effect, aloneAfter - baseLife})
	}
	order := func(a, b ranked) int {
		return cmp.Or(cmp.Compare(b.gain, a.gain), cmp.Compare(a.node, b.node), cmp.Compare(a.effect, b.effect))
	}
	slices.SortFunc(got, order)
	slices.SortFunc(brute, order)
	if !slices.Equal(got, brute) {
		t.Error("ranking differs from brute force")
	}
}

// BenchmarkNodes sweeps every candidate of the real level 90 Chieftain (~1500 nodes, target:
// < 0.3 s native); it needs $PBT_DATASETS.
func BenchmarkNodes(b *testing.B) {
	sim, base := realChieftain(b)
	for name, opts := range map[string][]power.Option{
		"one worker":              {power.Outputs(poe1.Life), power.Workers(1)},
		"all workers":             {power.Outputs(poe1.Life)},
		"all workers, distance 5": {power.Outputs(poe1.Life), power.MaxDistance(5)},
	} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			n := 0
			for b.Loop() {
				nps, err := power.Nodes(sim, base, opts...)
				if err != nil {
					b.Fatal(err)
				}
				n = len(nps)
			}
			b.ReportMetric(float64(n), "candidates")
		})
	}
}

// As in the game, a path leaves an ascendancy for the main tree only from an allocated node:
// for a fresh Scion Ascendant, the path to main-tree node 39821, beside the unallocated "Path of
// the Ranger", takes main-tree nodes only.
func TestRealAscendantPathStaysInMainTree(t *testing.T) {
	t.Parallel()
	s := realdata.Open(t, realdata.Current)
	sim, err := simulator.New(s)
	if err != nil {
		t.Fatal(err)
	}
	c, err := build.New(s, "Metadata/Characters/StrDexInt/StrDexInt")
	if err != nil {
		t.Fatal(err)
	}
	b := c.Build()
	b.Ascendancy = "Ascendant"
	tree, err := s.Tree(build.DefaultVariant)
	if err != nil {
		t.Fatal(err)
	}
	const besideRangerPath = 39821
	nps := nodes(t, sim, &b, power.Outputs(poe1.Life))
	i := slices.IndexFunc(nps, func(np power.NodePower) bool { return np.Node == besideRangerPath })
	if i < 0 {
		t.Fatalf("no candidate %d", besideRangerPath)
	}
	np := nps[i]
	for _, n := range tree.Nodes {
		if n.Ascendancy != "" && slices.Contains(np.Path, n.ID) {
			t.Errorf("path %v takes node %d of %s, want main-tree nodes only", np.Path, n.ID, n.Ascendancy)
		}
	}
	if np.AscendancyPoints != 0 {
		t.Errorf("got %d ascendancy points, want 0", np.AscendancyPoints)
	}
}
