package items_test

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/items"
	"github.com/xaroth/pbt-engine/statdesc"
)

// TestRoundTrip writes mods of the current real dataset as item text and reads it back: every
// rolling prefix and suffix and every eldritch implicit on the first base it spawns on, and
// every bench craft as a {crafted} line on the first base a prefix or suffix of its stat spawns
// on, each at its low and high values. A line is wrong when it reads as a stat the mod does not
// have; warned when it gets a warning other than item-roll-assumed.
func TestRoundTrip(t *testing.T) {
	t.Parallel()
	s := realdata.Open(t, realdata.Current)
	r, err := items.NewReader(s)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][3]int{
		"affix":    {6948, 0, 0},
		"eldritch": {6720, 0, 0},
		"crafted":  {1890, 0, 0},
	}
	for set, cases := range newGenerator(t, s).sets() {
		t.Run(set, func(t *testing.T) {
			t.Parallel()
			var wrong, warned int
			for _, c := range cases {
				got, codes := readBack(t, r, c)
				bad := slices.ContainsFunc(got, func(st string) bool { return !slices.Contains(c.stats, st) })
				if bad {
					wrong++
				}
				if len(codes) > 0 {
					warned++
				}
				if bad || len(codes) > 0 {
					t.Logf("%s: %q read as %v %v", c.mod, c.text, got, codes)
				}
			}
			if got := [3]int{len(cases), wrong, warned}; got != want[set] {
				t.Errorf("lines, wrong, warned: got %v, want %v", got, want[set])
			}
		})
	}
}

// roundTrip is an item of one generated mod: its text, and the stats the mod has.
type roundTrip struct {
	mod, text string
	stats     []string
}

// readBack parses c and returns the stats its lines read as and its warning codes other than
// item-roll-assumed.
func readBack(t *testing.T, r *items.Reader, c roundTrip) (stats, codes []string) {
	t.Helper()
	it, warnings, err := r.Parse(c.text)
	if err != nil {
		t.Fatalf("%s: %v", c.mod, err)
	}
	for _, m := range it.Mods {
		for _, sv := range m.Stats {
			stats = append(stats, sv.Stat)
		}
	}
	for _, w := range warnings {
		if w.Code != items.RollAssumed {
			codes = append(codes, string(w.Code))
		}
	}
	return stats, codes
}

// generator writes mods of a dataset as item text.
type generator struct {
	set   *statdesc.Set
	mods  []dataset.Mod
	stats []dataset.Stat
	bases []*dataset.ItemBase
	names map[string]string
	// tagSets are each base's tags, alone and with those each influence flag gives.
	tagSets map[*dataset.ItemBase][]influenced
	// rolling are the prefixes and suffixes of the item domain by their first stat.
	rolling map[uint32][]*dataset.Mod
}

// influenced is a base's tags with those an influence flag gives.
type influenced struct {
	flag string
	tags []string
}

func newGenerator(t *testing.T, s *dataset.Store) *generator {
	t.Helper()
	it, errItems := s.Items()
	text, errText := s.Text("en")
	mods, errMods := s.Mods()
	stats, errStats := s.Stats()
	if err := errors.Join(errItems, errText, errMods, errStats); err != nil {
		t.Fatal(err)
	}
	set, err := statdesc.NewSet(text.Descriptions)
	if err != nil {
		t.Fatal(err)
	}
	g := &generator{
		set: set, mods: mods, stats: stats,
		tagSets: map[*dataset.ItemBase][]influenced{}, rolling: map[uint32][]*dataset.Mod{},
	}
	g.names = baseNames(text.BaseNames)
	for i := range it.Bases {
		if g.names[it.Bases[i].ID] != "" {
			g.bases = append(g.bases, &it.Bases[i])
			g.tagSets[&it.Bases[i]] = withInfluences(it.Bases[i].Tags)
		}
	}
	for i := range g.mods {
		if m := &g.mods[i]; isAffix(m) && m.Domain == 1 {
			g.rolling[m.Stats[0].Stat] = append(g.rolling[m.Stats[0].Stat], m)
		}
	}
	return g
}

// baseNames maps base ids to their first non-empty English name.
func baseNames(names []dataset.Name) map[string]string {
	out := map[string]string{}
	for _, n := range names {
		if _, ok := out[n.ID]; !ok && n.Name != "" {
			out[n.ID] = n.Name
		}
	}
	return out
}

// withInfluences returns tags alone, then with the tags each influence flag gives.
func withInfluences(tags []string) []influenced {
	flags := map[string]string{
		"shaper": "Shaper Item", "elder": "Elder Item", "crusader": "Crusader Item",
		"eyrie": "Redeemer Item", "basilisk": "Hunter Item", "adjudicator": "Warlord Item",
	}
	out := make([]influenced, 0, 1+len(flags))
	out = append(out, influenced{tags: tags})
	for _, suffix := range slices.Sorted(maps.Keys(flags)) {
		more := slices.Clone(tags)
		for _, tag := range tags {
			more = append(more, tag+"_"+suffix)
		}
		out = append(out, influenced{flag: flags[suffix], tags: more})
	}
	return out
}

