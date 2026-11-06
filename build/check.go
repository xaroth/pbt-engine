package build

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
)

// MaxLevel is the highest character level.
const MaxLevel = 100

// Checked is a build whose ids all exist in one dataset version. Only Check creates it.
type Checked struct {
	build     Build
	version   *dataset.Version
	tree      *dataset.Tree
	class     *dataset.Class
	character *dataset.Character
	nodes     []*dataset.Node
}

// Open resolves the build's game and version in r and checks the build against it.
func Open(r *dataset.Registry, b *Build) (*Checked, error) {
	v, err := r.Resolve(b.Game, b.Version)
	if err != nil {
		return nil, err
	}
	return Check(b, v)
}

// Check checks every id of b against the dataset v and reports all problems found.
func Check(b *Build, v *dataset.Version) (*Checked, error) {
	if b.Game != v.Game() {
		return nil, fmt.Errorf("build: game %q, dataset is %s", b.Game, v.Game())
	}
	variant := cmp.Or(b.Variant, DefaultVariant)
	if !slices.Contains(v.Variants(), variant) {
		return nil, fmt.Errorf("build: dataset %s has no %s tree", v.Name(), variant)
	}
	tree, err := v.Tree(variant)
	if err != nil {
		return nil, err
	}
	chars, err := v.Characters()
	if err != nil {
		return nil, err
	}
	c := &Checked{build: *b, version: v, tree: tree}
	c.build.Variant = variant
	c.build.Nodes = slices.Clone(b.Nodes)

	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf("build: "+format, args...)) }

	if i := slices.IndexFunc(tree.Classes, func(cl dataset.Class) bool { return cl.Character == b.Class }); i >= 0 {
		c.class = &tree.Classes[i]
	} else {
		fail("class %q not in the %s tree", b.Class, variant)
	}
	if i := slices.IndexFunc(chars, func(ch dataset.Character) bool { return ch.ID == b.Class }); i >= 0 {
		c.character = &chars[i]
	} else if c.class != nil {
		fail("class %q not in characters", b.Class)
	}
	if b.Ascendancy != "" && c.class != nil &&
		!slices.ContainsFunc(c.class.Ascendancies, func(a dataset.Ascendancy) bool { return a.ID == b.Ascendancy }) {
		fail("ascendancy %q is not one of class %s", b.Ascendancy, b.Class)
	}
	if b.Level < 1 || b.Level > MaxLevel {
		fail("level %d, want 1 to %d", b.Level, MaxLevel)
	}
	switch b.Scenario.Bandit {
	case KillAll, Alira, Kraityn, Oak:
	default:
		fail("bandit %q, want alira, kraityn, oak or empty (kill all)", b.Scenario.Bandit)
	}
	switch b.Scenario.Penalty {
	case AfterAct10, AfterAct5, NoPenalty:
	default:
		fail("resistance penalty %q, want act5, none or empty (after act 10)", b.Scenario.Penalty)
	}

	slices.Sort(c.build.Nodes)
	for i, id := range c.build.Nodes {
		if i > 0 && c.build.Nodes[i-1] == id {
			fail("node %d allocated twice", id)
			continue
		}
		j, ok := slices.BinarySearchFunc(tree.Nodes, id, func(n dataset.Node, id uint32) int { return cmp.Compare(n.ID, id) })
		if !ok {
			fail("node %d not in the %s tree", id, variant)
			continue
		}
		n := &tree.Nodes[j]
		switch {
		case n.Ascendancy != "" && n.Ascendancy != b.Ascendancy:
			fail("node %d belongs to ascendancy %s", id, n.Ascendancy)
		case n.Kind == dataset.ClassStart && (c.class == nil || n.ID != c.class.StartNode):
			fail("node %d is the start of another class", id)
		default:
			c.nodes = append(c.nodes, n)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return c, nil
}

// Build returns a copy of the build as checked: variant filled in, nodes sorted.
func (c *Checked) Build() Build {
	b := c.build
	b.Nodes = slices.Clone(b.Nodes)
	return b
}

// Version returns the dataset the build was checked against.
func (c *Checked) Version() *dataset.Version { return c.version }

// Tree returns the build's tree variant.
func (c *Checked) Tree() *dataset.Tree { return c.tree }

// Class returns the build's class as the tree sees it.
func (c *Checked) Class() *dataset.Class { return c.class }

// Character returns the build's character row.
func (c *Checked) Character() *dataset.Character { return c.character }

// Nodes returns the allocated nodes, sorted by id.
func (c *Checked) Nodes() []*dataset.Node { return slices.Clone(c.nodes) }

// Level returns the character level.
func (c *Checked) Level() int { return c.build.Level }

// Ascendancy returns the ascendancy id, or empty.
func (c *Checked) Ascendancy() string { return c.build.Ascendancy }

// Scenario returns the scenario fields.
func (c *Checked) Scenario() Scenario { return c.build.Scenario }
