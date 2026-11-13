// Package pobxml reads Path of Building build files (PoE1) into their raw form: what the file
// says, in PoB's terms, with no game data applied. pob.FromXML turns it into a build.
//
// The elements read, as PoB writes them:
//
//	<PathOfBuilding>
//	  <Build level className ascendClassName bandit pantheonMajorGod pantheonMinorGod
//	         mainSocketGroup/>                1-based index of the main skill's group; older
//	                                          files: mainSkillIndex
//	  <Tree activeSpec>                       1-based index of the active spec
//	    <Spec title treeVersion classId ascendClassId secondaryAscendClassId
//	          nodes="id,id"                   allocated nodes; cluster jewel nodes are above 65535
//	          masteryEffects="{node,effect},{node,effect}">
//	      <Sockets><Socket nodeId itemId/></Sockets>         itemId 0 is an empty socket
//	      <Overrides><Override nodeId dn/></Overrides>       tattoos and other node overrides
//	    </Spec>
//	  </Tree>
//	  <Config activeConfigSet>
//	    <ConfigSet id><Input name number|boolean|string/></ConfigSet>
//	    <Input .../>                          older files: inputs without sets
//	  </Config>
//	  <Items activeItemSet>
//	    <Item id>item text</Item>
//	    <ItemSet id><Slot name itemId/></ItemSet>    itemId 0 is an empty slot
//	    <Slot .../>                           older files: slots without sets
//	  </Items>
//	  <Skills activeSkillSet>
//	    <SkillSet id>
//	      <Skill label enabled slot source    source: the item or tree node granting the group
//	             mainActiveSkill>             1-based, among the group's active skills
//	        <Gem nameSpec gemId skillId variantId level quality enabled enableGlobal1 enableGlobal2/>
//	      </Skill>
//	    </SkillSet>
//	    <Skill .../>                          older files: groups without sets
//	  </Skills>
//	</PathOfBuilding>
//
// Other elements and attributes are ignored. Written from the build files in pbt-conformance,
// with pbt-research docs/formats/build-xml.md (notes from PoB's source) as insight.
package pobxml

