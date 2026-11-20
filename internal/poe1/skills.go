package poe1

import (
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/internal/semantics"
)

// statSkillStages, read in a skill's context, is the stages the skill has built up, as its gem
// says; the skill's stats per stage scale with it.
const statSkillStages = "skill_stages"

// Errors of a sheet's skills.
var (
	// ErrSkill is a skill the sheet does not hold, or more skills than it can hold.
	ErrSkill = errors.New("skill not on the sheet")
	// ErrDamageType is a value that is not a DamageType.
	ErrDamageType = errors.New("unknown damage type")
)

// skillSlot is the skill in a slot: its gem, types and weapon restrictions.
type skillSlot struct {
	// skill is a copy of the skill SetSkills got, and key its sources' key.
	skill        *resolve.Skill
	key          skillKey
	source       resolve.GemSource
	types        []string
	restrictions []string
}

// skillKey is a skill's sources by slot and content.
type skillKey struct {
	slot int
	key  string
}

// skillKeys returns the key of each of skills in its slot. Rendering a skill costs more than
// comparing it with the one the slot has, so a slot whose skill is unchanged keeps its key.
func (sh *Sheet) skillKeys(skills []*resolve.Skill) []skillKey {
	keys := make([]skillKey, len(skills))
	for i, sk := range skills {
		if i < len(sh.skills) && reflect.DeepEqual(sh.skills[i].skill, sk) {
			keys[i] = sh.skills[i].key
		} else {
			keys[i] = skillKey{i, fmt.Sprint(*sk)}
		}
	}
	return keys
}

// SetSkills makes skills the sheet's skills: skill i takes context slot i, and the sources of
// its own stats (the stat set it uses, quality included, and each applied support) go on,
// those of other skills off. A skill's own stats reach that skill's values only. Every skill
// that reserves life or mana reserves it from the sheet's pools; outputs of the skill group
// read skill 0. A skill not
// seen yet in its slot gets its sources added once; adding is refused while a checkpoint is
// open (ErrCheckpointOpen).
func (sh *Sheet) SetSkills(skills []*resolve.Skill) error {
	if len(skills) > maxSkills {
		return fmt.Errorf("%w: %d skills, at most %d", ErrSkill, len(skills), maxSkills)
	}
	keys := sh.skillKeys(skills)
	want := make(map[skillKey]bool, len(skills))
	for _, k := range keys {
		want[k] = true
	}
	for k, srcs := range sh.skillSources {
		sh.setSources(srcs, want[k])
	}
	sh.skills = sh.skills[:0]
	sh.deriveFrom = maxSkills
	for i, sk := range skills {
		cp := *sk
		sh.skills = append(sh.skills, skillSlot{
			skill: &cp, key: keys[i], source: sk.Source, types: slices.Clone(sk.Types), restrictions: slices.Clone(sk.WeaponRestrictions),
		})
		k := keys[i]
		if _, ok := sh.skillSources[k]; ok {
			continue
		}
		if sh.marked {
			return fmt.Errorf("%w: skill %s", ErrCheckpointOpen, sk.Source)
		}
		srcs, err := sh.addSkill(sk, i)
		if err != nil {
			return err
		}
		if sh.skillSources == nil {
			sh.skillSources = map[skillKey][]core.SourceID{}
		}
		sh.skillSources[k] = srcs
	}
	return nil
}

// addSkill adds the sources of sk's own stats in slot, of the buff it grants you, of the curse
// it applies to the enemy and of its stages.
func (sh *Sheet) addSkill(sk *resolve.Skill, slot int) ([]core.SourceID, error) {
	b, buffs := buffOf(sk)
	bundles, buffStats, curseStats := sh.skillBundles(sk, b, buffs)
	srcs := make([]core.SourceID, 0, len(bundles)+1)
	for i := range bundles {
		src, err := sh.add(&bundles[i], sh.table, sh.gameVersion, slot)
		if err != nil {
			return nil, err
		}
		srcs = append(srcs, src)
	}
	ctx := sh.session.InternContext(sh.ctx.skillMask(slot, sk.Types, wield{}, nil, formNone))
	src, err := sh.addSkillRules(sk, slot, ctx)
	if err != nil {
		return nil, err
	}
	hit, err := sh.addHitRules(sk, slot)
	if err != nil {
		return nil, err
	}
	srcs = append(srcs, src, hit)
	var more []core.SourceID
	switch {
	case buffs:
		more, err = sh.addBuff(sk, b, buffStats, slot, ctx)
	case curseStats != nil:
		more, err = sh.addCurse(sk, curseStats, slot, ctx)
	}
	if err != nil {
		return nil, err
	}
	srcs = append(srcs, more...)
	if sk.Stages > 0 {
		stages, err := sh.addStages(sk, slot)
		if err != nil {
			return nil, err
		}
		srcs = append(srcs, stages)
	}
	return srcs, nil
}

// addStages gives skill_stages the stages sk's gem says, in slot.
func (sh *Sheet) addStages(sk *resolve.Skill, slot int) (core.SourceID, error) {
	src := sh.newSource(Source{RuleID: "skill-stages", Rule: fmt.Sprintf("%d stages of %s", sk.Stages, sk.Source)})
	pred, err := predicate(sh.session.Model(), semantics.ContextFilter{}, sh.ctx, slot)
	if err != nil {
		return 0, err
	}
	mod := core.Modifier{Stat: sh.stats[statSkillStages], Op: core.OpBase, Value: float64(sk.Stages), Source: src, Pred: pred}
	if _, err := sh.session.AddModifier(mod); err != nil {
		return 0, fmt.Errorf("sheet: skill stages: %w", err)
	}
	return src, nil
}

