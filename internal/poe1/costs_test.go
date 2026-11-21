package poe1_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// costSkill is a skill of types costing mana and life, with its own stats.
func costSkill(mana, life int32, types []string, stats ...resolve.StatValue) *resolve.Skill {
	sk := skill(0, types...)
	sk.Costs = []dataset.Cost{{Type: "Mana", Amount: mana}, {Type: "Life", Amount: life}}
	sk.StatSets = []resolve.SkillStatSet{{Stats: stats}}
	return sk
}

// costsOf returns the mana and life cost of sk on a sheet with the tree's stats.
func costsOf(t *testing.T, sk *resolve.Skill, stats ...resolve.StatValue) (mana, life float64) {
	t.Helper()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global, stats...)))
	setSkills(t, sh, sk)
	return outputOf(t, sh, poe1.ManaCost), outputOf(t, sh, poe1.LifeCost)
}

// Path of Building's Penance Brand of 3.28 build 24: 99 base mana, Inspiration's 39% less mana
// cost and the Life and Mana mastery's 15% paid with life. Life takes 15% of the 99, rounded
// (15); mana is 99 x 0.61 rounded up (61), less 15% rounded down: PoB's 51 and 15.
func TestCostPaidWithLife(t *testing.T) {
	t.Parallel()
	sk := costSkill(99, 0, []string{"Spell"})
	sk.Supports = []resolve.Support{support("inspiration", resolve.StatValue{Stat: "support_inspiration_mana_cost_+%_final", Value: -39})}
	mana, life := costsOf(t, sk, resolve.StatValue{Stat: "base_skill_cost_life_instead_of_mana_%", Value: 15})
	if mana != 51 || life != 15 {
		t.Errorf("got %v mana and %v life, want 51 and 15", mana, life)
	}
}

// The attack variant reaches attacks only.
func TestAttackCostPaidWithLife(t *testing.T) {
	t.Parallel()
	share := resolve.StatValue{Stat: "base_attack_skill_cost_life_instead_of_mana_%", Value: 20}
	for _, c := range []struct {
		types      []string
		mana, life float64
	}{
		{[]string{"Attack"}, 80, 20},
		{[]string{"Spell"}, 100, 0},
	} {
		if mana, life := costsOf(t, costSkill(100, 0, c.types), share); mana != c.mana || life != c.life {
			t.Errorf("%v: got %v mana and %v life, want %v and %v", c.types, mana, life, c.mana, c.life)
		}
	}
}

// The Covenant's life cost of 50% of the base mana cost adds to the life cost and leaves the
// mana cost whole.
func TestCostGainedAsLife(t *testing.T) {
	t.Parallel()
	mana, life := costsOf(t, costSkill(100, 10, []string{"Spell"}), resolve.StatValue{Stat: "base_skill_gain_life_cost_%_of_mana_cost", Value: 50})
	if mana != 100 || life != 60 {
		t.Errorf("got %v mana and %v life, want 100 and 60", mana, life)
	}
}

// Whispers of Infinity: no mana or life cost.
func TestCostEnergyShieldInstead(t *testing.T) {
	t.Parallel()
	mana, life := costsOf(t, costSkill(100, 10, []string{"Spell"}), resolve.StatValue{Stat: "base_skills_cost_es_instead_of_mana_life", Value: 1})
	if mana != 0 || life != 0 {
		t.Errorf("got %v mana and %v life, want 0 and 0", mana, life)
	}
}

// Reduced Mana Cost of Attacks reaches attacks only.
func TestAttackManaCost(t *testing.T) {
	t.Parallel()
	reduced := resolve.StatValue{Stat: "attack_mana_cost_+%", Value: -20}
	for _, c := range []struct {
		types []string
		want  float64
	}{
		{[]string{"Attack"}, 80},
		{[]string{"Spell"}, 100},
	} {
		if mana, _ := costsOf(t, costSkill(100, 0, c.types), reduced); mana != c.want {
			t.Errorf("%v: got %v, want %v", c.types, mana, c.want)
		}
	}
}

// Reap at level 27 costs 53 life; Path of Building's 3.29 build 73 has it at 161 with its
// supports. 20% more per Blood Charge at the default 5 doubles it.
func TestCostPerBloodCharge(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.CharacterSource{ID: "Metadata/Characters/Int/Int"}, resolve.Global,
		resolve.StatValue{Stat: "maximum_blood_scythe_charges", Value: 5},
	)))
	setSkills(t, sh, costSkill(0, 161, []string{"Spell"}, resolve.StatValue{Stat: "blood_scythe_cost_+%_final_per_charge", Value: 20}))
	if got := outputOf(t, sh, poe1.LifeCost); got != 322 {
		t.Errorf("got %v, want 322", got)
	}
}

// Mirage Warriors summoned by General's Cry use the skill, so the character pays nothing: the
// sheet gives no cost, and the cost read in the skill's context is 0.
func TestGeneralsCryCostsNothing(t *testing.T) {
	t.Parallel()
	sk := costSkill(20, 0, []string{"Attack", "Melee"})
	sk.Supports = []resolve.Support{support("generals cry", resolve.StatValue{Stat: "triggered_by_spiritual_cry", Value: 1})}
	byKey := map[output.Key]poe1.Output{}
	for _, o := range poe1.Outputs() {
		byKey[o.Key] = o
	}
	for _, k := range []output.Key{poe1.ManaCost, poe1.LifeCost} {
		if poe1.Gives(byKey[k], sk, poe1.Hands{}) {
			t.Errorf("%s given", k)
		}
	}
	sh := open(t, resolved())
	setSkills(t, sh, sk)
	if got, err := sh.SkillValue(0, "mana_cost"); err != nil || got != 0 {
		t.Errorf("mana cost: got %v, %v, want 0", got, err)
	}
}
