package build

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/xaroth/pbt-engine/dataset"
)

// MaxLevel is the highest character level.
const MaxLevel = 100

// Kinds of ProblemError: what keeps a build from being checked at all. Check returns a
// *CheckError, and errors.Is matches each of its problems' kinds.
var (
	ErrGame    = errors.New("game differs from the dataset")
	ErrVariant = errors.New("tree variant not in the dataset")
	ErrClass   = errors.New("class not in the tree or characters")
)

// Code names a kind of warning.
type Code string

// Warning codes of Check. Each warning says what Check did about it.
const (
	AscendancyNotOfClass  Code = "ascendancy-not-of-class"
	LevelOutOfRange       Code = "level-out-of-range"
	NodeUnknown           Code = "node-unknown"
	NodeRepeated          Code = "node-repeated"
	NodeOtherAscendancy   Code = "node-other-ascendancy"
	NodeOtherClassStart   Code = "node-other-class-start"
	NodesDisconnected     Code = "nodes-disconnected"
	MasteryNotAllocated   Code = "mastery-not-allocated"
	MasteryEffectUnknown  Code = "mastery-effect-unknown"
	MasteryEffectRepeated Code = "mastery-effect-repeated"
	MasteryNotChosen      Code = "mastery-not-chosen"
)

// Warning is something a run can go on without; Message says what was done about it. Subject
// names what it is about, such as "node 12345".
type Warning struct {
	Code    Code
	Subject string
	Message string
}

func (w Warning) String() string { return string(w.Code) + " " + w.Subject + ": " + w.Message }

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

// Checked is a build whose ids all exist in one dataset, with what did not fit fixed as its
// warnings say. Only Check creates it. The Store, Tree, Class, Character and Nodes it returns
// point into the Store's shared tables, so they are read-only, as everything a Store returns.
type Checked struct {
	build     Build
	store     *dataset.Store
	tree      *dataset.Tree
	class     *dataset.Class
	character *dataset.Character
	nodes     []*dataset.Node
	ascStart  *dataset.Node
	masteries []MasteryChoice
	warnings  []Warning
}

// MasteryChoice is an allocated mastery node and its chosen effect.
type MasteryChoice struct {
	Node   *dataset.Node
	Effect dataset.MasteryEffect
}

// Check checks b against the dataset s and fixes what it can: it clears an ascendancy of
// another class, clamps the level to 1 to 100, drops unknown, repeated and other classes' or
// ascendancies' nodes and mastery choices that do not fit, each with a warning. It warns
// without a fix about nodes not connected to the class or ascendancy start and about
// allocated masteries with no choice. It returns a *CheckError when the game, tree variant or
// class does not fit, or the error of a dataset group that fails to load. The point budget
// depends on the game's quests; the rules check it.
func Check(b *Build, s *dataset.Store) (*Checked, []Warning, error) {
	var ps problems
	m := s.Manifest()
	if b.Game != m.Game {
		ps.addf(ErrGame, 0, "build %q, dataset %q", b.Game, m.Game)
		return nil, nil, ps.err()
	}
	c := &Checked{build: *b.Clone(), store: s}
	c.build.fillDefaults()
	slices.Sort(c.build.Nodes)
	variant := c.build.Variant
	if !slices.Contains(m.Trees(), variant) {
		ps.addf(ErrVariant, 0, "%s in %s", variant, m.Version)
		return nil, nil, ps.err()
	}
	tree, err := s.Tree(variant)
	if err != nil {
		return nil, nil, fmt.Errorf("build: load tree: %w", err)
	}
	chars, err := s.Characters()
	if err != nil {
		return nil, nil, fmt.Errorf("build: load characters: %w", err)
	}
	c.tree = tree
	if !c.setClass(chars) {
		ps.addf(ErrClass, 0, "%q", b.Class)
		return nil, nil, ps.err()
	}
	c.checkAscendancy()
	c.checkLevel()
	c.checkNodes()
	c.checkConnected()
	c.checkMasteries()
	return c, slices.Clone(c.warnings), nil
}

