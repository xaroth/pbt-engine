package poe1

import (
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
	"github.com/xaroth/pbt-engine/internal/semantics"
)

// Engine stats of the curses and marks the character applies to the enemy. curse_effect, read
// in a curse skill's context, is the effect of its curse in percent: 100 with its increases
// and more multipliers. curse_limit is how many curses the character can apply to the enemy,
// marks included. curses_left, read in a curse skill's context, is what of the limit the
// curses before it leave; marks_before is how many marks come before it, of which an enemy
// can have one. enemy_is_cursed and enemy_is_marked are set while a curse or a mark applies.
const (
	statCurseEffect   = "curse_effect"
	statCurseLimit    = "curse_limit"
	statCursesLeft    = "curses_left"
	statMarksBefore   = "marks_before"
	statEnemyIsCursed = "enemy_is_cursed"
	statEnemyIsMarked = "enemy_is_marked"
)

// Skill types of curses: a hex or a mark.
const (
	typeHex  = "Hex"
	typeMark = "Mark"
)

// Curse rule values. Source: the game's text ("You can apply an additional Curse"), and PoB
// f9f4f3b CalcSetup.lua (EnemyCurseLimit base 1).
const (
	baseCurseEffect = 100
	baseCurseLimit  = 1
)

func curseMechanic() mechanic {
	stats := []string{statCurseEffect, statCurseLimit, statCursesLeft, statMarksBefore, statEnemyIsCursed, statEnemyIsMarked}
	return mechanic{name: "curses", stats: stats, def: func(def core.StatDef, ids map[string]core.StatID) core.StatDef {
		switch def.Name {
		case statCursesLeft:
			def.Includes = []core.StatID{mustID(ids, statCurseLimit)}
		case statEnemyIsCursed, statEnemyIsMarked:
			def.Flag = true
		}
		return def
	}}
}

// IsCurse reports whether sk applies a curse to the enemy: a hex or a mark.
func IsCurse(sk *resolve.Skill) bool {
	return slices.ContainsFunc(sk.Types, func(t string) bool { return t == typeHex || t == typeMark })
}

func isMark(sk *resolve.Skill) bool { return slices.Contains(sk.Types, typeMark) }

// splitCurse returns the stats of a curse skill's stat set that are its own and those its curse
// carries to the enemy, not nil: the stats with an entry in the curse scope of the table.
func (sh *Sheet) splitCurse(stats []resolve.StatValue) (own, curse []resolve.StatValue) {
	curse = []resolve.StatValue{}
	for _, sv := range stats {
		if _, ok := sh.table.Lookup(sv.Stat, resolve.Curse, sh.gameVersion); ok {
			curse = append(curse, sv)
		} else {
			own = append(own, sv)
		}
	}
	return own, curse
}

// addCurse adds the curse sk in slot, with context ctx, applies to the enemy, from the stats it
// carries: those stats, scaled by the curse's effect, and the enemy's cursed and marked
// conditions hold while it applies. A curse applies while the curses in earlier slots leave
// some of the limit, and a mark while no earlier one is a mark; it holds its place against
// the curses in later slots either way, but for a mark that an earlier one displaces. It
// returns the sources of both.
func (sh *Sheet) addCurse(sk *resolve.Skill, stats []resolve.StatValue, slot int, ctx core.CtxID) ([]core.SourceID, error) {
	at := func(stat string) core.Ref { return core.Ref{Actor: 0, Stat: sh.stats[stat], Ctx: int64(ctx)} }
	mark := isMark(sk)
	holds := []core.Cond{{Ref: at(statCursesLeft), Cmp: core.CmpGT}}
	if mark {
		holds = append(holds, core.Cond{Ref: at(statMarksBefore), Cmp: core.CmpEQ})
	}
	bd := resolve.Bundle{Actor: resolve.Player, Scope: resolve.Curse, Source: sk.Source, Stats: stats}
	curse, err := sh.addBundle(&bd, sh.table, sh.gameVersion, -1, &carried{effect: at(statCurseEffect), holds: holds})
	if err != nil {
		return nil, err
	}
	src := sh.newSource(Source{RuleID: "curse", Rule: "curse of " + sk.Source.String()})
	own, err := predicate(sh.session.Model(), semantics.ContextFilter{}, sh.ctx, slot)
	if err != nil {
		return nil, err
	}
	var slots core.Mask
	for s := slot + 1; s < maxSkills; s++ {
		slots = slots.Set(sh.ctx.skill.Bit(s))
	}
	later := core.Predicate{Req: []core.Mask{slots}}
	// A mark after the first takes no place: the enemy has one mark at a time.
	var takes []core.Cond
	if mark {
		takes = holds[1:]
	}
	mods := []core.Modifier{
		{Stat: sh.stats[statCurseEffect], Op: core.OpBase, Value: baseCurseEffect, Source: src, Pred: own},
		{Stat: sh.stats[statCursesLeft], Op: core.OpBase, Value: -1, Source: src, Pred: later, Conds: takes},
		{Stat: sh.stats[statEnemyIsCursed], Op: core.OpFlag, Value: 1, Source: src, Conds: holds},
	}
	if mark {
		mods = append(mods,
			core.Modifier{Stat: sh.stats[statMarksBefore], Op: core.OpBase, Value: 1, Source: src, Pred: later},
			core.Modifier{Stat: sh.stats[statEnemyIsMarked], Op: core.OpFlag, Value: 1, Source: src, Conds: holds},
		)
	}
	for _, m := range mods {
		if _, err := sh.session.AddModifier(m); err != nil {
			return nil, fmt.Errorf("sheet: curse of %s: %w", sk.Source, err)
		}
	}
	return []core.SourceID{curse, src}, nil
}

