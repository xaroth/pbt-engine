package dataset

import (
	"fmt"
	"math"
	"reflect"
	"slices"
)

// Skills is the skills group: gems, the granted effects they and other sources (items, the
// tree, monsters) grant, the stat sets of those effects, and their active skills. IDs are
// unique within each list; lists of stats keep the game's order, since level stats are
// positional and a few lists repeat a stat.
type Skills struct {
	// Types are the ActiveSkillType ids in game order; skills refer to a type by its index.
	// Supports' allowed and excluded types use the game's postfix form, in which the types
	// "AND", "OR" and "NOT" are operators.
	Types          []string
	Gems           []Gem
	GrantedEffects []GrantedEffect
	StatSets       []StatSet
	ActiveSkills   []ActiveSkill
}

// GemColour is a gem's colour, numbered as in SkillGems.GemColour.
type GemColour uint8

// Gem colours.
const (
	Red GemColour = iota + 1
	Green
	Blue
	White
)

// Gem is a skill gem item.
type Gem struct {
	// ID is the BaseItemTypes.Id of the gem.
	ID     string
	Colour GemColour
	// StrengthPercent, DexterityPercent and IntelligencePercent split the gem's attribute
	// requirement.
	StrengthPercent, DexterityPercent, IntelligencePercent int32
	Support                                                bool
	// Vaal marks a Vaal gem; VaalGem is the ID of a gem's Vaal version, RegularGem that of the
	// gem an awakened or exceptional gem is a version of, AwakenedGem that of the awakened one.
	Vaal        bool
	VaalGem     string
	RegularGem  string
	AwakenedGem string
	// Effects are the gem's variants, such as its transfigured versions, in game order.
	Effects []GemEffect
}

// GemStyle is the GemEffects.ItemColor of a gem effect, numbered as in the game.
type GemStyle uint8

// Gem styles.
const (
	TransfiguredX GemStyle = iota + 1
	TransfiguredY
	Trarthan
	Exceptional
	DefaultStyle
)

// GemEffect is one variant of a gem, a row of GemEffects.
type GemEffect struct {
	ID string
	// GrantedEffect and SecondaryGrantedEffect are GrantedEffect IDs; a Vaal gem's secondary
	// effect is its regular skill. The hardmode effects are the game's hardmode columns, empty
	// when unset.
	GrantedEffect, SecondaryGrantedEffect                 string
	HardmodeGrantedEffect, HardmodeSecondaryGrantedEffect string
	// Tags are GemTags ids.
	Tags  []string
	Style GemStyle
}

// GrantedEffect is a skill or support as a gem, item, passive or monster grants it.
type GrantedEffect struct {
	ID      string
	Support bool
	// ActiveSkill is the ActiveSkill ID; empty for supports.
	ActiveSkill string
	// CastTime is in milliseconds.
	CastTime int32
	// StatSets are StatSet IDs, the primary set first.
	StatSets []string
	// QualityStats are the stats per 1% quality, in thousandths.
	QualityStats []StatValue
	// AllowedTypes, ExcludedTypes and AddedTypes are a support's skill types, AddedMinionTypes
	// those it adds to minion skills; indexes into Skills.Types.
	AllowedTypes, ExcludedTypes, AddedTypes, AddedMinionTypes []uint16
	// SupportWeaponRestrictions are ItemClasses ids.
	SupportWeaponRestrictions []string
	SupportsGemsOnly          bool
	CannotBeSupported         bool
	// Levels are sorted by Level, without repeats.
	Levels []EffectLevel
}

// EffectLevel is a row of GrantedEffectsPerLevel. Times are in milliseconds.
type EffectLevel struct {
	Level       int32
	PlayerLevel int32
	Costs       []Cost
	// CostMultiplier is a support's cost multiplier in percent.
	CostMultiplier         int32
	Cooldown               int32
	CooldownGroup          int32
	StoredUses             int32
	VaalSouls              int32
	VaalStoredUses         int32
	SoulGainPrevention     int32
	AttackSpeedMultiplier  int32
	AttackTime             int32
	ManaReservationFlat    int32
	ManaReservationPercent int32 // hundredths of a percent
	LifeReservationFlat    int32
	LifeReservationPercent int32 // hundredths of a percent
}

// Cost is an amount of a CostTypes resource, such as "Mana".
type Cost struct {
	Type   string
	Amount int32
}

// StatSet is a row of GrantedEffectStatSets with its levels.
type StatSet struct {
	ID string
	// ImplicitStats are stat handles.
	ImplicitStats            []uint32
	ConstantStats            []StatValue
	BaseEffectiveness        float32
	IncrementalEffectiveness float32
	// Levels are sorted by Level, without repeats.
	Levels []StatSetLevel
}

