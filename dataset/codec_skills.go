package dataset

import (
	"fmt"
	"maps"
	"math"
	"slices"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/xaroth/pbt-engine/dataset/internal/fb"
)

// stringTable numbers the distinct strings of one kind, sorted, so the file does not depend on
// which row uses a string first.
type stringTable struct {
	ids   []string
	index map[string]uint16
}

// newStringTable numbers used; validateSkillTables keeps it within uint16 indexes.
func newStringTable(used map[string]bool) stringTable {
	t := stringTable{ids: slices.Sorted(maps.Keys(used)), index: make(map[string]uint16, len(used))}
	for i, id := range t.ids {
		t.index[id] = uint16(i)
	}
	return t
}

func (t stringTable) refs(ss []string) []uint16 {
	out := make([]uint16, len(ss))
	for i, s := range ss {
		out[i] = t.index[s]
	}
	return out
}

// skillTables are the file-wide tables rows refer to by index.
type skillTables struct {
	stats                     statTable
	classes, tags, costs, bas stringTable
}

func newSkillTables(all []Stat, s *Skills) skillTables {
	classes, tags, costs, bases := skillStrings(s)
	return skillTables{
		stats:   statTableOf(all, skillStats(s)),
		classes: newStringTable(classes), tags: newStringTable(tags), costs: newStringTable(costs), bas: newStringTable(bases),
	}
}

// maxTableSize is the most entries a table referred to by uint16 indexes can hold.
const maxTableSize = math.MaxUint16 + 1

// validateSkillTables checks that the skill types and the string tables of s fit uint16
// indexes.
func validateSkillTables(s *Skills) error {
	classes, tags, costs, bases := skillStrings(s)
	for _, x := range []struct {
		what string
		n    int
	}{{"skill types", len(s.Types)}, {"item classes", len(classes)}, {"gem tags", len(tags)}, {"cost types", len(costs)}, {"interpolation bases", len(bases)}} {
		if x.n > maxTableSize {
			return fmt.Errorf("%d %s, the file holds at most %d: %w", x.n, x.what, maxTableSize, ErrInvalid)
		}
	}
	return nil
}

// skillStats returns the stat handles s uses.
func skillStats(s *Skills) map[uint32]bool {
	used := map[uint32]bool{}
	for _, g := range s.GrantedEffects {
		addStats(used, g.QualityStats)
	}
	for _, ss := range s.StatSets {
		addHandles(used, ss.ImplicitStats)
		addStats(used, ss.ConstantStats)
		for _, l := range ss.Levels {
			for _, f := range l.FloatStats {
				used[f.Stat] = true
			}
			for _, st := range l.Stats {
				used[st.Stat] = true
			}
			addHandles(used, l.Flags)
		}
	}
	return used
}

// skillStrings returns the item classes, gem tags, cost types and interpolation bases s uses.
func skillStrings(s *Skills) (classes, tags, costs, bases map[string]bool) {
	classes, tags, costs, bases = map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, g := range s.Gems {
		for _, e := range g.Effects {
			addStrings(tags, e.Tags)
		}
	}
	for _, g := range s.GrantedEffects {
		addStrings(classes, g.SupportWeaponRestrictions)
		for _, l := range g.Levels {
			for _, c := range l.Costs {
				costs[c.Type] = true
			}
		}
	}
	for _, ss := range s.StatSets {
		for _, l := range ss.Levels {
			for _, f := range l.FloatStats {
				bases[f.Base] = true
			}
		}
	}
	for _, a := range s.ActiveSkills {
		addStrings(classes, a.WeaponRestrictions)
	}
	delete(classes, "")
	delete(tags, "")
	delete(costs, "")
	delete(bases, "")
	return classes, tags, costs, bases
}

func addStats(used map[uint32]bool, svs []StatValue) {
	for _, sv := range svs {
		used[sv.Stat] = true
	}
}

func addHandles(used map[uint32]bool, hs []uint32) {
	for _, h := range hs {
		used[h] = true
	}
}

func addStrings(used map[string]bool, ss []string) {
	for _, s := range ss {
		used[s] = true
	}
}

func (d *Dataset) skillsGroup() []group {
	if d.Skills == nil {
		return nil
	}
	return []group{{skillsFile, encodeSkills(d.Stats, d.Skills)}}
}

