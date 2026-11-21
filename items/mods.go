package items

import (
	"maps"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/statdesc"
)

// gameModeRuthless is Mods.GameMode of a mod that exists in Ruthless only.
const gameModeRuthless = 2

// modStat is a mod with the range of one of its stats.
type modStat struct {
	mod      *dataset.Mod
	min, max int32
}

// modIndex holds mods by the ids of their stats, each mod's stat ids, and by local stat the mods
// with it that roll on some item: a base's implicit or a mod that spawns.
type modIndex struct {
	byStat map[string][]modStat
	stats  map[*dataset.Mod][]string
	rolls  map[string][]*dataset.Mod
}

// indexMods indexes mods by the ids of their stats. Mods of Ruthless only are left out: items
// are read for the standard game.
func indexMods(mods []dataset.Mod, stats []dataset.Stat, bases []dataset.ItemBase) modIndex {
	implicit := map[string]bool{}
	for i := range bases {
		for _, id := range bases[i].Implicits {
			implicit[id] = true
		}
	}
	x := modIndex{byStat: map[string][]modStat{}, stats: map[*dataset.Mod][]string{}, rolls: map[string][]*dataset.Mod{}}
	for i := range mods {
		if mods[i].GameMode == gameModeRuthless {
			continue
		}
		for _, st := range mods[i].Stats {
			id := stats[st.Stat].ID
			x.byStat[id] = append(x.byStat[id], modStat{mod: &mods[i], min: min(st.Min, st.Max), max: max(st.Min, st.Max)})
			x.stats[&mods[i]] = append(x.stats[&mods[i]], id)
			if isLocal(id) && (implicit[mods[i].ID] || spawns(&mods[i])) {
				x.rolls[id] = append(x.rolls[id], &mods[i])
			}
		}
	}
	return x
}

// shown reports whether reading rd shows its k-th stat: a stat the text does not show reads as 0.
func shown(rd statdesc.RangeReading, k int) bool {
	return rd.Min[k] != 0 || rd.Max[k] != 0
}

// firstShown returns the index of the first stat rd shows, or -1.
func firstShown(rd statdesc.RangeReading) int {
	for k := range rd.Stats {
		if shown(rd, k) {
			return k
		}
	}
	return -1
}

// gives reports whether some mod that passes ok has every stat rd shows (see has), with rd's
// values in the mod's ranges scaled by scale when inRange is set.
func (r *Reader) gives(rd statdesc.RangeReading, inRange bool, ok func(*dataset.Mod) bool, scale scaler) bool {
	for k, st := range rd.Stats {
		if !shown(rd, k) {
			continue
		}
		for _, ms := range r.mods.byStat[st] {
			if ok(ms.mod) && r.has(ms.mod, rd, inRange, scale) {
				return true
			}
		}
	}
	return false
}

// givesSum reports whether two mods that pass ok, of no shared family, add up to rd's values,
// as they are or each scaled by scale: the game shows mods of one stat, such as a prefix and a suffix, as one
// line.
func (r *Reader) givesSum(rd statdesc.RangeReading, ok func(*dataset.Mod) bool, scale scaler) bool {
	k := firstShown(rd)
	if k < 0 {
		return false
	}
	var mods []*dataset.Mod
	for _, ms := range r.mods.byStat[rd.Stats[k]] {
		if ok(ms.mod) && r.has(ms.mod, rd, false, nil) && !slices.Contains(mods, ms.mod) {
			mods = append(mods, ms.mod)
		}
	}
	for i, a := range mods {
		for _, b := range mods[i+1:] {
			if !slices.ContainsFunc(a.Families, func(f string) bool { return slices.Contains(b.Families, f) }) && r.sums(a, b, rd, scale) {
				return true
			}
		}
	}
	return false
}

// sums reports whether a and b, as they are or scaled by scale, add up to rd's values for each
// stat rd shows.
func (r *Reader) sums(a, b *dataset.Mod, rd statdesc.RangeReading, scale scaler) bool {
	return r.sumsScaled(a, b, rd, 1, 1) || r.sumsScaled(a, b, rd, scale.of(a), scale.of(b))
}

// sumsScaled reports whether a scaled by fa and b by fb add up to rd's values for each stat rd
// shows.
func (r *Reader) sumsScaled(a, b *dataset.Mod, rd statdesc.RangeReading, fa, fb float64) bool {
	for k, st := range rd.Stats {
		if !shown(rd, k) {
			continue
		}
		x, okA := r.rangeOf(a, st)
		y, okB := r.rangeOf(b, st)
		x, y = x.scaled(fa), y.scaled(fb)
		if !okA || !okB || rd.Min[k] < x.min+y.min || rd.Max[k] > x.max+y.max {
			return false
		}
	}
	return true
}

// has reports whether m has every stat rd shows, with rd's values in m's ranges when inRange is
// set, as they are or scaled by scale; a stat whose value the text does not show need not be
// m's, and a passive's name is in any range. The game does not scale every stat, and the data
// does not say which it scales, so an unscaled value fits too.
func (r *Reader) has(m *dataset.Mod, rd statdesc.RangeReading, inRange bool, scale scaler) bool {
	ranges := make(map[string]modStat, len(rd.Stats))
	for k, st := range rd.Stats {
		if !shown(rd, k) {
			continue
		}
		ms, ok := r.rangeOf(m, st)
		if !ok {
			// The reader gives a stat the text does not show a placeholder; m need not have it.
			if r.hidden(rd, k, 0) {
				continue
			}
			return false
		}
		ranges[st] = ms
	}
	f := scale.of(m)
	return !inRange || r.inRanges(rd, ranges, 1) || f != 1 && r.inRanges(rd, ranges, f)
}

// inRanges reports whether rd's values are in ranges, by stat, scaled by f. A value out of range
// still fits when the mod at its own values renders the same text, as for a number the
// description writes as fixed words, such as "30%", or a scaled value the text rounds, such as
// "2.2%" for 2.15.
func (r *Reader) inRanges(rd statdesc.RangeReading, ranges map[string]modStat, f float64) bool {
	own := make(map[string]int32, len(ranges))
	out := false
	for k, st := range rd.Stats {
		ms, ok := ranges[st]
		if !ok || !shown(rd, k) {
			continue
		}
		ms = ms.scaled(f)
		own[st] = ms.min
		out = out || !r.passiveStats[st] && (rd.Min[k] < ms.min || rd.Max[k] > ms.max)
	}
	return !out || r.sameText(rd, own)
}

// sameText reports whether rd's stats render the same text at their low values and with the
// values in own put in.
func (r *Reader) sameText(rd statdesc.RangeReading, own map[string]int32) bool {
	read := make(map[string]int32, len(rd.Stats))
	for k, st := range rd.Stats {
		read[st] = rd.Min[k]
	}
	put := maps.Clone(read)
	maps.Copy(put, own)
	a, errA := r.set.Describe(statScope, read)
	b, errB := r.set.Describe(statScope, put)
	return errA == nil && errB == nil && len(a) > 0 &&
		slices.EqualFunc(a, b, func(x, y statdesc.Line) bool { return x.Text == y.Text })
}

// rangeOf returns the range of stat st on mod m.
func (r *Reader) rangeOf(m *dataset.Mod, st string) (modStat, bool) {
	i := slices.IndexFunc(r.mods.byStat[st], func(ms modStat) bool { return ms.mod == m })
	if i < 0 {
		return modStat{}, false
	}
	return r.mods.byStat[st][i], true
}
