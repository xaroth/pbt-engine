package poe1

import (
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/config"
	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// Engine stats of Arcane Surge. has_arcane_surge is set while you have it, a condition
// (conditions). arcane_surge_effect is its effect on you in percent, 100 with its increases;
// arcane_surge_cast_speed and arcane_surge_mana_regeneration are the increased cast speed and
// mana regeneration rate it grants before its effect. hits_with_spells is set while you hit
// with a spell of the build.
const (
	statHasArcaneSurge              = "has_arcane_surge"
	statArcaneSurgeEffect           = "arcane_surge_effect"
	statArcaneSurgeCastSpeed        = "arcane_surge_cast_speed"
	statArcaneSurgeManaRegeneration = "arcane_surge_mana_regeneration"
	statHitsWithSpells              = "hits_with_spells"
)

// ArcaneSurge is whether you have Arcane Surge, off by default. A support or passive that grants
// it sets it on too.
const ArcaneSurge config.Key[bool] = "arcane-surge"

// What Arcane Surge grants where nothing gives its values: 20% increased cast speed and 30%
// increased mana regeneration rate. Source: PoB b32759a CalcPerform.lua, the defaults of
// ArcaneSurgeCastSpeed and ArcaneSurgeManaRegen; the Arcane Surge support's constant stats give
// the same.
const (
	arcaneSurgeCastSpeed        = 20
	arcaneSurgeManaRegeneration = 30
)

// supportCharacterStats are the support stats that act on the character rather than on the
// supported skill: those of the Arcane Surge it grants you.
func supportCharacterStats() []string {
	return []string{
		"support_arcane_surge_gain_buff_on_mana_use_threshold",
		"support_arcane_surge_cast_speed_+%",
		"support_arcane_surge_mana_regeneration_rate_+%",
	}
}

func arcaneSurgeMechanic() mechanic {
	return mechanic{
		name:  "arcane surge",
		stats: []string{statArcaneSurgeEffect, statArcaneSurgeCastSpeed, statArcaneSurgeManaRegeneration, statHitsWithSpells},
		def: func(def core.StatDef, _ map[string]core.StatID) core.StatDef {
			if def.Name == statHitsWithSpells {
				def.Flag = true
			}
			return def
		},
	}
}

// addArcaneSurge adds the base values of Arcane Surge and what it grants while you have it:
// increased cast speed and mana regeneration rate, scaled by its effect without rounding, as
// PoB b32759a does (CalcPerform.lua). It adds the source that says a skill hits with spells,
// off.
func (sh *Sheet) addArcaneSurge() error {
	src := sh.newSource(Source{RuleID: "arcane-surge", Rule: "Arcane Surge"})
	effect := core.Scale{Ref: core.SelfRef(sh.stats[statArcaneSurgeEffect]), Step: baseBuffEffect, Ratio: true}
	holds := []core.Cond{{Ref: core.SelfRef(sh.stats[statHasArcaneSurge]), Cmp: core.CmpNE}}
	granted := [][2]string{{statCastSpeed, statArcaneSurgeCastSpeed}, {statManaRegenerationRate, statArcaneSurgeManaRegeneration}}
	bases := []core.Modifier{
		{Stat: sh.stats[statArcaneSurgeEffect], Op: core.OpBase, Value: baseBuffEffect, Source: src},
		{Stat: sh.stats[statArcaneSurgeCastSpeed], Op: core.OpBase, Value: arcaneSurgeCastSpeed, Source: src},
		{Stat: sh.stats[statArcaneSurgeManaRegeneration], Op: core.OpBase, Value: arcaneSurgeManaRegeneration, Source: src},
	}
	mods := make([]core.Modifier, 0, len(bases)+len(granted))
	mods = append(mods, bases...)
	for _, g := range granted {
		mods = append(mods, core.Modifier{
			Stat: sh.stats[g[0]], Op: core.OpInc, Value: 1, Source: src, Conds: holds,
			Scales: []core.Scale{{Ref: core.SelfRef(sh.stats[g[1]]), Step: 1}, effect},
		})
	}
	for _, m := range mods {
		if _, err := sh.session.AddModifier(m); err != nil {
			return fmt.Errorf("sheet: arcane surge: %w", err)
		}
	}
	sh.spellHits = sh.newSource(Source{RuleID: "hits-with-spells", Rule: "a skill of the build hits with spells"})
	if _, err := sh.session.AddModifier(core.Modifier{Stat: sh.stats[statHitsWithSpells], Op: core.OpFlag, Value: 1, Source: sh.spellHits}); err != nil {
		return fmt.Errorf("sheet: hits with spells: %w", err)
	}
	// The source is this session's, so this cannot fail.
	_ = sh.session.SetSourceEnabled(sh.spellHits, false)
	return nil
}

// setSpellHits switches hits_with_spells on when a skill on the sheet, but the default attack,
// is a spell that hits and that you use, not a totem, trap or mine. A totem's hits do not count,
// as in PoB b32759a, whose CalcPerform.lua sets TotemsSpellHitRecently where the passive reads
// TotemsHitSpellRecently, though the passive's text names them.
func (sh *Sheet) setSpellHits() {
	on := slices.ContainsFunc(sh.skills, func(sl skillSlot) bool {
		return !sl.source.Default() && SpellHit(sl.skill) && !hasAnyType(sl.skill, typeSummonsTotem, typeTrapped, typeRemoteMined)
	})
	_ = sh.session.SetSourceEnabled(sh.spellHits, on)
}

// splitCharacterStats returns the stats of a support that are its supported skill's and those
// that act on the character (supportCharacterStats).
func splitCharacterStats(stats []resolve.StatValue) (skill, character []resolve.StatValue) {
	for _, sv := range stats {
		if slices.Contains(supportCharacterStats(), sv.Stat) {
			character = append(character, sv)
		} else {
			skill = append(skill, sv)
		}
	}
	return skill, character
}
