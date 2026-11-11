package dataset_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/dataset"
)

func TestCompareMonsters(t *testing.T) {
	t.Parallel()
	a, b := withMonsters(), reversedStats(withMonsters())
	m := b.Monsters
	m.Types[0].DamageSpread = 30
	m.Resistances = m.Resistances[1:]
	m.Types[1].Resistances = ""
	m.Varieties[0].Mods2 = nil
	m.Varieties = append(m.Varieties, dataset.MonsterVariety{ID: "Metadata/Monsters/New", Type: "RaisedZombie"})
	m.Levels[1].AllyDamage = 300.5
	m.Levels = append(m.Levels, dataset.MonsterLevel{Level: 69, Difficulty: dataset.MercilessDifficulty})
	m.BaseStats[0].Value = 76
	m.Mods[0].Stats[0].Max = 31
	m.DisplayMinions = m.DisplayMinions[:1]
	m.Totems[1].Skins = m.Totems[1].Skins[:1]
	m.SpectreOverrides[0].Spectre = zombie
	m.MinionTypes[1].ActiveCountStat = m.MinionTypes[0].ActiveCountStat
	m.EditionTotems[0].Totem = 15
	b.Texts = nil
	got := compare(t, a, b).Monsters
	want := dataset.MonstersDiff{
		TypesChanged:            []string{"RaisedZombie"},
		ResistancesRemoved:      []string{"None"},
		VarietiesAdded:          []string{"Metadata/Monsters/New"},
		VarietiesChanged:        []string{zombie},
		LevelsAdded:             []int32{69},
		LevelsChanged:           []int32{68},
		BaseStatsChanged:        []string{"number_of_zombies_allowed"},
		ModsChanged:             []string{"MonsterImplicitDamageReduction1"},
		DisplayMinionsRemoved:   []int32{2},
		TotemsChanged:           []int32{15},
		SpectreOverridesChanged: []string{statue},
		MinionTypesChanged:      []string{"GolemMinion"},
		EditionTotemsChanged:    []string{"SupportSpellTotem"},
	}
	if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Compare monsters mismatch (-want +got):\n%s", diff)
	}
}

func TestCompareMonstersGroupAdded(t *testing.T) {
	t.Parallel()
	a, b := withSkills(), withMonsters()
	a.Stats = b.Stats
	got := compare(t, a, b).Monsters
	if len(got.VarietiesAdded) != 5 || len(got.ModsAdded) != 4 || len(got.TotemsAdded) != 2 || len(got.BaseStatsChanged) != 2 || len(got.VarietiesChanged) != 0 {
		t.Errorf("Compare monsters: got %+v, want every variety, mod, totem and base stat added", got)
	}
	same := compare(t, withMonsters(), reversedStats(withMonsters())).Monsters
	if diff := cmp.Diff(dataset.MonstersDiff{}, same, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Compare renumbered monsters: want no changes (-want +got):\n%s", diff)
	}
}