// addCurseLimit adds the curse limit before modifiers, baseCurseLimit.
func (sh *Sheet) addCurseLimit() error {
	src := sh.newSource(Source{RuleID: "curse-limit", Rule: fmt.Sprintf("curse limit (%d)", baseCurseLimit)})
	if _, err := sh.session.AddModifier(core.Modifier{Stat: sh.stats[statCurseLimit], Op: core.OpBase, Value: baseCurseLimit, Source: src}); err != nil {
		return fmt.Errorf("sheet: curse limit: %w", err)
	}
	return nil
}

// cursePrecedence orders the curses of a build that apply when it has more than the limit:
// those of a higher precedence apply first. In game the most recent curse replaces the oldest,
// which depends on play; the precedence follows PoB f9f4f3b (Data.lua cursePriority,
// CalcPerform.lua determineCursePriority): the item slot of the curse's group, then the gem's
// place in it, then the curse. groups are the build's skill groups.
func cursePrecedence(sk *resolve.Skill, groups []build.SkillGroup) int {
	const (
		slotStep   = 1000
		socketStep = 100
		maxSocket  = 8
	)
	src := sk.Source
	if src.Group < 0 || src.Group >= len(groups) {
		return curseNamePrecedence()[sk.GrantedEffect]
	}
	g := groups[src.Group].Slot
	if w, ok := map[build.Slot]build.Slot{build.SlotWeaponSwap: build.SlotWeapon, build.SlotOffhandSwap: build.SlotOffhand}[g]; ok {
		g = w
	}
	slot := slices.Index(curseSlotOrder(), g) + 1
	return slot*slotStep + min(src.Gem+1, maxSocket)*socketStep + curseNamePrecedence()[sk.GrantedEffect]
}

// curseSlotOrder are the item slots by precedence of the curses socketed in them, lowest
// first; a weapon swap slot counts as its weapon slot.
func curseSlotOrder() []build.Slot {
	return []build.Slot{
		build.SlotWeapon, build.SlotAmulet, build.SlotHelmet, build.SlotOffhand, build.SlotBodyArmour,
		build.SlotGloves, build.SlotBoots, build.SlotRing1, build.SlotRing2, build.SlotRing3,
	}
}

// curseNamePrecedence is the precedence of each curse by granted effect, lowest first.
func curseNamePrecedence() map[string]int {
	names := []string{
		"TemporalChains", "Enfeeble", "Vulnerability", "ElementalWeakness", "Flammability", "Frostbite", "Conductivity",
		"Despair", "Punishment", "WarlordsMark", "AssassinsMark", "SnipersMark", "PoachersMark",
	}
	out := make(map[string]int, len(names))
	for i, n := range names {
		out[n] = i + 1
	}
	return out
}

// curseList collects the curses of a build in build order: every hex and, since the enemy has
// one mark at a time, the first mark, as PoB f9f4f3b does (CalcPerform.lua, markSlotted).
type curseList struct {
	skills []resolve.SkillWarnings
	marked bool
}

// take adds sw and reports true when it is a curse the list keeps.
func (l *curseList) take(sw resolve.SkillWarnings) bool {
	if !IsCurse(sw.Skill) || isMark(sw.Skill) && l.marked {
		return false
	}
	l.marked = l.marked || isMark(sw.Skill)
	l.skills = append(l.skills, sw)
	return true
}

// ordered returns the curses by precedence (cursePrecedence with groups), highest first.
func (l *curseList) ordered(groups []build.SkillGroup) []resolve.SkillWarnings {
	slices.SortStableFunc(l.skills, func(a, b resolve.SkillWarnings) int {
		return cursePrecedence(b.Skill, groups) - cursePrecedence(a.Skill, groups)
	})
	return l.skills
}
