package poe1_test

import (
	"maps"
	"math"
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

const (
	animateGuardian = "Metadata/Items/Gems/SkillGemAnimateArmour"
	royalBurgonet   = "Metadata/Items/Armours/Helmets/HelmetStr10"
	jewelledFoil    = "Metadata/Items/Weapons/OneHandWeapons/OneHandThrustingSwords/Rapier21"
)

// guardianBuild is a level 90 Marauder whose second group's Animate Guardian, at gem level 20,
// uses gear; the first group's Fireball is the main skill.
func guardianBuild(gear build.Items) build.Build {
	return build.Build{
		Format: 1, Game: "poe1", Version: minionVersion, Class: "Metadata/Characters/Str/Str", Level: 90,
		Skills: build.Skills{Groups: []build.SkillGroup{
			{Gems: []build.Gem{{ID: "Metadata/Items/Gems/SkillGemFireball", Level: 20}}},
			{Gems: []build.Gem{{ID: animateGuardian, Level: 20}}, MinionItems: gear},
		}},
	}
}

// gearSwitch returns what a sheet is switched to for b with the character's items, its
// minions' gear compiled from the real dataset.
func gearSwitch(t *testing.T, b *build.Build, items ...poe1.Item) (*build.Checked, poe1.Switch) {
	t.Helper()
	s := realdata.Open(t, minionVersion)
	c, _, err := build.Check(b, s)
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.Items()
	if err != nil {
		t.Fatal(err)
	}
	gear := poe1.MinionGear{}
	for i, g := range c.Build().Skills.Groups {
		for _, slot := range slices.Sorted(maps.Keys(g.MinionItems)) {
			it := g.MinionItems[slot]
			base := data.Bases[slices.IndexFunc(data.Bases, func(b dataset.ItemBase) bool { return b.ID == it.Base })]
			ci := poe1.CompileItem(&it, &base)
			gear[i] = append(gear[i], poe1.Item{Slot: slot, Name: it.Name, Key: it.Base, Stats: ci.Stats, Weapon: ci.Weapon, Armour: ci.Armour})
		}
	}
	sw, err := poe1.SwitchOf(c, poe1.DefaultConfig(), items, gear, true)
	if err != nil {
		t.Fatal(err)
	}
	return c, sw
}

func gearSheet(t *testing.T, b *build.Build, items ...poe1.Item) *poe1.Sheet {
	t.Helper()
	c, sw := gearSwitch(t, b, items...)
	r, err := resolve.Resolve(c)
	if err != nil {
		t.Fatal(err)
	}
	sh := open(t, r)
	if err := sh.Switch(&sw); err != nil {
		t.Fatal(err)
	}
	return sh
}

func helmet(life int32) build.Item {
	return build.Item{Rarity: build.Rare, Name: "Grim Visor", Base: royalBurgonet, Mods: []build.ModLine{
		{Stats: []build.StatValue{{Stat: "base_maximum_life", Value: life}}},
	}}
}

// "+50 to maximum Life" on the guardian's helmet is the guardian's, and the character's life
// does not change. At gem level 20 the guardian has 7,390 life and 38% more: 7,390 x 1.38 =
// 10,198 bare, (7,390 + 50) x 1.38 = 10,267 with the helmet.
func TestGuardianHelmetLife(t *testing.T) {
	t.Parallel()
	bare, worn := guardianBuild(nil), guardianBuild(build.Items{build.SlotHelmet: helmet(50)})
	before, after := gearSheet(t, &bare), gearSheet(t, &worn)
	for _, c := range []struct {
		sh   *poe1.Sheet
		want float64
	}{{before, 10198}, {after, 10267}} {
		if got, err := c.sh.MinionValue(1, "maximum_life"); err != nil || got != c.want {
			t.Errorf("guardian life %v (%v), want %v", got, err, c.want)
		}
	}
	if b, a := value(t, before, "maximum_life"), value(t, after, "maximum_life"); a != b {
		t.Errorf("character life %v with the guardian's helmet, %v without", a, b)
	}
}

// Switching a sheet again to a build whose guardian uses gear adds no sources: each switch
// compiles the items anew, and the sheet knows the minion by its items' keys.
func TestGuardianGearSwitchAddsOnce(t *testing.T) {
	t.Parallel()
	b := guardianBuild(build.Items{build.SlotHelmet: helmet(50), build.SlotWeapon: {Name: "Foil", Base: jewelledFoil}})
	sh := gearSheet(t, &b)
	want := sh.SourceCount()
	for range 3 {
		_, sw := gearSwitch(t, &b)
		if err := sh.Switch(&sw); err != nil {
			t.Fatal(err)
		}
		if got := sh.SourceCount(); got != want {
			t.Fatalf("%d sources after switching again, want %d", got, want)
		}
	}
}

// A weapon's alternate quality and "Deal no Physical or Elemental Damage" on the guardian's gear
// reach the guardian's hit as they reach the character's.
func TestGuardianWeaponAlternateQuality(t *testing.T) {
	t.Parallel()
	hit := func(stats ...build.StatValue) float64 {
		foil := build.Item{Rarity: build.Rare, Name: "Foil", Base: jewelledFoil, Quality: 20, Mods: []build.ModLine{{Stats: stats}}}
		b := guardianBuild(build.Items{build.SlotWeapon: foil})
		b.Skills.Groups = b.Skills.Groups[1:]
		got, err := gearSheet(t, &b).Output(poe1.MinionAverageHit)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	plain := hit()
	noQuality := hit(build.StatValue{Stat: "local_quality_does_not_increase_physical_damage", Value: 1})
	none := hit(build.StatValue{Stat: "deal_no_elemental_physical_damage", Value: 1})
	t.Logf("plain %v, no quality %v, none %v", plain, noQuality, none)
	if !(noQuality < plain) || none != 0 {
		t.Errorf("hits %v, %v and %v", plain, noQuality, none)
	}
}

// A minion's attributes give it the bonuses a character's give it (PoB runs the same attribute
// bonuses for the minion): 100 strength from the guardian's helmet is 20% increased melee
// physical damage.
func TestGuardianStrengthBonus(t *testing.T) {
	t.Parallel()
	strong := guardianBuild(build.Items{build.SlotHelmet: {Rarity: build.Rare, Name: "Grim Visor", Base: royalBurgonet, Mods: []build.ModLine{
		{Stats: []build.StatValue{{Stat: "additional_strength", Value: 100}}},
	}}})
	bare := guardianBuild(nil)
	var got [2]float64
	for i, b := range []*build.Build{&bare, &strong} {
		var err error
		if got[i], err = gearSheet(t, b).MinionSkillValue(1, "damage", poe1.Physical); err != nil {
			t.Fatal(err)
		}
	}
	if want := got[0] * 1.2; math.Abs(got[1]-want) > 1e-9*want {
		t.Errorf("physical damage %v with 100 strength, want %v (20%% more than %v)", got[1], want, got[0])
	}
}
