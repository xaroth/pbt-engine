package dataset

import (
	"math"
	"slices"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/xaroth/pbt-engine/dataset/internal/fb"
)

// noStat is the file's value for an unset single stat, the schema's default.
const noStat = math.MaxUint32

func (d *Dataset) buffsGroup() []group {
	if d.Buffs == nil {
		return nil
	}
	return []group{{BuffsFile, encodeBuffs(d.Stats, d.Buffs)}}
}

// buffStats returns the stat handles b uses.
func buffStats(b *Buffs) map[uint32]bool {
	used := map[uint32]bool{}
	for _, d := range b.Definitions {
		for _, hs := range [][]uint32{d.Stats, d.ExtraStats, d.Flags, d.ConditionStats} {
			addHandles(used, hs)
		}
		for _, h := range []*uint32{d.MaximumStat, d.CurrentStat, d.MinimumStat} {
			if h != nil {
				used[*h] = true
			}
		}
	}
	return used
}

func encodeBuffs(all []Stat, bs *Buffs) []byte {
	st := statTableOf(all, buffStats(bs))
	b := flatbuffers.NewBuilder(0)
	defs := encodeList(b, fb.BuffsStartDefinitionsVector, bs.Definitions, func(d BuffDefinition) flatbuffers.UOffsetT {
		return encodeBuffDefinition(b, st, d)
	})
	flasks := encodeList(b, fb.BuffsStartFlasksVector, bs.Flasks, func(f FlaskBuff) flatbuffers.UOffsetT {
		item, buff := b.CreateString(f.Item), b.CreateSharedString(f.Buff)
		var values flatbuffers.UOffsetT
		if len(f.Values) > 0 {
			fb.FlaskBuffStartValuesVector(b, len(f.Values))
			for _, v := range slices.Backward(f.Values) {
				b.PrependInt32(v)
			}
			values = b.EndVector(len(f.Values))
		}
		fb.FlaskBuffStart(b)
		fb.FlaskBuffAddItem(b, item)
		fb.FlaskBuffAddBuff(b, buff)
		fb.FlaskBuffAddValues(b, values)
		return fb.FlaskBuffEnd(b)
	})
	tinctures := encodeList(b, fb.BuffsStartTincturesVector, bs.Tinctures, func(t Tincture) flatbuffers.UOffsetT {
		item := b.CreateString(t.Item)
		fb.TinctureStart(b)
		fb.TinctureAddItem(b, item)
		fb.TinctureAddDebuffInterval(b, t.DebuffInterval)
		fb.TinctureAddCooldown(b, t.Cooldown)
		return fb.TinctureEnd(b)
	})
	ids := stringVector(b, fb.BuffsStartStatIdsVector, st.ids)
	fb.BuffsStart(b)
	fb.BuffsAddDefinitions(b, defs)
	fb.BuffsAddFlasks(b, flasks)
	fb.BuffsAddTinctures(b, tinctures)
	fb.BuffsAddStatIds(b, ids)
	fb.FinishBuffsBuffer(b, fb.BuffsEnd(b))
	return b.FinishedBytes()
}

func encodeBuffDefinition(b *flatbuffers.Builder, st statTable, d BuffDefinition) flatbuffers.UOffsetT {
	handles := func(start startFunc, hs []uint32) flatbuffers.UOffsetT {
		if len(hs) == 0 {
			return 0
		}
		return uint32Vector(b, start, st.handlesToLocal(hs))
	}
	stats := handles(fb.BuffDefinitionStartStatsVector, d.Stats)
	extra := handles(fb.BuffDefinitionStartExtraStatsVector, d.ExtraStats)
	flags := handles(fb.BuffDefinitionStartFlagsVector, d.Flags)
	conditions := handles(fb.BuffDefinitionStartConditionStatsVector, d.ConditionStats)
	id, id2 := b.CreateString(d.ID), optString(b, d.ID2)
	fb.BuffDefinitionStart(b)
	fb.BuffDefinitionAddId(b, id)
	fb.BuffDefinitionAddCategory(b, d.Category)
	fb.BuffDefinitionAddId2(b, id2)
	fb.BuffDefinitionAddStats(b, stats)
	fb.BuffDefinitionAddExtraStats(b, extra)
	fb.BuffDefinitionAddFlags(b, flags)
	fb.BuffDefinitionAddConditionStats(b, conditions)
	fb.BuffDefinitionAddMaximumStat(b, st.optLocal(d.MaximumStat))
	fb.BuffDefinitionAddCurrentStat(b, st.optLocal(d.CurrentStat))
	fb.BuffDefinitionAddMinimumStat(b, st.optLocal(d.MinimumStat))
	fb.BuffDefinitionAddLimit(b, d.Limit)
	fb.BuffDefinitionAddSkillBuff(b, d.SkillBuff)
	fb.BuffDefinitionAddInvisible(b, d.Invisible)
	fb.BuffDefinitionAddRemovable(b, d.Removable)
	fb.BuffDefinitionAddCancelOnDeath(b, d.CancelOnDeath)
	fb.BuffDefinitionAddRecovery(b, d.Recovery)
	return fb.BuffDefinitionEnd(b)
}

func decodeBuffs(buf []byte, index map[string]uint32) (Buffs, error) {
	if !fb.BuffsBufferHasIdentifier(buf) {
		return Buffs{}, errIdentifier
	}
	root := fb.GetRootAsBuffs(buf, 0)
	h, err := handles(index, root.StatIdsLength(), root.StatIds)
	if err != nil {
		return Buffs{}, err
	}
	d := &skillDecoder{h: h}
	out := Buffs{
		Definitions: make([]BuffDefinition, root.DefinitionsLength()),
		Flasks:      make([]FlaskBuff, root.FlasksLength()),
		Tinctures:   make([]Tincture, root.TincturesLength()),
	}
	var def fb.BuffDefinition
	for i := range out.Definitions {
		root.Definitions(&def, i)
		out.Definitions[i] = BuffDefinition{
			ID: string(def.Id()), Category: def.Category(), ID2: string(def.Id2()),
			Stats:          d.handles(def.StatsLength(), def.Stats),
			ExtraStats:     d.handles(def.ExtraStatsLength(), def.ExtraStats),
			Flags:          d.handles(def.FlagsLength(), def.Flags),
			ConditionStats: d.handles(def.ConditionStatsLength(), def.ConditionStats),
			MaximumStat:    d.optStat(def.MaximumStat()), CurrentStat: d.optStat(def.CurrentStat()), MinimumStat: d.optStat(def.MinimumStat()),
			Limit: def.Limit(), SkillBuff: def.SkillBuff(), Invisible: def.Invisible(), Removable: def.Removable(),
			CancelOnDeath: def.CancelOnDeath(), Recovery: def.Recovery(),
		}
	}
	var f fb.FlaskBuff
	for i := range out.Flasks {
		root.Flasks(&f, i)
		fl := FlaskBuff{Item: string(f.Item()), Buff: string(f.Buff())}
		if n := f.ValuesLength(); n > 0 {
			fl.Values = make([]int32, n)
			for j := range fl.Values {
				fl.Values[j] = f.Values(j)
			}
		}
		out.Flasks[i] = fl
	}
	var t fb.Tincture
	for i := range out.Tinctures {
		root.Tinctures(&t, i)
		out.Tinctures[i] = Tincture{Item: string(t.Item()), DebuffInterval: t.DebuffInterval(), Cooldown: t.Cooldown()}
	}
	if d.err != nil {
		return Buffs{}, d.err
	}
	return out, nil
}
