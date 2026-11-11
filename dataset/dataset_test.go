package dataset_test

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	flatbuffers "github.com/google/flatbuffers/go"
	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/dataset/internal/fb"
	"github.com/xaroth/pbt-engine/statdesc"
)

func sample() *dataset.Dataset {
	return &dataset.Dataset{
		Manifest: dataset.Manifest{
			Game:    "poe1",
			Version: "3.29.3.3",
			Source:  map[string]string{"steamManifest": "1", "datSchemaCommit": "abc"},
		},
		Stats: []dataset.Stat{{ID: "level"}, {ID: "life_per_level"}, {ID: "base_maximum_all_resistances_%"}},
		Characters: []dataset.Character{
			{
				ID:           "Metadata/Characters/Str/Str",
				BaseStrength: 32, BaseDexterity: 14, BaseIntelligence: 14, BaseMaxLife: 38, BaseMaxMana: 34,
				Stats: []dataset.StatValue{{Stat: 0, Value: 1}, {Stat: 1, Value: 12}, {Stat: 2, Value: -75}},
			},
			{ID: "Metadata/Characters/Dex/Dex"},
		},
		Constants: []dataset.Constant{{ID: "life_per_strength", Value: 1, Per: 2}},
		Quests: []dataset.Quest{
			{Quest: "a2q7", Flag: "a2q7-QuestRewardTaken", Act: 2, PassivePoints: 1},
			{Quest: "a5q6", Flag: "a5q6-CompletedBoss", Act: 5, Stats: []dataset.StatValue{{Stat: 2, Value: -30}}},
			{Flag: "LabyrinthComplete1", AscendancyPoints: 2},
		},
		Trees: []dataset.Tree{
			{
				Variant: "standard", TotalPoints: 123, AscendancyPoints: 8,
				Classes: []dataset.Class{{
					Character: "Metadata/Characters/Str/Str", BaseStrength: 32, BaseDexterity: 14, BaseIntelligence: 14, StartNode: 1,
					Ascendancies: []dataset.Ascendancy{{ID: "Juggernaut"}},
				}},
				AlternateAscendancies: []dataset.Ascendancy{{ID: "Bloodline"}},
				Nodes: []dataset.Node{
					{ID: 1, Kind: dataset.ClassStart, Neighbours: []uint32{2}},
					{
						ID: 2, Kind: dataset.Notable, Ascendancy: "Juggernaut",
						Stats:          []dataset.StatValue{{Stat: 1, Value: 10}},
						Buffs:          [][]dataset.StatValue{{{Stat: 2, Value: -5}, {Stat: 0, Value: 1}}},
						SkillPoints:    1,
						MasteryEffects: []dataset.MasteryEffect{{ID: 7, Stats: []dataset.StatValue{{Stat: 0, Value: 3}}}},
						Neighbours:     []uint32{1},
						MultipleChoice: true, MultipleChoiceOption: true,
					},
				},
				JewelSlots: []uint32{2},
			},
			{Variant: "ruthless"},
		},
		Texts: sampleTexts(),
	}
}

func sampleTexts() []dataset.Text {
	return []dataset.Text{
		{
			Language: "en",
			Descriptions: []statdesc.Scope{
				{Name: "base", Descriptions: []statdesc.Description{
					{Stats: []string{"level"}},
					{Stats: []string{"life_per_level"}, Variants: []statdesc.Variant{{
						Limits:   []statdesc.Limit{{Min: math.MinInt32, Max: -1}},
						Text:     "{0} life",
						Handlers: []statdesc.Handler{{Name: "negate"}},
					}}},
				}},
				{Name: "child", Parent: "base"},
			},
			NodeNames:       []dataset.NodeName{{ID: 1, Name: "MARAUDER"}, {ID: 2, Name: "Strength"}},
			CharacterNames:  []dataset.Name{{ID: "Metadata/Characters/Str/Str", Name: "Marauder"}},
			AscendancyNames: []dataset.AscendancyName{{Variant: "standard", ID: "Juggernaut", Name: "Juggernaut"}},
		},
		{Language: "zh-hant"},
	}
}

