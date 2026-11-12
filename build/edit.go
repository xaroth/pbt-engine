package build

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/xaroth/pbt-engine/config"
)

// Errors of edits and Diff; match them with errors.Is.
var (
	ErrEditOp        = errors.New("build: unknown edit op")
	ErrNotComparable = errors.New("build: builds differ in game or variant")
)

// EditOp names what an Edit changes.
type EditOp string

// Edit ops.
const (
	OpAllocate      EditOp = "allocate"
	OpDeallocate    EditOp = "deallocate"
	OpSetMastery    EditOp = "set-mastery"
	OpClearMastery  EditOp = "clear-mastery"
	OpSetLevel      EditOp = "set-level"
	OpSetClass      EditOp = "set-class"
	OpSetAscendancy EditOp = "set-ascendancy"
	OpSetConfig     EditOp = "set-config"
	OpClearConfig   EditOp = "clear-config"
)

// Edit is one change to a build, as a plain value that serializes to JSON, so batches, undo
// and remote callers share it: {"op": "allocate", "nodes": [1, 2]}. Edits are not checked when
// applied; a simulation judges the build they produce.
type Edit struct {
	Op EditOp `json:"op"`
	// Nodes for allocate and deallocate.
	Nodes []uint32 `json:"nodes,omitzero"`
	// Node and Effect for set-mastery, Node for clear-mastery.
	Node   uint32 `json:"node,omitzero"`
	Effect uint32 `json:"effect,omitzero"`
	Level  int    `json:"level,omitzero"`
	Class  string `json:"class,omitzero"`
	// Ascendancy for set-ascendancy; empty clears it.
	Ascendancy string `json:"ascendancy,omitzero"`
	// Key and Value for set-config, Key for clear-config.
	Key   string          `json:"key,omitzero"`
	Value json.RawMessage `json:"value,omitzero"`
	// Skill addresses the group, and the gem, of a skill edit. SkillGroup for add-skill-group,
	// Gem for add-gem; Slot, Quality and Enabled for the edits of that name, and Level for
	// set-gem-level. Slot and Item for equip, Slot for unequip.
	Skill      *SkillAt    `json:"skill,omitzero"`
	SkillGroup *SkillGroup `json:"group,omitzero"`
	Gem        *Gem        `json:"gem,omitzero"`
	Slot       Slot        `json:"slot,omitzero"`
	Quality    int         `json:"quality,omitzero"`
	Enabled    bool        `json:"enabled,omitzero"`
	Item       *Item       `json:"item,omitzero"`
}

// Allocate allocates nodes.
func Allocate(nodes ...uint32) Edit { return Edit{Op: OpAllocate, Nodes: slices.Clone(nodes)} }

// Deallocate deallocates nodes.
func Deallocate(nodes ...uint32) Edit { return Edit{Op: OpDeallocate, Nodes: slices.Clone(nodes)} }

// SetMastery chooses effect on mastery node.
func SetMastery(node, effect uint32) Edit { return Edit{Op: OpSetMastery, Node: node, Effect: effect} }

// ClearMastery removes the choice on mastery node.
func ClearMastery(node uint32) Edit { return Edit{Op: OpClearMastery, Node: node} }

// SetLevel sets the character level.
func SetLevel(level int) Edit { return Edit{Op: OpSetLevel, Level: level} }

// SetClass sets the class. Ascendancy and nodes stay; a simulation warns about what no longer
// fits. Checked.ChangeClass gives the edits that also drop it.
func SetClass(class string) Edit { return Edit{Op: OpSetClass, Class: class} }

// SetAscendancy sets the ascendancy; empty clears it. Nodes stay; Checked.ChangeAscendancy gives
// the edits that also drop the old ascendancy's nodes and allocate the new start.
func SetAscendancy(ascendancy string) Edit { return Edit{Op: OpSetAscendancy, Ascendancy: ascendancy} }

// SetConfig sets config key k to v; a value that fails the key's check is an error.
func SetConfig[T any](k config.Key[T], v T) (Edit, error) {
	var vals config.Values
	if err := config.Set(&vals, k, v); err != nil {
		return Edit{}, fmt.Errorf("build: %w", err)
	}
	return Edit{Op: OpSetConfig, Key: k.Name(), Value: vals[k.Name()]}, nil
}

// ClearConfig removes the named config key.
func ClearConfig(name string) Edit { return Edit{Op: OpClearConfig, Key: name} }

