package dataset

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
)

// Monsters is the monsters group: what minions, totems, spectres and the enemy are made of.
// IDs are unique within each list.
type Monsters struct {
	Types       []MonsterType
	Resistances []MonsterResistance
	// Varieties are every MonsterVarieties row, since any monster can be raised as a spectre.
	Varieties []MonsterVariety
	// Levels are the DefaultMonsterStats rows, sorted by Level without repeats.
	Levels []MonsterLevel
	// BaseStats are the Stats block of Metadata/Monsters/Monster.ot, sorted by Stat without
	// repeats.
	BaseStats []StatValue
	// Mods are the mods of the game's monster domain, including those no variety lists, such as
	// the ones a monster's rarity adds.
	Mods             []Mod
	DisplayMinions   []DisplayMinion
	Totems           []Totem
	SpectreOverrides []SpectreOverride
	MinionTypes      []MinionType
	EditionTotems    []EditionTotem
}

// MonsterType is a row of MonsterTypes.
type MonsterType struct {
	ID string
	// Armour to DamageSpread are the MonsterTypes columns as the game holds them.
	Armour, Evasion, EnergyShield, Accuracy int32
	DamageSpread                            int32
	// Resistances is a MonsterResistance ID, empty for none.
	Resistances                  string
	PlayerMinion                 bool
	BaseDamageIgnoresAttackSpeed bool
	AltLife1, AltLife2           bool
}

// Resistances are resistances in percent.
type Resistances struct {
	Fire, Cold, Lightning, Chaos int32
}

// MonsterResistance is a row of MonsterResistances, with a value per difficulty.
type MonsterResistance struct {
	ID                       string
	Normal, Cruel, Merciless Resistances
}

// MonsterVariety is a row of MonsterVarieties.
type MonsterVariety struct {
	// ID is the MonsterVarieties.Id, such as "Metadata/Monsters/RaisedZombies/RaisedZombieStandard".
	ID string
	// Type is a MonsterType ID.
	Type string
	// LifeMultiplier, DamageMultiplier and AilmentThreshold are in percent.
	LifeMultiplier, DamageMultiplier int32
	AilmentThreshold                 int32
	// AttackTime is in milliseconds per attack, the game's AttackSpeed.
	AttackTime int32
	// MainHand and OffHand are ItemClasses ids, empty for none.
	MainHand, OffHand string
	// Tags are Tags ids.
	Tags []string
	// GrantedEffects are GrantedEffect IDs in the skills group.
	GrantedEffects []string
	// The mod lists are Mod IDs, one per game column: ModsKeys, ModsKeys2 (stances in 3.29),
	// Special_ModsKeys, Part1_ModsKeys, Part2_ModsKeys and Endgame_ModsKeys.
	Mods, Mods2, SpecialMods, Part1Mods, Part2Mods, EndgameMods []string
}

// MonsterLevel is a row of DefaultMonsterStats. Ally values are those of player minions.
type MonsterLevel struct {
	Level int32
	// Difficulty selects the MonsterResistance values: 1 normal, 2 cruel, 3 merciless.
	Difficulty                         int32
	Life, AllyLife, AltLife1, AltLife2 int32
	Damage, AllyDamage                 float32
	AltDamage1, AltDamage2             float32
	Armour, Evasion, EvasiveEvasion    int32
	Accuracy, AilmentThreshold         int32
	// PhysicalConversion is the game's MonsterPhysConversionMulti.
	PhysicalConversion int32
}

// Monster difficulties, as MonsterLevel.Difficulty numbers them.
const (
	NormalDifficulty int32 = iota + 1
	CruelDifficulty
	MercilessDifficulty
)

// DisplayMinion is a row of DisplayMinionMonsterType: the variety a skill's
// display_minion_monster_type stat value names.
type DisplayMinion struct {
	ID      int32
	Variety string
}

// Totem is the SkillTotemVariations rows of one SkillTotemsKey, the key ActiveSkill.Totem
// names.
type Totem struct {
	Key int32
	// Skins are sorted by Skin, without repeats.
	Skins []TotemSkin
}

// TotemSkin is the variety of one totem skin.
type TotemSkin struct {
	Skin    int32
	Variety string
}

// SpectreOverride is a row of SpectreOverrides: raising Monster as a spectre gives Spectre.
type SpectreOverride struct {
	Monster, Spectre string
}

// MinionType is a row of MinionType: the stat that limits a kind of minion and the stat that
// counts the active ones, as stat handles; nil when the row has none.
type MinionType struct {
	ID                         string
	LimitStat, ActiveCountStat *uint32
}

