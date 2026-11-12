package items

import (
	"fmt"
	"maps"
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/statdesc"
)

// storedValues returns rd with each value its text does not show, such as the level of
// "Curse Enemies with Despair on Hit", set to that stat's value on the mods passing ok that give
// rd (the reader can only give such a stat a placeholder). Where those mods differ on the value,
// the one most of them share is kept, the highest on a tie, with a warning.
func (r *Reader) storedValues(rd statdesc.RangeReading, ok func(*dataset.Mod) bool, inRange bool) (statdesc.RangeReading, *build.Warning) {
	var warning *build.Warning
	for k, st := range rd.Stats {
		if !shown(rd, k) {
			continue
		}
		v, set, w := r.storedValue(rd, k, r.modValues(rd, st, ok, inRange))
		if set {
			rd = withValue(rd, k, v)
		}
		if w != nil {
			warning = w
		}
	}
	return rd, warning
}

// modValues returns the values of stat st on the mods passing ok that give rd, one per mod.
func (r *Reader) modValues(rd statdesc.RangeReading, st string, ok func(*dataset.Mod) bool, inRange bool) []int32 {
	var vals []int32
	for _, ms := range r.mods.byStat[st] {
		if ok(ms.mod) && r.has(ms.mod, rd, inRange) {
			vals = append(vals, ms.max)
		}
	}
	return vals
}

// mostCommon returns the value most of vals share, the highest on a tie, and the distinct
// values in order.
func mostCommon(vals []int32) (int32, []int32) {
	count := map[int32]int{}
	for _, v := range vals {
		count[v]++
	}
	distinct := slices.Sorted(maps.Keys(count))
	best := distinct[0]
	for _, v := range distinct {
		if count[v] >= count[best] {
			best = v
		}
	}
	return best, distinct
}

// storedValue returns the value rd's k-th stat stores, given the matched mods' values of it, and
// whether it replaces the read one.
func (r *Reader) storedValue(rd statdesc.RangeReading, k int, vals []int32) (int32, bool, *build.Warning) {
	if len(vals) == 0 {
		// A stat the matched mods lack that the text does not show, such as a display stat
		// beside the one a skill enchantment sets, is left out.
		return 0, r.hidden(rd, k, 0), nil
	}
	probe := slices.IndexFunc(vals, func(v int32) bool { return v != rd.Max[k] })
	if probe < 0 || !r.hidden(rd, k, vals[probe]) {
		return 0, false, nil
	}
	v, distinct := mostCommon(vals)
	if len(distinct) == 1 {
		return v, true, nil
	}
	msg := fmt.Sprintf("the text does not show %s; the mods it can be give %v, %d kept", rd.Stats[k], distinct, v)
	return v, true, &build.Warning{Code: ValueAssumed, Message: msg}
}

// hidden reports whether rd's text is the same with its k-th stat at v: the text does not show
// that value. rd renders its own text at its high values, as the reader checked.
func (r *Reader) hidden(rd statdesc.RangeReading, k int, v int32) bool {
	at := func(x int32) map[string]int32 {
		m := make(map[string]int32, len(rd.Stats))
		for i, st := range rd.Stats {
			m[st] = rd.Max[i]
		}
		m[rd.Stats[k]] = x
		return m
	}
	a, errA := r.set.Describe(statScope, at(rd.Max[k]))
	b, errB := r.set.Describe(statScope, at(v))
	return errA == nil && errB == nil &&
		slices.EqualFunc(a, b, func(x, y statdesc.Line) bool { return x.Text == y.Text })
}

// withValue returns a copy of rd with its k-th stat at v.
func withValue(rd statdesc.RangeReading, k int, v int32) statdesc.RangeReading {
	rd.Min, rd.Max = slices.Clone(rd.Min), slices.Clone(rd.Max)
	rd.Min[k], rd.Max[k] = v, v
	return rd
}
