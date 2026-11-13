// Package timeless computes what a timeless jewel makes of a passive in its radius: for a jewel's
// alternate tree version, seed and conqueror, the replacement passive or the added stats the game
// picks, with the values it rolls for them.
//
// It takes the game's alternate tree tables (AlternateTreeVersions, AlternatePassiveSkills,
// AlternatePassiveAdditions) as plain values and names its picks by their position in them. Which
// nodes lie in the radius, and what the picks' stats mean, is for the caller.
package timeless

import (
	"errors"
	"fmt"
)

// Kind is a passive's type as the alternate tree tables number it (their PassiveType column).
type Kind uint8

// The kinds of passive.
const (
	// SmallAttribute is a small passive whose only stat is +Strength, +Dexterity or +Intelligence.
	SmallAttribute Kind = iota + 1
	SmallNormal
	Notable
	Keystone
	JewelSocket
)

// SmallKind gives the kind of a small passive from its stat ids.
func SmallKind(statIDs []string) Kind {
	if len(statIDs) == 1 {
		switch statIDs[0] {
		case "additional_strength", "additional_dexterity", "additional_intelligence":
			return SmallAttribute
		}
	}
	return SmallNormal
}

// Range is an inclusive range of values.
type Range struct {
	Min, Max int32
}

// Version is an AlternateTreeVersions row: what a jewel of this version does to each kind of
// passive.
type Version struct {
	// ID is the row's id, such as "Vaal". Elegant Hubris ("Eternal") seeds step by 20.
	ID                      string
	SmallAttributeReplaced  bool
	SmallNormalReplaced     bool
	SmallAttributeAdditions Range
	SmallNormalAdditions    Range
	NotableAdditions        Range
	// NotableReplacementChance is the chance in percent that a notable is replaced rather than
	// added to.
	NotableReplacementChance int32
}

// Replacement is an AlternatePassiveSkills row of the version.
type Replacement struct {
	Kinds []Kind
	// Stats holds the value range of each stat, in the row's stat order; at most 4.
	Stats  []Range
	Weight uint32
	// Additions is added to the version's addition count for the replaced passive's kind
	// (RandomMin and RandomMax). Zero means the replacement gets no additions.
	Additions Range
	// Conqueror is set on keystones: the conqueror whose keystone this is.
	Conqueror Conqueror
}

// Addition is an AlternatePassiveAdditions row of the version.
type Addition struct {
	Kinds []Kind
	// Stats holds the value range of each stat, in the row's stat order; at most 2.
	Stats  []Range
	Weight uint32
}

// Conqueror names the conqueror on a jewel as the alternate passive keystones number them.
type Conqueror struct {
	Index, Version uint32
}

// Tables is one version's rows: the version and, in table order, its replacements and additions.
type Tables struct {
	Version      Version
	Replacements []Replacement
	Additions    []Addition
}

// Jewel is a timeless jewel as its mod gives it.
type Jewel struct {
	// Seed is the seed the jewel shows.
	Seed      uint32
	Conqueror Conqueror
}

// Node is a passive in the jewel's radius.
type Node struct {
	// ID is the passive's node id (its PassiveSkillGraphId).
	ID   uint32
	Kind Kind
}

// MaxAdditions bounds how many additions one passive gets.
const MaxAdditions = 8

// Result is what the jewel makes of a passive.
type Result struct {
	Replaced bool
	// Replacement is the index of the replacing row in Tables.Replacements, or -1.
	Replacement int
	// Rolls holds the replacement's stat values, in its stat order.
	Rolls     [4]int32
	additions [MaxAdditions]Rolled
	count     int
}

// Rolled is an addition with its stat values.
type Rolled struct {
	// Addition is the index of the row in Tables.Additions.
	Addition int
	Rolls    [2]int32
}

// Additions gives the added stats, in the order the game rolls them.
func (r *Result) Additions() []Rolled {
	return r.additions[:r.count]
}

// ErrTables reports tables NewTree cannot use.
var ErrTables = errors.New("invalid timeless tables")

const (
	// eternalSeedStep is how much larger the seeds Elegant Hubris shows are than the ones it
	// draws from.
	eternalSeedStep = 20
	certain         = 100
)

// Tree is a version ready to conquer passives.
type Tree struct {
	tables       Tables
	eternal      bool
	replacements [JewelSocket][]int
	// replacementWeights holds the weights of replacements, in the same order.
	replacementWeights [JewelSocket][]uint32
	additions          [JewelSocket][]int
	additionWeight     [JewelSocket]uint32
}

// NewTree checks a version's tables and prepares them.
func NewTree(t Tables) (*Tree, error) {
	tree := &Tree{tables: t, eternal: t.Version.ID == "Eternal"}
	for i, r := range t.Replacements {
		if err := checkRow(r.Kinds, len(r.Stats), len(Result{}.Rolls)); err != nil {
			return nil, fmt.Errorf("replacement %d: %w", i, err)
		}
		for _, k := range r.Kinds {
			tree.replacements[k-1] = append(tree.replacements[k-1], i)
			tree.replacementWeights[k-1] = append(tree.replacementWeights[k-1], r.Weight)
		}
	}
	for i, a := range t.Additions {
		if err := checkRow(a.Kinds, len(a.Stats), len(Rolled{}.Rolls)); err != nil {
			return nil, fmt.Errorf("addition %d: %w", i, err)
		}
		for _, k := range a.Kinds {
			tree.additions[k-1] = append(tree.additions[k-1], i)
			tree.additionWeight[k-1] += a.Weight
		}
	}
	for k := SmallAttribute; k <= Notable; k++ {
		if err := tree.checkAdditions(k); err != nil {
			return nil, err
		}
	}
	return tree, nil
}

