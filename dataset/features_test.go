package dataset_test

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
)

// TestFeaturesOfLeagues reads the features of the pinned dataset of every league: each league
// keeps the bloodlines of the one before and adds its own, keystone and ascendancy tattoos come
// with 3.28, and Blightcore's proxy nodes with 3.30.
func TestFeaturesOfLeagues(t *testing.T) {
	t.Parallel()
	small := []dataset.OverrideTarget{
		dataset.SmallStrength, dataset.SmallDexterity, dataset.SmallIntelligence, dataset.SmallAttribute,
		dataset.StrengthNotable, dataset.DexterityNotable, dataset.IntelligenceNotable, dataset.MasteryTarget,
	}
	all := append(slices.Clone(small[:7]), dataset.KeystoneTarget, dataset.MasteryTarget, dataset.AscendancyNotable)
	radii := []string{"Small", "Medium", "Large", "VeryLarge", "Massive"}
	testCases := []struct {
		version    string
		added      []string
		targets    []dataset.OverrideTarget
		proxyKinds []dataset.ProxyKind
		jewels     dataset.JewelFeatures
	}{
		{
			realdata.Oldest,
			[]string{
				"Warden", "Warlock", "Primalist", "Trialmaster", "Oshabi", "KingInTheMists", "Catarina", "Aul",
				"Lycia", "Olroth", "Farrul", "Delirious", "Breachlord",
			},
			small,
			[]dataset.ProxyKind{dataset.ClusterProxy},
			dataset.JewelFeatures{Sockets: 60, ClusterSockets: 42, Radii: radii, TimelessVersions: 6},
		},
		{
			// The pinned 3.28 dataset was written before cluster sockets, radii and timeless
			// versions were extracted.
			realdata.Previous,
			[]string{"Necromantic"},
			all,
			[]dataset.ProxyKind{dataset.ClusterProxy},
			dataset.JewelFeatures{Sockets: 60},
		},
		{
			realdata.Current,
			[]string{"Brinerot", "Abyssal"},
			all,
			[]dataset.ProxyKind{dataset.ClusterProxy},
			dataset.JewelFeatures{Sockets: 60, ClusterSockets: 42, Radii: radii, TimelessVersions: 7},
		},
		{
			realdata.InGame,
			nil,
			all,
			[]dataset.ProxyKind{dataset.ClusterProxy, dataset.TreeProxy},
			dataset.JewelFeatures{Sockets: 60, ClusterSockets: 42, Radii: radii, TimelessVersions: 7},
		},
	}
	var bloodlines []string
	for _, tc := range testCases {
		bloodlines = append(bloodlines, tc.added...)
		want := slices.Clone(bloodlines)
		t.Run(tc.version, func(t *testing.T) {
			t.Parallel()
			f, err := realdata.Open(t, tc.version).Features("standard")
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(want, f.Bloodlines); diff != "" {
				t.Errorf("bloodlines (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff([]dataset.OverrideKind{dataset.Tattoo, dataset.Runegraft}, f.OverrideKinds); diff != "" {
				t.Errorf("override kinds (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.targets, f.OverrideTargets); diff != "" {
				t.Errorf("override targets (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.proxyKinds, f.ProxyKinds); diff != "" {
				t.Errorf("proxy kinds (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.jewels, f.Jewels); diff != "" {
				t.Errorf("jewels (-want +got):\n%s", diff)
			}
			if len(f.ItemClasses) != 32 || !slices.Contains(f.ItemClasses, "AbyssJewel") {
				t.Errorf("item classes: got %d %v, want the 32 with AbyssJewel", len(f.ItemClasses), f.ItemClasses)
			}
			if diff := cmp.Diff([]string{"enable_ring_slot_3"}, f.SlotStats); diff != "" {
				t.Errorf("slot stats (-want +got):\n%s", diff)
			}
		})
	}
}

// TestFeaturesOfEmptyContent checks that a tree without league content and a dataset without
// items give empty features.
func TestFeaturesOfEmptyContent(t *testing.T) {
	t.Parallel()
	s, err := dataset.Open(written(t, sample()))
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Features("ruthless")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(dataset.Features{}, f, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("ruthless (-want +got):\n%s", diff)
	}
	f, err = s.Features("standard")
	if err != nil {
		t.Fatal(err)
	}
	if f.SlotStats != nil || f.ItemClasses != nil || !slices.Equal(f.Bloodlines, []string{"Bloodline"}) {
		t.Errorf("standard: got slot stats %v, classes %v, bloodlines %v", f.SlotStats, f.ItemClasses, f.Bloodlines)
	}
}

// TestFeaturesSlotStats checks that a slot-opening stat is listed only when a node grants it.
func TestFeaturesSlotStats(t *testing.T) {
	t.Parallel()
	const ring3 = "enable_ring_slot_3"
	d := withItems()
	d.Stats = append(d.Stats, dataset.Stat{ID: ring3})
	for _, granted := range []bool{false, true} {
		if granted {
			n := &d.Trees[0].Nodes[1]
			n.Stats = append(n.Stats, dataset.StatValue{Stat: uint32(len(d.Stats) - 1), Value: 1})
		}
		s, err := dataset.Open(written(t, d))
		if err != nil {
			t.Fatal(err)
		}
		f, err := s.Features("standard")
		if err != nil {
			t.Fatal(err)
		}
		if got := slices.Contains(f.SlotStats, ring3); got != granted {
			t.Errorf("granted %t: got slot stats %v", granted, f.SlotStats)
		}
	}
}
