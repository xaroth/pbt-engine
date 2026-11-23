package items

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/xaroth/pbt-engine/build"
)

// parsed is item text split into its parts.
type parsed struct {
	item     build.Item
	baseName string
	lines    []modText
	// game is text the game copied, not Path of Building.
	game     bool
	scaling  scaling
	variants variants
}

type modText struct {
	kind build.ModKind
	text string
	// unscalable is a line nothing scales from its mod's range (see scaling).
	unscalable bool
}

// flagLines are lines that name a property of the item rather than a mod.
func flagLines() []string {
	return []string{
		"Corrupted", "Mirrored", "Split", "Unidentified", "Synthesised Item", "Fractured Item",
		"Shaper Item", "Elder Item", "Crusader Item", "Hunter Item", "Redeemer Item", "Warlord Item",
		"Searing Exarch Item", "Eater of Worlds Item",
	}
}

func split(text string) (parsed, error) {
	var lines []string
	for l := range strings.Lines(text) {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	var p parsed
	names, err := p.names(lines)
	if err != nil {
		return p, err
	}
	implicits := -1
	i := 1 + names
	for ; i < len(lines) && implicits < 0; i++ {
		implicits = p.property(lines[i])
	}
	if implicits < 0 {
		return p, fmt.Errorf("%w: no implicit count", ErrFormat)
	}
	p.modLines(lines[i:], implicits)
	return p, nil
}

// names reads the rarity and the name lines, and returns how many name lines there are.
func (p *parsed) names(lines []string) (int, error) {
	first := ""
	if len(lines) > 0 {
		first = lines[0]
	}
	rarity, ok := strings.CutPrefix(first, "Rarity: ")
	if !ok {
		return 0, fmt.Errorf("%w: no rarity line", ErrFormat)
	}
	p.item.Rarity = build.Rarity(strings.ToLower(rarity))
	if p.item.Rarity == "relic" {
		// A Relic is a unique of another look; its mods are the unique's.
		p.item.Rarity = build.Unique
	}
	names := 1
	switch p.item.Rarity {
	case build.Rare, build.Unique:
		names = 2
	case build.Magic, build.Normal:
	default:
		return 0, fmt.Errorf("%w: rarity %q", ErrFormat, rarity)
	}
	if len(lines) < 1+names {
		return 0, fmt.Errorf("%w: no name", ErrFormat)
	}
	p.baseName = lines[names]
	if p.item.Rarity != build.Normal {
		p.item.Name = lines[1]
	}
	return names, nil
}

// modLines reads the lines after the implicit count, the first implicits of them implicit.
func (p *parsed) modLines(lines []string, implicits int) {
	for k, l := range lines {
		if slices.Contains(flagLines(), l) {
			p.item.Flags = append(p.item.Flags, l)
			continue
		}
		t := tagged(l)
		if k < implicits && t.kind == build.Explicit {
			t.kind = build.Implicit
		}
		if !p.variants.keeps(t.variants) {
			continue
		}
		text := t.text
		if t.rolled {
			text = withRoll(text, t.roll)
		}
		p.lines = append(p.lines, modText{kind: t.kind, text: text, unscalable: t.unscalable})
	}
}

// property reads one property line into the item and returns the implicit count when the line
// is "Implicits: N", else -1.
func (p *parsed) property(l string) int {
	key, val, ok := strings.Cut(l, ": ")
	if !ok {
		if strings.HasSuffix(l, " Item") {
			p.item.Flags = append(p.item.Flags, l)
		}
		return -1
	}
	// The game adds notes such as " (augmented)".
	val, _, _ = strings.Cut(val, " (")
	n, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(val, "+"), "%"))
	if key == "Implicits" {
		return max(n, 0)
	}
	if key == "Sockets" {
		p.item.Sockets = val
	}
	if p.scaling.property(key, n, val) || p.variants.property(key, n) {
		return -1
	}
	if p.defenceRoll(key, val) {
		return -1
	}
	fields := map[string]*int{
		"Item Level": &p.item.ItemLevel, "Quality": &p.item.Quality, "Armour": &p.item.Armour,
		"Evasion": &p.item.Evasion, "Evasion Rating": &p.item.Evasion, "Energy Shield": &p.item.EnergyShield, "Ward": &p.item.Ward,
	}
	if f, ok := fields[key]; ok {
		*f = n
	}
	return -1
}

// defenceRoll reads a property that gives a defence's roll and reports whether key is one.
func (p *parsed) defenceRoll(key, val string) bool {
	d, ok := basePercentiles()[key]
	if !ok {
		return false
	}
	if roll, err := strconv.ParseFloat(val, 64); err == nil {
		if p.item.DefenceRolls == nil {
			p.item.DefenceRolls = map[build.Defence]float64{}
		}
		p.item.DefenceRolls[d] = roll
	}
	return true
}

// basePercentiles are Path of Building's properties that give a defence's roll.
func basePercentiles() map[string]build.Defence {
	out := map[string]build.Defence{}
	for _, d := range defenceProperties() {
		out[d.percentile] = d.key
	}
	return out
}

// defenceProperty names a defence in Path of Building's text: its property and the property of
// its roll.
type defenceProperty struct {
	key              build.Defence
	name, percentile string
	value            func(*build.Item) int
}