// encodeSkills writes s; Write validates it first, so its tables fit their indexes.
func encodeSkills(all []Stat, s *Skills) []byte {
	t := newSkillTables(all, s)
	b := flatbuffers.NewBuilder(0)
	gems := encodeList(b, fb.SkillsStartGemsVector, s.Gems, func(g Gem) flatbuffers.UOffsetT { return encodeGem(b, t, g) })
	effects := encodeList(b, fb.SkillsStartGrantedEffectsVector, s.GrantedEffects,
		func(g GrantedEffect) flatbuffers.UOffsetT { return encodeGrantedEffect(b, t, g) })
	sets := encodeList(b, fb.SkillsStartStatSetsVector, s.StatSets, func(ss StatSet) flatbuffers.UOffsetT { return encodeStatSet(b, t, ss) })
	actives := encodeList(b, fb.SkillsStartActiveSkillsVector, s.ActiveSkills, func(a ActiveSkill) flatbuffers.UOffsetT { return encodeActiveSkill(b, t, a) })
	types := stringVector(b, fb.SkillsStartSkillTypesVector, s.Types)
	classes := stringVector(b, fb.SkillsStartItemClassesVector, t.classes.ids)
	tags := stringVector(b, fb.SkillsStartGemTagsVector, t.tags.ids)
	costs := stringVector(b, fb.SkillsStartCostTypesVector, t.costs.ids)
	bases := stringVector(b, fb.SkillsStartInterpolationBasesVector, t.bas.ids)
	ids := stringVector(b, fb.SkillsStartStatIdsVector, t.stats.ids)
	fb.SkillsStart(b)
	fb.SkillsAddSkillTypes(b, types)
	fb.SkillsAddGems(b, gems)
	fb.SkillsAddGrantedEffects(b, effects)
	fb.SkillsAddStatSets(b, sets)
	fb.SkillsAddActiveSkills(b, actives)
	fb.SkillsAddItemClasses(b, classes)
	fb.SkillsAddGemTags(b, tags)
	fb.SkillsAddCostTypes(b, costs)
	fb.SkillsAddInterpolationBases(b, bases)
	fb.SkillsAddStatIds(b, ids)
	fb.FinishSkillsBuffer(b, fb.SkillsEnd(b))
	return b.FinishedBytes()
}

func encodeList[T any](b *flatbuffers.Builder, start startFunc, list []T, encode func(T) flatbuffers.UOffsetT) flatbuffers.UOffsetT {
	offs := make([]flatbuffers.UOffsetT, len(list))
	for i, v := range list {
		offs[i] = encode(v)
	}
	return tableVector(b, start, offs)
}

func uint16Vector(b *flatbuffers.Builder, start startFunc, vs []uint16) flatbuffers.UOffsetT {
	if len(vs) == 0 {
		return 0
	}
	start(b, len(vs))
	for _, v := range slices.Backward(vs) {
		b.PrependUint16(v)
	}
	return b.EndVector(len(vs))
}

// optString writes s, or nothing for an empty optional field.
func optString(b *flatbuffers.Builder, s string) flatbuffers.UOffsetT {
	if s == "" {
		return 0
	}
	return b.CreateSharedString(s)
}

func encodeGem(b *flatbuffers.Builder, t skillTables, g Gem) flatbuffers.UOffsetT {
	effects := encodeList(b, fb.GemStartEffectsVector, g.Effects, func(e GemEffect) flatbuffers.UOffsetT {
		id, ge, ge2 := b.CreateSharedString(e.ID), b.CreateSharedString(e.GrantedEffect), optString(b, e.SecondaryGrantedEffect)
		hm, hm2 := optString(b, e.HardmodeGrantedEffect), optString(b, e.HardmodeSecondaryGrantedEffect)
		tags := uint16Vector(b, fb.GemEffectStartTagsVector, t.tags.refs(e.Tags))
		fb.GemEffectStart(b)
		fb.GemEffectAddId(b, id)
		fb.GemEffectAddGrantedEffect(b, ge)
		fb.GemEffectAddSecondaryGrantedEffect(b, ge2)
		fb.GemEffectAddHardmodeGrantedEffect(b, hm)
		fb.GemEffectAddHardmodeSecondaryGrantedEffect(b, hm2)
		fb.GemEffectAddTags(b, tags)
		fb.GemEffectAddStyle(b, byte(e.Style))
		return fb.GemEffectEnd(b)
	})
	id, vaal, regular, awakened := b.CreateString(g.ID), optString(b, g.VaalGem), optString(b, g.RegularGem), optString(b, g.AwakenedGem)
	fb.GemStart(b)
	fb.GemAddId(b, id)
	fb.GemAddColour(b, byte(g.Colour))
	fb.GemAddStrengthPercent(b, g.StrengthPercent)
	fb.GemAddDexterityPercent(b, g.DexterityPercent)
	fb.GemAddIntelligencePercent(b, g.IntelligencePercent)
	fb.GemAddSupport(b, g.Support)
	fb.GemAddVaal(b, g.Vaal)
	fb.GemAddVaalGem(b, vaal)
	fb.GemAddRegularGem(b, regular)
	fb.GemAddAwakenedGem(b, awakened)
	fb.GemAddEffects(b, effects)
	return fb.GemEnd(b)
}

