// Package dataset reads and writes pbt datasets: one FlatBuffers file per table group plus a
// JSON manifest.
package dataset

//go:generate flatc --go -o internal stats.fbs characters.fbs text.fbs constants.fbs tree.fbs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// FormatVersion is the dataset format written by this package. Read rejects other versions.
const FormatVersion = 3

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

// Write writes d into dir as a dataset and returns the manifest it wrote. Format and Files are
// filled in; the other manifest fields come from d.Manifest.
func Write(dir string, d *Dataset) (Manifest, error) {
	if err := d.validate(); err != nil {
		return Manifest{}, err
	}
	type group struct {
		name string
		b    []byte
	}
	groups := []group{
		{statsFile, encodeStats(d.Stats)},
		{charactersFile, encodeCharacters(d.Characters)},
		{constantsFile, encodeConstants(d.Constants)},
	}
	for _, t := range d.Trees {
		groups = append(groups, group{treePrefix + t.Variant + fbSuffix, encodeTree(t)})
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
	mb, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestName), append(mb, '\n'), 0o644); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// Read reads the dataset in fsys with the display text of the given languages, such as "en".
// Text files of other languages are not read. It fails when the format version differs, a
// file read differs in size or hash from the manifest, a file does not decode, or a language
// is missing.
func Read(fsys fs.FS, languages ...string) (*Dataset, error) {
	v, err := Open(fsys)
	if err != nil {
		return nil, err
	}
	return v.Dataset(languages...)
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
