package poe1_test

import (
	"math"
	"testing"

	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

func skillValue(t *testing.T, sh *poe1.Sheet, i int, damage ...poe1.DamageType) float64 {
	t.Helper()
	v, err := sh.SkillValue(i, "damage", damage...)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func setSkills(t *testing.T, sh *poe1.Sheet, skills ...*resolve.Skill) {
	t.Helper()
	if err := sh.SetSkills(skills); err != nil {
		t.Fatal(err)
	}
}

func skill(group int, types ...string) *resolve.Skill {
	return &resolve.Skill{Source: resolve.GemSource{Group: group, ID: "gem"}, Types: types}
}

func TestSpellDamageReachesSpellsOnly(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global, resolve.StatValue{Stat: "spell_damage_+%", Value: 30})))
	setSkills(t, sh, skill(0, "Spell", "Fire"), skill(1, "Attack", "Melee"))
	if got := skillValue(t, sh, 0, poe1.Fire); got != 130 {
		t.Errorf("spell: got %v, want 130", got)
	}
	if got := skillValue(t, sh, 1, poe1.Physical); got != 100 {
		t.Errorf("attack: got %v, want 100", got)
	}
	if got := value(t, sh, "damage"); got != 100 {
		t.Errorf("sheet: got %v, want 100, outside any skill", got)
	}
}

// A fire mod reaches fire damage, of a skill tagged otherwise too, and damage converted from
// fire; not cold damage of a fire spell.
func TestFireDamageReachesFireOnly(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved(bundle(resolve.NodeSource{ID: 1}, resolve.Global,
		resolve.StatValue{Stat: "fire_damage_+%", Value: 20}, resolve.StatValue{Stat: "elemental_damage_+%", Value: 5})))
	setSkills(t, sh, skill(0, "Spell", "Fire"))
	for _, c := range []struct {
		damage []poe1.DamageType
		want   float64
	}{
		{[]poe1.DamageType{poe1.Fire}, 125},
		{[]poe1.DamageType{poe1.Cold}, 105},
		{[]poe1.DamageType{poe1.Physical}, 100},
		{[]poe1.DamageType{poe1.Cold, poe1.Fire}, 125},
		{nil, 100},
	} {
		if got := skillValue(t, sh, 0, c.damage...); got != c.want {
			t.Errorf("damage %v: got %v, want %v", c.damage, got, c.want)
		}
	}
}

// A skill's own stats reach that skill only, and switch with it.
func TestSkillStatsStayOnTheSkill(t *testing.T) {
	t.Parallel()
	sh := open(t, resolved())
	a := skill(0, "Spell")
	a.StatSets = []resolve.SkillStatSet{{Stats: []resolve.StatValue{{Stat: "damage_+%", Value: 10}}}}
	a.Supports = []resolve.Support{{Source: resolve.GemSource{Group: 0, Gem: 1, ID: "support"}, Stats: []resolve.StatValue{{Stat: "spell_damage_+%", Value: 40}}}}
	b := skill(1, "Spell")
	setSkills(t, sh, a, b)
	if got := skillValue(t, sh, 0); got != 150 {
		t.Errorf("own skill: got %v, want 150", got)
	}
	if got := skillValue(t, sh, 1); got != 100 {
		t.Errorf("other skill: got %v, want 100", got)
	}
	setSkills(t, sh, b)
	if got := skillValue(t, sh, 0); got != 100 {
		t.Errorf("skill set away, its slot: got %v, want 100", got)
	}
	setSkills(t, sh, a)
	if got := skillValue(t, sh, 0); got != 150 {
		t.Errorf("skill set back: got %v, want 150", got)
	}
}

// twoSetSpell is a made-up spell that hits for 100 fire damage with its primary stat set, and
// whose second set adds 50% increased fire damage and 10% critical strike chance.
func twoSetSpell(set int) *resolve.Skill {
	sk := skill(0, "Spell", "Damage", "Fire")
	sk.CastTime = 1000
	sk.StatSets = []resolve.SkillStatSet{
		{ID: "primary", Stats: []resolve.StatValue{
			{Stat: "spell_minimum_base_fire_damage", Value: 100}, {Stat: "spell_maximum_base_fire_damage", Value: 100},
		}},
		{ID: "second", SpellCritChance: 1000, Stats: []resolve.StatValue{{Stat: "fire_damage_+%", Value: 50}}},
	}
	sk.StatSet = set
	return sk
}

// The gem's stat set choice picks the set the hit uses; the second set's stats come on top of
// the primary set's. Against the default enemy's 50% fire resistance.
func TestStatSetChoice(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		set        int
		fire, crit float64
	}{{0, 50, 0}, {1, 75, 10}} {
		sh := open(t, resolved())
		setSkills(t, sh, twoSetSpell(c.set))
		if got := outputOf(t, sh, poe1.FireMinimum); got != c.fire {
			t.Errorf("set %d: fire minimum %v, want %v", c.set, got, c.fire)
		}
		if got := outputOf(t, sh, poe1.CriticalStrikeChance); got != c.crit {
			t.Errorf("set %d: critical strike chance %v, want %v", c.set, got, c.crit)
		}
	}
}

// A stat per stage scales with the stages the gem says: Bladefall of Volleys' 10% more damage
// per stage is 30% more at 3 stages, and nothing without stages.
func TestStagesScaleStatsPerStage(t *testing.T) {
	t.Parallel()
	for stages, want := range map[int]float64{0: 100, 3: 130} {
		sh := open(t, resolved())
		sk := skill(0, "Spell", "Damage", "Fire")
		sk.StatSets = []resolve.SkillStatSet{{Stats: []resolve.StatValue{{Stat: "bladefall_damage_per_stage_+%_final", Value: 10}}}}
		sk.Stages = stages
		setSkills(t, sh, sk)
		if got := skillValue(t, sh, 0, poe1.Fire); math.Abs(got-want) > 1e-9 {
			t.Errorf("%d stages: damage %v, want %v", stages, got, want)
		}
	}
}
