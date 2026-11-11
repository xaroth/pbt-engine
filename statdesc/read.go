package statdesc

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// Reading is one way lines of text render from stats.
type Reading struct {
	// Stats are the description's stats, in its order.
	Stats []string
	// Values are the stats' values. A stat the text does not show has the value nearest 0 that
	// the variant's limit allows, other than 0 when every value would be 0 (a description of
	// zeros renders nothing). A value shown rounded is the one nearest the exact inverse that
	// renders the same text.
	Values []int32
	// Lines is how many input lines the description spans.
	Lines int
}

// Read returns the readings of lines[0], together with the lines after it for descriptions
// that span several, in the named scope and its parents. Every reading renders the lines back
// exactly. None means no description renders the text.
//
// More than one reading means several descriptions render the same text, such as a local and
// a global stat; the first is the one the game's order prefers: descriptions spanning more
// lines first, then the named scope before its parents, then position in the scope.
func (s *Set) Read(scopeName string, lines []string) ([]Reading, error) {
	ms, err := s.matches(scopeName, lines)
	if err != nil {
		return nil, err
	}
	var out []Reading
	for _, m := range ms {
		r := m.reading
		if !slices.ContainsFunc(out, func(o Reading) bool { return slices.Equal(o.Stats, r.Stats) && slices.Equal(o.Values, r.Values) }) {
			out = append(out, r)
		}
	}
	return out, nil
}

// variantMatch is a reading with the variant that gave it.
type variantMatch struct {
	reading       Reading
	scope         *scope
	desc, variant int
}

// matches returns every variant's reading of lines, sorted as Read sorts them.
func (s *Set) matches(scopeName string, lines []string) ([]variantMatch, error) {
	root, ok := s.scopes[scopeName]
	if !ok {
		return nil, fmt.Errorf("unknown scope %s: %w", scopeName, ErrUnknownScope)
	}
	if len(lines) == 0 {
		return nil, nil
	}
	key := skeleton(lines[0])
	var out []variantMatch
	for sc := root; sc != nil; sc = sc.parent {
		for _, e := range sc.texts()[key] {
			if e.lines > len(lines) {
				continue
			}
			d := sc.Descriptions[e.desc]
			vals, ok := s.read(d, e, strings.Join(lines[:e.lines], "\n"))
			if !ok || !renders(root, sc, e.desc, d.Stats, vals) {
				continue
			}
			out = append(out, variantMatch{reading: Reading{Stats: slices.Clone(d.Stats), Values: vals, Lines: e.lines}, scope: sc, desc: e.desc, variant: e.variant})
		}
	}
	slices.SortStableFunc(out, func(a, b variantMatch) int { return cmp.Compare(b.reading.Lines, a.reading.Lines) })
	return out, nil
}

// renders reports whether rendering in root would use description desc of sc for each stat
// that has a value: no nearer scope or later description of sc describes it instead.
func renders(root, sc *scope, desc int, stats []string, vals []int32) bool {
	for i, st := range stats {
		if vals[i] == 0 {
			continue
		}
		for p := root; p != nil; p = p.parent {
			if j, ok := p.index[st]; ok {
				if p != sc || j != desc {
					return false
				}
				break
			}
		}
	}
	return true
}

// read matches text against entry e of description d and returns the stat values that render
// it, or false.
func (s *Set) read(d Description, e textEntry, text string) ([]int32, bool) {
	v := d.Variants[e.variant]
	shown, ok := e.tmpl.match(text, len(d.Stats))
	if !ok {
		return nil, false
	}
	vals := make([]int32, len(d.Stats))
	for i := range d.Stats {
		if shown[i] == "" {
			continue
		}
		if vals[i], ok = s.invert(v.Handlers, i, shown[i], false); !ok {
			return nil, false
		}
	}
	s.avoidZeros(v, shown, vals)
	stats := make(map[string]int32, len(d.Stats))
	for i, st := range d.Stats {
		stats[st] = vals[i]
	}
	got, ok, err := s.render(d, stats)
	return vals, ok && err == nil && got == text
}

