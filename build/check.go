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
	BloodlineUnknown      Code = "bloodline-unknown"
	LevelOutOfRange       Code = "level-out-of-range"
	NodeUnknown           Code = "node-unknown"
	NodeRepeated          Code = "node-repeated"
	NodeOtherAscendancy   Code = "node-other-ascendancy"
	NodeOtherClassStart   Code = "node-other-class-start"
	NodeProxy             Code = "node-proxy"
	NodesDisconnected     Code = "nodes-disconnected"
	MasteryNotAllocated   Code = "mastery-not-allocated"
	MasteryEffectUnknown  Code = "mastery-effect-unknown"
	MasteryEffectRepeated Code = "mastery-effect-repeated"
	MasteryNotChosen      Code = "mastery-not-chosen"
	ChoiceRepeated        Code = "choice-repeated"
)

// Warning is something a run can go on without; Message says what was done about it. Subject
// names what it is about, such as "node 12345".
type Warning struct {
	Code    Code   `json:"code"`
	Subject string `json:"subject"`
	Message string `json:"message"`
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
	build      Build
	store      *dataset.Store
	tree       *dataset.Tree
	class      *dataset.Class
	character  *dataset.Character
	chars      []dataset.Character
	nodes      []*dataset.Node
	ascStart   *dataset.Node
	bloodStart *dataset.Node
	masteries  []MasteryChoice
	warnings   []Warning
}

// MasteryChoice is an allocated mastery node and its chosen effect.
type MasteryChoice struct {
	Node   *dataset.Node
	Effect dataset.MasteryEffect
}

