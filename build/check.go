package build

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/xaroth/pbt-engine/dataset"
)

// MaxLevel is the highest character level.
const MaxLevel = 100

// Kinds of ProblemError. Check returns a *CheckError, and errors.Is matches each of its problems'
// kinds.
var (
	ErrGame                = errors.New("game differs from the dataset")
	ErrVariant             = errors.New("tree variant not in the dataset")
	ErrClass               = errors.New("class not in the tree or characters")
	ErrAscendancy          = errors.New("ascendancy not of the class")
	ErrLevel               = errors.New("level out of range")
	ErrCampaign            = errors.New("unknown campaign stage")
	ErrBandits             = errors.New("unknown bandit quest outcome")
	ErrNodeUnknown         = errors.New("node not in the tree")
	ErrNodeRepeated        = errors.New("node allocated more than once")
	ErrNodeAscendancy      = errors.New("node of another ascendancy")
	ErrNodeOtherClassStart = errors.New("node is another class's start")
)

// ProblemError is one way a build does not fit a dataset.
type ProblemError struct {
	// Kind is one of the Err values above.
	Kind error
	// Node is the node id of the node kinds, else 0.
	Node   uint32
	detail string
}

func (p *ProblemError) Error() string { return p.Kind.Error() + ": " + p.detail }

func (p *ProblemError) Unwrap() error { return p.Kind }

// CheckError lists every problem Check found.
type CheckError struct {
	Problems []*ProblemError
}

func (e *CheckError) Error() string {
	msgs := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		msgs[i] = p.Error()
	}
	return "build: " + strings.Join(msgs, "; ")
}

func (e *CheckError) Unwrap() []error {
	out := make([]error, len(e.Problems))
	for i, p := range e.Problems {
		out[i] = p
	}
	return out
}

// Checked is a build whose ids all exist in one dataset. Only Check creates it. The Store,
// Tree, Class, Character and Nodes it returns point into the Store's shared tables, so they are
// read-only, as everything a Store returns.
type Checked struct {
	build     Build
	store     *dataset.Store
	tree      *dataset.Tree
	class     *dataset.Class
	character *dataset.Character
	nodes     []*dataset.Node
}

// Check checks every id and scenario field of b against the dataset s. It returns a
// *CheckError listing every problem, or the error of a dataset group that fails to load.
// Tree connectivity, the point budget and mastery choices are not checked.
func Check(b *Build, s *dataset.Store) (*Checked, error) {
	var ps problems
	m := s.Manifest()
	if b.Game != m.Game {
		ps.addf(ErrGame, 0, "build %q, dataset %q", b.Game, m.Game)
		return nil, ps.err()
	}
	c := &Checked{build: *b, store: s}
	c.build.fillDefaults()
	c.build.Nodes = slices.Sorted(slices.Values(b.Nodes))
	variant := c.build.Variant
	if !slices.Contains(m.Trees(), variant) {
		ps.addf(ErrVariant, 0, "%s in %s", variant, m.Version)
		return nil, ps.err()
	}
	tree, err := s.Tree(variant)
	if err != nil {
		return nil, fmt.Errorf("build: load tree: %w", err)
	}
	chars, err := s.Characters()
	if err != nil {
		return nil, fmt.Errorf("build: load characters: %w", err)
	}
	c.tree = tree

	ps.checkClass(c, b, chars)
	ps.checkScenario(c, b)
	ps.checkNodes(c, b)
	if len(ps) > 0 {
		return nil, ps.err()
	}
	return c, nil
}

// problems collects what Check finds.
type problems []*ProblemError

func (ps *problems) addf(kind error, node uint32, format string, args ...any) {
	*ps = append(*ps, &ProblemError{Kind: kind, Node: node, detail: fmt.Sprintf(format, args...)})
}

func (ps *problems) err() error { return &CheckError{*ps} }

