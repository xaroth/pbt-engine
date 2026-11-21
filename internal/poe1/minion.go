package poe1

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// The actors of a sheet's session. The minion actor holds the minion of every skill that
// summons one, each in its skill's context.
const (
	playerActor core.ActorID = iota
	minionActor
	sheetActors
)

// Game stats of a skill that name the minion it summons and set its level, and of Monster.ot.
const (
	statDisplayMinionMonsterType               = "display_minion_monster_type"
	statBaseDisplayMinionActorLevel            = "base_display_minion_actor_level"
	statMinionActorLevelIsUserLevelUpToMaximum = "minion_actor_level_is_user_level_up_to_maximum"
	statMonsterBaseMaximumAllResistances       = "base_maximum_all_resistances_%"
	statMonsterBaseCriticalStrikeMultiplier    = "base_critical_strike_multiplier"
)

// A minion's attack, as PoB makes it (CalcActiveSkill.lua minion weaponData1, CalcPerform.lua
// "Minion Attacks always hit"): 5% critical strike chance, and hits that cannot be evaded. The
// game data read here gives neither.
const minionAttackCriticalChance = 5

// minionEnergyShieldOfLife is the share of its life a minion has as energy shield per 100 of its
// monster type's energy shield, as PoB exports it (Export/Scripts/minions.lua energyShield).
const minionEnergyShieldOfLife = 0.4

// Monster levels, as DefaultMonsterStats holds them.
const (
	minMonsterLevel = 1
	maxMonsterLevel = 100
)

// Minion is the minion a skill summons: its monster variety and level, and the base values
// they give it. Values follow PoB (CalcActiveSkill.lua minion creation, CalcPerform.lua minion
// base mods), read from the game's monster tables.
type Minion struct {
	// Variety is the MonsterVarieties id.
	Variety string
	Level   int
	// Life and EnergyShield are its maximum before modifiers.
	Life, EnergyShield float64
	Armour, Evasion    float64
	// Resistances are its resistances, MaximumResistance the maximum of each.
	Resistances       dataset.Resistances
	MaximumResistance float64
	// MinimumDamage and MaximumDamage are the physical damage of its attacks, AttackRate its
	// attacks per second.
	MinimumDamage, MaximumDamage float64
	AttackRate                   float64
	// CriticalStrikeMultiplier is every monster's, from Monster.ot.
	CriticalStrikeMultiplier float64
	// Skill is the minion's main skill, its variety's first, at the minion's level; nil when it
	// has none that resolves.
	Skill *resolve.Skill
}

// Hit reports whether the sheet gives mn's hit: its main skill is an attack with the minion's
// own damage. A minion that attacks with items, such as an Animated Guardian, has none, and a
// minion's spell needs its damage at the minion's level, which the dataset does not give.
func (mn *Minion) Hit() bool {
	return mn != nil && mn.Skill != nil && hasTypes(mn.Skill, typeAttack) && mn.MaximumDamage > 0
}

// SummonsMinion reports whether sk summons a minion whose monster the game names.
func SummonsMinion(sk *resolve.Skill) bool {
	return len(sk.StatSets) > 0 && slices.ContainsFunc(sk.Set().Stats, func(sv resolve.StatValue) bool {
		return sv.Stat == statDisplayMinionMonsterType
	})
}

// minionLevel returns the level of sk's minions for a character of characterLevel: the
// character's up to the maximum the skill says, else the level its level row gives them, else
// the level the row requires, as PoB has it (CalcActiveSkill.lua minion.level). Monster
// levels run from 1 to 100.
func minionLevel(sk *resolve.Skill, characterLevel int) int {
	stats := sk.Set().Stats
	level := sk.LevelRequirement
	if i := slices.IndexFunc(stats, func(sv resolve.StatValue) bool { return sv.Stat == statMinionActorLevelIsUserLevelUpToMaximum }); i >= 0 {
		level = min(characterLevel, int(stats[i].Value))
	} else if i := slices.IndexFunc(stats, func(sv resolve.StatValue) bool { return sv.Stat == statBaseDisplayMinionActorLevel }); i >= 0 {
		level = int(stats[i].Value)
	}
	return min(max(level, minMonsterLevel), maxMonsterLevel)
}