func checkRow(kinds []Kind, stats, most int) error {
	if stats > most {
		return fmt.Errorf("%w: %d stats, at most %d", ErrTables, stats, most)
	}
	if len(kinds) == 0 {
		return fmt.Errorf("%w: no kinds", ErrTables)
	}
	for _, k := range kinds {
		if k < SmallAttribute || k > JewelSocket {
			return fmt.Errorf("%w: kind %d is unknown", ErrTables, k)
		}
	}
	return nil
}

// Conquer gives what the jewel makes of a passive. A jewel socket stays as it is, and so does a
// keystone when the version has no keystone for the jewel's conqueror.
func (t *Tree) Conquer(j Jewel, n Node) Result {
	switch n.Kind {
	case Keystone:
		return t.keystone(j.Conqueror)
	case SmallAttribute, SmallNormal, Notable:
	default:
		return Result{Replacement: -1}
	}

	seed := j.Seed
	if t.eternal {
		seed /= eternalSeedStep
	}
	g := newRNG(n.ID, seed)
	res := Result{Replacement: -1}
	count := t.additionRange(n.Kind)
	if t.replaced(&g, n.Kind) {
		var ok bool
		if count, ok = t.replace(&g, n.Kind, &res); !ok {
			return res
		}
	}
	for range roll(&g, count) {
		res.additions[res.count] = t.addition(&g, n.Kind)
		res.count++
	}
	return res
}

// replaced tells whether the passive is replaced rather than added to.
func (t *Tree) replaced(g *rng, k Kind) bool {
	v := &t.tables.Version
	switch k {
	case Notable:
		// Drawn whether or not the version leaves the choice to chance.
		draw := g.between(0, certain)
		return v.NotableReplacementChance >= certain || draw < v.NotableReplacementChance
	case SmallAttribute:
		return v.SmallAttributeReplaced
	default:
		return v.SmallNormalReplaced
	}
}

// replace picks and rolls the replacement into res, and gives how many additions follow it;
// false when none follow.
func (t *Tree) replace(g *rng, k Kind, res *Result) (Range, bool) {
	at := g.pick(t.replacementWeights[k-1])
	if at < 0 {
		return Range{}, false
	}
	res.Replaced, res.Replacement = true, t.replacements[k-1][at]
	r := &t.tables.Replacements[res.Replacement]
	for s, rng := range r.Stats {
		res.Rolls[s] = roll(g, rng)
	}
	if r.Additions == (Range{}) {
		return Range{}, false
	}
	count := t.additionRange(k)
	return Range{Min: count.Min + r.Additions.Min, Max: count.Max + r.Additions.Max}, true
}

func (t *Tree) checkAdditions(k Kind) error {
	most := t.additionRange(k).Max
	for _, i := range t.replacements[k-1] {
		most = max(most, t.additionRange(k).Max+t.tables.Replacements[i].Additions.Max)
	}
	if most > MaxAdditions {
		return fmt.Errorf("%w: kind %d can get %d additions, at most %d", ErrTables, k, most, MaxAdditions)
	}
	if most > 0 && t.additionWeight[k-1] == 0 {
		return fmt.Errorf("%w: kind %d can get additions but no addition of weight above 0 applies", ErrTables, k)
	}
	return nil
}

func (t *Tree) keystone(c Conqueror) Result {
	for _, i := range t.replacements[Keystone-1] {
		r := &t.tables.Replacements[i]
		if r.Conqueror != c {
			continue
		}
		res := Result{Replaced: true, Replacement: i}
		if len(r.Stats) > 0 {
			res.Rolls[0] = r.Stats[0].Min
		}
		return res
	}
	return Result{Replacement: -1}
}

func (t *Tree) additionRange(k Kind) Range {
	v := &t.tables.Version
	switch k {
	case SmallAttribute:
		return v.SmallAttributeAdditions
	case Notable:
		return v.NotableAdditions
	default:
		return v.SmallNormalAdditions
	}
}

func (t *Tree) addition(g *rng, k Kind) Rolled {
	draw := g.below(t.additionWeight[k-1])
	pick := -1
	for _, i := range t.additions[k-1] {
		w := t.tables.Additions[i].Weight
		if draw < w {
			pick = i
			break
		}
		draw -= w
	}
	out := Rolled{Addition: pick}
	for s, rng := range t.tables.Additions[pick].Stats {
		out.Rolls[s] = roll(g, rng)
	}
	return out
}

func roll(g *rng, r Range) int32 {
	if r.Max > r.Min {
		return g.between(r.Min, r.Max)
	}
	return r.Min
}
