package timeless_test

import (
	"cmp"
	"encoding/json"
	"maps"
	"slices"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/poe1/timeless"
)

// agnosticSocket is the socket between Templar and Witch, by The Agnostic.
const agnosticSocket = 26196

// caspiro is Chitus's second version.
var caspiro = timeless.Conqueror{Index: 3, Version: 1} //nolint:gochecknoglobals // a constant

func TestVersions(t *testing.T) {
	t.Parallel()
	got, err := timeless.New(realdata.Open(t, realdata.Current)).Versions(build.DefaultVariant)
	if err != nil {
		t.Fatal(err)
	}
	want := []timeless.JewelVersion{
		{Index: 1, ID: "Vaal", Seeds: timeless.SeedRange{First: 100, Last: 8000, Step: 1}},
		{Index: 2, ID: "Karui", Seeds: timeless.SeedRange{First: 10000, Last: 18000, Step: 1}},
		{Index: 3, ID: "Maraketh", Seeds: timeless.SeedRange{First: 500, Last: 8000, Step: 1}},
		{Index: 4, ID: "Templar", Seeds: timeless.SeedRange{First: 2000, Last: 10000, Step: 1}},
		{Index: 5, ID: "Eternal", Seeds: timeless.SeedRange{First: 2000, Last: 160000, Step: 20}},
		{Index: 6, ID: "Kalguuran", Seeds: timeless.SeedRange{First: 100, Last: 8000, Step: 1}},
	}
	if diff := gocmp.Diff(want, got); diff != "" {
		t.Errorf("versions (-want +got):\n%s", diff)
	}
}

// Conquer gives, node by node, what the seed arithmetic picks.
func TestConquerMatchesArithmetic(t *testing.T) {
	t.Parallel()
	store := realdata.Open(t, realdata.Current)
	reader := timeless.New(store)
	versions, err := reader.Versions(build.DefaultVariant)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := store.Tree(build.DefaultVariant)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := store.Stats()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range versions {
		t.Run(v.ID, func(t *testing.T) {
			t.Parallel()
			rows := &tree.TimelessVersions[v.Index]
			arithmetic, err := timeless.NewTree(tables(rows))
			if err != nil {
				t.Fatal(err)
			}
			mid := v.Seeds.First + (v.Seeds.Last-v.Seeds.First)/v.Seeds.Step/2*v.Seeds.Step
			for _, seed := range []uint32{v.Seeds.First, mid, v.Seeds.Last} {
				got, err := reader.Conquer(build.DefaultVariant, timeless.Socketed{
					Socket: agnosticSocket, Version: v.Index, Seed: seed, Conqueror: caspiro,
				}, "en")
				if err != nil {
					t.Fatal(err)
				}
				if len(got) == 0 {
					t.Fatal("no passives")
				}
				for _, p := range got {
					checkArithmetic(t, arithmetic, rows, tree, stats, seed, p)
				}
			}
		})
	}
}

// checkArithmetic checks p against the seed arithmetic's pick for its node.
func checkArithmetic(
	t *testing.T, arithmetic *timeless.Tree, rows *dataset.TimelessVersion, tree *dataset.Tree, stats []dataset.Stat,
	seed uint32, p timeless.Passive,
) {
	t.Helper()
	k, ok := kindOf(nodeOf(t, tree, p.Node), stats)
	if !ok {
		t.Fatalf("node %d has no kind", p.Node)
	}
	res := arithmetic.Conquer(timeless.Jewel{Seed: seed, Conqueror: caspiro}, timeless.Node{ID: p.Node, Kind: k})
	if diff := gocmp.Diff(expected(rows, stats, &res), p.Stats); diff != "" || p.Replaced != res.Replaced {
		t.Errorf("seed %d node %d: replaced %v, want %v; stats (-want +got):\n%s", seed, p.Node, p.Replaced, res.Replaced, diff)
	}
}

// expected is the stats of res by id.
func expected(v *dataset.TimelessVersion, stats []dataset.Stat, res *timeless.Result) []timeless.Stat {
	out := []timeless.Stat{}
	if res.Replaced {
		for s, r := range v.Replacements[res.Replacement].Stats {
			out = append(out, timeless.Stat{ID: stats[r.Stat].ID, Value: res.Rolls[s]})
		}
	}
	for _, a := range res.Additions() {
		for s, r := range v.Additions[a.Addition].Stats {
			out = append(out, timeless.Stat{ID: stats[r.Stat].ID, Value: a.Rolls[s]})
		}
	}
	return out
}

