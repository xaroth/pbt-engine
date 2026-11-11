package simulator_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/simulator"
)

const (
	gemFireball    = "Metadata/Items/Gems/SkillGemFireball"
	gemInspiration = "Metadata/Items/Gems/SkillGemSupportInspiration"
	gemTwinA       = "Metadata/Items/Gems/SkillGemTwinA"
	gemTwinB       = "Metadata/Items/Gems/SkillGemTwinB"
)

// skillsSim is a sim over testStore with a skills group: Fireball with its transfigured
// Fireball of Splitting, Inspiration Support, and two gems named Twin with different effects,
// levels 1 to 20.
func skillsSim(tb testing.TB) *simulator.Sim {
	tb.Helper()
	d := testDataset("3.29.3.3", 10)
	levels := []dataset.EffectLevel{{Level: 1}, {Level: 20}}
	d.Skills = &dataset.Skills{
		Types: []string{"Spell"},
		Gems: []dataset.Gem{
			{ID: gemFireball, Colour: dataset.Blue, Effects: []dataset.GemEffect{
				{ID: "Fireball", GrantedEffect: "Fireball", Style: dataset.DefaultStyle},
				{ID: "FireballAltX", GrantedEffect: "FireballAltX", Style: dataset.TransfiguredX},
			}},
			{ID: gemInspiration, Colour: dataset.Blue, Support: true, Effects: []dataset.GemEffect{
				{ID: "SupportInspiration", GrantedEffect: "SupportInspiration", Style: dataset.DefaultStyle},
			}},
			{ID: gemTwinA, Colour: dataset.Blue, Effects: []dataset.GemEffect{{ID: "TwinA", GrantedEffect: "TwinA", Style: dataset.DefaultStyle}}},
			{ID: gemTwinB, Colour: dataset.Blue, Effects: []dataset.GemEffect{{ID: "TwinB", GrantedEffect: "TwinB", Style: dataset.DefaultStyle}}},
		},
		GrantedEffects: []dataset.GrantedEffect{
			{ID: "Fireball", ActiveSkill: "fireball", Levels: levels},
			{ID: "FireballAltX", ActiveSkill: "fireball_alt_x", Levels: levels},
			{ID: "SupportInspiration", Support: true, Levels: levels},
			{ID: "TwinA", ActiveSkill: "fireball", Levels: levels},
			{ID: "TwinB", ActiveSkill: "fireball", Levels: levels},
		},
		ActiveSkills: []dataset.ActiveSkill{{ID: "fireball", Types: []uint16{0}}, {ID: "fireball_alt_x", Types: []uint16{0}}},
	}
	d.Texts = []dataset.Text{{
		Language: "en",
		GemNames: []dataset.Name{
			{ID: gemFireball, Name: "Fireball"}, {ID: gemInspiration, Name: "Inspiration Support"}, {ID: gemTwinA, Name: "Twin"}, {ID: gemTwinB, Name: "Twin"},
		},
		SkillNames: []dataset.Name{
			{ID: "fireball", Name: "Fireball"}, {ID: "fireball_alt_x", Name: "Fireball of Splitting"},
		},
	}}
	s, err := simulator.New(storeOf(tb, d))
	if err != nil {
		tb.Fatal(err)
	}
	return s
}

func TestDataGem(t *testing.T) {
	t.Parallel()
	s := skillsSim(t)
	testCases := []struct {
		name string
		want build.GemID
	}{
		{"Fireball", build.GemID{ID: gemFireball}},
		{"Fireball of Splitting", build.GemID{ID: gemFireball, Effect: "FireballAltX"}},
		{"Inspiration", build.GemID{ID: gemInspiration}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got, err := s.Data().Gem(testCase.name)
			if err != nil || got != testCase.want {
				t.Errorf("got %+v (%v), want %+v", got, err, testCase.want)
			}
		})
	}
	if _, err := s.Data().Gem("Twin"); !errors.Is(err, simulator.ErrAmbiguousName) {
		t.Errorf("name of two gems: %v, want ErrAmbiguousName", err)
	}
	if _, err := s.Data().Gem("Firebolt"); !errors.Is(err, dataset.ErrNotInDataset) {
		t.Errorf("unknown gem: %v", err)
	}
	if _, err := newSim(t).Data().Gem("Fireball"); !errors.As(err, new(*dataset.MissingGroupError)) {
		t.Errorf("without a skills group: %v", err)
	}
}

// A gem the dataset lacks is dropped with a warning; the build's other gems are still not
// simulated.
func TestSimulateUnknownGem(t *testing.T) {
	t.Parallel()
	s := skillsSim(t)
	b := marauderBuild(90)
	fireball, err := s.Data().Gem("Fireball")
	if err != nil {
		t.Fatal(err)
	}
	b.Skills.AddGroup(build.SkillGroup{}).AddGem(fireball, 20, 0)
	b.Skills.Group(0).AddGem(build.GemID{ID: "Metadata/Items/Gems/SkillGemNothing"}, 20, 0)
	st := simulate(t, s, b)
	if got, want := codes(st), []build.Code{build.GemUnknown, simulator.SkillsNotSimulated}; !slices.Equal(got, want) {
		t.Errorf("codes %v, want %v", got, want)
	}
	if got := st.Build().Skills.Groups[0].Gems; len(got) != 2 {
		t.Errorf("Stats.Build changed the build's gems: %+v", got)
	}
}
