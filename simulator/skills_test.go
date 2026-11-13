package simulator_test

import (
	"encoding/json"
	"math"
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/poe1"
	"github.com/xaroth/pbt-engine/simulator"
)

// Fireball with Added Fire Damage at 20% quality, whose quality gives 10% increased Fire
// Damage to the skill it supports: the main skill's fire part takes it, its cold part does not,
// and the sheet without the skill has no skill outputs.
func TestMainSkillOutputs(t *testing.T) {
	t.Parallel()
	s := realSim(t)
	bare := marauderBuild(90)
	b := bare.Clone()
	b.Skills.AddGroup(build.SkillGroup{Gems: []build.Gem{
		{ID: "Metadata/Items/Gems/SkillGemFireball", Level: 20},
		{ID: "Metadata/Items/Gems/SupportGemAddedFireDamage", Level: 20, Quality: 20, Support: true},
	}})
	st := simulate(t, s, b)
	sk, ok := st.MainSkill()
	if !ok || sk.ID != "Metadata/Items/Gems/SkillGemFireball" || !slices.Contains(sk.Types, "Spell") {
		t.Fatalf("main skill %+v, %v", sk, ok)
	}
	if got := get(t, st, poe1.FireDamageModifier); math.Abs(got-110) > 1e-9 {
		t.Errorf("fire damage modifier %v, want 110", got)
	}
	if got := get(t, st, poe1.ColdDamageModifier); got != 100 {
		t.Errorf("cold damage modifier %v, want 100", got)
	}
	if _, ok := simulate(t, s, bare).Get(poe1.FireDamageModifier); ok {
		t.Error("a build without skills has a fire damage modifier")
	}
	data, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	validate(t, "Stats", data)
	ex, err := st.Explain(poe1.FireDamageModifier, simulator.Proportional)
	if err != nil {
		t.Fatal(err)
	}
	if ex.Skill == nil || ex.Skill.Group != 0 || ex.Skill.Gem != 0 {
		t.Errorf("explanation skill %+v, want group 0 gem 0", ex.Skill)
	}
	if !slices.ContainsFunc(ex.Lines, func(l simulator.Line) bool { return l.Value == 10 && l.Source.Bundle != nil }) {
		t.Errorf("lines %+v, want the support's 10", ex.Lines)
	}
	data, err = json.Marshal(ex)
	if err != nil {
		t.Fatal(err)
	}
	validate(t, "Explanation", data)
}
