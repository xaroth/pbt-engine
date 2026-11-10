// Package dataset reads and writes pbt datasets: one FlatBuffers file per table group plus a
// JSON manifest.
package dataset

//go:generate flatc --go -o internal schema/stats.fbs schema/characters.fbs schema/text.fbs schema/constants.fbs schema/tree.fbs schema/quests.fbs schema/items.fbs schema/mods.fbs

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

// Errors returned when reading datasets; match them with errors.Is.
var (
	ErrFormat = errors.New("dataset format")
	// ErrInvalid marks dataset content that fails decoding, validation or integrity checks.
	ErrInvalid      = errors.New("invalid dataset")
	ErrNotInDataset = errors.New("not in dataset")
	ErrDuplicate    = errors.New("added twice")
	ErrBadVersion   = errors.New("invalid game version")
	// ErrUnsupported marks capabilities the caller's rules do not implement.
	ErrUnsupported = errors.New("unsupported capabilities")
	errMalformed   = fmt.Errorf("malformed: %w", ErrInvalid)
)

// FormatVersion is the dataset format written by this package. Open rejects other versions.
const FormatVersion = 1

// ManifestName is the manifest's file name inside a dataset directory.
const ManifestName = "manifest.json"

const (
	statsFile      = "stats.fb"
	charactersFile = "characters.fb"
	constantsFile  = "constants.fb"
	questsFile     = "quests.fb"
	itemsFile      = "items.fb"
	modsFile       = "mods.fb"
	treePrefix     = "tree-"
	textPrefix     = "text-"
	fbSuffix       = ".fb"
)

// group is a table group file as Write writes it.
type group struct {
	name string
	b    []byte
}

// filePerm is world-readable because datasets are published files.
const filePerm = 0o644

// Write writes d into dir as a dataset and returns the manifest it wrote. Format, ID and Files
// are filled in; the other manifest fields come from d.Manifest.
func Write(dir string, d *Dataset) (Manifest, error) {
	if _, err := statIndex(d.Stats); err != nil {
		return Manifest{}, err
	}
	if err := d.validate(); err != nil {
		return Manifest{}, err
	}
	if err := d.Manifest.Capabilities.validate(); err != nil {
		return Manifest{}, err
	}
	groups := []group{
		{statsFile, encodeStats(d.Stats)},
		{charactersFile, encodeCharacters(d.Stats, d.Characters)},
		{constantsFile, encodeConstants(d.Constants)},
		{questsFile, encodeQuests(d.Stats, d.Quests)},
	}
	for _, t := range d.Trees {
		groups = append(groups, group{treePrefix + t.Variant + fbSuffix, encodeTree(d.Stats, t)})
	}
	for _, t := range d.Texts {
		groups = append(groups, group{textPrefix + t.Language + fbSuffix, encodeText(t)})
	}
	groups = append(groups, d.itemsGroup()...)
	m := d.Manifest
	m.Format = FormatVersion
	m.Files = nil
	for _, g := range groups {
		if err := os.WriteFile(filepath.Join(dir, g.name), g.b, filePerm); err != nil {
			return Manifest{}, fmt.Errorf("write %s: %w", g.name, err)
		}
		sum := sha256.Sum256(g.b)
		m.Files = append(m.Files, File{Name: g.name, Size: len(g.b), SHA256: hex.EncodeToString(sum[:])})
	}
	m.ID = m.contentID()
	mb, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return Manifest{}, fmt.Errorf("encode manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestName), append(mb, '\n'), filePerm); err != nil {
		return Manifest{}, fmt.Errorf("write manifest: %w", err)
	}
	return m, nil
}

// ReadManifest reads the manifest of the dataset in fsys. It fails when the format version
// differs, a capability is malformed, a file is listed twice, or the id does not match.
func ReadManifest(fsys fs.FS) (Manifest, error) {
	var m Manifest
	mb, err := fs.ReadFile(fsys, ManifestName)
	if err != nil {
		return m, fmt.Errorf("read manifest: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(mb))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, fmt.Errorf("%s: %w: %w", ManifestName, err, ErrInvalid)
	}
	if m.Format != FormatVersion {
		return m, fmt.Errorf("%w %d, want %d", ErrFormat, m.Format, FormatVersion)
	}
	if err := m.Capabilities.validate(); err != nil {
		return m, err
	}
	seen := map[string]bool{}
	for _, f := range m.Files {
		if seen[f.Name] {
			return m, fmt.Errorf("%s: file %s listed twice: %w", ManifestName, f.Name, ErrInvalid)
		}
		seen[f.Name] = true
	}
	if m.ID != m.contentID() {
		return m, fmt.Errorf("%s: id does not match game, version, capabilities and files: %w", ManifestName, ErrInvalid)
	}
	return m, nil
}

// contentID hashes the game, version, capabilities and file hashes, so datasets with the same
// version but different content or rules get different ids. Capabilities add input only when
// present, so a dataset without any is identified by game, version and files alone.
func (m Manifest) contentID() string {
	var s strings.Builder
	s.WriteString(m.Game + "\n" + m.Version + "\n")
	s.WriteString(m.Capabilities.idLines())
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

// decodeSafely turns a panic from reading a malformed buffer into an error; the Go FlatBuffers
// runtime has no verifier.
func decodeSafely(decode func([]byte) error, b []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", errMalformed, r)
		}
	}()
	return decode(b)
}

var errIdentifier = fmt.Errorf("wrong file identifier: %w", ErrInvalid)