import (
	"cmp"
	"encoding/xml"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Errors Parse returns; match them with errors.Is.
var (
	ErrNotPoB    = errors.New("pobxml: not a Path of Building build")
	ErrPoE2      = errors.New("pobxml: Path of Exile 2 build")
	ErrMissing   = errors.New("pobxml: missing")
	ErrMalformed = errors.New("pobxml: malformed")
)

// Roots of PoE1 and PoE2 build files.
const (
	rootPoE1 = "PathOfBuilding"
	rootPoE2 = "PathOfBuilding2"
)

// File is a PoB build file in PoB's terms.
type File struct {
	Build Build
	// Specs are the passive trees, in file order; Active indexes the active one.
	Specs  []Spec
	Active int
	// Config is the active config set's inputs, or the inputs of a file without sets.
	Config []Input
	Items  []Item
	// Equipped are the active item set's slots that hold an item, or the slots of a file
	// without sets, in file order.
	Equipped []Equipped
	// Skills are the active skill set's groups, or the groups of a file without sets.
	Skills []SkillGroup
}

// Build is the <Build> element.
type Build struct {
	Level                        int
	Class, Ascendancy, Bandit    string
	PantheonMajor, PantheonMinor string
	// MainSocketGroup is the 1-based index of the main skill's group in Skills; 0 when unset.
	MainSocketGroup int
}

// Spec is one passive tree.
type Spec struct {
	Title, TreeVersion                             string
	ClassID, AscendClassID, SecondaryAscendClassID int
	// Nodes are sorted, without duplicates.
	Nodes     []uint32
	Masteries []Mastery
	Sockets   []Socket
	Overrides []Override
}

// Mastery is a chosen mastery effect.
type Mastery struct{ Node, Effect uint32 }

// Socket is a jewel socket node and the item in it; Item 0 is empty.
type Socket struct {
	Node uint32
	Item int
}

// Override replaces a node, such as a tattoo; Name is PoB's display name.
type Override struct {
	Node uint32
	Name string
}

// Input is a config input; Type is the attribute holding the value: number, boolean or string.
type Input struct{ Name, Type, Value string }

// Item is an item; Name is the first line of its text after the rarity.
type Item struct {
	ID         int
	Name, Text string
}

// Equipped is an item in an item slot, by PoB's slot name, such as "Ring 1" or "Flask 2".
type Equipped struct {
	Slot string
	Item int
}

// SkillGroup is a socket group.
type SkillGroup struct {
	Label, Slot, Source string
	Enabled             bool
	// MainActiveSkill is the 1-based index of the main skill among the group's active skills;
	// 0 when unset or not a number, such as the "nil" poe.ninja writes.
	MainActiveSkill int
	Gems            []Gem
}

// Gem is a gem in a group. GemID is the gem's metadata path, SkillID its granted effect,
// VariantID its gem effect; older files have only the name. EnableGlobal1 and EnableGlobal2
// toggle a Vaal gem's two skills.
type Gem struct {
	Name, GemID, SkillID, VariantID       string
	Level, Quality                        int
	Enabled, EnableGlobal1, EnableGlobal2 bool
}

type xmlFile struct {
	XMLName xml.Name `xml:""`
	Build   *struct {
		Level         int    `xml:"level,attr"`
		Class         string `xml:"className,attr"`
		Ascendancy    string `xml:"ascendClassName,attr"`
		Bandit        string `xml:"bandit,attr"`
		PantheonMajor string `xml:"pantheonMajorGod,attr"`
		PantheonMinor string `xml:"pantheonMinorGod,attr"`
		MainGroup     string `xml:"mainSocketGroup,attr"`
		MainSkill     string `xml:"mainSkillIndex,attr"`
	} `xml:"Build"`
	Tree *struct {
		Active int       `xml:"activeSpec,attr"`
		Specs  []xmlSpec `xml:"Spec"`
	} `xml:"Tree"`
	Config struct {
		Active string     `xml:"activeConfigSet,attr"`
		Inputs []xmlInput `xml:"Input"`
		Sets   []struct {
			ID     string     `xml:"id,attr"`
			Inputs []xmlInput `xml:"Input"`
		} `xml:"ConfigSet"`
	} `xml:"Config"`
	Items struct {
		Active string `xml:"activeItemSet,attr"`
		Items  []struct {
			ID   int    `xml:"id,attr"`
			Text string `xml:",chardata"`
		} `xml:"Item"`
		Slots []xmlSlot `xml:"Slot"`
		Sets  []struct {
			ID    string    `xml:"id,attr"`
			Slots []xmlSlot `xml:"Slot"`
		} `xml:"ItemSet"`
	} `xml:"Items"`
	Skills struct {
		Active string     `xml:"activeSkillSet,attr"`
		Groups []xmlSkill `xml:"Skill"`
		Sets   []struct {
			ID     string     `xml:"id,attr"`
			Groups []xmlSkill `xml:"Skill"`
		} `xml:"SkillSet"`
	} `xml:"Skills"`
}

type xmlSpec struct {
	Title          string `xml:"title,attr"`
	TreeVersion    string `xml:"treeVersion,attr"`
	ClassID        int    `xml:"classId,attr"`
	AscendClassID  int    `xml:"ascendClassId,attr"`
	Secondary      int    `xml:"secondaryAscendClassId,attr"`
	Nodes          string `xml:"nodes,attr"`
	MasteryEffects string `xml:"masteryEffects,attr"`
	Sockets        []struct {
		Node uint32 `xml:"nodeId,attr"`
		Item int    `xml:"itemId,attr"`
	} `xml:"Sockets>Socket"`
	Overrides []struct {
		Node uint32 `xml:"nodeId,attr"`
		Name string `xml:"dn,attr"`
	} `xml:"Overrides>Override"`
}

type xmlSlot struct {
	Name string `xml:"name,attr"`
	Item int    `xml:"itemId,attr"`
}

type xmlInput struct {
	Name    string  `xml:"name,attr"`
	Number  *string `xml:"number,attr"`
	Boolean *string `xml:"boolean,attr"`
	String  *string `xml:"string,attr"`
}

type xmlSkill struct {
	Label      string `xml:"label,attr"`
	Enabled    string `xml:"enabled,attr"`
	Slot       string `xml:"slot,attr"`
	Source     string `xml:"source,attr"`
	MainActive string `xml:"mainActiveSkill,attr"`
	Gems       []struct {
		Name          string `xml:"nameSpec,attr"`
		GemID         string `xml:"gemId,attr"`
		SkillID       string `xml:"skillId,attr"`
		VariantID     string `xml:"variantId,attr"`
		Level         string `xml:"level,attr"`
		Quality       string `xml:"quality,attr"`
		Enabled       string `xml:"enabled,attr"`
		EnableGlobal1 string `xml:"enableGlobal1,attr"`
		EnableGlobal2 string `xml:"enableGlobal2,attr"`
	} `xml:"Gem"`
}

// Parse reads a PoE1 build file. It fails on XML it cannot decode, another root (ErrNotPoB,
// or ErrPoE2 for a PoE2 build), no <Build> or no active spec (ErrMissing), and node or mastery
// lists, gem levels and quality, and group indexes that are not numbers (ErrMalformed).
func Parse(data []byte) (*File, error) {
	var x xmlFile
	if err := xml.Unmarshal(data, &x); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotPoB, err)
	}
	if err := x.check(); err != nil {
		return nil, err
	}
	f := &File{
		Build: Build{
			Level: x.Build.Level, Class: x.Build.Class, Ascendancy: x.Build.Ascendancy, Bandit: x.Build.Bandit,
			PantheonMajor: x.Build.PantheonMajor, PantheonMinor: x.Build.PantheonMinor,
		},
		Active: max(x.Tree.Active, 1) - 1,
		Config: inputs(x),
	}
	var err error
	if f.Build.MainSocketGroup, err = number("mainSocketGroup", cmp.Or(x.Build.MainGroup, x.Build.MainSkill)); err != nil {
		return nil, err
	}
	if f.Skills, err = skills(x); err != nil {
		return nil, err
	}
	if f.Active >= len(x.Tree.Specs) {
		return nil, fmt.Errorf("%w: active spec %d of %d", ErrMissing, x.Tree.Active, len(x.Tree.Specs))
	}
	for _, xs := range x.Tree.Specs {
		s, err := spec(xs)
		if err != nil {
			return nil, err
		}
		f.Specs = append(f.Specs, s)
	}
	for _, it := range x.Items.Items {
		f.Items = append(f.Items, Item{ID: it.ID, Name: itemName(it.Text), Text: it.Text})
	}
	f.Equipped = equipped(x)
	return f, nil
}

