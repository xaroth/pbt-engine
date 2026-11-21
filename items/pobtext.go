package items

import (
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
	return map[string]build.Defence{
		"ArmourBasePercentile": build.DefenceArmour, "EvasionBasePercentile": build.DefenceEvasion,
		"EnergyShieldBasePercentile": build.DefenceEnergyShield, "WardBasePercentile": build.DefenceWard,
	}
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