// Check checks b against the dataset s and fixes what it can: it clears an ascendancy of
// another class and a bloodline not among the tree's alternate ascendancies, clamps the level
// to 1 to 100, drops unknown, repeated and proxy nodes, other classes' nodes, nodes of
// ascendancies other than the build's ascendancy and bloodline, and mastery choices that do
// not fit, each with a warning. It warns without a fix about nodes not connected to the class,
// ascendancy or bloodline start and about allocated masteries with no choice. Against the dataset's skills group it drops unknown gems,
// clears unknown variants and slots without sockets, corrects which gems are supports, clamps
// gem levels and quality, and moves a main skill or active gem that does not fit to the first,
// each with a warning; a dataset without the group gets one warning and the skills stay as
// they are. Against the items group it leaves out items in no item slot, of an unknown base
// or of a class the slot does not take, and stats the dataset does not have, and warns about
// mod lines not read as stats; it leaves out an offhand item the game refuses beside the
// weapon, and warns about a quiver with no bow; a dataset without the group gets one warning. The weapon-swap
// slots are not in use: their items are left out and the gems socketed in them disabled,
// without a warning, so the skills they grant or hold are disabled too. It returns a
// *CheckError when the game, tree variant or class does not fit, or the error of a dataset
// group that fails to load. The point budget depends on the game's quests; the rules check it.
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
	c.tree, c.chars = tree, chars
	if !c.setClass(chars) {
		ps.addf(ErrClass, 0, "%q", b.Class)
		return nil, nil, ps.err()
	}
	c.checkAscendancy()
	c.checkBloodline()
	c.checkLevel()
	c.checkNodes()
	c.checkChoices()
	c.checkConnected()
	c.checkMasteries()
	if err := c.checkSkills(); err != nil {
		return nil, nil, err
	}
	if err := c.checkItems(); err != nil {
		return nil, nil, err
	}
	c.setAsideSwap()
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
// start, ascendancy and bloodline nodes other than their starts, and the points the allocated
// nodes grant. A bloodline spends ascendancy points. An option of a multiple-choice node is free: its parent paid the point.
func (c *Checked) Points() (passive, ascendancy, granted int) {
	for _, n := range c.nodes {
		switch {
		case n.Kind == dataset.ClassStart, n.Kind == dataset.AscendancyStart, n.MultipleChoiceOption:
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

// EditWarnings returns the warnings Check would give about edits applied to the checked build,
// as far as they concern the edits themselves: nodes it cannot take, and mastery effects on a
// node that is not an allocated mastery or not of that mastery. Connectivity, mastery choices
// still to make and point budgets are about the whole build; Check gives those.
func (c *Checked) EditWarnings(edits []Edit) []Warning {
	allocated := map[uint32]bool{}
	for _, n := range c.build.Nodes {
		allocated[n] = true
	}
	var out []Warning
	for _, e := range edits {
		switch e.Op {
		case OpAllocate:
			out = append(out, c.allocateWarnings(e.Nodes, allocated)...)
		case OpDeallocate:
			for _, id := range e.Nodes {
				delete(allocated, id)
			}
		case OpSetMastery:
			if w, bad := c.effectProblem(e.Node, e.Effect, allocated[e.Node]); bad {
				out = append(out, w)
			}
		case OpClearMastery, OpSetLevel, OpSetClass, OpSetAscendancy, OpSetBloodline, OpSetConfig, OpClearConfig,
			OpAddSkillGroup, OpRemoveSkillGroup, OpSetSkillSlot, OpSetMainSkill, OpAddGem, OpRemoveGem,
			OpSetActiveGem, OpSetGemLevel, OpSetGemQuality, OpSetGemEnabled, OpEquip, OpUnequip:
		}
	}
	return out
}

// allocateWarnings returns the warnings for allocating ids and marks the ones taken.
func (c *Checked) allocateWarnings(ids []uint32, allocated map[uint32]bool) []Warning {
	var out []Warning
	for _, id := range ids {
		if w, bad := c.nodeProblem(id, c.node(id)); bad {
			out = append(out, w)
		} else {
			allocated[id] = true
		}
	}
	return out
}

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
	c.ascStart = c.ascendancyStart(b.Ascendancy)
}

func (c *Checked) checkBloodline() {
	b := &c.build
	if b.Bloodline == "" {
		return
	}
	if !slices.ContainsFunc(c.tree.AlternateAscendancies, func(a dataset.Ascendancy) bool { return a.ID == b.Bloodline }) {
		c.warnf(BloodlineUnknown, "bloodline "+b.Bloodline, "not an alternate ascendancy of the %s tree; cleared, its nodes dropped", b.Variant)
		b.Bloodline = ""
		return
	}
	c.bloodStart = c.ascendancyStart(b.Bloodline)
}

// ownAscendancy reports whether a node of ascendancy id, empty for the main tree, is one the
// build can hold: the main tree's, its ascendancy's or its bloodline's.
func (c *Checked) ownAscendancy(id string) bool {
	return id == "" || id == c.build.Ascendancy || id == c.build.Bloodline
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
		if w, bad := c.nodeProblem(id, n); bad {
			c.warnings = append(c.warnings, w)
			continue
		}
		c.nodes = append(c.nodes, n)
		kept = append(kept, id)
	}
	c.build.Nodes = kept
}

// nodeProblem returns the warning for allocating node id (n, nil when not in the tree) when the
// build cannot take it.
func (c *Checked) nodeProblem(id uint32, n *dataset.Node) (Warning, bool) {
	subject := fmt.Sprintf("node %d", id)
	switch {
	case n == nil:
		return Warning{NodeUnknown, subject, fmt.Sprintf("not in the %s tree of %s; dropped", c.build.Variant, c.store.Manifest().Version)}, true
	case !c.ownAscendancy(n.Ascendancy):
		return Warning{NodeOtherAscendancy, subject, fmt.Sprintf("a node of %s; dropped", n.Ascendancy)}, true
	case n.Kind == dataset.ClassStart && n.ID != c.class.StartNode:
		return Warning{NodeOtherClassStart, subject, "another class's start; dropped"}, true
	case n.Kind == dataset.Proxy:
		return Warning{NodeProxy, subject, "a proxy node, which only a jewel allocates; dropped"}, true
	}
	return Warning{}, false
}

// effectProblem returns the warning for choosing effect on node when it does not fit.
func (c *Checked) effectProblem(node, effect uint32, allocated bool) (Warning, bool) {
	subject := fmt.Sprintf("node %d", node)
	n := c.node(node)
	if n == nil || n.Kind != dataset.Mastery || !allocated {
		return Warning{MasteryNotAllocated, subject, fmt.Sprintf("effect %d chosen on a node that is not an allocated mastery; dropped", effect)}, true
	}
	if !slices.ContainsFunc(n.MasteryEffects, func(e dataset.MasteryEffect) bool { return e.ID == effect }) {
		return Warning{MasteryEffectUnknown, subject, fmt.Sprintf("effect %d is not one of this mastery's; dropped", effect)}, true
	}
	return Warning{}, false
}

func (c *Checked) node(id uint32) *dataset.Node {
	j, ok := slices.BinarySearchFunc(c.tree.Nodes, id, func(n dataset.Node, id uint32) int { return cmp.Compare(n.ID, id) })
	if !ok {
		return nil
	}
	return &c.tree.Nodes[j]
}

// checkConnected warns about allocated nodes no path of allocated nodes joins to the class,
// ascendancy or bloodline start. They stay allocated: jewels can make such trees legal.
func (c *Checked) checkConnected() {
	reached := c.joined(c.allocatedSet())
	var lost []string
	for _, n := range c.nodes {
		if !reached[n.ID] {
			lost = append(lost, strconv.FormatUint(uint64(n.ID), 10))
		}
	}
	if len(lost) > 0 {
		c.warnf(NodesDisconnected, "nodes "+strings.Join(lost, ", "), "not connected to the class, ascendancy or bloodline start; kept")
	}
}

// checkChoices keeps one option per multiple-choice node, the lowest id, and drops the others:
// a choice node takes one of its options.
func (c *Checked) checkChoices() {
	taken := map[uint32]uint32{}
	var dropped []uint32
	for _, n := range c.nodes {
		p := c.choiceOf(n)
		if p == nil {
			continue
		}
		if first, ok := taken[p.ID]; ok {
			c.warnf(ChoiceRepeated, fmt.Sprintf("node %d", n.ID), "node %d already takes option %d; dropped", p.ID, first)
			dropped = append(dropped, n.ID)
		} else {
			taken[p.ID] = n.ID
		}
	}
	if len(dropped) == 0 {
		return
	}
	c.nodes = slices.DeleteFunc(c.nodes, func(n *dataset.Node) bool { return slices.Contains(dropped, n.ID) })
	c.build.Nodes = slices.DeleteFunc(c.build.Nodes, func(id uint32) bool { return slices.Contains(dropped, id) })
}

// choiceOf returns the multiple-choice node n is an option of, or nil.
func (c *Checked) choiceOf(n *dataset.Node) *dataset.Node {
	if !n.MultipleChoiceOption {
		return nil
	}
	for _, nb := range n.Neighbours {
		if p := c.node(nb); p != nil && p.MultipleChoice {
			return p
		}
	}
	return nil
}

// reach returns the nodes reachable from roots through nodes in allowed, all of them allocated
// or starts, stepping only where Crosses allows. A mastery is reached but not passed through:
// its edges say which notables unlock it, not a path.
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
		if n := c.node(id); n != nil && n.Kind != dataset.Mastery {
			for _, nb := range n.Neighbours {
				if allowed[nb] && !reached[nb] && Crosses(n, c.node(nb), true) {
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
			c.warnf(MasteryNotChosen, fmt.Sprintf("node %d", n.ID), "allocated mastery with no effect chosen; it counts towards its group only")
		}
	}
}

// choose keeps the mastery choice on node id, or warns and reports false. chosen holds the
// effects kept so far.
func (c *Checked) choose(id uint32, chosen map[uint32]bool) bool {
	effect, subject := c.build.Masteries[id], fmt.Sprintf("node %d", id)
	n := c.allocated(id)
	if w, bad := c.effectProblem(id, effect, n != nil); bad {
		c.warnings = append(c.warnings, w)
		return false
	}
	i := slices.IndexFunc(n.MasteryEffects, func(e dataset.MasteryEffect) bool { return e.ID == effect })
	if chosen[effect] {
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
