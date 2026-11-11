package dataset

import (
	"maps"
	"slices"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/xaroth/pbt-engine/dataset/internal/fb"
)

func (d *Dataset) monstersGroup() []group {
	if d.Monsters == nil {
		return nil
	}
	return []group{{monstersFile, encodeMonsters(d.Stats, d.Monsters)}}
}

// monsterStats returns the stat handles m uses.
func monsterStats(m *Monsters) map[uint32]bool {
	used := map[uint32]bool{}
	addStats(used, m.BaseStats)
	for _, mod := range m.Mods {
		for _, s := range mod.Stats {
			used[s.Stat] = true
		}
	}
	for _, t := range m.MinionTypes {
		addHandles(used, setHandles(t.LimitStat, t.ActiveCountStat))
	}
	return used
}

// monsterTags returns the tags m's varieties and mods use.
func monsterTags(m *Monsters) map[string]bool {
	used := map[string]bool{}
	for _, v := range m.Varieties {
		addStrings(used, v.Tags)
	}
	for _, mod := range m.Mods {
		addStrings(used, mod.Tags)
		addStrings(used, mod.AddedTags)
		for _, w := range slices.Concat(mod.SpawnWeights, mod.GenerationWeights) {
			used[w.Tag] = true
		}
	}
	return used
}

// encodeMonsters writes m; Write validates it first.
func encodeMonsters(all []Stat, m *Monsters) []byte {
	st := statTableOf(all, monsterStats(m))
	tagIDs := slices.Sorted(maps.Keys(monsterTags(m)))
	tagIndex := indexOf(tagIDs, func(t string) string { return t })
	b := flatbuffers.NewBuilder(0)
	types := encodeList(b, fb.MonstersStartTypesVector, m.Types, func(t MonsterType) flatbuffers.UOffsetT { return encodeMonsterType(b, t) })
	resists := encodeList(b, fb.MonstersStartResistancesVector, m.Resistances,
		func(r MonsterResistance) flatbuffers.UOffsetT { return encodeMonsterResistance(b, r) })
	varieties := encodeList(b, fb.MonstersStartVarietiesVector, m.Varieties,
		func(v MonsterVariety) flatbuffers.UOffsetT { return encodeVariety(b, tagIndex, v) })
	levels := encodeList(b, fb.MonstersStartLevelsVector, m.Levels, func(l MonsterLevel) flatbuffers.UOffsetT { return encodeMonsterLevel(b, l) })
	base := statValueVector(b, fb.MonstersStartBaseStatsVector, st.toLocal(m.BaseStats, true))
	mods := encodeList(b, fb.MonstersStartModsVector, m.Mods, func(mod Mod) flatbuffers.UOffsetT { return encodeMod(b, st, tagIndex, mod) })
	displays := encodeList(b, fb.MonstersStartDisplayMinionsVector, m.DisplayMinions,
		func(d DisplayMinion) flatbuffers.UOffsetT { return encodeDisplayMinion(b, d) })
	totems := encodeList(b, fb.MonstersStartTotemsVector, m.Totems, func(t Totem) flatbuffers.UOffsetT { return encodeTotem(b, t) })
	overrides := encodeList(b, fb.MonstersStartSpectreOverridesVector, m.SpectreOverrides,
		func(s SpectreOverride) flatbuffers.UOffsetT { return encodeSpectreOverride(b, s) })
	minions := encodeList(b, fb.MonstersStartMinionTypesVector, m.MinionTypes, func(t MinionType) flatbuffers.UOffsetT { return encodeMinionType(b, st, t) })
	edition := encodeList(b, fb.MonstersStartEditionTotemsVector, m.EditionTotems, func(e EditionTotem) flatbuffers.UOffsetT { return encodeEditionTotem(b, e) })
	statIDs := stringVector(b, fb.MonstersStartStatIdsVector, st.ids)
	tags := stringVector(b, fb.MonstersStartTagIdsVector, tagIDs)
	fb.MonstersStart(b)
	fb.MonstersAddTypes(b, types)
	fb.MonstersAddResistances(b, resists)
	fb.MonstersAddVarieties(b, varieties)
	fb.MonstersAddLevels(b, levels)
	fb.MonstersAddBaseStats(b, base)
	fb.MonstersAddMods(b, mods)
	fb.MonstersAddDisplayMinions(b, displays)
	fb.MonstersAddTotems(b, totems)
	fb.MonstersAddSpectreOverrides(b, overrides)
	fb.MonstersAddMinionTypes(b, minions)
	fb.MonstersAddEditionTotems(b, edition)
	fb.MonstersAddStatIds(b, statIDs)
	fb.MonstersAddTagIds(b, tags)
	fb.FinishMonstersBuffer(b, fb.MonstersEnd(b))
	return b.FinishedBytes()
}

