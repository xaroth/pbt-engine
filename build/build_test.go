package build_test

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
)

const (
	marauder = "Metadata/Characters/Str/Str"
	ranger   = "Metadata/Characters/Dex/Dex"
)

// testStore is a dataset with two classes on one tree: Marauder starts at 1 with ascendancy
// nodes 10 and 12 (Juggernaut, from its start 20; 12 grants a point) and 11 (Berserker);
// Ranger starts at 2. Masteries 4 and 5 hang off 3 and share effect 7; 30 is reached only
// through mastery 4; 6 joins nothing. Juggernaut's 13 is a choice node with options 14 and 15.
// Proxy 40 hangs off 3, and 41 off 40 only. Bloodline Oshabi has 51 and 52 off its start 50;
// bloodline Lycia has 61 off its start 60.
func testStore(t *testing.T) *dataset.Store {
	t.Helper()
	return openStore(t, testDataset())
}

func testDataset() *dataset.Dataset {
	return &dataset.Dataset{
		Manifest:   dataset.Manifest{Game: "poe1", Version: "3.29.3.3"},
		Stats:      []dataset.Stat{{ID: "base_maximum_life"}},
		Characters: []dataset.Character{{ID: marauder}, {ID: ranger}, {ID: "Metadata/Characters/NotInTree"}},
		Trees: []dataset.Tree{{
			Variant: "standard",
			Classes: []dataset.Class{
				{Character: marauder, StartNode: 1, Ascendancies: []dataset.Ascendancy{{ID: "Juggernaut"}, {ID: "Berserker"}}},
				{Character: ranger, StartNode: 2, Ascendancies: []dataset.Ascendancy{{ID: "Deadeye"}}},
			},
			Nodes: []dataset.Node{
				{ID: 1, Kind: dataset.ClassStart, Neighbours: []uint32{3}},
				{ID: 2, Kind: dataset.ClassStart, Neighbours: []uint32{3}},
				{ID: 3, Neighbours: []uint32{1, 2, 4, 5, 40}, Stats: []dataset.StatValue{{Stat: 0, Value: 10}}},
				{ID: 4, Kind: dataset.Mastery, Neighbours: []uint32{3, 30}, MasteryEffects: []dataset.MasteryEffect{{ID: 7}, {ID: 8}}},
				{ID: 5, Kind: dataset.Mastery, Neighbours: []uint32{3}, MasteryEffects: []dataset.MasteryEffect{{ID: 7}, {ID: 9}}},
				{ID: 6},
				{ID: 10, Ascendancy: "Juggernaut", Neighbours: []uint32{20}},
				{ID: 11, Ascendancy: "Berserker"},
				{ID: 12, Ascendancy: "Juggernaut", Neighbours: []uint32{20}, SkillPoints: 1},
				{ID: 13, Ascendancy: "Juggernaut", Neighbours: []uint32{14, 15, 20}, MultipleChoice: true},
				{ID: 14, Ascendancy: "Juggernaut", Neighbours: []uint32{13}, MultipleChoiceOption: true},
				{ID: 15, Ascendancy: "Juggernaut", Neighbours: []uint32{13}, MultipleChoiceOption: true},
				{ID: 20, Kind: dataset.AscendancyStart, Ascendancy: "Juggernaut", Neighbours: []uint32{10, 12, 13}},
				{ID: 30, Neighbours: []uint32{4}},
				{ID: 40, Kind: dataset.Proxy, Neighbours: []uint32{3, 41}},
				{ID: 41, Kind: dataset.Notable, Neighbours: []uint32{40}},
				{ID: 50, Kind: dataset.AscendancyStart, Ascendancy: "Oshabi", Neighbours: []uint32{51, 52}},
				{ID: 51, Ascendancy: "Oshabi", Neighbours: []uint32{50}},
				{ID: 52, Kind: dataset.Notable, Ascendancy: "Oshabi", Neighbours: []uint32{50}},
				{ID: 60, Kind: dataset.AscendancyStart, Ascendancy: "Lycia", Neighbours: []uint32{61}},
				{ID: 61, Ascendancy: "Lycia", Neighbours: []uint32{60}},
			},
			AlternateAscendancies: []dataset.Ascendancy{{ID: "Oshabi"}, {ID: "Lycia"}},
		}},
	}
}

