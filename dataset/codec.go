package dataset

import (
	"fmt"
	"slices"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/xaroth/pbt-engine/dataset/internal/fb"
)

func encodeStats(stats []Stat) []byte {
	b := flatbuffers.NewBuilder(0)
	offs := make([]flatbuffers.UOffsetT, len(stats))
	for i, s := range stats {
		id := b.CreateString(s.ID)
		fb.StatStart(b)
		fb.StatAddId(b, id)
		offs[i] = fb.StatEnd(b)
	}
	vec := tableVector(b, fb.StatsStartStatsVector, offs)
	fb.StatsStart(b)
	fb.StatsAddStats(b, vec)
	fb.FinishStatsBuffer(b, fb.StatsEnd(b))
	return b.FinishedBytes()
}

func decodeStats(buf []byte) ([]Stat, error) {
	if !fb.StatsBufferHasIdentifier(buf) {
		return nil, errIdentifier
	}
	root := fb.GetRootAsStats(buf, 0)
	out := make([]Stat, root.StatsLength())
	var s fb.Stat
	for i := range out {
		root.Stats(&s, i)
		out[i] = Stat{ID: string(s.Id())}
	}
	return out, nil
}

func encodeCharacters(all []Stat, chars []Character) []byte {
	b := flatbuffers.NewBuilder(0)
	lists := make([][]StatValue, len(chars))
	for i, c := range chars {
		lists[i] = c.Stats
	}
	st := newStatTable(all, lists...)
	offs := make([]flatbuffers.UOffsetT, len(chars))
	for i, c := range chars {
		id := b.CreateString(c.ID)
		stats := statValueVector(b, fb.CharacterStartStatsVector, st.toLocal(c.Stats, true))
		fb.CharacterStart(b)
		fb.CharacterAddId(b, id)
		fb.CharacterAddBaseStrength(b, c.BaseStrength)
		fb.CharacterAddBaseDexterity(b, c.BaseDexterity)
		fb.CharacterAddBaseIntelligence(b, c.BaseIntelligence)
		fb.CharacterAddBaseMaxLife(b, c.BaseMaxLife)
		fb.CharacterAddBaseMaxMana(b, c.BaseMaxMana)
		fb.CharacterAddStats(b, stats)
		fb.CharacterAddUnarmedMinDamage(b, c.UnarmedDamage.Min)
		fb.CharacterAddUnarmedMaxDamage(b, c.UnarmedDamage.Max)
		fb.CharacterAddUnarmedAttackTime(b, c.UnarmedAttackTime)
		offs[i] = fb.CharacterEnd(b)
	}
	vec := tableVector(b, fb.CharactersStartCharactersVector, offs)
	ids := stringVector(b, fb.CharactersStartStatIdsVector, st.ids)
	fb.CharactersStart(b)
	fb.CharactersAddCharacters(b, vec)
	fb.CharactersAddStatIds(b, ids)
	fb.FinishCharactersBuffer(b, fb.CharactersEnd(b))
	return b.FinishedBytes()
}

func decodeCharacters(buf []byte, index map[string]uint32) ([]Character, error) {
	if !fb.CharactersBufferHasIdentifier(buf) {
		return nil, errIdentifier
	}
	root := fb.GetRootAsCharacters(buf, 0)
	h, err := handles(index, root.StatIdsLength(), root.StatIds)
	if err != nil {
		return nil, err
	}
	out := make([]Character, root.CharactersLength())
	var c fb.Character
	for i := range out {
		root.Characters(&c, i)
		stats, err := fromLocal(&c, h, true)
		if err != nil {
			return nil, fmt.Errorf("character %s: %w", c.Id(), err)
		}
		out[i] = Character{
			ID:                string(c.Id()),
			BaseStrength:      c.BaseStrength(),
			BaseDexterity:     c.BaseDexterity(),
			BaseIntelligence:  c.BaseIntelligence(),
			BaseMaxLife:       c.BaseMaxLife(),
			BaseMaxMana:       c.BaseMaxMana(),
			Stats:             stats,
			UnarmedDamage:     ValueRange{Min: c.UnarmedMinDamage(), Max: c.UnarmedMaxDamage()},
			UnarmedAttackTime: c.UnarmedAttackTime(),
		}
	}
	return out, nil
}

// tableVector writes offs as a vector; FlatBuffers builds vectors back to front.
func tableVector(b *flatbuffers.Builder, start func(*flatbuffers.Builder, int) flatbuffers.UOffsetT, offs []flatbuffers.UOffsetT) flatbuffers.UOffsetT {
	start(b, len(offs))
	for _, o := range slices.Backward(offs) {
		b.PrependUOffsetT(o)
	}
	return b.EndVector(len(offs))
}

// encodeGrantedSkill returns 0 for nil, so a table without one keeps the bytes it had before
// granted skills.
func encodeGrantedSkill(b *flatbuffers.Builder, g *GrantedSkill) flatbuffers.UOffsetT {
	if g == nil {
		return 0
	}
	effect := b.CreateSharedString(g.Effect)
	fb.GrantedSkillStart(b)
	fb.GrantedSkillAddEffect(b, effect)
	fb.GrantedSkillAddLevel(b, g.Level)
	return fb.GrantedSkillEnd(b)
}

func decodeGrantedSkill(g *fb.GrantedSkill) GrantedSkill {
	return GrantedSkill{Effect: string(g.Effect()), Level: g.Level()}
}

func stringVector(b *flatbuffers.Builder, start startFunc, ss []string) flatbuffers.UOffsetT {
	offs := make([]flatbuffers.UOffsetT, len(ss))
	for i, s := range ss {
		offs[i] = b.CreateString(s)
	}
	return tableVector(b, start, offs)
}
