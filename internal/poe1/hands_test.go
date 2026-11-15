package poe1_test

import (
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// weapon compiles a made-up one handed weapon of class with physical damage lo to hi, ms per
// attack and critical strike chance in hundredths of a percent.
func weapon(class string, lo, hi, ms, crit int32) *poe1.Weapon {
	base := &dataset.ItemBase{Class: class, Weapon: &dataset.WeaponValues{
		PhysicalDamage: dataset.ValueRange{Min: lo, Max: hi}, AttackTime: ms, CriticalChance: crit,
	}}
	return poe1.CompileItem(&build.Item{Name: class}, base).Weapon
}

// An attack dual wielding a claw (10 to 20, 1.5 per second, 5%) and a dagger (20 to 40, 1.2 per
// second, 10%) against an enemy without defences, with 200% critical strike multiplier, 50%
// increased claw physical damage and the character's 10% more attack speed while dual wielding.
// Main hand: 15 to 30, average 22.5 x (0.95 + 0.05 x 2) = 23.625 at 1.5 x 1.1 = 1.65 per second.
// Off hand: 20 to 40, average 30 x (0.9 + 0.1 x 2) = 33 at 1.2 x 1.1 = 1.32. Alternating hands
// the average hit and critical strike chance average, hits per second are the harmonic mean, and
// hit DPS is the average hit times it, each explained in full by its sources; a skill that hits
// with both at once adds the hands' hits.
func TestDualWieldAttack(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global,
		resolve.StatValue{Stat: "base_critical_strike_multiplier", Value: 200},
		resolve.StatValue{Stat: "physical_claw_damage_+%", Value: 50},
		resolve.StatValue{Stat: "dual_wield_inherent_attack_speed_+%_final", Value: 10},
	)))
	claw, dagger := weapon("Claw", 10, 20, 666, 500), weapon("Dagger", 20, 40, 833, 1000)
	if err := sh.SetHands(poe1.Hands{MainHand: claw, OffHand: dagger}); err != nil {
		t.Fatal(err)
	}
	sh.SetEnemy(poe1.EnemyStats{})
	attack := skill(0, "Attack", "Damage", "Melee")
	attack.StatSets = []resolve.SkillStatSet{{}}
	setSkills(t, sh, attack)
	rate := 2 / (1/1.65 + 1/1.32)
	check := func(name string, want map[output.Key]float64) {
		t.Helper()
		for k, w := range want {
			if got := outputOf(t, sh, k); math.Abs(got-w) > 1e-9 {
				t.Errorf("%s: %s: got %v, want %v", name, k, got, w)
			}
		}
	}
	check("alternating", map[output.Key]float64{
		poe1.MainHandPhysicalMinimum: 15, poe1.OffHandPhysicalMinimum: 20,
		poe1.MainHandAverageHit: 23.625, poe1.OffHandAverageHit: 33, poe1.AverageHit: (23.625 + 33) / 2,
		poe1.CriticalStrikeChance: 7.5, poe1.HitChance: 100, poe1.HitsPerSecond: rate, poe1.HitDPS: (23.625 + 33) / 2 * rate,
	})

	for _, k := range []output.Key{poe1.AverageHit, poe1.CriticalStrikeChance, poe1.HitsPerSecond, poe1.HitDPS} {
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

	both := skill(0, "Attack", "Damage", "Melee")
	both.StatSets = []resolve.SkillStatSet{{Stats: []resolve.StatValue{{Stat: "skill_double_hits_when_dual_wielding", Value: 1}}}}
	setSkills(t, sh, both)
	check("both at once", map[output.Key]float64{
		poe1.AverageHit: 23.625 + 33, poe1.HitsPerSecond: rate, poe1.HitDPS: (23.625 + 33) * rate,
	})

	if err := sh.SetHands(poe1.Hands{MainHand: claw}); err != nil {
		t.Fatal(err)
	}
	setSkills(t, sh, attack)
	check("one hand", map[output.Key]float64{poe1.AverageHit: 23.625, poe1.HitsPerSecond: 1.5})
}
