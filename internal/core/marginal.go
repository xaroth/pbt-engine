package core

import "slices"

// Margin is what a node loses without one source: Total before caps, Value after caps and
// rounding. Negative when the source lowers the node.
type Margin struct {
	Source       SourceID
	Total, Value float64
}

// Marginal returns, for every enabled source the node depends on (its terms, and through the
// nodes it reads their terms, conditions and scales), what the node loses with that source
// off. Sources that change neither Total nor Value are left out. Sources are listed in id
// order. Each source is turned off and back on outside the edit journal, so the session is as
// before.
func (s *Session) Marginal(n Node) ([]Margin, error) {
	e, err := s.Explain(n)
	if err != nil {
		return nil, err
	}
	srcs, err := s.closure(n)
	if err != nil {
		return nil, err
	}
	var out []Margin
	for _, src := range srcs {
		s.setSource(src, false)
		without, err := s.Explain(n)
		s.setSource(src, true)
		if err != nil {
			return nil, err
		}
		if m := (Margin{Source: src, Total: e.Total - without.Total, Value: e.Value - without.Value}); m.Total != 0 || m.Value != 0 {
			out = append(out, m)
		}
	}
	return out, nil
}

// closure returns the enabled sources of the terms of n and of every node it reads,
// transitively, sorted.
func (s *Session) closure(n Node) ([]SourceID, error) {
	seen := map[uint32]bool{s.nodeID(n): true}
	var srcs []SourceID
	queue := []Node{n}
	for len(queue) > 0 {
		e, err := s.Explain(queue[0])
		queue = queue[1:]
		if err != nil {
			return nil, err
		}
		for _, t := range e.Terms {
			if !s.srcOff[t.Source] {
				srcs = append(srcs, t.Source)
			}
		}
		for _, r := range e.Reads {
			if id := s.nodeID(r); !seen[id] {
				seen[id] = true
				queue = append(queue, r)
			}
		}
	}
	slices.Sort(srcs)
	return slices.Compact(srcs), nil
}