// skillBundles returns the bundles of sk's own stats: its stat set's and its supports'. Of its
// stat set, it returns apart the stats of its buff b when buffs is set, and those of its curse
// when it is one, non-nil then.
func (sh *Sheet) skillBundles(sk *resolve.Skill, b skillBuff, buffs bool) (bundles []resolve.Bundle, buff, curse []resolve.StatValue) {
	bundles = make([]resolve.Bundle, 0, 1+len(sk.Supports))
	if len(sk.StatSets) > 0 {
		own := sk.Set().Stats
		switch {
		case buffs:
			own, buff = splitBuff(own, b)
		case IsCurse(sk):
			own, curse = sh.splitCurse(own)
		}
		bundles = append(bundles, skillBundle(sk.Source, own))
	}
	for _, sp := range sk.Supports {
		bundles = append(bundles, skillBundle(sp.Source, sp.Stats))
	}
	return bundles, buff, curse
}

func skillBundle(src resolve.Source, stats []resolve.StatValue) resolve.Bundle {
	return resolve.Bundle{Actor: resolve.Player, Scope: resolve.Global, Source: src, Stats: stats}
}

// SkillValue returns engine stat name in the context of skill i of SetSkills, reading the part
// of its damage whose chain is damage: its type and the types it was converted from. With no
// damage types it reads no part of the damage.
func (sh *Sheet) SkillValue(i int, name string, damage ...DamageType) (float64, error) {
	n, err := sh.skillNode(i, name, damage, formNone)
	if err != nil {
		return 0, err
	}
	val, err := sh.session.Get(n)
	if err != nil {
		return 0, fmt.Errorf("sheet: skill %d %s: %w", i, name, err)
	}
	return val, nil
}

// handRead is which hand's values a skill's output reads.
type handRead uint8

const (
	// readSkill reads the skill's: for an attack with both hands, the combined values where a
	// stat combines them (handBits.hands), else the main hand's.
	readSkill handRead = iota
	readMainHand
	readOffHand
)

// skillNode returns the node of engine stat name in skill i's context for the damage chain
// damage in form.
func (sh *Sheet) skillNode(i int, name string, damage []DamageType, form damageForm) (core.Node, error) {
	return sh.skillHandNode(i, name, damage, form, readSkill)
}

// skillHandNode returns the node of engine stat name in skill i's context for the damage chain
// damage in form, read with the hand read says.
func (sh *Sheet) skillHandNode(i int, name string, damage []DamageType, form damageForm, read handRead) (core.Node, error) {
	id, ok := sh.stats[name]
	switch {
	case !ok:
		return core.Node{}, fmt.Errorf("%w %q", ErrUnknownStat, name)
	case i < 0 || i >= len(sh.skills):
		return core.Node{}, fmt.Errorf("%w: %d of %d", ErrSkill, i, len(sh.skills))
	}
	for _, d := range damage {
		if d < Physical || d > Chaos {
			return core.Node{}, fmt.Errorf("%w %d", ErrDamageType, d)
		}
	}
	w := sh.wieldOf(i, read)
	types := sh.skills[i].types
	return core.Node{Stat: id, Ctx: sh.session.InternContext(sh.ctx.skillMask(i, types, w, damage, form))}, nil
}

// wieldOf returns what skill i is read with for read: none when it is not an attack or the
// character has nothing to attack with, else the main hand, or the off hand for readOffHand
// when the skill attacks with both.
func (sh *Sheet) wieldOf(i int, read handRead) wield {
	sl, h := sh.skills[i], sh.hands
	if !slices.Contains(sl.types, typeAttack) || h.MainHand == nil {
		return wield{}
	}
	w := wield{attack: true, hand: handMain, weapon: h.MainHand.Kinds, dualWielding: h.dualWielding()}
	if !h.bothHands(sl.types, sl.restrictions) {
		return w
	}
	switch read {
	case readSkill:
		w.offWeapon = h.OffHand.Kinds
	case readOffHand:
		w.hand, w.weapon = handOff, h.OffHand.Kinds
	case readMainHand:
	}
	return w
}

// baseDamage is damage before modifiers: the base damage, 100%.
const baseDamage = 100

// addBaseDamage adds the base damage, and of the smallest and largest damage alone, as a rule's
// source, so increases and more multipliers apply to them by the default rule.
func (sh *Sheet) addBaseDamage() error {
	src := sh.newSource(Source{RuleID: "base-damage", Rule: "base damage (100%)"})
	for _, stat := range []string{statDamage, statMinimumDamage, statMaximumDamage} {
		mod := core.Modifier{Stat: sh.stats[stat], Op: core.OpBase, Value: baseDamage, Source: src}
		if _, err := sh.session.AddModifier(mod); err != nil {
			return fmt.Errorf("sheet: base damage: %w", err)
		}
	}
	return nil
}

func skillStageMechanic() mechanic {
	return mechanic{name: "skill stages", stats: []string{statSkillStages}}
}
