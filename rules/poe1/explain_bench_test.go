package poe1_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/resolve"
	rules "github.com/xaroth/pbt-engine/rules/poe1"
	sempoe1 "github.com/xaroth/pbt-engine/semantics/poe1"
)

// benchSheet is a level 90 character with 120 nodes of life, resistances and regeneration,
// about the size of a real tree.
func benchSheet(b *testing.B) *rules.Sheet {
	b.Helper()
	bundles := endgame()
	stats := [][]resolve.StatValue{
		{{Stat: "maximum_life_+%", Value: 8}},
		{{Stat: "base_maximum_life", Value: 20}, {Stat: "base_strength", Value: 10}},
		{{Stat: "base_fire_damage_resistance_%", Value: 12}, {Stat: "base_resist_all_elements_%", Value: 5}},
		{{Stat: "life_regeneration_rate_per_minute_%", Value: 30}},
	}
	for i := range 120 {
		bundles = append(bundles, bundle(resolve.NodeSource{ID: uint32(100 + i)}, resolve.Global, stats[i%len(stats)]...))
	}
	t, err := sempoe1.Load()
	if err != nil {
		b.Fatal(err)
	}
	sh, err := rules.Open(resolved(bundles...), t, rules.DefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	return sh
}

// BenchmarkProportional is the core proportional view of one output (01-goals, explanation of
// one output: < 1 ms native, < 3 ms WASM).
func BenchmarkProportional(b *testing.B) {
	sh := benchSheet(b)
	for _, stat := range []string{"maximum_life", "fire_resistance_capped", "life_regeneration"} {
		b.Run(stat, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := sh.Proportion(stat); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkExplain is Sheet.Explain per view on life, sources grouped and named (01-goals,
// explanation of one output: < 1 ms native).
func BenchmarkExplain(b *testing.B) {
	sh := benchSheet(b)
	for _, v := range []rules.View{rules.Tree, rules.Proportional, rules.Marginal} {
		b.Run(v.String(), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := sh.Explain("player.life", v); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
