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
//   - affix: every rolling prefix and suffix (Delve and unveiled ones included) on the first base
//     it spawns on; fractured: the same as {fractured} lines; out of range: the same one above
//     its high values;
//   - implicit: every base's implicits; eldritch: every eldritch implicit; corrupted: every
//     corruption implicit, on a corrupted item;
//   - enchant: every labyrinth enchantment that spawns, as an {enchant} line;
//   - unique: every unique mod, on a unique item;
//   - crafted and unmarked craft: every bench craft, with and without its {crafted} marker, on
//     the first base of each class the bench lists for it (leaving out crafts it no longer
//     offers), or without the crafting fields on the first base a prefix or suffix of its stat
//     spawns on;
//   - essence: every mod the dataset says an essence adds, on a base of each class it lists.
//
// A line is wrong when it reads as a stat the mod does not have, or a value its text shows
// otherwise, unless a prefix or suffix rolling on the item gives that stat and value: an older
// mod whose current version has a new stat reads as the current one ("current"). It is warned
// when it gets a warning other than item-roll-assumed; silent when wrong without a warning.
func TestRoundTrip(t *testing.T) {
	t.Parallel()
	s := realdata.Open(t, realdata.Current)
	r, err := items.NewReader(s)
	if err != nil {
		t.Fatal(err)
	}
	g := newGenerator(t, s)
	// lines, wrong, warned, current, silent.
	//   - affix, fractured: warned are lines two mods give at the same values with different
	//     stats (Delve helmets' reservation) or with a level the text does not show (curses on
	//     hit of two influence tiers).
	//   - out of range: one above a mod's high values. Most are warned out of range; the rest
	//     are values another mod or two of one stat give. The wrong ones read as a bench craft in
	//     range under the stand-in (a global craft on a wand, a local craft on a shield or gloves).
	//   - enchant: Chain Hook's rage chance, two tiers of one text.
	//   - unique: the game files do not say which unique has a mod, and unique mods of other stats
	//     share text; those are warned ambiguous, none silent.
	want := map[string][5]int{
		"affix":          {7512, 0, 16, 1, 0},
		"fractured":      {7512, 0, 16, 1, 0},
		"out of range":   {3754, 11, 1483, 0, 9},
		"implicit":       {1196, 0, 0, 0, 0},
		"eldritch":       {6720, 0, 0, 0, 0},
		"corrupted":      {476, 0, 0, 0, 0},
		"enchant":        {2922, 0, 4, 0, 0},
		"unique":         {17608, 129, 776, 0, 0},
		"crafted":        {1890, 0, 0, 0, 0},
		"unmarked craft": {1890, 0, 0, 0, 0},
	}
	if g.crafting {
		// Counts for a dataset with the crafting fields are pinned with the first such pin.
		want = map[string][5]int{}
	}
	for set, cases := range g.sets() {
		t.Run(set, func(t *testing.T) {
			t.Parallel()
			if set == "essence" && !g.crafting {
				t.Skip("the pinned dataset does not hold essence mods (Items.Crafting is false)")
			}
			w, ok := want[set]
			if !ok {
				t.Fatalf("no counts pinned for %d lines (crafting fields: %t)", len(cases), g.crafting)
			}
			if got := g.check(t, r, cases); got != w {
				t.Errorf("lines, wrong, warned, current, silent: got %v, want %v", got, w)
			}
		})
	}
}

