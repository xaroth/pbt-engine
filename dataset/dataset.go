// Package dataset reads and writes pbt datasets: one FlatBuffers file per table group plus a
// JSON manifest.
package dataset

//go:generate flatc --go -o internal stats.fbs characters.fbs text.fbs constants.fbs tree.fbs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
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
	if err := d.validate(); err != nil {
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

// Read reads the dataset in fsys with the display text of the given languages, such as "en".
// Text files of other languages are not read. It fails when the format version differs, the
// manifest id does not match its files, a file read differs in size or hash from the manifest,
// a file does not decode, or a language is missing.
func Read(fsys fs.FS, languages ...string) (*Dataset, error) {
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
	if d.Manifest.ID != d.Manifest.contentID() {
		return nil, fmt.Errorf("%s: id does not match game, version and files", ManifestName)
	}

	var index map[string]uint32
	required := map[string]func([]byte) error{
		statsFile: func(b []byte) (err error) {
			if d.Stats, err = decodeStats(b); err != nil {
				return err
			}
			index, err = statIndex(d.Stats)
			return err
		},
		charactersFile: func(b []byte) (err error) { d.Characters, err = decodeCharacters(b, index); return err },
		constantsFile:  func(b []byte) (err error) { d.Constants, err = decodeConstants(b); return err },
	}
	type read struct {
		name   string
		b      []byte
		decode func([]byte) error
	}
	var reads []read
	wanted := map[string]bool{}
	for _, l := range languages {
		wanted[l] = true
	}
	seen := map[string]bool{}
	for _, f := range d.Manifest.Files {
		if seen[f.Name] {
			return nil, fmt.Errorf("%s: file %s listed twice", ManifestName, f.Name)
		}
		seen[f.Name] = true
		decode, ok := required[f.Name]
		switch {
		case ok:
		case strings.HasPrefix(f.Name, treePrefix) && strings.HasSuffix(f.Name, fbSuffix):
			decode = func(b []byte) error {
				t, err := decodeTree(b, index)
				d.Trees = append(d.Trees, t)
				return err
			}
		case strings.HasPrefix(f.Name, textPrefix) && strings.HasSuffix(f.Name, fbSuffix):
			lang := strings.TrimSuffix(strings.TrimPrefix(f.Name, textPrefix), fbSuffix)
			if !wanted[lang] {
				continue
			}
			delete(wanted, lang)
			decode = func(b []byte) error {
				t, err := decodeText(b)
				d.Texts = append(d.Texts, t)
				return err
			}
		default:
			return nil, fmt.Errorf("%s: unknown file %s", ManifestName, f.Name)
		}
		delete(required, f.Name)
		b, err := fs.ReadFile(fsys, f.Name)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(b)
		if len(b) != f.Size || hex.EncodeToString(sum[:]) != f.SHA256 {
			return nil, fmt.Errorf("%s: size or sha256 differs from %s", f.Name, ManifestName)
		}
		if f.Name == statsFile {
			// Other files refer to stats by id, so stats are decoded first.
			reads = slices.Insert(reads, 0, read{f.Name, b, decode})
		} else {
			reads = append(reads, read{f.Name, b, decode})
		}
	}
	for _, name := range slices.Sorted(maps.Keys(required)) {
		return nil, fmt.Errorf("%s: missing file %s", ManifestName, name)
	}
	for _, lang := range slices.Sorted(maps.Keys(wanted)) {
		return nil, fmt.Errorf("dataset %s has no %s text", d.Manifest.Version, lang)
	}
	for _, r := range reads {
		if err := decodeSafely(r.decode, r.b); err != nil {
			return nil, fmt.Errorf("%s: %w", r.name, err)
		}
	}
	if err := d.validate(); err != nil {
		return nil, err
	}
	return d, nil
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
