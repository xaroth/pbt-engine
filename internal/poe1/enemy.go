package poe1

import (
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/core"
)

// Engine stats of the enemy a skill's damage meets, read in the skill's context.
// enemy_resistance, read for one damage type, is the enemy's resistance to it, its reductions
// included, and enemy_maximum_resistance its cap. enemy_exposure is the Exposure the enemy has
// to the type, its strongest instance; extra_exposure what is added to Exposure the character
// inflicts, and minimum_exposure the least that Exposure applies (negative, 0 for no minimum).
// penetration is how much of the resistance the character's hits of the type penetrate, and
// non_critical_penetration what non-critical strikes penetrate on top of it, and
// critical_penetration what critical strikes do.
// inverted_resistance_chance is the chance in percent that a hit treats the resistance as
// inverted, and critical_strikes_ignore_resistance a flag that critical strikes ignore it.
// hit_enemy_resistance is the resistance a non-critical hit of the type meets after all of
// these, critical_hit_enemy_resistance the one a critical strike meets.
// enemy_damage_taken is the damage the enemy takes of the type in percent, 100 before modifiers;
// enemy_armour and enemy_evasion are its armour and evasion rating.
const (
	statEnemyResistance                 = "enemy_resistance"
	statEnemyMaximumResistance          = "enemy_maximum_resistance"
	statEnemyExposure                   = "enemy_exposure"
	statExtraExposure                   = "extra_exposure"
	statMinimumExposure                 = "minimum_exposure"
	statPenetration                     = "penetration"
	statNonCriticalPenetration          = "non_critical_penetration"
	statCriticalPenetration             = "critical_penetration"
	statInvertedResistanceChance        = "inverted_resistance_chance"
	statCriticalStrikesIgnoreResistance = "critical_strikes_ignore_resistance"
	statHitEnemyResistance              = "hit_enemy_resistance"
	statCriticalHitEnemyResistance      = "critical_hit_enemy_resistance"
	statEnemyDamageTaken                = "enemy_damage_taken"
	statEnemyArmour                     = "enemy_armour"
	statEnemyEvasion                    = "enemy_evasion"
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
// enemy's level (DefaultMonsterStats) in place of Armour and Evasion. DamageTaken is the more
// multiplier of the damage the enemy takes, in percent.
type enemyPreset struct {
	Level               MonsterLevel
	ElementalResistance int
	ChaosResistance     int
	Armour, Evasion     Rating
	MonsterDefences     bool
	DamageTaken         int
}

// maxPresetLevel caps the character's level as the level of a normal monster or map boss.
const maxPresetLevel = 85

// enemyPresets are edition values: the game data has no values for bosses. Source: Path of
// Building f9f4f3b, Modules/ConfigOptions.lua (enemyIsBoss) and Modules/Data.lua (bossStats).
// Pinnacle and uber armour and evasion are its monster values at their level times the mean
// multipliers of its pinnacle bosses (150% and 124.9%) and of its uber bosses (125% and
// 116.6%), rounded. An uber boss takes 70% less damage.
//
//nolint:mnd // each value is named by its field and sourced above
func enemyPresets() map[EnemyKind]enemyPreset {
	return map[EnemyKind]enemyPreset{
		EnemyNone:     {MonsterDefences: true},
		EnemyBoss:     {ElementalResistance: 40, ChaosResistance: 25, MonsterDefences: true},
		EnemyPinnacle: {Level: 84, ElementalResistance: 50, ChaosResistance: 30, Armour: 61989, Evasion: 10143},
		EnemyUber:     {Level: 85, ElementalResistance: 50, ChaosResistance: 30, Armour: 55739, Evasion: 9778, DamageTaken: -70},
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

// enemyReadsMonsters reports whether EnemyOf reads the monster values per level for cfg: its
// enemy kind takes a normal monster's armour and evasion.
func enemyReadsMonsters(cfg config.Resolved) bool {
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

// Enemy resistance limits. A monster's maximum resistance is 75%; the enemy's is that or its
// configured resistance, if higher, at most 90%. Source: PoB f9f4f3b, Modules/CalcOffence.lua
// calcResistForType and Modules/Data.lua (EnemyMaxResist from the monster constant
// base_maximum_all_resistances_%, MaxResistCap); the dataset holds no monster constants.
const (
	enemyBaseMaximumResistance = 75
	enemyMaximumResistanceCap  = maxResistanceCap
)

// enemyMaximumResistance is the maximum of an enemy resistance configured at res.
func enemyMaximumResistance(res int) int {
	return min(max(res, enemyBaseMaximumResistance), enemyMaximumResistanceCap)
}

// enemyMods are the modifiers that hold the enemy's values on a sheet.
type enemyMods struct {
	resistances, maxima          map[DamageType]core.ModID
	armour, evasion, damageTaken core.ModID
}

// addEnemy adds the enemy's resistances and their maximums, each in the context of its damage
// type, its armour and evasion rating and the more multiplier of the damage it takes as a rule's source, set to
// the default enemy; and the damage it takes before modifiers, 100%.
func (sh *Sheet) addEnemy() error {
	src := sh.newSource(Source{RuleID: "enemy", Rule: "the enemy"})
	add := func(stat string, op core.Op, pred core.Predicate) (core.ModID, error) {
		id, err := sh.session.AddModifier(core.Modifier{Stat: sh.stats[stat], Op: op, Source: src, Pred: pred})
		if err != nil {
			return core.ModID{}, fmt.Errorf("sheet: enemy: %w", err)
		}
		return id, nil
	}
	sh.enemy.resistances, sh.enemy.maxima = map[DamageType]core.ModID{}, map[DamageType]core.ModID{}
	for _, d := range []DamageType{Fire, Cold, Lightning, Chaos} {
		pred := core.Predicate{Req: []core.Mask{core.MaskOf(sh.ctx.damage.Bit(int(d) - 1))}}
		var err error
		if sh.enemy.resistances[d], err = add(statEnemyResistance, core.OpBase, pred); err != nil {
			return err
		}
		if sh.enemy.maxima[d], err = add(statEnemyMaximumResistance, core.OpBase, pred); err != nil {
			return err
		}
	}
	var err error
	if sh.enemy.armour, err = add(statEnemyArmour, core.OpBase, core.Predicate{}); err != nil {
		return err
	}
	if sh.enemy.evasion, err = add(statEnemyEvasion, core.OpBase, core.Predicate{}); err != nil {
		return err
	}
	if sh.enemy.damageTaken, err = add(statEnemyDamageTaken, core.OpMore, core.Predicate{}); err != nil {
		return err
	}
	base := sh.newSource(Source{RuleID: "enemy-damage-taken", Rule: "damage the enemy takes (100%)"})
	if _, err := sh.session.AddModifier(core.Modifier{Stat: sh.stats[statEnemyDamageTaken], Op: core.OpBase, Value: percentScale, Source: base}); err != nil {
		return fmt.Errorf("sheet: enemy: %w", err)
	}
	if err := sh.addMinionEnemy(); err != nil {
		return err
	}
	sh.SetEnemy(DefaultEnemy())
	return nil
}

// enemyStats are the stats of the enemy a hit reads, which the player actor holds.
func enemyStats() []string {
	return []string{
		statEnemyResistance, statEnemyMaximumResistance, statEnemyExposure, statEnemyArmour, statEnemyEvasion, statEnemyDamageTaken,
	}
}

// addMinionEnemy makes the enemy a minion's hits meet the player's: each enemy stat of the
// minion actor is the player actor's in the same context, so what changes the enemy, such as
// the config, a curse or Exposure, changes it for both.
func (sh *Sheet) addMinionEnemy() error {
	src := sh.newSource(Source{RuleID: "minion-enemy", Rule: "the enemy a minion hits"})
	for _, stat := range enemyStats() {
		id := sh.stats[stat]
		mirror := core.Scale{Ref: core.Ref{Actor: int32(playerActor), Stat: id, Ctx: core.Self}, Step: 1, Ratio: true}
		mod := core.Modifier{Stat: id, Actor: minionActor, Op: core.OpBase, Value: 1, Source: src, Scales: []core.Scale{mirror}}
		if _, err := sh.session.AddModifier(mod); err != nil {
			return fmt.Errorf("sheet: minion enemy: %w", err)
		}
	}
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
		_ = sh.session.SetModifierValue(sh.enemy.maxima[d], float64(enemyMaximumResistance(values[d])))
	}
	_ = sh.session.SetModifierValue(sh.enemy.armour, float64(e.Armour))
	_ = sh.session.SetModifierValue(sh.enemy.evasion, float64(e.Evasion))
	_ = sh.session.SetModifierValue(sh.enemy.damageTaken, float64(enemyPresets()[e.Kind].DamageTaken))
	sh.setEnemyUnique(e.Kind)
}

// Formula names shown in explanations.
const (
	formulaHitEnemyResistance         = "enemy resistance a hit meets"
	formulaCriticalHitEnemyResistance = "enemy resistance a critical strike meets"
)

// mechanicEnemy names the enemy mechanic for those that come after it.
const mechanicEnemy = "enemy"

func enemyMechanic() mechanic {
	return mechanic{name: mechanicEnemy, stats: []string{
		statEnemyResistance,
		statEnemyMaximumResistance,
		statEnemyExposure,
		statExtraExposure,
		statMinimumExposure,
		statPenetration,
		statNonCriticalPenetration,
		statCriticalPenetration,
		statInvertedResistanceChance,
		statCriticalStrikesIgnoreResistance,
		statHitEnemyResistance,
		statCriticalHitEnemyResistance,
		statEnemyDamageTaken,
		statEnemyArmour,
		statEnemyEvasion,
	}, def: enemyDef}
}

// enemyDef completes def when it is a stat of the resistance a hit meets.
func enemyDef(def core.StatDef, ids map[string]core.StatID) core.StatDef {
	switch def.Name {
	case statEnemyExposure, statMinimumExposure:
		def.LowestBase = true
	case statInvertedResistanceChance:
		def.HasMin, def.Min, def.HasMax, def.Max = true, 0, true, percentScale
	case statCriticalStrikesIgnoreResistance:
		def.Flag = true
	case statHitEnemyResistance:
		h := newMetResistance(ids, hitNonCritical)
		def.FormulaName, def.Formula, def.Attribute = formulaHitEnemyResistance, h.formula(), h.attribute()
	case statCriticalHitEnemyResistance:
		h := newMetResistance(ids, hitCritical)
		def.FormulaName, def.Formula, def.Attribute = formulaCriticalHitEnemyResistance, h.formula(), h.attribute()
	}
	return def
}

// metBy is what meets a resistance: damage over time, a non-critical hit or a critical strike.
type metBy int

const (
	overTime metBy = iota
	hitNonCritical
	hitCritical
)

// metResistance is the resistance damage of one type meets, read in the type's context: the
// enemy's resistance with its Exposure, at least resistanceFloor and at most its maximum. A hit
// then treats it as inverted by its chance to, weighing the inverted and the actual resistance,
// and penetrates it, a non-critical hit also by what non-critical strikes penetrate and a
// critical strike by what critical strikes do; a critical strike that ignores the resistance meets none and penetrates nothing. Exposure is its
// strongest instance, and what the character adds to Exposure it inflicts, then its minimum,
// apply only while there is some. Penetration counts after the limits, so it can take the
// resistance below the floor; damage over time does not penetrate. All follow PoB
// (CalcOffence.lua calcResistForType, the hit's resistance with HitsInvertEleResChance and the
// skill's damage over time, CalcPerform.lua's exposures); the game text gives none of the order.
type metResistance struct {
	resistance, maximum, exposure, extra, minimum, penetration core.StatID
	nonCritical, critical, inverted, ignore                    core.StatID
	by                                                         metBy
}

func newMetResistance(ids map[string]core.StatID, by metBy) metResistance {
	return metResistance{
		resistance: mustID(ids, statEnemyResistance), maximum: mustID(ids, statEnemyMaximumResistance),
		exposure: mustID(ids, statEnemyExposure), extra: mustID(ids, statExtraExposure), minimum: mustID(ids, statMinimumExposure),
		penetration: mustID(ids, statPenetration), nonCritical: mustID(ids, statNonCriticalPenetration),
		critical: mustID(ids, statCriticalPenetration),
		inverted: mustID(ids, statInvertedResistanceChance), ignore: mustID(ids, statCriticalStrikesIgnoreResistance),
		by: by,
	}
}

// ignored reports whether a critical strike ignores the resistance, with stat reading a stat.
func (h metResistance) ignored(stat func(core.StatID) float64) bool {
	return h.by == hitCritical && stat(h.ignore) != 0
}

// limited returns the resistance with its Exposure within its limits, with stat reading a stat.
func (h metResistance) limited(stat func(core.StatID) float64) float64 {
	e, _ := h.exposed(stat)
	return min(max(stat(h.resistance)+e, resistanceFloor), stat(h.maximum))
}

// invertedPart returns what a hit's chance to treat the resistance v as inverted changes of it.
func (h metResistance) invertedPart(v float64, stat func(core.StatID) float64) float64 {
	if h.by == overTime {
		return 0
	}
	return -2 * v * stat(h.inverted) / percentScale
}

// penetrations returns the stats that penetrate the resistance.
func (h metResistance) penetrations() []core.StatID {
	switch h.by {
	case hitNonCritical:
		return []core.StatID{h.penetration, h.nonCritical}
	case hitCritical:
		return []core.StatID{h.penetration, h.critical}
	case overTime:
	}
	return nil
}

// exposed returns the Exposure the enemy has, with stat reading a stat, and how much of it its
// minimum adds.
func (h metResistance) exposed(stat func(core.StatID) float64) (e, raised float64) {
	if e = stat(h.exposure); e == 0 {
		return 0, 0
	}
	e += stat(h.extra)
	if m := stat(h.minimum); m < e {
		return m, m - e
	}
	return e, 0
}

func (h metResistance) formula() core.Formula {
	return func(r core.Reader, _ core.Agg) float64 {
		if h.ignored(r.Stat) {
			return 0
		}
		v := h.limited(r.Stat)
		v += h.invertedPart(v, r.Stat)
		for _, p := range h.penetrations() {
			v -= r.Stat(p)
		}
		return v
	}
}

// attribute credits the enemy's resistance, its Exposure, what its minimum adds, what treating
// it as inverted changes and any penetration, which counts against it; what the limits change
// goes to the maximum's sources. A critical strike that ignores the resistance credits what it
// takes off to the sources that make it ignore it.
func (h metResistance) attribute() core.Attribute {
	return func(a *core.Attributor) core.Attribution {
		total := a.Explanation().Total
		if h.ignored(a.Stat) {
			v := h.limited(a.Stat)
			parts, _ := a.Of(core.SelfRef(h.resistance))
			parts = rest(a, parts, v, core.SelfRef(h.maximum))
			return core.Attribution{Parts: append(parts, a.Spread(core.SelfRef(h.ignore), -v, core.OpMore)...), Amount: total}
		}
		parts, _ := a.Of(core.SelfRef(h.resistance))
		if e, raised := h.exposed(a.Stat); e != 0 {
			for _, s := range []core.StatID{h.exposure, h.extra} {
				p, _ := a.Of(core.SelfRef(s))
				parts = append(parts, p...)
			}
			if raised != 0 {
				p, _ := a.Of(core.SelfRef(h.minimum))
				parts = append(parts, scaled(p, raised/a.Stat(h.minimum))...)
			}
		}
		v := h.limited(a.Stat)
		parts = rest(a, parts, v, core.SelfRef(h.maximum))
		if inv := h.invertedPart(v, a.Stat); inv != 0 {
			parts = append(parts, a.Spread(core.SelfRef(h.inverted), inv, core.OpMore)...)
		}
		for _, p := range h.penetrations() {
			pen, _ := a.Of(core.SelfRef(p))
			parts = append(parts, scaled(pen, -1)...)
		}
		return core.Attribution{Parts: parts, Amount: total}
	}
}
