package poe1_test

import (
	"math"
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
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
	sw, err := poe1.SwitchOf(c, poe1.DefaultConfig(), nil, false)
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
// for one whose minion attacks with items (Animate Guardian) or casts spells (Flame Golem), nor
// for a skill without a minion.
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
		sw, err := poe1.SwitchOf(c, poe1.DefaultConfig(), nil, false)
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
		sw, err := poe1.SwitchOf(c, poe1.DefaultConfig(), nil, false)
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
