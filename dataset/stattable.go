package dataset

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/xaroth/pbt-engine/dataset/internal/fb"
)

// statTable maps dataset stat handles to a file's own stat indexes. A file lists the stat ids it
// uses, sorted, so its bytes do not change when the stats group gains or loses a stat.
type statTable struct {
	ids   []string
	local map[uint32]uint32
}

func newStatTable(stats []Stat, lists ...[]StatValue) statTable {
	used := map[uint32]bool{}
	for _, l := range lists {
		for _, sv := range l {
			used[sv.Stat] = true
		}
	}
	handles := slices.SortedFunc(maps.Keys(used), func(a, b uint32) int { return strings.Compare(stats[a].ID, stats[b].ID) })
	t := statTable{ids: make([]string, len(handles)), local: make(map[uint32]uint32, len(handles))}
	for i, h := range handles {
		t.ids[i] = stats[h].ID
		t.local[h] = uint32(i)
	}
	return t
}

// toLocal returns svs with file indexes. A sorted list stays sorted, now by file index.
func (t statTable) toLocal(svs []StatValue, sorted bool) []StatValue {
	out := make([]StatValue, len(svs))
	for i, sv := range svs {
		out[i] = StatValue{Stat: t.local[sv.Stat], Value: sv.Value}
	}
	if sorted {
		slices.SortFunc(out, byStat)
	}
	return out
}

func byStat(a, b StatValue) int { return cmp.Compare(a.Stat, b.Stat) }

// statIndex maps stat ids to handles. Ids must be unique and not empty.
func statIndex(stats []Stat) (map[string]uint32, error) {
	out := make(map[string]uint32, len(stats))
	for i, s := range stats {
		if s.ID == "" {
			return nil, fmt.Errorf("stat %d: empty id", i)
		}
		if _, ok := out[s.ID]; ok {
			return nil, fmt.Errorf("stat %s repeated", s.ID)
		}
		out[s.ID] = uint32(i)
	}
	return out, nil
}

// handles resolves a file's stat ids to dataset handles. The ids must be sorted and unique, as
// Write produces them, so no two file indexes map to one handle.
func handles(index map[string]uint32, n int, id func(int) []byte) ([]uint32, error) {
	out := make([]uint32, n)
	var prev string
	for i := range out {
		s := string(id(i))
		if i > 0 && s <= prev {
			return nil, fmt.Errorf("stat ids not sorted and unique at %q", s)
		}
		prev = s
		h, ok := index[s]
		if !ok {
			return nil, fmt.Errorf("unknown stat %q", s)
		}
		out[i] = h
	}
	return out, nil
}

// fromLocal returns file stat values with dataset handles. A sorted list is sorted again, now
// by handle.
func fromLocal(src statValueSource, h []uint32, sorted bool) ([]StatValue, error) {
	n := src.StatsLength()
	if n == 0 {
		return nil, nil
	}
	out := make([]StatValue, n)
	var sv fb.StatValue
	for i := range out {
		src.Stats(&sv, i)
		if int(sv.Stat()) >= len(h) {
			return nil, fmt.Errorf("stat %d out of range (%d ids)", sv.Stat(), len(h))
		}
		out[i] = StatValue{Stat: h[sv.Stat()], Value: sv.Value()}
	}
	if sorted {
		slices.SortFunc(out, byStat)
	}
	return out, nil
}
