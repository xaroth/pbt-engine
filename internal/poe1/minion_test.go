package poe1_test

import (
	"math"
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// minionVersion is the dataset the minion tests read.
const minionVersion = "3.29.3.6"

// switchedSheet opens the sheet of b on the real dataset of minionVersion and switches it to b.
func switchedSheet(t *testing.T, b *build.Build) *poe1.Sheet {
	t.Helper()
	b.Format, b.Game, b.Version = 1, "poe1", minionVersion
	s := realdata.Open(t, minionVersion)
	c, _, err := build.Check(b, s)
	if err != nil {
		t.Fatal(err)
	}
	r, err := resolve.Resolve(c)
	if err != nil {
		t.Fatal(err)
	}
	sw, err := poe1.SwitchOf(c, poe1.DefaultConfig(), nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	sh := open(t, r)
	if err := sh.Switch(&sw); err != nil {
		t.Fatal(err)
	}
	return sh
}

// A minion at gem level 20 is level 70, the level its level row gives it. On 3.29, an allied
// monster at level 70 has 1,478 life, 332.05 damage and 5,081 evasion (DefaultMonsterStats)
// and 14,047 armour (monsterArmour); every monster's maximum resistances are 75% (Monster.ot).
//   - Raging Spirit (SummonedSkull): life 1,478 x 216% = 3,192; damage 332.05 x 102% x 0.57 s
//     attack time = 193.05, spread 20%: 154 to 232; 1 / 0.57 attacks per second; resistances
//     MinionGeneric's 40/40/40/20.
//   - Flame Golem (FireElementalSummoned): life 1,478 x 582% = 8,601; energy shield 8,601.96 x
//     40% (its type's 100 energy shield) = 3,440; damage 332.05 x 150% x 1 s = 498.08: 398 to
//     598; resistances MinionFlameGolem's 70/40/40/20.
//
// Neither type scales armour or evasion. Values not set read 0.
func TestMinionBaseValues(t *testing.T) {
	t.Parallel()
	const marauder = "Metadata/Characters/Str/Str"
	type want = map[string]float64
	common := want{
		"level": 70, "armour": 14047, "evasion_rating": 5081, "maximum_fire_resistance": 75,
		"maximum_cold_resistance": 75, "maximum_lightning_resistance": 75, "maximum_chaos_resistance": 75,
		"cold_resistance": 40, "lightning_resistance": 40, "chaos_resistance": 20,
	}
	for _, c := range []struct {
		gem  string
		want want
	}{
		{"Metadata/Items/Gems/SkillGemSummonRagingSpirit", want{
			"maximum_life": 3192, "maximum_energy_shield": 0, "fire_resistance": 40,
		}},
		{"Metadata/Items/Gems/SkillGemSummonFlameGolem", want{
			"maximum_life": 8601, "maximum_energy_shield": 3440, "fire_resistance": 70,
		}},
	} {
		t.Run(c.gem, func(t *testing.T) {
			t.Parallel()
			b := build.Build{Class: marauder, Level: 90, Skills: build.Skills{Groups: []build.SkillGroup{
				{Gems: []build.Gem{{ID: c.gem, Level: 20}}},
			}}}
			sh := switchedSheet(t, &b)
			for name, w := range common {
				wantMinion(t, sh, name, w)
			}
			for name, w := range c.want {
				wantMinion(t, sh, name, w)
			}
		})
	}
}

// A minion's attacks have its physical damage and attack rate: Raging Spirit's 154 to 232 at
// 1 / 0.57 attacks per second, Flame Golem's 398 to 598 at 1 (TestMinionBaseValues). Its
// summoning skill, a spell, reads none of them.
func TestMinionDamage(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		gem            string
		min, max, rate float64
	}{
		{"Metadata/Items/Gems/SkillGemSummonRagingSpirit", 154, 232, 1 / 0.57},
		{"Metadata/Items/Gems/SkillGemSummonFlameGolem", 398, 598, 1},
	} {
		b := build.Build{Class: "Metadata/Characters/Str/Str", Level: 90, Skills: build.Skills{Groups: []build.SkillGroup{
			{Gems: []build.Gem{{ID: c.gem, Level: 20}}},
		}}}
		sh := switchedSheet(t, &b)
		for name, w := range map[string]float64{"base_minimum_damage": c.min, "base_maximum_damage": c.max, "attack_rate": c.rate} {
			got, err := sh.MinionAttackValue(0, name, poe1.Physical)
			if err != nil {
				t.Fatal(err)
			}
			if got != w {
				t.Errorf("%s: minion %s %v, want %v", c.gem, name, got, w)
			}
		}
		wantMinion(t, sh, "base_minimum_damage", 0)
	}
}

func wantMinion(t *testing.T, sh *poe1.Sheet, name string, want float64) {
	t.Helper()
	got, err := sh.MinionValue(0, name)
	if err != nil {
		t.Fatal(err)
	}
	if d := got - want; d > 1e-9 || d < -1e-9 {
		t.Errorf("minion %s %v, want %v", name, got, want)
	}
}

// A minion's main skill hits through the hit rules a skill of the character's has, on the
// minion: its attack has the minion's damage and attack rate, 5% critical strike chance,
// Monster.ot's 130% critical strike multiplier and always hits, against the default enemy. Path
// of Building 2.67.2 gives the same average hit (Minion.MainHand.AverageHit) and hit damage per
// second (Minion.TotalDPS) for each gem at level 20, alone, on a level 90 Marauder without
// passives or items.
func TestMinionHit(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		gem      string
		hit, dps float64
	}{
		{"Metadata/Items/Gems/SkillGemRaiseZombie", 162.65375, 139.0202991453},
		{"Metadata/Items/Gems/SkillGemSummonSkeletons", 165.064375, 206.33046875},
		{"Metadata/Items/Gems/SkillGemSummonChaosGolem", 244.36125, 244.36125},
		{"Metadata/Items/Gems/SkillGemSummonIceGolem", 219.113125, 257.78014705882},
	} {
		t.Run(c.gem, func(t *testing.T) {
			t.Parallel()
			b := build.Build{Class: "Metadata/Characters/Str/Str", Level: 90, Skills: build.Skills{Groups: []build.SkillGroup{
				{Gems: []build.Gem{{ID: c.gem, Level: 20}}},
			}}}
			sh := switchedSheet(t, &b)
			for k, want := range map[output.Key]float64{poe1.MinionAverageHit: c.hit, poe1.MinionHitDPS: c.dps, poe1.MinionHitChance: 100} {
				got, err := sh.Output(k)
				if err != nil {
					t.Fatal(err)
				}
				if math.Abs(got-want) > 1e-9*want {
					t.Errorf("%s %v, want %v", k, got, want)
				}
			}
		})
	}
}