// EditionTotem is a link the game data does not hold: the Totem whose varieties a support's
// totems use.
type EditionTotem struct {
	// GrantedEffect is the support's GrantedEffect ID.
	GrantedEffect string
	// Totem is a Totem key.
	Totem int32
	// Source says where the link comes from.
	Source string
}

func monsterTypeID(t MonsterType) string             { return t.ID }
func monsterResistanceID(r MonsterResistance) string { return r.ID }
func varietyID(v MonsterVariety) string              { return v.ID }
func modID(m Mod) string                             { return m.ID }
func minionTypeID(m MinionType) string               { return m.ID }
func displayMinionID(m DisplayMinion) int32          { return m.ID }
func totemKey(t Totem) int32                         { return t.Key }

// monsterIDs is what monsters group rows can refer to.
type monsterIDs struct {
	types, resistances, varieties, mods map[string]bool
	totems                              map[int32]bool
}

func newMonsterIDs(m *Monsters) (ids monsterIDs, err error) {
	if ids.types, err = idSet("monster type", m.Types, monsterTypeID); err != nil {
		return ids, err
	}
	if ids.resistances, err = idSet("monster resistance", m.Resistances, monsterResistanceID); err != nil {
		return ids, err
	}
	if ids.varieties, err = idSet("monster variety", m.Varieties, varietyID); err != nil {
		return ids, err
	}
	if ids.mods, err = idSet("mod", m.Mods, modID); err != nil {
		return ids, err
	}
	if _, err = idSet("minion type", m.MinionTypes, minionTypeID); err != nil {
		return ids, err
	}
	if _, err = idSet("spectre override", m.SpectreOverrides, func(s SpectreOverride) string { return s.Monster }); err != nil {
		return ids, err
	}
	if _, err = idSet("edition totem", m.EditionTotems, func(e EditionTotem) string { return e.GrantedEffect }); err != nil {
		return ids, err
	}
	if _, err = intSet("display minion", m.DisplayMinions, displayMinionID); err != nil {
		return ids, err
	}
	ids.totems, err = intSet("totem", m.Totems, totemKey)
	return ids, err
}

// intSet returns the integer ids of s, which must be unique.
func intSet[T any](what string, s []T, id func(T) int32) (map[int32]bool, error) {
	out := make(map[int32]bool, len(s))
	for _, v := range s {
		k := id(v)
		if out[k] {
			return nil, fmt.Errorf("%s %d repeated: %w", what, k, ErrInvalid)
		}
		out[k] = true
	}
	return out, nil
}

func (ids monsterIDs) ref(what string, set map[string]bool, id string) error {
	if !set[id] {
		return fmt.Errorf("%s %q not in monsters: %w", what, id, ErrInvalid)
	}
	return nil
}

// validateMonsters checks m against a stats group of n stats and the skills group's granted
// effects.
func validateMonsters(n int, effects map[string]bool, m *Monsters) error {
	ids, err := newMonsterIDs(m)
	if err != nil {
		return err
	}
	return firstError(
		checkEach("monster type", m.Types, monsterTypeID, ids.monsterType),
		checkEach("monster variety", m.Varieties, varietyID, func(v MonsterVariety) error { return ids.variety(effects, v) }),
		validateMonsterLevels(m.Levels),
		sortedStats("monster base stats", m.BaseStats, n),
		validateMonsterMods(n, m.Mods),
		checkEach("display minion", m.DisplayMinions, func(d DisplayMinion) string { return strconv.Itoa(int(d.ID)) },
			func(d DisplayMinion) error { return ids.ref("variety", ids.varieties, d.Variety) }),
		checkEach("totem", m.Totems, func(t Totem) string { return strconv.Itoa(int(t.Key)) }, ids.totem),
		checkEach("spectre override", m.SpectreOverrides, func(s SpectreOverride) string { return s.Monster }, ids.spectreOverride),
		checkEach("minion type", m.MinionTypes, minionTypeID, func(t MinionType) error {
			return handlesInRange("stats", setHandles(t.LimitStat, t.ActiveCountStat), n)
		}),
		checkEach("edition totem", m.EditionTotems, func(e EditionTotem) string { return e.GrantedEffect },
			func(e EditionTotem) error { return ids.editionTotem(effects, e) }),
	)
}

func (ids monsterIDs) monsterType(t MonsterType) error {
	if t.Resistances == "" {
		return nil
	}
	return ids.ref("resistances", ids.resistances, t.Resistances)
}

