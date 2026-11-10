package poe1_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// banditRewards are the 3.29.3.3 QuestStaticRewards rows of the bandit helps.
func banditRewards() []resolve.Bundle {
	q := func(id, flag, stat string, v float64) resolve.Bundle {
		return bundle(resolve.QuestSource{ID: id, Flag: flag}, resolve.Global, resolve.StatValue{Stat: stat, Value: v})
	}
	return []resolve.Bundle{
		q("a2q1", "a2q1-QuestRewardTaken", "base_movement_velocity_+%", 8),
		q("a2q2", "a2q2-QuestRewardTaken", "base_resist_all_elements_%", 15),
		q("a2q3", "a2q3-QuestRewardTaken", "base_maximum_life", 40),
	}
}

func openWith(t *testing.T, bundles []resolve.Bundle, values config.Values) *poe1.Sheet {
	t.Helper()
	cfg, problems := config.Resolve(poe1.ConfigSpecs(), config.Layer{Source: config.FromBuild, Values: values})
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	sh, err := poe1.Open(resolved(bundles...), table(t), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return sh
}

func TestQuestRewards(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		campaign poe1.CampaignStage
		bandits  poe1.BanditChoice
		want     map[string]float64
	}{
		// level 90 life 38 + 1080 + 16
		{name: "default", want: map[string]float64{"fire_resistance": -60, "chaos_resistance": -60, "maximum_life": 1134}},
		{name: "acts 6 to 10", campaign: poe1.CampaignActs6To10, want: map[string]float64{"fire_resistance": -30, "chaos_resistance": -30}},
		{name: "acts 1 to 5", campaign: poe1.CampaignActs1To5, want: map[string]float64{"fire_resistance": 0, "chaos_resistance": 0}},
		{name: "help alira", bandits: poe1.BanditsHelpAlira, want: map[string]float64{"fire_resistance": -45, "maximum_life": 1134}},
		{name: "help oak", bandits: poe1.BanditsHelpOak, want: map[string]float64{"fire_resistance": -60, "maximum_life": 1174}},
		{name: "kill all", bandits: poe1.BanditsKillAll, want: map[string]float64{"fire_resistance": -60, "maximum_life": 1134}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var v config.Values
			if testCase.campaign != "" {
				if err := config.Set(&v, poe1.Campaign, testCase.campaign); err != nil {
					t.Fatal(err)
				}
			}
			if err := config.Set(&v, poe1.Bandits, testCase.bandits); err != nil {
				t.Fatal(err)
			}
			sh := openWith(t, append(endgame(), banditRewards()...), v)
			for stat, want := range testCase.want {
				if got := value(t, sh, stat); got != want {
					t.Errorf("%s: got %v, want %v", stat, got, want)
				}
			}
		})
	}
}

func TestUnknownQuestFlagIsNoted(t *testing.T) {
	t.Parallel()
	odd := bundle(resolve.QuestSource{ID: "a9q9", Flag: "a9q9-Something"}, resolve.Global, resolve.StatValue{Stat: "base_maximum_life", Value: 100})
	sh := openWith(t, append(endgame(), odd), nil)
	if got := value(t, sh, "maximum_life"); got != 1134 {
		t.Errorf("unknown reward applied: life %v", got)
	}
	found := false
	for _, n := range sh.Notes() {
		if n.Source == (resolve.QuestSource{ID: "a9q9", Flag: "a9q9-Something"}) && n.Kind == poe1.NotApplied {
			found = true
		}
	}
	if !found {
		t.Errorf("no note for the unknown flag: %v", sh.Notes())
	}
}
