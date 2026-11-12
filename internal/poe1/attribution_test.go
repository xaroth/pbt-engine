package poe1_test

import (
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// Core sources of an endgame sheet: one per bundle in order, then the attribute bonuses.
const (
	srcCharacter core.SourceID = iota
	srcLevel
	srcNode
	srcKitava1
	srcKitava2
	srcOther // a bundle appended after endgame's
)

func proportion(t *testing.T, sh *poe1.Sheet, stat string) core.Proportion {
	t.Helper()
	p, err := sh.Proportion(stat)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func bySource(parts []core.Part) map[core.SourceID]float64 {
	m := map[core.SourceID]float64{}
	for _, p := range parts {
		m[p.Source] += p.Value
	}
	return m
}

func near(a, b float64) bool { return math.Abs(a-b) <= 1e-9*max(1, math.Abs(b)) }

func other(stats ...resolve.StatValue) resolve.Bundle {
	return bundle(resolve.NodeSource{ID: 2}, resolve.Global, stats...)
}

// TestEveryOutputExplains splits every catalogue output of sheets that exercise each formula,
// and checks the parts sum to the amount.
func TestEveryOutputExplains(t *testing.T) {
	t.Parallel()
	sheets := map[string][]resolve.StatValue{
		"plain": nil,
		"regeneration": {
			{Stat: "life_regeneration_rate_per_minute_%", Value: 120}, {Stat: "base_life_regeneration_rate_per_minute", Value: 600},
			{Stat: "life_regeneration_rate_+%", Value: 50}, {Stat: "keystone_eternal_youth", Value: 1},
		},
		"regeneration to energy shield": {
			{Stat: "base_life_regeneration_rate_per_minute", Value: 600}, {Stat: "life_regeneration_rate_per_minute_%", Value: 120},
			{Stat: "base_maximum_energy_shield", Value: 200}, {Stat: "regenerate_energy_shield_instead_of_life", Value: 1},
		},
		"transfers": {
			{Stat: "maximum_life_%_to_add_as_maximum_armour", Value: 10}, {Stat: "maximum_life_%_to_convert_to_maximum_energy_shield", Value: 30},
			{Stat: "mana_%_to_add_as_energy_shield", Value: 20}, {Stat: "maximum_life_+%", Value: 20},
			{Stat: "physical_damage_reduction_rating_+%", Value: 30},
		},
		"iron reflexes":     {{Stat: "keystone_iron_reflexes", Value: 1}, {Stat: "base_evasion_rating", Value: 85}},
		"chaos inoculation": {{Stat: "keystone_chaos_inoculation", Value: 1}, {Stat: "maximum_life_%_to_add_as_maximum_energy_shield", Value: 100}},
		"resistances": {
			{Stat: "base_fire_damage_resistance_%", Value: 200}, {Stat: "base_chaos_damage_resistance_%", Value: 20},
			{Stat: "chaos_damage_resistance_is_doubled", Value: 1},
			{Stat: "modifiers_to_fire_resistance_also_apply_to_cold_lightning_resistance_at_%_value", Value: 50},
		},
		"melding": {
			{Stat: "elemental_resistances_are_limited_by_highest_maximum_elemental_resistance", Value: 1},
			{Stat: "base_maximum_cold_damage_resistance_%", Value: 5}, {Stat: "base_resist_all_elements_%", Value: 200},
		},
	}
	for name, extra := range sheets {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sh := open(t, resolved(endgame(extra...)...))
			for _, o := range poe1.Outputs() {
				p := proportion(t, sh, poe1.OutputStat(o))
				sum := 0.0
				for _, pt := range p.Parts {
					sum += pt.Value
				}
				if !near(sum, p.Amount) {
					t.Errorf("%s: parts sum to %v, amount %v", o.Key, sum, p.Amount)
				}
				if v := value(t, sh, poe1.OutputStat(o)); p.Value != v {
					t.Errorf("%s: value %v, sheet %v", o.Key, p.Value, v)
				}
			}
		})
	}
}