// What reaches a minion from the character: the supports linked to its gem, which support the
// minion's skill (Melee Physical Damage) or give minion stats (Minion Damage at quality 20: more
// and increased damage), an aura (Anger's added fire damage at its effect on the minion), and
// the summoning skill's own stats it passes (Absolution's half of physical damage converted to
// lightning). Path of Building 2.67.2 gives the same average hit
// and hit damage per second for each gem at level 20 on a level 90 Marauder without passives or
// items.
func TestMinionHitReached(t *testing.T) {
	t.Parallel()
	zombie := build.Gem{ID: "Metadata/Items/Gems/SkillGemRaiseZombie", Level: 20}
	for _, c := range []struct {
		name     string
		groups   []build.SkillGroup
		hit, dps float64
	}{
		{"supports", []build.SkillGroup{{Gems: []build.Gem{
			zombie,
			{ID: "Metadata/Items/Gems/SupportGemMinionDamage", Level: 20, Quality: 20, Support: true},
			{ID: "Metadata/Items/Gems/SupportGemMeleePhysicalDamage", Level: 20, Support: true},
		}}}, 370.601875, 285.07836538462},
		{"aura", []build.SkillGroup{
			{Gems: []build.Gem{zombie}},
			{Gems: []build.Gem{{ID: "Metadata/Items/Gems/SkillGemAnger", Level: 20}}},
		}, 229.64375, 196.27670940171},
		{"passed", []build.SkillGroup{{Gems: []build.Gem{{ID: "Metadata/Items/Gems/SkillGemAbsolution", Level: 20}}}}, 414.0565625, 353.89449786325},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := build.Build{Class: "Metadata/Characters/Str/Str", Level: 90, Skills: build.Skills{Groups: c.groups}}
			sh := switchedSheet(t, &b)
			for k, want := range map[output.Key]float64{poe1.MinionAverageHit: c.hit, poe1.MinionHitDPS: c.dps} {
				got, err := sh.Output(k)
				if err != nil {
					t.Fatal(err)
				}
				if math.Abs(got-want) > 1e-9*want {
					t.Errorf("%s %v, want %v", k, got, want)
				}
			}
		})
	}
}

