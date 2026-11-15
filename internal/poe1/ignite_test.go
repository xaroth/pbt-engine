package poe1_test

import (
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// fireSpell is a made-up fire spell: 100 to 200 base fire damage, critChance in hundredths of a
// percent, 1 second cast time, with stats added to its stat set.
func fireSpell(critChance int32, stats ...resolve.StatValue) *resolve.Skill {
	sk := skill(0, "Spell", "Damage", "Fire")
	sk.CastTime = 1000
	sk.StatSets = []resolve.SkillStatSet{{SpellCritChance: critChance, Stats: append([]resolve.StatValue{
		{Stat: "spell_minimum_base_fire_damage", Value: 100}, {Stat: "spell_maximum_base_fire_damage", Value: 200},
	}, stats...)}}
	return sk
}

// Ignite deals 90% of the fire damage of the hit per second for 4 seconds; the default enemy
// takes half of it. The spell hits once a second, so it applies 4 x its chance per hit ignites
// over a duration. With fewer than one the damage scales down by that share at the middle of
// the hit's range (150); with more, the strongest sits at applied / (applied + 1) of the range.
// The strongest came from a critical strike with chance 1 - (1 - crit)^applied, and then takes
// the character's +50% Damage over Time Multiplier for Ailments from Critical Strikes.
func TestIgnite(t *testing.T) {
	t.Parallel()
	stat := func(name string, value float64) resolve.StatValue { return resolve.StatValue{Stat: name, Value: value} }
	chance := func(v float64) resolve.StatValue { return stat("base_chance_to_ignite_%", v) }
	for _, c := range []struct {
		name string
		crit int32
		node []resolve.StatValue
		own  []resolve.StatValue
		want float64
	}{
		{name: "no chance", want: 0},
		{name: "fewer than one", node: []resolve.StatValue{chance(10)}, want: 150 * 0.9 * 0.4 * 0.5},
		{name: "always", node: []resolve.StatValue{chance(100)}, want: 180 * 0.9 * 0.5},
		{
			// 0.5 per hit, 2 applied: roll 2/3, crit 1 - 0.5^2, every ignite from a crit.
			name: "critical strikes", crit: 5000, want: (100 + 100*2.0/3) * 1.5 * 0.9 * 0.5,
		},
		{
			// 0.75 per hit, 3 applied: roll 3/4, crit 1 - 0.5^3; from hits 0.5 x 0.125, from
			// crits 0.875.
			name: "hits and critical strikes", crit: 5000, node: []resolve.StatValue{chance(50)},
			want: 175 * (0.0625 + 1.5*0.875) / 0.9375 * 0.9 * 0.5,
		},
		{
			// Spell damage and hit damage stay off the ignite; burning damage and the fire
			// Damage over Time Multiplier reach it.
			name: "modifiers", node: []resolve.StatValue{
				chance(100), stat("spell_damage_+%", 100), stat("burn_damage_+%", 50), stat("fire_dot_multiplier_+", 20),
			},
			own:  []resolve.StatValue{stat("support_better_ailments_hit_damage_+%_final", 100)},
			want: 270 * 1.2 * 0.9 * 0.5,
		},
		{name: "longer", node: []resolve.StatValue{chance(10), stat("ignite_duration_+%", 100)}, want: 150 * 0.9 * 0.8 * 0.5},
		{name: "faster", node: []resolve.StatValue{chance(10), stat("faster_burn_%", 100)}, want: 150 * 0.9 * 2 * 0.2 * 0.5},
		{name: "cannot ignite", crit: 5000, node: []resolve.StatValue{chance(50)}, own: []resolve.StatValue{stat("never_ignite", 1)}, want: 0},
		{
			// Ailments are never from critical strikes: 0.5 per hit, 2 applied, no crit
			// multiplier.
			name: "never from critical strikes", crit: 5000, node: []resolve.StatValue{chance(50), stat("keystone_elemental_overload", 1)},
			want: (100 + 100*2.0/3) * 0.9 * 0.5,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global,
				append([]resolve.StatValue{stat("critical_ailment_dot_multiplier_+", 50)}, c.node...)...)))
			setSkills(t, sh, fireSpell(c.crit, c.own...))
			for _, k := range []output.Key{poe1.IgniteDPS, poe1.DotDPS} {
				if got := outputOf(t, sh, k); math.Abs(got-c.want) > 1e-9 {
					t.Errorf("%s: got %v, want %v", k, got, c.want)
				}
			}
		})
	}
}

// An ignite starts from the hit's base damage and conversion with the ailment's modifiers: of
// a physical spell converting half to fire, the fire half (50) without spell damage; with All
// Damage can Ignite, the physical half (50) too.
func TestIgniteSources(t *testing.T) {
	t.Parallel()
	stat := func(name string, value float64) resolve.StatValue { return resolve.StatValue{Stat: name, Value: value} }
	for _, c := range []struct {
		name string
		node []resolve.StatValue
		want float64
	}{
		{name: "converted", want: 50 * 0.9 * 0.5},
		{name: "all damage", node: []resolve.StatValue{stat("all_damage_can_ignite", 1)}, want: 100 * 0.9 * 0.5},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global, append([]resolve.StatValue{
				stat("base_chance_to_ignite_%", 100), stat("spell_damage_+%", 100),
			}, c.node...)...)))
			sk := skill(0, "Spell", "Damage", "Physical")
			sk.CastTime = 1000
			sk.StatSets = []resolve.SkillStatSet{{Stats: []resolve.StatValue{
				stat("spell_minimum_base_physical_damage", 100), stat("spell_maximum_base_physical_damage", 100),
				stat("skill_physical_damage_%_to_convert_to_fire", 50),
			}}}
			setSkills(t, sh, sk)
			if got := outputOf(t, sh, poe1.FireMaximum); math.Abs(got-50) > 1e-9 {
				t.Errorf("fire hit: got %v, want 50", got)
			}
			if got := outputOf(t, sh, poe1.IgniteDPS); math.Abs(got-c.want) > 1e-9 {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

// The sheet gives ignite for a hit the character uses itself with one hand.
func TestIgniteOutputGiven(t *testing.T) {
	t.Parallel()
	var ignite poe1.Output
	for _, o := range poe1.Outputs() {
		if o.Key == poe1.IgniteDPS {
			ignite = o
		}
	}
	for _, c := range []struct {
		name  string
		skill *resolve.Skill
		want  bool
	}{
		{"none", nil, false},
		{"spell hit", skill(0, "Spell", "Damage"), true},
		{"spell hit by a totem", skill(0, "Spell", "Damage", "SummonsTotem"), false},
		{"damage over time", skill(0, "Spell", "DamageOverTime"), false},
	} {
		if got := poe1.Gives(ignite, c.skill, poe1.Hands{}); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// Ignite and the damage over time it adds to explain to their amounts.
func TestExplainIgnite(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global,
		resolve.StatValue{Stat: "base_chance_to_ignite_%", Value: 50}, resolve.StatValue{Stat: "critical_ailment_dot_multiplier_+", Value: 50},
		resolve.StatValue{Stat: "burn_damage_+%", Value: 50}, resolve.StatValue{Stat: "fire_dot_multiplier_+", Value: 20},
	)))
	setSkills(t, sh, fireSpell(5000))
	for _, k := range []output.Key{poe1.IgniteDPS, poe1.DotDPS} {
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
