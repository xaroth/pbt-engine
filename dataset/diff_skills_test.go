package dataset_test

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/dataset"
)

func TestCompareSkills(t *testing.T) {
	t.Parallel()
	a, b := withSkills(), renumberedSkills()
	sk := b.Skills
	sk.Types = append(sk.Types, "Minion")
	sk.Gems = sk.Gems[:2]
	sk.Gems[1].Colour = dataset.Red
	sk.GrantedEffects[0].Levels[1].Costs[0].Amount = 24
	sk.GrantedEffects[4].AddedTypes = []uint16{typeSpell}
	sk.StatSets[0].Levels[1].FloatStats[1].Resolved = 906
	sk.StatSets[2].Levels[0].Flags = nil
	sk.StatSets = append(sk.StatSets, dataset.StatSet{ID: "ZNew"})
	sk.ActiveSkills[0].Types = sk.ActiveSkills[0].Types[:1]
	sk.ActiveSkills[2].Mine = 1
	sk.ActiveSkills = slices.Delete(sk.ActiveSkills, 3, 4)
	sk.GrantedEffects[5].ActiveSkill = ""
	b.Texts = nil
	got := compare(t, a, b).Skills
	want := dataset.SkillsDiff{
		TypesAdded:            []string{"Minion"},
		GemsRemoved:           []string{"Metadata/Items/Gems/SupportGemSpellTotem"},
		GemsChanged:           []string{"Metadata/Items/Gems/SkillGemVaalFireball"},
		GrantedEffectsChanged: []string{"Fireball", "SupportSpellTotem", "VaalFireball"},
		StatSetsAdded:         []string{"ZNew"},
		StatSetsChanged:       []string{"Fireball", "SupportSpellTotem"},
		ActiveSkillsRemoved:   []string{"vaal_fireball"},
		ActiveSkillsChanged:   []string{"fireball", "monster_slam"},
	}
	if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Compare skills mismatch (-want +got):\n%s", diff)
	}
}

func TestCompareSkillsGroupAdded(t *testing.T) {
	t.Parallel()
	a, b := sample(), withSkills()
	a.Stats = b.Stats
	got := compare(t, a, b).Skills
	if len(got.GemsAdded) != 3 || len(got.ActiveSkillsAdded) != 4 || len(got.TypesAdded) != 4 || len(got.GemsChanged) != 0 {
		t.Errorf("Compare skills: got %+v, want every gem, active skill and type added", got)
	}
	same := compare(t, withSkills(), renumberedSkills()).Skills
	if diff := cmp.Diff(dataset.SkillsDiff{}, same, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Compare renumbered skills: want no changes (-want +got):\n%s", diff)
	}
}
