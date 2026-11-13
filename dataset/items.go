package dataset

import "fmt"

// Items is the items group without its mods: item classes, the equippable base items, the
// tags bases and mods name, and where bench crafts and essences put their mods. A dataset
// without the group returns a MissingGroupError for it.
type Items struct {
	Classes []ItemClass
	// Tags are the Tags.Id of every tag a base, a mod or a spawn weight names.
	Tags  []string
	Bases []ItemBase
	// Crafting reports whether the group holds the bases' mod domains, the bench crafts and the
	// essence mods. Datasets written before those were extracted do not; their bases' domains
	// are 0 and they have no bench crafts or essence mods.
	Crafting bool
	// BenchCrafts are the crafting bench options that are not disabled, by the mod or
	// enchantment each adds; no two add the same mod.
	BenchCrafts []ModClasses
	// EssenceMods are the mods essences add, each once with the classes of every essence that
	// adds it.
	EssenceMods []ModClasses
}

// ItemClass is an item class, such as "Body Armour". Its display name is in Text.
type ItemClass struct {
	ID string
}

// ItemBase is a base item type. The values tables a base has are set; the others are nil.
type ItemBase struct {
	// ID is the BaseItemTypes.Id, such as "Metadata/Items/Armours/Helmets/HelmetStr1".
	ID string
	// Class is an ItemClass.ID.
	Class string
	// Implicits are Mod.IDs.
	Implicits []string
	Tags      []string
	Armour    *ArmourValues
	// Block is the chance to block in percent, set on shields only.
	Block  int32
	Weapon *WeaponValues
	Flask  *FlaskValues
	// ModDomain is BaseItemTypes.ModDomain as the game numbers it, the Mod.Domain of the mods
	// the base rolls, such as 1 for most items and 21 for cluster jewels. It is 0 when
	// Items.Crafting is false, and not 0 otherwise.
	ModDomain int32
}

// ModClasses is a mod and the item classes it can be put on.
type ModClasses struct {
	// Mod is a Mod.ID.
	Mod string
	// Classes are ItemClass.IDs, each in Items.Classes, none repeated and at least one.
	Classes []string
}

// ValueRange is an inclusive range of values.
type ValueRange struct {
	Min, Max int32
}

// ArmourValues are the defences of an armour or shield base; a defence it lacks is 0 to 0.
type ArmourValues struct {
	Armour, Evasion, EnergyShield, Ward ValueRange
	// MovementSpeed is in percent; negative is a penalty.
	MovementSpeed int32
}

// WeaponValues are the base values of a weapon.
type WeaponValues struct {
	PhysicalDamage ValueRange
	// AttackTime is in milliseconds per attack.
	AttackTime int32
	// CriticalChance is in hundredths of a percent.
	CriticalChance int32
	Range          int32
}

// FlaskKind classifies flasks, numbered as the game's Flasks.Type.
type FlaskKind uint8

// Flask kinds.
const (
	LifeFlask FlaskKind = iota + 1
	ManaFlask
	HybridFlask
	UtilityFlask
)

func (k FlaskKind) String() string {
	switch k {
	case LifeFlask:
		return "life"
	case ManaFlask:
		return "mana"
	case HybridFlask:
		return "hybrid"
	case UtilityFlask:
		return "utility"
	}
	return unknown
}

// FlaskValues are the base values of a flask.
type FlaskValues struct {
	Kind       FlaskKind
	Life, Mana int32
	// Duration is in tenths of a second.
	Duration      int32
	MaxCharges    int32
	ChargesPerUse int32
}

// Mod is a modifier an item can have. It belongs to the items group and is read on its own,
// since most uses of bases need none of the mods.
type Mod struct {
	// ID is the Mods.Id, such as "IncreasedLife1".
	ID string
	// Domain and Generation are Mods.Domain and Mods.GenerationType as the game numbers them,
	// such as 1 and 1 for an item prefix. Their names are not part of the dataset.
	Domain, Generation int32
	Level              int32
	// Type is the ModType.Name.
	Type string
	// Families are ModFamily.Ids; an item holds at most one mod of each family.
	Families []string
	// Stats are in game order; a stat can appear twice.
	Stats []ModStat
	// Tags are the mod's own tags; AddedTags are those it adds to its item.
	Tags, AddedTags                 []string
	SpawnWeights, GenerationWeights []TagWeight
	// GameMode is Mods.GameMode as the game numbers it: 0 when the mod exists in every game
	// mode, 1 in standard only, 2 in Ruthless only.
	GameMode int32
	// GrantedSkills are the skills the mod grants, in game order.
	GrantedSkills []GrantedSkill
}

// ModStat is a stat handle of a mod with its range. The Ruthless range equals Min and Max when
// the mod has no values of its own for Ruthless.
type ModStat struct {
	Stat                     uint32
	Min, Max                 int32
	RuthlessMin, RuthlessMax int32
}

// TagWeight is a weight that applies to items with the tag.
type TagWeight struct {
	Tag    string
	Weight int32
}

// MissingGroupError reports that a dataset lacks an optional table group, such as the items
// of a dataset written before the group existed. It matches ErrNotInDataset.
type MissingGroupError struct {
	Version, Group string
}

func (e *MissingGroupError) Error() string {
	return fmt.Sprintf("dataset %s has no %s group", e.Version, e.Group)
}

// Is reports whether target is ErrNotInDataset.
func (e *MissingGroupError) Is(target error) bool { return target == ErrNotInDataset }
