package dataset_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/dataset"
)

func TestCompareBuffs(t *testing.T) {
	t.Parallel()
	a, b := withBuffs(), renumberedBuffs()
	bs := b.Buffs
	bs.Definitions[0].Stats = bs.Definitions[0].Stats[:1]
	bs.Definitions[2].MinimumStat = nil
	bs.Definitions[3].Recovery = true
	bs.Definitions = append(bs.Definitions[:4], dataset.BuffDefinition{ID: "onslaught", Category: 1})
	bs.Flasks[0].Values[0] = 2000
	bs.Tinctures[0].Cooldown = 7000
	b.Texts[0].BuffNames = nil
	got := compare(t, a, b).Buffs
	want := dataset.BuffsDiff{
		BuffsAdded:       []string{"onslaught"},
		BuffsRemoved:     []string{"withered"},
		BuffsChanged:     []string{"flask_utility_ironskin", "frenzy_charge", "player_aura_armour"},
		FlasksChanged:    []string{"Metadata/Items/Flasks/FlaskUtility5"},
		TincturesChanged: []string{"Metadata/Items/Tinctures/Tincture1"},
	}
	if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Compare buffs mismatch (-want +got):\n%s", diff)
	}
}

func TestCompareBuffsGroupAdded(t *testing.T) {
	t.Parallel()
	a, b := withItems(), withBuffs()
	a.Stats, a.Items = b.Stats, b.Items
	got := compare(t, a, b).Buffs
	want := dataset.BuffsDiff{
		BuffsAdded:     []string{"curse_temporal_chains", "flask_utility_ironskin", "frenzy_charge", "player_aura_armour", "withered"},
		FlasksAdded:    []string{"Metadata/Items/Flasks/FlaskUtility5"},
		TincturesAdded: []string{"Metadata/Items/Tinctures/Tincture1"},
	}
	if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Compare buffs mismatch (-want +got):\n%s", diff)
	}
	same := compare(t, withBuffs(), renumberedBuffs()).Buffs
	if diff := cmp.Diff(dataset.BuffsDiff{}, same, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Compare renumbered buffs: want no changes (-want +got):\n%s", diff)
	}
}
