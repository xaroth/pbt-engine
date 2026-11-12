package dataset_test

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"slices"
	"testing"
	"testing/fstest"

	flatbuffers "github.com/google/flatbuffers/go"
	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/dataset/internal/fb"
)

// Stat handles withBuffs adds after sample's three.
const (
	armourRating uint32 = iota + 3
	determinationArmour
	maxFrenzy
	currentFrenzy
	curseCount
	immuneToIgnite
)

// withBuffs returns withItems with a buffs group: an aura, a curse, a charge, a stacking debuff
// and a utility flask's buff, the flask and a tincture.
func withBuffs() *dataset.Dataset {
	d := withItems()
	d.Stats = append(d.Stats,
		dataset.Stat{ID: "base_physical_damage_reduction_rating"}, dataset.Stat{ID: "determination_aura_armour_+%_final"},
		dataset.Stat{ID: "virtual_maximum_frenzy_charges"}, dataset.Stat{ID: "actual_current_frenzy_charges"},
		dataset.Stat{ID: "curse_count"}, dataset.Stat{ID: "base_immune_to_ignite"})
	d.Items.Classes = append(d.Items.Classes, dataset.ItemClass{ID: "UtilityFlask"}, dataset.ItemClass{ID: "Tincture"})
	d.Items.Bases = append(d.Items.Bases,
		dataset.ItemBase{
			ID: "Metadata/Items/Flasks/FlaskUtility5", Class: "UtilityFlask", ModDomain: 2,
			Flask: &dataset.FlaskValues{Kind: dataset.UtilityFlask, Duration: 60, MaxCharges: 30, ChargesPerUse: 15},
		},
		dataset.ItemBase{ID: "Metadata/Items/Tinctures/Tincture1", Class: "Tincture", ModDomain: 34})
	d.Buffs = sampleBuffs()
	d.Texts[0].BuffNames = []dataset.Name{{ID: "player_aura_armour", Name: "Determination Aura"}, {ID: "frenzy_charge", Name: "Frenzy Charges"}}
	return d
}

func sampleBuffs() *dataset.Buffs {
	return &dataset.Buffs{
		Definitions: []dataset.BuffDefinition{
			{
				ID: "player_aura_armour", Category: 1, Stats: []uint32{determinationArmour, armourRating},
				SkillBuff: true, Removable: true, CancelOnDeath: true,
			},
			{
				ID: "curse_temporal_chains", Category: 5, ID2: "CurseDebuff", Stats: []uint32{1, 1},
				ConditionStats: []uint32{curseCount}, SkillBuff: true,
			},
			{ID: "frenzy_charge", Category: 3, MaximumStat: new(maxFrenzy), CurrentStat: new(currentFrenzy), MinimumStat: new(uint32(0)), CancelOnDeath: true},
			{
				ID: "flask_utility_ironskin", Category: 4, Stats: []uint32{armourRating}, ExtraStats: []uint32{2},
				Flags: []uint32{immuneToIgnite},
			},
			{ID: "withered", Category: 2, Limit: 15, Invisible: true, Recovery: true},
		},
		Flasks:    []dataset.FlaskBuff{{Item: "Metadata/Items/Flasks/FlaskUtility5", Buff: "flask_utility_ironskin", Values: []int32{1500, 3}}},
		Tinctures: []dataset.Tincture{{Item: "Metadata/Items/Tinctures/Tincture1", DebuffInterval: 700, Cooldown: 8000}},
	}
}

func TestBuffsRoundTrip(t *testing.T) {
	t.Parallel()
	want := withBuffs()
	got, err := readAll(written(t, want))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want.Manifest = got.Manifest
	if diff := gocmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("round trip mismatch (-want +got):\n%s", diff)
	}
	if !slices.ContainsFunc(got.Manifest.Files, func(f dataset.File) bool { return f.Name == "buffs.fb" }) {
		t.Errorf("manifest: got files %v, want buffs.fb among them", got.Manifest.Files)
	}
}