// readAll opens the dataset in fsys and reads every group, in manifest order.
func readAll(fsys fs.FS) (*dataset.Dataset, error) {
	s, err := dataset.Open(fsys)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	d := &dataset.Dataset{Manifest: s.Manifest()}
	if d.Stats, err = s.Stats(); err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if d.Characters, err = s.Characters(); err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if d.Constants, err = s.Constants(); err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if d.Quests, err = s.Quests(); err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	for _, v := range s.Manifest().Trees() {
		t, err := s.Tree(v)
		if err != nil {
			return nil, fmt.Errorf("read: %w", err)
		}
		d.Trees = append(d.Trees, *t)
	}
	for _, l := range s.Manifest().Languages() {
		t, err := s.Text(l)
		if err != nil {
			return nil, fmt.Errorf("read: %w", err)
		}
		d.Texts = append(d.Texts, *t)
	}
	return d, errors.Join(readItems(s, d), readSkills(s, d))
}

// readSkills reads s's skills group into d, or nothing when s has none.
func readSkills(s *dataset.Store, d *dataset.Dataset) error {
	sk, err := s.Skills()
	switch {
	case errors.Is(err, dataset.ErrNotInDataset):
	case err != nil:
		return fmt.Errorf("read: %w", err)
	default:
		d.Skills = sk
	}
	return nil
}

// readItems reads s's items group into d, or nothing when s has none.
func readItems(s *dataset.Store, d *dataset.Dataset) error {
	var err error
	if d.Items, err = s.Items(); errors.Is(err, dataset.ErrNotInDataset) {
		return nil
	} else if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if d.Mods, err = s.Mods(); err != nil {
		return fmt.Errorf("read: %w", err)
	}
	return nil
}

// written writes d and returns the dataset files as a MapFS.
func written(tb testing.TB, d *dataset.Dataset) fstest.MapFS {
	tb.Helper()
	dir := tb.TempDir()
	if _, err := dataset.Write(dir, d); err != nil {
		tb.Fatalf("Write: %v", err)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		tb.Fatal(err)
	}
	fsys := fstest.MapFS{}
	for _, e := range ents {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			tb.Fatal(err)
		}
		fsys[e.Name()] = &fstest.MapFile{Data: b}
	}
	return fsys
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		d    *dataset.Dataset
	}{
		{name: "sample", d: sample()},
		{name: "empty", d: &dataset.Dataset{}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			m, err := dataset.Write(dir, testCase.d)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			got, err := readAll(os.DirFS(dir))
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			want := *testCase.d
			want.Manifest = m
			if diff := gocmp.Diff(&want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("round trip mismatch (-want +got):\n%s", diff)
			}
			if wantFiles := 4 + len(testCase.d.Trees) + len(testCase.d.Texts); m.Format != dataset.FormatVersion || len(m.Files) != wantFiles {
				t.Errorf("manifest: got format %d and %d files, want %d and %d", m.Format, len(m.Files), dataset.FormatVersion, wantFiles)
			}
		})
	}
}

func TestWriteIsDeterministic(t *testing.T) {
	t.Parallel()
	a, b := written(t, sample()), written(t, sample())
	if len(a) != len(b) {
		t.Fatalf("got %d and %d files, want equal", len(a), len(b))
	}
	for name, f := range a {
		if !bytes.Equal(f.Data, b[name].Data) {
			t.Errorf("%s: bytes differ between two writes", name)
		}
	}
}

