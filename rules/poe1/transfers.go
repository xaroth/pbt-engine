package poe1

import (
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/core"
	"github.com/xaroth/pbt-engine/semantics"
)

// Rules that move or copy a share of one stat's value to another, with parameters from and to.
const (
	// ruleGainAsExtra: "Gain X% of Maximum Life as Extra Armour".
	ruleGainAsExtra = "gain_as_extra"
	// ruleConvert: "X% of Maximum Life Converted to Energy Shield"; the source loses the share.
	ruleConvert = "convert"
)

// transfer is one supported use of ruleGainAsExtra or ruleConvert.
type transfer struct {
	rule, from, to string
}

// transfers are the pairs the poe1 table uses.
func transfers() []transfer {
	return []transfer{
		{rule: ruleGainAsExtra, from: statMaximumMana, to: statMaximumEnergyShield},
		{rule: ruleGainAsExtra, from: statMaximumLife, to: statArmour},
		{rule: ruleGainAsExtra, from: statMaximumLife, to: statMaximumEnergyShield},
		{rule: ruleConvert, from: statMaximumLife, to: statMaximumEnergyShield},
		{rule: ruleConvert, from: statEvasionRating, to: statArmour},
	}
}

// Names of the helper stats a transfer reads. They are internal: no table names them.
func (t transfer) rate() string { return t.rule + "." + t.from + "." + t.to + ".%" }
func (t transfer) inc() string  { return "increases." + t.from + "." + t.to }
func (t transfer) more() string { return "more." + t.from + "." + t.to }
func baseOf(stat string) string { return "base." + stat }

// addTransferStats registers each transfer's helper stats on b, after the engine stats, and
// sets the formulas of the stats that transfers take from or give to.
//
// The share starts from the source's flat amounts; the increases of source and target are
// summed (a modifier both include counts once) and the more multipliers of both multiply, as
// PoB does (CalcDefence.lua:1021 to 1060). A converted source keeps (1 - rate) of its value,
// at least none. Neither the text nor game data states this order; it is unvalidated.
func addTransferStats(b *core.ModelBuilder, ids map[string]core.StatID) {
	for _, t := range transfers() {
		from, to := statDef(t.from, ids).Includes, statDef(t.to, ids).Includes
		both := append([]core.StatID{mustID(ids, t.from), mustID(ids, t.to)}, from...)
		for _, inc := range to {
			if !slices.Contains(both, inc) {
				both = append(both, inc)
			}
		}
		register(b, ids, core.StatDef{Name: t.rate()})
		register(b, ids, core.StatDef{
			Name: t.inc(), Includes: both, FormulaSeesOverride: true,
			Formula: func(_ core.Reader, a core.Agg) float64 { return a.Inc },
		})
		register(b, ids, core.StatDef{
			Name: t.more(), Includes: both, FormulaSeesOverride: true,
			Formula: func(_ core.Reader, a core.Agg) float64 { return a.More },
		})
		register(b, ids, core.StatDef{
			Name: baseOf(t.from), Includes: append([]core.StatID{mustID(ids, t.from)}, from...), FormulaSeesOverride: true,
			Formula: sourceBase,
		})
	}
	for _, name := range transferStats() {
		if err := b.SetFormula(mustID(ids, name), transferFormula(name, ids)); err != nil {
			panic(err)
		}
	}
}

// sourceBase is the amount a share starts from: the source's flat amounts, or its override
// when one replaces the source (Chaos Inoculation's 1 life, Blood Magic's 0 mana). That an
// overridden source transfers from its override is ours; the text does not say and PoB adds
// exactly 1 under Chaos Inoculation. The increases and more helpers ignore overrides, so the
// share still takes the source's and target's increases.
func sourceBase(_ core.Reader, a core.Agg) float64 {
	if a.HasOverride {
		return a.Override
	}
	return a.Base
}

// register adds def unless a stat of its name is registered; transfers share base views and
// the increases and more of a pair.
func register(b *core.ModelBuilder, ids map[string]core.StatID, def core.StatDef) {
	if _, ok := ids[def.Name]; !ok {
		ids[def.Name] = b.MustAddStat(def)
	}
}

// transferStats are the stats transfers take from or give to, each once.
func transferStats() []string {
	var out []string
	for _, t := range transfers() {
		for _, s := range []string{t.from, t.to} {
			if !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
	}
	return out
}

// transferFormula is the formula of stat name: its own value, less what converts away, plus
// what transfers give it. It panics when the stat already has a formula, which it would replace.
func transferFormula(name string, ids map[string]core.StatID) core.Formula {
	if statDef(name, ids).Formula != nil {
		panic("poe1: transfer stat " + name + " has a formula")
	}
	type part struct{ rate, base, inc, more core.StatID }
	var away []core.StatID
	var in []part
	for _, t := range transfers() {
		if t.rule == ruleConvert && t.from == name {
			away = append(away, mustID(ids, t.rate()))
		}
		if t.to == name {
			in = append(in, part{mustID(ids, t.rate()), mustID(ids, baseOf(t.from)), mustID(ids, t.inc()), mustID(ids, t.more())})
		}
	}
	return func(rd core.Reader, a core.Agg) float64 {
		converted := 0.0
		for _, r := range away {
			converted += rd.Stat(r)
		}
		v := float64(a.Default() * (1 - min(converted, percentScale)/percentScale))
		for _, p := range in {
			share := float64(rd.Stat(p.base) * rd.Stat(p.rate) / percentScale)
			v += float64(share * (1 + rd.Stat(p.inc)/percentScale) * rd.Stat(p.more))
		}
		return v
	}
}

// applyRule adds the modifiers of an authored entry's rule for a stat of value v, and reports
// whether these rules implement it.
func (sh *Sheet) applyRule(e *semantics.Entry, v float64, src core.SourceID) (bool, error) {
	for _, t := range transfers() {
		if t.rule != e.Rule || t.from != e.Params["from"] || t.to != e.Params["to"] {
			continue
		}
		mod := core.Modifier{Stat: sh.stats[t.rate()], Op: core.OpBase, Value: e.RuleFactor.Apply(v), Source: src}
		if _, err := sh.session.AddModifier(mod); err != nil {
			return false, fmt.Errorf("sheet: %s: %w", e.Stat, err)
		}
		return true, nil
	}
	return false, nil
}
