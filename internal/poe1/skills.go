package poe1

import (
	"errors"
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// Errors of a sheet's skills.
var (
	// ErrSkill is a skill the sheet does not hold, or more skills than it can hold.
	ErrSkill = errors.New("skill not on the sheet")
	// ErrDamageType is a value that is not a DamageType.
	ErrDamageType = errors.New("unknown damage type")
)

// skillSlot is the skill in a slot: its gem and types.
type skillSlot struct {
	source resolve.GemSource
	types  []string
}

// skillKey is a skill's sources by slot and content.
type skillKey struct {
	slot int
	key  string
}

// SetSkills makes skills the sheet's skills: skill i takes context slot i, and the sources of
// its own stats (its primary stat set, quality included, and each applied support) go on,
// those of other skills off. A skill's own stats reach that skill's values only. Every skill
// that reserves life or mana reserves it from the sheet's pools; outputs of the skill group
// read skill 0. A skill not
// seen yet in its slot gets its sources added once; adding is refused while a checkpoint is
// open (ErrCheckpointOpen).
func (sh *Sheet) SetSkills(skills []*resolve.Skill) error {
	if len(skills) > maxSkills {
		return fmt.Errorf("%w: %d skills, at most %d", ErrSkill, len(skills), maxSkills)
	}
	want := make(map[skillKey]bool, len(skills))
	for i, sk := range skills {
		want[skillKey{i, fmt.Sprint(*sk)}] = true
	}
	for k, srcs := range sh.skillSources {
		sh.setSources(srcs, want[k])
	}
	sh.skills = sh.skills[:0]
	for i, sk := range skills {
		sh.skills = append(sh.skills, skillSlot{source: sk.Source, types: slices.Clone(sk.Types)})
		k := skillKey{i, fmt.Sprint(*sk)}
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

// addSkill adds the sources of sk's own stats in slot.
func (sh *Sheet) addSkill(sk *resolve.Skill, slot int) ([]core.SourceID, error) {
	bundles := make([]resolve.Bundle, 0, 1+len(sk.Supports))
	if len(sk.StatSets) > 0 {
		bundles = append(bundles, skillBundle(sk.Source, sk.StatSets[0].Stats))
	}
	for _, sp := range sk.Supports {
		bundles = append(bundles, skillBundle(sp.Source, sp.Stats))
	}
	srcs := make([]core.SourceID, 0, len(bundles)+1)
	for i := range bundles {
		src, err := sh.add(&bundles[i], sh.table, sh.gameVersion, slot)
		if err != nil {
			return nil, err
		}
		srcs = append(srcs, src)
	}
	ctx := sh.session.InternContext(sh.ctx.skillMask(slot, sk.Types, 0, nil, formNone))
	src, err := sh.addSkillRules(sk, slot, ctx)
	if err != nil {
		return nil, err
	}
	hit, err := sh.addHitRules(sk, slot)
	if err != nil {
		return nil, err
	}
	return append(srcs, src, hit), nil
}

func skillBundle(src resolve.GemSource, stats []resolve.StatValue) resolve.Bundle {
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

// skillNode returns the node of engine stat name in skill i's context for the damage chain
// damage in form.
func (sh *Sheet) skillNode(i int, name string, damage []DamageType, form damageForm) (core.Node, error) {
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
	var weapon weaponKinds
	if slices.Contains(sh.skills[i].types, typeAttack) {
		weapon = sh.mainHand
	}
	ctx := sh.session.InternContext(sh.ctx.skillMask(i, sh.skills[i].types, weapon, damage, form))
	return core.Node{Stat: id, Ctx: ctx}, nil
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
