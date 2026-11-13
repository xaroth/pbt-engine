package poe1_test

import (
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
