package poe1

import (
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/core"
)

// statDoubleHitsWhenDualWielding is a flag for a skill that hits with both hands at once.
const statDoubleHitsWhenDualWielding = "double_hits_when_dual_wielding"

// Hands is what the character attacks with: the weapon in each hand, nil for none, and in the
// main hand without a weapon the class's unarmed attacks. A shield or quiver is not a weapon.
type Hands struct {
	MainHand, OffHand *Weapon
}

// HandsOf returns what items put in the hands of a character of class ch: without a weapon in
// the main hand, its unarmed attacks (UnarmedWeapon), as PoB has them (CalcSetup.lua weaponData1).
func HandsOf(items []Item, ch *dataset.Character) Hands {
	var h Hands
	for _, it := range items {
		switch {
		case it.Node != 0:
		case it.Slot == build.SlotWeapon:
			h.MainHand = it.Weapon
		case it.Slot == build.SlotOffhand:
			h.OffHand = it.Weapon
		}
	}
	if h.MainHand == nil && ch != nil {
		h.MainHand = UnarmedWeapon(ch)
	}
	return h
}

// dualWielding reports whether the character holds a weapon in each hand, as PoB has it
// (CalcPerform.lua DualWielding): its bare main hand counts.
func (h Hands) dualWielding() bool { return h.MainHand != nil && h.OffHand != nil }

// bothHands reports whether an attack with types and weapon restrictions attacks with both
// hands: the off hand holds a weapon it can be used with and it is not a main hand only skill.
func (h Hands) bothHands(types, restrictions []string) bool {
	return h.OffHand != nil && h.OffHand.usableWith(restrictions) && !slices.Contains(types, typeMainHandOnly)
}

// handBits are the hand and weapon context bits of a sheet model, the same in every model as
// damageDimension's.
type handBits struct {
	main, off core.Mask
	// weapon and offHandWeapon are every bit of the weapon and off hand weapon dimensions.
	weapon, offHandWeapon core.Dimension
}

func handContext() handBits {
	var b core.ModelBuilder
	c := addContexts(&b)
	return handBits{
		main: core.MaskOf(c.hand.Bit(int(handMain))), off: core.MaskOf(c.hand.Bit(int(handOff))),
		weapon: c.weapon, offHandWeapon: c.offHandWeapon,
	}
}

// hands returns the main and off hand contexts of an attack read in m with both hands, and
// false when m is not such a context: the main hand's is m without the off hand weapon's kinds,
// the off hand's has those kinds as its weapon's and the off hand's bit for the main hand's.
func (h handBits) hands(m core.Mask) (main, off core.Mask, ok bool) {
	var offKinds, asWeapon, weapon core.Mask
	for k := range len(h.weapon.Values) {
		weapon = weapon.Set(h.weapon.Bit(k))
		if b := h.offHandWeapon.Bit(k); m.Has(b) {
			offKinds, asWeapon = offKinds.Set(b), asWeapon.Set(h.weapon.Bit(k))
		}
	}
	if offKinds.IsZero() {
		return m, m, false
	}
	main = m.AndNot(offKinds)
	return main, main.AndNot(weapon).AndNot(h.main).Or(asWeapon).Or(h.off), true
}

// combineMode is how an attack with both hands combines its hands' values of a stat.
type combineMode uint8

const (
	// combineAverage is their average: a chance of a use, which hits with one hand.
	combineAverage combineMode = iota
	// combineRate is their harmonic mean: uses per second, alternating hands.
	combineRate
	// combineHits is their average, or their sum for a skill that hits with both at once.
	combineHits
)

// weight is what each hand's value counts for in the combined value of a mode other than
// combineRate.
func (c combineMode) weight(double bool) float64 {
	if c == combineHits && double {
		return 1
	}
	return half
}

func (c combineMode) combine(main, off float64, double bool) float64 {
	if c != combineRate {
		return (main + off) * c.weight(double)
	}
	if main <= 0 || off <= 0 {
		return 0
	}
	return 1 / ((1/main + 1/off) * half)
}

func handsMechanic() mechanic {
	return mechanic{name: "hands", after: []string{mechanicHit, mechanicAccuracy}, stats: []string{statDoubleHitsWhenDualWielding}, def: handsDef}
}