func encodeGrantedEffect(b *flatbuffers.Builder, t skillTables, g GrantedEffect) flatbuffers.UOffsetT {
	levels := encodeList(b, fb.GrantedEffectStartLevelsVector, g.Levels, func(l EffectLevel) flatbuffers.UOffsetT { return encodeEffectLevel(b, t, l) })
	id, active := b.CreateSharedString(g.ID), optString(b, g.ActiveSkill)
	sets := encodeList(b, fb.GrantedEffectStartStatSetsVector, g.StatSets, b.CreateSharedString)
	var quality flatbuffers.UOffsetT
	if len(g.QualityStats) > 0 {
		quality = statValueVector(b, fb.GrantedEffectStartQualityStatsVector, t.stats.toLocal(g.QualityStats, false))
	}
	allowed := uint16Vector(b, fb.GrantedEffectStartAllowedTypesVector, g.AllowedTypes)
	excluded := uint16Vector(b, fb.GrantedEffectStartExcludedTypesVector, g.ExcludedTypes)
	added := uint16Vector(b, fb.GrantedEffectStartAddedTypesVector, g.AddedTypes)
	addedMinion := uint16Vector(b, fb.GrantedEffectStartAddedMinionTypesVector, g.AddedMinionTypes)
	weapons := uint16Vector(b, fb.GrantedEffectStartSupportWeaponRestrictionsVector, t.classes.refs(g.SupportWeaponRestrictions))
	fb.GrantedEffectStart(b)
	fb.GrantedEffectAddId(b, id)
	fb.GrantedEffectAddSupport(b, g.Support)
	fb.GrantedEffectAddActiveSkill(b, active)
	fb.GrantedEffectAddCastTime(b, g.CastTime)
	fb.GrantedEffectAddStatSets(b, sets)
	fb.GrantedEffectAddQualityStats(b, quality)
	fb.GrantedEffectAddAllowedTypes(b, allowed)
	fb.GrantedEffectAddExcludedTypes(b, excluded)
	fb.GrantedEffectAddAddedTypes(b, added)
	fb.GrantedEffectAddAddedMinionTypes(b, addedMinion)
	fb.GrantedEffectAddSupportWeaponRestrictions(b, weapons)
	fb.GrantedEffectAddSupportsGemsOnly(b, g.SupportsGemsOnly)
	fb.GrantedEffectAddCannotBeSupported(b, g.CannotBeSupported)
	fb.GrantedEffectAddLevels(b, levels)
	return fb.GrantedEffectEnd(b)
}

func encodeEffectLevel(b *flatbuffers.Builder, t skillTables, l EffectLevel) flatbuffers.UOffsetT {
	var costs flatbuffers.UOffsetT
	if len(l.Costs) > 0 {
		fb.EffectLevelStartCostsVector(b, len(l.Costs))
		for _, c := range slices.Backward(l.Costs) {
			fb.CreateCost(b, t.costs.index[c.Type], c.Amount)
		}
		costs = b.EndVector(len(l.Costs))
	}
	fb.EffectLevelStart(b)
	fb.EffectLevelAddLevel(b, l.Level)
	fb.EffectLevelAddPlayerLevel(b, l.PlayerLevel)
	fb.EffectLevelAddCosts(b, costs)
	fb.EffectLevelAddCostMultiplier(b, l.CostMultiplier)
	fb.EffectLevelAddCooldown(b, l.Cooldown)
	fb.EffectLevelAddCooldownGroup(b, l.CooldownGroup)
	fb.EffectLevelAddStoredUses(b, l.StoredUses)
	fb.EffectLevelAddVaalSouls(b, l.VaalSouls)
	fb.EffectLevelAddVaalStoredUses(b, l.VaalStoredUses)
	fb.EffectLevelAddSoulGainPrevention(b, l.SoulGainPrevention)
	fb.EffectLevelAddAttackSpeedMultiplier(b, l.AttackSpeedMultiplier)
	fb.EffectLevelAddAttackTime(b, l.AttackTime)
	fb.EffectLevelAddManaReservationFlat(b, l.ManaReservationFlat)
	fb.EffectLevelAddManaReservationPercent(b, l.ManaReservationPercent)
	fb.EffectLevelAddLifeReservationFlat(b, l.LifeReservationFlat)
	fb.EffectLevelAddLifeReservationPercent(b, l.LifeReservationPercent)
	return fb.EffectLevelEnd(b)
}

