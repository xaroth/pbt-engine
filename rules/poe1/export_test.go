package poe1

import "github.com/xaroth/pbt-engine/core"

// Proportion exposes the proportional view of an engine stat to tests until Sheet.Explain.
func (sh *Sheet) Proportion(name string) (core.Proportion, error) { return sh.proportion(name) }

// OutputStat returns the engine stat of catalogue output o.
func OutputStat(o Output) string { return o.stat }
