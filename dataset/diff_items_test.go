package dataset_test

import (
	"errors"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/dataset"
)

func TestCompareItems(t *testing.T) {
	t.Parallel()
	a, b := withItems(), withItems()
	b.Texts = nil
	b.Items.Classes = b.Items.Classes[1:]
	b.Items.Tags = append(b.Items.Tags, "ring")
	b.Items.Bases = b.Items.Bases[1:3]
	b.Items.Bases[0].Block = 25
	b.Items.Bases = append(b.Items.Bases, dataset.ItemBase{ID: "Ring", Class: "Shield"})
	b.Items.BenchCrafts[0].Classes = []string{"Shield"}
	b.Mods[0].Stats[0].RuthlessMax = 21
	b.Mods = append(b.Mods, dataset.Mod{ID: "New", Domain: 1, Generation: 2, Type: "New"})
	// Moving stat handles changes nothing; mods compare by stat id.
	b.Stats = append([]dataset.Stat{{ID: "new_stat"}}, b.Stats...)
	for _, c := range b.Characters {
		for i := range c.Stats {
			c.Stats[i].Stat++
		}
	}
	for _, q := range b.Quests {
		for i := range q.Stats {
			q.Stats[i].Stat++
		}
	}
	for _, n := range b.Trees[0].Nodes {
		for i := range n.Stats {
			n.Stats[i].Stat++
		}
		for _, bf := range n.Buffs {
			for i := range bf {
				bf[i].Stat++
			}
		}
		for _, e := range n.MasteryEffects {
			for i := range e.Stats {
				e.Stats[i].Stat++
			}
		}
	}
	for _, m := range b.Mods {
		for i := range m.Stats {
			m.Stats[i].Stat++
		}
	}

	want := dataset.ItemsDiff{
		ClassesRemoved:     []string{"Helmet"},
		TagsAdded:          []string{"ring"},
		BasesAdded:         []string{"Ring"},
		BasesRemoved:       []string{"Metadata/Items/Armours/Helmets/HelmetStr1", "Metadata/Items/Flasks/FlaskLife1"},
		BasesChanged:       []string{"Metadata/Items/Armours/Shields/ShieldStr1"},
		ModsAdded:          []string{"New"},
		ModsChanged:        []string{"IncreasedLife1"},
		BenchCraftsChanged: []string{"CraftedLife1"},
	}
	got := compare(t, a, b)
	if diff := gocmp.Diff(want, got.Items, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Compare items mismatch (-want +got):\n%s", diff)
	}
	if diff := gocmp.Diff([]string{"new_stat"}, got.StatsAdded); diff != "" {
		t.Errorf("Compare stats mismatch (-want +got):\n%s", diff)
	}

	// A dataset without the group compares as an empty one.
	got = compare(t, sample(), withItems())
	want = dataset.ItemsDiff{
		ClassesAdded: []string{"Helmet", "LifeFlask", "One Hand Sword", "Shield"},
		TagsAdded:    []string{"default", "has_attack_mod", "helmet", "life", "shield", "str_armour", "weapon"},
		BasesAdded: []string{
			"Metadata/Items/Armours/Helmets/HelmetStr1", "Metadata/Items/Armours/Shields/ShieldStr1",
			"Metadata/Items/Flasks/FlaskLife1", "Metadata/Items/Weapons/OneHandWeapons/OneHandSwords/OneHandSword1",
		},
		ModsAdded:        []string{"AccuracyImplicit", "BlockImplicit", "CraftedLife1", "IncreasedLife1"},
		BenchCraftsAdded: []string{"CraftedLife1"},
	}
	if diff := gocmp.Diff(want, got.Items, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Compare without items mismatch (-want +got):\n%s", diff)
	}
	if got := compare(t, sample(), sample()); !gocmp.Equal(dataset.ItemsDiff{}, got.Items, cmpopts.EquateEmpty()) {
		t.Errorf("Compare without items on both sides: got %+v, want empty", got.Items)
	}
}

func TestCompareModGameMode(t *testing.T) {
	t.Parallel()
	b := withItems()
	b.Mods[1].GameMode = 2
	want := dataset.ItemsDiff{ModsChanged: []string{"BlockImplicit"}}
	if diff := gocmp.Diff(want, compare(t, withItems(), b).Items, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Compare items mismatch (-want +got):\n%s", diff)
	}
}

func TestCompareBaseModDomainAndBenchCrafts(t *testing.T) {
	t.Parallel()
	b := withItems()
	b.Items.Bases[3].ModDomain = 3
	b.Items.BenchCrafts = []dataset.BenchCraft{{Mod: "IncreasedLife1", Classes: []string{"Helmet"}}}
	want := dataset.ItemsDiff{
		BasesChanged:       []string{"Metadata/Items/Flasks/FlaskLife1"},
		BenchCraftsAdded:   []string{"IncreasedLife1"},
		BenchCraftsRemoved: []string{"CraftedLife1"},
	}
	if diff := gocmp.Diff(want, compare(t, withItems(), b).Items, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Compare items mismatch (-want +got):\n%s", diff)
	}
}

func TestCompareItemsFails(t *testing.T) {
	t.Parallel()
	x, err := dataset.Open(written(t, withItems()))
	if err != nil {
		t.Fatal(err)
	}
	good := written(t, withItems())
	y, err := dataset.Open(withFile(t, good, "mods.fb", good["mods.fb"].Data[:40]))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dataset.Compare(x, y); !errors.Is(err, dataset.ErrInvalid) {
		t.Errorf("Compare with damaged mods: got %v, want ErrInvalid", err)
	}
}