// Interpolation says how a level stat's value follows the user's level, numbered as the
// game's StatInterpolationTypes.
type Interpolation uint8

// Interpolations.
const (
	NoInterpolation Interpolation = iota + 1
	LinearInterpolation
	// EffectivenessInterpolation scales the value by the skill's damage effectiveness at the
	// user's level.
	EffectivenessInterpolation
)

// StatSetLevel is a row of GrantedEffectStatSetsPerLevel. FloatStats and then Stats are the
// row's positional stats.
type StatSetLevel struct {
	Level       int32
	PlayerLevel int32
	// SpellCritChance and AttackCritChance are in hundredths of a percent.
	SpellCritChance, AttackCritChance int32
	// DamageEffectiveness and BaseMultiplier are in ten-thousandths, added to 1.
	DamageEffectiveness, BaseMultiplier int32
	FloatStats                          []FloatStat
	Stats                               []LevelStat
	// Flags are stat handles of stats set to 1.
	Flags []uint32
}

// FloatStat is a stat with a fractional base value, a FloatStats entry.
type FloatStat struct {
	Stat  uint32
	Value float32
	// Resolved is the game's value at the row's level, BaseResolvedValues.
	Resolved      int32
	Interpolation Interpolation
	// Base is the EffectivenessCostConstants id the value is relative to.
	Base string
}

// LevelStat is an integer stat of a level row, an AdditionalStats entry.
type LevelStat struct {
	Stat          uint32
	Value         int32
	Interpolation Interpolation
}

// ActiveSkill is a row of ActiveSkills.
type ActiveSkill struct {
	ID string
	// Types and MinionTypes are indexes into Skills.Types.
	Types, MinionTypes []uint16
	// WeaponRestrictions are ItemClasses ids.
	WeaponRestrictions []string
	// Totem is the SkillTotemId, the key of the skill's totem varieties in SkillTotemVariations.
	Totem int32
	// Mine is the SkillMine, the key of the skill's mine objects in SkillMineVariations. Traps
	// have no such link in the skill tables; trap skills have the "Trapped" type.
	Mine int32
	// TransfigureBase is the ID of the active skill a transfigured one is based on.
	TransfigureBase string
}

// validateSkills checks s against a stats group of n stats.
func validateSkills(n int, s *Skills) error {
	ids, err := newSkillIDs(s)
	if err != nil {
		return err
	}
	return firstError(
		validateSkillTables(s),
		checkEach("gem", s.Gems, gemID, ids.gem),
		sameGemEffects(s.Gems),
		checkEach("granted effect", s.GrantedEffects, grantedEffectID, func(g GrantedEffect) error { return ids.grantedEffect(n, g) }),
		checkEach("stat set", s.StatSets, statSetID, func(ss StatSet) error { return validateStatSet(n, ss) }),
		checkEach("active skill", s.ActiveSkills, activeSkillID, ids.activeSkill),
	)
}

func gemID(g Gem) string                     { return g.ID }
func grantedEffectID(g GrantedEffect) string { return g.ID }
func statSetID(s StatSet) string             { return s.ID }
func activeSkillID(a ActiveSkill) string     { return a.ID }

// firstError returns the first error that is not nil.
func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// checkEach returns the first error of check over list, prefixed with the row's id.
func checkEach[T any](what string, list []T, id func(T) string, check func(T) error) error {
	for _, v := range list {
		if err := check(v); err != nil {
			return fmt.Errorf("%s %s: %w", what, id(v), err)
		}
	}
	return nil
}

// typeOperator returns the number of values a skill type takes when it is an operator in a
// support's allowed and excluded types.
func typeOperator(name string) (arity int, ok bool) {
	const binary, unary = 2, 1
	switch name {
	case "AND", "OR":
		return binary, true
	case "NOT":
		return unary, true
	}
	return 0, false
}

// skillIDs is what skills group rows can refer to.
type skillIDs struct {
	gems, effects, sets, actives map[string]bool
	types                        []string
}

func newSkillIDs(s *Skills) (ids skillIDs, err error) {
	ids.types = s.Types
	if _, err = idSet("skill type", s.Types, func(t string) string { return t }); err != nil {
		return ids, err
	}
	if ids.gems, err = idSet("gem", s.Gems, gemID); err != nil {
		return ids, err
	}
	if ids.effects, err = idSet("granted effect", s.GrantedEffects, grantedEffectID); err != nil {
		return ids, err
	}
	if ids.sets, err = idSet("stat set", s.StatSets, statSetID); err != nil {
		return ids, err
	}
	ids.actives, err = idSet("active skill", s.ActiveSkills, activeSkillID)
	return ids, err
}