// renumbered returns sample with a stat added at the front and the stats in reverse order, as a
// later game version might number them.
func renumbered() *dataset.Dataset {
	d := sample()
	old := uint32(len(d.Stats))
	stats := []dataset.Stat{{ID: "new_stat"}}
	for _, s := range slices.Backward(d.Stats) {
		stats = append(stats, s)
	}
	d.Stats = stats
	remap := func(svs []dataset.StatValue, sorted bool) {
		for i := range svs {
			svs[i].Stat = old - svs[i].Stat
		}
		if sorted {
			slices.SortFunc(svs, func(a, b dataset.StatValue) int { return cmp.Compare(a.Stat, b.Stat) })
		}
	}
	for _, c := range d.Characters {
		remap(c.Stats, true)
	}
	for _, q := range d.Quests {
		remap(q.Stats, true)
	}
	for _, n := range d.Trees[0].Nodes {
		remap(n.Stats, true)
		for _, b := range n.Buffs {
			remap(b, false)
		}
		for _, e := range n.MasteryEffects {
			remap(e.Stats, false)
		}
	}
	return d
}

func TestFilesIndependentOfStatOrder(t *testing.T) {
	t.Parallel()
	a, b := written(t, sample()), written(t, renumbered())
	for name := range a {
		same := bytes.Equal(a[name].Data, b[name].Data)
		if want := name != "stats.fb" && name != dataset.ManifestName; same != want {
			t.Errorf("%s: got equal bytes %v, want %v", name, same, want)
		}
	}
	got, err := readAll(b)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want := renumbered()
	want.Manifest = got.Manifest
	if diff := gocmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("round trip mismatch (-want +got):\n%s", diff)
	}
}

func TestManifestID(t *testing.T) {
	t.Parallel()
	id := func(d *dataset.Dataset) string {
		m, err := dataset.Write(t.TempDir(), d)
		if err != nil {
			t.Fatalf("Write: %v", err)
		}
		return m.ID
	}
	base := id(sample())
	if len(base) != 64 || base != id(sample()) {
		t.Errorf("ID: got %q, want 64 hex digits, equal for equal content", base)
	}
	changed := sample()
	changed.Trees[0].Nodes[1].Stats[0].Value++
	source := sample()
	source.Manifest.Source["datSchemaCommit"] = "def"
	if id(changed) == base {
		t.Errorf("ID: unchanged after a node stat changed")
	}
	if id(source) != base {
		t.Errorf("ID: changed with only the source map changed")
	}
}

// checkError fails t unless err contains want and, when target is set, matches it with
// errors.Is.
func checkError(t *testing.T, err, target error, want string) {
	t.Helper()
	switch {
	case err == nil:
		t.Errorf("got nil error, want %q", want)
	case target != nil && !errors.Is(err, target):
		t.Errorf("got %v, want errors.Is %v", err, target)
	case !strings.Contains(err.Error(), want):
		t.Errorf("got %v, want an error containing %q", err, want)
	}
}

func TestWriteErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, want string
		stats      []dataset.StatValue
	}{
		{name: "unsorted", want: "stats not sorted or repeated", stats: []dataset.StatValue{{Stat: 1}, {Stat: 0}}},
		{name: "repeated", want: "stats not sorted or repeated", stats: []dataset.StatValue{{Stat: 1}, {Stat: 1}}},
		{name: "out of range", want: "stat 3 out of range", stats: []dataset.StatValue{{Stat: 3}}},
	}
	treeCases := []struct {
		name, want string
		change     func(d *dataset.Dataset)
	}{
		{name: "unsorted nodes", want: "nodes not sorted", change: func(d *dataset.Dataset) {
			n := d.Trees[0].Nodes
			n[0], n[1] = n[1], n[0]
		}},
		{name: "repeated node", want: "node 1 repeated", change: func(d *dataset.Dataset) { d.Trees[0].Nodes[1].ID = 1 }},
		{name: "node stat out of range", want: "node 2: stat 9 out of range", change: func(d *dataset.Dataset) { d.Trees[0].Nodes[1].Stats[0].Stat = 9 }},
		{name: "buff stat out of range", want: "node 2 buff: stat 9 out of range", change: func(d *dataset.Dataset) { d.Trees[0].Nodes[1].Buffs[0][0].Stat = 9 }},
		{
			name: "mastery stat out of range", want: "node 2 mastery effect: stat 9 out of range",
			change: func(d *dataset.Dataset) { d.Trees[0].Nodes[1].MasteryEffects[0].Stats[0].Stat = 9 },
		},
		{name: "unknown neighbour", want: "neighbour 5 not in tree", change: func(d *dataset.Dataset) { d.Trees[0].Nodes[0].Neighbours = []uint32{5} }},
		{name: "unsorted neighbours", want: "neighbours not sorted", change: func(d *dataset.Dataset) { d.Trees[0].Nodes[0].Neighbours = []uint32{2, 1} }},
		{name: "unknown start node", want: "start node 5 not in tree", change: func(d *dataset.Dataset) { d.Trees[0].Classes[0].StartNode = 5 }},
		{name: "unknown jewel slot", want: "jewel slot 5 not in tree", change: func(d *dataset.Dataset) { d.Trees[0].JewelSlots = []uint32{5} }},
		{name: "repeated variant", want: `tree variant "standard"`, change: func(d *dataset.Dataset) { d.Trees[1].Variant = "standard" }},
		{name: "empty variant", want: `tree variant ""`, change: func(d *dataset.Dataset) { d.Trees[1].Variant = "" }},
		{name: "constant per zero", want: "per is 0", change: func(d *dataset.Dataset) { d.Constants[0].Per = 0 }},
		{name: "quest flag repeated", want: "empty or repeated", change: func(d *dataset.Dataset) { d.Quests[1].Flag = d.Quests[0].Flag }},
		{name: "quest flag empty", want: "empty or repeated", change: func(d *dataset.Dataset) { d.Quests[0].Flag = "" }},
		{name: "quest stats unsorted", want: "not sorted", change: func(d *dataset.Dataset) {
			d.Quests[1].Stats = []dataset.StatValue{{Stat: 2, Value: 1}, {Stat: 0, Value: 1}}
		}},
		{name: "repeated stat id", want: "stat level repeated", change: func(d *dataset.Dataset) { d.Stats[1].ID = "level" }},
		{name: "empty stat id", want: "stat 1: empty id", change: func(d *dataset.Dataset) { d.Stats[1].ID = "" }},
		{name: "description parent missing", want: "parent gone not in set", change: func(d *dataset.Dataset) { d.Texts[0].Descriptions[1].Parent = "gone" }},
		{name: "description with too many stats", want: "257 stats in one description", change: func(d *dataset.Dataset) {
			sc := &d.Texts[0].Descriptions[0].Descriptions[0]
			sc.Stats = make([]string, 257)
			for i := range sc.Stats {
				sc.Stats[i] = strconv.Itoa(i)
			}
		}},
		{name: "variant not a tag", want: `tree variant "../x"`, change: func(d *dataset.Dataset) { d.Trees[1].Variant = "../x" }},
		{name: "language not a tag", want: `text language "EN"`, change: func(d *dataset.Dataset) { d.Texts[1].Language = "EN" }},
		{name: "repeated language", want: `text language "en"`, change: func(d *dataset.Dataset) { d.Texts[1].Language = "en" }},
		{name: "unknown class character", want: "class character x not in characters", change: func(d *dataset.Dataset) { d.Trees[0].Classes[0].Character = "x" }},
		{name: "unsorted node names", want: "node names not sorted", change: func(d *dataset.Dataset) {
			n := d.Texts[0].NodeNames
			n[0], n[1] = n[1], n[0]
		}},
		{name: "name for unknown node", want: "name for unknown node 9", change: func(d *dataset.Dataset) { d.Texts[0].NodeNames[1].ID = 9 }},
		{name: "name for unknown character", want: "name for unknown character x", change: func(d *dataset.Dataset) { d.Texts[0].CharacterNames[0].ID = "x" }},
		{
			name: "name for unknown ascendancy", want: "name for unknown ascendancy Juggernaut in tree ruthless",
			change: func(d *dataset.Dataset) { d.Texts[0].AscendancyNames[0].Variant = "ruthless" },
		},
	}
	for _, testCase := range treeCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			d := sample()
			testCase.change(d)
			_, err := dataset.Write(t.TempDir(), d)
			checkError(t, err, nil, testCase.want)
		})
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			d := sample()
			d.Characters[0].Stats = testCase.stats
			_, err := dataset.Write(t.TempDir(), d)
			checkError(t, err, nil, testCase.want)
		})
	}
}

