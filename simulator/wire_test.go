package simulator_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/output"
	"github.com/xaroth/pbt-engine/pob"
	"github.com/xaroth/pbt-engine/simulator"
)

const schemaFile = "wire.schema.json"

// wireGolden compares the indented JSON of v with testdata/wire/name, or writes it with
// -update, and checks it against the schema's definition def unless def is empty.
func wireGolden(t *testing.T, name, def string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if def != "" {
		validate(t, def, data)
	}
	path := filepath.Join("testdata", "wire", name)
	if *update {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to write it)", err)
	}
	if diff := cmp.Diff(string(want), string(data)); diff != "" {
		t.Errorf("%s differs; if the wire format changed on purpose, update %s and run with -update (-want +got):\n%s",
			path, schemaFile, diff)
	}
}

// The wire JSON of each result kind equals its golden file and matches the schema. A change
// of shape fails here until the goldens are rewritten, and then fails the schema check until
// the schema says the same.
func TestWireGolden(t *testing.T) {
	t.Parallel()
	s := newSim(t)
	b := marauderBuild(90, 2, 3, 4, 99)
	b.Masteries = map[uint32]uint32{4: 7}
	b.Version = "3.28"
	few := simulator.Outputs(poe1.Dexterity, poe1.Life, poe1.FireResistance, poe1.LifeRegeneration)
	across := acrossDatasets(t, few)
	compare, err := s.Compare(marauderBuild(1), marauderBuild(90, 2, 6), few, simulator.Set(poe1.Campaign, poe1.CampaignActs1To5))
	if err != nil {
		t.Fatal(err)
	}
	sweep, err := s.Sweep(marauderBuild(70, 2), [][]build.Edit{
		{build.Allocate(3)}, {build.SetGemLevel(5, 0, 20)}, {build.Allocate(99)}, nil,
	}, few)
	if err != nil {
		t.Fatal(err)
	}
	testCases := []struct {
		name, def string
		v         any
	}{
		{"stats.json", "Stats", simulate(t, s, b)},
		{"diff.json", "Diff", compare},
		{"diff-datasets.json", "Diff", across},
		// A node change with no fields or stats still lists them, as empty lists.
		{"diff-unconvertible.json", "Diff", &simulator.Diff{
			Before: compare.Before, After: compare.After, DataChanges: []simulator.NodeChange{{Node: 7}},
		}},
		{"result.json", "Result", sweep[0]},
		{"result-error.json", "Result", sweep[1]},
		{"result-warning.json", "Result", sweep[2]},
		{"result-no-edits.json", "Result", sweep[3]},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			wireGolden(t, testCase.name, testCase.def, testCase.v)
		})
	}
}

// acrossDatasets diffs one build on a 3.28 dataset, where node 2 has several stats, and on
// testStore, where it has one.
func acrossDatasets(t *testing.T, opts ...simulator.Option) *simulator.Diff {
	t.Helper()
	d := testDataset("3.28.0.16", 10)
	// stats 3 to 7 are base strength, dexterity, intelligence, life and mana, in index order,
	// which is not the order of their ids
	node := &d.Trees[0].Nodes[1]
	node.Stats = []dataset.StatValue{{Stat: 3, Value: 1}, {Stat: 4, Value: 2}, {Stat: 5, Value: 3}, {Stat: 6, Value: 10}, {Stat: 7, Value: 4}}
	older, err := simulator.New(storeOf(t, d))
	if err != nil {
		t.Fatal(err)
	}
	across, err := simulate(t, older, marauderBuild(90, 2, 3), opts...).Diff(simulate(t, newSim(t), marauderBuild(90, 2, 3), opts...))
	if err != nil {
		t.Fatal(err)
	}
	return across
}

var errReadFailed = errors.New("read failed")

// A sweep variant whose output read fails has no changes.
func TestWireResultErrorHasNoChanges(t *testing.T) {
	t.Parallel()
	fail := errReadFailed
	keys := []output.Key{poe1.Life, poe1.Mana}
	read := func(k output.Key) (float64, error) {
		if k == poe1.Mana {
			return 0, fail
		}
		return 1, nil
	}
	if got, err := simulator.ReadChanges(keys, nil, read); !errors.Is(err, fail) || got != nil {
		t.Errorf("failing read: changes %v, err %v; want none and the read's error", got, err)
	}
	ok := func(output.Key) (float64, error) { return 1, nil }
	if got, err := simulator.ReadChanges(keys, nil, ok); err != nil || len(got) != 2 {
		t.Errorf("reads: changes %v, err %v; want 2", got, err)
	}
}