// check reads cases back and returns their lines, wrong, warned, current and silent counts.
func (g *generator) check(t *testing.T, r *items.Reader, cases []roundTrip) [5]int {
	t.Helper()
	n := [5]int{len(cases)}
	for _, c := range cases {
		got, codes := readBack(t, r, c)
		bad := slices.ContainsFunc(got, func(sv build.StatValue) bool { return !g.matches(c.values, sv) })
		if bad && !slices.ContainsFunc(got, func(sv build.StatValue) bool { return !g.rollingGives(c, sv) }) {
			bad = false
			n[3]++
		}
		if bad {
			n[1]++
		}
		if len(codes) > 0 {
			n[2]++
		}
		if bad && len(codes) == 0 {
			n[4]++
		}
		if bad || len(codes) > 0 {
			t.Logf("%s: %q read as %v %v", c.mod, c.text, got, codes)
		}
	}
	return n
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
		if !isAffix(m) || !g.onBase(m, c.base) || !rtSpawns(m, c.tags) {
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
	set      *statdesc.Set
	mods     []dataset.Mod
	byID     map[string]*dataset.Mod
	stats    []dataset.Stat
	bases    []*dataset.ItemBase
	names    map[string]string
	crafting bool
	essences []dataset.ModClasses
	// bench holds the item classes of each bench craft, with the crafting fields.
	bench map[string][]string
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
		set: set, mods: mods, byID: map[string]*dataset.Mod{}, stats: stats, names: baseNames(text.BaseNames),
		crafting: it.Crafting, essences: it.EssenceMods, bench: map[string][]string{},
		tagSets: map[*dataset.ItemBase][]influenced{}, rolling: map[uint32][]*dataset.Mod{},
	}
	for i := range it.Bases {
		if g.names[it.Bases[i].ID] != "" {
			g.bases = append(g.bases, &it.Bases[i])
			g.tagSets[&it.Bases[i]] = withInfluences(it.Bases[i].Tags)
		}
	}
	for _, c := range it.BenchCrafts {
		g.bench[c.Mod] = c.Classes
	}
	for i := range g.mods {
		m := &g.mods[i]
		g.byID[m.ID] = m
		if isAffix(m) && m.Domain == 1 {
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
	rtUnique    = 3
	rtCorrupted = 5
	rtCrafted   = 9
	rtEnchant   = 10
	rtDelve     = 16
	rtUnveiled  = 28
	rtExarch    = 28
	rtEater     = 29
)

func isAffix(m *dataset.Mod) bool {
	return (m.Generation == 1 || m.Generation == 2) && m.GameMode != rtRuthless && len(m.Stats) > 0
}

// domain returns the mod domain of base b: its own with the crafting fields, else the one the
// reader takes for its class.
func (g *generator) domain(b *dataset.ItemBase) int32 {
	if g.crafting {
		return b.ModDomain
	}
	return rtDomain(b.Class)
}

// rtDomain returns the mod domain of items of class, as the reader takes it without the crafting
// fields.
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

// onBase reports whether m's domain is that of base b's mods: its own, or Delve and unveiled
// mods on items.
func (g *generator) onBase(m *dataset.Mod, b *dataset.ItemBase) bool {
	d := g.domain(b)
	return m.Domain == d || d == 1 && (m.Domain == rtDelve || m.Domain == rtUnveiled)
}

// item is how one mod is written: its set, base, flag, rarity and line marker.
type item struct {
	set, flag, rarity, marker string
	base                      *dataset.ItemBase
	implicit, above           bool
}

// sets returns the generated items by set.
func (g *generator) sets() map[string][]roundTrip {
	out := map[string][]roundTrip{}
	for i := range g.mods {
		m := &g.mods[i]
		for _, it := range g.place(m) {
			for _, high := range []bool{false, true} {
				if it.above && !high {
					continue
				}
				values := g.values(m, high, it.above)
				if c, ok := g.write(m.ID, it, values); ok {
					out[it.set] = append(out[it.set], c)
				}
			}
		}
	}
	for _, b := range g.bases {
		if c, ok := g.implicits(b, false); ok {
			out["implicit"] = append(out["implicit"], c)
		}
		if c, ok := g.implicits(b, true); ok {
			out["implicit"] = append(out["implicit"], c)
		}
	}
	out["essence"] = g.essenceSet()
	return out
}

// essenceSet writes every essence mod the dataset lists on a base of each class it lists.
func (g *generator) essenceSet() []roundTrip {
	var out []roundTrip
	for _, e := range g.essences {
		for _, it := range g.essenceItems(e) {
			for _, high := range []bool{false, true} {
				if c, ok := g.write(e.Mod, it, g.values(g.byID[e.Mod], high, false)); ok {
					out = append(out, c)
				}
			}
		}
	}
	return out
}

// write writes values as item it, or false when they render no text.
func (g *generator) write(mod string, it item, values map[string]int32) (roundTrip, bool) {
	lines := g.render(values)
	if len(lines) == 0 || it.base == nil {
		return roundTrip{}, false
	}
	return roundTrip{mod: mod, text: g.text(it, lines), values: values, base: it.base, tags: g.tagsWith(it.base, it.flag)}, true
}

// place returns how mod m is written in each set it belongs to.
func (g *generator) place(m *dataset.Mod) []item {
	switch {
	case m.GameMode == rtRuthless || len(m.Stats) == 0:
		return nil
	case m.Domain == rtCrafted:
		return g.craftItems(m)
	case isAffix(m):
		b, flag := g.spawnBase(m)
		return []item{
			{set: "affix", base: b, flag: flag}, {set: "fractured", base: b, flag: flag, marker: "{fractured}"},
			{set: "out of range", base: b, flag: flag, above: true},
		}
	case m.Domain != 1:
		return nil
	case m.Generation == rtUnique:
		return []item{{set: "unique", base: g.uniqueBase(m), rarity: "UNIQUE"}}
	}
	b, flag := g.spawnBase(m)
	switch m.Generation {
	case rtExarch, rtEater:
		return []item{{set: "eldritch", base: b, flag: flag, implicit: true}}
	case rtCorrupted:
		return []item{{set: "corrupted", base: b, flag: "Corrupted", implicit: true}}
	case rtEnchant:
		return []item{{set: "enchant", base: b, marker: "{enchant}", implicit: true}}
	}
	return nil
}

// craftItems places bench craft m, with and without its marker: with the crafting fields on the
// first base of each class the bench lists for it, and not at all when the bench no longer
// offers it; without them on the first base a prefix or suffix of its stat spawns on.
func (g *generator) craftItems(m *dataset.Mod) []item {
	bases := []*dataset.ItemBase{g.craftBase(m)}
	if g.crafting {
		bases = g.firstOfClasses(g.bench[m.ID])
	}
	out := make([]item, 0, 2*len(bases))
	for _, b := range bases {
		out = append(out, item{set: "crafted", base: b, marker: "{crafted}"}, item{set: "unmarked craft", base: b})
	}
	return out
}

// firstOfClasses returns the first base of each of classes.
func (g *generator) firstOfClasses(classes []string) []*dataset.ItemBase {
	var out []*dataset.ItemBase
	for _, class := range classes {
		if i := slices.IndexFunc(g.bases, func(b *dataset.ItemBase) bool { return b.Class == class }); i >= 0 {
			out = append(out, g.bases[i])
		}
	}
	return out
}

// essenceItems places essence mod e on the first base of each class it lists.
func (g *generator) essenceItems(e dataset.ModClasses) []item {
	bases := g.firstOfClasses(e.Classes)
	out := make([]item, 0, len(bases))
	for _, b := range bases {
		out = append(out, item{set: "essence", base: b})
	}
	return out
}

// implicits writes base b's implicits at their low or high values, or false when it has none.
func (g *generator) implicits(b *dataset.ItemBase, high bool) (roundTrip, bool) {
	values := map[string]int32{}
	for _, id := range b.Implicits {
		if m := g.byID[id]; m != nil && m.GameMode != rtRuthless {
			maps.Copy(values, g.values(m, high, false))
		}
	}
	if len(values) == 0 {
		return roundTrip{}, false
	}
	return g.write(strings.Join(b.Implicits, "+"), item{set: "implicit", base: b, implicit: true}, values)
}

// text is an item of it with lines.
func (g *generator) text(it item, lines []string) string {
	rarity := cmpOr(it.rarity, "RARE")
	head, tail := "Rarity: "+rarity+"\nX\n"+g.names[it.base.ID]+"\n", ""
	switch {
	case it.flag == "Corrupted":
		tail = it.flag + "\n"
	case it.flag != "":
		head += it.flag + "\n"
	}
	implicits := 0
	if it.implicit {
		implicits = len(lines)
	}
	return fmt.Sprintf("%sImplicits: %d\n%s%s\n%s", head, implicits, it.marker, strings.Join(lines, "\n"+it.marker), tail)
}

func cmpOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
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

// spawnBase returns the first base m spawns on, with the influence flag it needs.
func (g *generator) spawnBase(m *dataset.Mod) (*dataset.ItemBase, string) {
	for _, b := range g.bases {
		if !g.onBase(m, b) {
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

// craftBase returns the first base a prefix or suffix of m's first stat spawns on: the pinned
// dataset does not say which items a bench craft applies to.
func (g *generator) craftBase(m *dataset.Mod) *dataset.ItemBase {
	for _, b := range g.bases {
		if g.domain(b) == 1 && slices.ContainsFunc(g.rolling[m.Stats[0].Stat], func(x *dataset.Mod) bool { return rtSpawns(x, b.Tags) }) {
			return b
		}
	}
	return nil
}

// uniqueBase returns a base for unique mod m: the game files do not say which unique has it. A
// mod with a local stat goes where craftBase puts it; one without, on the first item that is
// neither armour nor a weapon, where no stat reads as local.
func (g *generator) uniqueBase(m *dataset.Mod) *dataset.ItemBase {
	if slices.ContainsFunc(m.Stats, func(st dataset.ModStat) bool { return strings.HasPrefix(g.stats[st.Stat].ID, "local_") }) {
		return g.craftBase(m)
	}
	i := slices.IndexFunc(g.bases, func(b *dataset.ItemBase) bool { return g.domain(b) == 1 && b.Armour == nil && b.Weapon == nil })
	return g.bases[i]
}

func rtSpawns(m *dataset.Mod, tags []string) bool {
	for _, w := range m.SpawnWeights {
		if slices.Contains(tags, w.Tag) {
			return w.Weight > 0
		}
	}
	return false
}

// values returns m's stat values at its low or high end, or one above its high end.
func (g *generator) values(m *dataset.Mod, high, above bool) map[string]int32 {
	vals := map[string]int32{}
	for _, st := range m.Stats {
		v := min(st.Min, st.Max)
		if high {
			v = max(st.Min, st.Max)
		}
		if above {
			v++
		}
		vals[g.stats[st.Stat].ID] = v
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
