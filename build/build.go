// Package build holds the canonical build: what a player chose, by game id. A Build is the
// stored form; Check turns it into a Checked build whose ids exist in one dataset version.
// A build file looks like this:
//
//	{"format": 1, "game": "poe1", "version": "3.29", "class": "Metadata/Characters/Str/Str",
//	 "ascendancy": "Juggernaut", "level": 90, "nodes": [47175, 17765, 31628], "config": {"bandits": "help-oak"}}
package build

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/xaroth/pbt-engine/config"
)

// Errors Parse returns; match them with errors.Is.
var (
	ErrTrailingData = errors.New("build: data after the build")
	ErrFormat       = errors.New("build: format")
)

// Format is the build file format written by Encode. Parse rejects other formats. Until the
// first release the format may be bumped freely, and older files need not stay readable. A
// field added within a format is optional and left out when empty (omitzero), so a build that
// does not use it stays readable by an engine that predates it. A map field also needs nil for
// empty, by clearing it before encoding or with omitempty: omitzero writes {} for an empty map
// that is not nil.
const Format = 1

// DefaultVariant is the tree variant of a build that names none.
const DefaultVariant = "standard"

// Build is a character build as stored and exchanged.
type Build struct {
	Format int `json:"format"`
	// Game is the edition, such as "poe1".
	Game string `json:"game"`
	// Version is the requested game version, resolved by dataset.Registry.Resolve.
	Version string `json:"version"`
	// Variant is the tree variant, such as "standard" or "ruthless".
	Variant string `json:"variant"`
	// Class is the dataset Character.ID, such as "Metadata/Characters/Str/Str".
	Class string `json:"class"`
	// Ascendancy is an Ascendancy.ID of the class, or empty.
	Ascendancy string `json:"ascendancy,omitzero"`
	// Bloodline is an Ascendancy.ID of the tree's AlternateAscendancies, or empty. Any class
	// may take any of them.
	Bloodline string `json:"bloodline,omitzero"`
	Level     int    `json:"level"`
	// Nodes are the allocated passive node ids (hashes), including ascendancy nodes.
	Nodes []uint32 `json:"nodes"`
	// Masteries maps an allocated mastery node's id to the id of its chosen effect.
	Masteries map[uint32]uint32 `json:"masteries,omitzero"`
	// Overrides maps an allocated node's id to the id of the tattoo or runegraft put on it, a
	// NodeOverride.ID of the tree.
	Overrides map[uint32]string `json:"overrides,omitempty"`
	// Config holds config values by key name, such as {"campaign": "acts-6-10"}. Keys and
	// values are the game rules'; Check does not read them.
	Config config.Values `json:"config,omitzero"`
	// Skills are the skill groups.
	Skills Skills `json:"skills,omitzero"`
	// Items are the equipped items by slot.
	Items Items `json:"items,omitzero"`
	// Jewels maps a jewel socket node's id to the jewel in it.
	Jewels Jewels `json:"jewels,omitempty"`
}

// Parse decodes a build file. Unknown fields, another format and trailing data are errors.
// An empty variant becomes DefaultVariant.
func Parse(data []byte) (*Build, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	b := &Build{}
	if err := dec.Decode(b); err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	// More would accept a trailing } or ].
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, ErrTrailingData
	}
	if b.Format != Format {
		return nil, fmt.Errorf("%w %d, want %d", ErrFormat, b.Format, Format)
	}
	b.fillDefaults()
	return b, nil
}

// Encode writes b as an indented build file with Format and defaults set and nodes sorted.
func (b *Build) Encode() ([]byte, error) {
	c := *b
	c.Format = Format
	c.fillDefaults()
	c.Nodes = slices.Sorted(slices.Values(b.Nodes))
	if c.Nodes == nil {
		c.Nodes = []uint32{}
	}
	out, err := json.MarshalIndent(&c, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("build: encode: %w", err)
	}
	return append(out, '\n'), nil
}

// Clone returns a deep copy of b.
func (b *Build) Clone() *Build {
	c := *b
	c.Nodes = slices.Clone(b.Nodes)
	c.Masteries = maps.Clone(b.Masteries)
	c.Overrides = maps.Clone(b.Overrides)
	c.Config = b.Config.Clone()
	c.Skills = b.Skills.clone()
	c.Items = cloneItems(b.Items)
	c.Jewels = cloneItems(b.Jewels)
	return &c
}

func (b *Build) fillDefaults() {
	b.Variant = cmp.Or(b.Variant, DefaultVariant)
}
