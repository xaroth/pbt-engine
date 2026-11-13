package poe1_test

import (
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/realdata"
)

// TestGemBonusesInDataset checks that each gem bonus names a stat and gem tags the dataset has.
func TestGemBonusesInDataset(t *testing.T) {
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
	bonuses := poe1.GemBonuses()
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
