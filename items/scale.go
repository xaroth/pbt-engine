package items

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// percent is 100%.
const percent = 100

// defaultCatalystQuality is the quality of a catalyst whose text gives none, as Path of Building
// takes it.
const defaultCatalystQuality = 20

// catalyst is a catalyst by its name in Path of Building's text, by the modifiers the game's
// quality line names, and with the mod tags its quality scales.
type catalyst struct {
	name, modifiers string
	tags            []string
}

// catalysts are Path of Building's catalysts and their tags. "prefix" and "suffix" are not mod
// tags; a prefix or suffix has them (see scaleTags).
func catalysts() []catalyst {
	return []catalyst{
		{"Abrasive", "Attack", []string{"attack"}},
		{"Accelerating", "Speed", []string{"speed"}},
		{"Dextral", "Suffix", []string{"suffix"}},
		{"Fertile", "Life and Mana", []string{"life", "mana", "resource"}},
		{"Imbued", "Caster", []string{"caster"}},
		{"Intrinsic", "Attribute", []string{"jewellery_attribute", "attribute"}},
		{"Noxious", "Physical and Chaos", []string{"physical_damage", "chaos_damage"}},
		{"Prismatic", "Resistance", []string{"jewellery_resistance", "resistance"}},
		{"Sinistral", "Prefix", []string{"prefix"}},
		{"Tempering", "Defence", []string{"jewellery_defense", "defences", "armour", "evasion", "energyshield"}},
		{"Turbulent", "Elemental", []string{"jewellery_elemental", "elemental_damage"}},
		{"Unstable", "Critical", []string{"critical"}},
	}
}

// magnitude is a "N% increased <kind> <tags> Modifier magnitudes" line: it scales the mods of
// lines of its kinds (any kind when none) that have every one of its tags, and when anyTags is
// set one of those too.
type magnitude struct {
	percent int
	kinds   []build.ModKind
	tags    []string
	anyTags []string
}

// magnitudePattern is Path of Building's pattern of a magnitude line, on lower case text.
var magnitudePattern = regexp.MustCompile(`(\d+)% (increased|reduced) ?([a-z ]*) modifier magnitudes`)

// magnitudeKinds are the line kinds a magnitude line's kind word names.
func magnitudeKinds() map[string][]build.ModKind {
	return map[string][]build.ModKind{
		string(build.Explicit): {build.Explicit, build.Crafted, build.Fractured},
		string(build.Implicit): {build.Implicit},
		string(build.Enchant):  {build.Enchant},
	}
}

// magnitudeWords are the words of magnitude lines that differ from the tag they name.
func magnitudeWords() map[string]string {
	return map[string]string{"defence": "defences", "enchantment": "enchant"}
}

// readMagnitude reads text as a magnitude line.
func readMagnitude(text string) (magnitude, bool) {
	m := magnitudePattern.FindStringSubmatch(strings.ToLower(text))
	if m == nil {
		return magnitude{}, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return magnitude{}, false
	}
	if m[2] == "reduced" {
		n = -n
	}
	if m[3] == "explicit physical and chaos damage" {
		return magnitude{percent: n, kinds: magnitudeKinds()[string(build.Explicit)], tags: []string{"damage"}, anyTags: []string{"physical", "chaos"}}, true
	}
	mg := magnitude{percent: n}
	for w := range strings.FieldsSeq(m[3]) {
		if t, ok := magnitudeWords()[w]; ok {
			w = t
		}
		if kinds, ok := magnitudeKinds()[w]; ok {
			mg.kinds = kinds
		} else {
			mg.tags = append(mg.tags, w)
		}
	}
	return mg, true
}

func (mg magnitude) scales(kind build.ModKind, tags []string) bool {
	return (mg.kinds == nil || slices.Contains(mg.kinds, kind)) &&
		!slices.ContainsFunc(mg.tags, func(t string) bool { return !slices.Contains(tags, t) }) &&
		(mg.anyTags == nil || slices.ContainsFunc(mg.anyTags, func(t string) bool { return slices.Contains(tags, t) }))
}

// scaling is what scales an item's mod values from their mods' ranges: its catalyst's quality
// and its magnitude lines, as Path of Building scales them.
type scaling struct {
	// catalyst are the tags of the item's catalyst; quality is its quality when qualitySet.
	catalyst   []string
	quality    int
	qualitySet bool
	magnitudes []magnitude
}

// property reads a catalyst property, Path of Building's "Catalyst" or "CatalystQuality" or the
// game's "Quality (<modifiers> Modifiers)", and reports whether key is one.
func (s *scaling) property(key string, n int, val string) bool {
	switch key {
	case "Catalyst":
		if i := slices.IndexFunc(catalysts(), func(c catalyst) bool { return c.name == val }); i >= 0 {
			s.catalyst = catalysts()[i].tags
		}
		return true
	case "CatalystQuality":
		s.quality, s.qualitySet = n, true
		return true
	}
	mods, prefixed := strings.CutPrefix(key, "Quality (")
	mods, suffixed := strings.CutSuffix(mods, " Modifiers)")
	if !prefixed || !suffixed {
		return false
	}
	if i := slices.IndexFunc(catalysts(), func(c catalyst) bool { return c.modifiers == mods }); i >= 0 {
		s.catalyst, s.quality, s.qualitySet = catalysts()[i].tags, n, true
	}
	return true
}

// addMagnitudes adds the magnitude lines among lines.
func (s *scaling) addMagnitudes(lines []modText) {
	for _, l := range lines {
		if mg, ok := readMagnitude(l.text); ok {
			s.magnitudes = append(s.magnitudes, mg)
		}
	}
}

// scaler returns the factor of each mod on a line of kind; nil when nothing scales the item.
func (s *scaling) scaler(kind build.ModKind) scaler {
	if s.catalyst == nil && s.magnitudes == nil {
		return nil
	}
	return func(m *dataset.Mod) float64 {
		if len(m.GrantedSkills) > 0 {
			return 1
		}
		tags := scaleTags(m)
		f := 1.0
		if slices.ContainsFunc(s.catalyst, func(t string) bool { return slices.Contains(tags, t) }) {
			q := defaultCatalystQuality
			if s.qualitySet {
				q = s.quality
			}
			f = float64(percent+q) / percent
		}
		for _, mg := range s.magnitudes {
			if mg.scales(kind, tags) {
				f += float64(mg.percent) / percent
			}
		}
		return f
	}
}

// scaleTags are m's tags with those Path of Building gives a prefix, a suffix or an unveiled mod.
func scaleTags(m *dataset.Mod) []string {
	tags := slices.Clip(m.Tags)
	switch m.Generation {
	case genPrefix:
		tags = append(tags, "prefix")
	case genSuffix:
		tags = append(tags, "suffix")
	}
	if m.Domain == domainUnveiled {
		tags = append(tags, "unveiled")
	}
	return tags
}

// scaler is the factor a line's values are scaled by from a mod's range; nil scales nothing.
type scaler func(*dataset.Mod) float64

func (s scaler) of(m *dataset.Mod) float64 {
	if s == nil {
		return 1
	}
	return s(m)
}

// scaled returns ms with its range scaled by f, each value truncated toward zero as Path of
// Building's itemLib.formatValue does.
func (ms modStat) scaled(f float64) modStat {
	if f == 1 {
		return ms
	}
	a, b := int32(math.Trunc(float64(ms.min)*f)), int32(math.Trunc(float64(ms.max)*f))
	ms.min, ms.max = min(a, b), max(a, b)
	return ms
}
