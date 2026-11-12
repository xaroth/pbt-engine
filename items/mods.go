package items

import (
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/statdesc"
)

// modStat is a mod with the range of one of its stats.
type modStat struct {
	mod      *dataset.Mod
	min, max int32
}

// indexMods indexes mods by the ids of their stats, each mod once per stat it has.
func indexMods(mods []dataset.Mod, stats []dataset.Stat) map[string][]modStat {
	out := map[string][]modStat{}
	for i := range mods {
		for _, st := range mods[i].Stats {
			id := stats[st.Stat].ID
			out[id] = append(out[id], modStat{mod: &mods[i], min: min(st.Min, st.Max), max: max(st.Min, st.Max)})
		}
	}
	return out
}

// shown reports whether reading rd shows its k-th stat: a stat the text does not show reads as 0.
func shown(rd statdesc.RangeReading, k int) bool {
	return rd.Min[k] != 0 || rd.Max[k] != 0
}

// gives reports whether some mod that passes ok has every stat rd shows, with rd's values in the
// mod's ranges when inRange is set.
func (r *Reader) gives(rd statdesc.RangeReading, inRange bool, ok func(*dataset.Mod) bool) bool {
	k := -1
	for i := range rd.Stats {
		if shown(rd, i) {
			k = i
			break
		}
	}
	if k < 0 {
		return false
	}
	for _, ms := range r.mods[rd.Stats[k]] {
		if ok(ms.mod) && r.has(ms.mod, rd, inRange) {
			return true
		}
	}
	return false
}

// has reports whether m has every stat rd shows, with rd's values in m's ranges when inRange is
// set.
func (r *Reader) has(m *dataset.Mod, rd statdesc.RangeReading, inRange bool) bool {
	for k, st := range rd.Stats {
		if !shown(rd, k) {
			continue
		}
		ms, ok := r.rangeOf(m, st)
		if !ok || inRange && (rd.Min[k] < ms.min || rd.Max[k] > ms.max) {
			return false
		}
	}
	return true
}

// rangeOf returns the range of stat st on mod m.
func (r *Reader) rangeOf(m *dataset.Mod, st string) (modStat, bool) {
	i := slices.IndexFunc(r.mods[st], func(ms modStat) bool { return ms.mod == m })
	if i < 0 {
		return modStat{}, false
	}
	return r.mods[st][i], true
}
