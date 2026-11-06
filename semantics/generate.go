package semantics

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/statdesc"
)

// Description scopes of passive node text.
const (
	PassiveScope = "Metadata/StatDescriptions/passive_skill_stat_descriptions.txt"
	AuraScope    = "Metadata/StatDescriptions/passive_skill_aura_stat_descriptions.txt"
)

// PoBLine is one entry of PoB's ModCache: a mod line and how PoB parses it.
type PoBLine struct {
	Text string   `json:"text"`
	Mods []PoBMod `json:"mods"`
	// Unparsed is the part of the line PoB could not parse; empty when fully parsed.
	Unparsed string `json:"unparsed"`
}

// PoBMod is one modifier PoB parses from a line.
type PoBMod struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Flags        int64             `json:"flags"`
	KeywordFlags int64             `json:"keywordFlags"`
	Value        *float64          `json:"value"`
	Tags         []json.RawMessage `json:"tags"`
}

// Input is what Generate reads.
type Input struct {
	Game, Version, PoB string
	Stats              []dataset.Stat
	Tree               *dataset.Tree // the standard tree
	Text               *statdesc.Set
	ModCache           []PoBLine
	// Overrides are hand-written entries; their status is set to Override.
	Overrides []Entry
}

type statInfo struct {
	uses     int
	nonzero  bool
	sheet    bool
	sigs     map[string][]Mod
	problems []string
}

func (s *statInfo) problem(format string, args ...any) {
	p := fmt.Sprintf(format, args...)
	if !slices.Contains(s.problems, p) {
		s.problems = append(s.problems, p)
	}
}

// Generate derives a semantics table from the lines that standard tree nodes and mastery
// effects render to (node buffs are auras and are left to a later table), and PoB's parse of each line. A stat is Generated when every line it
// appears on is its only stat, PoB fully parses the line into modifiers with no flags or
// conditions, every sheet modifier has an engine stat, and all lines agree on the ratio of
// modifier value to stat value. Anything less leaves a sheet stat Unknown with the reason.
func Generate(in Input) (*Table, error) {
	cache := newModCache(in.ModCache)
	infos := map[string]*statInfo{}
	info := func(id string) *statInfo {
		if infos[id] == nil {
			infos[id] = &statInfo{sigs: map[string][]Mod{}}
		}
		return infos[id]
	}

	observe := func(svs []dataset.StatValue, scope string) error {
		values := map[string]int32{}
		for _, sv := range svs {
			id := in.Stats[sv.Stat].ID
			values[id] = sv.Value
			info(id).uses++
		}
		lines, err := in.Text.Describe(scope, values)
		if err != nil {
			return err
		}
		described := map[string]bool{}
		for _, l := range lines {
			var present []string
			for _, st := range l.Stats {
				if values[st] != 0 {
					present = append(present, st)
					described[st] = true
				}
			}
			observeLine(cache, l, present, values, info)
		}
		for id, v := range values {
			if v != 0 {
				info(id).nonzero = true
				if !described[id] {
					info(id).problem("no description line")
				}
			}
		}
		return nil
	}
	for _, n := range in.Tree.Nodes {
		if err := observe(n.Stats, PassiveScope); err != nil {
			return nil, fmt.Errorf("node %d: %w", n.ID, err)
		}
		for _, e := range n.MasteryEffects {
			if err := observe(e.Stats, PassiveScope); err != nil {
				return nil, fmt.Errorf("node %d mastery effect %d: %w", n.ID, e.ID, err)
			}
		}
	}

	t := &Table{Format: Format, Game: in.Game, Version: in.Version, PoB: in.PoB}
	overrides := map[string]Entry{}
	for _, o := range in.Overrides {
		if _, dup := overrides[o.Stat]; dup {
			return nil, fmt.Errorf("override %s given twice", o.Stat)
		}
		if o.Reason == "" {
			return nil, fmt.Errorf("override %s has no reason", o.Stat)
		}
		o.Status = Override
		overrides[o.Stat] = o
	}
	for id, inf := range infos {
		if o, ok := overrides[id]; ok {
			o.Uses = inf.uses
			t.Entries = append(t.Entries, o)
			delete(overrides, id)
			continue
		}
		t.Entries = append(t.Entries, decide(id, inf))
	}
	for _, o := range overrides {
		t.Entries = append(t.Entries, o)
	}
	slices.SortFunc(t.Entries, func(a, b Entry) int { return strings.Compare(a.Stat, b.Stat) })
	for _, e := range t.Entries {
		if err := e.check(); err != nil {
			return nil, err
		}
	}
	return t, nil
}