// Buff definitions alone need no items group.
func TestBuffsWithoutItems(t *testing.T) {
	t.Parallel()
	want := sample()
	want.Stats = withBuffs().Stats
	want.Buffs = &dataset.Buffs{Definitions: sampleBuffs().Definitions}
	got, err := readAll(written(t, want))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if diff := gocmp.Diff(want.Buffs, got.Buffs, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("round trip mismatch (-want +got):\n%s", diff)
	}
}

// A dataset without a buffs group writes its text file as before the group existed.
func TestTextWithoutBuffNamesUnchanged(t *testing.T) {
	t.Parallel()
	plain, named := sample(), sample()
	named.Texts[0].BuffNames = []dataset.Name{}
	a, b := written(t, plain), written(t, named)
	if !bytes.Equal(a["text-en.fb"].Data, b["text-en.fb"].Data) {
		t.Errorf("text-en.fb: bytes differ with empty buff names, want equal")
	}
}

// renumberedBuffs returns withBuffs with its stats reversed, as a later game version might
// number them.
func renumberedBuffs() *dataset.Dataset {
	d := withBuffs()
	n := uint32(len(d.Stats))
	h := func(x uint32) uint32 { return n - 1 - x }
	d.Stats = slices.Clone(d.Stats)
	slices.Reverse(d.Stats)
	for _, c := range d.Characters {
		renumber(c.Stats, h)
		slices.SortFunc(c.Stats, func(a, b dataset.StatValue) int { return int(a.Stat) - int(b.Stat) })
	}
	for _, q := range d.Quests {
		renumber(q.Stats, h)
	}
	for _, nd := range d.Trees[0].Nodes {
		renumber(nd.Stats, h)
		for _, b := range nd.Buffs {
			renumber(b, h)
		}
		for _, e := range nd.MasteryEffects {
			renumber(e.Stats, h)
		}
	}
	for i := range d.Mods {
		for j := range d.Mods[i].Stats {
			d.Mods[i].Stats[j].Stat = h(d.Mods[i].Stats[j].Stat)
		}
	}
	renumberBuffs(d.Buffs, h)
	return d
}

func renumberBuffs(b *dataset.Buffs, h func(uint32) uint32) {
	for i := range b.Definitions {
		def := &b.Definitions[i]
		for _, hs := range [][]uint32{def.Stats, def.ExtraStats, def.Flags, def.ConditionStats} {
			renumberHandles(hs, h)
		}
		for _, p := range []**uint32{&def.MaximumStat, &def.CurrentStat, &def.MinimumStat} {
			if *p != nil {
				*p = new(h(**p))
			}
		}
	}
}

func TestBuffsFileIndependentOfStatOrder(t *testing.T) {
	t.Parallel()
	a, b := written(t, withBuffs()), written(t, renumberedBuffs())
	if !bytes.Equal(a["buffs.fb"].Data, b["buffs.fb"].Data) {
		t.Errorf("buffs.fb: bytes differ after renumbering stats, want equal")
	}
	got, err := readAll(b)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if diff := gocmp.Diff(renumberedBuffs().Buffs, got.Buffs, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("round trip mismatch (-want +got):\n%s", diff)
	}
}

func TestBuffsWriteErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, want string
		change     func(d *dataset.Dataset)
	}{
		{name: "buff id empty", want: "buff 1: empty id", change: func(d *dataset.Dataset) { d.Buffs.Definitions[1].ID = "" }},
		{name: "buff repeated", want: "buff withered repeated", change: func(d *dataset.Dataset) { d.Buffs.Definitions[0].ID = "withered" }},
		{name: "stat out of range", want: "buff player_aura_armour: stats: stat 99 out of range", change: func(d *dataset.Dataset) {
			d.Buffs.Definitions[0].Stats[1] = 99
		}},
		{name: "extra stat out of range", want: "buff flask_utility_ironskin: extra stats: stat 99 out of range", change: func(d *dataset.Dataset) {
			d.Buffs.Definitions[3].ExtraStats[0] = 99
		}},
		{name: "flag out of range", want: "buff flask_utility_ironskin: flags: stat 99 out of range", change: func(d *dataset.Dataset) {
			d.Buffs.Definitions[3].Flags[0] = 99
		}},
		{name: "condition stat out of range", want: "buff curse_temporal_chains: condition stats: stat 99 out of range", change: func(d *dataset.Dataset) {
			d.Buffs.Definitions[1].ConditionStats[0] = 99
		}},
		{name: "maximum stat out of range", want: "buff frenzy_charge: maximum: stat 99 out of range", change: func(d *dataset.Dataset) {
			d.Buffs.Definitions[2].MaximumStat = new(uint32(99))
		}},
		{name: "current stat out of range", want: "buff frenzy_charge: current: stat 99 out of range", change: func(d *dataset.Dataset) {
			d.Buffs.Definitions[2].CurrentStat = new(uint32(99))
		}},
		{name: "minimum stat out of range", want: "buff frenzy_charge: minimum: stat 99 out of range", change: func(d *dataset.Dataset) {
			d.Buffs.Definitions[2].MinimumStat = new(uint32(99))
		}},
		{name: "limit negative", want: "buff withered: limit -1 negative", change: func(d *dataset.Dataset) { d.Buffs.Definitions[4].Limit = -1 }},
		{name: "flasks without items", want: "flask buffs or tinctures without the items group", change: func(d *dataset.Dataset) {
			d.Items, d.Mods, d.Texts[0].ItemClassNames, d.Texts[0].BaseNames, d.Texts[0].ModNames = nil, nil, nil, nil, nil
			d.Buffs.Tinctures = nil
		}},
		{name: "tinctures without items", want: "flask buffs or tinctures without the items group", change: func(d *dataset.Dataset) {
			d.Items, d.Mods, d.Texts[0].ItemClassNames, d.Texts[0].BaseNames, d.Texts[0].ModNames = nil, nil, nil, nil, nil
			d.Buffs.Flasks = nil
		}},
		{name: "flask item empty", want: "flask 0: empty id", change: func(d *dataset.Dataset) { d.Buffs.Flasks[0].Item = "" }},
		{name: "flask repeated", want: "flask Metadata/Items/Flasks/FlaskUtility5 repeated", change: func(d *dataset.Dataset) {
			d.Buffs.Flasks = append(d.Buffs.Flasks, d.Buffs.Flasks[0])
		}},
		{name: "flask item not a flask", want: "flask Metadata/Items/Armours/Helmets/HelmetStr1: not a flask base in items", change: func(d *dataset.Dataset) {
			d.Buffs.Flasks[0].Item = "Metadata/Items/Armours/Helmets/HelmetStr1"
		}},
		{name: "flask item unknown", want: "flask x: not a flask base in items", change: func(d *dataset.Dataset) { d.Buffs.Flasks[0].Item = "x" }},
		{name: "flask buff unknown", want: `flask Metadata/Items/Flasks/FlaskUtility5: buff "x" not in buffs`, change: func(d *dataset.Dataset) {
			d.Buffs.Flasks[0].Buff = "x"
		}},
		{name: "flask values", want: "flask Metadata/Items/Flasks/FlaskUtility5: 1 values for the 2 stats of buff flask_utility_ironskin",
			change: func(d *dataset.Dataset) { d.Buffs.Flasks[0].Values = d.Buffs.Flasks[0].Values[:1] }},
		{name: "tincture item empty", want: "tincture 0: empty id", change: func(d *dataset.Dataset) { d.Buffs.Tinctures[0].Item = "" }},
		{name: "tincture repeated", want: "tincture Metadata/Items/Tinctures/Tincture1 repeated", change: func(d *dataset.Dataset) {
			d.Buffs.Tinctures = append(d.Buffs.Tinctures, d.Buffs.Tinctures[0])
		}},
		{name: "tincture item unknown", want: "tincture x: not a base in items", change: func(d *dataset.Dataset) { d.Buffs.Tinctures[0].Item = "x" }},
		{name: "unknown buff name", want: "text en: name for unknown buff x", change: func(d *dataset.Dataset) { d.Texts[0].BuffNames[1].ID = "x" }},
		{name: "buff names without buffs", want: "text en: name for unknown buff player_aura_armour", change: func(d *dataset.Dataset) { d.Buffs = nil }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			d := withBuffs()
			testCase.change(d)
			_, err := dataset.Write(t.TempDir(), d)
			checkError(t, err, dataset.ErrInvalid, testCase.want)
		})
	}
}