// problems collects what Check finds.
type problems []*ProblemError

func (ps *problems) addf(kind error, node uint32, format string, args ...any) {
	*ps = append(*ps, &ProblemError{Kind: kind, Node: node, detail: fmt.Sprintf(format, args...)})
}

func (ps *problems) err() error { return &CheckError{*ps} }

// Build returns a copy of the build as checked: defaults filled in, nodes sorted, the fixes of
// the warnings applied.
func (c *Checked) Build() Build { return *c.build.Clone() }

// Warnings returns what Check found and did.
func (c *Checked) Warnings() []Warning { return slices.Clone(c.warnings) }

// Masteries returns the mastery choices kept, by node id.
func (c *Checked) Masteries() []MasteryChoice { return slices.Clone(c.masteries) }

// Points returns the passive points the allocation uses: passive nodes other than the class
// start, ascendancy nodes other than the ascendancy start, and the points the allocated nodes
// grant.
func (c *Checked) Points() (passive, ascendancy, granted int) {
	for _, n := range c.nodes {
		switch {
		case n.Kind == dataset.ClassStart, n.Kind == dataset.AscendancyStart:
		case n.Ascendancy != "":
			ascendancy++
		default:
			passive++
		}
		granted += int(n.SkillPoints)
	}
	return passive, ascendancy, granted
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

func (c *Checked) warnf(code Code, subject, format string, args ...any) {
	c.warnings = append(c.warnings, Warning{Code: code, Subject: subject, Message: fmt.Sprintf(format, args...)})
}

// setClass sets c's class and character, and reports whether both exist.
func (c *Checked) setClass(chars []dataset.Character) bool {
	b := &c.build
	if i := slices.IndexFunc(c.tree.Classes, func(cl dataset.Class) bool { return cl.Character == b.Class }); i >= 0 {
		c.class = &c.tree.Classes[i]
	}
	if i := slices.IndexFunc(chars, func(ch dataset.Character) bool { return ch.ID == b.Class }); i >= 0 {
		c.character = &chars[i]
	}
	return c.class != nil && c.character != nil
}

func (c *Checked) checkAscendancy() {
	b := &c.build
	if b.Ascendancy == "" {
		return
	}
	if !slices.ContainsFunc(c.class.Ascendancies, func(a dataset.Ascendancy) bool { return a.ID == b.Ascendancy }) {
		c.warnf(AscendancyNotOfClass, "ascendancy "+b.Ascendancy, "not an ascendancy of %s; cleared, its nodes dropped", b.Class)
		b.Ascendancy = ""
		return
	}
	for i := range c.tree.Nodes {
		if n := &c.tree.Nodes[i]; n.Kind == dataset.AscendancyStart && n.Ascendancy == b.Ascendancy {
			c.ascStart = n
		}
	}
}

func (c *Checked) checkLevel() {
	if l := c.build.Level; l < 1 || l > MaxLevel {
		c.build.Level = min(max(l, 1), MaxLevel)
		c.warnf(LevelOutOfRange, "level", "%d is outside 1 to %d; %d used", l, MaxLevel, c.build.Level)
	}
}

// checkNodes keeps the allocated nodes that exist and fit the class and ascendancy.
func (c *Checked) checkNodes() {
	var kept []uint32
	for i, id := range c.build.Nodes {
		subject := fmt.Sprintf("node %d", id)
		if i > 0 && c.build.Nodes[i-1] == id {
			c.warnf(NodeRepeated, subject, "allocated more than once; counted once")
			continue
		}
		n := c.node(id)
		switch {
		case n == nil:
			c.warnf(NodeUnknown, subject, "not in the %s tree of %s; dropped", c.build.Variant, c.store.Manifest().Version)
		case n.Ascendancy != "" && n.Ascendancy != c.build.Ascendancy:
			c.warnf(NodeOtherAscendancy, subject, "a node of %s; dropped", n.Ascendancy)
		case n.Kind == dataset.ClassStart && n.ID != c.class.StartNode:
			c.warnf(NodeOtherClassStart, subject, "another class's start; dropped")
		default:
			c.nodes = append(c.nodes, n)
			kept = append(kept, id)
		}
	}
	c.build.Nodes = kept
}

func (c *Checked) node(id uint32) *dataset.Node {
	j, ok := slices.BinarySearchFunc(c.tree.Nodes, id, func(n dataset.Node, id uint32) int { return cmp.Compare(n.ID, id) })
	if !ok {
		return nil
	}
	return &c.tree.Nodes[j]
}

// checkConnected warns about allocated nodes no path of allocated nodes joins to the class
// start or the ascendancy start. They stay allocated: jewels can make such trees legal.
func (c *Checked) checkConnected() {
	allocated := map[uint32]bool{c.class.StartNode: true}
	for _, n := range c.nodes {
		allocated[n.ID] = true
	}
	roots := []uint32{c.class.StartNode}
	if c.ascStart != nil {
		allocated[c.ascStart.ID] = true
		roots = append(roots, c.ascStart.ID)
	}
	reached := c.reach(roots, allocated)
	var lost []string
	for _, n := range c.nodes {
		if !reached[n.ID] {
			lost = append(lost, strconv.FormatUint(uint64(n.ID), 10))
		}
	}
	if len(lost) > 0 {
		c.warnf(NodesDisconnected, "nodes "+strings.Join(lost, ", "), "not connected to the class or ascendancy start; kept")
	}
}

// reach returns the nodes reachable from roots through nodes in allowed.
func (c *Checked) reach(roots []uint32, allowed map[uint32]bool) map[uint32]bool {
	reached := map[uint32]bool{}
	queue := slices.Clone(roots)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if reached[id] {
			continue
		}
		reached[id] = true
		if n := c.node(id); n != nil {
			for _, nb := range n.Neighbours {
				if allowed[nb] && !reached[nb] {
					queue = append(queue, nb)
				}
			}
		}
	}
	return reached
}