// The catalogue's JSON equals its golden file, and each entry matches the schema.
func TestWireOutputs(t *testing.T) {
	t.Parallel()
	outputs := newSim(t).Outputs()
	for _, o := range outputs {
		data, err := json.Marshal(o)
		if err != nil {
			t.Fatal(err)
		}
		validate(t, "Output", data)
	}
	wireGolden(t, "outputs.json", "", outputs)
}

// An explanation's JSON in each view equals its golden file and matches the schema.
func TestWireExplanation(t *testing.T) {
	t.Parallel()
	b := marauderBuild(90, 2, 3, 4, 6, 8, 10)
	b.Masteries = map[uint32]uint32{4: 7, 6: 9}
	st := simulate(t, newSim(t), b)
	for _, v := range []simulator.View{simulator.Tree, simulator.Proportional, simulator.Marginal} {
		ex, err := st.Explain(poe1.Life, v)
		if err != nil {
			t.Fatal(err)
		}
		wireGolden(t, "explanation-"+v.String()+".json", "Explanation", ex)
	}
	// Fire resistance has the campaign's quest rewards, through the stat all elements share.
	ex, err := st.Explain(poe1.FireResistance, simulator.Proportional)
	if err != nil {
		t.Fatal(err)
	}
	wireGolden(t, "explanation-quest.json", "Explanation", ex)
}

// Cold resistance takes half of node 20's fire resistance through node 21's share: the tree
// view has the term at rate 0.5, the proportional view credits node 21 with node 20's
// modifier as the part's from.
func TestWireExplanationShared(t *testing.T) {
	t.Parallel()
	d := testDataset("3.29.3.3", 20)
	fire, share := uint32(len(d.Stats)), uint32(len(d.Stats)+1)
	d.Stats = append(d.Stats, dataset.Stat{ID: "base_fire_damage_resistance_%"},
		dataset.Stat{ID: "modifiers_to_fire_resistance_also_apply_to_cold_lightning_resistance_at_%_value"})
	tree := &d.Trees[0]
	tree.Nodes[0].Neighbours = append(tree.Nodes[0].Neighbours, 20, 21)
	tree.Nodes = append(tree.Nodes,
		dataset.Node{ID: 20, Neighbours: []uint32{1}, Stats: []dataset.StatValue{{Stat: fire, Value: 30}}},
		dataset.Node{ID: 21, Neighbours: []uint32{1}, Stats: []dataset.StatValue{{Stat: share, Value: 50}}},
	)
	s, err := simulator.New(storeOf(t, d))
	if err != nil {
		t.Fatal(err)
	}
	st := simulate(t, s, marauderBuild(90, 20, 21))
	for _, v := range []simulator.View{simulator.Tree, simulator.Proportional} {
		ex, err := st.Explain(poe1.ColdResistance, v)
		if err != nil {
			t.Fatal(err)
		}
		wireGolden(t, "explanation-shared-"+v.String()+".json", "Explanation", ex)
	}
}

