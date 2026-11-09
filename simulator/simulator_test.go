package simulator_test

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"slices"
	"sync"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/output"
	"github.com/xaroth/pbt-engine/rules/poe1"
	"github.com/xaroth/pbt-engine/simulator"
)

const marauder = "Metadata/Characters/Str/Str"

func statIDs() []string {
	return []string{
		"level", "life_per_level", "base_maximum_all_resistances_%", "base_strength", "base_dexterity", "base_intelligence",
		"base_maximum_life", "base_maximum_mana", "maximum_life_+%", "base_resist_all_elements_%", "base_chaos_damage_resistance_%",
	}
}

// testStore is a Marauder (32 strength, 38 life plus 12 per level, 75% maximum resistances)
// with nodes 2 (+20 life), 3 (+10% life) and mastery 4 (effect 7: +5% life) off the start 1,
// node 5 joining nothing, and both Kitava rewards (-30% all resistances each).
func testStore(tb testing.TB) *dataset.Store {
	tb.Helper()
	d := &dataset.Dataset{
		Manifest: dataset.Manifest{Game: "poe1", Version: "3.29.3.3"},
		Characters: []dataset.Character{{
			ID: marauder, BaseStrength: 32, BaseMaxLife: 38,
			Stats: []dataset.StatValue{{Stat: 0, Value: 1}, {Stat: 1, Value: 12}, {Stat: 2, Value: 75}},
		}},
		Constants: []dataset.Constant{
			{ID: "life_per_strength", Value: 1, Per: 2}, {ID: "mana_per_intelligence", Value: 1, Per: 2},
			{ID: "evasion_rating_+%_per_dexterity", Value: 1, Per: 5}, {ID: "energy_shield_+%_per_intelligence", Value: 1, Per: 10},
		},
		Quests: []dataset.Quest{
			{Quest: "a5q6", Flag: "a5q6-CompletedBoss", Act: 5, Stats: []dataset.StatValue{{Stat: 9, Value: -30}, {Stat: 10, Value: -30}}},
			{Quest: "a10q3", Flag: "a10q3-UsedRewardBook", Act: 10, PassivePoints: 2},
			{Quest: "a10q3", Flag: "EndGameAvailable", Act: 10, Stats: []dataset.StatValue{{Stat: 9, Value: -30}, {Stat: 10, Value: -30}}},
		},
		Trees: []dataset.Tree{{
			Variant: "standard",
			Classes: []dataset.Class{{Character: marauder, StartNode: 1, Ascendancies: []dataset.Ascendancy{{ID: "Juggernaut"}}}},
			Nodes: []dataset.Node{
				{ID: 1, Kind: dataset.ClassStart, Neighbours: []uint32{2, 3, 4}},
				{ID: 2, Neighbours: []uint32{1}, Stats: []dataset.StatValue{{Stat: 6, Value: 20}}},
				{ID: 3, Neighbours: []uint32{1}, Stats: []dataset.StatValue{{Stat: 8, Value: 10}}},
				{ID: 4, Kind: dataset.Mastery, Neighbours: []uint32{1}, MasteryEffects: []dataset.MasteryEffect{{ID: 7, Stats: []dataset.StatValue{{Stat: 8, Value: 5}}}}},
				{ID: 5, Stats: []dataset.StatValue{{Stat: 6, Value: 1}}},
			},
		}},
	}
	for _, id := range statIDs() {
		d.Stats = append(d.Stats, dataset.Stat{ID: id})
	}
	dir := tb.TempDir()
	if _, err := dataset.Write(dir, d); err != nil {
		tb.Fatal(err)
	}
	s, err := dataset.Open(os.DirFS(dir))
	if err != nil {
		tb.Fatal(err)
	}
	return s
}

func newSim(tb testing.TB) *simulator.Sim {
	tb.Helper()
	s, err := simulator.New(testStore(tb))
	if err != nil {
		tb.Fatal(err)
	}
	return s
}

func marauderBuild(level int, nodes ...uint32) *build.Build {
	return &build.Build{Format: 1, Game: "poe1", Version: "3.29", Class: marauder, Level: level, Nodes: nodes}
}

func simulate(tb testing.TB, s *simulator.Sim, b *build.Build, opts ...simulator.Option) *simulator.Stats {
	tb.Helper()
	st, err := s.Simulate(b, opts...)
	if err != nil {
		tb.Fatal(err)
	}
	return st
}

