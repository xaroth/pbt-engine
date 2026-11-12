package dataset

import (
	"cmp"
	"fmt"
	"math"
	"slices"
)

// Layout is where the nodes of one tree variant are drawn, as the tree export places them.
// Coordinates are the export's: x grows to the right and y downwards.
type Layout struct {
	// Variant is the Tree.Variant the layout places.
	Variant string
	// MinX, MinY, MaxX and MaxY bound the drawn tree.
	MinX, MinY, MaxX, MaxY float64
	// Orbits are indexed by orbit number.
	Orbits []Orbit
	// Groups are sorted by ID.
	Groups []LayoutGroup
	// Nodes are sorted by ID and hold every node of the variant's tree.
	Nodes []LayoutNode
	// ClassArt is sorted by Class; a class without art has no entry.
	ClassArt []ClassArt
}

// Orbit is a ring around a group's centre that nodes sit on.
type Orbit struct {
	Radius float64
	// Angles are the node angles by orbit index, in degrees clockwise from straight up. They
	// are not always evenly spaced.
	Angles []float64
}

// LayoutGroup is a cluster of nodes around one centre.
type LayoutGroup struct {
	// ID is the group's key in the tree export, from 1.
	ID   uint32
	X, Y float64
	// Orbits are the orbit rings drawn around the group, which need not include every orbit
	// its nodes sit on.
	Orbits []uint8
	// Background is nil for a group drawn without one.
	Background *GroupBackground
	// Proxy marks the group of a cluster jewel socket's proxy node.
	Proxy bool
}

// GroupBackground is the art behind a group.
type GroupBackground struct {
	// Image is a key of the group background sprite sheet, such as PSGroupBackground3.
	Image string
	// HalfImage means Image is the top half, and the bottom half is its mirror.
	HalfImage bool
}

// LayoutNode places one node and names its art. Art is named by the game's art path, such
// as Art/2DArt/SkillIcons/passives/Blood2.png, which the tree export's sprite sheets key on.
type LayoutNode struct {
	ID uint32
	// Group is the LayoutGroup.ID the node sits in, or 0 for a node not drawn on the tree,
	// such as a keystone only a jewel grants; Orbit and OrbitIndex are then 0.
	Group      uint32
	Orbit      uint8
	OrbitIndex uint16
	Icon       string
	// MasteryInactiveIcon, MasteryActiveIcon and MasteryActiveEffect are a mastery's art
	// while none of its group is allocated, once an effect is chosen, and behind its group
	// then; empty for other nodes.
	MasteryInactiveIcon, MasteryActiveIcon, MasteryActiveEffect string
}

// ClassArt is the illustration drawn around a class's start.
type ClassArt struct {
	// Class is the class id, an index into Tree.Classes.
	Class uint32
	// Image is the illustration's art path.
	Image string
	X, Y  float64
}

// Position returns where node id is drawn: its group's centre plus its orbit's radius at its
// orbit index's angle. ok is false for a node not in the layout or not drawn.
func (l *Layout) Position(id uint32) (x, y float64, ok bool) {
	i, found := slices.BinarySearchFunc(l.Nodes, id, func(n LayoutNode, id uint32) int { return cmp.Compare(n.ID, id) })
	if !found || l.Nodes[i].Group == 0 {
		return 0, 0, false
	}
	n := l.Nodes[i]
	g, found := slices.BinarySearchFunc(l.Groups, n.Group, compareGroupID)
	if !found || int(n.Orbit) >= len(l.Orbits) || int(n.OrbitIndex) >= len(l.Orbits[n.Orbit].Angles) {
		return 0, 0, false
	}
	o := l.Orbits[n.Orbit]
	sin, cos := math.Sincos(o.Angles[n.OrbitIndex] * math.Pi / 180) //nolint:mnd // degrees to radians
	return l.Groups[g].X + o.Radius*sin, l.Groups[g].Y - o.Radius*cos, true
}

// validateLayout checks l against the tree of its variant, nil when there is none.
func validateLayout(t *Tree, l *Layout) error {
	what := "layout " + l.Variant
	if t == nil {
		return fmt.Errorf("%s: no tree of that variant: %w", what, ErrInvalid)
	}
	if l.MinX > l.MaxX || l.MinY > l.MaxY {
		return fmt.Errorf("%s: bounds min above max: %w", what, ErrInvalid)
	}
	for i, o := range l.Orbits {
		if len(o.Angles) == 0 {
			return fmt.Errorf("%s: orbit %d has no angles: %w", what, i, ErrInvalid)
		}
	}
	if err := validateLayoutGroups(what, l.Groups); err != nil {
		return err
	}
	if err := validateLayoutNodes(what, t, l); err != nil {
		return err
	}
	return validateClassArt(what, t, l.ClassArt)
}

func validateLayoutGroups(what string, groups []LayoutGroup) error {
	for i, g := range groups {
		if g.ID == 0 || (i > 0 && groups[i-1].ID >= g.ID) {
			return fmt.Errorf("%s: group %d: id 0, not sorted or repeated: %w", what, g.ID, ErrInvalid)
		}
		if g.Background != nil && g.Background.Image == "" {
			return fmt.Errorf("%s group %d: background without image: %w", what, g.ID, ErrInvalid)
		}
	}
	return nil
}

// validateLayoutNodes checks that l places exactly t's nodes, each on a group, orbit and
// orbit index that exist, and names its icon.
func validateLayoutNodes(what string, t *Tree, l *Layout) error {
	if len(l.Nodes) != len(t.Nodes) {
		return fmt.Errorf("%s: %d nodes, tree has %d: %w", what, len(l.Nodes), len(t.Nodes), ErrInvalid)
	}
	for i, n := range l.Nodes {
		if n.ID != t.Nodes[i].ID {
			return fmt.Errorf("%s: node %d not in tree, or nodes not sorted: %w", what, n.ID, ErrInvalid)
		}
		if err := validateLayoutNode(fmt.Sprintf("%s node %d", what, n.ID), l, n); err != nil {
			return err
		}
	}
	return nil
}

func validateLayoutNode(what string, l *Layout, n LayoutNode) error {
	switch {
	case n.Icon == "":
		return fmt.Errorf("%s: no icon: %w", what, ErrInvalid)
	case n.Group == 0 && (n.Orbit != 0 || n.OrbitIndex != 0):
		return fmt.Errorf("%s: orbit without group: %w", what, ErrInvalid)
	case n.Group == 0:
		return nil
	}
	if _, ok := slices.BinarySearchFunc(l.Groups, n.Group, compareGroupID); !ok {
		return fmt.Errorf("%s: group %d not in layout: %w", what, n.Group, ErrInvalid)
	}
	if int(n.Orbit) >= len(l.Orbits) || int(n.OrbitIndex) >= len(l.Orbits[n.Orbit].Angles) {
		return fmt.Errorf("%s: orbit %d index %d not in layout: %w", what, n.Orbit, n.OrbitIndex, ErrInvalid)
	}
	return nil
}

func validateClassArt(what string, t *Tree, art []ClassArt) error {
	for i, a := range art {
		if int(a.Class) >= len(t.Classes) || (i > 0 && art[i-1].Class >= a.Class) {
			return fmt.Errorf("%s: class art for class %d: not sorted, repeated or no such class: %w", what, a.Class, ErrInvalid)
		}
		if a.Image == "" {
			return fmt.Errorf("%s: class art for class %d has no image: %w", what, a.Class, ErrInvalid)
		}
	}
	return nil
}

func compareGroupID(g LayoutGroup, id uint32) int { return cmp.Compare(g.ID, id) }
