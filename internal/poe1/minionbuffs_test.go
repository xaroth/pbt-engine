package poe1_test

import (
	"errors"
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// zombieHit and zombieDPS are a level 20 Raise Zombie's minion's average hit and hit damage per
// second without buffs (TestMinionHit).
const (
	zombieHit = 162.65375
	zombieDPS = 139.0202991453
)

// The buffs that reach a minion: Flesh Offering's buff, Feeding Frenzy, Meat Shield's more
// damage against enemies near you and frenzy charges, each on a level 20 Raise Zombie, gems at
// level 20 and quality 20, on a level 90 Marauder without passives or items. Path of Building
// 2.67.2 gives the same average hit (Minion.MainHand.AverageHit) and hit damage per second
// (Minion.TotalDPS) with each config set, and the unbuffed values with it off.
func TestMinionBuffs(t *testing.T) {
	t.Parallel()
	zombie := build.Gem{ID: raiseZombie, Level: 20}
	support := func(id string) build.Gem {
		return build.Gem{ID: "Metadata/Items/Gems/" + id, Level: 20, Quality: 20, Support: true}
	}
	feedingFrenzy := []build.SkillGroup{{Gems: []build.Gem{zombie, support("SupportGemFeedingFrenzy")}}}
	meatShield := []build.SkillGroup{{Gems: []build.Gem{zombie, support("SupportGemMeatShield")}}}
	fleshOffering := []build.SkillGroup{
		{Gems: []build.Gem{zombie}},
		{Gems: []build.Gem{{ID: "Metadata/Items/Gems/SkillGemFleshOffering", Level: 20, Quality: 20}}},
	}
	alone := []build.SkillGroup{{Gems: []build.Gem{zombie}}}
	on := func(k config.Key[bool]) func(*config.Values) error {
		return func(v *config.Values) error { return config.Set(v, k, true) }
	}
	charges := func(n int, use bool) func(*config.Values) error {
		return func(v *config.Values) error {
			return errors.Join(config.Set(v, poe1.MinionFrenzyCharges, use), config.Set(v, poe1.MinionFrenzyChargeCount, poe1.ChargeCount(n)))
		}
	}
	for _, c := range []struct {
		name     string
		groups   []build.SkillGroup
		set      func(*config.Values) error
		hit, dps float64
	}{
		// 10% more damage, 10% increased attack speed.
		{"feeding frenzy", feedingFrenzy, on(poe1.FeedingFrenzy), 179.020625, 168.30998931624},
		{"feeding frenzy off", feedingFrenzy, nil, zombieHit, zombieDPS},
		// Feeding Frenzy is a buff on you that reaches every minion.
		{"feeding frenzy unsupported", alone, on(poe1.FeedingFrenzy), 179.020625, 168.30998931624},
		// 30% more damage.
		{"meat shield", meatShield, on(poe1.EnemyNearYou), 211.500625, 180.76976495726},
		{"meat shield off", meatShield, nil, zombieHit, zombieDPS},
		// 30% increased attack speed at 110% effect: 33%.
		{"flesh offering", fleshOffering, nil, zombieHit, 184.89699786325},
		{"flesh offering off", alone, nil, zombieHit, zombieDPS},
		// Each charge 4% more damage and 4% increased attack speed: the maximum, 3, when no count is
		// given; a count above it is not capped.
		{"frenzy charges", alone, charges(0, true), 182.1925, 174.4064957265},
		{"frenzy charges 2", alone, charges(2, true), 175.721875, 162.20480769231},
		{"frenzy charges 5", alone, charges(5, true), 195.13375, 200.13717948718},
		{"frenzy charges off", alone, charges(2, false), zombieHit, zombieDPS},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			sh := configuredSheet(t, &build.Build{Skills: build.Skills{Groups: c.groups}}, c.set)
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

// configuredSheet opens the sheet of b, a level 90 Marauder, on the real dataset of
// minionVersion with the config set gives, and switches it to b.
func configuredSheet(t *testing.T, b *build.Build, set func(*config.Values) error) *poe1.Sheet {
	t.Helper()
	b.Format, b.Game, b.Version, b.Class, b.Level = 1, "poe1", minionVersion, "Metadata/Characters/Str/Str", 90
	var v config.Values
	if set != nil {
		if err := set(&v); err != nil {
			t.Fatal(err)
		}
	}
	cfg, problems := config.Resolve(poe1.ConfigSpecs(), config.Layer{Source: config.FromBuild, Values: v})
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	c, _, err := build.Check(b, realdata.Open(t, minionVersion))
	if err != nil {
		t.Fatal(err)
	}
	r, err := resolve.Resolve(c)
	if err != nil {
		t.Fatal(err)
	}
	sw, err := poe1.SwitchOf(c, cfg, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	sh := open(t, r)
	if err := sh.Switch(&sw); err != nil {
		t.Fatal(err)
	}
	return sh
}
