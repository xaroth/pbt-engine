// Package statdesc turns stat values into display text using stat description templates.
package statdesc

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// Errors NewSet and Describe return; match them with errors.Is.
var (
	ErrInvalid      = errors.New("invalid description set")
	ErrUnknownScope = errors.New("unknown description scope")
	ErrBadTemplate  = errors.New("bad description text")
)

// Scope is one stat description file. Lookups fall through to Parent when a stat is not
// described here.
type Scope struct {
	Name         string
	Parent       string
	Descriptions []Description
}

// Description renders a set of stats together. No variants means the stats are known and have
// no text.
type Description struct {
	Stats    []string
	Variants []Variant
}

// Variant is one text choice. The first variant whose limits all match the stat values wins.
type Variant struct {
	Limits   []Limit // one per stat
	Text     string
	Handlers []Handler
}

// Limit is an inclusive range, or with Not set, a single excluded value (Min).
type Limit struct {
	Min, Max int32
	Not      bool
}

// Handler transforms the value of stat Stat (an index into Description.Stats) before display.
type Handler struct {
	Name string
	Stat int
}

// Set holds scopes by name.
type Set struct {
	scopes   map[string]*scope
	handlers map[string]handler
}

type scope struct {
	Scope

	parent *scope
	index  map[string]int
	// texts indexes the variants by the skeleton of their first line, for Read; built on first
	// use.
	texts func() map[string][]textEntry
}

// NewSet indexes scopes. Every parent must be in scopes.
func NewSet(scopes []Scope) (*Set, error) {
	s := &Set{scopes: make(map[string]*scope, len(scopes)), handlers: handlers()}
	for _, sc := range scopes {
		if _, dup := s.scopes[sc.Name]; dup {
			return nil, fmt.Errorf("scope %s defined twice: %w", sc.Name, ErrInvalid)
		}
		x, err := newScope(sc)
		if err != nil {
			return nil, err
		}
		s.scopes[sc.Name] = x
	}
	if err := linkParents(s.scopes); err != nil {
		return nil, err
	}
	if err := checkCycles(s.scopes); err != nil {
		return nil, err
	}
	return s, nil
}

func newScope(sc Scope) (*scope, error) {
	x := &scope{Scope: sc, index: map[string]int{}}
	x.texts = sync.OnceValue(x.indexTexts)
	for i, d := range sc.Descriptions {
		if err := checkDescription(d); err != nil {
			return nil, fmt.Errorf("scope %s description %d: %w", sc.Name, i, err)
		}
		for _, st := range d.Stats {
			x.index[st] = i // a later description of the same stat wins, as in game text
		}
	}
	return x, nil
}

func checkDescription(d Description) error {
	if len(d.Stats) == 0 {
		return fmt.Errorf("no stats: %w", ErrInvalid)
	}
	for _, v := range d.Variants {
		if len(v.Limits) != len(d.Stats) {
			return fmt.Errorf("%d limits for %d stats: %w", len(v.Limits), len(d.Stats), ErrInvalid)
		}
		for _, h := range v.Handlers {
			if h.Stat < 0 || h.Stat >= len(d.Stats) {
				return fmt.Errorf("handler %s stat %d out of range: %w", h.Name, h.Stat, ErrInvalid)
			}
		}
	}
	return nil
}

func linkParents(scopes map[string]*scope) error {
	for _, x := range scopes {
		if x.Parent == "" {
			continue
		}
		p, ok := scopes[x.Parent]
		if !ok {
			return fmt.Errorf("scope %s: parent %s not in set: %w", x.Name, x.Parent, ErrInvalid)
		}
		x.parent = p
	}
	return nil
}

func checkCycles(scopes map[string]*scope) error {
	for _, x := range scopes {
		seen := map[*scope]bool{}
		for p := x; p != nil; p = p.parent {
			if seen[p] {
				return fmt.Errorf("scope %s: parent cycle: %w", x.Name, ErrInvalid)
			}
			seen[p] = true
		}
	}
	return nil
}

// ErrUnknownHandler is returned for a handler Describe cannot apply.
var ErrUnknownHandler = errors.New("unknown handler")

// Line is one rendered description. Text can contain newlines.
type Line struct {
	Text  string
	Stats []string // the description's stats, in template order
}

// Describe returns the text lines for stats in the named scope. Stats with value 0 are not
// described on their own. Lines are ordered parent scope first, then by position in the scope.
func (s *Set) Describe(scopeName string, stats map[string]int32) ([]Line, error) {
	root, ok := s.scopes[scopeName]
	if !ok {
		return nil, fmt.Errorf("unknown scope %s: %w", scopeName, ErrUnknownScope)
	}
	var out []Line
	for _, p := range picks(root, stats) {
		d := p.sc.Descriptions[p.index]
		line, ok, err := s.render(d, stats)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(d.Stats, " "), err)
		}
		if ok {
			out = append(out, Line{Text: line, Stats: d.Stats})
		}
	}
	return out, nil
}

