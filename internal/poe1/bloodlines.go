package poe1

import (
	"fmt"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
)

// Bloodline is an alternate ascendancy of a tree, which a character takes beside its ascendancy.
type Bloodline struct {
	// ID is the tree's id, as build.Build.Bloodline holds it.
	ID string `json:"id"`
	// Classes are the Character.IDs of the classes that may take it, in the tree's order.
	Classes []string `json:"classes"`
	// Legacy marks a bloodline the game no longer grants: the Warden, Warlock and Primalist.
	Legacy bool `json:"legacy,omitzero"`
}

// Bloodlines returns the bloodlines of the tree variant in s, in the tree's order. Any class may
// take any of them. It reads only the variant's classes and alternate ascendancies, so it loads
// neither the stats nor the text groups.
func Bloodlines(s *dataset.Store, variant string) ([]Bloodline, error) {
	classes, err := s.Classes(variant)
	if err != nil {
		return nil, fmt.Errorf("bloodlines: %w", err)
	}
	alternates, err := s.AlternateAscendancies(variant)
	if err != nil {
		return nil, fmt.Errorf("bloodlines: %w", err)
	}
	ids := make([]string, len(classes))
	for i, c := range classes {
		ids[i] = c.Character
	}
	out := make([]Bloodline, len(alternates))
	for i, a := range alternates {
		out[i] = Bloodline{ID: a.ID, Classes: slices.Clone(ids), Legacy: legacyBloodline(a.ID)}
	}
	return out, nil
}

// legacyBloodline reports whether the game no longer grants bloodline id, which Path of
// Building hides.
func legacyBloodline(id string) bool {
	switch id {
	case "Warden", "Warlock", "Primalist":
		return true
	}
	return false
}
