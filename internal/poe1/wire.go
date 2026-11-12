package poe1

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/xaroth/pbt-engine/internal/resolve"
)

// The JSON of an Explanation is part of the simulator's wire format; the schema in
// simulator/wire.schema.json describes it. As there, every field is always present and lists
// are never null, except Tree, which is null outside the tree view.

type wireExplanation struct {
	Output    Output        `json:"output"`
	View      string        `json:"view"`
	Version   string        `json:"version"`
	Amount    float64       `json:"amount"`
	Effective float64       `json:"effective"`
	Value     float64       `json:"value"`
	CapBy     []string      `json:"capBy"`
	Lines     []wireLine    `json:"lines"`
	Tree      *wireTreeNode `json:"tree"`
}

type wireLine struct {
	Source Source         `json:"source"`
	Value  float64        `json:"value"`
	Total  float64        `json:"total"`
	Parts  []wireLinePart `json:"parts"`
}

type wireLinePart struct {
	Op    string   `json:"op"`
	Value float64  `json:"value"`
	Via   []string `json:"via"`
	From  *Source  `json:"from"`
}

type wireTreeNode struct {
	Stat       string          `json:"stat"`
	Formula    string          `json:"formula"`
	Terms      []wireTreeTerm  `json:"terms"`
	Overridden bool            `json:"overridden"`
	Total      float64         `json:"total"`
	Capped     float64         `json:"capped"`
	Value      float64         `json:"value"`
	Reads      []*wireTreeNode `json:"reads"`
	Repeat     bool            `json:"repeat"`
}

type wireTreeTerm struct {
	Source Source  `json:"source"`
	Op     string  `json:"op"`
	Value  float64 `json:"value"`
	Stat   string  `json:"stat"`
	Rate   float64 `json:"rate"`
	Held   bool    `json:"held"`
}

// MarshalJSON encodes the explanation with its view and ops by name and sources by kind.
func (ex Explanation) MarshalJSON() ([]byte, error) {
	w := wireExplanation{
		Output: ex.Output, View: ex.View.String(), Version: ex.Version,
		Amount: ex.Amount, Effective: ex.Effective, Value: ex.Value,
		CapBy: orEmpty(ex.CapBy), Lines: make([]wireLine, 0, len(ex.Lines)), Tree: wireTree(ex.Tree),
	}
	for _, l := range ex.Lines {
		wl := wireLine{Source: l.Source, Value: l.Value, Total: l.Total, Parts: make([]wireLinePart, 0, len(l.Parts))}
		for _, p := range l.Parts {
			wl.Parts = append(wl.Parts, wireLinePart{Op: p.Op.String(), Value: p.Value, Via: orEmpty(p.Via), From: p.From})
		}
		w.Lines = append(w.Lines, wl)
	}
	return marshal(w)
}

func wireTree(n *TreeNode) *wireTreeNode {
	if n == nil {
		return nil
	}
	w := &wireTreeNode{
		Stat: n.Stat, Formula: n.Formula, Overridden: n.Overridden, Total: n.Total, Capped: n.Capped, Value: n.Value,
		Repeat: n.Repeat, Terms: make([]wireTreeTerm, 0, len(n.Terms)), Reads: make([]*wireTreeNode, 0, len(n.Reads)),
	}
	for _, t := range n.Terms {
		w.Terms = append(w.Terms, wireTreeTerm{Source: t.Source, Op: t.Op.String(), Value: t.Value, Stat: t.Stat, Rate: t.Rate, Held: t.Held})
	}
	for _, r := range n.Reads {
		w.Reads = append(w.Reads, wireTree(r))
	}
	return w
}

// MarshalJSON encodes the source as an object whose kind says which other fields it has:
// character (id), level, node (node, effect: 0 for the node's own stats), quest (quest, flag)
// or rule (id, and text in English).
func (s Source) MarshalJSON() ([]byte, error) {
	if s.Bundle == nil {
		return marshal(struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
			Text string `json:"text"`
		}{"rule", s.RuleID, s.Rule})
	}
	switch src := s.Bundle.Source.(type) {
	case resolve.CharacterSource:
		return marshal(struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
		}{"character", src.ID})
	case resolve.LevelSource:
		return marshal(struct {
			Kind string `json:"kind"`
		}{"level"})
	case resolve.NodeSource:
		return marshal(struct {
			Kind   string `json:"kind"`
			Node   uint32 `json:"node"`
			Effect uint32 `json:"effect"`
		}{"node", src.ID, src.Effect})
	case resolve.QuestSource:
		return marshal(struct {
			Kind  string `json:"kind"`
			Quest string `json:"quest"`
			Flag  string `json:"flag"`
		}{"quest", src.ID, src.Flag})
	}
	return nil, fmt.Errorf("%w %T", errSourceKind, s.Bundle.Source)
}

var errSourceKind = errors.New("poe1: no JSON for source")

func marshal(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("poe1: %w", err)
	}
	return data, nil
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
