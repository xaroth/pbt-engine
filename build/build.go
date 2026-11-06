// Package build holds the canonical build: what a player chose, by game id. A Build is the
// serialised form; Check turns it into a Checked build whose ids exist in one dataset.
package build

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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
	Nodes    []uint32 `json:"nodes"`
	Scenario Scenario `json:"scenario,omitzero"`
}

// Scenario holds the build's situation fields that the sheet depends on.
type Scenario struct {
	Bandit  Bandit  `json:"bandit,omitzero"`
	Penalty Penalty `json:"resistancePenalty,omitzero"`
}

// Bandit is the bandit quest reward. The zero value is killing all bandits.
type Bandit string

// Bandit rewards.
const (
	KillAll Bandit = ""
	Alira   Bandit = "alira"
	Kraityn Bandit = "kraityn"
	Oak     Bandit = "oak"
)

// Penalty is the campaign stage that sets the resistance penalty. The zero value is the end
// of the campaign.
type Penalty string

// Resistance penalty stages.
const (
	AfterAct10 Penalty = ""
	AfterAct5  Penalty = "act5"
	NoPenalty  Penalty = "none"
)

// Parse decodes a build file. Unknown fields, another format and trailing data are errors.
// An empty variant becomes DefaultVariant.
func Parse(data []byte) (*Build, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	b := &Build{}
	if err := dec.Decode(b); err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	if dec.More() {
		return nil, errors.New("build: data after the build")
	}
	if b.Format != Format {
		return nil, fmt.Errorf("build: format %d, want %d", b.Format, Format)
	}
	if b.Variant == "" {
		b.Variant = DefaultVariant
	}
	return b, nil
}

// Encode writes b as an indented build file with Format set and nodes sorted.
func (b *Build) Encode() ([]byte, error) {
	c := *b
	c.Format = Format
	c.Nodes = slices.Sorted(slices.Values(b.Nodes))
	if c.Nodes == nil {
		c.Nodes = []uint32{}
	}
	out, err := json.MarshalIndent(&c, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
