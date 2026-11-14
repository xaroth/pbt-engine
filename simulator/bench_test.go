package simulator_test

import (
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/benchbuilds"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/simulator"
)

// BenchmarkSimulate simulates a synthetic build without skills or items on a warm sim, the
// sheet taken from the pool: the tree and sheet alone. BenchmarkSimulateReal is a typical build.
func BenchmarkSimulate(b *testing.B) {
	s := newSim(b)
	base := marauderBuild(90, 2, 3, 4)
	base.Masteries = map[uint32]uint32{4: 7}
	simulate(b, s, base)
	b.ReportAllocs()
	for b.Loop() {
		simulate(b, s, base)
	}
}

// BenchmarkSweepVariant sweeps single-node variants on the fast path (target per variant:
// < 0.2 ms native; ns/variant).
func BenchmarkSweepVariant(b *testing.B) {
	s := newSim(b)
	base := marauderBuild(90, 2)
	variants := make([][]build.Edit, 64)
	for i := range variants {
		variants[i] = []build.Edit{build.Allocate(uint32(3 + i%3))}
	}
	b.ReportAllocs()
	n := 0
	for b.Loop() {
		if _, err := s.Sweep(base, variants, simulator.Outputs(poe1.Life), simulator.Workers(1)); err != nil {
			b.Fatal(err)
		}
		n += len(variants)
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(n), "ns/variant")
}

// BenchmarkStatsExplain explains life in each view on Stats whose sheet is open already
// (target per output: < 1 ms native).
func BenchmarkStatsExplain(b *testing.B) {
	st := simulate(b, newSim(b), marauderBuild(90, 2, 3))
	for _, v := range []simulator.View{simulator.Tree, simulator.Proportional, simulator.Marginal} {
		if _, err := st.Explain(poe1.Life, v); err != nil {
			b.Fatal(err)
		}
		b.Run(v.String(), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := st.Explain(poe1.Life, v); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkItemWhatIf runs what-ifs on the level 90 Chieftain with a rare helmet on the real
// dataset: single-node sweep variants (ns/variant), and comparing the build with the helmet
// re-equipped with its armour value changed.
func BenchmarkItemWhatIf(b *testing.B) {
	s := realSim(b)
	base := chieftainWithHelmet(b, s)
	b.Run("sweep", func(b *testing.B) {
		variants := make([][]build.Edit, 0, 64)
		for _, n := range base.Nodes[:64] {
			variants = append(variants, []build.Edit{build.Deallocate(n)})
		}
		b.ReportAllocs()
		n := 0
		for b.Loop() {
			if _, err := s.Sweep(base, variants, simulator.Outputs(poe1.Life), simulator.Workers(1)); err != nil {
				b.Fatal(err)
			}
			n += len(variants)
		}
		b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(n), "ns/variant")
	})
	b.Run("re-equip", func(b *testing.B) {
		other := base.Clone()
		it := other.Items[build.SlotHelmet]
		it.Armour = 767
		if err := other.Apply(build.Equip(build.SlotHelmet, it)); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		for b.Loop() {
			if _, err := s.Compare(base, other, simulator.Outputs(poe1.Life, poe1.Armour)); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkSimulateReal simulates each benchmark build, with its skills and items, on a warm sim
// on the pinned current dataset (target: < 3 ms native).
func BenchmarkSimulateReal(b *testing.B) {
	s, cases := realBench(b)
	for _, c := range cases {
		b.Run(c.Name, func(b *testing.B) {
			simulate(b, s, c.Build)
			b.ReportAllocs()
			for b.Loop() {
				simulate(b, s, c.Build)
			}
		})
	}
}

// BenchmarkSweepReal is a hover preview on each benchmark build: one variant that allocates the
// path to a notable, every output.
func BenchmarkSweepReal(b *testing.B) {
	s, cases := realBench(b)
	for _, c := range cases {
		variants := [][]build.Edit{{c.hover}}
		b.Run(c.Name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := s.Sweep(c.Build, variants, simulator.Workers(1)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkCompareReal compares each benchmark build with itself after allocating the path to a
// notable, as applying a click does.
func BenchmarkCompareReal(b *testing.B) {
	s, cases := realBench(b)
	for _, c := range cases {
		after := c.Build.Clone()
		if err := after.Apply(c.hover); err != nil {
			b.Fatal(err)
		}
		b.Run(c.Name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := s.Compare(c.Build, after); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// realBench returns a sim on the pinned current dataset and the benchmark builds, each with a
// hover: the path to the unallocated notable of lowest id that is 3 to 8 nodes away.
func realBench(b *testing.B) (*simulator.Sim, []realCase) {
	b.Helper()
	d := benchbuilds.Open(b)
	s, err := simulator.New(d.Store)
	if err != nil {
		b.Fatal(err)
	}
	cases := benchbuilds.Builds(b, d)
	out := make([]realCase, 0, len(cases))
	for _, c := range cases {
		out = append(out, realCase{Case: c, hover: hoverEdit(b, d.Store, c.Build)})
	}
	return s, out
}

type realCase struct {
	benchbuilds.Case

	hover build.Edit
}

func hoverEdit(tb testing.TB, store *dataset.Store, bd *build.Build) build.Edit {
	tb.Helper()
	c, _, err := build.Check(bd, store)
	if err != nil {
		tb.Fatal(err)
	}
	for _, n := range c.Tree().Nodes {
		if n.Kind != dataset.Notable || n.Ascendancy != "" || slices.Contains(bd.Nodes, n.ID) {
			continue
		}
		if e, ok := c.AllocatePath(n.ID); ok && len(e.Nodes) >= 3 && len(e.Nodes) <= 8 {
			return e
		}
	}
	tb.Fatal("no notable 3 to 8 nodes away")
	return build.Edit{}
}
