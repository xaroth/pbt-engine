package dataset

import "github.com/xaroth/pbt-engine/statdesc"

// Dataset holds the tables of one game version.
type Dataset struct {
	Manifest   Manifest
	Stats      []Stat
	Characters []Character
	// Descriptions are the stat description scopes used to render stat text.
	Descriptions []statdesc.Scope
	Constants    []Constant
	// Trees has one entry per tree variant, such as standard and ruthless.
	Trees []Tree
}

// Manifest identifies a dataset and its files.
type Manifest struct {
	Format  int    `json:"format"`
	Game    string `json:"game"`
	Version string `json:"version"`
	// Source records where the data came from, such as the game build and schema revision.
	Source map[string]string `json:"source"`
	Files  []File            `json:"files"`
}

// File is one table group file listed in the manifest.
type File struct {
	Name   string `json:"name"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
}

// Stat is a row of the stats group. Other groups refer to stats by row index.
type Stat struct {
	ID string
}

// StatValue is a stat row index and its value.
type StatValue struct {
	Stat  uint32
	Value int32
}

// Character is a playable class.
type Character struct {
	ID               string
	Name             string
	BaseStrength     int32
	BaseDexterity    int32
	BaseIntelligence int32
	BaseMaxLife      int32
	BaseMaxMana      int32
	// Stats are sorted by Stat, without duplicates.
	Stats []StatValue
}

// Constant is an edition constant the game data does not hold, as Value per Per.
type Constant struct {
	ID    string
	Value int32
	Per   int32
}

// Tree is one passive tree variant.
type Tree struct {
	Variant               string
	TotalPoints           int32
	AscendancyPoints      int32
	Classes               []Class
	AlternateAscendancies []Ascendancy
	// Nodes are sorted by ID.
	Nodes      []Node
	JewelSlots []uint32
}

// Class is a character class as the tree sees it, indexed like the tree export.
type Class struct {
	Name             string
	BaseStrength     int32
	BaseDexterity    int32
	BaseIntelligence int32
	StartNode        uint32
	Ascendancies     []Ascendancy
}

// Ascendancy is an ascendancy or alternate ascendancy class.
type Ascendancy struct {
	ID   string
	Name string
}

// NodeKind classifies passive nodes.
type NodeKind uint8

// Node kinds.
const (
	Normal NodeKind = iota
	Notable
	Keystone
	Mastery
	JewelSocket
	ClassStart
	AscendancyStart
	Proxy
)

func (k NodeKind) String() string {
	switch k {
	case Normal:
		return "normal"
	case Notable:
		return "notable"
	case Keystone:
		return "keystone"
	case Mastery:
		return "mastery"
	case JewelSocket:
		return "jewel socket"
	case ClassStart:
		return "class start"
	case AscendancyStart:
		return "ascendancy start"
	case Proxy:
		return "proxy"
	}
	return "unknown"
}

// Node is a passive tree node.
type Node struct {
	ID   uint32
	Name string
	Kind NodeKind
	// Ascendancy is the name of the ascendancy the node belongs to, or empty.
	Ascendancy string
	// Stats are sorted by Stat, without duplicates.
	Stats []StatValue
	// Buffs are auras the node grants; their stats render with the passive aura scope.
	Buffs          [][]StatValue
	SkillPoints    int32
	MasteryEffects []MasteryEffect
	// Neighbours are the IDs of adjacent nodes, sorted. Edges are undirected.
	Neighbours           []uint32
	MultipleChoice       bool
	MultipleChoiceOption bool
}

// MasteryEffect is one selectable effect of a mastery node.
type MasteryEffect struct {
	ID    uint32
	Stats []StatValue
}