func encodeMonsterType(b *flatbuffers.Builder, t MonsterType) flatbuffers.UOffsetT {
	id, resists := b.CreateString(t.ID), optString(b, t.Resistances)
	fb.MonsterTypeStart(b)
	fb.MonsterTypeAddId(b, id)
	fb.MonsterTypeAddArmour(b, t.Armour)
	fb.MonsterTypeAddEvasion(b, t.Evasion)
	fb.MonsterTypeAddEnergyShield(b, t.EnergyShield)
	fb.MonsterTypeAddAccuracy(b, t.Accuracy)
	fb.MonsterTypeAddDamageSpread(b, t.DamageSpread)
	fb.MonsterTypeAddResistances(b, resists)
	fb.MonsterTypeAddPlayerMinion(b, t.PlayerMinion)
	fb.MonsterTypeAddBaseDamageIgnoresAttackSpeed(b, t.BaseDamageIgnoresAttackSpeed)
	fb.MonsterTypeAddAltLife1(b, t.AltLife1)
	fb.MonsterTypeAddAltLife2(b, t.AltLife2)
	return fb.MonsterTypeEnd(b)
}

func encodeMonsterResistance(b *flatbuffers.Builder, r MonsterResistance) flatbuffers.UOffsetT {
	id := b.CreateString(r.ID)
	fb.MonsterResistanceStart(b)
	fb.MonsterResistanceAddId(b, id)
	for _, x := range []struct {
		add func(*flatbuffers.Builder, flatbuffers.UOffsetT)
		r   Resistances
	}{{fb.MonsterResistanceAddNormal, r.Normal}, {fb.MonsterResistanceAddCruel, r.Cruel}, {fb.MonsterResistanceAddMerciless, r.Merciless}} {
		x.add(b, fb.CreateResistances(b, x.r.Fire, x.r.Cold, x.r.Lightning, x.r.Chaos))
	}
	return fb.MonsterResistanceEnd(b)
}

func sharedStrings(b *flatbuffers.Builder, start startFunc, ss []string) flatbuffers.UOffsetT {
	if len(ss) == 0 {
		return 0
	}
	return encodeList(b, start, ss, b.CreateSharedString)
}

func encodeVariety(b *flatbuffers.Builder, tagIndex map[string]uint32, v MonsterVariety) flatbuffers.UOffsetT {
	id, typ := b.CreateString(v.ID), b.CreateSharedString(v.Type)
	mainHand, offHand := optString(b, v.MainHand), optString(b, v.OffHand)
	var tags flatbuffers.UOffsetT
	if len(v.Tags) > 0 {
		tags = uint32Vector(b, fb.MonsterVarietyStartTagsVector, indexes(tagIndex, v.Tags))
	}
	effects := sharedStrings(b, fb.MonsterVarietyStartGrantedEffectsVector, v.GrantedEffects)
	mods := sharedStrings(b, fb.MonsterVarietyStartModsVector, v.Mods)
	mods2 := sharedStrings(b, fb.MonsterVarietyStartMods2Vector, v.Mods2)
	special := sharedStrings(b, fb.MonsterVarietyStartSpecialModsVector, v.SpecialMods)
	part1 := sharedStrings(b, fb.MonsterVarietyStartPart1ModsVector, v.Part1Mods)
	part2 := sharedStrings(b, fb.MonsterVarietyStartPart2ModsVector, v.Part2Mods)
	endgame := sharedStrings(b, fb.MonsterVarietyStartEndgameModsVector, v.EndgameMods)
	fb.MonsterVarietyStart(b)
	fb.MonsterVarietyAddId(b, id)
	fb.MonsterVarietyAddType(b, typ)
	fb.MonsterVarietyAddLifeMultiplier(b, v.LifeMultiplier)
	fb.MonsterVarietyAddDamageMultiplier(b, v.DamageMultiplier)
	fb.MonsterVarietyAddAilmentThreshold(b, v.AilmentThreshold)
	fb.MonsterVarietyAddAttackTime(b, v.AttackTime)
	fb.MonsterVarietyAddMainHand(b, mainHand)
	fb.MonsterVarietyAddOffHand(b, offHand)
	fb.MonsterVarietyAddTags(b, tags)
	fb.MonsterVarietyAddGrantedEffects(b, effects)
	fb.MonsterVarietyAddMods(b, mods)
	fb.MonsterVarietyAddMods2(b, mods2)
	fb.MonsterVarietyAddSpecialMods(b, special)
	fb.MonsterVarietyAddPart1Mods(b, part1)
	fb.MonsterVarietyAddPart2Mods(b, part2)
	fb.MonsterVarietyAddEndgameMods(b, endgame)
	return fb.MonsterVarietyEnd(b)
}

