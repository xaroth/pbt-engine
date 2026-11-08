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
	scopes map[string]*scope
}

type scope struct {
	Scope

	parent *scope
	index  map[string]int
}

// NewSet indexes scopes. Every parent must be in scopes.
func NewSet(scopes []Scope) (*Set, error) {
	s := &Set{scopes: make(map[string]*scope, len(scopes))}
	for _, sc := range scopes {
		if _, dup := s.scopes[sc.Name]; dup {
			return nil, fmt.Errorf("scope %s defined twice: %w", sc.Name, ErrInvalid)
		}
		x := &scope{Scope: sc, index: map[string]int{}}
		for i, d := range sc.Descriptions {
			if len(d.Stats) == 0 {
				return nil, fmt.Errorf("scope %s description %d: no stats: %w", sc.Name, i, ErrInvalid)
			}
			for _, v := range d.Variants {
				if len(v.Limits) != len(d.Stats) {
					return nil, fmt.Errorf("scope %s description %d: %d limits for %d stats: %w", sc.Name, i, len(v.Limits), len(d.Stats), ErrInvalid)
				}
				for _, h := range v.Handlers {
					if h.Stat < 0 || h.Stat >= len(d.Stats) {
						return nil, fmt.Errorf("scope %s description %d: handler %s stat %d out of range: %w", sc.Name, i, h.Name, h.Stat, ErrInvalid)
					}
				}
			}
			for _, st := range d.Stats {
				x.index[st] = i // a later description of the same stat wins, as in game text
			}
		}
		s.scopes[sc.Name] = x
	}
	for _, x := range s.scopes {
		if x.Parent == "" {
			continue
		}
		p, ok := s.scopes[x.Parent]
		if !ok {
			return nil, fmt.Errorf("scope %s: parent %s not in set: %w", x.Name, x.Parent, ErrInvalid)
		}
		x.parent = p
	}
	for _, x := range s.scopes {
		seen := map[*scope]bool{}
		for p := x; p != nil; p = p.parent {
			if seen[p] {
				return nil, fmt.Errorf("scope %s: parent cycle: %w", x.Name, ErrInvalid)
			}
			seen[p] = true
		}
	}
	return s, nil
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
	type pick struct {
		depth, index int
		sc           *scope
	}
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

	var out []Line
	for _, p := range order {
		d := p.sc.Descriptions[p.index]
		line, ok, err := render(d, stats)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(d.Stats, " "), err)
		}
		if ok {
			out = append(out, Line{Text: line, Stats: d.Stats})
		}
	}
	return out, nil
}

func render(d Description, stats map[string]int32) (string, bool, error) {
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
		f, ok := handlers[h.Name]
		if !ok {
			return "", false, fmt.Errorf("%w %s", ErrUnknownHandler, h.Name)
		}
		if f != nil {
			nums[h.Stat] = f(nums[h.Stat])
		}
	}
	text, err := format(v.Text, nums)
	if err != nil {
		return "", false, err
	}
	return text, true, nil
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
	p := math.Pow(10, float64(n.dp))
	return math.Round(n.v*p) / p
}

func format(text string, nums []number) (string, error) {
	var b strings.Builder
	next := 0
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch c {
		case '[':
			end := strings.IndexByte(text[i:], ']')
			if end < 0 {
				return "", fmt.Errorf("unclosed [ in %q: %w", text, ErrBadTemplate)
			}
			tag := text[i+1 : i+end]
			if bar := strings.LastIndexByte(tag, '|'); bar >= 0 {
				tag = tag[bar+1:]
			}
			b.WriteString(tag)
			i += end
		case '{':
			end := strings.IndexByte(text[i:], '}')
			if end < 0 {
				return "", fmt.Errorf("unclosed { in %q: %w", text, ErrBadTemplate)
			}
			spec := text[i+1 : i+end]
			idx, flags, _ := strings.Cut(spec, ":")
			n := next
			if idx != "" {
				var err error
				if n, err = strconv.Atoi(idx); err != nil {
					return "", fmt.Errorf("placeholder {%s} in %q: %w", spec, text, ErrBadTemplate)
				}
			}
			next = n + 1
			if n >= len(nums) {
				return "", fmt.Errorf("placeholder {%s} has no stat in %q: %w", spec, text, ErrBadTemplate)
			}
			s := nums[n].String()
			if strings.HasPrefix(flags, "+") && !strings.HasPrefix(s, "-") {
				s = "+" + s
			}
			b.WriteString(s)
			i += end
		case '\\':
			if i+1 < len(text) && text[i+1] == 'n' {
				b.WriteByte('\n')
				i++
				continue
			}
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), nil
}
