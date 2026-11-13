package dataset

// TimelessVersion is an alternate tree version, the game's AlternateTreeVersions row, with the
// AlternatePassiveSkills and AlternatePassiveAdditions rows of that version: what a timeless
// jewel of the version may make of each passive in its radius.
type TimelessVersion struct {
	// ID is the row's id, such as "Vaal".
	ID                                                              string
	SmallAttributeReplaced, SmallNormalReplaced                     bool
	SmallAttributeAdditions, SmallNormalAdditions, NotableAdditions ValueRange
	// NotableReplacementChance is the chance in percent that a notable is replaced rather than
	// added to.
	NotableReplacementChance int32
	// Replacements and Additions are in game table order.
	Replacements []TimelessReplacement
	Additions    []TimelessAddition
}

// TimelessReplacement is a passive that replaces one in a timeless jewel's radius.
type TimelessReplacement struct {
	// ID is the game's row id. Some ids do not match the row's stats, so it is data only.
	ID    string
	Kinds []TimelessKind
	// Stats are in row order; at most 4.
	Stats  []StatRange
	Weight uint32
	// Additions is added to the version's addition count (RandomMin and RandomMax).
	Additions ValueRange
	// Conqueror is set on keystones: the conqueror whose keystone this is.
	Conqueror TimelessConqueror
}

// TimelessAddition is stats a timeless jewel adds to a passive in its radius.
type TimelessAddition struct {
	// ID is the game's row id; data only.
	ID    string
	Kinds []TimelessKind
	// Stats are in row order; at most 2.
	Stats  []StatRange
	Weight uint32
}

// StatRange is a stat handle and the range its value rolls in.
type StatRange struct {
	Stat     uint32
	Min, Max int32
}

// TimelessConqueror names a conqueror as the alternate passive keystones number them.
type TimelessConqueror struct {
	Index, Version uint32
}

// TimelessKind is a kind of passive, as the alternate passive rows number them (their
// PassiveType).
type TimelessKind uint8

// Timeless kinds.
const (
	// TimelessSmallAttribute is a small passive whose only stat is an attribute.
	TimelessSmallAttribute TimelessKind = iota + 1
	TimelessSmallNormal
	TimelessNotable
	TimelessKeystone
	TimelessJewelSocket
)
