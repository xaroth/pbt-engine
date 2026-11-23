package simulator_test

import (
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/output"
	"github.com/xaroth/pbt-engine/poe1"
	"github.com/xaroth/pbt-engine/simulator"
)

const guardianSword = `Rarity: RARE
Gale Edge
Jewelled Foil
Item Level: 85
Implicits: 0
Adds 10 to 20 Physical Damage
10% increased Attack Speed
`

// An Animated Guardian at gem level 20 attacks with the sword its group gives it, read from the
// guardian skill's result while Fireball is the main skill. On 3.29 a Jewelled Foil deals 32 to
// 60 physical damage at 1.6 attacks per second with a 5.5% critical strike chance; its local mods
// make that 42 to 80 at 1.76. The gem's 38% more minion damage gives hits of round(42 x 1.38) =
// 58 to round(80 x 1.38) = 110, 84 on average; critical strikes at the minion's 130% multiplier
// add 5.5% x 30%: 85.386. Minion attacks always hit, and the enemy has no armour.
func TestGuardianAttacksWithItsSword(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	sword, warnings, err := s.Data().Item(guardianSword)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("read the sword: %v, %v", warnings, err)
	}
	b := marauderBuild(90)
	b.Skills.Groups = []build.SkillGroup{
		{Gems: []build.Gem{{ID: "Metadata/Items/Gems/SkillGemFireball", Level: 20}}},
		{Gems: []build.Gem{{ID: "Metadata/Items/Gems/SkillGemAnimateArmour", Level: 20}}, MinionItems: build.Items{build.SlotWeapon: sword}},
	}
	st := simulate(t, s, b, simulator.EverySkill(), simulator.Set(poe1.EnemyArmour, 0))
	guardian := st.Skill(simulator.SkillRef{Group: 1})
	for k, want := range map[output.Key]float64{
		poe1.MinionAverageHit: 85.386, poe1.MinionHitChance: 100, poe1.MinionHitsPerSecond: 1.76, poe1.MinionHitDPS: 85.386 * 1.76,
	} {
		if got, ok := guardian.Get(k); !ok || math.Abs(got-want) > 1e-9*want {
			t.Errorf("%s %v (%v), want %v", k, got, ok, want)
		}
	}
	if _, ok := st.Get(poe1.MinionAverageHit); ok {
		t.Error("the main skill, Fireball, has a minion hit")
	}
}

const chainsOfCommand = `Rarity: UNIQUE
Chains of Command
Saintly Chainmail
Implicits: 0
Trigger Level 20 Animate Guardian's Weapon when Animated Guardian Kills an Enemy
`

// guardiansWeaponBuild is a level 90 Marauder wearing Chains of Command, with lines added to
// it, whose first group is an Animate Guardian at gem level 20 and second the Animate Guardian's
// Weapon the body armour grants; both minions use guardianSword.
func guardiansWeaponBuild(t *testing.T, s *simulator.Sim, lines string) *build.Build {
	t.Helper()
	sword, warnings, err := s.Data().Item(guardianSword)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("read the sword: %v, %v", warnings, err)
	}
	body, warnings, err := s.Data().Item(chainsOfCommand + lines)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("read the body armour: %v, %v", warnings, err)
	}
	b := marauderBuild(90)
	b.Items = build.Items{build.SlotBodyArmour: body}
	b.Skills.Groups = []build.SkillGroup{
		{Gems: []build.Gem{{ID: "Metadata/Items/Gems/SkillGemAnimateArmour", Level: 20}}, MinionItems: build.Items{build.SlotWeapon: sword}},
		{
			Source: &build.SkillSource{Item: build.SlotBodyArmour}, Gems: []build.Gem{{Skill: "AnimateGuardiansWeapon", Level: 20}},
			MinionItems: build.Items{build.SlotWeapon: sword},
		},
	}
	return b
}

// The Animated Weapon that Animate Guardian's Weapon summons, though the skill is triggered,
// attacks with the sword of its group's gear: 42 to 80 physical damage at 1.76 attacks per
// second (TestGuardianAttacksWithItsSword), plus the skill's 126 to 184 added physical damage
// at level 20, 216 on average. The skill has no damage multiplier at level 20 in the 3.29 data.
// Critical strikes at 5.5% and the minion's 130% multiplier add 5.5% x 30%: 219.564.
func TestGuardiansWeaponAttacksWithTheSword(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	st := simulate(t, s, guardiansWeaponBuild(t, s, ""), simulator.EverySkill(), simulator.Set(poe1.EnemyArmour, 0))
	weapon := st.Skill(simulator.SkillRef{Group: 1})
	for k, want := range map[output.Key]float64{poe1.MinionAverageHit: 219.564, poe1.MinionHitDPS: 219.564 * 1.76} {
		if got, ok := weapon.Get(k); !ok || math.Abs(got-want) > 1e-9*want {
			t.Errorf("%s %v (%v), want %v", k, got, ok, want)
		}
	}
}

// "Animated Guardian deals 5% increased Damage per Animated Weapon" counts the 14 Animated
// Weapons the level 20 skill allows: 70% increased damage for the guardian, none for the
// weapons. The guardian's sword hits for 42 to 80 with the gem's 38% more; 70% increased makes
// that round(42 x 1.38 x 1.7) = 99 to round(80 x 1.38 x 1.7) = 188, 143.5 on average, and
// critical strikes add 5.5% x 30%: 145.86775. The guardian is the main skill, so the weapon
// skill is not on the sheet and still sets the number allowed.
func TestGuardianDamagePerAnimatedWeapon(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	b := guardiansWeaponBuild(t, s, "Animated Guardian deals 5% increased Damage per Animated Weapon\n")
	st := simulate(t, s, b, simulator.Set(poe1.EnemyArmour, 0))
	if got, want := get(t, st, poe1.MinionAverageHit), 145.86775; math.Abs(got-want) > 1e-9*want {
		t.Errorf("guardian average hit %v, want %v", got, want)
	}
	every := simulate(t, s, b, simulator.EverySkill(), simulator.Set(poe1.EnemyArmour, 0))
	if got, ok := every.Skill(simulator.SkillRef{Group: 1}).Get(poe1.MinionAverageHit); !ok || math.Abs(got-219.564) > 1e-9 {
		t.Errorf("Animated Weapon average hit %v (%v), want 219.564", got, ok)
	}
}

// A group counted as 20 Animated Weapons where an Animate Weapon gem at level 21 allows 15 is
// warned about once.
func TestAnimatedWeaponCountAboveLimit(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	b := guardiansWeaponBuild(t, s, "")
	b.Skills.Groups[1].Count = 20
	b.Skills.Groups = append(b.Skills.Groups, build.SkillGroup{Gems: []build.Gem{{ID: "Metadata/Items/Gems/SkillGemAnimateWeapon", Level: 21}}})
	st := simulate(t, s, b, simulator.EverySkill())
	var got []simulator.Warning
	for _, w := range st.Warnings() {
		if w.Code == simulator.MinionCountAboveLimit {
			got = append(got, w)
		}
	}
	want := "skill group 2: count 20 is above the 15 Animated Weapons allowed; the count is used"
	if len(got) != 1 || got[0].Subject+": "+got[0].Message != want {
		t.Errorf("warnings %v, want one: %s", got, want)
	}
}
