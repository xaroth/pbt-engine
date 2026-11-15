package poe1_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// axeAttack is a sheet attacking once a second with an axe that deals 100 to 200 physical
// damage and strikes critically with crit chance in hundredths of a percent, always hitting an enemy without
// defences, with stats on a node and config values.
func axeAttack(t *testing.T, crit int32, values config.Values, stats ...resolve.StatValue) *poe1.Sheet {
	t.Helper()
	sh := openWith(t, []resolve.Bundle{
		bundle(resolve.CharacterSource{ID: "c"}, resolve.Global, resolve.StatValue{Stat: "base_critical_strike_multiplier", Value: 150}),
		bundle(resolve.NodeSource{ID: 1}, resolve.Global, append(stats,
			resolve.StatValue{Stat: "global_always_hit", Value: 1},
			resolve.StatValue{Stat: "critical_ailment_dot_multiplier_+", Value: 50},
		)...),
	}, values)
	axe := weapon("One Hand Axe", 100, 200, 1000, crit)
	if err := equip(sh, []poe1.Item{{Slot: build.SlotWeapon, Name: "Axe", Key: "axe", Weapon: axe}}); err != nil {
		t.Fatal(err)
	}
	sh.SetEnemy(poe1.EnemyStats{})
	attack := skill(0, "Attack", "Damage", "Melee")
	attack.StatSets = []resolve.SkillStatSet{{}}
	setSkills(t, sh, attack)
	return sh
}

// Bleeding deals 70% of the hit's physical damage per second for 5 seconds. The axe hits once a
// second, so 5 x the chance per hit bleeds apply over a duration. Only the strongest deals
// damage: with fewer than one the damage scales down at the middle of the hit's range (150);
// with n more, the strongest sits at n / (n + 1) of it. With Crimson Dance 8 deal damage at
// once, so 5 sit at the middle and all deal damage, 50% less. A moving enemy takes 200% more,
// but not with Crimson Dance.
func TestBleed(t *testing.T) {
	t.Parallel()
	stat := func(name string, value float64) resolve.StatValue { return resolve.StatValue{Stat: name, Value: value} }
	chance := func(v float64) resolve.StatValue { return stat("bleed_on_hit_with_attacks_%", v) }
	moving := map[string]string{"enemy-moving": "true"}
	for _, c := range []struct {
		name   string
		crit   int32
		config map[string]string
		stats  []resolve.StatValue
		want   float64
	}{
		{name: "no chance", want: 0},
		{name: "fewer than one", stats: []resolve.StatValue{chance(10)}, want: 150 * 0.7 * 0.5},
		{name: "always", stats: []resolve.StatValue{chance(100)}, want: (100 + 100*5.0/6) * 0.7},
		{name: "moving", config: moving, stats: []resolve.StatValue{chance(100)}, want: (100 + 100*5.0/6) * 0.7 * 3},
		{name: "crimson dance", stats: []resolve.StatValue{chance(100), stat("keystone_crimson_dance", 1)}, want: 150 * 0.7 * 5 * 0.5},
		{
			name: "crimson dance, moving", config: moving, stats: []resolve.StatValue{chance(100), stat("keystone_crimson_dance", 1)},
			want: 150 * 0.7 * 5 * 0.5,
		},
		{
			// 10 applied over a duration twice as long; dealt twice as fast, 2.5 over a quarter.
			name: "longer", stats: []resolve.StatValue{chance(100), stat("base_bleed_duration_+%", 100)}, want: (100 + 100*10.0/11) * 0.7,
		},
		{name: "faster", stats: []resolve.StatValue{chance(100), stat("faster_bleed_%", 100)}, want: (100 + 100*2.5/3.5) * 0.7 * 2},
		{
			// 0.5 per hit, 2.5 applied: roll 2.5/3.5, crit 1 - 0.5^2.5; from hits 0.5 x 0.5^2.5,
			// from crits 0.5 x (1 - 0.5^2.5), the latter with +50%.
			name: "critical strikes", crit: 5000, stats: []resolve.StatValue{chance(50)},
			want: (100 + 100*2.5/3.5) * (math.Pow(0.5, 2.5) + 1.5*(1-math.Pow(0.5, 2.5))) * 0.7,
		},
		{
			// Melee damage and physical damage with axes stay off bleeding; damage with
			// bleeding, axe ailment damage and damage with ailments from melee hits reach it.
			name: "modifiers", stats: []resolve.StatValue{
				chance(100), stat("melee_damage_+%", 100), stat("physical_axe_damage_+%", 100),
				stat("bleeding_damage_+%", 30), stat("axe_ailment_damage_+%", 20),
				stat("support_fortify_ailment_damage_+%_final_from_melee_hits", 50), stat("bleeding_dot_multiplier_+", 10),
			},
			want: (100 + 100*5.0/6) * 1.5 * 1.5 * 1.1 * 0.7,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			values := config.Values{}
			for k, v := range c.config {
				values[k] = json.RawMessage(v)
			}
			sh := axeAttack(t, c.crit, values, c.stats...)
			for _, k := range []output.Key{poe1.BleedDPS, poe1.DotDPS} {
				if got := outputOf(t, sh, k); math.Abs(got-c.want) > 1e-9 {
					t.Errorf("%s: got %v, want %v", k, got, c.want)
				}
			}
		})
	}
}

// Bleeding and the damage over time it adds to explain to their amounts.
func TestExplainBleed(t *testing.T) {
	t.Parallel()
	sh := axeAttack(t, 5000, config.Values{},
		resolve.StatValue{Stat: "bleed_on_hit_with_attacks_%", Value: 50}, resolve.StatValue{Stat: "bleeding_damage_+%", Value: 50},
		resolve.StatValue{Stat: "keystone_crimson_dance", Value: 1},
	)
	for _, k := range []output.Key{poe1.BleedDPS, poe1.DotDPS} {
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