// MinionOf returns the minion sk summons, for a character of characterLevel, from the monsters
// group m and skills x of a dataset with stats; false when sk summons none or m lacks its
// monster. A level m lacks is an error.
func MinionOf(sk *resolve.Skill, characterLevel int, m *dataset.Monsters, x *dataset.SkillIndex, stats []dataset.Stat) (Minion, bool, error) {
	if !SummonsMinion(sk) {
		return Minion{}, false, nil
	}
	display := int32(statOf(sk.Set().Stats, statDisplayMinionMonsterType))
	d := find(m.DisplayMinions, func(d *dataset.DisplayMinion) bool { return d.ID == display })
	if d == nil {
		return Minion{}, false, nil
	}
	v := find(m.Varieties, func(v *dataset.MonsterVariety) bool { return v.ID == d.Variety })
	if v == nil {
		return Minion{}, false, nil
	}
	t := find(m.Types, func(t *dataset.MonsterType) bool { return t.ID == v.Type })
	if t == nil {
		return Minion{}, false, nil
	}
	level := minionLevel(sk, characterLevel)
	l := find(m.Levels, func(l *dataset.MonsterLevel) bool { return int(l.Level) == level })
	if l == nil {
		return Minion{}, false, fmt.Errorf("poe1: no monster values for minion level %d: %w", level, dataset.ErrNotInDataset)
	}
	mn := minionOf(v, t, l, m, monsterBase(m, stats, statMonsterBaseMaximumAllResistances, enemyBaseMaximumResistance))
	mn.CriticalStrikeMultiplier = monsterBase(m, stats, statMonsterBaseCriticalStrikeMultiplier, 0)
	if len(v.GrantedEffects) > 0 {
		// Not an error: a minion whose skill does not resolve has no hit.
		mn.Skill, _ = resolve.MonsterSkill(x, stats, v.GrantedEffects[0], mn.Level, sk.Source)
	}
	return mn, true, nil
}

// find returns the first element of s f holds for, nil for none.
func find[T any](s []T, f func(*T) bool) *T {
	for i := range s {
		if f(&s[i]) {
			return &s[i]
		}
	}
	return nil
}

// minionOf returns the minion of variety v of type t at level l. A player's minion takes an
// allied monster's life and damage per level, any other monster a monster's; an allied monster
// type with an alternative life scaling takes that.
func minionOf(v *dataset.MonsterVariety, t *dataset.MonsterType, l *dataset.MonsterLevel, m *dataset.Monsters, maxRes float64) Minion {
	life, damage := float64(l.Life), float64(l.Damage)
	if t.PlayerMinion {
		life, damage = float64(l.AllyLife), float64(l.AllyDamage)
		switch {
		case t.AltLife1:
			life = float64(l.AltLife1)
		case t.AltLife2:
			life = float64(l.AltLife2)
		}
	}
	mn := Minion{
		Variety:           v.ID,
		Level:             int(l.Level),
		Life:              math.Floor(life * (float64(v.LifeMultiplier) / percentScale)),
		Armour:            roundHalf(float64(l.Armour) * typeScale(t.Armour)),
		Evasion:           roundHalf(float64(l.Evasion) * typeScale(t.Evasion)),
		MaximumResistance: maxRes,
	}
	if t.EnergyShield != 0 {
		// Energy shield scales from allied life whatever the life scaling (CalcPerform.lua esTable).
		es := float64(l.AllyLife) * (float64(v.LifeMultiplier) / percentScale) * (minionEnergyShieldOfLife * float64(t.EnergyShield) / percentScale)
		mn.EnergyShield = math.Floor(es)
	}
	if r := find(m.Resistances, func(r *dataset.MonsterResistance) bool { return r.ID == t.Resistances }); r != nil {
		mn.Resistances = r.Merciless
	}
	// Grouped as PoB multiplies, so values at a rounding boundary round the same.
	hit := damage * (float64(v.DamageMultiplier) / percentScale)
	if !t.BaseDamageIgnoresAttackSpeed {
		hit *= float64(v.AttackTime) / millisecondsPerSecond
	}
	spread := float64(t.DamageSpread) / percentScale
	mn.MinimumDamage, mn.MaximumDamage = roundHalf(hit*(1-spread)), roundHalf(hit*(1+spread))
	if v.AttackTime > 0 {
		mn.AttackRate = millisecondsPerSecond / float64(v.AttackTime)
	}
	return mn
}

// typeScale is a monster type's percentage of a defence per level, 100% when it has none.
func typeScale(pct int32) float64 {
	if pct == 0 {
		return 1
	}
	return float64(pct) / percentScale
}

