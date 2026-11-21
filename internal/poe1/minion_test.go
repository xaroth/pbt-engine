package poe1_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// switchedSheet opens the sheet of b on the real dataset of version and switches it to b.
func switchedSheet(t *testing.T, version string, b *build.Build) *poe1.Sheet {
	t.Helper()
	b.Format, b.Game, b.Version = 1, "poe1", version
	s := realdata.Open(t, version)
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
// monster at level 70 has 1,478 life, 332.05 damage, 33,714 armour and 5,081 evasion
// (DefaultMonsterStats); every monster's maximum resistances are 75% (Monster.ot).
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
		"level": 70, "armour": 33714, "evasion_rating": 5081, "maximum_fire_resistance": 75,
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
			sh := switchedSheet(t, "3.29.3.6", &b)
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
		sh := switchedSheet(t, "3.29.3.6", &b)
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
