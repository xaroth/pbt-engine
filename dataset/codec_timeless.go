package dataset

import (
	"fmt"
	"slices"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/xaroth/pbt-engine/dataset/internal/fb"
)

func timelessStatLists(vs []TimelessVersion) [][]StatValue {
	var lists [][]StatValue
	add := func(rs []StatRange) {
		for _, r := range rs {
			lists = append(lists, []StatValue{{Stat: r.Stat}})
		}
	}
	for _, v := range vs {
		for _, r := range v.Replacements {
			add(r.Stats)
		}
		for _, a := range v.Additions {
			add(a.Stats)
		}
	}
	return lists
}

func encodeTimelessKinds(b *flatbuffers.Builder, start startFunc, kinds []TimelessKind) flatbuffers.UOffsetT {
	start(b, len(kinds))
	for _, k := range slices.Backward(kinds) {
		b.PrependByte(byte(k))
	}
	return b.EndVector(len(kinds))
}

func encodeStatRanges(b *flatbuffers.Builder, st statTable, start startFunc, rs []StatRange) flatbuffers.UOffsetT {
	start(b, len(rs))
	for _, r := range slices.Backward(rs) {
		fb.CreateStatRange(b, st.local[r.Stat], r.Min, r.Max)
	}
	return b.EndVector(len(rs))
}

func encodeTimeless(b *flatbuffers.Builder, st statTable, vs []TimelessVersion) flatbuffers.UOffsetT {
	offs := make([]flatbuffers.UOffsetT, len(vs))
	for i, v := range vs {
		repOffs := make([]flatbuffers.UOffsetT, len(v.Replacements))
		for j, r := range v.Replacements {
			repOffs[j] = encodeTimelessReplacement(b, st, r)
		}
		addOffs := make([]flatbuffers.UOffsetT, len(v.Additions))
		for j, a := range v.Additions {
			addOffs[j] = encodeTimelessAddition(b, st, a)
		}
		reps := tableVector(b, fb.TimelessVersionStartReplacementsVector, repOffs)
		adds := tableVector(b, fb.TimelessVersionStartAdditionsVector, addOffs)
		id := b.CreateString(v.ID)
		fb.TimelessVersionStart(b)
		fb.TimelessVersionAddId(b, id)
		fb.TimelessVersionAddSmallAttributeReplaced(b, v.SmallAttributeReplaced)
		fb.TimelessVersionAddSmallNormalReplaced(b, v.SmallNormalReplaced)
		fb.TimelessVersionAddSmallAttributeAdditionsMin(b, v.SmallAttributeAdditions.Min)
		fb.TimelessVersionAddSmallAttributeAdditionsMax(b, v.SmallAttributeAdditions.Max)
		fb.TimelessVersionAddSmallNormalAdditionsMin(b, v.SmallNormalAdditions.Min)
		fb.TimelessVersionAddSmallNormalAdditionsMax(b, v.SmallNormalAdditions.Max)
		fb.TimelessVersionAddNotableAdditionsMin(b, v.NotableAdditions.Min)
		fb.TimelessVersionAddNotableAdditionsMax(b, v.NotableAdditions.Max)
		fb.TimelessVersionAddNotableReplacementChance(b, v.NotableReplacementChance)
		fb.TimelessVersionAddReplacements(b, reps)
		fb.TimelessVersionAddAdditions(b, adds)
		offs[i] = fb.TimelessVersionEnd(b)
	}
	return tableVector(b, fb.TreeStartTimelessVersionsVector, offs)
}

func encodeTimelessReplacement(b *flatbuffers.Builder, st statTable, r TimelessReplacement) flatbuffers.UOffsetT {
	id := b.CreateString(r.ID)
	kinds := encodeTimelessKinds(b, fb.TimelessReplacementStartKindsVector, r.Kinds)
	stats := encodeStatRanges(b, st, fb.TimelessReplacementStartStatsVector, r.Stats)
	fb.TimelessReplacementStart(b)
	fb.TimelessReplacementAddId(b, id)
	fb.TimelessReplacementAddKinds(b, kinds)
	fb.TimelessReplacementAddStats(b, stats)
	fb.TimelessReplacementAddWeight(b, r.Weight)
	fb.TimelessReplacementAddAdditionsMin(b, r.Additions.Min)
	fb.TimelessReplacementAddAdditionsMax(b, r.Additions.Max)
	fb.TimelessReplacementAddConquerorIndex(b, r.Conqueror.Index)
	fb.TimelessReplacementAddConquerorVersion(b, r.Conqueror.Version)
	return fb.TimelessReplacementEnd(b)
}

