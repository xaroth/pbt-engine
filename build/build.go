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
	"slices"
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
	Nodes    []uint32 `json:"nodes"`
	Scenario Scenario `json:"scenario"`
}

// Scenario is where the character stands in the game, as far as the sheet depends on it.
type Scenario struct {
	Campaign Campaign `json:"campaign"`
	Bandits  Bandits  `json:"bandits,omitzero"`
}

// Campaign is how far the character is through the story campaign. Defeating Kitava at the
// end of act 5 and again at the end of act 10 each lowers the character's resistances.
type Campaign string

// Campaign stages.
const (
	// CampaignComplete is after defeating Kitava in act 10. It is the default, since the
	// endgame (maps) opens only then.
	CampaignComplete Campaign = "complete"
	// CampaignActs6To10 is after defeating Kitava in act 5, before act 10's Kitava.
	CampaignActs6To10 Campaign = "acts-6-10"
	// CampaignActs1To5 is before defeating Kitava in act 5.
	CampaignActs1To5 Campaign = "acts-1-5"
)

// Bandits is the outcome of the act 2 quest Deal with the Bandits. The zero value is a
// character that has not completed the quest, so it has no reward from it.
type Bandits string

// Quest outcomes.
const (
	BanditsNotCompleted Bandits = ""
	BanditsKillAll      Bandits = "kill-all"
	BanditsHelpAlira    Bandits = "help-alira"
	BanditsHelpKraityn  Bandits = "help-kraityn"
	BanditsHelpOak      Bandits = "help-oak"
)

// Parse decodes a build file. Unknown fields, another format and trailing data are errors.
// An empty variant becomes DefaultVariant and an empty campaign CampaignComplete.
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

func (b *Build) fillDefaults() {
	b.Variant = cmp.Or(b.Variant, DefaultVariant)
	b.Scenario.Campaign = cmp.Or(b.Scenario.Campaign, CampaignComplete)
}
