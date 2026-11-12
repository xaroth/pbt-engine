package items_test

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/items"
	"github.com/xaroth/pbt-engine/statdesc"
)

// TestRoundTrip writes mods of the current real dataset as item text at their low and high
// values and reads it back:
//   - affix: every rolling prefix and suffix, on the first base it spawns on;
//   - eldritch: every eldritch implicit, as an implicit line;
//   - corrupted: every corruption implicit, as an implicit line of a corrupted item;
//   - enchant: every labyrinth enchantment that spawns, as an {enchant} line;
//   - given: every essence-type, Delve and elevated influence mod;
//   - crafted and unmarked craft: every bench craft, with and without its {crafted} marker.
//
// The dataset does not say which items a bench craft or an essence applies to, so those go on
// the first base a prefix or suffix of their stat spawns on. A line is wrong when it reads as a
// stat the mod does not have, or a value its text shows otherwise, unless a prefix or suffix
// rolling on the item gives that stat and value: an older mod whose current version has a new
// stat reads as the current one (counted as "current"). It is warned when it gets a warning
// other than item-roll-assumed.
func TestRoundTrip(t *testing.T) {
	t.Parallel()
	s := realdata.Open(t, realdata.Current)
	r, err := items.NewReader(s)
	if err != nil {
		t.Fatal(err)
	}
	g := newGenerator(t, s)
	// lines, wrong, warned, current. Warned in affix: curses on hit whose level the text does
	// not show, where two tiers of an influence mod differ on it; in enchant, the same for Chain
	// Hook's rage chance. Wrong in given: older elevated ("Uber", "Maven") mods whose text a mod
	// of another stat also reads, and hybrid local defence mods on shields, whose second line
	// reads as a global craft until the dataset says which items a craft applies to.
	want := map[string][4]int{
		"affix":          {6948, 0, 12, 0},
		"eldritch":       {6720, 0, 0, 0},
		"corrupted":      {476, 0, 0, 0},
		"enchant":        {2922, 0, 4, 0},
		"given":          {2066, 38, 19, 35},
		"crafted":        {1890, 0, 0, 0},
		"unmarked craft": {1890, 0, 0, 0},
	}
	for set, cases := range g.sets() {
		t.Run(set, func(t *testing.T) {
			t.Parallel()
			var wrong, warned, current int
			for _, c := range cases {
				got, codes := readBack(t, r, c)
				bad := slices.ContainsFunc(got, func(sv build.StatValue) bool { return !g.matches(c.values, sv) })
				if bad && !slices.ContainsFunc(got, func(sv build.StatValue) bool { return !g.rollingGives(c, sv) }) {
					bad = false
					current++
				}
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
			if got := [4]int{len(cases), wrong, warned, current}; got != want[set] {
				t.Errorf("lines, wrong, warned, current: got %v, want %v", got, want[set])
			}
		})
	}
}

// roundTrip is an item of one generated mod: its text, the mod's stat values in it, and the
// item's base and tags.
type roundTrip struct {
	mod, text string
	values    map[string]int32
	base      *dataset.ItemBase
	tags      []string
}

// readBack parses c and returns the stats its lines read as and its warning codes other than
// item-roll-assumed.
func readBack(t *testing.T, r *items.Reader, c roundTrip) (stats []build.StatValue, codes []string) {
	t.Helper()
	it, warnings, err := r.Parse(c.text)
	if err != nil {
		t.Fatalf("%s: %v", c.mod, err)
	}
	for _, m := range it.Mods {
		stats = append(stats, m.Stats...)
	}
	for _, w := range warnings {
		if w.Code != items.RollAssumed {
			codes = append(codes, string(w.Code))
		}
	}
	return stats, codes
}

// rollingGives reports whether a prefix or suffix rolling on c's item has sv's stat with its
// value in range.
func (g *generator) rollingGives(c roundTrip, sv build.StatValue) bool {
	for i := range g.mods {
		m := &g.mods[i]
		if !isAffix(m) || m.Domain != rtDomain(c.base.Class) || !rtSpawns(m, c.tags) {
			continue
		}
		for _, st := range m.Stats {
			if g.stats[st.Stat].ID == sv.Stat && sv.Value >= min(st.Min, st.Max) && sv.Value <= max(st.Min, st.Max) {
				return true
			}
		}
	}
	return false
}

// matches reports whether sv is one of values, at its value or at one the text shows the same,
// as when a display rounds.
func (g *generator) matches(values map[string]int32, sv build.StatValue) bool {
	v, ok := values[sv.Stat]
	if !ok {
		return false
	}
	if v == sv.Value {
		return true
	}
	other := maps.Clone(values)
	other[sv.Stat] = sv.Value
	a, errA := g.set.Describe(mainScope, values)
	b, errB := g.set.Describe(mainScope, other)
	return errA == nil && errB == nil && slices.EqualFunc(a, b, func(x, y statdesc.Line) bool { return x.Text == y.Text })
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
	rtRuthless  = 2
	rtCorrupted = 5
	rtCrafted   = 9
	rtEnchant   = 10
	rtDelve     = 16
	rtExarch    = 28
	rtEater     = 29
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
		sets := []string{set}
		if set == "crafted" {
			sets = append(sets, "unmarked craft")
		}
		for _, high := range []bool{false, true} {
			values := g.values(m, high)
			lines := g.render(values)
			if len(lines) == 0 {
				continue
			}
			for _, set := range sets {
				out[set] = append(out[set], roundTrip{mod: m.ID, text: g.text(set, b, flag, lines), values: values, base: b, tags: g.tagsWith(b, flag)})
			}
		}
	}
	return out
}

