package poe1_test

import (
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// axe is a made-up two handed axe: 10 to 20 base physical damage, 800 ms per attack, 5%
// critical strike chance, 20% quality, with local added physical and lightning damage, increased
// physical damage, attack speed and critical strike chance, and a global stat.
func axe() (*build.Item, *dataset.ItemBase) {
	line := func(stats ...build.StatValue) build.ModLine { return build.ModLine{Kind: build.Explicit, Stats: stats} }
	it := &build.Item{Name: "Axe", Quality: 20, Mods: []build.ModLine{
		line(build.StatValue{Stat: "local_minimum_added_physical_damage", Value: 5}, build.StatValue{Stat: "local_maximum_added_physical_damage", Value: 10}),
		line(build.StatValue{Stat: "local_minimum_added_lightning_damage", Value: 1}, build.StatValue{Stat: "local_maximum_added_lightning_damage", Value: 30}),
		line(build.StatValue{Stat: "local_physical_damage_+%", Value: 50}),
		line(build.StatValue{Stat: "local_attack_speed_+%", Value: 10}),
		line(build.StatValue{Stat: "local_critical_strike_chance_+%", Value: 20}),
		line(build.StatValue{Stat: "additional_strength", Value: 10}),
	}}
	base := &dataset.ItemBase{Class: "Two Hand Axe", Weapon: &dataset.WeaponValues{
		PhysicalDamage: dataset.ValueRange{Min: 10, Max: 20}, AttackTime: 800, CriticalChance: 500,
	}}
	return it, base
}

// A weapon's physical damage is (base + local added) x the local increase x the quality, its
// lightning damage the local added damage; attacks per second are 1.25 x 1.1 = 1.375, rounded to
// 1.38; critical strike chance 5% x 1.2. The local stats stay on the weapon.
func TestCompileWeapon(t *testing.T) {
	t.Parallel()
	got := poe1.CompileItem(axe())
	if got.Weapon == nil {
		t.Fatal("no weapon")
	}
	w := got.Weapon
	want := [poe1.Chaos + 1]poe1.DamageRange{poe1.Physical: {Min: 27, Max: 54}, poe1.Lightning: {Min: 1, Max: 30}}
	if diff := cmp.Diff(want, w.Damage); diff != "" {
		t.Errorf("damage (-want +got):\n%s", diff)
	}
	if w.AttackRate != 1.38 || w.CriticalChance != 6 {
		t.Errorf("attack rate %v and critical strike chance %v, want 1.38 and 6", w.AttackRate, w.CriticalChance)
	}
	if diff := cmp.Diff([]resolve.StatValue{{Stat: "additional_strength", Value: 10}}, got.Stats); diff != "" {
		t.Errorf("stats (-want +got):\n%s", diff)
	}
	if poe1.CompileItem(&build.Item{}, &dataset.ItemBase{Class: "Shield"}).Weapon != nil {
		t.Error("a shield compiled to a weapon")
	}
}

// An attack with the axe against an enemy without defences: physical 27 to 54, 50% increased (40.5 rounds to 41) by
// two handed melee and axe modifiers but not by one handed ones; lightning 1 to 30 with 10 added
// lightning damage to attacks at 150% effectiveness, 16 to 45. A 6% chance of a 150% critical
// strike. 1.38 attacks per second, 10% more from the skill, 20% increased: 1.38 x 1.1 x 1.2.
// A spell gets none of the weapon's values.
func TestAttackHit(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global,
		resolve.StatValue{Stat: "base_critical_strike_multiplier", Value: 150},
		resolve.StatValue{Stat: "two_handed_melee_physical_damage_+%", Value: 30},
		resolve.StatValue{Stat: "physical_axe_damage_+%", Value: 20},
		resolve.StatValue{Stat: "one_handed_melee_physical_damage_+%", Value: 100},
		resolve.StatValue{Stat: "attack_minimum_added_lightning_damage", Value: 10},
		resolve.StatValue{Stat: "attack_maximum_added_lightning_damage", Value: 10},
		resolve.StatValue{Stat: "attack_speed_+%", Value: 20},
	)))
	c := poe1.CompileItem(axe())
	if err := sh.SetItems([]poe1.Item{{Slot: build.SlotWeapon, Name: "Axe", Key: "axe", Stats: c.Stats, Weapon: c.Weapon}}); err != nil {
		t.Fatal(err)
	}
	sh.SetEnemy(poe1.EnemyStats{})
	attack := skill(0, "Attack", "Damage", "Melee")
	attack.AttackSpeedMultiplier = 10
	attack.StatSets = []resolve.SkillStatSet{{DamageEffectiveness: 5000}}
	setSkills(t, sh, attack)
	physical, lightning := (41.0+81)/2, (16.0+45)/2
	for k, want := range map[output.Key]float64{
		poe1.PhysicalMinimum: 41, poe1.PhysicalMaximum: 81, poe1.LightningMinimum: 16, poe1.LightningMaximum: 45,
		poe1.CriticalStrikeChance: 6, poe1.AverageHit: (physical + lightning) * (0.94 + 0.06*1.5),
		poe1.HitsPerSecond: 1.38 * 1.1 * 1.2,
	} {
		if got := outputOf(t, sh, k); math.Abs(got-want) > 1e-9 {
			t.Errorf("%s: got %v, want %v", k, got, want)
		}
	}
	setSkills(t, sh, spell())
	if got := outputOf(t, sh, poe1.PhysicalMinimum); got != 0 {
		t.Errorf("spell's physical minimum: got %v, want 0", got)
	}
}
