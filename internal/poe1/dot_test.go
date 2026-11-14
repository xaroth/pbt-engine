package poe1_test

import (
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// righteousFire is a made-up skill that burns for 70% of maximum life and energy shield per
// second as fire damage over time, with stats added to its stat set.
func righteousFire(stats ...resolve.StatValue) *resolve.Skill {
	sk := skill(0, "Spell", "Area", "Fire", "DamageOverTime")
	sk.StatSets = []resolve.SkillStatSet{{Stats: append([]resolve.StatValue{
		{Stat: "base_righteous_fire_%_of_max_life_to_deal_to_nearby_per_minute", Value: 4200},
		{Stat: "base_righteous_fire_%_of_max_energy_shield_to_deal_to_nearby_per_minute", Value: 4200},
	}, stats...)}}
	return sk
}

// With 1000 life and 200 energy shield the skill burns for 840 fire damage per second, of
// which the default enemy (50% fire resistance) takes half. Increases and more multipliers
// to damage over time and the Damage over Time Multiplier scale it; modifiers to spell damage
// only when the skill says so, and modifiers to hits never. Exposure lowers the resistance it
// meets, penetration does not.
func TestDamageOverTime(t *testing.T) {
	t.Parallel()
	stat := func(name string, value float64) resolve.StatValue { return resolve.StatValue{Stat: name, Value: value} }
	support := func(stats ...resolve.StatValue) resolve.Support {
		return resolve.Support{Source: resolve.GemSource{Gem: 1, ID: "support"}, Stats: stats}
	}
	for _, c := range []struct {
		name     string
		node     []resolve.StatValue
		own      []resolve.StatValue
		supports []resolve.Support
		want     float64
	}{
		{name: "base", want: 420},
		{
			name: "increased and more", node: []resolve.StatValue{stat("damage_over_time_+%", 50)},
			supports: []resolve.Support{support(stat("support_efficacy_damage_over_time_+%_final", 20))}, want: 420 * 1.5 * 1.2,
		},
		{name: "multiplier", node: []resolve.StatValue{stat("dot_multiplier_+", 20), stat("fire_dot_multiplier_+", 30)}, want: 420 * 1.5},
		{name: "cold multiplier", node: []resolve.StatValue{stat("cold_dot_multiplier_+", 30)}, want: 420},
		{name: "spell damage", node: []resolve.StatValue{stat("spell_damage_+%", 100)}, want: 420},
		{
			name: "spell damage applying to damage over time", node: []resolve.StatValue{stat("spell_damage_+%", 100)},
			own: []resolve.StatValue{stat("spell_damage_modifiers_apply_to_skill_dot", 1)}, want: 840,
		},
		{name: "hit damage", supports: []resolve.Support{support(stat("support_cruelty_hit_damage_+%_final", 100))}, want: 420},
		{name: "exposure", node: []resolve.StatValue{stat("fire_exposure_on_hit_magnitude", -10)}, want: 840 * 0.6},
		{name: "penetration", node: []resolve.StatValue{stat("base_reduce_enemy_fire_resistance_%", 20)}, want: 420},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global, append([]resolve.StatValue{
				stat("base_maximum_life", 1000), stat("base_maximum_energy_shield", 200),
			}, c.node...)...)))
			sk := righteousFire(c.own...)
			sk.Supports = c.supports
			setSkills(t, sh, sk)
			if got := outputOf(t, sh, poe1.DotDPS); math.Abs(got-c.want) > 1e-9 {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

// Physical damage over time meets neither armour nor a resistance; an uber boss takes 70% less.
func TestPhysicalDamageOverTime(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved())
	sk := skill(0, "Spell", "Physical", "DamageOverTime")
	sk.StatSets = []resolve.SkillStatSet{{Stats: []resolve.StatValue{{Stat: "base_physical_damage_to_deal_per_minute", Value: 6000}}}}
	setSkills(t, sh, sk)
	if got := outputOf(t, sh, poe1.DotDPS); got != 100 {
		t.Errorf("against the default enemy: got %v, want 100", got)
	}
	sh.SetEnemy(poe1.EnemyStats{Kind: poe1.EnemyUber, Armour: 50000})
	if got := outputOf(t, sh, poe1.DotDPS); math.Abs(got-30) > 1e-9 {
		t.Errorf("against an uber boss: got %v, want 30", got)
	}
}

// The sheet gives damage over time for a skill of the DamageOverTime type only.
func TestDotOutputGiven(t *testing.T) {
	t.Parallel()
	var dot poe1.Output
	for _, o := range poe1.Outputs() {
		if o.Key == poe1.DotDPS {
			dot = o
		}
	}
	for _, c := range []struct {
		name  string
		skill *resolve.Skill
		want  bool
	}{
		{"none", nil, false},
		{"spell hit", skill(0, "Spell", "Damage"), false},
		{"damage over time", skill(0, "Spell", "DamageOverTime"), true},
	} {
		if got := poe1.Gives(dot, c.skill, poe1.Hands{}); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// Damage over time explains to its amount, crediting its base to the life and energy shield it
// is a share of.
func TestExplainDamageOverTime(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global,
		resolve.StatValue{Stat: "base_maximum_life", Value: 1000}, resolve.StatValue{Stat: "base_maximum_energy_shield", Value: 200},
		resolve.StatValue{Stat: "damage_over_time_+%", Value: 50}, resolve.StatValue{Stat: "dot_multiplier_+", Value: 20},
		resolve.StatValue{Stat: "fire_exposure_on_hit_magnitude", Value: -10},
	)))
	setSkills(t, sh, righteousFire())
	sh.SetEnemy(poe1.EnemyStats{Kind: poe1.EnemyUber, FireResistance: 50})
	for _, k := range []output.Key{poe1.DotDPS} {
		ex := explainOf(t, sh, k, poe1.Proportional)
		sum := 0.0
		for _, l := range ex.Lines {
			sum += l.Value
		}
		if !near(sum, ex.Amount) || ex.Amount == 0 {
			t.Errorf("%s: lines sum to %v, amount %v", k, sum, ex.Amount)
		}
	}
}
