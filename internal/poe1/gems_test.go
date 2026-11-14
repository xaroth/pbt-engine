package poe1_test

import (
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/realdata"
)

// TestGemRulesInDataset checks that each stat of the gem rules is in the dataset, and that
// each gem bonus names gem tags the dataset has.
func TestGemRulesInDataset(t *testing.T) {
	t.Parallel()
	s := realdata.Open(t, realdata.Current)
	skills, err := s.Skills()
	if err != nil {
		t.Fatal(err)
	}
	tags := map[string]bool{}
	for _, g := range skills.Gems {
		for _, e := range g.Effects {
			for _, tag := range e.Tags {
				tags[tag] = true
			}
		}
	}
	rules := poe1.GemRules()
	if has, err := s.HasStat(rules.AlwaysMatchingStat); err != nil || !has {
		t.Errorf("stat %s not in %s", rules.AlwaysMatchingStat, realdata.Current)
	}
	bonuses := rules.Bonuses
	stats := make([]string, 0, len(bonuses))
	for _, b := range bonuses {
		if has, err := s.HasStat(b.Stat); err != nil || !has {
			t.Errorf("stat %s not in %s", b.Stat, realdata.Current)
		}
		if slices.Contains(stats, b.Stat) {
			t.Errorf("stat %s listed twice", b.Stat)
		}
		stats = append(stats, b.Stat)
		for _, tag := range slices.Concat(b.Gems.Tags, b.Gems.AnyTags, b.Gems.NotTags) {
			if !tags[tag] {
				t.Errorf("%s: gem tag %s not in %s", b.Stat, tag, realdata.Current)
			}
		}
	}
}