func encodeStatSet(b *flatbuffers.Builder, t skillTables, s StatSet) flatbuffers.UOffsetT {
	levels := encodeList(b, fb.StatSetStartLevelsVector, s.Levels, func(l StatSetLevel) flatbuffers.UOffsetT { return encodeStatSetLevel(b, t, l) })
	id := b.CreateSharedString(s.ID)
	var implicit, constant flatbuffers.UOffsetT
	if len(s.ImplicitStats) > 0 {
		implicit = uint32Vector(b, fb.StatSetStartImplicitStatsVector, t.stats.handlesToLocal(s.ImplicitStats))
	}
	if len(s.ConstantStats) > 0 {
		constant = statValueVector(b, fb.StatSetStartConstantStatsVector, t.stats.toLocal(s.ConstantStats, false))
	}
	fb.StatSetStart(b)
	fb.StatSetAddId(b, id)
	fb.StatSetAddImplicitStats(b, implicit)
	fb.StatSetAddConstantStats(b, constant)
	fb.StatSetAddBaseEffectiveness(b, s.BaseEffectiveness)
	fb.StatSetAddIncrementalEffectiveness(b, s.IncrementalEffectiveness)
	fb.StatSetAddLevels(b, levels)
	return fb.StatSetEnd(b)
}

func encodeStatSetLevel(b *flatbuffers.Builder, t skillTables, l StatSetLevel) flatbuffers.UOffsetT {
	var floats, stats, flags flatbuffers.UOffsetT
	if len(l.FloatStats) > 0 {
		fb.StatSetLevelStartFloatStatsVector(b, len(l.FloatStats))
		for _, f := range slices.Backward(l.FloatStats) {
			fb.CreateFloatStat(b, t.stats.local[f.Stat], f.Value, f.Resolved, byte(f.Interpolation), t.bas.index[f.Base])
		}
		floats = b.EndVector(len(l.FloatStats))
	}
	if len(l.Stats) > 0 {
		fb.StatSetLevelStartStatsVector(b, len(l.Stats))
		for _, s := range slices.Backward(l.Stats) {
			fb.CreateLevelStat(b, t.stats.local[s.Stat], s.Value, byte(s.Interpolation))
		}
		stats = b.EndVector(len(l.Stats))
	}
	if len(l.Flags) > 0 {
		flags = uint32Vector(b, fb.StatSetLevelStartFlagsVector, t.stats.handlesToLocal(l.Flags))
	}
	fb.StatSetLevelStart(b)
	fb.StatSetLevelAddLevel(b, l.Level)
	fb.StatSetLevelAddPlayerLevel(b, l.PlayerLevel)
	fb.StatSetLevelAddSpellCritChance(b, l.SpellCritChance)
	fb.StatSetLevelAddAttackCritChance(b, l.AttackCritChance)
	fb.StatSetLevelAddDamageEffectiveness(b, l.DamageEffectiveness)
	fb.StatSetLevelAddBaseMultiplier(b, l.BaseMultiplier)
	fb.StatSetLevelAddFloatStats(b, floats)
	fb.StatSetLevelAddStats(b, stats)
	fb.StatSetLevelAddFlags(b, flags)
	return fb.StatSetLevelEnd(b)
}

func encodeActiveSkill(b *flatbuffers.Builder, t skillTables, a ActiveSkill) flatbuffers.UOffsetT {
	id, base := b.CreateSharedString(a.ID), optString(b, a.TransfigureBase)
	types := uint16Vector(b, fb.ActiveSkillStartTypesVector, a.Types)
	minion := uint16Vector(b, fb.ActiveSkillStartMinionTypesVector, a.MinionTypes)
	weapons := uint16Vector(b, fb.ActiveSkillStartWeaponRestrictionsVector, t.classes.refs(a.WeaponRestrictions))
	fb.ActiveSkillStart(b)
	fb.ActiveSkillAddId(b, id)
	fb.ActiveSkillAddTypes(b, types)
	fb.ActiveSkillAddMinionTypes(b, minion)
	fb.ActiveSkillAddWeaponRestrictions(b, weapons)
	fb.ActiveSkillAddTotem(b, a.Totem)
	fb.ActiveSkillAddMine(b, a.Mine)
	fb.ActiveSkillAddTransfigureBase(b, base)
	return fb.ActiveSkillEnd(b)
}
