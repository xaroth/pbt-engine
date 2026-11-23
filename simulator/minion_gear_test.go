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
