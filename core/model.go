// Package core is the game-independent evaluator: stats, modifiers, contexts, actors, lazy
// memoised evaluation, incremental edits and revert. Rules packages register stats, context
// dimensions and formulas on a Model; a Session holds one build's modifiers and evaluates it.
//
// Results are deterministic: modifiers are visited in a fixed order and no float operation is
// left to be fused, so the same input gives the same bits on every platform.
package core

import (
	"fmt"
	"math"
)

// StatID identifies an engine stat within a Model.
type StatID uint32

// Rounding is applied to a stat's value after clamping.
type Rounding uint8

// Rounding modes.
const (
	RoundNone  Rounding = iota
	RoundHalf           // floor(x + 0.5), PoE's round for positive and negative values
	RoundFloor          // towards negative infinity
	RoundTrunc          // towards zero
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
	// CycleBreak marks a stat family whose conditional reads of each other are resolved by
	// the game without recursion (attributes granting attributes). A condition or scale on a
	// modifier of a CycleBreak stat that reads another CycleBreak stat sees the unconditional
	// value: the same node evaluated without any conditional or scaled modifier.
	CycleBreak bool
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
func (d *Dimension) Bit(i int) Bit { return d.First + Bit(i) }

// Model holds stat definitions and context dimensions. It is built once by rules, then shared
// read-only by any number of sessions; it must not change after the first session is created.
type Model struct {
	stats    []StatDef
	byName   map[string]StatID
	dims     []*Dimension
	nBits    int
	frozen   bool
	bitNames map[string]Bit // "dimension.value"
}

// NewModel returns an empty model.
func NewModel() *Model {
	return &Model{byName: map[string]StatID{}, bitNames: map[string]Bit{}}
}

func (m *Model) mutable() {
	if m.frozen {
		panic("core: model changed after a session was created")
	}
}

// AddStat registers a stat. Names are unique.
func (m *Model) AddStat(def StatDef) (StatID, error) {
	m.mutable()
	if _, dup := m.byName[def.Name]; dup {
		return 0, fmt.Errorf("core: stat %q registered twice", def.Name)
	}
	if def.Places < 0 || def.Places > 15 {
		return 0, fmt.Errorf("core: stat %q: places %d out of range", def.Name, def.Places)
	}
	id := StatID(len(m.stats))
	for _, inc := range def.Includes {
		if inc >= id {
			return 0, fmt.Errorf("core: stat %q includes unknown stat %d", def.Name, inc)
		}
	}
	m.stats = append(m.stats, def)
	m.byName[def.Name] = id
	return id, nil
}

// MustAddStat is AddStat that panics on error, for rules' static registration.
func (m *Model) MustAddStat(def StatDef) StatID {
	id, err := m.AddStat(def)
	if err != nil {
		panic(err)
	}
	return id
}

// SetFormula sets the formula of a registered stat, so formulas may refer to stats
// registered after it.
func (m *Model) SetFormula(id StatID, f Formula) {
	m.mutable()
	m.stats[id].Formula = f
}

// Stat looks up a stat by name.
func (m *Model) Stat(name string) (StatID, bool) {
	id, ok := m.byName[name]
	return id, ok
}

// StatDef returns the definition of a stat.
func (m *Model) StatDef(id StatID) *StatDef { return &m.stats[id] }

// NumStats returns the number of registered stats.
func (m *Model) NumStats() int { return len(m.stats) }

// AddDimension registers a context dimension with its values, each getting one bit.
func (m *Model) AddDimension(name string, values ...string) (*Dimension, error) {
	m.mutable()
	if m.nBits+len(values) > MaxBits {
		return nil, fmt.Errorf("core: dimension %q exceeds %d context bits", name, MaxBits)
	}
	for _, d := range m.dims {
		if d.Name == name {
			return nil, fmt.Errorf("core: dimension %q registered twice", name)
		}
	}
	d := &Dimension{Name: name, First: Bit(m.nBits), Values: values}
	for i, v := range values {
		key := name + "." + v
		if _, dup := m.bitNames[key]; dup {
			return nil, fmt.Errorf("core: dimension %q value %q registered twice", name, v)
		}
		m.bitNames[key] = d.Bit(i)
	}
	m.nBits += len(values)
	m.dims = append(m.dims, d)
	return d, nil
}

// Bit looks up a context bit by "dimension.value".
func (m *Model) Bit(name string) (Bit, bool) {
	b, ok := m.bitNames[name]
	return b, ok
}

// Dimensions returns the registered dimensions in registration order.
func (m *Model) Dimensions() []*Dimension { return m.dims }

func (d *StatDef) finish(r float64) float64 {
	if d.HasMin {
		r = fmax(r, d.Min)
	}
	if d.HasMax {
		r = fmin(r, d.Max)
	}
	if d.Round != RoundNone {
		p := pow10[d.Places]
		x := r
		if d.Places != 0 {
			x = float64(r * p)
		}
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

var pow10 = [16]float64{1, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9, 1e10, 1e11, 1e12, 1e13, 1e14, 1e15}

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