// defenceProperties are the defences in the order Path of Building writes them.
func defenceProperties() []defenceProperty {
	return []defenceProperty{
		{build.DefenceArmour, "Armour", "ArmourBasePercentile", func(it *build.Item) int { return it.Armour }},
		{build.DefenceEvasion, "Evasion", "EvasionBasePercentile", func(it *build.Item) int { return it.Evasion }},
		{build.DefenceEnergyShield, "Energy Shield", "EnergyShieldBasePercentile", func(it *build.Item) int { return it.EnergyShield }},
		{build.DefenceWard, "Ward", "WardBasePercentile", func(it *build.Item) int { return it.Ward }},
	}
}

// PoBText writes it as Path of Building's item text, which Parse reads back to it; base is the
// English name of its base. Its abyss jewels are items of their own and are not written.
//
// Lines are in the order Path of Building writes them (Item.lua BuildRaw). An implicit line
// after an explicit one reads back as explicit. Flags Parse reads after the mods are written
// there, others before the implicit count.
func PoBText(it build.Item, base string) string {
	var lines []string
	trailing := slices.DeleteFunc(slices.Clone(it.Flags), func(f string) bool { return !slices.Contains(flagLines(), f) })
	lines = append(lines, nameLines(it, base)...)
	lines = append(lines, propertyLines(it)...)
	lines = append(lines, fmt.Sprintf("Implicits: %d", implicitCount(it.Mods)))
	for _, m := range it.Mods {
		for l := range strings.SplitSeq(m.Text, "\n") {
			lines = append(lines, kindTags()[m.Kind]+l)
		}
	}
	lines = append(lines, trailing...)
	return strings.Join(lines, "\n") + "\n"
}

// nameLines are the rarity and name lines of it, of base.
func nameLines(it build.Item, base string) []string {
	rarity := "Rarity: " + strings.ToUpper(string(it.Rarity))
	switch it.Rarity {
	case build.Rare, build.Unique:
		return []string{rarity, it.Name, base}
	case build.Magic:
		return []string{rarity, cmp.Or(it.Name, base)}
	case build.Normal:
	}
	return []string{rarity, base}
}

// propertyLines are the property lines of it before the implicit count: its defences with their
// rolls, the flags Parse reads only there, its item level, quality and sockets.
func propertyLines(it build.Item) []string {
	var out []string
	for _, d := range defenceProperties() {
		if v := d.value(&it); v > 0 {
			out = append(out, fmt.Sprintf("%s: %d", d.name, v))
		}
		if r, ok := it.DefenceRolls[d.key]; ok {
			out = append(out, d.percentile+": "+strconv.FormatFloat(r, 'g', -1, 64))
		}
	}
	for _, f := range it.Flags {
		if !slices.Contains(flagLines(), f) {
			out = append(out, f)
		}
	}
	for _, p := range []struct {
		name  string
		value int
	}{{"Item Level", it.ItemLevel}, {"Quality", it.Quality}} {
		if p.value > 0 {
			out = append(out, fmt.Sprintf("%s: %d", p.name, p.value))
		}
	}
	if it.Sockets != "" {
		out = append(out, "Sockets: "+it.Sockets)
	}
	return out
}

// implicitCount is the number of lines up to the last implicit or enchant line before the first
// explicit one. Tagged lines among them, such as a crafted implicit, keep their kind.
func implicitCount(mods []build.ModLine) int {
	n, lines := 0, 0
	for _, m := range mods {
		if m.Kind == build.Explicit {
			break
		}
		lines += strings.Count(m.Text, "\n") + 1
		if m.Kind == build.Implicit || m.Kind == build.Enchant {
			n = lines
		}
	}
	return n
}

// kindTags are the tags Path of Building marks a mod line of a kind with.
func kindTags() map[build.ModKind]string {
	return map[build.ModKind]string{build.Crafted: "{crafted}", build.Fractured: "{fractured}", build.Enchant: "{enchant}"}
}

// lineTags are the leading {tag} markers of a Path of Building mod line.
type lineTags struct {
	kind build.ModKind
	text string
	// variants are the unique's variants the line belongs to; none is every variant.
	variants []int
	// roll is where the line's ranges roll, 0 to 1, when rolled.
	roll   float64
	rolled bool
	// unscalable is a line nothing scales from its mod's range (see scaling).
	unscalable bool
}

// tagged strips the leading {tag} markers of a mod line and returns them with the line.
func tagged(l string) lineTags {
	t := lineTags{kind: build.Explicit}
	for strings.HasPrefix(l, "{") {
		end := strings.IndexByte(l, '}')
		if end < 0 {
			break
		}
		tag, val, _ := strings.Cut(l[1:end], ":")
		t.set(tag, val)
		l = l[end+1:]
	}
	t.text = l
	return t
}

// set reads one tag with its value.
func (t *lineTags) set(tag, val string) {
	switch tag {
	case "crafted":
		t.kind = build.Crafted
	case "fractured":
		t.kind = build.Fractured
	case "enchant":
		t.kind = build.Enchant
	case "unscalable":
		t.unscalable = true
	case "variant":
		for v := range strings.SplitSeq(val, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				t.variants = append(t.variants, n)
			}
		}
	case "range":
		if r, err := strconv.ParseFloat(val, 64); err == nil {
			t.roll, t.rolled = r, true
		}
	}
}