// place returns m's set, the base it goes on and the flag it needs, or no base.
func (g *generator) place(m *dataset.Mod) (string, *dataset.ItemBase, string) {
	switch {
	case m.GameMode == rtRuthless || len(m.Stats) == 0:
		return "", nil, ""
	case m.Domain == rtCrafted:
		return "crafted", g.craftBase(m), ""
	case isAffix(m):
		return g.placeAffix(m)
	case m.Domain == 1:
		return g.placeImplicit(m)
	}
	return "", nil, ""
}

// placeAffix places a prefix or suffix: a rolling one on a base it spawns on, a Delve,
// essence-type or elevated one as a given mod.
func (g *generator) placeAffix(m *dataset.Mod) (string, *dataset.ItemBase, string) {
	switch {
	case m.Domain == rtDelve:
		return "given", g.craftBase(m), ""
	case m.Domain == 1 && !rtSpawnsAnywhere(m):
		b, flag := g.elevatedBase(m)
		if b == nil && rtNamesNoKind(m) {
			b = g.craftBase(m)
		}
		return "given", b, flag
	}
	b, flag := g.spawnBase(m)
	return "affix", b, flag
}

// placeImplicit places an eldritch or corruption implicit or an enchantment on a base it spawns
// on.
func (g *generator) placeImplicit(m *dataset.Mod) (string, *dataset.ItemBase, string) {
	b, flag := g.spawnBase(m)
	switch m.Generation {
	case rtExarch, rtEater:
		return "eldritch", b, flag
	case rtCorrupted:
		return "corrupted", b, "Corrupted"
	case rtEnchant:
		return "enchant", b, ""
	}
	return "", nil, ""
}

// text is a rare item of base b with lines, as set writes them, and flag.
func (g *generator) text(set string, b *dataset.ItemBase, flag string, lines []string) string {
	implicits, prefix := 0, ""
	switch set {
	case "eldritch", "corrupted":
		implicits = len(lines)
	case "enchant":
		implicits, prefix = len(lines), "{enchant}"
	case "crafted":
		prefix = "{crafted}"
	}
	head, tail := "Rarity: RARE\nX\n"+g.names[b.ID]+"\n", ""
	switch {
	case flag == "Corrupted":
		tail = flag + "\n"
	case flag != "":
		head += flag + "\n"
	}
	return fmt.Sprintf("%sImplicits: %d\n%s%s\n%s", head, implicits, prefix, strings.Join(lines, "\n"+prefix), tail)
}

// elevatedBase returns the first base whose tags with an influence's name one of m's weights,
// with that influence's flag.
func (g *generator) elevatedBase(m *dataset.Mod) (*dataset.ItemBase, string) {
	for _, b := range g.bases {
		if rtDomain(b.Class) != 1 {
			continue
		}
		for _, x := range g.tagSets[b][1:] {
			added := x.tags[len(b.Tags):]
			if slices.ContainsFunc(m.SpawnWeights, func(w dataset.TagWeight) bool { return slices.Contains(added, w.Tag) }) {
				return b, x.flag
			}
		}
	}
	return nil, ""
}

// tagsWith returns b's tags with those flag gives.
func (g *generator) tagsWith(b *dataset.ItemBase, flag string) []string {
	for _, x := range g.tagSets[b] {
		if x.flag == flag && flag != "" {
			return x.tags
		}
	}
	return b.Tags
}

func rtSpawnsAnywhere(m *dataset.Mod) bool {
	return slices.ContainsFunc(m.SpawnWeights, func(w dataset.TagWeight) bool { return w.Weight > 0 })
}

// rtNamesNoKind reports whether m's weights name no tag but "default", as an essence's mods.
func rtNamesNoKind(m *dataset.Mod) bool {
	return !slices.ContainsFunc(m.SpawnWeights, func(w dataset.TagWeight) bool { return w.Tag != "default" })
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

// values returns m's stat values at its low or high end.
func (g *generator) values(m *dataset.Mod, high bool) map[string]int32 {
	vals := map[string]int32{}
	for _, st := range m.Stats {
		vals[g.stats[st.Stat].ID] = min(st.Min, st.Max)
		if high {
			vals[g.stats[st.Stat].ID] = max(st.Min, st.Max)
		}
	}
	return vals
}

// render returns the text lines of vals.
func (g *generator) render(vals map[string]int32) []string {
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