func encodeMonsterLevel(b *flatbuffers.Builder, l MonsterLevel) flatbuffers.UOffsetT {
	fb.MonsterLevelStart(b)
	fb.MonsterLevelAddLevel(b, l.Level)
	fb.MonsterLevelAddDifficulty(b, l.Difficulty)
	fb.MonsterLevelAddLife(b, l.Life)
	fb.MonsterLevelAddAllyLife(b, l.AllyLife)
	fb.MonsterLevelAddAltLife1(b, l.AltLife1)
	fb.MonsterLevelAddAltLife2(b, l.AltLife2)
	fb.MonsterLevelAddDamage(b, l.Damage)
	fb.MonsterLevelAddAllyDamage(b, l.AllyDamage)
	fb.MonsterLevelAddAltDamage1(b, l.AltDamage1)
	fb.MonsterLevelAddAltDamage2(b, l.AltDamage2)
	fb.MonsterLevelAddArmour(b, l.Armour)
	fb.MonsterLevelAddEvasion(b, l.Evasion)
	fb.MonsterLevelAddEvasiveEvasion(b, l.EvasiveEvasion)
	fb.MonsterLevelAddAccuracy(b, l.Accuracy)
	fb.MonsterLevelAddAilmentThreshold(b, l.AilmentThreshold)
	fb.MonsterLevelAddPhysicalConversion(b, l.PhysicalConversion)
	return fb.MonsterLevelEnd(b)
}

func encodeTotem(b *flatbuffers.Builder, t Totem) flatbuffers.UOffsetT {
	skins := encodeList(b, fb.TotemStartSkinsVector, t.Skins, func(s TotemSkin) flatbuffers.UOffsetT {
		v := b.CreateSharedString(s.Variety)
		fb.TotemSkinStart(b)
		fb.TotemSkinAddSkin(b, s.Skin)
		fb.TotemSkinAddVariety(b, v)
		return fb.TotemSkinEnd(b)
	})
	fb.TotemStart(b)
	fb.TotemAddKey(b, t.Key)
	fb.TotemAddSkins(b, skins)
	return fb.TotemEnd(b)
}

func encodeDisplayMinion(b *flatbuffers.Builder, d DisplayMinion) flatbuffers.UOffsetT {
	v := b.CreateSharedString(d.Variety)
	fb.DisplayMinionStart(b)
	fb.DisplayMinionAddId(b, d.ID)
	fb.DisplayMinionAddVariety(b, v)
	return fb.DisplayMinionEnd(b)
}

func encodeSpectreOverride(b *flatbuffers.Builder, s SpectreOverride) flatbuffers.UOffsetT {
	monster, spectre := b.CreateSharedString(s.Monster), b.CreateSharedString(s.Spectre)
	fb.SpectreOverrideStart(b)
	fb.SpectreOverrideAddMonster(b, monster)
	fb.SpectreOverrideAddSpectre(b, spectre)
	return fb.SpectreOverrideEnd(b)
}

func encodeMinionType(b *flatbuffers.Builder, st statTable, t MinionType) flatbuffers.UOffsetT {
	id := b.CreateString(t.ID)
	fb.MinionTypeStart(b)
	fb.MinionTypeAddId(b, id)
	fb.MinionTypeAddLimitStat(b, st.optLocal(t.LimitStat))
	fb.MinionTypeAddActiveCountStat(b, st.optLocal(t.ActiveCountStat))
	return fb.MinionTypeEnd(b)
}

func encodeEditionTotem(b *flatbuffers.Builder, e EditionTotem) flatbuffers.UOffsetT {
	ge, source := b.CreateSharedString(e.GrantedEffect), b.CreateString(e.Source)
	fb.EditionTotemStart(b)
	fb.EditionTotemAddGrantedEffect(b, ge)
	fb.EditionTotemAddTotem(b, e.Totem)
	fb.EditionTotemAddSource(b, source)
	return fb.EditionTotemEnd(b)
}