// avoidZeros sets the values of stats the text does not show. Stats that are all 0 render
// nothing: a shown stat whose display hides small values, as "0%", takes the nearest other
// value, and else the hidden ones cannot be 0.
func (s *Set) avoidZeros(v Variant, shown []string, vals []int32) {
	nonZero := slices.ContainsFunc(vals, func(x int32) bool { return x != 0 })
	for i := range vals {
		if !nonZero && shown[i] != "" {
			vals[i], nonZero = s.invert(v.Handlers, i, shown[i], true)
		}
	}
	for i := range vals {
		if shown[i] == "" {
			vals[i] = nearestZero(v.Limits[i], nonZero)
		}
	}
}

// invert returns the value of stat i whose display through handlers is shown: the integer
// nearest the exact inverse whose display matches, other than 0 with nonZero.
func (s *Set) invert(handlers []Handler, i int, shown string, nonZero bool) (int32, bool) {
	shown = strings.TrimPrefix(shown, "+")
	x, err := strconv.ParseFloat(shown, 64)
	if err != nil {
		return 0, false
	}
	var hs []handler
	for _, h := range handlers {
		f, ok := s.handlers[h.Name]
		if !ok {
			return 0, false
		}
		if h.Stat == i {
			hs = append(hs, f)
		}
	}
	exact := inverse(hs, x)
	// A display rounded to dp decimals hides every value within one step of it.
	window := 1
	if n := forward(hs, 0); n.set {
		window += int(math.Ceil(math.Abs(inverse(hs, x+math.Pow10(-n.dp)) - exact)))
	}
	return nearestShown(hs, exact, window, shown, nonZero)
}

// nearestShown returns the integer within window of exact, nearest first, that hs display as
// shown, other than 0 with nonZero.
func nearestShown(hs []handler, exact float64, window int, shown string, nonZero bool) (int32, bool) {
	c := math.Round(exact)
	for k := range window + 1 {
		for _, cand := range nearestFirst(c, float64(k), exact) {
			if cand >= math.MinInt32 && cand <= math.MaxInt32 && (cand != 0 || !nonZero) && forward(hs, cand).String() == shown {
				return int32(cand), true
			}
		}
	}
	return 0, false
}

// nearestFirst returns c-k and c+k, the one nearer exact first; c alone when k is 0.
func nearestFirst(c, k, exact float64) []float64 {
	if k == 0 {
		return []float64{c}
	}
	if math.Abs(c-k-exact) <= math.Abs(c+k-exact) {
		return []float64{c - k, c + k}
	}
	return []float64{c + k, c - k}
}

func forward(hs []handler, v float64) number {
	n := number{v: v}
	for _, h := range hs {
		if h.apply != nil {
			n = h.apply(n)
		}
	}
	return n
}

func inverse(hs []handler, v float64) float64 {
	for _, h := range slices.Backward(hs) {
		if h.invert != nil {
			v = h.invert(v)
		}
	}
	return v
}

// nearestZero returns the value nearest 0 that l allows, or with zeroOK false the nearest
// other than 0, positive first.
func nearestZero(l Limit, zeroOK bool) int32 {
	for _, v := range []int32{0, 1, -1} {
		if (v != 0 || zeroOK) && l.Not != (v >= l.Min && v <= l.Max) {
			return v
		}
	}
	if l.Not || l.Min > 0 {
		return max(l.Min, 1)
	}
	return l.Max
}

// textEntry is a variant as Read matches it.
type textEntry struct {
	desc, variant int
	tmpl          template
	lines         int
}

