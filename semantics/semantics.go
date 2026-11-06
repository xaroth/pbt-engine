// Package semantics says what game stat ids mean in engine terms: a table from stat id to
// engine modifier templates, each entry with a status. Tables are generated per dataset from
// Path of Building's parsed mod lines (see Generate) plus hand-written overrides, and loaded
// by the interpret stage.
package semantics

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
)

// Format is the table format written by Encode. Load rejects other formats.
const Format = 1

// Status says how far a stat's meaning is known.
type Status string

// Statuses. Only Generated and Override entries carry modifiers.
const (
	// Generated: derived from PoB's parse of every line the stat appears in.
	Generated Status = "generated"
	// Override: written by hand, with a reason; wins over generation.
	Override Status = "override"
	// Unknown: the stat feeds the sheet, or may, but no meaning could be derived.
	Unknown Status = "unknown"
	// NotSheet: PoB parses the stat into modifiers none of which the sheet reads yet.
	NotSheet Status = "not-sheet"
)

// Op is how an engine modifier combines. The names match core's ops.
type Op string

// Ops.
const (
	OpBase     Op = "base"
	OpInc      Op = "inc"
	OpMore     Op = "more"
	OpOverride Op = "override"
	OpFlag     Op = "flag"
)

// Mod is one engine modifier a stat produces: engine stat, op, and the modifier's value as
// stat value * Mul / Div. A ratio, not a float factor, keeps per-minute stats exact.
type Mod struct {
	Stat string  `json:"stat"`
	Op   Op      `json:"op"`
	Mul  float64 `json:"mul"`
	Div  float64 `json:"div"`
}

// Value returns the modifier value for a game stat value.
func (m Mod) Value(stat float64) float64 { return stat * m.Mul / m.Div }

// Entry is the meaning of one game stat id.
type Entry struct {
	Stat   string `json:"stat"`
	Status Status `json:"status"`
	Mods   []Mod  `json:"mods,omitzero"`
	// Reason explains an override or why a stat is unknown.
	Reason string `json:"reason,omitzero"`
	// Sheet is set when PoB parses at least one of the stat's lines into a modifier the sheet
	// reads, so an Unknown entry with Sheet set is a gap in the sheet.
	Sheet bool `json:"sheet,omitzero"`
	// Uses counts the standard tree nodes and mastery effects carrying the stat.
	Uses int `json:"uses,omitzero"`
}

// Table is the semantics of one dataset version. Entries are sorted by Stat.
type Table struct {
	Format  int     `json:"format"`
	Game    string  `json:"game"`
	Version string  `json:"version"`
	PoB     string  `json:"pob"`
	Entries []Entry `json:"entries"`
}

// Lookup returns the entry of a game stat id.
func (t *Table) Lookup(stat string) (Entry, bool) {
	i, ok := slices.BinarySearchFunc(t.Entries, stat, func(e Entry, s string) int { return cmp.Compare(e.Stat, s) })
	if !ok {
		return Entry{}, false
	}
	return t.Entries[i], true
}

// Load decodes a table, rejecting unknown fields, other formats, unsorted or repeated stats
// and modifiers on entries that cannot carry them.
func Load(data []byte) (*Table, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	t := &Table{}
	if err := dec.Decode(t); err != nil {
		return nil, fmt.Errorf("semantics: %w", err)
	}
	if t.Format != Format {
		return nil, fmt.Errorf("semantics: format %d, want %d", t.Format, Format)
	}
	for i, e := range t.Entries {
		if i > 0 && t.Entries[i-1].Stat >= e.Stat {
			return nil, fmt.Errorf("semantics: entry %s not sorted or repeated", e.Stat)
		}
		if err := e.check(); err != nil {
			return nil, err
		}
	}
	return t, nil
}

func (e *Entry) check() error {
	switch e.Status {
	case Generated, Override:
		for _, m := range e.Mods {
			if !slices.Contains(EngineStats, m.Stat) {
				return fmt.Errorf("semantics: %s: unknown engine stat %q", e.Stat, m.Stat)
			}
			if !(m.Div > 0) {
				return fmt.Errorf("semantics: %s: div %v, want > 0", e.Stat, m.Div)
			}
			switch m.Op {
			case OpBase, OpInc, OpMore, OpOverride, OpFlag:
			default:
				return fmt.Errorf("semantics: %s: unknown op %q", e.Stat, m.Op)
			}
		}
	case Unknown, NotSheet:
		if len(e.Mods) > 0 {
			return fmt.Errorf("semantics: %s: status %s with modifiers", e.Stat, e.Status)
		}
	default:
		return fmt.Errorf("semantics: %s: unknown status %q", e.Stat, e.Status)
	}
	return nil
}

// SheetOnly returns t without the entries that do not feed the sheet: NotSheet entries and
// Unknown entries PoB does not parse into a sheet modifier. Their reasons quote game text.
func (t *Table) SheetOnly() *Table {
	out := *t
	out.Entries = slices.DeleteFunc(slices.Clone(t.Entries), func(e Entry) bool {
		return e.Status == NotSheet || (e.Status == Unknown && !e.Sheet)
	})
	return &out
}

// Encode writes t as indented JSON.
func (t *Table) Encode() ([]byte, error) {
	out, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