func (s *Set) render(d Description, stats map[string]int32) (string, bool, error) {
	vals := make([]int32, len(d.Stats))
	for i, st := range d.Stats {
		vals[i] = stats[st]
	}
	v, ok := match(d.Variants, vals)
	if !ok {
		return "", false, nil
	}
	nums := make([]number, len(vals))
	for i, x := range vals {
		nums[i] = number{v: float64(x)}
	}
	for _, h := range v.Handlers {
		f, ok := s.handlers[h.Name]
		if !ok {
			return "", false, fmt.Errorf("%w %s", ErrUnknownHandler, h.Name)
		}
		if f.apply != nil {
			nums[h.Stat] = f.apply(nums[h.Stat])
		}
	}
	text, err := format(v.Text, nums)
	if err != nil {
		return "", false, err
	}
	return text, true, nil
}

// pick is a description to render: its position in a scope, and how far up from the root.
type pick struct {
	depth, index int
	sc           *scope
}

// picks returns the descriptions of the non-zero stats, parent scope first, then by position.
func picks(root *scope, stats map[string]int32) []pick {
	picked := map[pick]bool{}
	for st, v := range stats {
		if v == 0 {
			continue
		}
		depth := 0
		for sc := root; sc != nil; sc = sc.parent {
			if i, ok := sc.index[st]; ok {
				picked[pick{depth, i, sc}] = true
				break
			}
			depth++
		}
	}
	order := make([]pick, 0, len(picked))
	for p := range picked {
		order = append(order, p)
	}
	slices.SortFunc(order, func(a, b pick) int {
		if a.depth != b.depth {
			return cmp.Compare(b.depth, a.depth)
		}
		return cmp.Compare(a.index, b.index)
	})
	return order
}

func match(variants []Variant, vals []int32) (Variant, bool) {
	for _, v := range variants {
		ok := true
		for i, l := range v.Limits {
			if l.Not == (vals[i] >= l.Min && vals[i] <= l.Max) {
				ok = false
				break
			}
		}
		if ok {
			return v, true
		}
	}
	return Variant{}, false
}

// number is a display value. dp < 0 prints the shortest exact form; otherwise the value is
// rounded to dp decimals and trailing zeros are dropped unless fixed is set.
type number struct {
	v     float64
	dp    int
	fixed bool
	set   bool // dp was set by a handler
}

func (n number) String() string {
	v := n.rounded()
	if n.set && n.fixed {
		return strconv.FormatFloat(v, 'f', n.dp, 64)
	}
	if v == math.Trunc(v) {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	// Shortest float32 form drops float64 noise such as 0.30000000000000004.
	return strconv.FormatFloat(v, 'f', -1, 32)
}

func (n number) rounded() float64 {
	if !n.set {
		return n.v
	}
	p := math.Pow10(n.dp)
	return math.Round(n.v*p) / p
}

func format(text string, nums []number) (string, error) {
	var b strings.Builder
	next := 0
	for i := 0; i < len(text); i++ {
		var err error
		switch c := text[i]; {
		case c == '[':
			i, err = writeTag(&b, text, i)
		case c == '{':
			i, next, err = writePlaceholder(&b, text, i, nums, next)
		case c == '\\' && i+1 < len(text) && text[i+1] == 'n':
			b.WriteByte('\n')
			i++
		default:
			b.WriteByte(c)
		}
		if err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

// writeTag writes the display text of the [tag|text] at text[i] and returns the index of its
// closing bracket.
func writeTag(b *strings.Builder, text string, i int) (int, error) {
	end := strings.IndexByte(text[i:], ']')
	if end < 0 {
		return 0, fmt.Errorf("unclosed [ in %q: %w", text, ErrBadTemplate)
	}
	tag := text[i+1 : i+end]
	if bar := strings.LastIndexByte(tag, '|'); bar >= 0 {
		tag = tag[bar+1:]
	}
	b.WriteString(tag)
	return i + end, nil
}

// writePlaceholder writes the number the {index:flags} at text[i] names, or nums[next] when it
// names none. It returns the index of the closing brace and the next implicit index.
func writePlaceholder(b *strings.Builder, text string, i int, nums []number, next int) (int, int, error) {
	end, n, flags, err := parsePlaceholder(text, i, next, len(nums))
	if err != nil {
		return 0, 0, err
	}
	s := nums[n].String()
	if strings.HasPrefix(flags, "+") && !strings.HasPrefix(s, "-") {
		s = "+" + s
	}
	b.WriteString(s)
	return end, n + 1, nil
}

// parsePlaceholder parses the {index:flags} at text[i] of a description of stats stats: the
// index of its closing brace, the stat it names (next when it names none) and its flags.
func parsePlaceholder(text string, i, next, stats int) (int, int, string, error) {
	end := strings.IndexByte(text[i:], '}')
	if end < 0 {
		return 0, 0, "", fmt.Errorf("unclosed { in %q: %w", text, ErrBadTemplate)
	}
	spec := text[i+1 : i+end]
	idx, flags, _ := strings.Cut(spec, ":")
	n := next
	if idx != "" {
		var err error
		if n, err = strconv.Atoi(idx); err != nil {
			return 0, 0, "", fmt.Errorf("placeholder {%s} in %q: %w", spec, text, ErrBadTemplate)
		}
	}
	if n < 0 || n >= stats {
		return 0, 0, "", fmt.Errorf("placeholder {%s} has no stat in %q: %w", spec, text, ErrBadTemplate)
	}
	return i + end, n, flags, nil
}