const (
	rtRuthless = 2
	rtCrafted  = 9
	rtExarch   = 28
	rtEater    = 29
)

func isAffix(m *dataset.Mod) bool {
	return (m.Generation == 1 || m.Generation == 2) && m.GameMode != rtRuthless && len(m.Stats) > 0
}

// rtDomain returns the mod domain of items of class, as the reader takes it.
func rtDomain(class string) int32 {
	switch class {
	case "LifeFlask", "ManaFlask", "HybridFlask", "UtilityFlask":
		return 2
	case "Jewel":
		return 10
	case "AbyssJewel":
		return 13
	case "Tincture":
		return 34
	}
	return 1
}

// sets returns the generated items by set.
func (g *generator) sets() map[string][]roundTrip {
	out := map[string][]roundTrip{}
	for i := range g.mods {
		m := &g.mods[i]
		set, b, flag := g.place(m)
		if b == nil {
			continue
		}
		for _, high := range []bool{false, true} {
			if lines := g.render(m, high); len(lines) > 0 {
				out[set] = append(out[set], roundTrip{mod: m.ID, text: g.text(set, b, flag, lines), stats: g.statIDs(m)})
			}
		}
	}
	return out
}

// place returns m's set, the base it goes on and the influence flag it needs, or no base.
func (g *generator) place(m *dataset.Mod) (string, *dataset.ItemBase, string) {
	if m.GameMode == rtRuthless || len(m.Stats) == 0 {
		return "", nil, ""
	}
	switch {
	case m.Domain == rtCrafted:
		return "crafted", g.craftBase(m), ""
	case isAffix(m):
		b, flag := g.spawnBase(m)
		return "affix", b, flag
	case m.Domain == 1 && (m.Generation == rtExarch || m.Generation == rtEater):
		b, flag := g.spawnBase(m)
		return "eldritch", b, flag
	}
	return "", nil, ""
}

// text is a rare item of base b with lines: implicit for an eldritch mod, crafted for a craft.
func (g *generator) text(set string, b *dataset.ItemBase, flag string, lines []string) string {
	head := "Rarity: RARE\nX\n" + g.names[b.ID] + "\n"
	if flag != "" {
		head += flag + "\n"
	}
	implicits, prefix := 0, ""
	switch set {
	case "eldritch":
		implicits = len(lines)
	case "crafted":
		prefix = "{crafted}"
	}
	return fmt.Sprintf("%sImplicits: %d\n%s%s\n", head, implicits, prefix, strings.Join(lines, "\n"+prefix))
}

// spawnBase returns the first base m spawns on, with the influence flag it needs.
func (g *generator) spawnBase(m *dataset.Mod) (*dataset.ItemBase, string) {
	for _, b := range g.bases {
		if rtDomain(b.Class) != m.Domain {
			continue
		}
		for _, x := range g.tagSets[b] {
			if rtSpawns(m, x.tags) {
				return b, x.flag
			}
		}
	}
	return nil, ""
}

// craftBase returns the first base a prefix or suffix of m's first stat spawns on: the dataset
// does not say which items a bench craft applies to.
func (g *generator) craftBase(m *dataset.Mod) *dataset.ItemBase {
	for _, b := range g.bases {
		if rtDomain(b.Class) == 1 && slices.ContainsFunc(g.rolling[m.Stats[0].Stat], func(x *dataset.Mod) bool { return rtSpawns(x, b.Tags) }) {
			return b
		}
	}
	return nil
}

func rtSpawns(m *dataset.Mod, tags []string) bool {
	for _, w := range m.SpawnWeights {
		if slices.Contains(tags, w.Tag) {
			return w.Weight > 0
		}
	}
	return false
}

// render returns m's text lines at its low or high values.
func (g *generator) render(m *dataset.Mod, high bool) []string {
	vals := map[string]int32{}
	for _, st := range m.Stats {
		vals[g.stats[st.Stat].ID] = min(st.Min, st.Max)
		if high {
			vals[g.stats[st.Stat].ID] = max(st.Min, st.Max)
		}
	}
	ls, err := g.set.Describe(mainScope, vals)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range ls {
		out = append(out, strings.Split(l.Text, "\n")...)
	}
	return out
}

func (g *generator) statIDs(m *dataset.Mod) []string {
	out := make([]string, 0, len(m.Stats))
	for _, st := range m.Stats {
		out = append(out, g.stats[st.Stat].ID)
	}
	return out
}
