package poe1

import (
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/output"
)

// statFullDPS names full DPS in a tree explanation; it is no engine stat.
const statFullDPS = "full_dps"

const formulaFullDPS = "hit damage per second of each skill counted, times its count, plus the best ignite and bleeding and every poison, " +
	"damage over time at most 35791394"

// FullDPSSkill is a skill on the sheet counted in the build's full DPS: its slot and how many
// of it are in use.
type FullDPSSkill struct {
	Slot, Count int
}

// SetFullDPS makes skills the sheet's skills counted in full DPS.
func (sh *Sheet) SetFullDPS(skills []FullDPSSkill) { sh.fullDPS = slices.Clone(skills) }

// fullDPSTerm is what one skill counted adds to full DPS: its hit damage per second times its
// count, or one of its damaging ailments'.
type fullDPSTerm struct {
	slot int
	// o is the output read, n its node and read its value.
	o    Output
	n    core.Node
	read float64
	// value is what the term adds before the damage over time cap.
	value float64
	dot   bool
}

// fullDPSOf is the build's full DPS and the terms it sums.
type fullDPSOf struct {
	terms []fullDPSTerm
	// hit is the sum of the hit terms and dot that of the damage over time terms before the cap;
	// total is full DPS.
	hit, dot, total float64
}

// uncapped is full DPS without the damage over time cap.
func (f fullDPSOf) uncapped() float64 { return f.hit + f.dot }

// dotScale is the share of the damage over time terms full DPS keeps under the cap.
func (f fullDPSOf) dotScale() float64 {
	if f.dot <= dotDPSCap {
		return 1
	}
	return dotDPSCap / f.dot
}

// fullDPSValue returns full DPS as PoB's calcFullDPS gives it, over the skills counted other
// than the one in slot skip (-1 for none): each one's hit damage per second, or its minion's,
// times its count; plus the best ignite, a minion's among them, the best bleeding and each
// one's poisons times its count, together at most dotDPSCap. A totem skill counts one totem's
// hits, as PoB does. A hit whose rate the sheet does not give adds nothing.
func (sh *Sheet) fullDPSValue(skip int) (fullDPSOf, error) {
	var all []fullDPSTerm
	for _, fs := range sh.fullDPS {
		if fs.Slot == skip || fs.Slot >= len(sh.skills) {
			continue
		}
		terms, err := sh.fullDPSTerms(fs)
		if err != nil {
			return fullDPSOf{}, err
		}
		all = append(all, terms...)
	}
	return sumFullDPS(all), nil
}

// sumFullDPS sums all, of the best ignite and bleeding only, in their order.
func sumFullDPS(all []fullDPSTerm) fullDPSOf {
	var out fullDPSOf
	var ignite, bleed *fullDPSTerm
	for _, t := range all {
		switch t.o.Key {
		case IgniteDPS, MinionIgniteDPS:
			ignite = best(ignite, t)
		case BleedDPS:
			bleed = best(bleed, t)
		default:
			out.terms = append(out.terms, t)
		}
	}
	for _, t := range []*fullDPSTerm{ignite, bleed} {
		if t != nil {
			out.terms = append(out.terms, *t)
		}
	}
	for _, t := range out.terms {
		if t.dot {
			out.dot += t.value
		} else {
			out.hit += t.value
		}
	}
	out.total = out.hit + min(out.dot, dotDPSCap)
	return out
}

// fullDPSTerms returns the terms of fs: its hit's and its ailments'.
func (sh *Sheet) fullDPSTerms(fs FullDPSSkill) ([]fullDPSTerm, error) {
	hit, err := sh.fullDPSHitTerm(fs)
	if err != nil {
		return nil, err
	}
	ailments, err := sh.fullDPSAilments(fs)
	return slices.Concat(hit, ailments), err
}

// fullDPSHitTerm returns the hit term of fs, none when the sheet gives no hit rate for it.
func (sh *Sheet) fullDPSHitTerm(fs FullDPSSkill) ([]fullDPSTerm, error) {
	k, ok := sh.fullDPSHit(fs.Slot)
	if !ok {
		return nil, nil
	}
	t, err := sh.fullDPSTerm(fs.Slot, k)
	t.value = t.read * float64(fs.Count)
	return []fullDPSTerm{t}, err
}

// fullDPSAilments returns the ailment terms of fs the sheet gives: ignite, its own or its
// minion's, and bleeding at their value, poison times fs's count.
func (sh *Sheet) fullDPSAilments(fs FullDPSSkill) ([]fullDPSTerm, error) {
	var out []fullDPSTerm
	for _, k := range []output.Key{IgniteDPS, MinionIgniteDPS, BleedDPS, PoisonDPS} {
		if !sh.gives(k, fs.Slot) {
			continue
		}
		t, err := sh.fullDPSTerm(fs.Slot, k)
		if err != nil {
			return nil, err
		}
		t.dot, t.value = true, t.read
		if k == PoisonDPS {
			t.value *= float64(fs.Count)
		}
		if t.value > 0 {
			out = append(out, t)
		}
	}
	return out, nil
}