// check rejects other roots and files without <Build> or a spec.
func (x *xmlFile) check() error {
	switch x.XMLName.Local {
	case rootPoE1:
	case rootPoE2:
		return ErrPoE2
	default:
		return fmt.Errorf("%w: root <%s>", ErrNotPoB, x.XMLName.Local)
	}
	if x.Build == nil {
		return fmt.Errorf("%w: <Build>", ErrMissing)
	}
	if x.Tree == nil || len(x.Tree.Specs) == 0 {
		return fmt.Errorf("%w: <Tree><Spec>", ErrMissing)
	}
	return nil
}

func spec(x xmlSpec) (Spec, error) {
	s := Spec{
		Title: x.Title, TreeVersion: x.TreeVersion,
		ClassID: x.ClassID, AscendClassID: x.AscendClassID, SecondaryAscendClassID: x.Secondary,
	}
	var err error
	if s.Nodes, err = nodes(x.Nodes); err != nil {
		return s, err
	}
	if s.Masteries, err = masteries(x.MasteryEffects); err != nil {
		return s, err
	}
	for _, so := range x.Sockets {
		s.Sockets = append(s.Sockets, Socket{Node: so.Node, Item: so.Item})
	}
	for _, o := range x.Overrides {
		s.Overrides = append(s.Overrides, Override{Node: o.Node, Name: o.Name})
	}
	return s, nil
}

