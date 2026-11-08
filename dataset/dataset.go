// Package dataset reads and writes pbt datasets: one FlatBuffers file per table group plus a
// JSON manifest.
package dataset

//go:generate flatc --go -o internal schema/stats.fbs schema/characters.fbs schema/text.fbs schema/constants.fbs schema/tree.fbs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// FormatVersion is the dataset format written by this package. Read rejects other versions.
const FormatVersion = 4

// ManifestName is the manifest's file name inside a dataset directory.
const ManifestName = "manifest.json"

const (
	statsFile      = "stats.fb"
	charactersFile = "characters.fb"
	constantsFile  = "constants.fb"
	treePrefix     = "tree-"
	textPrefix     = "text-"
	fbSuffix       = ".fb"
)

// Write writes d into dir as a dataset and returns the manifest it wrote. Format, ID and Files
// are filled in; the other manifest fields come from d.Manifest.
func Write(dir string, d *Dataset) (Manifest, error) {
	if _, err := statIndex(d.Stats); err != nil {
		return Manifest{}, err
	}
	if err := d.validate(true); err != nil {
		return Manifest{}, err
	}
	type group struct {
		name string
		b    []byte
	}
	groups := []group{
		{statsFile, encodeStats(d.Stats)},
		{charactersFile, encodeCharacters(d.Stats, d.Characters)},
		{constantsFile, encodeConstants(d.Constants)},
	}
	for _, t := range d.Trees {
		groups = append(groups, group{treePrefix + t.Variant + fbSuffix, encodeTree(d.Stats, t)})
	}
	for _, t := range d.Texts {
		groups = append(groups, group{textPrefix + t.Language + fbSuffix, encodeText(t)})
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
	m.ID = m.contentID()
	mb, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestName), append(mb, '\n'), 0o644); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// Options selects the groups Read loads besides stats, characters and constants, which it
// always loads. ReadManifest lists what a dataset holds.
type Options struct {
	// Trees are tree variants, such as "standard".
	Trees []string
	// Languages are display text languages, such as "en".
	Languages []string
}

// ReadManifest reads the manifest of the dataset in fsys. It fails when the format version
// differs, a file is listed twice, or the id does not match the files.
func ReadManifest(fsys fs.FS) (Manifest, error) {
	var m Manifest
	mb, err := fs.ReadFile(fsys, ManifestName)
	if err != nil {
		return m, err
	}
	dec := json.NewDecoder(bytes.NewReader(mb))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, fmt.Errorf("%s: %w", ManifestName, err)
	}
	if m.Format != FormatVersion {
		return m, fmt.Errorf("dataset format %d, want %d", m.Format, FormatVersion)
	}
	seen := map[string]bool{}
	for _, f := range m.Files {
		if seen[f.Name] {
			return m, fmt.Errorf("%s: file %s listed twice", ManifestName, f.Name)
		}
		seen[f.Name] = true
	}
	if m.ID != m.contentID() {
		return m, fmt.Errorf("%s: id does not match game, version and files", ManifestName)
	}
	return m, nil
}

// contentID hashes the game, version and file hashes, so datasets with the same version but
// different content get different ids.
func (m Manifest) contentID() string {
	var s strings.Builder
	s.WriteString(m.Game + "\n" + m.Version + "\n")
	for _, f := range slices.SortedFunc(slices.Values(m.Files), func(a, b File) int { return strings.Compare(a.Name, b.Name) }) {
		s.WriteString(f.Name + " " + f.SHA256 + "\n")
	}
	sum := sha256.Sum256([]byte(s.String()))
	return hex.EncodeToString(sum[:])
}

// Trees returns the tree variants in the dataset, in manifest order.
func (m Manifest) Trees() []string { return m.tagged(treePrefix) }

// Languages returns the text languages in the dataset, in manifest order.
func (m Manifest) Languages() []string { return m.tagged(textPrefix) }

func (m Manifest) tagged(prefix string) []string {
	var out []string
	for _, f := range m.Files {
		if isGroup(f.Name, prefix) {
			out = append(out, groupTag(f.Name, prefix))
		}
	}
	return out
}

// Read reads the dataset in fsys with the groups o selects; files not selected are not read.
// It is Open followed by Store.Dataset, and fails when either does. The Store is not kept, so
// the Dataset belongs to the caller.
func Read(fsys fs.FS, o Options) (*Dataset, error) {
	s, err := Open(fsys)
	if err != nil {
		return nil, err
	}
	return s.Dataset(o)
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

var errIdentifier = errors.New("wrong file identifier")