// indexTexts indexes the variants with text by the skeleton of their first line, in scope
// order. Variants whose text does not parse are left out: Describe fails on them too.
func (x *scope) indexTexts() map[string][]textEntry {
	out := map[string][]textEntry{}
	for i, d := range x.Descriptions {
		for j, v := range d.Variants {
			t, err := compile(v.Text, len(d.Stats))
			if err != nil {
				continue
			}
			key, _, _ := strings.Cut(t.skeleton(), "\n")
			out[key] = append(out[key], textEntry{desc: i, variant: j, tmpl: t, lines: strings.Count(t.String(), "\n") + 1})
		}
	}
	return out
}

// template is a variant's text as literal parts and number slots.
type template []segment

// segment is literal text, or with stat >= 0 the number of that stat.
type segment struct {
	lit  string
	stat int
}

// compile parses text as format reads it: tags become their display text, placeholders slots.
func compile(text string, stats int) (template, error) {
	var c compiler
	next := 0
	for i := 0; i < len(text); i++ {
		switch ch := text[i]; {
		case ch == '[':
			end, err := writeTag(&c.lit, text, i)
			if err != nil {
				return nil, err
			}
			i = end
		case ch == '{':
			end, n, _, err := parsePlaceholder(text, i, next, stats)
			if err != nil {
				return nil, err
			}
			c.slot(n)
			next, i = n+1, end
		case ch == '\\' && i+1 < len(text) && text[i+1] == 'n':
			c.lit.WriteByte('\n')
			i++
		default:
			c.lit.WriteByte(ch)
		}
	}
	c.flush()
	return c.t, nil
}

// compiler collects a template's segments, the literal text in lit until the next slot.
type compiler struct {
	t   template
	lit strings.Builder
}

func (c *compiler) slot(stat int) {
	c.flush()
	c.t = append(c.t, segment{stat: stat})
}

func (c *compiler) flush() {
	if c.lit.Len() > 0 {
		c.t = append(c.t, segment{lit: c.lit.String(), stat: -1})
		c.lit.Reset()
	}
}

// String returns the template with each slot as #.
func (t template) String() string {
	var b strings.Builder
	for _, s := range t {
		if s.stat < 0 {
			b.WriteString(s.lit)
		} else {
			b.WriteByte('#')
		}
	}
	return b.String()
}

// skeleton returns the template's text as skeleton returns a line it renders.
func (t template) skeleton() string {
	var b strings.Builder
	for _, s := range t {
		if s.stat < 0 {
			b.WriteString(s.lit)
		} else {
			b.WriteByte('0')
		}
	}
	return skeleton(b.String())
}

// match returns the number text of each stat's first slot in text ("" for a stat with none),
// or false when text does not have the template's shape.
func (t template) match(text string, stats int) ([]string, bool) {
	shown := make([]string, stats)
	for _, s := range t {
		if s.stat < 0 {
			if !strings.HasPrefix(text, s.lit) {
				return nil, false
			}
			text = text[len(s.lit):]
			continue
		}
		n := numberLen(text)
		if n == 0 {
			return nil, false
		}
		if shown[s.stat] == "" {
			shown[s.stat] = text[:n]
		}
		text = text[n:]
	}
	return shown, text == ""
}

// skeleton replaces each number in line, with its sign, by #, so a line and the template that
// renders it have the same skeleton.
func skeleton(line string) string {
	var b strings.Builder
	for i := 0; i < len(line); {
		if n := numberLen(line[i:]); n > 0 {
			b.WriteByte('#')
			i += n
			continue
		}
		b.WriteByte(line[i])
		i++
	}
	return b.String()
}

// numberLen returns the length of the number at the start of s: an optional sign, digits, and
// optional decimals; 0 when s starts with none.
func numberLen(s string) int {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	end := digitsEnd(s, i)
	if end == i {
		return 0
	}
	if end+1 < len(s) && s[end] == '.' && isDigit(s[end+1]) {
		end = digitsEnd(s, end+1)
	}
	return end
}

// digitsEnd returns the index after the digits of s starting at i.
func digitsEnd(s string, i int) int {
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	return i
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
