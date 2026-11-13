package dataset

import (
	"cmp"
	"slices"
)

// NodeOverride is a tattoo or runegraft: stats that take the place of the stats of the
// allocated node it is put on. Its English name is in the names file, keyed by ID.
type NodeOverride struct {
	// ID is the game's PassiveSkillOverrides id, such as "Tasalio3".
	ID   string
	Kind OverrideKind
	// Stats are sorted by Stat, without duplicates.
	Stats []StatValue
	// Icon is the art path, as LayoutNode.Icon names it.
	Icon   string
	Target OverrideTarget
	// MinNeighbours and MaxNeighbours bound how many neighbours the replaced node has, from
	// the game's RequiresAdjacent and MaxAdjacent; 0 is no bound.
	MinNeighbours, MaxNeighbours int32
	// ExcludedNodes are nodes it may not replace, from the game's TattooBlockingPassive,
	// sorted.
	ExcludedNodes []uint32
}

// OverrideKind classifies node overrides.
type OverrideKind uint8

// Override kinds.
const (
	Tattoo OverrideKind = iota
	Runegraft
)

func (k OverrideKind) String() string {
	switch k {
	case Tattoo:
		return "tattoo"
	case Runegraft:
		return "runegraft"
	}
	return unknown
}

// OverrideTarget is the kind of node an override may replace.
type OverrideTarget uint8

// Override targets.
const (
	SmallStrength OverrideTarget = iota
	SmallDexterity
	SmallIntelligence
	// SmallAttribute is a small node of any one attribute.
	SmallAttribute
	// StrengthNotable, DexterityNotable and IntelligenceNotable are notables that grant +30
	// to the attribute.
	StrengthNotable
	DexterityNotable
	IntelligenceNotable
	KeystoneTarget
	MasteryTarget
	AscendancyNotable
)

func (t OverrideTarget) String() string {
	names := [...]string{
		"small strength", "small dexterity", "small intelligence", "small attribute",
		"strength notable", "dexterity notable", "intelligence notable", "keystone", "mastery", "ascendancy notable",
	}
	if int(t) < len(names) {
		return names[t]
	}
	return unknown
}

// Override returns the override of t with id.
func (t *Tree) Override(id string) (*NodeOverride, bool) {
	i, ok := slices.BinarySearchFunc(t.Overrides, id, func(o NodeOverride, id string) int { return cmp.Compare(o.ID, id) })
	if !ok {
		return nil, false
	}
	return &t.Overrides[i], true
}
