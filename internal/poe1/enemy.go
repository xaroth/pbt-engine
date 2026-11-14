package poe1

import (
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/core"
)

// EnemyKind is what the enemy a build is measured against is: a normal monster or a boss of
// some tier. Its values for level, resistances, armour and evasion apply where the config sets
// none.
type EnemyKind string

// Enemy kinds.
const (
	// EnemyNone is a normal monster of the enemy's level.
	EnemyNone EnemyKind = "none"
	// EnemyBoss is a map boss.
	EnemyBoss EnemyKind = "boss"
	// EnemyPinnacle is a pinnacle boss, such as the Shaper or the Searing Exarch. It is the
	// default.
	EnemyPinnacle EnemyKind = "pinnacle"
	// EnemyUber is the uber version of a pinnacle boss.
	EnemyUber EnemyKind = "uber"
)

// enemyKinds are the enemy kinds, weakest first.
func enemyKinds() []EnemyKind {
	return []EnemyKind{EnemyNone, EnemyBoss, EnemyPinnacle, EnemyUber}
}

// Valid reports whether k is an enemy kind.
func (k EnemyKind) Valid() error {
	if slices.Contains(enemyKinds(), k) {
		return nil
	}
	return fmt.Errorf("%w: enemy %q, want %s, %s, %s or %s", ErrConfigValue, k, EnemyNone, EnemyBoss, EnemyPinnacle, EnemyUber)
}

// MonsterLevel is a monster's level, from 1.
type MonsterLevel int

// Valid reports whether l is at least 1.
func (l MonsterLevel) Valid() error {
	if l >= 1 {
		return nil
	}
	return fmt.Errorf("%w: level %d, want 1 or more", ErrConfigValue, l)
}

// Rating is an armour or evasion rating, from 0.
type Rating int

// Valid reports whether r is not negative.
func (r Rating) Valid() error {
	if r >= 0 {
		return nil
	}
	return fmt.Errorf("%w: rating %d, want 0 or more", ErrConfigValue, r)
}

// Enemy config keys. A key without a value takes the value of the enemy kind; their defaults
// are those of the default kind, EnemyPinnacle.
const (
	Enemy                    config.Key[EnemyKind]    = "enemy"
	EnemyLevel               config.Key[MonsterLevel] = "enemy-level"
	EnemyFireResistance      config.Key[int]          = "enemy-fire-resistance"
	EnemyColdResistance      config.Key[int]          = "enemy-cold-resistance"
	EnemyLightningResistance config.Key[int]          = "enemy-lightning-resistance"
	EnemyChaosResistance     config.Key[int]          = "enemy-chaos-resistance"
	EnemyArmour              config.Key[Rating]       = "enemy-armour"
	EnemyEvasion             config.Key[Rating]       = "enemy-evasion"
)

// enemyPreset is what an enemy kind sets. A zero level means the character's level, at most
// maxPresetLevel. MonsterDefences means the game's armour and evasion of a monster of the
// enemy's level (DefaultMonsterStats) in place of Armour and Evasion.
type enemyPreset struct {
	Level               MonsterLevel
	ElementalResistance int
	ChaosResistance     int
	Armour, Evasion     Rating
	MonsterDefences     bool
}

// maxPresetLevel caps the character's level as the level of a normal monster or map boss.
const maxPresetLevel = 85

// enemyPresets are edition values: the game data has no values for bosses. Source: Path of
// Building f9f4f3b, Modules/ConfigOptions.lua (enemyIsBoss) and Modules/Data.lua (bossStats).
// Pinnacle and uber armour and evasion are its monster values at their level times the mean
// multipliers of its pinnacle bosses (150% and 124.9%) and of its uber bosses (125% and
// 116.6%), rounded.
//
//nolint:mnd // each value is named by its field and sourced above
func enemyPresets() map[EnemyKind]enemyPreset {
	return map[EnemyKind]enemyPreset{
		EnemyNone:     {MonsterDefences: true},
		EnemyBoss:     {ElementalResistance: 40, ChaosResistance: 25, MonsterDefences: true},
		EnemyPinnacle: {Level: 84, ElementalResistance: 50, ChaosResistance: 30, Armour: 61989, Evasion: 10143},
		EnemyUber:     {Level: 85, ElementalResistance: 50, ChaosResistance: 30, Armour: 55739, Evasion: 9778},
	}
}

func enemySpecs() []config.Spec {
	p := enemyPresets()[EnemyPinnacle]
	return []config.Spec{
		config.SpecOf(Enemy, EnemyPinnacle, enemyKinds()...),
		config.SpecOf(EnemyLevel, p.Level),
		config.SpecOf(EnemyFireResistance, p.ElementalResistance),
		config.SpecOf(EnemyColdResistance, p.ElementalResistance),
		config.SpecOf(EnemyLightningResistance, p.ElementalResistance),
		config.SpecOf(EnemyChaosResistance, p.ChaosResistance),
		config.SpecOf(EnemyArmour, p.Armour),
		config.SpecOf(EnemyEvasion, p.Evasion),
	}
}

