package poe1

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/xaroth/pbt-engine/build"
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
	// Skill is the minion's main skill, its variety's first, at the minion's level, with the
	// summoning skill's supports that support it; nil when it has none that resolves.
	Skill *resolve.Skill
	// Passed are the summoning skill's own stats that its minion's skill has too
	// (passedStats).
	Passed []resolve.StatValue
	// Items are the items the minion uses, for a skill whose minion takes gear (equip), and
	// Hands the weapons they put in its hands.
	Items []Item
	Hands Hands
}

// MinionGear are the compiled items of the minions that take gear, by the index of the skill
// group whose main skill summons them (build.SkillGroup.MinionItems).
type MinionGear map[int][]Item

// passedStats are, by granted effect, the stats of a summoning skill that its minion's skill
// has too, as PoB's skill data passes them (Data/Skills, a statMap entry with a MinionModifier):
// a stat whose meaning is the skill's own, which other skills do not pass. Stats only minions
// read, such as minion_damage_+%, reach them through the table.
func passedStats() map[string][]string {
	conversion := []string{"skill_physical_damage_%_to_convert_to_lightning"}
	guardiansWeapon := []string{"attack_minimum_added_physical_damage", "attack_maximum_added_physical_damage", "active_skill_damage_+%_final"}
	return map[string][]string{"Absolution": conversion, "AbsolutionAltX": conversion, "AnimateGuardiansWeapon": guardiansWeapon}
}

// Monster varieties of minions.
const (
	varietyAnimatedGuardian = "Metadata/Monsters/AnimatedItem/AnimatedArmour"
	varietyAnimatedWeapon   = "Metadata/Monsters/AnimatedItem/AnimatedWeapon"
)

// minionKind is a kind of minion that stats name, a value of the minion dimension.
type minionKind uint8

const (
	minionAnimatedGuardian minionKind = iota
)

func minionKindNames() []string { return []string{minionAnimatedGuardian: "animated_guardian"} }

// minionKinds are the kinds of minions by monster variety.
func minionKinds() map[string]minionKind {
	return map[string]minionKind{varietyAnimatedGuardian: minionAnimatedGuardian}
}

// namedVarieties are, by granted effect, the monster varieties of the minions of skills whose
// stats do not name them, as PoB's skill data lists them (minionList).
func namedVarieties() map[string]string {
	return map[string]string{"AnimateGuardiansWeapon": varietyAnimatedWeapon}
}

// passed returns the stats of sk that its minion's skill has too.
func passed(sk *resolve.Skill) []resolve.StatValue {
	ids := passedStats()[sk.GrantedEffect]
	var out []resolve.StatValue
	for _, sv := range sk.Set().Stats {
		if slices.Contains(ids, sv.Stat) {
			out = append(out, sv)
		}
	}
	return out
}

// Hit reports whether the sheet gives mn's hit: its main skill is an attack with the minion's
// own damage or a weapon of its items. A minion's spell needs its damage at the minion's level,
// which the dataset does not give.
func (mn *Minion) Hit() bool {
	return mn != nil && mn.Skill != nil && hasTypes(mn.Skill, typeAttack) && (mn.MaximumDamage > 0 || mn.armed())
}

// equip gives mn items: their stats are the minion's, and a weapon in the main hand replaces its
// monster's damage, attacks per second and critical strike chance, with the off hand's weapon
// for a second hand, as PoB has it (CalcActiveSkill.lua minion weaponData1 and weaponData2,
// CalcPerform.lua minion itemList). An off hand weapon without one in the main hand is left
// out: the minion's own attack is not a weapon to dual wield with.
func (mn *Minion) equip(items []Item) {
	mn.Items = items
	mn.Hands = HandsOf(items, nil)
	if mn.Hands.MainHand == nil {
		mn.Hands.OffHand = nil
	}
}

// armed reports whether mn attacks with a weapon of its items.
func (mn *Minion) armed() bool { return mn.Hands.MainHand != nil }

