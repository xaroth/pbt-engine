// Package core is the game-independent evaluator: stats, modifiers, contexts, actors and lazy,
// memoised, incremental evaluation. Rules packages register stats, context dimensions and
// formulas on a ModelBuilder; a Session holds one build's modifiers over the built Model and
// evaluates it.
//
// Results are deterministic: modifiers are visited in a fixed order and no float operation is
// left to be fused, so the same input gives the same bits on every platform.
package core

import (
	"fmt"
	"maps"
	"math"
	"slices"
)

// StatID identifies an engine stat within a Model.
type StatID uint32

// Rounding is applied to a stat's value after clamping.
type Rounding uint8

// Rounding modes.
const (
	RoundNone Rounding = iota
	// RoundHalf is floor(x + 0.5): halves round up, for negative values too. That this is the
	// game's rounding is our assumption, unvalidated.
	RoundHalf
	RoundFloor // towards negative infinity
	RoundTrunc // towards zero
)

// Agg is the aggregation of the modifiers matching one node.
type Agg struct {
	Base, Inc   float64 // sums
	More        float64 // product of (1 + v/100), 1 when none
	Flag        bool    // any OpFlag modifier matched
	Override    float64 // value of the last matching override
	HasOverride bool
}

// Default is the default aggregation: Base * (1 + Inc/100) * More.
func (a Agg) Default() float64 {
	return a.Base * (1.0 + a.Inc/100.0) * a.More
}

// Formula computes a node's value before clamping and rounding. It is not called when an
// override matched. Reads through r are recorded as dependencies of the node.
type Formula func(r Reader, a Agg) float64

// StatDef describes an engine stat.
type StatDef struct {
	Name     string
	Round    Rounding
	Places   int8 // decimal places kept by Round; 0 rounds to integers
	Min, Max float64
	HasMin   bool
	HasMax   bool
	// Includes lists stats whose modifiers also apply to this stat
	// (fire resistance includes elemental resistance).
	Includes []StatID
	// Flag stats are 1 when any OpFlag modifier matches, else 0, unless a formula says otherwise.
	Flag bool
	// CycleFamily groups stats whose conditions and scales read each other in cycles
	// (attributes granting attributes); 0 is no family. A condition or scale of a stat that
	// reads a stat of the same family sees the unconditional value: that node evaluated
	// without any conditional or scaled modifier. An unconditional node still runs its
	// formula, so formulas of one family that read each other are a CycleError. This is our
	// working policy until the game's resolution is known per family; it is unvalidated.
	CycleFamily uint8
	// Formula replaces the default aggregation; nil uses Agg.Default (or the flag rule).
	Formula Formula
}

// Bit is one value of a context dimension.
type Bit uint16

// MaxBits is the number of context bits a Model can register over all dimensions.
const MaxBits = 256

// Mask is a set of context bits.
type Mask [MaxBits / 64]uint64

// Set returns m with bit b set.
func (m Mask) Set(b Bit) Mask {
	m[b>>6] |= 1 << (b & 63)
	return m
}

// Has reports whether bit b is set.
func (m Mask) Has(b Bit) bool { return m[b>>6]&(1<<(b&63)) != 0 }

// Intersects reports whether m and o share a bit.
func (m Mask) Intersects(o Mask) bool {
	return m[0]&o[0] != 0 || m[1]&o[1] != 0 || m[2]&o[2] != 0 || m[3]&o[3] != 0
}

// IsZero reports whether no bit is set.
func (m Mask) IsZero() bool { return m == Mask{} }

// MaskOf returns the mask with the given bits set.
func MaskOf(bits ...Bit) Mask {
	var m Mask
	for _, b := range bits {
		m = m.Set(b)
	}
	return m
}

// Dimension is a typed context dimension (damage type, delivery, skill type).
type Dimension struct {
	Name   string
	First  Bit
	Values []string
}

// Bit returns the bit of the value at index i.
func (d Dimension) Bit(i int) Bit { return d.First + Bit(i) }

// ModelBuilder registers stats and context dimensions. The zero value is ready to use.
type ModelBuilder struct {
	stats    []StatDef
	byName   map[string]StatID
	dimNames map[string]bool
	nBits    int
	bitNames map[string]Bit // "dimension.value"
}

// AddStat registers a stat. Names are unique; includes must name stats registered earlier.
func (b *ModelBuilder) AddStat(def StatDef) (StatID, error) {
	if _, dup := b.byName[def.Name]; dup {
		return 0, fmt.Errorf("%w: stat %q", ErrDuplicateName, def.Name)
	}
	if def.Round > RoundTrunc {
		return 0, fmt.Errorf("%w: stat %q: rounding %d", ErrOutOfRange, def.Name, def.Round)
	}
	if def.Places < 0 || def.Places > maxPlaces {
		return 0, fmt.Errorf("%w: stat %q: places %d, want 0 to %d", ErrOutOfRange, def.Name, def.Places, maxPlaces)
	}
	id := StatID(len(b.stats))
	for _, inc := range def.Includes {
		if inc >= id {
			return 0, fmt.Errorf("%w: stat %q includes stat %d", ErrBadHandle, def.Name, inc)
		}
	}
	if b.byName == nil {
		b.byName = map[string]StatID{}
	}
	def.Includes = slices.Clone(def.Includes)
	b.stats = append(b.stats, def)
	b.byName[def.Name] = id
	return id, nil
}