func get(tb testing.TB, st *simulator.Stats, k output.Key) float64 {
	tb.Helper()
	v, ok := st.Get(k)
	if !ok {
		tb.Fatalf("%s not computed", k)
	}
	return v
}

func codes(st *simulator.Stats) []build.Code {
	out := make([]build.Code, 0, len(st.Warnings()))
	for _, w := range st.Warnings() {
		out = append(out, w.Code)
	}
	return out
}

func TestSimulate(t *testing.T) {
	t.Parallel()
	s := newSim(t)
	b := marauderBuild(90, 2, 3, 4)
	b.Masteries = map[uint32]uint32{4: 7}
	st := simulate(t, s, b)
	// (38 + 12 * 90 + 16 + 20) * 1.15 = 1327.1
	if v := get(t, st, poe1.Life); v != 1327 {
		t.Errorf("life %v, want 1327", v)
	}
	if v := get(t, st, poe1.FireResistance); v != -60 {
		t.Errorf("fire resistance %v, want -60", v)
	}
	if len(st.Keys()) != len(poe1.Outputs()) || len(st.Warnings()) != 0 {
		t.Errorf("keys %d, warnings %v", len(st.Keys()), st.Warnings())
	}
	b.Nodes[0] = 5
	if v := get(t, st, poe1.Life); v != 1327 {
		t.Errorf("stats changed with the build: life %v", v)
	}
}

func setCampaign(tb testing.TB, stage poe1.CampaignStage) config.Values {
	tb.Helper()
	var v config.Values
	if err := config.Set(&v, poe1.Campaign, stage); err != nil {
		tb.Fatal(err)
	}
	return v
}

// Config resolves per call, sim, build, default; per-call values leave the sim unchanged and
// Clear hands a key back.
func TestConfigLayers(t *testing.T) {
	t.Parallel()
	s := newSim(t)
	b := marauderBuild(90)
	b.Config = setCampaign(t, poe1.CampaignActs1To5)
	check := func(st *simulator.Stats, fire float64, src config.Source) {
		t.Helper()
		e, _ := st.Config(poe1.Campaign)
		if v := get(t, st, poe1.FireResistance); v != fire || e.Source != src {
			t.Errorf("fire %v from %s, want %v from %s", v, e.Source, fire, src)
		}
	}
	check(simulate(t, s, b), 0, config.FromBuild)
	if err := s.Configure(simulator.Set(poe1.Campaign, poe1.CampaignComplete)); err != nil {
		t.Fatal(err)
	}
	check(simulate(t, s, b), -60, config.FromSim)
	check(simulate(t, s, b, simulator.Set(poe1.Campaign, poe1.CampaignActs6To10)), -30, config.FromCall)
	check(simulate(t, s, b, simulator.Clear("campaign")), 0, config.FromBuild)
	check(simulate(t, s, b), -60, config.FromSim)
	if err := s.Configure(simulator.Clear("campaign")); err != nil {
		t.Fatal(err)
	}
	check(simulate(t, s, b), 0, config.FromBuild)
	check(simulate(t, s, marauderBuild(90)), -60, config.Default)
	if len(s.Config()) != 0 {
		t.Errorf("sim config %v, want empty", s.Config())
	}
}

func TestWarnings(t *testing.T) {
	t.Parallel()
	s := newSim(t)
	b := marauderBuild(1, 2, 3, 4, 5, 99)
	b.Version = "3.28"
	b.Config = config.Values{"campaign": json.RawMessage(`"act 3"`), "charges": json.RawMessage(`3`)}
	st := simulate(t, s, b)
	want := []build.Code{
		build.NodeUnknown, build.NodesDisconnected, build.MasteryNotChosen, simulator.VersionMismatch,
		simulator.ConfigBadValue, simulator.ConfigUnknownKey, poe1.PointsOverBudget,
	}
	if got := codes(st); !slices.Equal(got, want) {
		t.Errorf("codes\n got %v\nwant %v", got, want)
	}
}