func nodeOf(tb testing.TB, tree *dataset.Tree, id uint32) *dataset.Node {
	tb.Helper()
	i, ok := slices.BinarySearchFunc(tree.Nodes, id, func(n dataset.Node, id uint32) int { return cmp.Compare(n.ID, id) })
	if !ok {
		tb.Fatalf("node %d not in the tree", id)
	}
	return &tree.Nodes[i]
}

// The jewel the game client showed on 3.30: Elegant Hubris, "Commissioned 67340 coins to
// commemorate Caspiro", in socket 26196.
func TestConquerInGameJewel(t *testing.T) {
	t.Parallel()
	reader := timeless.New(realdata.Open(t, realdata.InGame))
	got, err := reader.Conquer(build.DefaultVariant, timeless.Socketed{
		Socket: agnosticSocket, Version: eternal(t, reader), Seed: 67340, Conqueror: caspiro,
	}, "en")
	if err != nil {
		t.Fatal(err)
	}
	want := map[uint32]timeless.Passive{
		34173: {Node: 34173, Replaced: true, Name: "Slum Lord", Stats: []timeless.Stat{{ID: "minion_damage_+%", Value: 80}}},
		33435: {Node: 33435, Replaced: true, Name: "Slum Lord", Stats: []timeless.Stat{{ID: "minion_damage_+%", Value: 80}}},
		54694: {Node: 54694, Replaced: true, Name: "Eternal Resilience", Stats: []timeless.Stat{
			{ID: "gain_endurance_charge_per_second_if_have_been_hit_recently", Value: 1},
		}},
		30693: {Node: 30693, Replaced: true, Name: "Robust Diet", Stats: []timeless.Stat{{ID: "maximum_life_+%", Value: 10}}},
	}
	for _, p := range got {
		w, ok := want[p.Node]
		if !ok {
			continue
		}
		delete(want, p.Node)
		if p.Replaced != w.Replaced || p.Name != w.Name {
			t.Errorf("node %d: replaced %v by %q, want %q", p.Node, p.Replaced, p.Name, w.Name)
		}
		if !gocmp.Equal(w.Stats, p.Stats) {
			t.Errorf("node %d: stats %+v, want %+v", p.Node, p.Stats, w.Stats)
		}
	}
	if len(want) > 0 {
		t.Errorf("not in the radius: %v", slices.Sorted(maps.Keys(want)))
	}
}

// A search on the socket of the jewel the game client showed lists its seed.
func TestSearchInGameJewel(t *testing.T) {
	t.Parallel()
	reader := timeless.New(realdata.Open(t, realdata.InGame))
	version := eternal(t, reader)
	testCases := []struct {
		name      string
		allocated []uint32
	}{
		{name: "every passive in radius"},
		{name: "the two allocated", allocated: []uint32{34173, 33435}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got, err := reader.Search(build.DefaultVariant, timeless.Query{
				Socket: agnosticSocket, Version: version, Language: "en", Allocated: testCase.allocated,
				Wants: []timeless.Want{{Name: "Slum Lord", Min: 2}},
			})
			if err != nil {
				t.Fatal(err)
			}
			i := slices.IndexFunc(got, func(m timeless.Match) bool { return m.Seed == 67340 })
			if i < 0 {
				t.Fatalf("seed 67340 not among %d matches", len(got))
			}
			if got[i].Totals[0] < 2 {
				t.Errorf("seed 67340 totals %v", got[i].Totals)
			}
		})
	}
}

