package core

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// Errors of the proportional view.
var (
	// ErrNoAttribution is a node whose stat has a Formula but no Attribute, so its value
	// cannot be split among sources.
	ErrNoAttribution = errors.New("core: formula has no attribution")
	// ErrAttributionCycle is an Attribute that, directly or through others, asks for the
	// proportional view of the node being attributed.
	ErrAttributionCycle = errors.New("core: attribution cycle")
)

// Part is one source's share of a node's total amount.
type Part struct {
	// Source is the source credited.
	Source SourceID
	// Op is the layer the share comes from: OpBase, OpInc, OpMore, OpOverride or OpFlag.
	Op    Op
	Value float64
	// Mod is the modifier applied, when HasMod. For a shared stat's modifier, Source is the
	// source granting the share and Mod the shared modifier.
	Mod    ModID
	HasMod bool
	// Via lists the nodes the share passed through, nearest first, ending at the node the
	// modifier is on; empty for the node's own terms.
	Via []Node
}

// Attribution is how a stat's Attribute splits a node.
type Attribution struct {
	// Parts sum to Amount.
	Parts []Part
	// Amount is the total amount the parts explain, before any cap: for a capped value, the
	// uncapped one.
	Amount float64
	// CapBy lists the nodes whose values capped Amount to the node's total, if any.
	CapBy []Node
}

// Attribute splits a node's value among sources. It is the counterpart of a Formula; the
// Attributor gives it the node's explanation and the views of other nodes.
type Attribute func(a *Attributor) Attribution

// Proportion is the proportional view of a node: Parts sum to Amount, the total amount
// before caps. Effective is the value after the stat's minimum and maximum and any cap the
// formula applies, Value after rounding.
type Proportion struct {
	Node              Node
	Parts             []Part
	Amount, Effective float64
	Value             float64
	CapBy             []Node
}

// Proportional returns the proportional view of node n.
func (s *Session) Proportional(n Node) (Proportion, error) {
	if err := s.checkNode(n); err != nil {
		return Proportion{}, err
	}
	p := &proportions{s: s, done: map[uint32]Attribution{}, busy: map[uint32]bool{}}
	at, e, err := p.of(n)
	if err != nil {
		return Proportion{}, err
	}
	return Proportion{Node: n, Parts: at.Parts, Amount: at.Amount, Effective: e.Capped, Value: e.Value, CapBy: at.CapBy}, nil
}

// proportions computes attributions within one Proportional call, each node once.
type proportions struct {
	s    *Session
	done map[uint32]Attribution
	busy map[uint32]bool
}

func (p *proportions) of(n Node) (Attribution, Explanation, error) {
	e, err := p.s.Explain(n)
	if err != nil {
		return Attribution{}, Explanation{}, err
	}
	id := p.s.nodeID(n)
	if at, ok := p.done[id]; ok {
		return at, e, nil
	}
	if p.busy[id] {
		return Attribution{}, e, fmt.Errorf("%w: %s", ErrAttributionCycle, e.Stat)
	}
	p.busy[id] = true
	defer delete(p.busy, id)
	a := &Attributor{p: p, e: e, def: &p.s.m.stats[n.Stat]}
	at := a.attribute()
	if a.err != nil {
		return Attribution{}, e, a.err
	}
	p.done[id] = at
	return at, e, nil
}

// Attributor is what an Attribute works with: the node's explanation, other nodes' values
// and proportional views, and the default rule.
type Attributor struct {
	p   *proportions
	e   Explanation
	def *StatDef
	err error
}

// Explanation returns the explanation of the node being attributed.
func (a *Attributor) Explanation() Explanation { return a.e }

// Node returns the node a Ref names, seen from the node being attributed, as its formula
// reads it.
func (a *Attributor) Node(ref Ref) Node {
	n := Node{Actor: a.e.Node.Actor, Stat: ref.Stat, Ctx: a.e.Node.Ctx}
	if ref.Actor != Self {
		n.Actor = ActorID(ref.Actor) //nolint:gosec // the formula read it, so it is in range
	}
	if ref.Ctx != Self {
		n.Ctx = CtxID(ref.Ctx) //nolint:gosec // the formula read it, so it is in range
	}
	n.Uncond = a.def.CycleFamily != 0 && a.p.s.m.stats[n.Stat].CycleFamily == a.def.CycleFamily
	return n
}

// Stat returns the value of stat on the node's actor and context, as its formula reads it.
func (a *Attributor) Stat(stat StatID) float64 {
	v, err := a.p.s.GetNode(a.Node(SelfRef(stat)))
	a.fail(err)
	return v
}

// Of returns the parts of another node, each with that node added to Via, and its amount.
func (a *Attributor) Of(ref Ref) ([]Part, float64) {
	n := a.Node(ref)
	at, _, err := a.p.of(n)
	if a.fail(err) {
		return nil, 0
	}
	out := make([]Part, len(at.Parts))
	for i, pt := range at.Parts {
		pt.Via = append([]Node{n}, pt.Via...)
		out[i] = pt
	}
	return out, at.Amount
}

