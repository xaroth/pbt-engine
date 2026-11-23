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
	spiritShield    = "Metadata/Items/Armours/Shields/ShieldInt1"
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

// The guardian's template gives it 10% more attack speed while dual wielding (Animated
// Guardian, PoB's Data/Minions.lua): two foils attack 10% faster than one.
func TestGuardianDualWieldSpeed(t *testing.T) {
	t.Parallel()
	foil := build.Item{Name: "Foil", Base: jewelledFoil}
	rate := func(gear build.Items) float64 {
		b := guardianBuild(gear)
		b.Skills.Groups = b.Skills.Groups[1:]
		got, err := gearSheet(t, &b).Output(poe1.MinionHitsPerSecond)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	one := rate(build.Items{build.SlotWeapon: foil})
	two := rate(build.Items{build.SlotWeapon: foil, build.SlotOffhand: foil})
	if want := one * 1.1; math.Abs(two-want) > 1e-9*want {
		t.Errorf("%v hits per second dual wielding, want %v (10%% more than %v)", two, want, one)
	}
}

// characterLife is a body armour that brings the character of guardianBuild to want maximum
// life.
func characterLife(t *testing.T, b *build.Build, want float64) poe1.Item {
	t.Helper()
	have := value(t, gearSheet(t, b), "maximum_life")
	return poe1.Item{Slot: build.SlotBodyArmour, Name: "Life", Key: "life", Stats: []resolve.StatValue{{Stat: "base_maximum_life", Value: want - have}}}
}

// The Grey Wind's "Attacks with this Weapon have Added Fire Damage equal to 12% of Player's
// Maximum Life" on the guardian's weapon reads the character's life: 600 fire damage at 5,000.
func TestGuardianWeaponFireOfCharacterLife(t *testing.T) {
	t.Parallel()
	foil := build.Item{Rarity: build.Unique, Name: "The Grey Wind", Base: jewelledFoil, Mods: []build.ModLine{
		{Stats: []build.StatValue{{Stat: "local_fire_damage_from_life_%", Value: 12}}},
	}}
	b := guardianBuild(build.Items{build.SlotWeapon: foil})
	b.Skills.Groups = b.Skills.Groups[1:]
	sh := gearSheet(t, &b, characterLife(t, &b, 5000))
	if got := value(t, sh, "maximum_life"); got != 5000 {
		t.Fatalf("character life %v, want 5000", got)
	}
	for _, stat := range []string{"base_minimum_damage", "base_maximum_damage"} {
		if got, err := sh.MinionSkillValue(0, stat, poe1.Fire); err != nil || got != 600 {
			t.Errorf("guardian %s %v (%v), want 600", stat, got, err)
		}
	}
}

// Rathpith Globe's "4% increased Spell Damage per 100 Player Maximum Life" on the guardian's
// shield reads the character's life: 200% increased spell damage at 5,000, twice what a plain
// "100% increased Spell Damage" gives.
func TestGuardianSpellDamagePerCharacterLife(t *testing.T) {
	t.Parallel()
	damage := func(stats ...build.StatValue) float64 {
		shield := build.Item{Rarity: build.Unique, Name: "Rathpith Globe", Base: spiritShield, Mods: []build.ModLine{{Stats: stats}}}
		b := guardianBuild(build.Items{build.SlotOffhand: shield})
		b.Skills.Groups = b.Skills.Groups[1:]
		got, err := gearSheet(t, &b, characterLife(t, &b, 5000)).MinionValue(0, "damage", poe1.Fire)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	bare := damage()
	plain := damage(build.StatValue{Stat: "spell_damage_+%", Value: 100})
	perLife := damage(build.StatValue{Stat: "spell_damage_+%_per_100_max_life", Value: 4})
	if want := bare + 2*(plain-bare); math.Abs(perLife-want) > 1e-9*want {
		t.Errorf("spell damage %v, want %v (bare %v, 100%% increased %v)", perLife, want, bare, plain)
	}
}