// checkClass sets c's class and character and checks the ascendancy.
func (ps *problems) checkClass(c *Checked, b *Build, chars []dataset.Character) {
	if i := slices.IndexFunc(c.tree.Classes, func(cl dataset.Class) bool { return cl.Character == b.Class }); i >= 0 {
		c.class = &c.tree.Classes[i]
	}
	if i := slices.IndexFunc(chars, func(ch dataset.Character) bool { return ch.ID == b.Class }); i >= 0 {
		c.character = &chars[i]
	}
	if c.class == nil || c.character == nil {
		ps.addf(ErrClass, 0, "%q", b.Class)
	}
	if b.Ascendancy != "" && c.class != nil &&
		!slices.ContainsFunc(c.class.Ascendancies, func(a dataset.Ascendancy) bool { return a.ID == b.Ascendancy }) {
		ps.addf(ErrAscendancy, 0, "%q, class %s", b.Ascendancy, b.Class)
	}
}

func (ps *problems) checkScenario(c *Checked, b *Build) {
	if b.Level < 1 || b.Level > MaxLevel {
		ps.addf(ErrLevel, 0, "%d, want 1 to %d", b.Level, MaxLevel)
	}
	switch c.build.Scenario.Campaign {
	case CampaignComplete, CampaignActs6To10, CampaignActs1To5:
	default:
		ps.addf(ErrCampaign, 0, "%q, want %s, %s or %s", b.Scenario.Campaign, CampaignComplete, CampaignActs6To10, CampaignActs1To5)
	}
	switch b.Scenario.Bandits {
	case BanditsNotCompleted, BanditsKillAll, BanditsHelpAlira, BanditsHelpKraityn, BanditsHelpOak:
	default:
		ps.addf(ErrBandits, 0, "%q, want %s, %s, %s, %s or empty (quest not completed)",
			b.Scenario.Bandits, BanditsKillAll, BanditsHelpAlira, BanditsHelpKraityn, BanditsHelpOak)
	}
}

func (ps *problems) checkNodes(c *Checked, b *Build) {
	for i, id := range c.build.Nodes {
		if i > 0 && c.build.Nodes[i-1] == id {
			ps.addf(ErrNodeRepeated, id, "node %d", id)
			continue
		}
		j, ok := slices.BinarySearchFunc(c.tree.Nodes, id, func(n dataset.Node, id uint32) int { return cmp.Compare(n.ID, id) })
		if !ok {
			ps.addf(ErrNodeUnknown, id, "node %d", id)
			continue
		}
		if n := &c.tree.Nodes[j]; ps.nodeAllowed(n, c, b) {
			c.nodes = append(c.nodes, n)
		}
	}
}

func (ps *problems) nodeAllowed(n *dataset.Node, c *Checked, b *Build) bool {
	switch {
	case n.Ascendancy != "" && n.Ascendancy != b.Ascendancy:
		ps.addf(ErrNodeAscendancy, n.ID, "node %d of %s", n.ID, n.Ascendancy)
		return false
	case n.Kind == dataset.ClassStart && (c.class == nil || n.ID != c.class.StartNode):
		ps.addf(ErrNodeOtherClassStart, n.ID, "node %d", n.ID)
		return false
	}
	return true
}

// Build returns a copy of the build as checked: defaults filled in, nodes sorted.
func (c *Checked) Build() Build {
	b := c.build
	b.Nodes = slices.Clone(b.Nodes)
	return b
}

// Store returns the dataset the build was checked against.
func (c *Checked) Store() *dataset.Store { return c.store }

// Tree returns the build's tree variant.
func (c *Checked) Tree() *dataset.Tree { return c.tree }

// Class returns the build's class as the tree sees it.
func (c *Checked) Class() *dataset.Class { return c.class }

// Character returns the build's character row.
func (c *Checked) Character() *dataset.Character { return c.character }

// Nodes returns the allocated nodes, sorted by id.
func (c *Checked) Nodes() []*dataset.Node { return slices.Clone(c.nodes) }
