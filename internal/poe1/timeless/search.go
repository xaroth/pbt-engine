package timeless

import (
	"cmp"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
)

// Want is a result a search looks for, totalled over the passives searched: the number of
// passives replaced by any of Replacements, or else the sum of Stat's values the jewel gives.
type Want struct {
	// Replacements are row indexes in the version's replacements; empty when the want is a stat.
	Replacements []int
	// Stat is a stat handle.
	Stat uint32
	// Weight scales the total into the seed's score.
	Weight float64
	// Min is the least total a seed must reach.
	Min int64
}

// Match is a seed that reaches every want's Min.
type Match struct {
	// Seed is the seed the jewel shows.
	Seed uint32
	// Score is the sum of each want's total times its weight.
	Score float64
	// Totals holds each want's total, in the wants' order.
	Totals []int64
}

// hit is where a row adds to a want: to its count, or the value of one of the row's stats.
type hit struct {
	want int
	// stat is the row's stat index, or -1 to count the passive.
	stat int
}

// Search conquers nodes with every seed of seeds and no conqueror, and returns the seeds
// whose totals reach every want's Min, by score from highest, then by seed. Limit bounds the
// matches returned; 0 returns all of them.
func (v *Versions) Search(version int, seeds SeedRange, nodes []Node, wants []Want, limit int) []Match {
	if v.Name(version) == "" || seeds.Step == 0 {
		return nil
	}
	rows, tree := &v.rows[version], v.trees[version]
	h := hitsOf(rows, wants)
	nodes = slices.DeleteFunc(slices.Clone(nodes), func(n Node) bool { return !h.wanted(rows, n.Kind) })

	var out []Match
	totals := make([]int64, len(wants))
	for seed := seeds.First; seed <= seeds.Last; seed += seeds.Step {
		clear(totals)
		for _, n := range nodes {
			res := tree.Conquer(Jewel{Seed: seed}, n)
			h.add(&res, totals)
		}
		if m, ok := match(seed, wants, totals); ok {
			out = append(out, m)
		}
	}
	slices.SortStableFunc(out, func(a, b Match) int { return cmp.Compare(b.Score, a.Score) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// hits are where each replacement and addition row of a version adds to the wants.
type hits struct {
	replacements, additions [][]hit
}

func hitsOf(v *dataset.TimelessVersion, wants []Want) hits {
	h := hits{replacements: make([][]hit, len(v.Replacements)), additions: make([][]hit, len(v.Additions))}
	for w, want := range wants {
		for _, r := range want.Replacements {
			h.replacements[r] = append(h.replacements[r], hit{want: w, stat: -1})
		}
		if len(want.Replacements) > 0 {
			continue
		}
		for r, row := range v.Replacements {
			h.replacements[r] = appendStatHits(h.replacements[r], w, want.Stat, row.Stats)
		}
		for a, row := range v.Additions {
			h.additions[a] = appendStatHits(h.additions[a], w, want.Stat, row.Stats)
		}
	}
	return h
}

func appendStatHits(hs []hit, want int, stat uint32, stats []dataset.StatRange) []hit {
	for s, sr := range stats {
		if sr.Stat == stat {
			hs = append(hs, hit{want: want, stat: s})
		}
	}
	return hs
}

// add adds what res gives to totals.
func (h *hits) add(res *Result, totals []int64) {
	if res.Replaced {
		for _, hh := range h.replacements[res.Replacement] {
			if hh.stat < 0 {
				totals[hh.want]++
			} else {
				totals[hh.want] += int64(res.Rolls[hh.stat])
			}
		}
	}
	for _, a := range res.Additions() {
		for _, hh := range h.additions[a.Addition] {
			totals[hh.want] += int64(a.Rolls[hh.stat])
		}
	}
}

// wanted tells whether a row a passive of kind k can get adds to a want. Conquering one that
// cannot is the larger part of a search's time, and leaving it out changes no total.
func (h *hits) wanted(v *dataset.TimelessVersion, k Kind) bool {
	for r, row := range v.Replacements {
		if len(h.replacements[r]) > 0 && slices.Contains(row.Kinds, dataset.TimelessKind(k)) {
			return true
		}
	}
	for a, row := range v.Additions {
		if len(h.additions[a]) > 0 && slices.Contains(row.Kinds, dataset.TimelessKind(k)) {
			return true
		}
	}
	return false
}

func match(seed uint32, wants []Want, totals []int64) (Match, bool) {
	m := Match{Seed: seed}
	for w, want := range wants {
		if totals[w] < want.Min {
			return Match{}, false
		}
		m.Score += want.Weight * float64(totals[w])
	}
	m.Totals = slices.Clone(totals)
	return m, true
}
