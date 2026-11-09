package poe1

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/xaroth/pbt-engine/core"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/resolve"
	"github.com/xaroth/pbt-engine/statdesc"
)

// Stat description scopes of node bundles.
const (
	treeScope = "Metadata/StatDescriptions/passive_skill_stat_descriptions.txt"
	auraScope = "Metadata/StatDescriptions/passive_skill_aura_stat_descriptions.txt"
)

// Namer names sources for rendering.
type Namer interface {
	// Name names a source, such as "node 47389 Heart of Oak".
	Name(s Source) string
	// Lines returns a source's stats as game text, empty when there is none.
	Lines(s Source) []string
}

// PlainNamer names sources by their ids, without game text.
type PlainNamer struct{}

// Name returns the source's id.
func (PlainNamer) Name(s Source) string { return s.String() }

// Lines returns nothing.
func (PlainNamer) Lines(Source) []string { return nil }

// TextNamer names sources with a dataset's text.
type TextNamer struct {
	text  *dataset.Text
	set   *statdesc.Set
	nodes map[uint32]string
}

// NewNamer returns a Namer using a dataset's text in one language: node and class names and
// node stats rendered with the stat descriptions.
func NewNamer(text *dataset.Text) (*TextNamer, error) {
	set, err := statdesc.NewSet(text.Descriptions)
	if err != nil {
		return nil, fmt.Errorf("namer: %w", err)
	}
	nodes := make(map[uint32]string, len(text.NodeNames))
	for _, n := range text.NodeNames {
		nodes[n.ID] = n.Name
	}
	return &TextNamer{text: text, set: set, nodes: nodes}, nil
}

// Name names a node by id and name, a character by class name, others by id.
func (t *TextNamer) Name(s Source) string {
	if s.Bundle == nil {
		return s.Rule
	}
	switch src := s.Bundle.Source.(type) {
	case resolve.NodeSource:
		if name, ok := t.nodes[src.ID]; ok {
			return fmt.Sprintf("node %d %s", src.ID, name)
		}
	case resolve.CharacterSource:
		for _, n := range t.text.CharacterNames {
			if n.ID == src.ID {
				return n.Name + " (character)"
			}
		}
	case resolve.LevelSource, resolve.QuestSource:
	}
	return s.String()
}

// Lines renders a node bundle's stats with the stat descriptions.
func (t *TextNamer) Lines(s Source) []string {
	if s.Bundle == nil {
		return nil
	}
	if _, ok := s.Bundle.Source.(resolve.NodeSource); !ok {
		return nil
	}
	scope := treeScope
	if s.Bundle.Scope == resolve.Buff {
		scope = auraScope
	}
	stats := make(map[string]int32, len(s.Bundle.Stats))
	for _, sv := range s.Bundle.Stats {
		stats[sv.Stat] += int32(math.Round(sv.Value))
	}
	ls, err := t.set.Describe(scope, stats)
	if err != nil {
		return []string{"(" + err.Error() + ")"}
	}
	var out []string
	for _, l := range ls {
		out = append(out, strings.Split(l.Text, "\n")...)
	}
	return out
}

// Render writes ex as text, naming sources with nm.
func Render(w io.Writer, ex *Explanation, nm Namer) error {
	r := &renderer{w: w, nm: nm}
	r.header(ex)
	switch ex.View {
	case Tree:
		r.tree(ex.Tree, 1)
	case Proportional:
		r.proportional(ex)
	case Marginal:
		r.marginal(ex)
	}
	return r.err
}

type renderer struct {
	w   io.Writer
	nm  Namer
	err error
}

func (r *renderer) printf(format string, args ...any) {
	if r.err == nil {
		_, r.err = fmt.Fprintf(r.w, format, args...)
	}
}

func (r *renderer) header(ex *Explanation) {
	r.printf("%s: %s%s", ex.Output.Label, num(ex.Value), ex.Output.Unit)
	if over := ex.overCap(); over > 0 {
		r.printf(" (+%s%s over)", num(over), ex.Output.Unit)
	}
	r.printf("  [%s, dataset %s]\n", ex.View, ex.Version)
	if ex.View == Marginal {
		r.printf("  change without each source: value (total before caps)\n")
	}
	if num(ex.Amount) != num(ex.Effective) {
		r.printf("  total %s, effective %s", num(ex.Amount), num(ex.Effective))
		if len(ex.CapBy) > 0 {
			r.printf(", capped by %s", strings.Join(ex.CapBy, ", "))
		}
		r.printf("\n")
	}
}

func (r *renderer) proportional(ex *Explanation) {
	for _, l := range ex.Lines {
		r.printf("  %10s  %s%s", num(l.Value), r.nm.Name(l.Source), r.quoted(l.Source))
		if len(l.Parts) == 1 && len(l.Parts[0].Via) == 0 && l.Parts[0].From == nil {
			r.printf(" (%s)\n", opName(l.Parts[0].Op))
			continue
		}
		r.printf("\n")
		for _, p := range l.Parts {
			r.printf("  %10s    %s %s", "", opName(p.Op), num(p.Value))
			if len(p.Via) > 0 {
				r.printf(" via %s", strings.Join(p.Via, " < "))
			}
			if p.From != nil {
				r.printf(" applying %s", r.nm.Name(*p.From))
			}
			r.printf("\n")
		}
	}
}

func (r *renderer) marginal(ex *Explanation) {
	for _, l := range ex.Lines {
		r.printf("  %10s  %s%s", num(-l.Value), r.nm.Name(l.Source), r.quoted(l.Source))
		if num(l.Total) != num(l.Value) {
			r.printf(" (total %s)", num(-l.Total))
		}
		r.printf("\n")
	}
}

func (r *renderer) tree(t *TreeNode, depth int) {
	pad := strings.Repeat("  ", depth)
	if t.Repeat {
		r.printf("%s%s = %s (above)\n", pad, t.Stat, num(t.Value))
		return
	}
	r.printf("%s%s = %s", pad, t.Stat, num(t.Value))
	if t.Formula != "" {
		r.printf(" [%s]", t.Formula)
	}
	if num(t.Total) != num(t.Capped) {
		r.printf(" (total %s, effective %s)", num(t.Total), num(t.Capped))
	}
	if t.Overridden {
		r.printf(" (overridden)")
	}
	r.printf("\n")
	for _, tt := range t.Terms {
		r.printf("%s  %s %s  %s", pad, opName(tt.Op), num(tt.Value), r.nm.Name(tt.Source))
		if tt.Stat != "" {
			r.printf(" on %s", tt.Stat)
		}
		if tt.Rate != 1 {
			r.printf(" at %s%%", num(tt.Rate*percentScale))
		}
		if !tt.Held {
			r.printf(" (condition not met)")
		}
		r.printf("\n")
	}
	for _, c := range t.Reads {
		r.tree(c, depth+1)
	}
}

// quoted returns the source's game text as ` "line / line"`, or nothing.
func (r *renderer) quoted(s Source) string {
	ls := r.nm.Lines(s)
	if len(ls) == 0 {
		return ""
	}
	return ` "` + strings.Join(ls, " / ") + `"`
}

func opName(op core.Op) string {
	if op == core.OpInc {
		return "increased"
	}
	return op.String()
}

// num formats v with at most two decimals.
func num(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" {
		return "0"
	}
	return s
}