// best returns t when it is larger than b, which wins a tie as PoB's first skill does.
func best(b *fullDPSTerm, t fullDPSTerm) *fullDPSTerm {
	if t.read <= 0 || b != nil && t.read <= b.read {
		return b
	}
	return &t
}

// fullDPSHit returns the hit output full DPS reads for the skill in slot: its minion's hit
// damage per second, or its own; false when the sheet gives neither.
func (sh *Sheet) fullDPSHit(slot int) (output.Key, bool) {
	for _, k := range []output.Key{MinionHitDPS, HitDPS} {
		if sh.gives(k, slot) {
			return k, true
		}
	}
	return "", false
}

// gives reports whether the sheet gives output k for the skill in slot.
func (sh *Sheet) gives(k output.Key, slot int) bool {
	var mn *Minion
	if slot < len(sh.slotMinions) {
		mn = sh.slotMinions[slot]
	}
	return Gives(sh.outputs[k], sh.skills[slot].skill, mn, sh.hands)
}

func (sh *Sheet) fullDPSTerm(slot int, k output.Key) (fullDPSTerm, error) {
	o := sh.outputs[k]
	n, err := sh.outputNode(o, slot)
	if err != nil {
		return fullDPSTerm{}, err
	}
	v, err := sh.session.Get(n)
	if err != nil {
		return fullDPSTerm{}, fmt.Errorf("sheet: %s: %w", k, err)
	}
	return fullDPSTerm{slot: slot, o: o, n: n, read: v}, nil
}

// fullDPSPart returns what the skill in slot adds to full DPS by its hits: its hit damage per
// second, or its minion's, times its count; 0 for a skill not counted.
func (sh *Sheet) fullDPSPart(slot int) (float64, error) {
	i := slices.IndexFunc(sh.fullDPS, func(fs FullDPSSkill) bool { return fs.Slot == slot })
	if i < 0 || slot >= len(sh.skills) {
		return 0, nil
	}
	k, ok := sh.fullDPSHit(slot)
	if !ok {
		return 0, nil
	}
	t, err := sh.fullDPSTerm(slot, k)
	if err != nil {
		return 0, err
	}
	return t.read * float64(sh.fullDPS[i].Count), nil
}

// skillSource is the source that names the skill in slot in an explanation of full DPS.
func (sh *Sheet) skillSource(slot int) Source {
	return Source{Bundle: &resolve.Bundle{Actor: resolve.Player, Source: sh.skills[slot].source}}
}

// explainFullDPS explains full DPS in view v: in the proportional and marginal views a line
// per skill counted, in the tree view a term per hit or ailment summed with what it reads.
func (sh *Sheet) explainFullDPS(ex *Explanation, v View) error {
	f, err := sh.fullDPSValue(-1)
	if err != nil {
		return err
	}
	ex.Amount, ex.Effective, ex.Value = f.uncapped(), f.total, f.total
	switch v {
	case Tree:
		return sh.fullDPSTree(ex, f)
	case Proportional:
		sh.fullDPSProportional(ex, f)
	case Marginal:
		return sh.fullDPSMarginal(ex, f)
	default:
		return fmt.Errorf("%w: %d", ErrUnknownView, v)
	}
	return nil
}

func (sh *Sheet) fullDPSTree(ex *Explanation, f fullDPSOf) error {
	root := &TreeNode{Stat: statFullDPS, Formula: formulaFullDPS, Total: f.uncapped(), Capped: f.total, Value: f.total}
	seen := map[core.Node]bool{}
	for _, t := range f.terms {
		root.Terms = append(root.Terms, TreeTerm{Source: sh.skillSource(t.slot), Op: core.OpBase, Value: t.value, Stat: t.o.stat, Rate: 1, Held: true})
		child, err := sh.treeNode(t.n, seen)
		if err != nil {
			return err
		}
		root.Reads = append(root.Reads, child)
	}
	ex.Tree = root
	return nil
}

func (sh *Sheet) fullDPSProportional(ex *Explanation, f fullDPSOf) {
	scale := f.dotScale()
	bySlot := map[int]int{}
	for _, t := range f.terms {
		i, ok := bySlot[t.slot]
		if !ok {
			i = len(ex.Lines)
			bySlot[t.slot] = i
			ex.Lines = append(ex.Lines, Line{Source: sh.skillSource(t.slot)})
		}
		v := t.value
		if t.dot {
			v *= scale
		}
		ex.Lines[i].Value += v
		ex.Lines[i].Parts = append(ex.Lines[i].Parts, LinePart{Op: core.OpBase, Value: v, Via: []string{t.o.stat}})
	}
	sortLines(ex.Lines, func(l Line) float64 { return l.Value })
}

func (sh *Sheet) fullDPSMarginal(ex *Explanation, f fullDPSOf) error {
	for _, fs := range sh.fullDPS {
		if fs.Slot >= len(sh.skills) {
			continue
		}
		without, err := sh.fullDPSValue(fs.Slot)
		if err != nil {
			return err
		}
		ex.Lines = append(ex.Lines, Line{Source: sh.skillSource(fs.Slot), Value: f.total - without.total, Total: f.uncapped() - without.uncapped()})
	}
	sortLines(ex.Lines, func(l Line) float64 { return l.Value })
	return nil
}