// Spread splits amount among the sources of another node in proportion to their parts, with
// op as the layer. A node whose parts sum to 0 splits it equally among them. It fails when
// the node has no parts.
func (a *Attributor) Spread(ref Ref, amount float64, op Op) []Part {
	parts, total := a.Of(ref)
	if a.err != nil {
		return nil
	}
	if len(parts) == 0 {
		a.fail(fmt.Errorf("%w: %s spreads over %s, which has no sources", ErrNoAttribution, a.e.Stat, a.p.s.m.stats[ref.Stat].Name))
		return nil
	}
	for i := range parts {
		if total != 0 {
			parts[i].Value = float64(amount * parts[i].Value / total)
		} else {
			parts[i].Value = amount / float64(len(parts))
		}
		parts[i].Op = op
	}
	return parts
}

// BaseTerms returns the parts of the node's base terms.
func (a *Attributor) BaseTerms() []Part {
	var out []Part
	for _, t := range a.e.Terms {
		if t.Held && t.Op == OpBase {
			out = append(out, a.credit(t, t.Value)...)
		}
	}
	return out
}

// Default returns the default rule's parts: base terms, then increased and more terms over
// their sum. It is Layers(BaseTerms()).
func (a *Attributor) Default() []Part { return a.Layers(a.BaseTerms()) }

// Layers returns base followed by the node's increased and more terms applied to the sum of
// base: an increased term gets B i/100; more terms split B (1 + I/100)(M - 1) by the weight
// ln(1 + m/100). A more factor of 0 or below takes the whole more part, split equally among
// such terms.
func (a *Attributor) Layers(base []Part) []Part {
	out := append([]Part(nil), base...)
	b := 0.0
	for _, pt := range base {
		b += pt.Value
	}
	inc, more, nonPositive := a.sums()
	bi := float64(b * (1 + inc/percent))
	for _, t := range a.e.Terms {
		if !t.Held {
			continue
		}
		switch t.Op {
		case OpInc:
			out = append(out, a.credit(t, float64(b*t.Value)/percent)...)
		case OpMore:
			if v, ok := moreShare(t.Value, bi, more, nonPositive); ok {
				out = append(out, a.credit(t, v)...)
			}
		case OpBase, OpOverride, OpFlag:
		}
	}
	return out
}

// attribute splits the node by the rule that computed it.
func (a *Attributor) attribute() Attribution {
	switch {
	case a.e.Overridden:
		return a.override()
	case a.def.Formula != nil && a.def.Attribute == nil:
		a.fail(fmt.Errorf("%w: %s", ErrNoAttribution, a.e.Stat))
		return Attribution{}
	case a.def.Formula != nil:
		return a.def.Attribute(a)
	case a.def.Flag:
		return a.flag()
	}
	return Attribution{Parts: a.Default(), Amount: a.e.Total}
}

// sums returns the node's increased sum, more product and number of more factors of 0 or
// below.
func (a *Attributor) sums() (inc, more float64, nonPositive int) {
	more = 1
	for _, t := range a.e.Terms {
		switch {
		case !t.Held:
		case t.Op == OpInc:
			inc += t.Value
		case t.Op == OpMore:
			f := 1 + t.Value/percent
			more *= f
			if f <= 0 {
				nonPositive++
			}
		}
	}
	return inc, more, nonPositive
}

// moreShare is a more term m's share of bi (M - 1), and false when it gets none.
func moreShare(m, bi, more float64, nonPositive int) (float64, bool) {
	f := 1 + m/percent
	switch {
	case nonPositive > 0 && f <= 0:
		return float64(bi*(more-1)) / float64(nonPositive), true
	case nonPositive > 0:
		return 0, false
	case more == 1:
		return float64(bi * math.Log(f)), true
	}
	return float64(bi * math.Log(f) * ((more - 1) / math.Log(more))), true
}

// credit returns the parts for term t worth v: its own source, or for a shared stat's term
// the sources granting the share (spread by their parts of the rate).
func (a *Attributor) credit(t Term, v float64) []Part {
	if !t.Shared {
		return []Part{{Source: t.Source, Op: t.Op, Value: v, Mod: t.Mod, HasMod: true}}
	}
	for _, sh := range a.def.Shares {
		if sh.Stat == t.Stat {
			parts := a.Spread(sh.Rate, v, t.Op)
			for i := range parts {
				parts[i].Mod, parts[i].HasMod = t.Mod, true
			}
			return parts
		}
	}
	return []Part{{Source: t.Source, Op: t.Op, Value: v, Mod: t.Mod, HasMod: true}}
}

// override gives the whole value to the matching override, the last one held.
func (a *Attributor) override() Attribution {
	for _, t := range slices.Backward(a.e.Terms) {
		if t.Held && t.Op == OpOverride {
			return Attribution{Parts: []Part{{Source: t.Source, Op: OpOverride, Value: a.e.Total, Mod: t.Mod, HasMod: true}}, Amount: a.e.Total}
		}
	}
	return Attribution{Amount: a.e.Total}
}

// flag splits a flag stat's 1 equally among its flag terms.
func (a *Attributor) flag() Attribution {
	var flags []Term
	for _, t := range a.e.Terms {
		if t.Held && t.Op == OpFlag {
			flags = append(flags, t)
		}
	}
	at := Attribution{Amount: a.e.Total}
	for _, t := range flags {
		at.Parts = append(at.Parts, Part{Source: t.Source, Op: OpFlag, Value: a.e.Total / float64(len(flags)), Mod: t.Mod, HasMod: true})
	}
	return at
}

// fail records err, the first one wins, and reports whether an error is recorded.
func (a *Attributor) fail(err error) bool {
	if err != nil && a.err == nil {
		a.err = err
	}
	return a.err != nil
}