// charactersFile builds a characters file with one character whose stats use the given file
// indexes, for layouts Write never produces.
func charactersFile(ids []string, stats []uint32) []byte {
	b := flatbuffers.NewBuilder(0)
	idOffs := make([]flatbuffers.UOffsetT, len(ids))
	for i, id := range ids {
		idOffs[i] = b.CreateString(id)
	}
	name := b.CreateString("Metadata/Characters/Str/Str")
	fb.CharacterStartStatsVector(b, len(stats))
	for _, s := range slices.Backward(stats) {
		fb.CreateStatValue(b, s, 1)
	}
	statVec := b.EndVector(len(stats))
	fb.CharacterStart(b)
	fb.CharacterAddId(b, name)
	fb.CharacterAddStats(b, statVec)
	char := fb.CharacterEnd(b)
	fb.CharactersStartCharactersVector(b, 1)
	b.PrependUOffsetT(char)
	chars := b.EndVector(1)
	fb.CharactersStartStatIdsVector(b, len(ids))
	for _, o := range slices.Backward(idOffs) {
		b.PrependUOffsetT(o)
	}
	idVec := b.EndVector(len(ids))
	fb.CharactersStart(b)
	fb.CharactersAddCharacters(b, chars)
	fb.CharactersAddStatIds(b, idVec)
	fb.FinishCharactersBuffer(b, fb.CharactersEnd(b))
	return b.FinishedBytes()
}