// SummonsMinion reports whether sk summons a minion whose monster the game or namedVarieties
// names.
func SummonsMinion(sk *resolve.Skill) bool {
	if _, ok := namedVarieties()[sk.GrantedEffect]; ok {
		return true
	}
	return len(sk.StatSets) > 0 && slices.ContainsFunc(sk.Set().Stats, func(sv resolve.StatValue) bool {
		return sv.Stat == statDisplayMinionMonsterType
	})
}

// varietyOf returns the id of the monster variety of the minion sk summons in m, false for none.
func varietyOf(sk *resolve.Skill, m *dataset.Monsters) (string, bool) {
	if id, ok := namedVarieties()[sk.GrantedEffect]; ok {
		return id, true
	}
	display := int32(statOf(sk.Set().Stats, statDisplayMinionMonsterType))
	d := find(m.DisplayMinions, func(d *dataset.DisplayMinion) bool { return d.ID == display })
	if d == nil {
		return "", false
	}
	return d.Variety, true
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
	id, ok := varietyOf(sk, m)
	if !ok {
		return Minion{}, false, nil
	}
	v := find(m.Varieties, func(v *dataset.MonsterVariety) bool { return v.ID == id })
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
		mn.Skill, _ = resolve.MinionSkill(x, stats, v.GrantedEffects[0], mn.Level, sk)
	}
	if mn.Skill != nil {
		mn.Passed = passed(sk)
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
		Armour:            roundHalf(float64(monsterArmour(int(l.Level))) * typeScale(t.Armour)),
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
// slots, by slot: nil for a skill that summons none. A minion that takes gear uses its group's
// (gear). It returns none when no such skill summons one or the dataset has no monsters group.
func minionsOf(sw *Switch, gear MinionGear) ([]*Minion, error) {
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
			if build.MinionSlots(sk.GrantedEffect) != nil {
				mn.equip(gear[sk.Source.Group])
			}
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

// keyOf returns mn's key in slot, by its values, its skill's and its items' keys, not their
// addresses: each switch resolves the skill and compiles the items anew.
func keyOf(slot int, mn *Minion) minionKey {
	values := *mn
	values.Skill, values.Items, values.Hands = nil, nil, Hands{}
	k := minionKey{slot: slot, key: fmt.Sprint(values)}
	if mn.Skill != nil {
		k.key += fmt.Sprint(*mn.Skill)
	}
	for _, it := range mn.Items {
		k.key += fmt.Sprintf("|%s %s", it.Slot, it.Key)
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
// the damage dimension. A minion armed with a weapon attacks with the weapon's damage, attack
// rate and critical strike chance instead (addWeapon).
func (mn *Minion) bases(dim core.Dimension, attack core.Mask) []minionBase {
	physicalAttack := []core.Mask{attack, damageBit(dim, Physical)}
	own := []minionBase{
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
		{statHitsCannotBeEvaded, core.OpFlag, 1, []core.Mask{attack}},
	}
	if mn.armed() {
		return own
	}
	return slices.Concat(own, []minionBase{
		{statBaseMinimumDamage, core.OpBase, mn.MinimumDamage, physicalAttack},
		{statBaseMaximumDamage, core.OpBase, mn.MaximumDamage, physicalAttack},
		{statAttackRate, core.OpBase, mn.AttackRate, []core.Mask{attack}},
		{statCriticalStrikeChance, core.OpBase, minionAttackCriticalChance, []core.Mask{attack}},
	})
}

// addMinion adds mn's base values, its weapons, its items' stats, and its skill's stats and hit,
// as sources whose modifiers reach the minion actor in the context of the skill in slot.
func (sh *Sheet) addMinion(mn *Minion, slot int) ([]core.SourceID, error) {
	src, err := sh.addMinionBases(mn, slot)
	if err != nil {
		return nil, err
	}
	srcs := []core.SourceID{src}
	skill := core.MaskOf(sh.ctx.skill.Bit(slot))
	for hd, w := range []*Weapon{handMain: mn.Hands.MainHand, handOff: mn.Hands.OffHand} {
		if w == nil {
			continue
		}
		src, err := sh.addWeapon(w, hand(hd), minionActor, skill)
		if err != nil {
			return nil, err
		}
		srcs = append(srcs, src)
	}
	if srcs, err = sh.addBundles(srcs, mn.itemBundles(), slot); err != nil {
		return nil, err
	}
	if mn.Skill == nil {
		return srcs, nil
	}
	srcs, err = sh.addBundles(srcs, mn.bundles(), slot)
	if err != nil {
		return nil, err
	}
	hit, err := sh.addHitRules(mn.Skill, slot, minionActor)
	if err != nil {
		return nil, err
	}
	return append(srcs, hit), nil
}

// addMinionBases adds mn's base values to the minion actor in the context of the skill in slot,
// and returns their source.
func (sh *Sheet) addMinionBases(mn *Minion, slot int) (core.SourceID, error) {
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
			return 0, fmt.Errorf("sheet: minion %s: %w", mn.Variety, err)
		}
	}
	return src, nil
}

// bundles returns the bundles of the stats of mn's skill: its stat set's, those the summoning
// skill passes and its supports', less the support stats that act on the character.
func (mn *Minion) bundles() []resolve.Bundle {
	sk := mn.Skill
	var out []resolve.Bundle
	if len(sk.StatSets) > 0 {
		out = append(out, minionBundle(sk.Source, sk.Set().Stats))
	}
	if len(mn.Passed) > 0 {
		out = append(out, minionBundle(sk.Source, mn.Passed))
	}
	for _, sp := range sk.Supports {
		own, _ := splitCharacterStats(sp.Stats)
		out = append(out, minionBundle(sp.Source, own))
	}
	return out
}

// itemBundles returns the bundles of the stats of mn's items.
func (mn *Minion) itemBundles() []resolve.Bundle {
	out := make([]resolve.Bundle, 0, len(mn.Items))
	for _, it := range mn.Items {
		out = append(out, minionBundle(resolve.ItemSource{Slot: it.Slot, Name: it.Name}, it.Stats))
	}
	return out
}

func minionBundle(src resolve.Source, stats []resolve.StatValue) resolve.Bundle {
	return resolve.Bundle{Actor: resolve.Minion, Scope: resolve.Global, Source: src, Stats: stats}
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
	var mn *Minion
	if i < len(sh.slotMinions) {
		mn = sh.slotMinions[i]
	}
	if mn != nil && mn.Skill != nil {
		types = mn.Skill.Types
		w = mn.wield()
	}
	mask := sh.ctx.minionMask(mn, sh.ctx.skillMask(i, types, w, damage, form))
	return core.Node{Actor: minionActor, Stat: n.Stat, Ctx: sh.session.InternContext(mask)}, nil
}

// minionMask returns m with the kind of mn, if it is one stats name.
func (c contexts) minionMask(mn *Minion, m core.Mask) core.Mask {
	if mn == nil {
		return m
	}
	if k, ok := minionKinds()[mn.Variety]; ok {
		m = m.Set(c.minion.Bit(int(k)))
	}
	return m
}

// wield returns what mn's skill attacks with: its main hand, with its weapon when armed, and
// both hands when its off hand holds a weapon the skill can be used with. Not an attack, nothing.
func (mn *Minion) wield() wield {
	sk, h := mn.Skill, mn.Hands
	if !slices.Contains(sk.Types, typeAttack) {
		return wield{}
	}
	w := wield{attack: true, hand: handMain}
	if !mn.armed() {
		return w
	}
	w.weapon, w.dualWielding = h.MainHand.Kinds, h.dualWielding()
	if h.bothHands(sk.Types, sk.WeaponRestrictions) {
		w.offWeapon = h.OffHand.Kinds
	}
	return w
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
