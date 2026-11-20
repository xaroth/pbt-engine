package poe1_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// A melee attack with a made-up mace (maceAttack) against an enemy without defences. Double damage counts only on hits that do not
// deal triple damage. Every third hit is a Ruthless Blow, counted as 33% of hits, rounded as
// PoB rounds it. The effects scale the hit before armour reduces it: against 500 armour a hit
// of 200 loses 500 / (500 + 5 x 200). Maim's damage taken holds while the enemy is Maimed;
// Tailwind's 8% increased action speed speeds up the attack.
func TestHitEffects(t *testing.T) {
	t.Parallel()
	stat := func(name string, value float64) resolve.StatValue { return resolve.StatValue{Stat: name, Value: value} }
	ruthless := []resolve.StatValue{stat("support_ruthless_big_hit_max_count", 3), stat("support_ruthless_big_hit_damage_+%_final", 83)}
	maim := []resolve.StatValue{stat("support_maimed_enemies_physical_damage_taken_+%", 10)}
	for _, c := range []struct {
		name     string
		stats    []resolve.StatValue
		buff     []resolve.StatValue
		config   map[string]string
		notMelee bool
		armour   int
		hit      float64
		rate     float64
	}{
		{name: "none", hit: 100, rate: 1},
		{name: "double damage", stats: []resolve.StatValue{stat("chance_to_deal_double_damage_%", 20)}, hit: 120, rate: 1},
		{name: "double damage above 100%", stats: []resolve.StatValue{stat("chance_to_deal_double_damage_%", 150)}, hit: 200, rate: 1},
		{name: "triple damage", stats: []resolve.StatValue{stat("base_chance_to_deal_triple_damage_%", 10)}, hit: 120, rate: 1},
		{
			name:  "double and triple damage",
			stats: []resolve.StatValue{stat("chance_to_deal_double_damage_%", 20), stat("base_chance_to_deal_triple_damage_%", 10)},
			hit:   100 * (1 + 0.2*0.9 + 0.2), rate: 1,
		},
		{name: "ruthless", stats: ruthless, hit: 100 * (1 + 0.33*0.83), rate: 1},
		{name: "ruthless, not melee", stats: ruthless, notMelee: true, hit: 100, rate: 1},
		{
			name: "ruthless and double damage", stats: append([]resolve.StatValue{stat("chance_to_deal_double_damage_%", 20)}, ruthless...),
			hit: 100 * 1.2 * (1 + 0.33*0.83), rate: 1,
		},
		{
			name: "double damage before armour", stats: []resolve.StatValue{stat("chance_to_deal_double_damage_%", 100)}, armour: 500,
			hit: 200 * (1 - 500.0/1500), rate: 1,
		},
		{name: "maim, not maimed", stats: maim, hit: 100, rate: 1},
		{name: "maim, maimed", stats: maim, config: map[string]string{"enemy-maimed": "true"}, hit: 110, rate: 1},
		{name: "tailwind", buff: []resolve.StatValue{stat("action_speed_-%", -8)}, hit: 100, rate: 1.08},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			values := config.Values{}
			for k, v := range c.config {
				values[k] = json.RawMessage(v)
			}
			bundles := []resolve.Bundle{bundle(resolve.NodeSource{ID: 1}, resolve.Global, c.stats...)}
			if c.buff != nil {
				bundles = append(bundles, bundle(resolve.NodeSource{ID: 2}, resolve.Buff, c.buff...))
			}
			sh := maceAttack(t, openWith(t, bundles, values), c.armour, !c.notMelee)
			for k, want := range map[output.Key]float64{
				poe1.PhysicalMinimum: c.hit, poe1.AverageHit: c.hit, poe1.HitsPerSecond: c.rate,
			} {
				if got := outputOf(t, sh, k); math.Abs(got-want) > 1e-9 {
					t.Errorf("%s: got %v, want %v", k, got, want)
				}
			}
			linesAddUp(t, sh, poe1.PhysicalMinimum, poe1.AverageHit, poe1.HitsPerSecond)
		})
	}
}

// maceAttack sets sh to attack with a made-up mace that hits for 100 physical damage once a
// second, never critically, against an enemy with armour and no other defences; a melee attack
// when melee is set.
func maceAttack(t *testing.T, sh *poe1.Sheet, armour int, melee bool) *poe1.Sheet {
	t.Helper()
	if err := sh.SetHands(poe1.Hands{MainHand: weapon("One Hand Mace", 100, 100, 1000, 0)}); err != nil {
		t.Fatal(err)
	}
	sh.SetEnemy(poe1.EnemyStats{Armour: armour})
	types := []string{"Attack", "Damage"}
	if melee {
		types = append(types, "Melee")
	}
	attack := skill(0, types...)
	attack.StatSets = []resolve.SkillStatSet{{}}
	setSkills(t, sh, attack)
	return sh
}

// linesAddUp checks that the proportional explanation of each of keys on sh adds up to it.
func linesAddUp(t *testing.T, sh *poe1.Sheet, keys ...output.Key) {
	t.Helper()
	for _, k := range keys {
		ex, err := sh.Explain(k, poe1.Proportional)
		if err != nil {
			t.Fatal(err)
		}
		sum := 0.0
		for _, l := range ex.Lines {
			sum += l.Value
		}
		if math.Abs(sum-ex.Amount) > 1e-9 {
			t.Errorf("%s: lines add to %v, want %v", k, sum, ex.Amount)
		}
	}
}