// monsterBase is stat id of the base stats of Monster.ot in m, with stats the dataset's stats;
// otherwise when they do not hold it.
func monsterBase(m *dataset.Monsters, stats []dataset.Stat, id string, otherwise float64) float64 {
	for _, sv := range m.BaseStats {
		if int(sv.Stat) < len(stats) && stats[sv.Stat].ID == id {
			return float64(sv.Value)
		}
	}
	return otherwise
}

// minionsOf returns the minion of each skill of sw whose outputs are read, which are the first
// slots, by slot: nil for a skill that summons none. It returns none when no such skill summons
// one or the dataset has no monsters group.
func minionsOf(sw *Switch) ([]*Minion, error) {
	read := sw.Skills.Read()
	if !slices.ContainsFunc(read, SummonsMinion) {
		return nil, nil
	}
	m, err := sw.Checked.Store().Monsters()
	switch {
	case errors.Is(err, dataset.ErrNotInDataset):
		return nil, nil
	case err != nil:
		return nil, err //nolint:wrapcheck // wrapped by the caller
	}
	stats, err := sw.Checked.Store().Stats()
	if err != nil {
		return nil, err //nolint:wrapcheck // wrapped by the caller
	}
	x, err := sw.Checked.Store().SkillIndex()
	if err != nil {
		return nil, err //nolint:wrapcheck // wrapped by the caller
	}
	out := make([]*Minion, len(read))
	for i, sk := range read {
		mn, ok, err := MinionOf(sk, sw.Checked.Build().Level, m, x, stats)
		switch {
		case err != nil:
			return nil, err
		case ok:
			out[i] = &mn
		}
	}
	return out, nil
}

// minionKey is a minion's slot and its values with its skill's.
type minionKey struct {
	slot int
	key  string
}

func keyOf(slot int, mn *Minion) minionKey {
	k := minionKey{slot: slot, key: fmt.Sprint(*mn)}
	if mn.Skill != nil {
		k.key += fmt.Sprint(*mn.Skill)
	}
	return k
}

// SetMinions makes minions the minions of the skills of SetSkills, by slot: each nil entry
// summons none. A minion's base values and its skill's stats and hit hold for the minion actor
// in its summoning skill's context only. A minion not seen yet in its slot gets its sources
// added once; adding is refused while a checkpoint is open (ErrCheckpointOpen).
func (sh *Sheet) SetMinions(minions []*Minion) error {
	want := map[minionKey]*Minion{}
	for i, mn := range minions {
		if mn != nil {
			want[keyOf(i, mn)] = mn
		}
	}
	for k, srcs := range sh.minions {
		sh.setSources(srcs, want[k] != nil)
	}
	sh.slotMinions = slices.Clone(minions)
	for k, mn := range want {
		if _, ok := sh.minions[k]; ok {
			continue
		}
		if sh.marked {
			return fmt.Errorf("%w: minion %s", ErrCheckpointOpen, mn.Variety)
		}
		srcs, err := sh.addMinion(mn, k.slot)
		if err != nil {
			return err
		}
		if sh.minions == nil {
			sh.minions = map[minionKey][]core.SourceID{}
		}
		sh.minions[k] = srcs
	}
	return nil
}

// minionBase is one of a minion's base values as a modifier, in the contexts of req besides
// its skill's.
type minionBase struct {
	stat  string
	op    core.Op
	value float64
	req   []core.Mask
}