// Warnings, catalogue entries and edits read back into the values they were written from.
func TestWireRoundTrip(t *testing.T) {
	t.Parallel()
	item := build.Item{
		Rarity: build.Rare, Name: "Doom Crown", Base: "Metadata/Items/Armours/Helmets/HelmetStr10", ItemLevel: 84, Quality: 20,
		Sockets: "R-R-B G", Armour: 500, Evasion: 1, EnergyShield: 2, Ward: 3, Flags: []string{"Corrupted"},
		Mods: []build.ModLine{{
			Kind: build.Explicit, Text: "+80 to maximum Life", Mod: "IncreasedLife7",
			Stats: []build.StatValue{{Stat: "base_maximum_life", Value: 80}},
		}},
	}
	gem := build.Gem{ID: "Fireball", Effect: "Alt", Level: 20, Quality: 23, Support: true, Disabled: true}
	group := build.SkillGroup{Slot: build.SlotHelmet, Gems: []build.Gem{gem}, Active: 1}
	setConfig, err := build.SetConfig(poe1.Campaign, poe1.CampaignActs1To5)
	if err != nil {
		t.Fatal(err)
	}
	edits := []build.Edit{
		build.Allocate(1, 2), build.Deallocate(3), build.SetMastery(4, 7), build.ClearMastery(4), build.SetLevel(90),
		build.SetClass("Metadata/Characters/Str/Str"), build.SetAscendancy("Juggernaut"), setConfig, build.ClearConfig("campaign"),
		build.AddSkillGroup(group), build.RemoveSkillGroup(1), build.SetSkillSlot(0, build.SlotRing1), build.SetMainSkill(2),
		build.AddGem(0, group.Gems[0]), build.RemoveGem(0, 1), build.SetActiveGem(0, 1), build.SetGemLevel(0, 1, 21),
		build.SetGemQuality(0, 1, 23), build.SetGemEnabled(0, 1, true), build.Equip(build.SlotHelmet, item), build.Unequip(build.SlotBelt),
	}
	for _, e := range edits {
		roundTrip(t, "Edit", e)
	}
	roundTrip(t, "Warning", build.Warning{Code: build.NodeUnknown, Subject: "node 99", Message: "dropped"})
	for _, o := range newSim(t).Outputs() {
		roundTrip(t, "Output", o)
	}
}

// roundTrip checks v's JSON against the schema's definition def and that it reads back into v.
func roundTrip[T any](t *testing.T, def string, v T) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	validate(t, def, data)
	var got T
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(v, got, cmpopts.IgnoreUnexported(simulator.Output{})); diff != "" {
		t.Errorf("%s round trip (-want +got):\n%s", data, diff)
	}
}

// Equal results give equal bytes, though Result and NodeChange hold maps.
func TestWireDeterministic(t *testing.T) {
	t.Parallel()
	res, err := newSim(t).Sweep(marauderBuild(70, 2), [][]build.Edit{{build.Allocate(3)}})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []any{res, acrossDatasets(t)} {
		want, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		for range 20 {
			if got, err := json.Marshal(v); err != nil || !bytes.Equal(got, want) {
				t.Fatalf("got %s, %v; want %s", got, err, want)
			}
		}
	}
}

// The wire JSON of a real build's results and explanations matches the schema on both pinned datasets.
func TestWireRealData(t *testing.T) {
	t.Parallel()
	previous, err := simulator.New(realdata.Open(t, realdata.Previous))
	if err != nil {
		t.Fatal(err)
	}
	current, err := simulator.New(realdata.Open(t, realdata.Current))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := dataset.OpenRoot(os.DirFS(realdata.Root(t)))
	if reg == nil {
		t.Fatal(err)
	}
	base, _, err := pob.FromXML(chieftainXML(t), reg)
	if err != nil {
		t.Fatal(err)
	}
	next := base.Clone()
	if err := next.Apply(build.Deallocate(base.Nodes[len(base.Nodes)-5:]...)); err != nil {
		t.Fatal(err)
	}
	st := simulate(t, current, base)
	compare, err := current.Compare(base, next)
	if err != nil {
		t.Fatal(err)
	}
	across, err := simulate(t, previous, base).Diff(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(across.DataChanges) == 0 {
		t.Error("3.28 to 3.29: no data changes, want some to check against the schema")
	}
	variants := make([][]build.Edit, 0, 5)
	for _, n := range base.Nodes[len(base.Nodes)-5:] {
		variants = append(variants, []build.Edit{build.Deallocate(n)})
	}
	results, err := current.Sweep(base, variants)
	if err != nil {
		t.Fatal(err)
	}
	check := func(def string, v any) {
		t.Helper()
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		validate(t, def, data)
	}
	check("Stats", st)
	for _, k := range st.Keys() {
		for _, v := range []simulator.View{simulator.Tree, simulator.Proportional, simulator.Marginal} {
			ex, err := st.Explain(k, v)
			if err != nil {
				t.Fatal(err)
			}
			check("Explanation", ex)
		}
	}
	check("Diff", compare)
	check("Diff", across)
	for _, r := range results {
		check("Result", r)
	}
}
