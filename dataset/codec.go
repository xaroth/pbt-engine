package dataset

import (
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

func encodeCharacters(chars []Character) []byte {
	b := flatbuffers.NewBuilder(0)
	offs := make([]flatbuffers.UOffsetT, len(chars))
	for i, c := range chars {
		id := b.CreateString(c.ID)
		name := b.CreateString(c.Name)
		fb.CharacterStartStatsVector(b, len(c.Stats))
		for _, sv := range slices.Backward(c.Stats) {
			fb.CreateStatValue(b, sv.Stat, sv.Value)
		}
		stats := b.EndVector(len(c.Stats))
		fb.CharacterStart(b)
		fb.CharacterAddId(b, id)
		fb.CharacterAddName(b, name)
		fb.CharacterAddBaseStrength(b, c.BaseStrength)
		fb.CharacterAddBaseDexterity(b, c.BaseDexterity)
		fb.CharacterAddBaseIntelligence(b, c.BaseIntelligence)
		fb.CharacterAddBaseMaxLife(b, c.BaseMaxLife)
		fb.CharacterAddBaseMaxMana(b, c.BaseMaxMana)
		fb.CharacterAddStats(b, stats)
		offs[i] = fb.CharacterEnd(b)
	}
	vec := tableVector(b, fb.CharactersStartCharactersVector, offs)
	fb.CharactersStart(b)
	fb.CharactersAddCharacters(b, vec)
	fb.FinishCharactersBuffer(b, fb.CharactersEnd(b))
	return b.FinishedBytes()
}

func decodeCharacters(buf []byte) ([]Character, error) {
	if !fb.CharactersBufferHasIdentifier(buf) {
		return nil, errIdentifier
	}
	root := fb.GetRootAsCharacters(buf, 0)
	out := make([]Character, root.CharactersLength())
	var c fb.Character
	var sv fb.StatValue
	for i := range out {
		root.Characters(&c, i)
		stats := make([]StatValue, c.StatsLength())
		for j := range stats {
			c.Stats(&sv, j)
			stats[j] = StatValue{Stat: sv.Stat(), Value: sv.Value()}
		}
		out[i] = Character{
			ID:               string(c.Id()),
			Name:             string(c.Name()),
			BaseStrength:     c.BaseStrength(),
			BaseDexterity:    c.BaseDexterity(),
			BaseIntelligence: c.BaseIntelligence(),
			BaseMaxLife:      c.BaseMaxLife(),
			BaseMaxMana:      c.BaseMaxMana(),
			Stats:            stats,
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
