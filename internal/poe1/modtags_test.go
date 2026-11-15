package poe1_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
)

// A line is a Life Modifier when a mod with the life tag gives all its stats and no stat the
// item lacks. So an armour line is one only when the item also has the life line of the hybrid
// armour and life mod, and a line of a two-stat mod counts by that mod's tags with the other
// stat on the item.
func TestLifeModifiers(t *testing.T) {
	t.Parallel()
	stats := []dataset.Stat{
		{ID: "base_maximum_life"}, {ID: "local_base_physical_damage_reduction_rating"},
		{ID: "minion_maximum_life_+%"}, {ID: "life_leech_from_attack_permyriad"}, {ID: "attack_speed_+%"},
	}
	mod := func(id string, tags []string, stats ...uint32) dataset.Mod {
		m := dataset.Mod{ID: id, Tags: tags}
		for _, s := range stats {
			m.Stats = append(m.Stats, dataset.ModStat{Stat: s})
		}
		return m
	}
	tags := poe1.NewModTags([]dataset.Mod{
		mod("IncreasedLife1", []string{"resource", "life"}, 0),
		mod("LocalArmour1", []string{"defences", "armour"}, 1),
		mod("LocalArmourAndLife1", []string{"defences", "resource", "life", "armour"}, 1, 0),
		mod("MinionLife1", []string{"minion", "resource", "life"}, 2),
		mod("MinionLifeNoTag", []string{"minion"}, 2),
		mod("LeechAndSpeed1", []string{"resource", "life", "attack", "speed"}, 3, 4),
	}, stats)
	line := func(ids ...string) build.ModLine {
		l := build.ModLine{Kind: build.Explicit}
		for _, id := range ids {
			l.Stats = append(l.Stats, build.StatValue{Stat: id, Value: 1})
		}
		return l
	}
	for _, tc := range []struct {
		name  string
		lines []build.ModLine
		want  bool
	}{
		{"maximum life", []build.ModLine{line("base_maximum_life")}, true},
		{"armour", []build.ModLine{line("local_base_physical_damage_reduction_rating")}, false},
		{"tagged by one of its mods", []build.ModLine{line("minion_maximum_life_+%")}, true},
		{"one line of a two-stat mod", []build.ModLine{line("attack_speed_+%"), line("life_leech_from_attack_permyriad")}, true},
		{"one line of a two-stat mod, the other line missing", []build.ModLine{line("attack_speed_+%")}, false},
		{"unread line", []build.ModLine{{Kind: build.Explicit, Text: "?"}}, false},
		{"no lines", nil, false},
		{"armour then life", []build.ModLine{line("local_base_physical_damage_reduction_rating"), line("base_maximum_life")}, true},
	} {
		if got := tags.LifeModifiers(&build.Item{Mods: tc.lines}); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}