// The sheet gives a minion's hit for a skill whose minion attacks with damage of its own: not
// for one whose minion attacks with items and has none (Animate Guardian without gear) or casts
// spells (Flame Golem), nor for a skill without a minion.
func TestMinionHitGiven(t *testing.T) {
	t.Parallel()
	hit := poe1.Outputs()[slices.IndexFunc(poe1.Outputs(), func(o poe1.Output) bool { return o.Key == poe1.MinionAverageHit })]
	for gem, want := range map[string]bool{
		"Metadata/Items/Gems/SkillGemSummonRagingSpirit": true,
		"Metadata/Items/Gems/SkillGemAnimateArmour":      false,
		"Metadata/Items/Gems/SkillGemSummonFlameGolem":   false,
		"Metadata/Items/Gems/SkillGemFireball":           false,
	} {
		b := build.Build{
			Format: 1, Game: "poe1", Version: "3.29.3.6", Class: "Metadata/Characters/Str/Str", Level: 90,
			Skills: build.Skills{Groups: []build.SkillGroup{{Gems: []build.Gem{{ID: gem, Level: 20}}}}},
		}
		c, _, err := build.Check(&b, realdata.Open(t, minionVersion))
		if err != nil {
			t.Fatal(err)
		}
		sw, err := poe1.SwitchOf(c, poe1.DefaultConfig(), nil, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		if minion := gem != "Metadata/Items/Gems/SkillGemFireball"; (sw.Minion(0) != nil) != minion {
			t.Errorf("%s: minion %v, want one %v", gem, sw.Minion(0), minion)
		}
		if got := poe1.Gives(hit, sw.Skills.Main, sw.Minion(0), sw.Hands); got != want {
			t.Errorf("%s: minion hit given %v, want %v", gem, got, want)
		}
	}
}

// Switching a sheet again to the same build adds no sources: each switch resolves the minion's
// skill anew, and the sheet knows it by its values, as a pooled sheet reused for every
// simulation needs.
func TestMinionSwitchAddsOnce(t *testing.T) {
	t.Parallel()
	b := build.Build{Class: "Metadata/Characters/Str/Str", Level: 90, Skills: build.Skills{Groups: []build.SkillGroup{
		{Gems: []build.Gem{{ID: "Metadata/Items/Gems/SkillGemSummonSkeletons", Level: 20}}},
	}}}
	sh := switchedSheet(t, &b)
	c, _, err := build.Check(&b, realdata.Open(t, minionVersion))
	if err != nil {
		t.Fatal(err)
	}
	want := sh.SourceCount()
	for range 3 {
		sw, err := poe1.SwitchOf(c, poe1.DefaultConfig(), nil, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		if err := sh.Switch(&sw); err != nil {
			t.Fatal(err)
		}
		if got := sh.SourceCount(); got != want {
			t.Fatalf("%d sources after switching again, want %d", got, want)
		}
	}
	got, err := sh.Output(poe1.MinionAverageHit)
	if err != nil || got != 165.064375 {
		t.Errorf("minion average hit %v (%v), want 165.064375", got, err)
	}
}

// A minion's hit ignites as the character's does, read on the minion: a level 20 Raise Zombie
// with "Minions have 100% chance to Ignite" and "Minions deal 100 to 200 additional Fire Damage"
// on a ring, on a level 90 Marauder without passives, its group counted in full DPS. The
// minion's critical strikes add Monster.ot's +30% to the Damage over Time Multiplier of their
// ailments. Path of Building 2.67.2 gives the same average hit, hit damage per second, ignite
// damage per second (Minion.IgniteDPS) and full DPS, the hit's plus the ignite's.
func TestMinionIgnite(t *testing.T) {
	t.Parallel()
	const hitDPS, igniteDPS = 204.0844017094, 83.667697944825
	b := build.Build{
		Format: 1, Game: "poe1", Version: minionVersion, Class: "Metadata/Characters/Str/Str", Level: 90,
		Skills: build.Skills{Groups: []build.SkillGroup{{Gems: []build.Gem{{ID: raiseZombie, Level: 20}}, FullDPS: true}}},
	}
	ring := poe1.Item{Slot: build.SlotRing1, Name: "Test Loop", Key: "ring", Stats: []resolve.StatValue{
		{Stat: "minion_chance_to_ignite_%", Value: 100},
		{Stat: "minion_global_minimum_added_fire_damage", Value: 100},
		{Stat: "minion_global_maximum_added_fire_damage", Value: 200},
	}}
	sh := gearSheet(t, &b, ring)
	for k, want := range map[output.Key]float64{
		poe1.MinionAverageHit: 238.77875, poe1.MinionHitDPS: hitDPS, poe1.MinionIgniteDPS: igniteDPS, poe1.TotalDPS: hitDPS + igniteDPS,
	} {
		got, err := sh.Output(k)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(got-want) > 1e-9*want {
			t.Errorf("%s %v, want %v", k, got, want)
		}
	}
}

// The character's stats that act on its minions, each on a level 20 Raise Zombie of a level 90
// Marauder without passives, whose minion crits at 5% with Monster.ot's 130% multiplier and
// whose average hit is zombieHit. Raise Zombie's minions stay until they die, so using it is
// not using a Minion Skill Recently. The values follow PoB's ModParser.lua for each line; the
// average hit rounds each damage roll, so 30% more damage from Meat Shield gives the same
// (TestMinionBuffs).
func TestMinionStatsOfTheCharacter(t *testing.T) {
	t.Parallel()
	alone := []build.SkillGroup{{Gems: []build.Gem{{ID: raiseZombie, Level: 20}}}}
	on := func(k config.Key[bool]) func(*config.Values) error {
		return func(v *config.Values) error { return config.Set(v, k, true) }
	}
	const fullLife = "minion_chance_to_deal_double_damage_while_on_full_life_%"
	for _, c := range []struct {
		name  string
		stat  string
		value float64
		set   func(*config.Values) error
		// read is the minion's engine stat, read on its attack's physical damage.
		read string
		want float64
		hit  float64
	}{
		{"double damage", "minion_chance_to_deal_double_damage_%", 8, nil, "double_damage_chance", 8, zombieHit * 1.08},
		{"double damage on full life", fullLife, 5, on(poe1.MinionsOnFullLife), "double_damage_chance", 5, zombieHit * 1.05},
		{"double damage, not on full life", fullLife, 5, nil, "double_damage_chance", 0, zombieHit},
		{"used a minion skill recently", "minion_damage_+%_if_have_used_a_minion_skill_recently", 30, on(poe1.UsedMinionSkillRecently), "damage", 130, 211.500625},
		{"no minion skill used recently", "minion_damage_+%_if_have_used_a_minion_skill_recently", 30, nil, "damage", 100, zombieHit},
		// The Marauder's 3 maximum power charges, not the minion's: 150% increased, 12.5%.
		{"critical strike chance per maximum power charge", "minion_critical_strike_chance_+%_per_maximum_power_charge", 50, nil, "critical_strike_chance", 12.5, 0},
		// +5% to critical strike chance: 10%.
		{"base critical strike chance", "minion_additional_base_critical_strike_chance", 500, nil, "critical_strike_chance", 10, 0},
		// The Whispers: 50% increased damage and attack speed.
		{"whispers", "minions_go_crazy_on_crit_ms", 5000, nil, "damage", 150, 244.1075},
		{"whispers' attack speed", "minions_go_crazy_on_crit_ms", 5000, nil, "attack_speed", 150, 0},
		// Unholy Might converts all physical damage to chaos.
		{"unholy might", "minion_has_unholy_might", 1, nil, "damage_converted_to_chaos", 100, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			ring := poe1.Item{Slot: build.SlotRing1, Name: "ring", Key: c.stat, Stats: []resolve.StatValue{{Stat: c.stat, Value: c.value}}}
			sh := configuredSheet(t, &build.Build{Skills: build.Skills{Groups: alone}}, c.set, ring)
			got, err := sh.MinionAttackValue(0, c.read, poe1.Physical)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(got-c.want) > 1e-9 {
				t.Errorf("minion %s %v, want %v", c.read, got, c.want)
			}
			if c.hit == 0 {
				return
			}
			if got := outputOf(t, sh, poe1.MinionAverageHit); math.Abs(got-c.hit) > 1e-9*c.hit {
				t.Errorf("minion average hit %v, want %v", got, c.hit)
			}
		})
	}
}