// handsDef completes def when it is a stat an attack with both hands combines from its hands
// (handBits.hands): the chance to hit and that a use strikes critically average, hits per
// second are the harmonic mean, the average hit averages, or adds for a skill that hits with
// both at once, and hit DPS is the hands' average hit times chance to hit, combined as the
// average hit, times the combined hits per second. In a context of one hand def's own formula
// applies. All follow PoB (CalcOffence.lua combineStat: AVERAGE, HARMONICMEAN, and DPS for the
// average damage and DPS); the game text gives none of them.
func handsDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	var mode combineMode
	switch def.Name {
	case statDoubleHitsWhenDualWielding:
		def.Flag = true
		return def
	case statHitChance, statEffectiveCriticalStrikeChance:
		mode = combineAverage
	case statHitsPerSecond:
		mode = combineRate
	case statAverageHit:
		mode = combineHits
	case statHitDPS:
		h := handsDPS{
			average: mustID(ids, statAverageHit), chance: mustID(ids, statHitChance), rate: mustID(ids, statHitsPerSecond),
			double: mustID(ids, statDoubleHitsWhenDualWielding), hands: handContext(),
		}
		def.Formula, def.Attribute = h.formula(def.Formula), h.attribute(def.Attribute)
		return def
	default:
		return def
	}
	h := handsStat{double: mustID(ids, statDoubleHitsWhenDualWielding), mode: mode, hands: handContext()}
	def.Formula, def.Attribute = h.formula(def.Formula), h.attribute(def.Attribute)
	return def
}

// handsStat is a stat an attack with both hands combines from its hands' values of it.
type handsStat struct {
	double core.StatID
	mode   combineMode
	hands  handBits
}

func (h handsStat) formula(own core.Formula) core.Formula {
	return func(r core.Reader, agg core.Agg) float64 {
		main, off, ok := h.hands.hands(r.Mask())
		if !ok {
			return own(r, agg)
		}
		return h.mode.combine(r.StatAt(r.Self(), main), r.StatAt(r.Self(), off), r.Stat(h.double) != 0)
	}
}

// attribute credits each hand's sources in proportion to its value; in a context of one hand,
// the own rule.
func (h handsStat) attribute(own core.Attribute) core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		main, off, ok := h.hands.hands(a.Mask())
		if !ok {
			return own(a)
		}
		e := a.Explanation()
		total, stat := e.Total, e.Node.Stat
		mainParts, mv := a.Of(a.RefAt(stat, main))
		offParts, ov := a.Of(a.RefAt(stat, off))
		if mv+ov == 0 {
			return core.Attribution{Amount: total}
		}
		f := total / (mv + ov)
		return core.Attribution{Parts: append(scaled(mainParts, f), scaled(offParts, f)...), Amount: total}
	}
}

// handsDPS is the hit DPS of an attack with both hands.
type handsDPS struct {
	average, chance, rate, double core.StatID
	hands                         handBits
}

// landed returns each hand's average hit times its chance to hit, with stat reading a stat in a
// context.
func (h handsDPS) landed(main, off core.Mask, stat func(core.StatID, core.Mask) float64) (float64, float64) {
	return stat(h.average, main) * stat(h.chance, main) / percentScale, stat(h.average, off) * stat(h.chance, off) / percentScale
}

func (h handsDPS) formula(own core.Formula) core.Formula {
	return func(r core.Reader, agg core.Agg) float64 {
		main, off, ok := h.hands.hands(r.Mask())
		if !ok {
			return own(r, agg)
		}
		m, o := h.landed(main, off, r.StatAt)
		return combineHits.combine(m, o, r.Stat(h.double) != 0) * r.Stat(h.rate)
	}
}

// attribute credits each hand's average hit's sources with what its hits that land add at the
// combined hits per second, and the hits per second's sources with the rest; in a context of one
// hand, the own rule.
func (h handsDPS) attribute(own core.Attribute) core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		main, off, ok := h.hands.hands(a.Mask())
		if !ok {
			return own(a)
		}
		total := a.Explanation().Total
		w := combineHits.weight(a.Stat(h.double) != 0)
		var parts []core.Part
		for _, m := range []core.Mask{main, off} {
			avg, _ := a.Of(a.RefAt(h.average, m))
			parts = append(parts, scaled(avg, a.Read(a.RefAt(h.chance, m))/percentScale*w)...)
		}
		return core.Attribution{Parts: rest(a, parts, total, core.SelfRef(h.rate)), Amount: total}
	}
}
