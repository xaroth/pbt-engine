package poe1

import (
	"slices"

	"github.com/xaroth/pbt-engine/internal/core"
)

// Engine stats of a hit's damage with conversion, read in a skill's context for one damage
// type. minimum_hit_damage and maximum_hit_damage are the hit of that type after damage and
// conversion; the _before_conversion stats are the hit of the type before what converts away.
const (
	statMinimumHitDamageBeforeConversion = "minimum_hit_damage_before_conversion"
	statMaximumHitDamageBeforeConversion = "maximum_hit_damage_before_conversion"
	statMinimumHitDamage                 = "minimum_hit_damage"
	statMaximumHitDamage                 = "maximum_hit_damage"
)

// conversionOrder is the order damage converts in: a type converts only to the types after it.
// It follows PoB (CalcOffence.lua dmgTypeList); the game data does not hold it.
func conversionOrder() []DamageType {
	return []DamageType{Physical, Lightning, Cold, Fire, Chaos}
}

// Names of a skill's conversion stats to destination type d. Each is read in the context of
// the source type, so a table modifier names the source as its damage context.
func skillConversionStat(d DamageType) string { return "skill_damage_converted_to_" + d.String() }
func conversionStat(d DamageType) string      { return "damage_converted_to_" + d.String() }
func gainStat(d DamageType) string            { return "damage_gained_as_" + d.String() }

// conversionDestinations are the types damage can convert to or be gained as.
func conversionDestinations() []DamageType { return conversionOrder()[1:] }

// conversionStats are the engine stats of conversion and gain, in registration order.
func conversionStats() []string {
	const perDestination = 3 // the skill's conversion, other conversion and gain
	out := make([]string, 0, perDestination*len(conversionDestinations()))
	for _, d := range conversionDestinations() {
		out = append(out, skillConversionStat(d), conversionStat(d), gainStat(d))
	}
	return out
}

// conversionIDs are the ids of the conversion stats by destination type.
type conversionIDs struct {
	skill, global, gain [Chaos + 1]core.StatID
}

func newConversionIDs(ids map[string]core.StatID) conversionIDs {
	var c conversionIDs
	for _, d := range conversionDestinations() {
		c.skill[d], c.global[d], c.gain[d] = mustID(ids, skillConversionStat(d)), mustID(ids, conversionStat(d)), mustID(ids, gainStat(d))
	}
	return c
}

// conversionRates are the shares of one damage type that go to each later type, as fractions:
// converted by the skill, converted otherwise, and gained as extra; keep is what stays.
type conversionRates struct {
	skill, global, gain [Chaos + 1]float64
	keep                float64
}

// to returns the share of the source that damage type d receives.
func (r conversionRates) to(d DamageType) float64 { return r.skill[d] + r.global[d] + r.gain[d] }

// rates returns the conversion rates of src, with read reading a stat in src's context.
//
// Each rate counts from 0. The skill's own conversion applies first: above 100% it is scaled
// to 100% and no other conversion applies; otherwise other conversion above what is left is
// scaled down to it. Gain as extra is not limited. All follow PoB (CalcOffence.lua
// buildConversionTable); the game data does not hold them.
func (c conversionIDs) rates(src DamageType, read func(core.StatID) float64) conversionRates {
	r := conversionRates{keep: 1}
	i := slices.Index(conversionOrder(), src)
	if i < 0 {
		return r
	}
	skillTotal, globalTotal := 0.0, 0.0
	for _, d := range conversionOrder()[i+1:] {
		r.skill[d] = max(read(c.skill[d]), 0) / percentScale
		r.global[d] = max(read(c.global[d]), 0) / percentScale
		r.gain[d] = max(read(c.gain[d]), 0) / percentScale
		skillTotal += r.skill[d]
		globalTotal += r.global[d]
	}
	switch {
	case skillTotal > 1:
		for d := range r.skill {
			r.skill[d] /= skillTotal
			r.global[d] = 0
		}
	case skillTotal+globalTotal > 1:
		for d := range r.global {
			r.global[d] *= (1 - skillTotal) / globalTotal
		}
	}
	r.keep = 1 - min(skillTotal+globalTotal, 1)
	return r
}

// damageMasks are a damage dimension's bits, each type's and all of them.
type damageMasks struct {
	dim core.Dimension
	all core.Mask
}

func newDamageMasks() damageMasks {
	dim := damageDimension()
	m := damageMasks{dim: dim}
	for d := Physical; d <= Chaos; d++ {
		m.all = m.all.Or(damageBit(dim, d))
	}
	return m
}

// first returns the earliest type in conversion order of mask m, and false when it has none.
func (m damageMasks) first(mask core.Mask) (DamageType, bool) {
	for _, d := range conversionOrder() {
		if mask.Intersects(damageBit(m.dim, d)) {
			return d, true
		}
	}
	return 0, false
}

// only returns mask with d as its only damage type.
func (m damageMasks) only(mask core.Mask, d DamageType) core.Mask {
	return mask.AndNot(m.all).Or(damageBit(m.dim, d))
}