func (ids skillIDs) ref(what string, set map[string]bool, id string) error {
	if id != "" && !set[id] {
		return fmt.Errorf("%s %s not in skills: %w", what, id, ErrInvalid)
	}
	return nil
}

// sameGemEffects checks that an effect id used by more than one gem names the same effect
// each time; the game shares a few GemEffects rows between gems.
func sameGemEffects(gems []Gem) error {
	seen := map[string]GemEffect{}
	for _, g := range gems {
		for _, e := range g.Effects {
			if prev, ok := seen[e.ID]; ok && !reflect.DeepEqual(prev, e) {
				return fmt.Errorf("gem %s: effect %s differs from the effect of the same id in another gem: %w", g.ID, e.ID, ErrInvalid)
			}
			seen[e.ID] = e
		}
	}
	return nil
}

func (ids skillIDs) gem(g Gem) error {
	if g.Colour < Red || g.Colour > White {
		return fmt.Errorf("colour %d out of range: %w", g.Colour, ErrInvalid)
	}
	for _, r := range [][2]string{{"vaal gem", g.VaalGem}, {"regular gem", g.RegularGem}, {"awakened gem", g.AwakenedGem}} {
		if err := ids.ref(r[0], ids.gems, r[1]); err != nil {
			return err
		}
	}
	if _, err := idSet("effect", g.Effects, func(e GemEffect) string { return e.ID }); err != nil {
		return err
	}
	return checkEach("effect", g.Effects, func(e GemEffect) string { return e.ID }, ids.gemEffect)
}

func (ids skillIDs) gemEffect(e GemEffect) error {
	if e.GrantedEffect == "" {
		return fmt.Errorf("no granted effect: %w", ErrInvalid)
	}
	if e.Style < TransfiguredX || e.Style > DefaultStyle {
		return fmt.Errorf("style %d out of range: %w", e.Style, ErrInvalid)
	}
	for _, ge := range []string{e.GrantedEffect, e.SecondaryGrantedEffect, e.HardmodeGrantedEffect, e.HardmodeSecondaryGrantedEffect} {
		if err := ids.ref("granted effect", ids.effects, ge); err != nil {
			return err
		}
	}
	return nil
}

func (ids skillIDs) grantedEffect(n int, g GrantedEffect) error {
	if err := ids.ref("active skill", ids.actives, g.ActiveSkill); err != nil {
		return err
	}
	for _, s := range g.StatSets {
		if s == "" {
			return fmt.Errorf("empty stat set: %w", ErrInvalid)
		}
		if err := ids.ref("stat set", ids.sets, s); err != nil {
			return err
		}
	}
	if err := statsInRange("quality", g.QualityStats, n); err != nil {
		return err
	}
	for _, ts := range [][]uint16{g.AllowedTypes, g.ExcludedTypes} {
		if err := ids.typeExpression(ts); err != nil {
			return err
		}
	}
	for _, ts := range [][]uint16{g.AddedTypes, g.AddedMinionTypes} {
		if err := ids.plainTypes(ts); err != nil {
			return err
		}
	}
	return validateEffectLevels(g.Levels)
}

func validateEffectLevels(levels []EffectLevel) error {
	for i, l := range levels {
		if i > 0 && levels[i-1].Level >= l.Level {
			return fmt.Errorf("levels not sorted or repeated: %w", ErrInvalid)
		}
		if slices.ContainsFunc(l.Costs, func(c Cost) bool { return c.Type == "" }) {
			return fmt.Errorf("level %d: cost without type: %w", l.Level, ErrInvalid)
		}
	}
	return nil
}

func (ids skillIDs) typeName(t uint16) (string, error) {
	if int(t) >= len(ids.types) {
		return "", fmt.Errorf("skill type %d out of range (%d types): %w", t, len(ids.types), ErrInvalid)
	}
	return ids.types[t], nil
}

// typeExpression checks a support's allowed or excluded types in the game's postfix form: a
// type pushes a value, AND and OR take two values and NOT one, each pushing its result. Values
// left at the end are alternatives, so an expression may leave several.
func (ids skillIDs) typeExpression(ts []uint16) error {
	depth := 0
	for _, t := range ts {
		name, err := ids.typeName(t)
		if err != nil {
			return err
		}
		arity, op := typeOperator(name)
		if !op {
			depth++
			continue
		}
		if depth < arity {
			return fmt.Errorf("skill types: %s needs %d values, has %d: %w", name, arity, depth, ErrInvalid)
		}
		depth -= arity - 1
	}
	return nil
}

