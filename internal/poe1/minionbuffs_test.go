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
		return minionCharges(poe1.MinionFrenzyCharges, poe1.MinionFrenzyChargeCount, n, use)
	}
	power := func(n int, use bool) func(*config.Values) error {
		return minionCharges(poe1.MinionPowerCharges, poe1.MinionPowerChargeCount, n, use)
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
		// Each power charge 50% increased critical strike chance on the minion's 5%: 7.5%, 10%,
		// 12.5% with the maximum, 3, when no count is given, and 17.5%, not capped.
		{"power charges", alone, power(0, true), 166.259375, 142.10202991453},
		{"power charges 1", alone, power(1, true), 163.855625, 140.04754273504},
		{"power charges 2", alone, power(2, true), 165.0575, 141.07478632479},
		{"power charges 5", alone, power(5, true), 168.663125, 144.15651709402},
		{"power charges off", alone, power(2, false), zombieHit, zombieDPS},
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

func minionCharges(on config.Key[bool], count config.Key[poe1.ChargeCount], n int, use bool) func(*config.Values) error {
	return func(v *config.Values) error {
		return errors.Join(config.Set(v, on, use), config.Set(v, count, poe1.ChargeCount(n)))
	}
}

// Each endurance charge gives a minion 4% physical damage reduction and 4% elemental damage
// reduction (Monster.ot): its maximum, 3, when no count is given; a count above it is not
// capped. Path of Building 2.67.2 gives the minion the same per charge (CalcPerform.lua
// initMinionModDB); it shows no output of them the comparison reads.
func TestMinionEnduranceCharges(t *testing.T) {
	t.Parallel()
	alone := []build.SkillGroup{{Gems: []build.Gem{{ID: raiseZombie, Level: 20}}}}
	for _, c := range []struct {
		name    string
		n       int
		use     bool
		charges float64
	}{
		{"maximum", 0, true, 3},
		{"2", 2, true, 2},
		{"5", 5, true, 5},
		{"off", 2, false, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			sh := configuredSheet(t, &build.Build{Skills: build.Skills{Groups: alone}},
				minionCharges(poe1.MinionEnduranceCharges, poe1.MinionEnduranceChargeCount, c.n, c.use))
			for name, want := range map[string]float64{
				"endurance_charges": c.charges, "physical_damage_reduction": 4 * c.charges, "elemental_damage_reduction": 4 * c.charges,
			} {
				wantMinion(t, sh, name, want)
			}
		})
	}
}

// configuredSheet opens the sheet of b, a level 90 Marauder, on the real dataset of
// minionVersion with the config set gives, and switches it to b with the character's items.
func configuredSheet(t *testing.T, b *build.Build, set func(*config.Values) error, items ...poe1.Item) *poe1.Sheet {
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
	sw, err := poe1.SwitchOf(c, cfg, items, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	sh := open(t, r)
	if err := sh.Switch(&sw); err != nil {
		t.Fatal(err)
	}
	return sh
}
