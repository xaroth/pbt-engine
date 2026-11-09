package simulator_test

import (
	"math"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/output"
	"github.com/xaroth/pbt-engine/rules/poe1"
	"github.com/xaroth/pbt-engine/simulator"
)

func randomBuild(r *rand.Rand) *build.Build {
	b := marauderBuild([]int{1, 50, 90}[r.IntN(3)])
	for _, n := range []uint32{2, 3, 4, 5} {
		if r.IntN(2) == 0 {
			b.Nodes = append(b.Nodes, n)
		}
	}
	if r.IntN(2) == 0 {
		b.Masteries = map[uint32]uint32{4: 7}
	}
	return b
}

// Compare equals two fresh simulations bit for bit, for builds of one shape and of two.
func TestCompareEqualsFresh(t *testing.T) {
	t.Parallel()
	s := newSim(t)
	r := rand.New(rand.NewPCG(4, 2))
	for i := range 40 {
		a, b := randomBuild(r), randomBuild(r)
		d, err := s.Compare(a, b)
		if err != nil {
			t.Fatal(err)
		}
		sameStats(t, "before", simulate(t, newSim(t), a), d.Before)
		sameStats(t, "after", simulate(t, newSim(t), b), d.After)
		edits, err := build.Diff(a, b)
		if err != nil || len(edits) != len(d.Edits) {
			t.Fatalf("pair %d: edits %v, want %v", i, d.Edits, edits)
		}
	}
}

func TestDiff(t *testing.T) {
	t.Parallel()
	s := newSim(t)
	d, err := s.Compare(marauderBuild(90), marauderBuild(90, 2), simulator.Outputs(poe1.Life, poe1.Mana))
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Changed(); !slices.Equal(got, []output.Key{poe1.Life}) {
		t.Errorf("changed %v", got)
	}
	// 1134 to 1154
	if c := d.Get(poe1.Life); !c.Has || c.Before != 1134 || c.After != 1154 || c.Abs != 20 || math.Abs(c.Pct-20.0/1134*100) > 1e-12 {
		t.Errorf("life change %+v", c)
	}
	if len(d.DataChanges) != 0 {
		t.Errorf("one dataset, data changes %v", d.DataChanges)
	}
}

// A compare across two datasets lists the allocated nodes whose data changed.
func TestDiffAcrossDatasets(t *testing.T) {
	t.Parallel()
	older, err := simulator.New(testStoreWith(t, "3.28.0.16", 10))
	if err != nil {
		t.Fatal(err)
	}
	b := marauderBuild(90, 2, 3)
	before := simulate(t, older, b)
	after := simulate(t, newSim(t), b)
	d, err := before.Diff(after)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Edits) != 0 || !slices.Contains(d.Changed(), poe1.Life) {
		t.Errorf("edits %v, changed %v", d.Edits, d.Changed())
	}
	if len(d.DataChanges) != 1 || d.DataChanges[0].Node != 2 || d.DataChanges[0].Before["base_maximum_life"] != 10 ||
		d.DataChanges[0].After["base_maximum_life"] != 20 {
		t.Errorf("data changes %+v", d.DataChanges)
	}
	if before.Version() != "3.28.0.16" || after.Version() != "3.29.3.3" {
		t.Errorf("versions %s, %s", before.Version(), after.Version())
	}
}

func TestStatsExplain(t *testing.T) {
	t.Parallel()
	b := marauderBuild(90, 2, 3, 4)
	b.Masteries = map[uint32]uint32{4: 7}
	st := simulate(t, newSim(t), b)
	for _, v := range []poe1.View{poe1.Tree, poe1.Proportional, poe1.Marginal} {
		ex, err := st.Explain(poe1.Life, v)
		if err != nil {
			t.Fatal(err)
		}
		if ex.Value != get(t, st, poe1.Life) {
			t.Errorf("%s: value %v, stats %v", v, ex.Value, get(t, st, poe1.Life))
		}
	}
	ex, err := st.Explain(poe1.Life, poe1.Proportional)
	if err != nil {
		t.Fatal(err)
	}
	sum := 0.0
	for _, l := range ex.Lines {
		sum += l.Value
	}
	if math.Abs(sum-ex.Amount) > 1e-9 || len(ex.Lines) != 5 {
		t.Errorf("lines %d sum %v, amount %v", len(ex.Lines), sum, ex.Amount)
	}
	if _, err := st.Explain("player.nothing", poe1.Tree); err == nil {
		t.Error("unknown output: no error")
	}
}

// Explanations of one Stats from several goroutines (run with -race).
func TestStatsExplainConcurrent(t *testing.T) {
	t.Parallel()
	st := simulate(t, newSim(t), marauderBuild(90, 2, 3))
	var wg sync.WaitGroup
	for g := range 6 {
		wg.Go(func() {
			for _, k := range []output.Key{poe1.Life, poe1.FireResistance, poe1.Mana} {
				if _, err := st.Explain(k, []poe1.View{poe1.Tree, poe1.Proportional, poe1.Marginal}[g%3]); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
}
