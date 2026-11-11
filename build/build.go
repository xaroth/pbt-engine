// Package build holds the canonical build: what a player chose, by game id. A Build is the
// stored form; Check turns it into a Checked build whose ids exist in one dataset version.
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

// Format is the build file format written by Encode. Parse rejects other formats.
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
	Level      int    `json:"level"`
	// Nodes are the allocated passive node ids (hashes), including ascendancy nodes.
	Nodes []uint32 `json:"nodes"`
	// Masteries maps an allocated mastery node's id to the id of its chosen effect.
	Masteries map[uint32]uint32 `json:"masteries,omitzero"`
	// Config holds config values by key name, such as {"campaign": "acts-6-10"}. Keys and
	// values are the game rules'; Check does not read them.
	Config config.Values `json:"config,omitzero"`
	// Skills are the skill groups.
	Skills Skills `json:"skills,omitzero"`
	// Items are the equipped items by slot.
	Items Items `json:"items,omitzero"`
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
	c.Config = b.Config.Clone()
	c.Skills = b.Skills.clone()
	c.Items = b.Items.clone()
	return &c
}

func (b *Build) fillDefaults() {
	b.Variant = cmp.Or(b.Variant, DefaultVariant)
}
