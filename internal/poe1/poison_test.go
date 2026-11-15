package poe1_test

import (
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// poisonSpell is a sheet with a made-up spell that deals 100 to 200 base physical and chaos
// damage once a second and strikes critically with crit chance in hundredths of a percent,
// against an enemy without resistances, with stats on a node.
func poisonSpell(t *testing.T, crit int32, stats ...resolve.StatValue) *poe1.Sheet {
	t.Helper()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global, stats...)))
	sh.SetEnemy(poe1.EnemyStats{})
	sk := skill(0, "Spell", "Damage", "Physical", "Chaos")
	sk.CastTime = 1000
	sk.StatSets = []resolve.SkillStatSet{{SpellCritChance: crit, Stats: []resolve.StatValue{
		{Stat: "spell_minimum_base_physical_damage", Value: 100}, {Stat: "spell_maximum_base_physical_damage", Value: 200},
		{Stat: "spell_minimum_base_chaos_damage", Value: 100}, {Stat: "spell_maximum_base_chaos_damage", Value: 200},
	}}}
	setSkills(t, sh, sk)
	return sh
}

// Poison deals 30% of the hit's physical and chaos damage per second for 2 seconds, from the
// middle of the hit's range (150 of each). Every poison deals damage: the spell hits once a
// second, so 2 x the chance per hit are on the enemy at once.
// A poison from a critical strike, as often as the hit strikes critically, takes the +50%
// Damage over Time Multiplier for Poison from Critical Strikes.
func TestPoison(t *testing.T) {
	t.Parallel()
	stat := func(name string, value float64) resolve.StatValue { return resolve.StatValue{Stat: name, Value: value} }
	chance := func(v float64) resolve.StatValue { return stat("base_chance_to_poison_on_hit_%", v) }
	for _, c := range []struct {
		name  string
		crit  int32
		stats []resolve.StatValue
		want  float64
	}{
		{name: "no chance", want: 0},
		{name: "fewer than one", stats: []resolve.StatValue{chance(20)}, want: 300 * 0.3 * 0.4},
		{name: "always", stats: []resolve.StatValue{chance(100)}, want: 300 * 0.3 * 2},
		{
			// Twice as long holds twice as many; twice as fast deals each twice the damage
			// per second over half as long.
			name: "longer and faster", stats: []resolve.StatValue{chance(100), stat("base_poison_duration_+%", 100), stat("faster_poison_%", 100)},
			want: 300 * 0.3 * 2 * 2,
		},
		{
			name: "critical strikes", crit: 5000, stats: []resolve.StatValue{chance(100), stat("critical_poison_dot_multiplier_+", 50)},
			want: 300 * 0.3 * 1.25 * 2,
		},
		{
			// Spell damage stays off poison; damage with poison and the Damage over Time
			// Multiplier for Poison reach it.
			name: "modifiers", stats: []resolve.StatValue{
				chance(100), stat("spell_damage_+%", 100), stat("base_poison_damage_+%", 50), stat("poison_dot_multiplier_+", 20),
			},
			want: 300 * 1.5 * 1.2 * 0.3 * 2,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			sh := poisonSpell(t, c.crit, c.stats...)
			for _, k := range []output.Key{poe1.PoisonDPS, poe1.DotDPS} {
				if got := outputOf(t, sh, k); math.Abs(got-c.want) > 1e-9 {
					t.Errorf("%s: got %v, want %v", k, got, c.want)
				}
			}
		})
	}
}

// Poison and the damage over time it adds to explain to their amounts.
func TestExplainPoison(t *testing.T) {
	t.Parallel()
	sh := poisonSpell(t, 5000,
		resolve.StatValue{Stat: "base_chance_to_poison_on_hit_%", Value: 50}, resolve.StatValue{Stat: "critical_poison_dot_multiplier_+", Value: 50},
		resolve.StatValue{Stat: "base_poison_damage_+%", Value: 50},
	)
	for _, k := range []output.Key{poe1.PoisonDPS, poe1.DotDPS} {
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