func observeLine(cache *modCache, l statdesc.Line, present []string, values map[string]int32, info func(string) *statInfo) {
	var mods []PoBMod
	var problems []string
	for _, text := range strings.Split(l.Text, "\n") {
		pl, ok := cache.lookup(text)
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("line not in PoB's ModCache: %q", text))
		case pl.Unparsed != "":
			problems = append(problems, fmt.Sprintf("PoB does not parse %q", text))
		default:
			mods = append(mods, pl.Mods...)
		}
	}
	var engine []Mod
	for _, m := range mods {
		names, known := pobNames[m.Name]
		if !known || len(names) == 0 {
			continue
		}
		for _, st := range present {
			info(st).sheet = true
		}
		op, okType := pobTypes[m.Type]
		if why := unsupported(m, okType, op); why != "" {
			problems = append(problems, fmt.Sprintf("PoB modifier %s %s %s", m.Name, m.Type, why))
			continue
		}
		if len(present) != 1 {
			continue
		}
		for _, name := range names {
			mod := Mod{Stat: name, Op: op, Mul: 0, Div: 1}
			if op != OpFlag {
				mod.Mul, mod.Div = ratio(*m.Value, float64(values[present[0]]))
			}
			engine = append(engine, mod)
		}
	}
	if len(present) > 1 {
		for _, st := range present {
			if info(st).sheet {
				info(st).problem("shares a line with %s", strings.Join(slices.DeleteFunc(slices.Clone(present), func(s string) bool { return s == st }), ", "))
			}
		}
		return
	}
	for _, st := range present {
		inf := info(st)
		for _, p := range problems {
			inf.problem("%s", p)
		}
		if len(problems) == 0 {
			inf.sigs[signature(engine)] = engine
		}
	}
}

// ratio expresses mod / stat as a small fraction where one exists (1/60 for per-minute
// stats), else as the float quotient over 1.
func ratio(mod, stat float64) (mul, div float64) {
	q := mod / stat
	for d := 1.0; d <= 1000; d++ {
		n := math.Round(q * d)
		if math.Abs(n/d-q) <= 1e-12*math.Max(1, math.Abs(q)) {
			return n, d
		}
	}
	return q, 1
}

func signature(mods []Mod) string {
	var b strings.Builder
	for _, m := range mods {
		fmt.Fprintf(&b, "%s %s %v/%v;", m.Stat, m.Op, m.Mul, m.Div)
	}
	return b.String()
}

func decide(id string, inf *statInfo) Entry {
	e := Entry{Stat: id, Uses: inf.uses, Sheet: inf.sheet}
	switch {
	case !inf.nonzero:
		e.Status, e.Reason = NotSheet, "only appears with value 0"
	case inf.sheet && len(inf.problems) == 0 && len(inf.sigs) == 1:
		e.Status = Generated
		for _, mods := range inf.sigs {
			e.Mods = mods
		}
	case inf.sheet && len(inf.problems) == 0:
		sigs := slices.Sorted(maps.Keys(inf.sigs))
		e.Status, e.Reason = Unknown, "lines disagree on the meaning: "+strings.Join(sigs, " | ")
	case len(inf.problems) > 0:
		slices.Sort(inf.problems)
		e.Status, e.Reason = Unknown, strings.Join(inf.problems, "; ")
	default:
		e.Status = NotSheet
	}
	return e
}

// unsupported says why a PoB modifier cannot become a plain engine modifier yet, or returns
// "" when it can. PoB's Global tag only marks a line as not local to an item, so it is
// accepted.
func unsupported(m PoBMod, okType bool, op Op) string {
	var why []string
	if !okType {
		why = append(why, "has an unknown type")
	}
	if m.Flags != 0 || m.KeywordFlags != 0 {
		why = append(why, "has flags")
	}
	if op != OpFlag && m.Value == nil {
		why = append(why, "has no number value")
	}
	for _, raw := range m.Tags {
		var tag map[string]any
		if err := json.Unmarshal(raw, &tag); err != nil {
			why = append(why, "has an unreadable tag")
			continue
		}
		if len(tag) == 1 && tag["type"] == "Global" {
			continue
		}
		why = append(why, fmt.Sprintf("has a %v tag", tag["type"]))
	}
	return strings.Join(why, ", ")
}

var number = regexp.MustCompile(`\d+(?:\.\d+)?`)

// modCache finds PoB's parse of a line. PoB's ModCache holds only lines with the numbers it
// has seen, so a line with one number is also matched by template against a cached line with
// one number, and the cached modifier values are scaled by the ratio of the two numbers.
// PoB's parser reads numbers from the line linearly, which makes the scaling exact.
type modCache struct {
	exact    map[string]*PoBLine
	template map[string]*PoBLine
}

func newModCache(lines []PoBLine) *modCache {
	c := &modCache{exact: map[string]*PoBLine{}, template: map[string]*PoBLine{}}
	for i := range lines {
		l := &lines[i]
		c.exact[l.Text] = l
		if l.Unparsed != "" || len(number.FindAllString(l.Text, -1)) != 1 {
			continue
		}
		key := number.ReplaceAllString(l.Text, "#")
		if prev, ok := c.template[key]; !ok || l.Text < prev.Text {
			c.template[key] = l // the smallest text, so the choice is stable
		}
	}
	return c
}

func (c *modCache) lookup(text string) (*PoBLine, bool) {
	if l, ok := c.exact[text]; ok {
		return l, true
	}
	nums := number.FindAllString(text, -1)
	if len(nums) != 1 {
		return nil, false
	}
	ex, ok := c.template[number.ReplaceAllString(text, "#")]
	if !ok {
		return nil, false
	}
	have, _ := strconv.ParseFloat(nums[0], 64)
	was, _ := strconv.ParseFloat(number.FindString(ex.Text), 64)
	if was == 0 {
		return nil, false
	}
	out := &PoBLine{Text: text}
	for _, m := range ex.Mods {
		if m.Value != nil {
			v := *m.Value * have / was
			m.Value = &v
		}
		out.Mods = append(out.Mods, m)
	}
	return out, true
}