// convertedHit is a hit's smallest or largest damage of one type before any of it converts to
// later types, read in the context of a damage chain: the type is the chain's earliest, and
// the chain holds every type it is converted to on the way to the type read.
//
// It is the type's own damage (hitDamage) taken by the damage modifier of the whole chain,
// plus what each earlier type converts or gives to it. That share is the earlier type's
// damage before conversion, read with the earlier type added to the chain, so it takes the
// modifiers of every type it passes through. Shares from earlier types are rounded half up
// when both the smallest and largest are non-zero. Without damage of its own, the type's
// damage is the shares alone, without its modifiers again. Order and rounding follow PoB
// (CalcOffence.lua calcDamage); the game text gives none of them.
type convertedHit struct {
	own hitDamage
	// minimum and maximum are the smallest and largest damage before conversion; isMaximum
	// tells which this is.
	minimum, maximum core.StatID
	isMaximum        bool
	// ownMinimum and ownMaximum are the smallest and largest own damage, which decide
	// whether the type has damage of its own.
	ownMinimum, ownMaximum hitDamage
	conversion             conversionIDs
	masks                  damageMasks
}

func newConvertedHit(ids map[string]core.StatID, isMaximum bool) convertedHit {
	h := convertedHit{
		minimum: mustID(ids, statMinimumHitDamageBeforeConversion), maximum: mustID(ids, statMaximumHitDamageBeforeConversion),
		isMaximum: isMaximum, ownMinimum: newHitDamage(ids, false), ownMaximum: newHitDamage(ids, true),
		conversion: newConversionIDs(ids), masks: newDamageMasks(),
	}
	h.own = h.ownMinimum
	if isMaximum {
		h.own = h.ownMaximum
	}
	return h
}

// share is one earlier type's share of the damage read, before rounding.
type share struct {
	from             DamageType
	rates            conversionRates
	minimum, maximum float64
}

// shares returns what earlier types give type d in mask, with stat reading a stat in a mask.
func (h convertedHit) shares(d DamageType, mask core.Mask, stat func(core.StatID, core.Mask) float64) []share {
	var out []share
	for _, from := range conversionOrder() {
		if from == d {
			break
		}
		at := h.masks.only(mask, from)
		r := h.conversion.rates(from, func(s core.StatID) float64 { return stat(s, at) })
		rate := r.to(d)
		if rate <= 0 {
			continue
		}
		chain := mask.Or(damageBit(h.masks.dim, from))
		out = append(out, share{from: from, rates: r, minimum: rate * stat(h.minimum, chain), maximum: rate * stat(h.maximum, chain)})
	}
	return out
}

// added totals shares, the smallest and largest, rounded half up when both are non-zero.
func added(shares []share) (minimum, maximum float64) {
	for _, s := range shares {
		minimum += s.minimum
		maximum += s.maximum
	}
	if minimum != 0 && maximum != 0 {
		minimum, maximum = roundHalf(minimum), roundHalf(maximum)
	}
	return minimum, maximum
}

// value returns the damage in mask of type d, with stat reading a stat in a mask.
func (h convertedHit) value(d DamageType, mask core.Mask, stat func(core.StatID, core.Mask) float64) float64 {
	addMin, addMax := added(h.shares(d, mask, stat))
	add := addMin
	if h.isMaximum {
		add = addMax
	}
	at := h.masks.only(mask, d)
	read := func(s core.StatID) float64 { return stat(s, at) }
	if h.ownMinimum.unscaled(read) == 0 && h.ownMaximum.unscaled(read) == 0 {
		return add
	}
	return roundHalf(h.own.unscaled(read)*stat(h.own.damage, mask)/percentScale*read(h.own.extreme)/percentScale + add)
}

func (h convertedHit) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		d, ok := h.masks.first(r.Mask())
		if !ok {
			return 0
		}
		return h.value(d, r.Mask(), r.StatAt)
	}
}

// attribute credits the type's own base and added damage with the chain's damage modifier and
// the smallest or largest damage's over them, and each share from an earlier type to the
// sources of the conversion or gain that gives it, by their part of the rate; what the base
// damage multiplier, damage effectiveness and rounding change goes to the damage
// effectiveness's sources.
func (h convertedHit) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		mask := a.Mask()
		d, ok := h.masks.first(mask)
		if !ok {
			return core.Attribution{Amount: total}
		}
		stat := func(s core.StatID, m core.Mask) float64 { return a.Read(a.RefAt(s, m)) }
		at := h.masks.only(mask, d)
		var parts []core.Part
		for _, s := range []core.StatID{h.own.base, h.own.added} {
			if v := stat(s, at); v != 0 {
				parts = append(parts, a.Spread(a.RefAt(s, at), v, core.OpBase)...)
			}
		}
		parts = a.LayersOver(a.RefAt(h.own.extreme, at), a.LayersOver(core.SelfRef(h.own.damage), parts))
		for _, s := range h.shares(d, mask, stat) {
			v := s.minimum
			if h.isMaximum {
				v = s.maximum
			}
			from := h.masks.only(mask, s.from)
			rate := s.rates.to(d)
			for _, c := range []struct {
				stat core.StatID
				part float64
			}{{h.conversion.skill[d], s.rates.skill[d]}, {h.conversion.global[d], s.rates.global[d]}, {h.conversion.gain[d], s.rates.gain[d]}} {
				if c.part != 0 {
					parts = append(parts, a.Spread(a.RefAt(c.stat, from), v*c.part/rate, core.OpBase)...)
				}
			}
		}
		return core.Attribution{Parts: rest(a, parts, total, a.RefAt(h.own.effectiveness, at)), Amount: total}
	}
}

