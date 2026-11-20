package simulator_test

import (
	"testing"

	gocmp "github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/simulator"
)

// TestBudget pins the budget on 3.29 against its quest rows. The default config is the
// campaign complete without the bandit kill: 21 quests worth 1 point and a10q3 worth 2. Up to
// act 5 it is 9 quests worth 1. Four labyrinths give 2 ascendancy points each. Simulate and
// Budget give the same.
func TestBudget(t *testing.T) {
	t.Parallel()
	sim, err := simulator.New(realdata.Open(t, realdata.Current))
	if err != nil {
		t.Fatal(err)
	}
	testCases := []struct {
		name  string
		level int
		opts  []simulator.Option
		want  simulator.Budget
	}{
		{name: "level 1", level: 1, want: simulator.Budget{Passive: 23, Ascendancy: 8, Ascendancies: map[string]int{}, Quests: 23}},
		{name: "level 100", level: 100, want: simulator.Budget{Passive: 122, Ascendancy: 8, Ascendancies: map[string]int{}, Level: 99, Quests: 23}},
		{name: "level 100, acts 1 to 5", level: 100, opts: []simulator.Option{simulator.Set(poe1.Campaign, poe1.CampaignActs1To5)},
			want: simulator.Budget{Passive: 108, Ascendancy: 8, Ascendancies: map[string]int{}, Level: 99, Quests: 9}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := &build.Build{Format: 1, Game: "poe1", Version: "3.29", Class: marauder, Level: testCase.level}
			st, err := sim.Simulate(b, append(testCase.opts, simulator.Outputs(poe1.Life))...)
			if err != nil {
				t.Fatal(err)
			}
			if diff := gocmp.Diff(testCase.want, st.Budget()); diff != "" {
				t.Errorf("Simulate (-want +got):\n%s", diff)
			}
			c, _, err := build.Check(b, sim.Store())
			if err != nil {
				t.Fatal(err)
			}
			got, err := sim.Budget(c, testCase.opts...)
			if err != nil {
				t.Fatal(err)
			}
			if diff := gocmp.Diff(testCase.want, got); diff != "" {
				t.Errorf("Budget (-want +got):\n%s", diff)
			}
		})
	}
}