func TestReadErrors(t *testing.T) {
	t.Parallel()
	good := written(t, sample())

	// edit returns a copy of good with the manifest changed by f. The id is recomputed to
	// match, unless f sets it, so each case fails on its own change.
	edit := func(f func(m map[string]any)) fstest.MapFS {
		out := fstest.MapFS{}
		for k, v := range good {
			out[k] = &fstest.MapFile{Data: bytes.Clone(v.Data)}
		}
		var m map[string]any
		if err := json.Unmarshal(good[dataset.ManifestName].Data, &m); err != nil {
			t.Fatal(err)
		}
		delete(m, "id")
		f(m)
		if _, ok := m["id"]; !ok {
			b, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			var parsed dataset.Manifest
			if err := json.Unmarshal(b, &parsed); err != nil {
				t.Fatal(err)
			}
			m["id"] = dataset.ContentID(parsed)
		}
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		out[dataset.ManifestName] = &fstest.MapFile{Data: b}
		return out
	}
	files := func(m map[string]any) []any { return m["files"].([]any) }
	// replace sets file i's content to b and fixes its manifest entry.
	replace := func(i int, b []byte) fstest.MapFS {
		var name string
		fsys := edit(func(m map[string]any) {
			f := files(m)[i].(map[string]any)
			sum := sha256.Sum256(b)
			f["size"], f["sha256"] = len(b), hex.EncodeToString(sum[:])
			name = f["name"].(string)
		})
		fsys[name] = &fstest.MapFile{Data: b}
		return fsys
	}
	oneStat := &dataset.Dataset{Stats: sample().Stats[:1]}
	oneStatBytes := written(t, oneStat)["stats.fb"].Data
	charsBytes := good["characters.fb"].Data

	testCases := []struct {
		name, want string
		is         error // when set, err must match it with errors.Is
		fsys       fstest.MapFS
	}{
		{name: "no manifest", want: "open manifest.json", is: fs.ErrNotExist, fsys: fstest.MapFS{}},
		{name: "manifest not json", want: "manifest.json: unexpected EOF", is: dataset.ErrInvalid, fsys: fstest.MapFS{dataset.ManifestName: {Data: []byte("{")}}},
		{name: "unknown manifest field", want: `unknown field "extra"`, fsys: edit(func(m map[string]any) { m["extra"] = 1 })},
		{name: "other format", want: "dataset format 2, want 1", fsys: edit(func(m map[string]any) { m["format"] = dataset.FormatVersion + 1 })},
		{name: "unknown file", want: "unknown file x.fb", fsys: edit(func(m map[string]any) { files(m)[0].(map[string]any)["name"] = "x.fb" })},
		{name: "missing group", want: "missing file characters.fb", fsys: edit(func(m map[string]any) { m["files"] = files(m)[:1] })},
		{name: "file absent", want: "open stats.fb", is: fs.ErrNotExist, fsys: func() fstest.MapFS {
			f := edit(func(map[string]any) {})
			delete(f, "stats.fb")
			return f
		}()},
		{name: "hash differs", want: "stats.fb: size or sha256 differs", fsys: edit(func(m map[string]any) { files(m)[0].(map[string]any)["sha256"] = "00" })},
		{name: "wrong identifier", want: "stats.fb: wrong file identifier", is: dataset.ErrInvalid, fsys: replace(0, charsBytes)},
		{name: "truncated", want: "characters.fb: malformed", is: dataset.ErrInvalid, fsys: replace(1, charsBytes[:len(charsBytes)/2])},
		{name: "stats too few for characters", want: `characters.fb: unknown stat "base_maximum_all_resistances_%"`, fsys: replace(0, oneStatBytes)},
		{
			name: "stat ids unsorted", want: `characters.fb: stat ids not sorted and unique at "level"`,
			fsys: replace(1, charactersFile([]string{"life_per_level", "level"}, []uint32{0})),
		},
		{
			name: "stat id repeated", want: `characters.fb: stat ids not sorted and unique at "level"`,
			fsys: replace(1, charactersFile([]string{"level", "level"}, []uint32{0, 1})),
		},
		{
			name: "stat index out of range", want: "characters.fb: character Metadata/Characters/Str/Str: stat 2 out of range (2 ids)",
			fsys: replace(1, charactersFile([]string{"level", "life_per_level"}, []uint32{2})),
		},
		{
			name: "capability at default version", want: `capability "armour": version 1`, is: dataset.ErrInvalid,
			fsys: edit(func(m map[string]any) { m["capabilities"] = map[string]any{"armour": 1} }),
		},
		{name: "id differs", want: "id does not match", fsys: edit(func(m map[string]any) { m["id"] = "00" })},
		{name: "file listed twice", want: "file stats.fb listed twice", fsys: edit(func(m map[string]any) { m["files"] = append(files(m), files(m)[0]) })},
		{name: "truncated tree", want: "tree-standard.fb: malformed", fsys: replace(4, good["tree-standard.fb"].Data[:40])},
		{name: "truncated text", want: "text-en.fb: malformed", fsys: replace(6, good["text-en.fb"].Data[:40])},
		{name: "text not a text file", want: "text-en.fb: wrong file identifier", fsys: replace(6, good["constants.fb"].Data)},
		{name: "tree of another variant", want: `tree-standard.fb: holds tree variant "ruthless"`, fsys: replace(4, good["tree-ruthless.fb"].Data)},
		{name: "text of another language", want: `text-en.fb: holds text language "zh-hant"`, fsys: replace(6, good["text-zh-hant.fb"].Data)},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := readAll(testCase.fsys)
			checkError(t, err, testCase.is, testCase.want)
		})
	}
}

func TestReadTrees(t *testing.T) {
	t.Parallel()
	fsys := written(t, sample())
	m, err := dataset.ReadManifest(fsys)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if diff := gocmp.Diff([]string{"standard", "ruthless"}, m.Trees()); diff != "" {
		t.Errorf("Manifest.Trees (-want +got):\n%s", diff)
	}
	if diff := gocmp.Diff([]string{"en", "zh-hant"}, m.Languages()); diff != "" {
		t.Errorf("Manifest.Languages (-want +got):\n%s", diff)
	}
}