func openStore(t *testing.T, d *dataset.Dataset) *dataset.Store {
	t.Helper()
	dir := t.TempDir()
	if _, err := dataset.Write(dir, d); err != nil {
		t.Fatal(err)
	}
	s, err := dataset.Open(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func juggernaut() *build.Build {
	return &build.Build{
		Format: build.Format, Game: "poe1", Version: "3.29", Variant: "standard",
		Class: marauder, Ascendancy: "Juggernaut", Level: 90, Nodes: []uint32{10, 3, 1},
		Config: config.Values{"campaign": json.RawMessage(`"acts-6-10"`), "bandits": json.RawMessage(`"help-oak"`)},
	}
}

// A build encodes as its file, and the file parses back to the build; an empty optional field
// such as the bloodline is left out of the file.
func TestEncodeParse(t *testing.T) {
	t.Parallel()
	const head = `{
  "format": 1,
  "game": "poe1",
  "version": "3.29",
  "variant": "standard",
  "class": "Metadata/Characters/Str/Str",
  "ascendancy": "Juggernaut",
`
	const tail = `  "level": 90,
  "nodes": [
    1,
    3,
    10
  ],
  "config": {
    "bandits": "help-oak",
    "campaign": "acts-6-10"
  }
}
`
	testCases := []struct {
		name, bloodline, file string
	}{
		{"without a bloodline", "", head + tail},
		{"with a bloodline", "Oshabi", head + `  "bloodline": "Oshabi",` + "\n" + tail},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := juggernaut()
			b.Bloodline = testCase.bloodline
			data, err := b.Encode()
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(testCase.file, string(data)); diff != "" {
				t.Errorf("Encode (-want +got):\n%s", diff)
			}
			got, err := build.Parse([]byte(testCase.file))
			if err != nil {
				t.Fatal(err)
			}
			b.Nodes = []uint32{1, 3, 10}
			if diff := cmp.Diff(b, got); diff != "" {
				t.Errorf("Parse (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDefaults(t *testing.T) {
	t.Parallel()
	want := &build.Build{
		Format: 1, Game: "poe1", Version: "3.29", Variant: build.DefaultVariant, Class: "x", Level: 1, Nodes: []uint32{},
	}
	parsed, err := build.Parse([]byte(`{"format":1,"game":"poe1","version":"3.29","class":"x","level":1,"nodes":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, parsed); diff != "" {
		t.Errorf("Parse (-want +got):\n%s", diff)
	}
	data, err := (&build.Build{Game: "poe1", Version: "3.29", Class: "x", Level: 1}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := build.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, encoded); diff != "" {
		t.Errorf("Parse(Encode) of a build without defaults (-want +got):\n%s", diff)
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, data, want string
	}{
		{"not json", `{`, "build: unexpected EOF"},
		{"unknown field", `{"format":1,"jewels":[]}`, `build: json: unknown field "jewels"`},
		{"other format", `{"format":2}`, "build: format 2, want 1"},
		{"no format", `{"game":"poe1"}`, "build: format 0, want 1"},
		{"trailing data", `{"format":1} {}`, "build: data after the build"},
		{"trailing brace", `{"format":1}}`, "build: data after the build"},
		{"trailing bracket", `{"format":1}]`, "build: data after the build"},
		{"wrong type", `{"format":1,"level":"90"}`, "build: json: cannot unmarshal string into Go struct field Build.level"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := build.Parse([]byte(testCase.data))
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("got %v, want an error containing %q", err, testCase.want)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()
	s := testStore(t)
	b := juggernaut()
	b.Variant = ""
	c, warnings, err := build.Check(b, s)
	if err != nil || len(warnings) != 0 {
		t.Fatal(err, warnings)
	}
	ids := make([]uint32, 0, len(c.Nodes()))
	for _, n := range c.Nodes() {
		ids = append(ids, n.ID)
	}
	if diff := cmp.Diff([]uint32{1, 3, 10}, ids); diff != "" {
		t.Errorf("Nodes (-want +got):\n%s", diff)
	}
	got := []any{c.Class().StartNode, c.Character().ID, c.Tree().Variant, c.Store() == s}
	if diff := cmp.Diff([]any{uint32(1), marauder, "standard", true}, got); diff != "" {
		t.Errorf("class start, character, tree, version (-want +got):\n%s", diff)
	}
	want := *juggernaut()
	want.Nodes = []uint32{1, 3, 10}
	if diff := cmp.Diff(want, c.Build()); diff != "" {
		t.Errorf("Build (-want +got):\n%s", diff)
	}
}

// A bloodline's start and nodes are the build's own, joined to the bloodline start.
func TestCheckBloodline(t *testing.T) {
	t.Parallel()
	b := juggernaut()
	b.Bloodline = "Oshabi"
	b.Nodes = append(b.Nodes, 50, 51, 52)
	c, warnings, err := build.Check(b, testStore(t))
	if err != nil || len(warnings) != 0 {
		t.Fatal(err, warnings)
	}
	want := *b.Clone()
	want.Nodes = []uint32{1, 3, 10, 50, 51, 52}
	if diff := cmp.Diff(want, c.Build()); diff != "" {
		t.Errorf("Build (-want +got):\n%s", diff)
	}
}

func TestCheckErrors(t *testing.T) {
	t.Parallel()
	s := testStore(t)
	testCases := []struct {
		name string
		edit func(b *build.Build)
		want error
	}{
		{"game", func(b *build.Build) { b.Game = "poe2" }, build.ErrGame},
		{"variant", func(b *build.Build) { b.Variant = "ruthless" }, build.ErrVariant},
		{"class not in tree", func(b *build.Build) { b.Class = "Metadata/Characters/NotInTree" }, build.ErrClass},
		{"class unknown", func(b *build.Build) { b.Class = "x" }, build.ErrClass},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := juggernaut()
			testCase.edit(b)
			_, _, err := build.Check(b, s)
			if _, ok := errors.AsType[*build.CheckError](err); !ok || !errors.Is(err, testCase.want) {
				t.Errorf("got %v, want a *CheckError matching %v", err, testCase.want)
			}
		})
	}
}

func TestCheckWarnings(t *testing.T) {
	t.Parallel()
	s := testStore(t)
	testCases := []struct {
		name  string
		edit  func(b *build.Build)
		codes []build.Code
		check func(t *testing.T, b build.Build)
	}{
		{"ascendancy of another class", func(b *build.Build) { b.Ascendancy = "Deadeye" },
			[]build.Code{build.AscendancyNotOfClass, build.NodeOtherAscendancy},
			func(t *testing.T, b build.Build) {
				t.Helper()
				if b.Ascendancy != "" || slices.Contains(b.Nodes, 10) {
					t.Errorf("ascendancy %q, nodes %v", b.Ascendancy, b.Nodes)
				}
			}},
		{"level 0", func(b *build.Build) { b.Level = 0 }, []build.Code{build.LevelOutOfRange},
			func(t *testing.T, b build.Build) {
				t.Helper()
				if b.Level != 1 {
					t.Errorf("level %d", b.Level)
				}
			}},
		{"level 101", func(b *build.Build) { b.Level = 101 }, []build.Code{build.LevelOutOfRange},
			func(t *testing.T, b build.Build) {
				t.Helper()
				if b.Level != 100 {
					t.Errorf("level %d", b.Level)
				}
			}},
		{"unknown, repeated, other ascendancy, other class start", func(b *build.Build) { b.Nodes = append(b.Nodes, 99, 3, 11, 2) },
			[]build.Code{build.NodeOtherClassStart, build.NodeRepeated, build.NodeOtherAscendancy, build.NodeUnknown},
			func(t *testing.T, b build.Build) {
				t.Helper()
				if !slices.Equal(b.Nodes, []uint32{1, 3, 10}) {
					t.Errorf("nodes %v", b.Nodes)
				}
			}},
		{"bloodline node without the bloodline", func(b *build.Build) { b.Nodes = append(b.Nodes, 50, 51) },
			[]build.Code{build.NodeOtherAscendancy, build.NodeOtherAscendancy},
			func(t *testing.T, b build.Build) {
				t.Helper()
				if !slices.Equal(b.Nodes, []uint32{1, 3, 10}) {
					t.Errorf("nodes %v", b.Nodes)
				}
			}},
		{"node of another bloodline", func(b *build.Build) {
			b.Bloodline = "Oshabi"
			b.Nodes = append(b.Nodes, 50, 51, 61)
		}, []build.Code{build.NodeOtherAscendancy},
			func(t *testing.T, b build.Build) {
				t.Helper()
				if !slices.Equal(b.Nodes, []uint32{1, 3, 10, 50, 51}) {
					t.Errorf("nodes %v", b.Nodes)
				}
			}},
		{"unknown bloodline", func(b *build.Build) {
			b.Bloodline = "Berserker"
			b.Nodes = append(b.Nodes, 11)
		}, []build.Code{build.BloodlineUnknown, build.NodeOtherAscendancy},
			func(t *testing.T, b build.Build) {
				t.Helper()
				if b.Bloodline != "" || slices.Contains(b.Nodes, 11) {
					t.Errorf("bloodline %q, nodes %v", b.Bloodline, b.Nodes)
				}
			}},
		{"proxy", func(b *build.Build) { b.Nodes = append(b.Nodes, 40) }, []build.Code{build.NodeProxy},
			func(t *testing.T, b build.Build) {
				t.Helper()
				if slices.Contains(b.Nodes, 40) {
					t.Errorf("nodes %v", b.Nodes)
				}
			}},
		{"through a mastery", func(b *build.Build) {
			b.Nodes = append(b.Nodes, 4, 30)
			b.Masteries = map[uint32]uint32{4: 7}
		}, []build.Code{build.NodesDisconnected}, nil},
		{"two options of one choice", func(b *build.Build) { b.Nodes = append(b.Nodes, 13, 14, 15) }, []build.Code{build.ChoiceRepeated},
			func(t *testing.T, b build.Build) {
				t.Helper()
				if slices.Contains(b.Nodes, 15) || !slices.Contains(b.Nodes, 14) {
					t.Errorf("nodes %v", b.Nodes)
				}
			}},
		{"disconnected", func(b *build.Build) { b.Nodes = append(b.Nodes, 6) }, []build.Code{build.NodesDisconnected},
			func(t *testing.T, b build.Build) {
				t.Helper()
				if !slices.Contains(b.Nodes, 6) {
					t.Errorf("disconnected node dropped: %v", b.Nodes)
				}
			}},
		{"masteries", func(b *build.Build) {
			b.Nodes = append(b.Nodes, 4, 5)
			b.Masteries = map[uint32]uint32{4: 7, 5: 7, 3: 8, 6: 9}
		}, []build.Code{build.MasteryNotAllocated, build.MasteryEffectRepeated, build.MasteryNotAllocated, build.MasteryNotChosen},
			func(t *testing.T, b build.Build) {
				t.Helper()
				if len(b.Masteries) != 1 || b.Masteries[4] != 7 {
					t.Errorf("masteries %v", b.Masteries)
				}
			}},
		{"effect of another mastery", func(b *build.Build) {
			b.Nodes = append(b.Nodes, 4)
			b.Masteries = map[uint32]uint32{4: 9}
		}, []build.Code{build.MasteryEffectUnknown, build.MasteryNotChosen}, nil},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			checkWarnings(t, s, testCase.edit, testCase.codes, testCase.check)
		})
	}
}

func TestEditWarningsProxy(t *testing.T) {
	t.Parallel()
	c := checked(t, testStore(t), juggernaut())
	got := c.EditWarnings([]build.Edit{build.Allocate(40)})
	if len(got) != 1 || got[0].Code != build.NodeProxy {
		t.Errorf("warnings %v, want one %s", got, build.NodeProxy)
	}
}

// checkWarnings checks juggernaut() edited by edit: the warning codes, the checked build by
// check when not nil, and that Check left its argument alone.
func checkWarnings(t *testing.T, s *dataset.Store, edit func(*build.Build), want []build.Code, check func(*testing.T, build.Build)) {
	t.Helper()
	b := juggernaut()
	edit(b)
	original := b.Clone()
	c, warnings, err := build.Check(b, s)
	if err != nil {
		t.Fatal(err)
	}
	codes := make([]build.Code, 0, len(warnings))
	for _, w := range warnings {
		codes = append(codes, w.Code)
	}
	if diff := cmp.Diff(want, codes, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("codes (-want +got):\n%s\n%v", diff, warnings)
	}
	if check != nil {
		check(t, c.Build())
	}
	if diff := cmp.Diff(original, b); diff != "" {
		t.Errorf("Check changed its argument (-want +got):\n%s", diff)
	}
}

func TestCheckedMasteriesAndPoints(t *testing.T) {
	t.Parallel()
	b := juggernaut()
	b.Nodes = append(b.Nodes, 4, 12, 13, 14, 20)
	b.Masteries = map[uint32]uint32{4: 8}
	c, warnings, err := build.Check(b, testStore(t))
	if err != nil || len(warnings) != 0 {
		t.Fatal(err, warnings)
	}
	ms := c.Masteries()
	if len(ms) != 1 || ms[0].Node.ID != 4 || ms[0].Effect.ID != 8 {
		t.Errorf("masteries %+v", ms)
	}
	// 3 and 4 passive (1 is the start); 10, 12 and 13 ascendancy (20 is its start, option 14 is
	// free); 12 grants 1
	if p, a, g := c.Points(); p != 2 || a != 3 || g != 1 {
		t.Errorf("points: %d passive, %d ascendancy, %d granted", p, a, g)
	}
}