// EnemyStats is the enemy a build's damage is measured against.
type EnemyStats struct {
	Kind                                                                 EnemyKind
	Level                                                                int
	FireResistance, ColdResistance, LightningResistance, ChaosResistance int
	Armour, Evasion                                                      int
}

// EnemyReadsMonsters reports whether EnemyOf reads the monster values per level for cfg: its
// enemy kind takes a normal monster's armour and evasion.
func EnemyReadsMonsters(cfg config.Resolved) bool {
	kind, src := config.Lookup(cfg, Enemy)
	if src == config.Default {
		kind = EnemyPinnacle
	}
	return enemyPresets()[kind].MonsterDefences
}

// EnemyOf resolves the enemy of cfg for a character of characterLevel: each value cfg sets,
// else the enemy kind's. levels are the game's monster values per level (DefaultMonsterStats);
// a kind without its own armour and evasion reads them there, and a level they lack is an
// error.
func EnemyOf(cfg config.Resolved, characterLevel int, levels []dataset.MonsterLevel) (EnemyStats, error) {
	kind, src := config.Lookup(cfg, Enemy)
	if src == config.Default {
		kind = EnemyPinnacle
	}
	p, ok := enemyPresets()[kind]
	if !ok {
		return EnemyStats{}, fmt.Errorf("poe1: %w: enemy %q", ErrConfigValue, kind)
	}
	if p.Level == 0 {
		p.Level = MonsterLevel(min(characterLevel, maxPresetLevel))
	}
	level := int(set(cfg, EnemyLevel, p.Level))
	if p.MonsterDefences {
		i := slices.IndexFunc(levels, func(l dataset.MonsterLevel) bool { return int(l.Level) == level })
		if i < 0 {
			return EnemyStats{}, fmt.Errorf("poe1: no monster values for enemy level %d: %w", level, dataset.ErrNotInDataset)
		}
		p.Armour, p.Evasion = Rating(levels[i].Armour), Rating(levels[i].Evasion)
	}
	return EnemyStats{
		Kind:                kind,
		Level:               level,
		FireResistance:      set(cfg, EnemyFireResistance, p.ElementalResistance),
		ColdResistance:      set(cfg, EnemyColdResistance, p.ElementalResistance),
		LightningResistance: set(cfg, EnemyLightningResistance, p.ElementalResistance),
		ChaosResistance:     set(cfg, EnemyChaosResistance, p.ChaosResistance),
		Armour:              int(set(cfg, EnemyArmour, p.Armour)),
		Evasion:             int(set(cfg, EnemyEvasion, p.Evasion)),
	}, nil
}

// set returns k's value in cfg, or preset when cfg has only its default.
func set[T any](cfg config.Resolved, k config.Key[T], preset T) T {
	if v, src := config.Lookup(cfg, k); src != config.Default {
		return v
	}
	return preset
}

// enemyMods are the modifiers that hold the enemy's values on a sheet.
type enemyMods struct {
	resistances map[DamageType]core.ModID
	armour      core.ModID
}

// addEnemy adds the enemy's resistances, each in the context of its damage type, and armour
// as a rule's source, set to the default enemy.
func (sh *Sheet) addEnemy() error {
	src := sh.newSource(Source{RuleID: "enemy", Rule: "the enemy"})
	sh.enemy.resistances = map[DamageType]core.ModID{}
	for _, d := range []DamageType{Fire, Cold, Lightning, Chaos} {
		pred := core.Predicate{Req: []core.Mask{core.MaskOf(sh.ctx.damage.Bit(int(d) - 1))}}
		id, err := sh.session.AddModifier(core.Modifier{Stat: sh.stats[statEnemyResistance], Op: core.OpBase, Source: src, Pred: pred})
		if err != nil {
			return fmt.Errorf("sheet: enemy: %w", err)
		}
		sh.enemy.resistances[d] = id
	}
	id, err := sh.session.AddModifier(core.Modifier{Stat: sh.stats[statEnemyArmour], Op: core.OpBase, Source: src})
	if err != nil {
		return fmt.Errorf("sheet: enemy: %w", err)
	}
	sh.enemy.armour = id
	sh.SetEnemy(DefaultEnemy())
	return nil
}

// DefaultEnemy is the enemy of the default kind.
func DefaultEnemy() EnemyStats {
	p := enemyPresets()[EnemyPinnacle]
	return EnemyStats{
		Kind: EnemyPinnacle, Level: int(p.Level),
		FireResistance: p.ElementalResistance, ColdResistance: p.ElementalResistance, LightningResistance: p.ElementalResistance,
		ChaosResistance: p.ChaosResistance, Armour: int(p.Armour), Evasion: int(p.Evasion),
	}
}

// SetEnemy makes e the enemy the sheet's hits are measured against.
func (sh *Sheet) SetEnemy(e EnemyStats) {
	values := map[DamageType]int{Fire: e.FireResistance, Cold: e.ColdResistance, Lightning: e.LightningResistance, Chaos: e.ChaosResistance}
	for d, id := range sh.enemy.resistances {
		// The ids are this session's, so these cannot fail.
		_ = sh.session.SetModifierValue(id, float64(values[d]))
	}
	_ = sh.session.SetModifierValue(sh.enemy.armour, float64(e.Armour))
}
