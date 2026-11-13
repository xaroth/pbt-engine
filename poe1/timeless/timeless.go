// Package timeless computes what a timeless jewel makes of a passive in its radius: for a jewel's
// alternate tree version, seed and conqueror, the replacement passive or the added stats the game
// picks, with the values it rolls for them. It takes the game's alternate tree tables as plain
// values. Every name is an alias of the engine's own.
package timeless

import (
	rules "github.com/xaroth/pbt-engine/internal/poe1/timeless"
)

type (
	// Kind is a passive's type as the alternate tree tables number it.
	Kind = rules.Kind
	// Range is an inclusive range of values.
	Range = rules.Range
	// Version is an AlternateTreeVersions row.
	Version = rules.Version
	// Replacement is an AlternatePassiveSkills row of the version.
	Replacement = rules.Replacement
	// Addition is an AlternatePassiveAdditions row of the version.
	Addition = rules.Addition
	// Conqueror names the conqueror on a jewel.
	Conqueror = rules.Conqueror
	// Tables is one version's rows.
	Tables = rules.Tables
	// Jewel is a timeless jewel as its mod gives it.
	Jewel = rules.Jewel
	// Node is a passive in the jewel's radius.
	Node = rules.Node
	// Result is what the jewel makes of a passive.
	Result = rules.Result
	// Rolled is an addition with its stat values.
	Rolled = rules.Rolled
	// Tree is a version ready to conquer passives.
	Tree = rules.Tree
)

// The kinds of passive.
const (
	SmallAttribute = rules.SmallAttribute
	SmallNormal    = rules.SmallNormal
	Notable        = rules.Notable
	Keystone       = rules.Keystone
	JewelSocket    = rules.JewelSocket
)

// MaxAdditions bounds how many additions one passive gets.
const MaxAdditions = rules.MaxAdditions

// NewTree checks a version's tables and prepares them.
func NewTree(t Tables) (*Tree, error) {
	return rules.NewTree(t) //nolint:wrapcheck // the same function under its public name
}

// SmallKind gives the kind of a small passive from its stat ids.
func SmallKind(statIDs []string) Kind {
	return rules.SmallKind(statIDs)
}
