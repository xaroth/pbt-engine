package power_test

import (
	"math"
	"os"
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/power"
	"github.com/xaroth/pbt-engine/rules/poe1"
	"github.com/xaroth/pbt-engine/simulator"
)

const marauder = "Metadata/Characters/Str/Str"

// testSim is a Marauder tree: start 1 joins 2 and 5, both join 3, mastery 7 hangs off 3
// (effects 70, +5% life, and 71, +3% life), Juggernaut start 20 joins 21, node 9 joins
// nothing. Nodes 2, 3, 5 and 21 give 10, 20, 30 and 40 life.
func testSim(tb testing.TB) *simulator.Sim {
	tb.Helper()
	stats := []string{
		"level", "life_per_level", "base_maximum_all_resistances_%", "base_strength", "base_dexterity", "base_intelligence",
		"base_maximum_life", "base_maximum_mana", "maximum_life_+%",
	}
	life := func(v int32) []dataset.StatValue { return []dataset.StatValue{{Stat: 6, Value: v}} }
	d := &dataset.Dataset{
		Manifest: dataset.Manifest{Game: "poe1", Version: "3.29.3.3"},
		Characters: []dataset.Character{{
			ID: marauder, BaseStrength: 32, BaseMaxLife: 38,
			Stats: []dataset.StatValue{{Stat: 0, Value: 1}, {Stat: 1, Value: 12}, {Stat: 2, Value: 75}},
		}},
		Constants: []dataset.Constant{
			{ID: "life_per_strength", Value: 1, Per: 2}, {ID: "mana_per_intelligence", Value: 1, Per: 2},
			{ID: "evasion_rating_+%_per_dexterity", Value: 1, Per: 5}, {ID: "energy_shield_+%_per_intelligence", Value: 1, Per: 10},
		},
		Trees: []dataset.Tree{{
			Variant: "standard",
			Classes: []dataset.Class{{Character: marauder, StartNode: 1, Ascendancies: []dataset.Ascendancy{{ID: "Juggernaut"}}}},
			Nodes: []dataset.Node{
				{ID: 1, Kind: dataset.ClassStart, Neighbours: []uint32{2, 5}},
				{ID: 2, Neighbours: []uint32{1, 3}, Stats: life(10)},
				{ID: 3, Neighbours: []uint32{2, 5, 7}, Stats: life(20)},
				{ID: 5, Neighbours: []uint32{1, 3}, Stats: life(30)},
				{ID: 7, Kind: dataset.Mastery, Neighbours: []uint32{3}, MasteryEffects: []dataset.MasteryEffect{
					{ID: 70, Stats: []dataset.StatValue{{Stat: 8, Value: 5}}}, {ID: 71, Stats: []dataset.StatValue{{Stat: 8, Value: 3}}},
				}},
				{ID: 9, Stats: life(99)},
				{ID: 20, Kind: dataset.AscendancyStart, Ascendancy: "Juggernaut", Neighbours: []uint32{21}},
				{ID: 21, Ascendancy: "Juggernaut", Neighbours: []uint32{20}, Stats: life(40)},
			},
		}},
	}
	for _, id := range stats {
		d.Stats = append(d.Stats, dataset.Stat{ID: id})
	}
	dir := tb.TempDir()
	if _, err := dataset.Write(dir, d); err != nil {
		tb.Fatal(err)
	}
	s, err := dataset.Open(os.DirFS(dir))
	if err != nil {
		tb.Fatal(err)
	}
	sim, err := simulator.New(s)
	if err != nil {
		tb.Fatal(err)
	}
	return sim
}

func juggernaut(nodes ...uint32) *build.Build {
	return &build.Build{Format: 1, Game: "poe1", Version: "3.29", Class: marauder, Ascendancy: "Juggernaut", Level: 10, Nodes: nodes}
}

type summary struct {
	node, effect uint32
	path         []uint32
	points       int
}

func summarize(nps []power.NodePower) []summary {
	out := make([]summary, 0, len(nps))
	for _, np := range nps {
		out = append(out, summary{np.Node, np.Effect, np.Path, np.Points})
	}
	return out
}

func nodes(t *testing.T, sim *simulator.Sim, b *build.Build, opts ...power.Option) []power.NodePower {
	t.Helper()
	nps, err := power.Nodes(sim, b, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return nps
}

func TestCandidatesAndPaths(t *testing.T) {
	t.Parallel()
	sim := testSim(t)
	got := summarize(nodes(t, sim, juggernaut()))
	// 3 is two steps away through 2 or 5: the lower id wins; 9 joins nothing
	want := []summary{
		{2, 0, []uint32{2}, 1}, {3, 0, []uint32{2, 3}, 2}, {5, 0, []uint32{5}, 1},
		{7, 70, []uint32{2, 3, 7}, 3}, {7, 71, []uint32{2, 3, 7}, 3}, {21, 0, []uint32{21}, 1},
	}
	if !slices.EqualFunc(want, got, func(a, b summary) bool {
		return a.node == b.node && a.effect == b.effect && a.points == b.points && slices.Equal(a.path, b.path)
	}) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
	// an allocated mastery offers its other effect, with no path
	b := juggernaut(2, 3, 7)
	b.Masteries = map[uint32]uint32{7: 70}
	got = summarize(nodes(t, sim, b))
	if !slices.ContainsFunc(got, func(s summary) bool { return s.node == 7 && s.effect == 71 && s.points == 0 && len(s.path) == 0 }) ||
		slices.ContainsFunc(got, func(s summary) bool { return s.node == 7 && s.effect == 70 }) {
		t.Errorf("allocated mastery: %v", got)
	}
}

func TestMaxDistance(t *testing.T) {
	t.Parallel()
	sim := testSim(t)
	for limit, want := range map[int][]uint32{1: {2, 5, 21}, 2: {2, 3, 5, 21}, 3: {2, 3, 5, 7, 7, 21}, 0: {2, 3, 5, 7, 7, 21}, 99: {2, 3, 5, 7, 7, 21}} {
		nps := nodes(t, sim, juggernaut(), power.MaxDistance(limit))
		got := make([]uint32, 0, len(nps))
		for _, np := range nps {
			if np.Points > limit && limit > 0 {
				t.Errorf("limit %d: node %d at %d points", limit, np.Node, np.Points)
			}
			got = append(got, np.Node)
		}
		if !slices.Equal(want, got) {
			t.Errorf("limit %d: got %v, want %v", limit, got, want)
		}
	}
}

// Alone and path results equal simulating the edited builds.
func TestResults(t *testing.T) {
	t.Parallel()
	sim := testSim(t)
	base := juggernaut()
	for _, np := range nodes(t, sim, base, power.Outputs(poe1.Life)) {
		for name, tc := range map[string]struct {
			r     simulator.Result
			nodes []uint32
		}{"alone": {np.Alone, []uint32{np.Node}}, "path": {np.ViaPath, np.Path}} {
			b := base.Clone()
			b.Nodes = append(b.Nodes, tc.nodes...)
			if np.Effect != 0 {
				b.Masteries = map[uint32]uint32{np.Node: np.Effect}
			}
			st, err := sim.Simulate(b)
			if err != nil {
				t.Fatal(err)
			}
			want, _ := st.Get(poe1.Life)
			if c := tc.r.Changes[poe1.Life]; tc.r.Err != nil || math.Float64bits(c.After) != math.Float64bits(want) {
				t.Errorf("node %d effect %d %s: %+v, want %v", np.Node, np.Effect, name, c, want)
			}
		}
	}
}
