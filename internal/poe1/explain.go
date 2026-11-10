package poe1

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// Source is what a sheet value can be credited to: a resolved bundle (character, level,
// node, quest) or a rule's own source, such as an inherent attribute bonus.
type Source struct {
	// Bundle is the bundle the source came from; nil for a rule's source.
	Bundle *resolve.Bundle
	// Rule describes a rule's source.
	Rule string
}

func (s Source) String() string {
	if s.Bundle != nil {
		return s.Bundle.Source.String()
	}
	return s.Rule
}

// View is a way to explain an output.
type View uint8

// Views of an explanation.
const (
	// Tree shows how the value is built: terms per stat, nodes read, caps and rounding.
	Tree View = iota + 1
	// Proportional splits the total among sources; the lines sum to Amount.
	Proportional
	// Marginal lists what the output loses without each source.
	Marginal
)

func (v View) String() string {
	switch v {
	case Tree:
		return "tree"
	case Proportional:
		return "proportional"
	case Marginal:
		return "marginal"
	}
	return "view?"
}

// Explanation explains one catalogue output in one view. Amount is the total before caps,
// Effective the value after them and Value the value after rounding, as the sheet shows it.
type Explanation struct {
	Output  Output
	View    View
	Version string
	Amount  float64
	// Effective and Value as the sheet shows them.
	Effective, Value float64
	// CapBy names the engine stats whose values capped Amount.
	CapBy []string
	// Lines are the sources, for the proportional and marginal views.
	Lines []Line
	// Tree is the root of the tree view.
	Tree *TreeNode
}

// Line is one source's share (proportional) or loss (marginal).
type Line struct {
	Source Source
	// Value is the share of Amount, or for the marginal view what the output's value loses.
	Value float64
	// Total is, for the marginal view, what Amount loses.
	Total float64
	// Parts break a proportional share down by layer and path.
	Parts []LinePart
}

// LinePart is part of a source's share: from one layer, through one path of engine stats.
type LinePart struct {
	Op    core.Op
	Value float64
	// Via lists the engine stats the share passed through, nearest first.
	Via []string
	// From is the source of the modifier applied when it is not the line's source: a shared
	// fire resistance modifier credited to the source granting the share.
	From *Source
}

// TreeNode is one engine stat's node in the tree view.
type TreeNode struct {
	Stat, Formula        string
	Terms                []TreeTerm
	Overridden           bool
	Total, Capped, Value float64
	Reads                []*TreeNode
	// Repeat marks a node already shown higher up; its terms and reads are left out.
	Repeat bool
}

// TreeTerm is one modifier considered for a node.
type TreeTerm struct {
	Source Source
	Op     core.Op
	Value  float64
	// Stat is the engine stat the modifier is on, when it is not the node's own.
	Stat string
	// Rate is the share rate for a shared stat's modifier, else 1.
	Rate float64
	// Held is false for a modifier whose condition failed.
	Held bool
}

// Explain explains catalogue output k in view v.
func (sh *Sheet) Explain(k output.Key, v View) (*Explanation, error) {
	o, ok := outputByKey(k)
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrUnknownOutput, k)
	}
	n := core.Node{Stat: sh.stats[o.stat]}
	ex := &Explanation{Output: o, View: v, Version: sh.version}
	var err error
	switch v {
	case Tree:
		err = sh.explainTree(ex, n)
	case Proportional:
		err = sh.explainProportional(ex, n)
	case Marginal:
		err = sh.explainMarginal(ex, n)
	default:
		err = fmt.Errorf("%w: %d", ErrUnknownView, v)
	}
	if err != nil {
		return nil, fmt.Errorf("sheet: explain %s: %w", k, err)
	}
	return ex, nil
}

// amountStat is the engine stat whose total is o's amount before caps: a capped resistance's
// uncapped total, else o's own stat.
func amountStat(o Output) string {
	for _, r := range resistances() {
		if o.stat == r.capped {
			return r.total
		}
	}
	return o.stat
}

// amount returns the total of o's amount stat.
func (sh *Sheet) amount(o Output) (float64, error) {
	e, err := sh.session.Explain(core.Node{Stat: sh.stats[amountStat(o)]})
	if err != nil {
		return 0, fmt.Errorf("amount: %w", err)
	}
	return e.Total, nil
}

func (sh *Sheet) statName(id core.StatID) string {
	def, _ := sh.session.Model().StatDef(id)
	return def.Name
}

func (sh *Sheet) explainTree(ex *Explanation, n core.Node) error {
	seen := map[core.Node]bool{}
	root, err := sh.treeNode(n, seen)
	if err != nil {
		return err
	}
	ex.Tree, ex.Effective, ex.Value = root, root.Capped, root.Value
	ex.Amount, err = sh.amount(ex.Output)
	return err
}

func (sh *Sheet) treeNode(n core.Node, seen map[core.Node]bool) (*TreeNode, error) {
	e, err := sh.session.Explain(n)
	if err != nil {
		return nil, fmt.Errorf("tree: %w", err)
	}
	t := &TreeNode{Stat: e.Stat, Formula: e.Formula, Overridden: e.Overridden, Total: e.Total, Capped: e.Capped, Value: e.Value}
	if seen[n] {
		t.Repeat = true
		return t, nil
	}
	seen[n] = true
	for _, term := range e.Terms {
		tt := TreeTerm{Source: sh.sources[term.Source], Op: term.Op, Value: term.Value, Rate: term.Rate, Held: term.Held}
		if term.Stat != n.Stat {
			tt.Stat = sh.statName(term.Stat)
		}
		t.Terms = append(t.Terms, tt)
	}
	for _, r := range e.Reads {
		child, err := sh.treeNode(r, seen)
		if err != nil {
			return nil, err
		}
		t.Reads = append(t.Reads, child)
	}
	return t, nil
}