// MustAddStat is AddStat that panics on error, for rules' static registration.
func (b *ModelBuilder) MustAddStat(def StatDef) StatID {
	id, err := b.AddStat(def)
	if err != nil {
		panic(err)
	}
	return id
}

// SetFormula sets the formula of a registered stat, so formulas may read stats registered
// after it.
func (b *ModelBuilder) SetFormula(id StatID, f Formula) error {
	if int(id) >= len(b.stats) {
		return fmt.Errorf("%w: stat %d", ErrBadHandle, id)
	}
	b.stats[id].Formula = f
	return nil
}

// AddDimension registers a context dimension with its values, each getting one bit.
func (b *ModelBuilder) AddDimension(name string, values ...string) (Dimension, error) {
	if b.nBits+len(values) > MaxBits {
		return Dimension{}, fmt.Errorf("%w: dimension %q exceeds %d context bits", ErrOutOfRange, name, MaxBits)
	}
	if b.dimNames[name] {
		return Dimension{}, fmt.Errorf("%w: dimension %q", ErrDuplicateName, name)
	}
	d := Dimension{Name: name, First: Bit(b.nBits), Values: slices.Clone(values)}
	keys := map[string]Bit{}
	for i, v := range values {
		key := name + "." + v
		_, earlier := b.bitNames[key] // dimension "a.b" value "c" and "a" value "b.c" share a key
		if _, dup := keys[key]; dup || earlier {
			return Dimension{}, fmt.Errorf("%w: dimension %q value %q", ErrDuplicateName, name, v)
		}
		keys[key] = d.Bit(i)
	}
	if b.dimNames == nil {
		b.dimNames = map[string]bool{}
		b.bitNames = map[string]Bit{}
	}
	b.dimNames[name] = true
	maps.Copy(b.bitNames, keys)
	b.nBits += len(values)
	return d, nil
}

// Build returns a Model of the stats and dimensions registered so far. The model does not
// change when the builder registers more.
func (b *ModelBuilder) Build() *Model {
	return &Model{
		stats:    slices.Clone(b.stats),
		byName:   maps.Clone(b.byName),
		bitNames: maps.Clone(b.bitNames),
	}
}

// Model holds stat definitions and context dimensions. It is immutable, so any number of
// sessions and goroutines can share it. The zero Model has no stats and no dimensions.
type Model struct {
	stats    []StatDef
	byName   map[string]StatID
	bitNames map[string]Bit
}

// Stat looks up a stat by name.
func (m *Model) Stat(name string) (StatID, bool) {
	id, ok := m.byName[name]
	return id, ok
}

// StatDef returns a copy of a stat's definition, and false for an unknown id.
func (m *Model) StatDef(id StatID) (StatDef, bool) {
	if int(id) >= len(m.stats) {
		return StatDef{}, false
	}
	def := m.stats[id]
	def.Includes = slices.Clone(def.Includes)
	return def, true
}

// NumStats returns the number of stats; ids run from 0 to NumStats()-1.
func (m *Model) NumStats() int { return len(m.stats) }

// Bit looks up a context bit by "dimension.value".
func (m *Model) Bit(name string) (Bit, bool) {
	b, ok := m.bitNames[name]
	return b, ok
}

func (d *StatDef) finish(r float64) float64 {
	if d.HasMin {
		r = fmax(r, d.Min)
	}
	if d.HasMax {
		r = fmin(r, d.Max)
	}
	if d.Round != RoundNone {
		p := math.Pow10(int(d.Places))
		x := r
		if d.Places != 0 {
			x = float64(r * p)
		}
		//exhaustive:ignore // RoundNone is excluded by the enclosing if
		switch d.Round {
		case RoundHalf:
			x = math.Floor(x + 0.5)
		case RoundFloor:
			x = math.Floor(x)
		case RoundTrunc:
			x = math.Trunc(x)
		}
		if d.Places != 0 {
			x /= p
		}
		r = x
	}
	return r + 0.0 // normalise -0
}

// maxPlaces is the most decimal places a stat can round to.
const maxPlaces = 15

// fmin and fmax pick the first argument on ties, so signed zeros are deterministic.
func fmin(a, b float64) float64 {
	if b < a {
		return b
	}
	return a
}

func fmax(a, b float64) float64 {
	if b > a {
		return b
	}
	return a
}