// A search for exactly what one seed gives ranks that seed first.
func TestSearchFindsPlantedSeed(t *testing.T) {
	t.Parallel()
	reader := timeless.New(realdata.Open(t, realdata.Current))
	const vaal, planted = 1, 4321
	passives, err := reader.Conquer(build.DefaultVariant, timeless.Socketed{Socket: agnosticSocket, Version: vaal, Seed: planted}, "en")
	if err != nil {
		t.Fatal(err)
	}
	names, totals := map[string]int64{}, map[string]int64{}
	for _, p := range passives {
		if p.Name != "" {
			names[p.Name]++
		}
		for _, s := range p.Stats {
			totals[s.ID] += int64(s.Value)
		}
	}
	wants := make([]timeless.Want, 0, len(names)+len(totals))
	for _, name := range slices.Sorted(maps.Keys(names)) {
		wants = append(wants, timeless.Want{Name: name, Min: names[name]})
	}
	for _, id := range slices.Sorted(maps.Keys(totals)) {
		wants = append(wants, timeless.Want{Stat: id, Weight: 1, Min: totals[id]})
	}
	got, err := reader.Search(build.DefaultVariant, timeless.Query{
		Socket: agnosticSocket, Version: vaal, Wants: wants, Language: "en", Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0].Seed != planted {
		seeds := make([]uint32, len(got))
		for i, m := range got {
			seeds[i] = m.Seed
		}
		t.Fatalf("matches %v, want %d first", seeds, planted)
	}
	byNode := map[uint32]timeless.Passive{}
	for _, p := range passives {
		byNode[p.Node] = p
	}
	for _, p := range got[0].Passives {
		if !gocmp.Equal(byNode[p.Node], p) {
			t.Errorf("node %d: %+v, Conquer gives %+v", p.Node, p, byNode[p.Node])
		}
	}
}

func eternal(tb testing.TB, r *timeless.Reader) int {
	tb.Helper()
	versions, err := r.Versions(build.DefaultVariant)
	if err != nil {
		tb.Fatal(err)
	}
	i := slices.IndexFunc(versions, func(v timeless.JewelVersion) bool { return v.ID == "Eternal" })
	if i < 0 {
		tb.Fatal("no Eternal version")
	}
	return versions[i].Index
}

// Matches rank by the weighted sum of their totals, from highest, then by seed.
func TestSearchRanksByScore(t *testing.T) {
	t.Parallel()
	reader := timeless.New(realdata.Open(t, realdata.InGame))
	wants := []timeless.Want{{Name: "Slum Lord", Weight: 10}, {Stat: "maximum_life_+%", Weight: 0.5}}
	got, err := reader.Search(build.DefaultVariant, timeless.Query{
		Socket: agnosticSocket, Version: eternal(t, reader), Language: "en", Wants: wants, Limit: 200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 200 {
		t.Fatalf("%d matches, want 200", len(got))
	}
	for i, m := range got {
		if want := 10*float64(m.Totals[0]) + 0.5*float64(m.Totals[1]); m.Score != want {
			t.Errorf("seed %d: score %v, want %v", m.Seed, m.Score, want)
		}
		if i > 0 && (m.Score > got[i-1].Score || m.Score == got[i-1].Score && m.Seed < got[i-1].Seed) {
			t.Errorf("seed %d (%v) ranks after seed %d (%v)", m.Seed, m.Score, got[i-1].Seed, got[i-1].Score)
		}
	}
}

// The types a host sends and reads encode with lowercase keys, nested ones included.
func TestJSONKeys(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		v    any
		want string
	}{
		{
			name: "version",
			v:    timeless.JewelVersion{Index: 1, ID: "Vaal", Seeds: timeless.SeedRange{First: 100, Last: 8000, Step: 1}},
			want: `{"index":1,"id":"Vaal","seeds":{"first":100,"last":8000,"step":1}}`,
		},
		{
			name: "socketed",
			v:    timeless.Socketed{Socket: 26196, Version: 5, Seed: 67340, Conqueror: caspiro},
			want: `{"socket":26196,"version":5,"seed":67340,"conqueror":{"index":3,"version":1}}`,
		},
		{
			name: "query",
			v:    timeless.Query{Socket: 26196, Version: 5, Wants: []timeless.Want{{Name: "Slum Lord", Min: 2}}, Language: "en"},
			want: `{"socket":26196,"version":5,"wants":[{"name":"Slum Lord","stat":"","weight":0,"min":2}],"allocated":null,"language":"en","limit":0}`,
		},
		{
			name: "match",
			v: timeless.Match{Seed: 67340, Score: 2, Totals: []int64{2}, Passives: []timeless.Passive{
				{Node: 34173, Replaced: true, Name: "Slum Lord", Stats: []timeless.Stat{{ID: "minion_damage_+%", Value: 80}}},
			}},
			want: `{"seed":67340,"score":2,"totals":[2],"passives":[{"node":34173,"replaced":true,"name":"Slum Lord","stats":[{"id":"minion_damage_+%","value":80}]}]}`,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got, err := json.Marshal(testCase.v)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != testCase.want {
				t.Errorf("got  %s\nwant %s", got, testCase.want)
			}
		})
	}
}