func TestOutputsAndErrors(t *testing.T) {
	t.Parallel()
	s := newSim(t)
	st := simulate(t, s, marauderBuild(1), simulator.Outputs(poe1.Life, poe1.Mana))
	if _, ok := st.Get(poe1.FireResistance); ok || len(st.Keys()) != 2 {
		t.Errorf("keys %v", st.Keys())
	}
	if _, err := s.Simulate(marauderBuild(1), simulator.Outputs("player.nothing")); !errors.Is(err, simulator.ErrUnknownOutput) {
		t.Errorf("unknown output: %v", err)
	}
	other := marauderBuild(1)
	other.Class = "Metadata/Characters/Dex/Dex"
	if _, err := s.Simulate(other); !errors.Is(err, simulator.ErrUnreconcilable) || !errors.Is(err, build.ErrClass) {
		t.Errorf("other class: %v", err)
	}
	if err := s.Configure(simulator.Outputs(poe1.Life)); !errors.Is(err, simulator.ErrOption) {
		t.Errorf("outputs on configure: %v", err)
	}
	if err := s.Configure(simulator.Set(poe1.Campaign, "act 3")); !errors.Is(err, simulator.ErrOption) {
		t.Errorf("bad value: %v", err)
	}
}

func sameStats(tb testing.TB, what string, a, b *simulator.Stats) {
	tb.Helper()
	for _, k := range a.Keys() {
		x, y := get(tb, a, k), get(tb, b, k)
		if math.Float64bits(x) != math.Float64bits(y) {
			tb.Fatalf("%s: %s %v and %v", what, k, x, y)
		}
	}
}

// A pooled sheet gives the bits a fresh one does, whatever ran on it before.
func TestReuse(t *testing.T) {
	t.Parallel()
	s := newSim(t)
	a, b := marauderBuild(90, 2, 3), marauderBuild(90, 4)
	b.Masteries = map[uint32]uint32{4: 7}
	first := simulate(t, s, a)
	simulate(t, s, b, simulator.Set(poe1.Campaign, poe1.CampaignActs1To5))
	sameStats(t, "a after b", first, simulate(t, s, a))
	sameStats(t, "b on a fresh sim", simulate(t, newSim(t), b), simulate(t, s, b))
}

// Concurrent runs and Configure calls: each run matches its own config snapshot.
func TestConcurrent(t *testing.T) {
	t.Parallel()
	s := newSim(t)
	fire := map[poe1.CampaignStage]float64{poe1.CampaignComplete: -60, poe1.CampaignActs6To10: -30, poe1.CampaignActs1To5: 0}
	stages := []poe1.CampaignStage{poe1.CampaignComplete, poe1.CampaignActs6To10, poe1.CampaignActs1To5}
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 30 {
				if g == 0 {
					if err := s.Configure(simulator.Set(poe1.Campaign, stages[i%3])); err != nil {
						t.Error(err)
					}
					continue
				}
				st, err := s.Simulate(marauderBuild(90, uint32(2+i%3)))
				if err != nil {
					t.Error(err)
					return
				}
				e, _ := st.Config(poe1.Campaign)
				stage, _ := e.Value.(poe1.CampaignStage)
				if v, _ := st.Get(poe1.FireResistance); v != fire[stage] {
					t.Errorf("campaign %s: fire %v", stage, v)
				}
			}
		})
	}
	wg.Wait()
}

func TestNew(t *testing.T) {
	t.Parallel()
	store := testStore(t)
	s, err := simulator.New(store, simulator.Set(poe1.Campaign, poe1.CampaignActs1To5))
	if err != nil {
		t.Fatal(err)
	}
	if v := get(t, simulate(t, s, marauderBuild(1)), poe1.FireResistance); v != 0 || s.Store() != store {
		t.Errorf("fire %v", v)
	}
	if _, err := simulator.New(store, simulator.Set(poe1.Bandits, "help-eramir")); !errors.Is(err, simulator.ErrOption) {
		t.Errorf("bad option: %v", err)
	}
}

// Level is switched on a pooled sheet: one sheet serves every level with fresh results.
func TestLevelsShareASheet(t *testing.T) {
	t.Parallel()
	s := newSim(t)
	for _, level := range []int{90, 1, 47, 90, 100, 1} {
		b := marauderBuild(level, 2, 3)
		sameStats(t, "level", simulate(t, newSim(t), b), simulate(t, s, b))
	}
	if n := s.IdleSheets(); n != 1 {
		t.Errorf("%d idle sheets for one shape, want 1", n)
	}
}

// The pool keeps at most MaxIdle sheets, however many run at once.
func TestPoolLimit(t *testing.T) {
	t.Parallel()
	s := newSim(t)
	var wg sync.WaitGroup
	for range 3 * simulator.MaxIdle {
		wg.Go(func() {
			if _, err := s.Simulate(marauderBuild(90, 2)); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if n := s.IdleSheets(); n > simulator.MaxIdle {
		t.Errorf("%d idle sheets, limit %d", n, simulator.MaxIdle)
	}
}
