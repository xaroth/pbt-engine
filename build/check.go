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
	var problems []*ProblemError
	fail := func(kind error, node uint32, format string, args ...any) {
		problems = append(problems, &ProblemError{Kind: kind, Node: node, detail: fmt.Sprintf(format, args...)})
	}
	stop := func() (*Checked, error) { return nil, &CheckError{problems} }

	m := s.Manifest()
	if b.Game != m.Game {
		fail(ErrGame, 0, "build %q, dataset %q", b.Game, m.Game)
		return stop()
	}
	c := &Checked{build: *b, store: s}
	c.build.fillDefaults()
	c.build.Nodes = slices.Sorted(slices.Values(b.Nodes))
	variant := c.build.Variant
	if !slices.Contains(m.Trees(), variant) {
		fail(ErrVariant, 0, "%s in %s", variant, m.Version)
		return stop()
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

	if i := slices.IndexFunc(tree.Classes, func(cl dataset.Class) bool { return cl.Character == b.Class }); i >= 0 {
		c.class = &tree.Classes[i]
	}
	if i := slices.IndexFunc(chars, func(ch dataset.Character) bool { return ch.ID == b.Class }); i >= 0 {
		c.character = &chars[i]
	}
	if c.class == nil || c.character == nil {
		fail(ErrClass, 0, "%q", b.Class)
	}
	if b.Ascendancy != "" && c.class != nil &&
		!slices.ContainsFunc(c.class.Ascendancies, func(a dataset.Ascendancy) bool { return a.ID == b.Ascendancy }) {
		fail(ErrAscendancy, 0, "%q, class %s", b.Ascendancy, b.Class)
	}
	if b.Level < 1 || b.Level > MaxLevel {
		fail(ErrLevel, 0, "%d, want 1 to %d", b.Level, MaxLevel)
	}
	switch c.build.Scenario.Campaign {
	case CampaignComplete, CampaignActs6To10, CampaignActs1To5:
	default:
		fail(ErrCampaign, 0, "%q, want %s, %s or %s", b.Scenario.Campaign, CampaignComplete, CampaignActs6To10, CampaignActs1To5)
	}
	switch b.Scenario.Bandits {
	case BanditsNotCompleted, BanditsKillAll, BanditsHelpAlira, BanditsHelpKraityn, BanditsHelpOak:
	default:
		fail(ErrBandits, 0, "%q, want %s, %s, %s, %s or empty (quest not completed)",
			b.Scenario.Bandits, BanditsKillAll, BanditsHelpAlira, BanditsHelpKraityn, BanditsHelpOak)
	}

	for i, id := range c.build.Nodes {
		if i > 0 && c.build.Nodes[i-1] == id {
			fail(ErrNodeRepeated, id, "node %d", id)
			continue
		}
		j, ok := slices.BinarySearchFunc(tree.Nodes, id, func(n dataset.Node, id uint32) int { return cmp.Compare(n.ID, id) })
		if !ok {
			fail(ErrNodeUnknown, id, "node %d", id)
			continue
		}
		n := &tree.Nodes[j]
		switch {
		case n.Ascendancy != "" && n.Ascendancy != b.Ascendancy:
			fail(ErrNodeAscendancy, id, "node %d of %s", id, n.Ascendancy)
		case n.Kind == dataset.ClassStart && (c.class == nil || n.ID != c.class.StartNode):
			fail(ErrNodeOtherClassStart, id, "node %d", id)
		default:
			c.nodes = append(c.nodes, n)
		}
	}
	if len(problems) > 0 {
		return stop()
	}
	return c, nil
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