// buffsFile builds a buffs file with buffs of the given ids, each carrying stat index stat, for
// layouts Write never produces.
func buffsFile(ids []string, stat uint32, statIDs []string) []byte {
	b := flatbuffers.NewBuilder(0)
	offs := make([]flatbuffers.UOffsetT, len(ids))
	for i, id := range ids {
		stats := uint32Vec(b, fb.BuffDefinitionStartStatsVector, []uint32{stat})
		name := b.CreateString(id)
		fb.BuffDefinitionStart(b)
		fb.BuffDefinitionAddId(b, name)
		fb.BuffDefinitionAddStats(b, stats)
		offs[i] = fb.BuffDefinitionEnd(b)
	}
	fb.BuffsStartDefinitionsVector(b, len(offs))
	for _, o := range slices.Backward(offs) {
		b.PrependUOffsetT(o)
	}
	defs := b.EndVector(len(offs))
	statVec := stringVec(b, fb.BuffsStartStatIdsVector, statIDs)
	fb.BuffsStart(b)
	fb.BuffsAddDefinitions(b, defs)
	fb.BuffsAddStatIds(b, statVec)
	fb.FinishBuffsBuffer(b, fb.BuffsEnd(b))
	return b.FinishedBytes()
}

func TestBuffsReadErrors(t *testing.T) {
	t.Parallel()
	good := written(t, withBuffs())
	replaced := func(name string, data []byte) fstest.MapFS { return rewritten(t, good, name, data) }
	buffs := good["buffs.fb"].Data
	fewStats := written(t, &dataset.Dataset{Stats: sample().Stats})["stats.fb"].Data
	noItems := sample()
	noItems.Stats = withBuffs().Stats
	noItems.Buffs = &dataset.Buffs{}
	testCases := []struct {
		name, want string
		fsys       fstest.MapFS
	}{
		{name: "truncated", want: "buffs.fb: malformed", fsys: replaced("buffs.fb", buffs[:len(buffs)/2])},
		{name: "wrong identifier", want: "buffs.fb: wrong file identifier", fsys: replaced("buffs.fb", good["constants.fb"].Data)},
		{name: "stat not in stats group", want: `buffs.fb: unknown stat "actual_current_frenzy_charges"`, fsys: replaced("stats.fb", fewStats)},
		{name: "stat index out of range", want: "buffs.fb: stat 1 out of range (1 ids)", fsys: replaced("buffs.fb", buffsFile([]string{"a"}, 1, []string{"level"}))},
		{name: "fails validation", want: "buffs.fb: buff a repeated", fsys: replaced("buffs.fb", buffsFile([]string{"a", "a"}, 0, []string{"level"}))},
		{
			name: "flasks without items group", want: "buffs.fb: flask buffs or tinctures without the items group",
			fsys: rewritten(t, written(t, noItems), "buffs.fb", buffs),
		},
		{name: "items unreadable", want: "items.fb: malformed", fsys: replaced("items.fb", good["items.fb"].Data[:8])},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s, err := dataset.Open(testCase.fsys)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			_, err = s.Buffs()
			checkError(t, err, dataset.ErrInvalid, testCase.want)
		})
	}
}