// checkMasteries keeps the mastery choices that fit and warns about allocated masteries with
// none. An effect can be chosen once; the lowest node keeps it.
func (c *Checked) checkMasteries() {
	chosen := map[uint32]bool{}
	for _, id := range slices.Sorted(maps.Keys(c.build.Masteries)) {
		if !c.choose(id, chosen) {
			delete(c.build.Masteries, id)
		}
	}
	for _, n := range c.nodes {
		if _, ok := c.build.Masteries[n.ID]; n.Kind == dataset.Mastery && !ok {
			c.warnf(MasteryNotChosen, fmt.Sprintf("node %d", n.ID), "allocated mastery with no effect chosen; it gives nothing")
		}
	}
}

// choose keeps the mastery choice on node id, or warns and reports false. chosen holds the
// effects kept so far.
func (c *Checked) choose(id uint32, chosen map[uint32]bool) bool {
	effect, subject := c.build.Masteries[id], fmt.Sprintf("node %d", id)
	n := c.allocated(id)
	if n == nil || n.Kind != dataset.Mastery {
		c.warnf(MasteryNotAllocated, subject, "effect %d chosen on a node that is not an allocated mastery; dropped", effect)
		return false
	}
	i := slices.IndexFunc(n.MasteryEffects, func(e dataset.MasteryEffect) bool { return e.ID == effect })
	switch {
	case i < 0:
		c.warnf(MasteryEffectUnknown, subject, "effect %d is not one of this mastery's; dropped", effect)
		return false
	case chosen[effect]:
		c.warnf(MasteryEffectRepeated, subject, "effect %d is chosen on another mastery already; dropped", effect)
		return false
	}
	chosen[effect] = true
	c.masteries = append(c.masteries, MasteryChoice{Node: n, Effect: n.MasteryEffects[i]})
	return true
}

func (c *Checked) allocated(id uint32) *dataset.Node {
	for _, n := range c.nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}
