package poe1

import (
	"errors"
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// ErrSkill is a skill the sheet does not hold, or more skills than it can hold.
var ErrSkill = errors.New("skill not on the sheet")

// skillKey is a skill's sources by slot and content.
type skillKey struct {
	slot int
	key  string
}

// SetSkills makes skills the sheet's skills: skill i takes context slot i, and the sources of
// its own stats (its primary stat set, quality included, and each applied support) go on,
// those of other skills off. A skill's own stats reach that skill's values only. A skill not
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
		sh.skills = append(sh.skills, slices.Clone(sk.Types))
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
	srcs := make([]core.SourceID, 0, len(bundles))
	for i := range bundles {
		src, err := sh.add(&bundles[i], sh.table, sh.gameVersion, slot)
		if err != nil {
			return nil, err
		}
		srcs = append(srcs, src)
	}
	return srcs, nil
}

func skillBundle(src resolve.GemSource, stats []resolve.StatValue) resolve.Bundle {
	return resolve.Bundle{Actor: resolve.Player, Scope: resolve.Global, Source: src, Stats: stats}
}

// SkillValue returns engine stat name in the context of skill i of SetSkills, reading the part
// of its damage of type d, or no part for NoDamage.
func (sh *Sheet) SkillValue(i int, name string, d DamageType) (float64, error) {
	n, err := sh.skillNode(i, name, d)
	if err != nil {
		return 0, err
	}
	val, err := sh.session.Get(n)
	if err != nil {
		return 0, fmt.Errorf("sheet: skill %d %s: %w", i, name, err)
	}
	return val, nil
}

// skillNode returns the node of engine stat name in skill i's context for damage type d.
func (sh *Sheet) skillNode(i int, name string, d DamageType) (core.Node, error) {
	id, ok := sh.stats[name]
	switch {
	case !ok:
		return core.Node{}, fmt.Errorf("%w %q", ErrUnknownStat, name)
	case i < 0 || i >= len(sh.skills):
		return core.Node{}, fmt.Errorf("%w: %d of %d", ErrSkill, i, len(sh.skills))
	case d > Chaos:
		return core.Node{}, fmt.Errorf("sheet: damage type %d", d)
	}
	ctx := sh.session.InternContext(sh.ctx.skillMask(i, sh.skills[i], d))
	return core.Node{Stat: id, Ctx: ctx}, nil
}

// baseDamage is damage before modifiers: the base damage, 100%.
const baseDamage = 100

// addBaseDamage adds the base damage as a rule's source, so increases and more multipliers
// apply to it by the default rule.
func (sh *Sheet) addBaseDamage() error {
	src := sh.newSource(Source{RuleID: "base-damage", Rule: "base damage (100%)"})
	mod := core.Modifier{Stat: sh.stats[statDamage], Op: core.OpBase, Value: baseDamage, Source: src}
	if _, err := sh.session.AddModifier(mod); err != nil {
		return fmt.Errorf("sheet: base damage: %w", err)
	}
	return nil
}