func (ids monsterIDs) variety(effects map[string]bool, v MonsterVariety) error {
	if err := ids.ref("type", ids.types, v.Type); err != nil {
		return err
	}
	if slices.Contains(v.Tags, "") {
		return fmt.Errorf("empty tag: %w", ErrInvalid)
	}
	for _, ge := range v.GrantedEffects {
		if !effects[ge] {
			return fmt.Errorf("granted effect %q not in skills: %w", ge, ErrInvalid)
		}
	}
	for _, mod := range slices.Concat(v.Mods, v.Mods2, v.SpecialMods, v.Part1Mods, v.Part2Mods, v.EndgameMods) {
		if err := ids.ref("mod", ids.mods, mod); err != nil {
			return err
		}
	}
	return nil
}

func (ids monsterIDs) totem(t Totem) error {
	if !slices.IsSortedFunc(t.Skins, func(a, b TotemSkin) int { return cmp.Compare(a.Skin, b.Skin) }) {
		return fmt.Errorf("skins not sorted: %w", ErrInvalid)
	}
	if _, err := intSet("skin", t.Skins, func(s TotemSkin) int32 { return s.Skin }); err != nil {
		return err
	}
	for _, s := range t.Skins {
		if err := ids.ref("variety", ids.varieties, s.Variety); err != nil {
			return err
		}
	}
	return nil
}

func (ids monsterIDs) spectreOverride(s SpectreOverride) error {
	if err := ids.ref("monster", ids.varieties, s.Monster); err != nil {
		return err
	}
	return ids.ref("spectre", ids.varieties, s.Spectre)
}

func (ids monsterIDs) editionTotem(effects map[string]bool, e EditionTotem) error {
	if !effects[e.GrantedEffect] {
		return fmt.Errorf("granted effect %q not in skills: %w", e.GrantedEffect, ErrInvalid)
	}
	if !ids.totems[e.Totem] {
		return fmt.Errorf("totem %d not in monsters: %w", e.Totem, ErrInvalid)
	}
	if e.Source == "" {
		return fmt.Errorf("no source: %w", ErrInvalid)
	}
	return nil
}

func validateMonsterLevels(levels []MonsterLevel) error {
	for i, l := range levels {
		if i > 0 && levels[i-1].Level >= l.Level {
			return fmt.Errorf("monster levels not sorted or repeated at %d: %w", l.Level, ErrInvalid)
		}
		if l.Difficulty < NormalDifficulty || l.Difficulty > MercilessDifficulty {
			return fmt.Errorf("monster level %d: difficulty %d out of range: %w", l.Level, l.Difficulty, ErrInvalid)
		}
		for _, f := range []float32{l.Damage, l.AllyDamage, l.AltDamage1, l.AltDamage2} {
			if !finite(f) {
				return fmt.Errorf("monster level %d: damage %v not finite: %w", l.Level, f, ErrInvalid)
			}
		}
	}
	return nil
}

// validateMonsterMods checks the group's mods against a stats group of n stats. Their tags are
// the group's own, so only empty tags are rejected.
func validateMonsterMods(n int, mods []Mod) error {
	tags := map[string]bool{}
	for _, m := range mods {
		addStrings(tags, slices.Concat(m.Tags, m.AddedTags))
		for _, w := range slices.Concat(m.SpawnWeights, m.GenerationWeights) {
			tags[w.Tag] = true
		}
	}
	delete(tags, "")
	return checkEach("mod", mods, modID, func(m Mod) error { return validateMod(n, tags, m) })
}

// validateMonsterNames checks the monster names of texts against m, which is nil when the
// dataset has no monsters group.
func validateMonsterNames(texts []Text, m *Monsters) error {
	var varieties map[string]bool
	if m != nil {
		varieties, _ = idSet("monster variety", m.Varieties, varietyID)
	}
	for _, t := range texts {
		for _, nm := range t.MonsterNames {
			if !varieties[nm.ID] {
				return fmt.Errorf("text %s: name for unknown monster %s: %w", t.Language, nm.ID, ErrInvalid)
			}
		}
	}
	return nil
}

// validateMonstersGroup checks d's monsters group, its references to stats and skills, and
// the monster names in d's texts.
func (d *Dataset) validateMonstersGroup() error {
	if d.Monsters != nil {
		if d.Skills == nil {
			return fmt.Errorf("monsters without skills: %w", ErrInvalid)
		}
		effects, _ := idSet("granted effect", d.Skills.GrantedEffects, grantedEffectID)
		if err := validateMonsters(len(d.Stats), effects, d.Monsters); err != nil {
			return err
		}
	}
	return validateMonsterNames(d.Texts, d.Monsters)
}

// setHandles returns the handles of hs that are set.
func setHandles(hs ...*uint32) []uint32 {
	var out []uint32
	for _, h := range hs {
		if h != nil {
			out = append(out, *h)
		}
	}
	return out
}
