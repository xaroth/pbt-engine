// Package dataset reads and writes pbt datasets: one FlatBuffers file per table group plus a
// JSON manifest.
package dataset

//go:generate flatc --go -o internal stats.fbs characters.fbs

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// FormatVersion is the dataset format written by this package. Read rejects other versions.
const FormatVersion = 1

// ManifestName is the manifest's file name inside a dataset directory.
const ManifestName = "manifest.json"

const (
	statsFile      = "stats.fb"
	charactersFile = "characters.fb"
)

// Dataset holds the tables of one game version.
type Dataset struct {
	Manifest   Manifest
	Stats      []Stat
	Characters []Character
}

// Manifest identifies a dataset and its files.
type Manifest struct {
	Format  int    `json:"format"`
	Game    string `json:"game"`
	Version string `json:"version"`
	// Source records where the data came from, such as the game build and schema revision.
	Source map[string]string `json:"source"`
	Files  []File            `json:"files"`
}

// File is one table group file listed in the manifest.
type File struct {
	Name   string `json:"name"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
}

// Stat is a row of the stats group. Other groups refer to stats by row index.
type Stat struct {
	ID string
}

// Character is a playable class.
type Character struct {
	ID               string
	Name             string
	BaseStrength     int32
	BaseDexterity    int32
	BaseIntelligence int32
	BaseMaxLife      int32
	BaseMaxMana      int32
	// Stats are sorted by Stat, without duplicates.
	Stats []StatValue
}

// StatValue is a stat row index and its value.
type StatValue struct {
	Stat  uint32
	Value int32
}

// Write writes d into dir as a dataset and returns the manifest it wrote. Format and Files are
// filled in; the other manifest fields come from d.Manifest.
func Write(dir string, d *Dataset) (Manifest, error) {
	if err := d.validate(); err != nil {
		return Manifest{}, err
	}
	groups := []struct {
		name string
		b    []byte
	}{
		{statsFile, encodeStats(d.Stats)},
		{charactersFile, encodeCharacters(d.Characters)},
	}
	m := d.Manifest
	m.Format = FormatVersion
	m.Files = nil
	for _, g := range groups {
		if err := os.WriteFile(filepath.Join(dir, g.name), g.b, 0o644); err != nil {
			return Manifest{}, err
		}
		sum := sha256.Sum256(g.b)
		m.Files = append(m.Files, File{Name: g.name, Size: len(g.b), SHA256: hex.EncodeToString(sum[:])})
	}
	mb, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestName), append(mb, '\n'), 0o644); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// Read reads the dataset in fsys. It fails when the format version differs, a file's size or
// hash differs from the manifest, or a file does not decode.
func Read(fsys fs.FS) (*Dataset, error) {
	mb, err := fs.ReadFile(fsys, ManifestName)
	if err != nil {
		return nil, err
	}
	d := &Dataset{}
	dec := json.NewDecoder(bytes.NewReader(mb))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d.Manifest); err != nil {
		return nil, fmt.Errorf("%s: %w", ManifestName, err)
	}
	if d.Manifest.Format != FormatVersion {
		return nil, fmt.Errorf("dataset format %d, want %d", d.Manifest.Format, FormatVersion)
	}

	decoders := map[string]func([]byte) error{
		statsFile:      func(b []byte) (err error) { d.Stats, err = decodeStats(b); return err },
		charactersFile: func(b []byte) (err error) { d.Characters, err = decodeCharacters(b); return err },
	}
	for _, f := range d.Manifest.Files {
		decode, ok := decoders[f.Name]
		if !ok {
			return nil, fmt.Errorf("%s: unknown file %s", ManifestName, f.Name)
		}
		delete(decoders, f.Name)
		b, err := fs.ReadFile(fsys, f.Name)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(b)
		if len(b) != f.Size || hex.EncodeToString(sum[:]) != f.SHA256 {
			return nil, fmt.Errorf("%s: size or sha256 differs from %s", f.Name, ManifestName)
		}
		if err := decodeSafely(decode, b); err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
	}
	for name := range decoders {
		return nil, fmt.Errorf("%s: missing file %s", ManifestName, name)
	}
	if err := d.validate(); err != nil {
		return nil, err
	}
	return d, nil
}

// decodeSafely turns a panic from reading a malformed buffer into an error; the Go FlatBuffers
// runtime has no verifier.
func decodeSafely(decode func([]byte) error, b []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("malformed: %v", r)
		}
	}()
	return decode(b)
}

func (d *Dataset) validate() error {
	n := uint32(len(d.Stats))
	for _, c := range d.Characters {
		if !slices.IsSortedFunc(c.Stats, func(a, b StatValue) int { return cmp.Compare(a.Stat, b.Stat) }) {
			return fmt.Errorf("character %s: stats not sorted", c.ID)
		}
		for i, sv := range c.Stats {
			if sv.Stat >= n {
				return fmt.Errorf("character %s: stat %d out of range (%d stats)", c.ID, sv.Stat, n)
			}
			if i > 0 && c.Stats[i-1].Stat == sv.Stat {
				return fmt.Errorf("character %s: stat %d repeated", c.ID, sv.Stat)
			}
		}
	}
	return nil
}

var errIdentifier = errors.New("wrong file identifier")