func TestAttributions(t *testing.T) {
	t.Parallel()
	t.Run("regeneration credits the percentage", func(t *testing.T) {
		t.Parallel()
		// 2% of 1106 life per second, 50% increased: 22.12 + 11.06
		sh := open(t, resolved(append(endgame(
			resolve.StatValue{Stat: "life_regeneration_rate_per_minute_%", Value: 120}),
			other(resolve.StatValue{Stat: "life_regeneration_rate_+%", Value: 50}))...))
		life := value(t, sh, "maximum_life")
		got := bySource(proportion(t, sh, "life_regeneration").Parts)
		if !near(got[srcNode], life*0.02) || !near(got[srcOther], life*0.01) || len(got) != 2 {
			t.Errorf("got %v for %v life", got, life)
		}
	})
	t.Run("life regeneration to energy shield credits only what carries over", func(t *testing.T) {
		t.Parallel()
		zealot := []resolve.StatValue{
			{Stat: "base_maximum_energy_shield", Value: 200}, {Stat: "regenerate_energy_shield_instead_of_life", Value: 1},
		}
		testCases := []struct {
			name       string
			node, more resolve.StatValue
			want       func(es float64) float64
		}{
			// -10 flat does not carry over; 2% of energy shield does
			{
				name: "negative flat",
				node: resolve.StatValue{Stat: "life_regeneration_rate_per_minute_%", Value: 120},
				more: resolve.StatValue{Stat: "base_life_regeneration_rate_per_minute", Value: -600},
				want: func(es float64) float64 { return es * 0.02 },
			},
			// -2% of energy shield does not carry over; 10 flat does
			{
				name: "negative percent",
				node: resolve.StatValue{Stat: "base_life_regeneration_rate_per_minute", Value: 600},
				more: resolve.StatValue{Stat: "life_regeneration_rate_per_minute_%", Value: -120},
				want: func(float64) float64 { return 10 },
			},
		}
		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				t.Parallel()
				sh := open(t, resolved(append(endgame(append(zealot, testCase.node)...), other(testCase.more))...))
				want := testCase.want(value(t, sh, "maximum_energy_shield"))
				got := bySource(proportion(t, sh, "energy_shield_regeneration").Parts)
				if !near(got[srcNode], want) || len(got) != 1 {
					t.Errorf("got %v, want %v from the node alone", got, want)
				}
			})
		}
	})
	t.Run("conversion is a negative line", func(t *testing.T) {
		t.Parallel()
		sh := open(t, resolved(append(endgame(), other(resolve.StatValue{Stat: "maximum_life_%_to_convert_to_maximum_energy_shield", Value: 30}))...))
		p := proportion(t, sh, "maximum_life")
		full := p.Amount / 0.7
		if got := bySource(p.Parts); !near(got[srcOther], -0.3*full) || got[srcCharacter] != 1118 {
			t.Errorf("got %v, amount %v", got, p.Amount)
		}
		es := bySource(proportion(t, sh, "maximum_energy_shield").Parts)
		if es[srcOther] < 0.3*full {
			t.Errorf("energy shield from conversion: got %v, want at least %v", es[srcOther], 0.3*full)
		}
	})
	t.Run("chaos doubled credits the flag", func(t *testing.T) {
		t.Parallel()
		sh := open(t, resolved(append(endgame(resolve.StatValue{Stat: "base_chaos_damage_resistance_%", Value: 20}),
			other(resolve.StatValue{Stat: "chaos_damage_resistance_is_doubled", Value: 1}))...))
		// -40 doubled: -80, the doubling's -40 to the flag
		if got := bySource(proportion(t, sh, "chaos_resistance").Parts); got[srcOther] != -40 || got[srcNode] != 20 {
			t.Errorf("got %v", got)
		}
	})
	t.Run("shared fire modifiers credit the share", func(t *testing.T) {
		t.Parallel()
		sh := open(t, resolved(append(endgame(resolve.StatValue{Stat: "base_fire_damage_resistance_%", Value: 30}),
			other(resolve.StatValue{Stat: "modifiers_to_fire_resistance_also_apply_to_cold_lightning_resistance_at_%_value", Value: 50}))...))
		if got := bySource(proportion(t, sh, "cold_resistance").Parts); got[srcOther] != 15 || got[srcNode] != 0 {
			t.Errorf("got %v", got)
		}
	})
	t.Run("capped resistance explains the total", func(t *testing.T) {
		t.Parallel()
		sh := open(t, resolved(endgame(resolve.StatValue{Stat: "base_fire_damage_resistance_%", Value: 200})...))
		p := proportion(t, sh, "fire_resistance_capped")
		if p.Amount != 140 || p.Effective != 75 || p.Value != 75 || len(p.CapBy) != 1 || bySource(p.Parts)[srcNode] != 200 {
			t.Errorf("got %+v", p)
		}
	})
	t.Run("melding passes the highest maximum through", func(t *testing.T) {
		t.Parallel()
		sh := open(t, resolved(append(endgame(resolve.StatValue{Stat: "base_maximum_cold_damage_resistance_%", Value: 5}),
			other(resolve.StatValue{Stat: "elemental_resistances_are_limited_by_highest_maximum_elemental_resistance", Value: 1}))...))
		p := proportion(t, sh, "effective_maximum_fire_resistance")
		if got := bySource(p.Parts); p.Amount != 80 || got[srcNode] != 5 || got[srcCharacter] != 75 {
			t.Errorf("got %v, amount %v", got, p.Amount)
		}
	})
}