// plainTypes checks a list of skill types that holds no operators.
func (ids skillIDs) plainTypes(ts []uint16) error {
	for _, t := range ts {
		name, err := ids.typeName(t)
		if err != nil {
			return err
		}
		if _, op := typeOperator(name); op {
			return fmt.Errorf("skill types: operator %s outside allowed or excluded types: %w", name, ErrInvalid)
		}
	}
	return nil
}

func (ids skillIDs) activeSkill(a ActiveSkill) error {
	if err := ids.plainTypes(a.Types); err != nil {
		return err
	}
	if err := ids.plainTypes(a.MinionTypes); err != nil {
		return err
	}
	return ids.ref("transfigure base", ids.actives, a.TransfigureBase)
}

// finite reports whether f is neither NaN nor infinite.
func finite(f float32) bool {
	return !math.IsNaN(float64(f)) && !math.IsInf(float64(f), 0)
}

func validateStatSet(n int, s StatSet) error {
	if !finite(s.BaseEffectiveness) || !finite(s.IncrementalEffectiveness) {
		return fmt.Errorf("effectiveness %v, %v not finite: %w", s.BaseEffectiveness, s.IncrementalEffectiveness, ErrInvalid)
	}
	if err := handlesInRange("implicit", s.ImplicitStats, n); err != nil {
		return err
	}
	if err := statsInRange("constant", s.ConstantStats, n); err != nil {
		return err
	}
	for i, l := range s.Levels {
		if i > 0 && s.Levels[i-1].Level >= l.Level {
			return fmt.Errorf("levels not sorted or repeated: %w", ErrInvalid)
		}
		if err := validateStatSetLevel(n, l); err != nil {
			return fmt.Errorf("level %d: %w", l.Level, err)
		}
	}
	return nil
}

func validateStatSetLevel(n int, l StatSetLevel) error {
	for _, f := range l.FloatStats {
		if err := checkLevelStat(n, f.Stat, f.Interpolation); err != nil {
			return err
		}
		if f.Base == "" {
			return fmt.Errorf("float stat %d: no interpolation base: %w", f.Stat, ErrInvalid)
		}
		if !finite(f.Value) {
			return fmt.Errorf("float stat %d: value %v not finite: %w", f.Stat, f.Value, ErrInvalid)
		}
	}
	for _, s := range l.Stats {
		if err := checkLevelStat(n, s.Stat, s.Interpolation); err != nil {
			return err
		}
	}
	return handlesInRange("flags", l.Flags, n)
}

func checkLevelStat(n int, stat uint32, i Interpolation) error {
	if int(stat) >= n {
		return fmt.Errorf("stat %d out of range (%d stats): %w", stat, n, ErrInvalid)
	}
	if i < NoInterpolation || i > EffectivenessInterpolation {
		return fmt.Errorf("stat %d: interpolation %d out of range: %w", stat, i, ErrInvalid)
	}
	return nil
}

func handlesInRange(what string, hs []uint32, n int) error {
	for _, h := range hs {
		if int(h) >= n {
			return fmt.Errorf("%s: stat %d out of range (%d stats): %w", what, h, n, ErrInvalid)
		}
	}
	return nil
}

// validateSkillNames checks the gem and skill names of texts against s, which is nil when the
// dataset has no skills group.
func validateSkillNames(texts []Text, s *Skills) error {
	var gems, actives map[string]bool
	if s != nil {
		gems, _ = idSet("gem", s.Gems, gemID)
		actives, _ = idSet("active skill", s.ActiveSkills, activeSkillID)
	}
	for _, t := range texts {
		for _, c := range []struct {
			what  string
			names []Name
			ids   map[string]bool
		}{{"gem", t.GemNames, gems}, {"skill", t.SkillNames, actives}} {
			for _, nm := range c.names {
				if !c.ids[nm.ID] {
					return fmt.Errorf("text %s: name for unknown %s %s: %w", t.Language, c.what, nm.ID, ErrInvalid)
				}
			}
		}
	}
	return nil
}

// validateSkillsGroup checks d's skills group and the skill names in d's texts.
func (d *Dataset) validateSkillsGroup() error {
	if d.Skills != nil {
		if err := validateSkills(len(d.Stats), d.Skills); err != nil {
			return err
		}
	}
	return validateSkillNames(d.Texts, d.Skills)
}