// Apply applies edits to b in order. It stops at an edit with an unknown op, or a skill edit
// addressing a group or gem b does not have (ErrEditTarget), after applying the ones before it.
func (b *Build) Apply(edits ...Edit) error {
	for i, e := range edits {
		if err := b.apply(e); err != nil {
			return fmt.Errorf("edit %d: %w", i, err)
		}
	}
	return nil
}

func (b *Build) apply(e Edit) error {
	switch e.Op {
	case OpAllocate, OpDeallocate:
		b.applyNodes(e)
	case OpSetMastery, OpClearMastery:
		b.applyMastery(e)
	case OpSetConfig, OpClearConfig:
		b.applyConfig(e)
	case OpSetLevel:
		b.Level = e.Level
	case OpSetClass:
		b.Class = e.Class
	case OpSetAscendancy:
		b.Ascendancy = e.Ascendancy
	case OpEquip, OpUnequip:
		b.applyItem(e)
	default:
		if slices.Contains(skillOps(), e.Op) {
			return b.Skills.apply(e)
		}
		return fmt.Errorf("%w %q", ErrEditOp, e.Op)
	}
	return nil
}

func (b *Build) applyNodes(e Edit) {
	if e.Op == OpDeallocate {
		b.Nodes = slices.DeleteFunc(b.Nodes, func(n uint32) bool { return slices.Contains(e.Nodes, n) })
		return
	}
	b.Nodes = append(b.Nodes, e.Nodes...)
	slices.Sort(b.Nodes)
	b.Nodes = slices.Compact(b.Nodes)
}

func (b *Build) applyMastery(e Edit) {
	if e.Op == OpClearMastery {
		delete(b.Masteries, e.Node)
		return
	}
	if b.Masteries == nil {
		b.Masteries = map[uint32]uint32{}
	}
	b.Masteries[e.Node] = e.Effect
}

func (b *Build) applyConfig(e Edit) {
	if e.Op == OpClearConfig {
		b.Config.Clear(e.Key)
		return
	}
	if b.Config == nil {
		b.Config = config.Values{}
	}
	b.Config[e.Key] = slices.Clone(e.Value)
}

// Diff returns the edits that turn a into b: class, ascendancy, level, nodes deallocated then
// allocated, mastery choices cleared then set, config keys cleared then set, each in sorted
// order, then the skill groups, then the items. Version and format are not compared. Builds of
// another game or tree variant cannot be turned into each other.
func Diff(a, b *Build) ([]Edit, error) {
	if a.Game != b.Game || a.Variant != b.Variant {
		return nil, fmt.Errorf("%w: %s %s and %s %s", ErrNotComparable, a.Game, a.Variant, b.Game, b.Variant)
	}
	var out []Edit
	if a.Class != b.Class {
		out = append(out, SetClass(b.Class))
	}
	if a.Ascendancy != b.Ascendancy {
		out = append(out, SetAscendancy(b.Ascendancy))
	}
	if a.Level != b.Level {
		out = append(out, SetLevel(b.Level))
	}
	if gone := minus(a.Nodes, b.Nodes); len(gone) > 0 {
		out = append(out, Deallocate(gone...))
	}
	if added := minus(b.Nodes, a.Nodes); len(added) > 0 {
		out = append(out, Allocate(added...))
	}
	out = append(out, masteryDiff(a.Masteries, b.Masteries)...)
	out = append(out, configDiff(a.Config, b.Config)...)
	out = append(out, skillsDiff(a.Skills, b.Skills)...)
	return append(out, itemsDiff(a.Items, b.Items)...), nil
}

// minus returns the ids in x and not in y, sorted and unique.
func minus(x, y []uint32) []uint32 {
	var out []uint32
	for _, n := range x {
		if !slices.Contains(y, n) {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func masteryDiff(a, b map[uint32]uint32) []Edit {
	var out []Edit
	for _, n := range slices.Sorted(maps.Keys(a)) {
		if _, ok := b[n]; !ok {
			out = append(out, ClearMastery(n))
		}
	}
	for _, n := range slices.Sorted(maps.Keys(b)) {
		if e, ok := a[n]; !ok || e != b[n] {
			out = append(out, SetMastery(n, b[n]))
		}
	}
	return out
}

func configDiff(a, b config.Values) []Edit {
	var out []Edit
	for _, k := range a.Names() {
		if _, ok := b[k]; !ok {
			out = append(out, ClearConfig(k))
		}
	}
	for _, k := range b.Names() {
		if v, ok := a[k]; !ok || string(v) != string(b[k]) {
			out = append(out, Edit{Op: OpSetConfig, Key: k, Value: slices.Clone(b[k])})
		}
	}
	return out
}