func TestStoreBuffsMissing(t *testing.T) {
	t.Parallel()
	s, err := dataset.Open(written(t, sample()))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, err = s.Buffs()
	checkError(t, err, dataset.ErrNotInDataset, "dataset 3.29.3.3 has no buffs group")
	missing, ok := errors.AsType[*dataset.MissingGroupError](err)
	if !ok || *missing != (dataset.MissingGroupError{Version: "3.29.3.3", Group: "buffs"}) {
		t.Errorf("got %#v, want a MissingGroupError for buffs of 3.29.3.3", err)
	}
}

// Buffs reads stats, items and its own file, and nothing else.
func TestStoreReadsBuffsOnUse(t *testing.T) {
	t.Parallel()
	want := withBuffs()
	fsys := maps.Clone(written(t, want))
	for _, n := range []string{"characters.fb", "constants.fb", "quests.fb", "mods.fb", "tree-standard.fb", "tree-ruthless.fb", "text-en.fb", "text-zh-hant.fb"} {
		fsys[n] = &fstest.MapFile{Data: []byte("x")}
	}
	s, err := dataset.Open(fsys)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := s.Buffs()
	if err != nil {
		t.Fatalf("Buffs: %v", err)
	}
	if diff := gocmp.Diff(want.Buffs, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Buffs mismatch (-want +got):\n%s", diff)
	}
}

// syntheticBuffs returns a dataset whose buffs group has about the row counts and id lengths
// of the 3.29.3.3 game data: 2,814 buff definitions with about 4,700 stats, 18 flask buffs and
// 10 tinctures.
func syntheticBuffs() *dataset.Dataset {
	d := &dataset.Dataset{Stats: make([]dataset.Stat, 2000)}
	for i := range d.Stats {
		d.Stats[i].ID = fmt.Sprintf("stat_%04d", i)
	}
	d.Items = &dataset.Items{Classes: []dataset.ItemClass{{ID: "UtilityFlask"}, {ID: "Tincture"}}}
	b := &dataset.Buffs{}
	for i := range 2814 {
		def := dataset.BuffDefinition{ID: syntheticID("player_buff_definition_", i), Category: int32(i % 18), Stats: []uint32{uint32(i % 2000)}, CancelOnDeath: true}
		if i%3 == 0 {
			def.Stats = append(def.Stats, uint32((i+1)%2000), uint32((i+2)%2000))
		}
		if i%5 == 0 {
			def.ID2 = "GenericBuffAura"
		}
		if i%10 == 0 {
			def.Flags = []uint32{uint32((i + 3) % 2000)}
		}
		b.Definitions = append(b.Definitions, def)
	}
	for i := range 18 {
		id := syntheticID("flask", i)
		d.Items.Bases = append(d.Items.Bases, dataset.ItemBase{ID: id, Class: "UtilityFlask", Flask: &dataset.FlaskValues{Kind: dataset.UtilityFlask}})
		b.Flasks = append(b.Flasks, dataset.FlaskBuff{Item: id, Buff: syntheticID("player_buff_definition_", i*3), Values: []int32{40, 5, 1}})
	}
	for i := range 10 {
		id := syntheticID("tincture", i)
		d.Items.Bases = append(d.Items.Bases, dataset.ItemBase{ID: id, Class: "Tincture"})
		b.Tinctures = append(b.Tinctures, dataset.Tincture{Item: id, DebuffInterval: 700, Cooldown: 8000})
	}
	d.Buffs = b
	return d
}

// BenchmarkBuffs opens a dataset and loads its buffs group, which has the row counts of the
// 3.29.3.3 game data, with the items it checks the flasks against (target: < 5 ms native).
func BenchmarkBuffs(b *testing.B) {
	fsys := written(b, syntheticBuffs())
	b.ReportAllocs()
	for b.Loop() {
		s, err := dataset.Open(fsys)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := s.Buffs(); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(len(fsys["buffs.fb"].Data))/1e3, "file-kB")
}