// Formula names shown in explanations.
const (
	formulaHitDamage           = "hit damage of a damage type"
	formulaHitBeforeConversion = "hit damage of a damage type before conversion"
)

// mechanicConversion names the conversion mechanic for those that come after it.
const mechanicConversion = "conversion"

func conversionMechanic() mechanic {
	return mechanic{
		name: mechanicConversion, after: []string{mechanicDamage},
		stats: slices.Concat(conversionStats(), []string{
			statMinimumHitDamageBeforeConversion,
			statMaximumHitDamageBeforeConversion,
			statMinimumHitDamage,
			statMaximumHitDamage,
		}),
		def:      conversionDef,
		complete: func(b *core.ModelBuilder, r registered) { addConversionFormulas(b, r.ids) },
	}
}

// conversionDef completes def when it is a stat of a hit's damage with conversion.
func conversionDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statMinimumHitDamageBeforeConversion, statMaximumHitDamageBeforeConversion:
		// The formula reads both stats, so addConversionFormulas sets it once they are registered.
		def.HasMin = true
	case statMinimumHitDamage, statMaximumHitDamage:
		before := statMinimumHitDamageBeforeConversion
		if def.Name == statMaximumHitDamage {
			before = statMaximumHitDamageBeforeConversion
		}
		h := newConvertedAwayHit(ids, before)
		def.HasMin = true
		def.FormulaName, def.Formula, def.Attribute = formulaHitDamage, h.formula(), h.attribute()
	}
	return def
}

// addConversionFormulas sets the formulas of the hit damage before conversion, which read each
// other.
func addConversionFormulas(b *core.ModelBuilder, ids map[string]core.StatID) {
	for _, name := range []string{statMinimumHitDamageBeforeConversion, statMaximumHitDamageBeforeConversion} {
		h := newConvertedHit(ids, name == statMaximumHitDamageBeforeConversion)
		id := mustID(ids, name)
		if err := b.SetFormula(id, h.formula()); err != nil {
			panic(err)
		}
		if err := b.SetAttribute(id, formulaHitBeforeConversion, h.attribute()); err != nil {
			panic(err)
		}
	}
}

// convertedAwayHit is a hit's smallest or largest damage of one type: its damage before
// conversion less what converts to later types. The type is the earliest of the context's
// damage types, as for convertedHit.
type convertedAwayHit struct {
	before     core.StatID
	conversion conversionIDs
	masks      damageMasks
}

func newConvertedAwayHit(ids map[string]core.StatID, before string) convertedAwayHit {
	return convertedAwayHit{before: mustID(ids, before), conversion: newConversionIDs(ids), masks: newDamageMasks()}
}

// rates returns the conversion rates of the type read in mask, and false when mask has none.
func (h convertedAwayHit) rates(mask core.Mask, stat func(core.StatID, core.Mask) float64) (DamageType, conversionRates, bool) {
	d, ok := h.masks.first(mask)
	if !ok {
		return 0, conversionRates{}, false
	}
	at := h.masks.only(mask, d)
	return d, h.conversion.rates(d, func(s core.StatID) float64 { return stat(s, at) }), true
}

func (h convertedAwayHit) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		_, rates, ok := h.rates(r.Mask(), r.StatAt)
		if !ok {
			return 0
		}
		return r.Stat(h.before) * rates.keep
	}
}

// attribute credits the damage before conversion, and what converts away as a negative more
// part to the sources of each conversion, by its part of the converted share.
func (h convertedAwayHit) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		parts, before := a.Of(core.SelfRef(h.before))
		d, rates, ok := h.rates(a.Mask(), func(s core.StatID, m core.Mask) float64 { return a.Read(a.RefAt(s, m)) })
		lost := before * (rates.keep - 1)
		if !ok || lost == 0 {
			return core.Attribution{Parts: parts, Amount: total}
		}
		converted := 1 - rates.keep
		at := h.masks.only(a.Mask(), d)
		for _, to := range conversionDestinations() {
			for _, c := range []struct {
				stat core.StatID
				part float64
			}{{h.conversion.skill[to], rates.skill[to]}, {h.conversion.global[to], rates.global[to]}} {
				if c.part != 0 {
					parts = append(parts, a.Spread(a.RefAt(c.stat, at), lost*c.part/converted, core.OpMore)...)
				}
			}
		}
		return core.Attribution{Parts: parts, Amount: total}
	}
}
