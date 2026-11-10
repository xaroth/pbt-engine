package core

// Explanation is how one node's value is built: the tree view, one level deep. The nodes in
// Reads have explanations of their own.
type Explanation struct {
	Node Node
	// Stat is the node's stat name.
	Stat string
	// Formula is the stat's FormulaName; empty for the default aggregation or the flag rule.
	Formula string
	// Terms are the modifiers considered, in evaluation order: own stat, includes, shares.
	// Terms whose condition failed are listed with Held false.
	Terms []Term
	Agg   Agg
	// Overridden is true when an override replaced the aggregation or formula.
	Overridden bool
	// Reads are the nodes read by conditions, scales, share rates and the formula, each once,
	// in first-read order.
	Reads []Node
	// Total is the value before the stat's minimum and maximum, Capped after them, and Value
	// after rounding: the node's value.
	Total, Capped, Value float64
}

// Explain returns the explanation of node n.
func (s *Session) Explain(n Node) (Explanation, error) {
	if _, err := s.Get(n); err != nil {
		return Explanation{}, err
	}
	defer s.recoverEval()
	var c collector
	var deps []uint32
	v := s.eval(s.nodeID(n), &deps, &c)
	if err := s.takeErr(); err != nil {
		return Explanation{}, err
	}
	def := &s.m.stats[n.Stat]
	e := Explanation{
		Node: n, Stat: def.Name, Formula: def.FormulaName, Terms: c.terms, Agg: c.agg,
		Overridden: c.agg.HasOverride && (def.Formula == nil || !def.FormulaSeesOverride),
		Total:      c.total, Capped: def.clamp(c.total), Value: v,
	}
	seen := make(map[uint32]bool, len(deps))
	for _, d := range deps {
		if !seen[d] {
			seen[d] = true
			e.Reads = append(e.Reads, s.node(d))
		}
	}
	return e, nil
}