// bases returns mn's base values; damage, attack rate, critical strike chance and that its hits
// cannot be evaded in the context of attacks, damage in that of physical damage too, with dim
// the damage dimension.
func (mn *Minion) bases(dim core.Dimension, attack core.Mask) []minionBase {
	physicalAttack := []core.Mask{attack, damageBit(dim, Physical)}
	return []minionBase{
		{resolve.LevelStat, core.OpBase, float64(mn.Level), nil},
		{statMaximumLife, core.OpBase, mn.Life, nil},
		{statMaximumEnergyShield, core.OpBase, mn.EnergyShield, nil},
		{statArmour, core.OpBase, mn.Armour, nil},
		{statEvasionRating, core.OpBase, mn.Evasion, nil},
		{statFireResistance, core.OpBase, float64(mn.Resistances.Fire), nil},
		{statColdResistance, core.OpBase, float64(mn.Resistances.Cold), nil},
		{statLightningResistance, core.OpBase, float64(mn.Resistances.Lightning), nil},
		{statChaosResistance, core.OpBase, float64(mn.Resistances.Chaos), nil},
		{statMaximumFireResistance, core.OpBase, mn.MaximumResistance, nil},
		{statMaximumColdResistance, core.OpBase, mn.MaximumResistance, nil},
		{statMaximumLightningResistance, core.OpBase, mn.MaximumResistance, nil},
		{statMaximumChaosResistance, core.OpBase, mn.MaximumResistance, nil},
		{statCriticalStrikeMultiplier, core.OpBase, mn.CriticalStrikeMultiplier, nil},
		{statBaseMinimumDamage, core.OpBase, mn.MinimumDamage, physicalAttack},
		{statBaseMaximumDamage, core.OpBase, mn.MaximumDamage, physicalAttack},
		{statAttackRate, core.OpBase, mn.AttackRate, []core.Mask{attack}},
		{statCriticalStrikeChance, core.OpBase, minionAttackCriticalChance, []core.Mask{attack}},
		{statHitsCannotBeEvaded, core.OpFlag, 1, []core.Mask{attack}},
	}
}

// addMinion adds mn's base values, and its skill's stats and hit, as sources whose modifiers
// reach the minion actor in the context of the skill in slot.
func (sh *Sheet) addMinion(mn *Minion, slot int) ([]core.SourceID, error) {
	src := sh.newSource(Source{RuleID: "minion-base", Rule: fmt.Sprintf("base values of %s at level %d", mn.Variety, mn.Level)})
	attack, ok := sh.session.Model().Bit(dimType + "." + typeAttack)
	if !ok {
		panic("poe1: no attack context")
	}
	skill := core.MaskOf(sh.ctx.skill.Bit(slot))
	for _, b := range mn.bases(sh.ctx.damage, core.MaskOf(attack)) {
		if b.value == 0 {
			continue
		}
		pred := core.Predicate{Req: append([]core.Mask{skill}, b.req...)}
		mod := core.Modifier{Actor: minionActor, Stat: sh.stats[b.stat], Op: b.op, Value: b.value, Source: src, Pred: pred}
		if _, err := sh.session.AddModifier(mod); err != nil {
			return nil, fmt.Errorf("sheet: minion %s: %w", mn.Variety, err)
		}
	}
	srcs := []core.SourceID{src}
	if mn.Skill == nil {
		return srcs, nil
	}
	sk := mn.Skill
	if len(sk.StatSets) > 0 {
		bd := resolve.Bundle{Actor: resolve.Minion, Scope: resolve.Global, Source: sk.Source, Stats: sk.Set().Stats}
		own, err := sh.add(&bd, sh.table, sh.gameVersion, slot)
		if err != nil {
			return nil, err
		}
		srcs = append(srcs, own)
	}
	hit, err := sh.addHitRules(sk, slot, minionActor)
	if err != nil {
		return nil, err
	}
	return append(srcs, hit), nil
}

// minionNode returns the node of engine stat name of the minion of skill i, in that skill's
// context with its minion's skill's types, attacking with its main hand, for the damage chain
// damage in form. Without a minion skill the context has no types.
func (sh *Sheet) minionNode(i int, name string, damage []DamageType, form damageForm) (core.Node, error) {
	n, err := sh.skillNode(i, name, damage, form)
	if err != nil {
		return core.Node{}, err
	}
	var types []string
	var w wield
	if i < len(sh.slotMinions) && sh.slotMinions[i] != nil && sh.slotMinions[i].Skill != nil {
		types = sh.slotMinions[i].Skill.Types
		if slices.Contains(types, typeAttack) {
			w = wield{attack: true, hand: handMain}
		}
	}
	return core.Node{Actor: minionActor, Stat: n.Stat, Ctx: sh.session.InternContext(sh.ctx.skillMask(i, types, w, damage, form))}, nil
}

// MinionValue returns engine stat name of the minion of skill i of SetSkills, in that skill's
// context, reading the part of its damage whose chain is damage as SkillValue does. A skill
// without a minion reads 0.
func (sh *Sheet) MinionValue(i int, name string, damage ...DamageType) (float64, error) {
	n, err := sh.skillNode(i, name, damage, formNone)
	if err != nil {
		return 0, err
	}
	n.Actor = minionActor
	val, err := sh.session.Get(n)
	if err != nil {
		return 0, fmt.Errorf("sheet: minion of skill %d %s: %w", i, name, err)
	}
	return val, nil
}