// nodes reads "id,id,..."; empty fields are skipped.
func nodes(list string) ([]uint32, error) {
	var out []uint32
	for f := range strings.SplitSeq(list, ",") {
		if f = strings.TrimSpace(f); f == "" {
			continue
		}
		n, err := strconv.ParseUint(f, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("%w: node %q", ErrMalformed, f)
		}
		out = append(out, uint32(n))
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

// masteries reads "{node,effect},{node,effect}".
func masteries(list string) ([]Mastery, error) {
	var out []Mastery
	for pair := range strings.SplitSeq(list, "},") {
		pair = strings.Trim(strings.TrimSpace(pair), "{}")
		if pair == "" {
			continue
		}
		n, e, ok := strings.Cut(pair, ",")
		node, err1 := strconv.ParseUint(strings.TrimSpace(n), 10, 32)
		effect, err2 := strconv.ParseUint(strings.TrimSpace(e), 10, 32)
		if !ok || err1 != nil || err2 != nil {
			return nil, fmt.Errorf("%w: mastery effect %q", ErrMalformed, pair)
		}
		out = append(out, Mastery{Node: uint32(node), Effect: uint32(effect)})
	}
	return out, nil
}

// inputs are the active config set's, the first set's when none is active, or the flat list.
func inputs(x xmlFile) []Input {
	list := x.Config.Inputs
	if len(x.Config.Sets) > 0 {
		list = x.Config.Sets[0].Inputs
		for _, s := range x.Config.Sets {
			if s.ID == x.Config.Active {
				list = s.Inputs
			}
		}
	}
	out := make([]Input, 0, len(list))
	for _, in := range list {
		switch {
		case in.Number != nil:
			out = append(out, Input{Name: in.Name, Type: "number", Value: *in.Number})
		case in.Boolean != nil:
			out = append(out, Input{Name: in.Name, Type: "boolean", Value: *in.Boolean})
		case in.String != nil:
			out = append(out, Input{Name: in.Name, Type: "string", Value: *in.String})
		default:
			out = append(out, Input{Name: in.Name})
		}
	}
	return out
}

// skills are the active skill set's groups, the first set's when none is active, or the flat
// list. Levels and quality that are not integers are ErrMalformed.
func skills(x xmlFile) ([]SkillGroup, error) {
	list := x.Skills.Groups
	if len(x.Skills.Sets) > 0 {
		list = x.Skills.Sets[0].Groups
		for _, s := range x.Skills.Sets {
			if s.ID == x.Skills.Active {
				list = s.Groups
			}
		}
	}
	out := make([]SkillGroup, 0, len(list))
	for _, g := range list {
		sg := SkillGroup{Label: g.Label, Slot: g.Slot, Source: g.Source, Enabled: flag(g.Enabled)}
		// PoB takes a value that is not a number as unset, so it is not ErrMalformed.
		if n, err := strconv.Atoi(g.MainActive); err == nil {
			sg.MainActiveSkill = n
		}
		var err error
		for _, xg := range g.Gems {
			gem := Gem{
				Name: xg.Name, GemID: xg.GemID, SkillID: xg.SkillID, VariantID: xg.VariantID,
				Enabled: flag(xg.Enabled), EnableGlobal1: flag(xg.EnableGlobal1), EnableGlobal2: flag(xg.EnableGlobal2),
			}
			if gem.Level, err = number("gem level", xg.Level); err != nil {
				return nil, err
			}
			if gem.Quality, err = number("gem quality", xg.Quality); err != nil {
				return nil, err
			}
			sg.Gems = append(sg.Gems, gem)
		}
		out = append(out, sg)
	}
	return out, nil
}

// equipped are the slots holding an item of the active item set, the first set's when none is
// active, or of the flat list.
func equipped(x xmlFile) []Equipped {
	list := x.Items.Slots
	if len(x.Items.Sets) > 0 {
		list = x.Items.Sets[0].Slots
		for _, s := range x.Items.Sets {
			if s.ID == x.Items.Active {
				list = s.Slots
			}
		}
	}
	var out []Equipped
	for _, s := range list {
		if s.Item != 0 {
			out = append(out, Equipped{Slot: s.Name, Item: s.Item})
		}
	}
	return out
}

// flag reads a boolean attribute that is true unless it says false.
func flag(s string) bool { return s != "false" }

// number reads an integer attribute; empty is 0.
func number(what, s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%w: %s %q", ErrMalformed, what, s)
	}
	return n, nil
}

// itemName is the first non-empty line of an item's text after its "Rarity:" line.
func itemName(text string) string {
	for line := range strings.Lines(text) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Rarity:") {
			continue
		}
		return line
	}
	return ""
}
