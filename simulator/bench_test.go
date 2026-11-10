package simulator_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/rules/poe1"
	"github.com/xaroth/pbt-engine/simulator"
)

// BenchmarkSimulate simulates a build on a warm sim, the sheet taken from the pool (01-goals,
// full evaluation of a typical build: < 3 ms native).
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

// BenchmarkSweepVariant sweeps single-node variants on the fast path (01-goals, one what-if:
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
// (01-goals, explanation of one output: < 1 ms native).
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