func (sh *Sheet) explainProportional(ex *Explanation, n core.Node) error {
	p, err := sh.session.Proportional(n)
	if err != nil {
		return fmt.Errorf("proportional: %w", err)
	}
	ex.Amount, ex.Effective, ex.Value = p.Amount, p.Effective, p.Value
	for _, c := range p.CapBy {
		ex.CapBy = append(ex.CapBy, sh.statName(c.Stat))
	}
	byID := map[core.SourceID]*Line{}
	var order []core.SourceID
	for _, pt := range p.Parts {
		l, ok := byID[pt.Source]
		if !ok {
			l = &Line{Source: sh.sources[pt.Source]}
			byID[pt.Source] = l
			order = append(order, pt.Source)
		}
		l.Value += pt.Value
		l.Parts = sh.addPart(l.Parts, pt)
	}
	for _, id := range order {
		ex.Lines = append(ex.Lines, *byID[id])
	}
	ex.Lines = dropZeros(ex.Lines)
	trimCommonVia(ex.Lines)
	sortLines(ex.Lines, func(l Line) float64 { return l.Value })
	return nil
}

// trimCommonVia drops the path every part shares, such as the uncapped total a capped value
// passes through, merging parts that become the same.
func trimCommonVia(lines []Line) {
	common := commonVia(lines)
	if len(common) == 0 {
		return
	}
	for i := range lines {
		var parts []LinePart
		for _, p := range lines[i].Parts {
			p.Via = slices.Clone(p.Via[len(common):])
			if len(p.Via) == 0 {
				p.Via = nil
			}
			parts = mergePart(parts, p)
		}
		lines[i].Parts = parts
	}
}

// dropZeros leaves out lines whose share is 0 in every part, such as a node's "0% increased
// maximum Life", unless all are: a value a flag turns off still names the flag.
func dropZeros(lines []Line) []Line {
	kept := slices.DeleteFunc(slices.Clone(lines), func(l Line) bool {
		return !slices.ContainsFunc(l.Parts, func(p LinePart) bool { return p.Value != 0 })
	})
	if len(kept) == 0 {
		return lines
	}
	return kept
}

// commonVia returns the longest path prefix all parts share.
func commonVia(lines []Line) []string {
	var common []string
	first := true
	for _, l := range lines {
		for _, p := range l.Parts {
			if first {
				common, first = p.Via, false
				continue
			}
			n := 0
			for n < len(common) && n < len(p.Via) && common[n] == p.Via[n] {
				n++
			}
			common = common[:n]
		}
	}
	return common
}

// mergePart adds lp to parts, summing it into a part of the same layer, path and source.
func mergePart(parts []LinePart, lp LinePart) []LinePart {
	for i := range parts {
		if parts[i].Op == lp.Op && slices.Equal(parts[i].Via, lp.Via) && sameSource(parts[i].From, lp.From) {
			parts[i].Value += lp.Value
			return parts
		}
	}
	return append(parts, lp)
}

// addPart merges pt into parts by layer, path and modifier source.
func (sh *Sheet) addPart(parts []LinePart, pt core.Part) []LinePart {
	lp := LinePart{Op: pt.Op, Value: pt.Value}
	for _, v := range pt.Via {
		lp.Via = append(lp.Via, sh.statName(v.Stat))
	}
	if pt.HasMod {
		if m, _, err := sh.session.Modifier(pt.Mod); err == nil && m.Source != pt.Source {
			from := sh.sources[m.Source]
			lp.From = &from
		}
	}
	return mergePart(parts, lp)
}

func sameSource(a, b *Source) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Bundle == b.Bundle && a.Rule == b.Rule
}

// explainMarginal lists what the output's value loses without each source, and what its
// amount before caps loses, from the amount stat.
func (sh *Sheet) explainMarginal(ex *Explanation, n core.Node) error {
	e, err := sh.session.Explain(n)
	if err != nil {
		return fmt.Errorf("marginal: %w", err)
	}
	ex.Effective, ex.Value = e.Capped, e.Value
	if ex.Amount, err = sh.amount(ex.Output); err != nil {
		return err
	}
	values, err := sh.session.Marginal(n)
	if err != nil {
		return fmt.Errorf("marginal: %w", err)
	}
	totals, err := sh.session.Marginal(core.Node{Stat: sh.stats[amountStat(ex.Output)]})
	if err != nil {
		return fmt.Errorf("marginal: %w", err)
	}
	byID := map[core.SourceID]*Line{}
	var order []core.SourceID
	line := func(src core.SourceID) *Line {
		if l, ok := byID[src]; ok {
			return l
		}
		byID[src] = &Line{Source: sh.sources[src]}
		order = append(order, src)
		return byID[src]
	}
	for _, m := range values {
		line(m.Source).Value = m.Value
	}
	for _, m := range totals {
		line(m.Source).Total = m.Total
	}
	slices.Sort(order)
	for _, id := range order {
		ex.Lines = append(ex.Lines, *byID[id])
	}
	sortLines(ex.Lines, func(l Line) float64 { return l.Total })
	return nil
}

// sortLines orders lines by the absolute value of key, largest first; ties keep source order.
func sortLines(lines []Line, key func(Line) float64) {
	slices.SortStableFunc(lines, func(a, b Line) int { return cmp.Compare(math.Abs(key(b)), math.Abs(key(a))) })
}
