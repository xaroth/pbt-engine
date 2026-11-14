package simulator

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/output"
)

// wireValue is an output's value with the catalogue entry that says how to show it.
type wireValue struct {
	Output

	Value float64 `json:"value"`
}

type wireStats struct {
	Version  string         `json:"version"`
	Outputs  []wireValue    `json:"outputs"`
	Warnings []Warning      `json:"warnings"`
	Skill    *wireMainSkill `json:"skill"`
	Skills   []wireSkill    `json:"skills"`
}

type wireSkill struct {
	SkillRef

	GrantedEffect string      `json:"grantedEffect"`
	Outputs       []wireValue `json:"outputs"`
}

type wireSkillChanges struct {
	SkillRef

	Changes []wireChange `json:"changes"`
}

type wireMainSkill struct {
	Group         int      `json:"group"`
	Gem           int      `json:"gem"`
	ID            string   `json:"id"`
	GrantedEffect string   `json:"grantedEffect"`
	Level         int      `json:"level"`
	Quality       int      `json:"quality"`
	Types         []string `json:"types"`
}

// wireChange is Change with its catalogue entry. Pct is null where Change.Pct is NaN, which
// JSON cannot hold.
type wireChange struct {
	Output

	Before  float64  `json:"before"`
	After   float64  `json:"after"`
	Abs     float64  `json:"abs"`
	Pct     *float64 `json:"pct"`
	Changed bool     `json:"changed"`
}

type wireNodeChange struct {
	Node   uint32            `json:"node"`
	Fields []string          `json:"fields"`
	Before []build.StatValue `json:"before"`
	After  []build.StatValue `json:"after"`
}

type wireDiff struct {
	Before       wireStats          `json:"before"`
	After        wireStats          `json:"after"`
	Changes      []wireChange       `json:"changes"`
	Edits        []build.Edit       `json:"edits"`
	DataChanges  []wireNodeChange   `json:"dataChanges"`
	SkillChanges []wireSkillChanges `json:"skillChanges"`
}

type wireResult struct {
	Index        int                `json:"index"`
	Edits        []build.Edit       `json:"edits"`
	Changes      []wireChange       `json:"changes"`
	SkillChanges []wireSkillChanges `json:"skillChanges"`
	Warnings     []Warning          `json:"warnings"`
	Error        *string            `json:"error"`
}

// MarshalJSON encodes the values, each with its catalogue entry, the warnings, the main skill
// (null without one), each skill's values asked for and the dataset version. It leaves out the
// build and config the Stats keeps for Explain.
func (st *Stats) MarshalJSON() ([]byte, error) { return marshal(st.wire(catalogue())) }

// MarshalJSON encodes both sides, the change of each output computed on both, the edits
// (null when the builds cannot be turned into each other), the data changes and the change of
// each skill's outputs computed on both.
func (d *Diff) MarshalJSON() ([]byte, error) {
	cat := catalogue()
	w := wireDiff{
		Before:       d.Before.wire(cat),
		After:        d.After.wire(cat),
		Changes:      []wireChange{},
		Edits:        d.Edits,
		DataChanges:  make([]wireNodeChange, 0, len(d.DataChanges)),
		SkillChanges: []wireSkillChanges{},
	}
	for _, ss := range d.Before.perSkill {
		if !d.After.has(ss.Ref) {
			continue
		}
		sc := wireSkillChanges{SkillRef: ss.Ref, Changes: []wireChange{}}
		for _, k := range ss.keys {
			if c := d.GetSkill(ss.Ref, k); c.Has {
				sc.Changes = append(sc.Changes, changeOf(cat[k], c))
			}
		}
		w.SkillChanges = append(w.SkillChanges, sc)
	}
	for _, k := range d.Before.keys {
		if c := d.Get(k); c.Has {
			w.Changes = append(w.Changes, changeOf(cat[k], c))
		}
	}
	for _, nc := range d.DataChanges {
		w.DataChanges = append(w.DataChanges, wireNodeChange{
			Node: nc.Node, Fields: orEmpty(nc.Fields), Before: statList(nc.Before), After: statList(nc.After),
		})
	}
	return marshal(w)
}

// MarshalJSON encodes the result with its changes in catalogue order, its skills' changes by
// skill, and Err as its message.
func (r Result) MarshalJSON() ([]byte, error) {
	outputs := poe1.Outputs()
	w := wireResult{
		Index:        r.Index,
		Edits:        orEmpty(r.Edits),
		Changes:      changeList(outputs, r.Changes),
		SkillChanges: make([]wireSkillChanges, 0, len(r.SkillChanges)),
		Warnings:     orEmpty(r.Warnings),
	}
	for _, ref := range slices.SortedFunc(maps.Keys(r.SkillChanges), compareRefs) {
		w.SkillChanges = append(w.SkillChanges, wireSkillChanges{SkillRef: ref, Changes: changeList(outputs, r.SkillChanges[ref])})
	}
	if r.Err != nil {
		msg := r.Err.Error()
		w.Error = &msg
	}
	return marshal(w)
}

// changeList returns changes in the order of outputs.
func changeList(outputs []Output, changes map[output.Key]Change) []wireChange {
	out := make([]wireChange, 0, len(changes))
	for _, o := range outputs {
		if c, ok := changes[o.Key]; ok {
			out = append(out, changeOf(o, c))
		}
	}
	return out
}

func (st *Stats) wire(cat map[output.Key]Output) wireStats {
	w := wireStats{
		Version: st.Version(), Outputs: valueList(cat, st.keys, st.values), Warnings: orEmpty(st.warnings),
		Skills: make([]wireSkill, 0, len(st.perSkill)),
	}
	for _, ss := range st.perSkill {
		w.Skills = append(w.Skills, wireSkill{SkillRef: ss.Ref, GrantedEffect: ss.GrantedEffect, Outputs: valueList(cat, ss.keys, ss.values)})
	}
	if sk, ok := st.MainSkill(); ok {
		w.Skill = &wireMainSkill{
			Group: sk.Group, Gem: sk.Gem, ID: sk.ID, GrantedEffect: sk.GrantedEffect,
			Level: sk.Level, Quality: sk.Quality, Types: orEmpty(sk.Types),
		}
	}
	return w
}

func valueList(cat map[output.Key]Output, keys []output.Key, values map[output.Key]float64) []wireValue {
	out := make([]wireValue, 0, len(keys))
	for _, k := range keys {
		out = append(out, wireValue{Output: cat[k], Value: values[k]})
	}
	return out
}

func changeOf(o Output, c Change) wireChange {
	w := wireChange{Output: o, Before: c.Before, After: c.After, Abs: c.Abs, Changed: differs(c)}
	if !math.IsNaN(c.Pct) {
		w.Pct = &c.Pct
	}
	return w
}

// differs reports whether a change's sides differ in any bit.
func differs(c Change) bool { return math.Float64bits(c.Before) != math.Float64bits(c.After) }

func catalogue() map[output.Key]Output {
	out := map[output.Key]Output{}
	for _, o := range poe1.Outputs() {
		out[o.Key] = o
	}
	return out
}

// statList returns stats by id as a list sorted by id.
func statList(stats map[string]int32) []build.StatValue {
	out := make([]build.StatValue, 0, len(stats))
	for _, id := range slices.Sorted(maps.Keys(stats)) {
		out = append(out, build.StatValue{Stat: id, Value: stats[id]})
	}
	return out
}

func marshal(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("simulator: %w", err)
	}
	return data, nil
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