func encodeTimelessAddition(b *flatbuffers.Builder, st statTable, a TimelessAddition) flatbuffers.UOffsetT {
	id := b.CreateString(a.ID)
	kinds := encodeTimelessKinds(b, fb.TimelessAdditionStartKindsVector, a.Kinds)
	stats := encodeStatRanges(b, st, fb.TimelessAdditionStartStatsVector, a.Stats)
	fb.TimelessAdditionStart(b)
	fb.TimelessAdditionAddId(b, id)
	fb.TimelessAdditionAddKinds(b, kinds)
	fb.TimelessAdditionAddStats(b, stats)
	fb.TimelessAdditionAddWeight(b, a.Weight)
	return fb.TimelessAdditionEnd(b)
}

// timelessRow is what reading a replacement or addition row needs of it.
type timelessRow interface {
	Id() []byte
	Kinds(j int) fb.TimelessKind
	KindsLength() int
	Stats(obj *fb.StatRange, j int) bool
	StatsLength() int
}

func decodeTimelessRow(r timelessRow, h []uint32) ([]TimelessKind, []StatRange, error) {
	var kinds []TimelessKind
	if n := r.KindsLength(); n > 0 {
		kinds = make([]TimelessKind, n)
	}
	for j := range kinds {
		kinds[j] = TimelessKind(r.Kinds(j))
	}
	var stats []StatRange
	var s fb.StatRange
	for j := range r.StatsLength() {
		r.Stats(&s, j)
		if int(s.Stat()) >= len(h) {
			return nil, nil, fmt.Errorf("timeless row %s: stat %d out of range (%d ids): %w", r.Id(), s.Stat(), len(h), ErrInvalid)
		}
		stats = append(stats, StatRange{Stat: h[s.Stat()], Min: s.Min(), Max: s.Max()})
	}
	return kinds, stats, nil
}

func decodeTimeless(root *fb.Tree, h []uint32) ([]TimelessVersion, error) {
	var out []TimelessVersion
	var v fb.TimelessVersion
	var r fb.TimelessReplacement
	var a fb.TimelessAddition
	for i := range root.TimelessVersionsLength() {
		root.TimelessVersions(&v, i)
		tv := TimelessVersion{
			ID:                       string(v.Id()),
			SmallAttributeReplaced:   v.SmallAttributeReplaced(),
			SmallNormalReplaced:      v.SmallNormalReplaced(),
			SmallAttributeAdditions:  ValueRange{Min: v.SmallAttributeAdditionsMin(), Max: v.SmallAttributeAdditionsMax()},
			SmallNormalAdditions:     ValueRange{Min: v.SmallNormalAdditionsMin(), Max: v.SmallNormalAdditionsMax()},
			NotableAdditions:         ValueRange{Min: v.NotableAdditionsMin(), Max: v.NotableAdditionsMax()},
			NotableReplacementChance: v.NotableReplacementChance(),
		}
		for j := range v.ReplacementsLength() {
			v.Replacements(&r, j)
			kinds, stats, err := decodeTimelessRow(&r, h)
			if err != nil {
				return nil, err
			}
			tv.Replacements = append(tv.Replacements, TimelessReplacement{
				ID: string(r.Id()), Kinds: kinds, Stats: stats, Weight: r.Weight(),
				Additions: ValueRange{Min: r.AdditionsMin(), Max: r.AdditionsMax()},
				Conqueror: TimelessConqueror{Index: r.ConquerorIndex(), Version: r.ConquerorVersion()},
			})
		}
		for j := range v.AdditionsLength() {
			v.Additions(&a, j)
			kinds, stats, err := decodeTimelessRow(&a, h)
			if err != nil {
				return nil, err
			}
			tv.Additions = append(tv.Additions, TimelessAddition{ID: string(a.Id()), Kinds: kinds, Stats: stats, Weight: a.Weight()})
		}
		out = append(out, tv)
	}
	return out, nil
}
