package poe1

import (
	"regexp"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// Proportion exposes the proportional view of an engine stat to tests until Sheet.Explain.
func (sh *Sheet) Proportion(name string) (core.Proportion, error) { return sh.proportion(name) }

// OutputStat returns the engine stat of catalogue output o.
func OutputStat(o Output) string { return o.stat }

// Excluded returns a func that, for a stat whose text matches the sheet phrases but that
// FeedsSheet rejects, returns why and the line; ok is false for every other stat.
func Excluded(s *dataset.Store) (func(stat string, scope resolve.Scope) (why, line string, ok bool), error) {
	t, err := newSheetText(s)
	if err != nil {
		return nil, err
	}
	return func(stat string, scope resolve.Scope) (string, string, bool) {
		j := t.judge(stat, scope)
		return j.excludes, j.excluded, !j.feeds && j.excludes != ""
	}, nil
}

// Rules returns every rule FeedsSheet decides with, keyed "group: name".
func Rules() map[string]*regexp.Regexp {
	out := map[string]*regexp.Regexp{}
	p := newPatterns()
	for group, rules := range map[string][]rule{
		"feeds by id": p.feedsByID, "not theirs by id": p.notTheirsByID, "not sheet by id": p.notSheetByID,
		"sheet line": p.sheetLines, "not their line": p.notTheirLines, "not sheet line": p.notSheetLines,
	} {
		for _, r := range rules {
			out[group+": "+r.name] = r.re
		}
	}
	return out
}
