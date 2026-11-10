package dataset

import "github.com/xaroth/pbt-engine/statdesc"

// Dataset holds the tables of one game version, as Write takes them. Readers use Open, which
// returns a Store that reads each group on first use.
type Dataset struct {
	Manifest   Manifest
	Stats      []Stat
	Characters []Character
	Constants  []Constant
	// Quests are the campaign and labyrinth rewards, in game table order.
	Quests []Quest
	// Trees has one entry per tree variant, such as standard and ruthless.
	Trees []Tree
	// Texts holds display text per language; nothing else in the dataset depends on them.
	Texts []Text
}

// Text is the display text of one language: stat description templates and names by id.
type Text struct {
	// Language is a lowercase tag such as "en".
	Language     string
	Descriptions []statdesc.Scope
	// NodeNames are sorted by ID.
	NodeNames []NodeName
	// CharacterNames are keyed by Character.ID.
	CharacterNames []Name
	// AscendancyNames are keyed by tree variant and Ascendancy.ID; the same id can have a
	// different name in another variant.
	AscendancyNames []AscendancyName
}

// NodeName is the display name of a passive node.
type NodeName struct {
	ID   uint32
	Name string
}

// Name is the display name of the object with id ID.
type Name struct {
	ID   string
	Name string
}

// AscendancyName is the display name of an ascendancy in one tree variant.
type AscendancyName struct {
	Variant string
	ID      string
	Name    string
}

// Manifest identifies a dataset and its files.
type Manifest struct {
	Format  int    `json:"format"`
	Game    string `json:"game"`
	Version string `json:"version"`
	// ID is a hash of Game, Version, Capabilities and the file hashes. Datasets rebuilt with
	// different content keep their version but get a new ID.
	ID string `json:"id"`
	// Capabilities are the rule versions the dataset needs; see Capabilities.
	Capabilities Capabilities `json:"capabilities,omitempty"`
	// Source optionally records where the data came from. Published datasets leave it out:
	// their recipe sits next to them (pbt-data's source.json).
	Source map[string]string `json:"source,omitempty"`
	Files  []File            `json:"files"`
}

// File is one table group file listed in the manifest.
type File struct {
	Name   string `json:"name"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
}

// Stat is a row of the stats group. In memory, other groups refer to stats by index into
// Dataset.Stats, called a handle; files refer to them by id.
type Stat struct {
	ID string
}

// StatValue is a stat handle and its value.
type StatValue struct {
	Stat  uint32
	Value int32
}

// Character is a playable class.
type Character struct {
	ID               string
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

// Quest is the static reward one quest flag grants: passive skill points, ascendancy points,
// passive refund points and stats.
type Quest struct {
	// Quest is the Quest.Id, such as "a2q7"; empty for a reward no quest owns, such as a
	// labyrinth.
	Quest string
	// Flag is the QuestFlags.Id that grants the reward. Flags are unique.
	Flag             string
	Act              int32
	PassivePoints    int32
	AscendancyPoints int32
	RefundPoints     int32
	// Stats are sorted by Stat, without duplicates.
	Stats []StatValue
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

// Class is a character class as the tree sees it; its index in Tree.Classes is its class id.
type Class struct {
	// Character is the Character.ID of the class.
	Character        string
	BaseStrength     int32
	BaseDexterity    int32
	BaseIntelligence int32
	StartNode        uint32
	Ascendancies     []Ascendancy
}

// Ascendancy is an ascendancy or alternate ascendancy class.
type Ascendancy struct {
	ID string
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
	Kind NodeKind
	// Ascendancy is the ID of the ascendancy the node belongs to, or empty.
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
