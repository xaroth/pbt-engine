package dataset

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"sync"
)

// Store holds the table groups of one dataset opened from its manifest. Each group is read,
// checked against the groups it refers to and decoded on first use, then shared. A Store is
// safe for concurrent use. Everything it returns, from the getters and from Dataset, is shared
// with every other caller and read-only: sorting or editing a returned slice, tree or node
// changes it for all of them and races with concurrent readers.
//
// A group that fails to load returns the same error for the life of the Store, since
// sync.OnceValues keeps the first result. On a local fs.FS a failed read or hash check does not
// fix itself, so retrying would not help; an fs.FS that fetches over a network needs its own
// retry, or the caller opens a new Store.
type Store struct {
	manifest Manifest
	fsys     fs.FS
	files    map[string]File

	stats      func() (statsGroup, error)
	characters func() ([]Character, error)
	constants  func() ([]Constant, error)
	// trees and texts are keyed by variant and language, fixed by the manifest.
	trees map[string]func() (*Tree, error)
	texts map[string]func() (*Text, error)
}

type statsGroup struct {
	stats []Stat
	index map[string]uint32
}

// Open reads the manifest in fsys and reads no table group yet. It fails when ReadManifest
// does, or the manifest lists an unknown file or misses stats, characters or constants.
func Open(fsys fs.FS) (*Store, error) {
	m, err := ReadManifest(fsys)
	if err != nil {
		return nil, err
	}
	s := &Store{
		manifest: m,
		fsys:     fsys,
		files:    make(map[string]File, len(m.Files)),
		trees:    map[string]func() (*Tree, error){},
		texts:    map[string]func() (*Text, error){},
	}
	for _, f := range m.Files {
		s.files[f.Name] = f
		switch {
		case f.Name == statsFile || f.Name == charactersFile || f.Name == constantsFile:
		case isGroup(f.Name, treePrefix):
			variant := groupTag(f.Name, treePrefix)
			s.trees[variant] = sync.OnceValues(func() (*Tree, error) { return s.loadTree(variant) })
		case isGroup(f.Name, textPrefix):
			lang := groupTag(f.Name, textPrefix)
			s.texts[lang] = sync.OnceValues(func() (*Text, error) { return s.loadText(lang) })
		default:
			return nil, fmt.Errorf("%s: unknown file %s", ManifestName, f.Name)
		}
	}
	for _, name := range []string{charactersFile, constantsFile, statsFile} {
		if _, ok := s.files[name]; !ok {
			return nil, fmt.Errorf("%s: missing file %s", ManifestName, name)
		}
	}
	s.stats = sync.OnceValues(s.loadStats)
	s.characters = sync.OnceValues(s.loadCharacters)
	s.constants = sync.OnceValues(s.loadConstants)
	return s, nil
}

func isGroup(name, prefix string) bool {
	return strings.HasPrefix(name, prefix) && strings.HasSuffix(name, fbSuffix)
}

func groupTag(name, prefix string) string {
	return strings.TrimSuffix(strings.TrimPrefix(name, prefix), fbSuffix)
}

// readGroup reads a file listed in the manifest, checks its size and hash, and decodes it.
func readGroup[T any](s *Store, name string, decode func([]byte) (T, error)) (T, error) {
	var out T
	f := s.files[name]
	b, err := fs.ReadFile(s.fsys, name)
	if err != nil {
		return out, err
	}
	sum := sha256.Sum256(b)
	if len(b) != f.Size || hex.EncodeToString(sum[:]) != f.SHA256 {
		return out, fmt.Errorf("%s: size or sha256 differs from %s", name, ManifestName)
	}
	err = decodeSafely(func(b []byte) (err error) { out, err = decode(b); return err }, b)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

// Manifest returns the dataset's manifest.
func (s *Store) Manifest() Manifest { return s.manifest }

// Stats returns the stats group, shared and read-only.
func (s *Store) Stats() ([]Stat, error) {
	g, err := s.stats()
	return g.stats, err
}

// Characters returns the characters group, shared and read-only.
func (s *Store) Characters() ([]Character, error) { return s.characters() }

// Constants returns the edition constants group, shared and read-only.
func (s *Store) Constants() ([]Constant, error) { return s.constants() }

// Tree returns the tree of one variant, such as "standard", shared and read-only.
func (s *Store) Tree(variant string) (*Tree, error) {
	load, ok := s.trees[variant]
	if !ok {
		return nil, fmt.Errorf("dataset %s has no %s tree", s.manifest.Version, variant)
	}
	return load()
}

// Text returns the display text of one language, such as "en", shared and read-only. Its node and ascendancy names
// are not checked against trees, which it does not read; Dataset checks them against the trees
// it holds.
func (s *Store) Text(language string) (*Text, error) {
	load, ok := s.texts[language]
	if !ok {
		return nil, fmt.Errorf("dataset %s has no %s text", s.manifest.Version, language)
	}
	return load()
}

// Dataset returns stats, characters, constants and the trees and languages o selects, in
// manifest order. It fails when a selected tree or language is missing or a group fails to
// load. Text names of nodes are checked when o selects every tree, names of ascendancies
// against the selected trees. The Dataset shares its tables with s and every other caller, so
// it is read-only; Read gives a Dataset the caller owns.
func (s *Store) Dataset(o Options) (*Dataset, error) {
	for _, variant := range o.Trees {
		if _, ok := s.trees[variant]; !ok {
			return nil, fmt.Errorf("dataset %s has no %s tree", s.manifest.Version, variant)
		}
	}
	for _, lang := range o.Languages {
		if _, ok := s.texts[lang]; !ok {
			return nil, fmt.Errorf("dataset %s has no %s text", s.manifest.Version, lang)
		}
	}
	d := &Dataset{Manifest: s.manifest}
	var err error
	if d.Stats, err = s.Stats(); err != nil {
		return nil, err
	}
	if d.Characters, err = s.Characters(); err != nil {
		return nil, err
	}
	if d.Constants, err = s.Constants(); err != nil {
		return nil, err
	}
	allTrees := true
	for _, variant := range s.manifest.Trees() {
		if !slices.Contains(o.Trees, variant) {
			allTrees = false
			continue
		}
		t, err := s.Tree(variant)
		if err != nil {
			return nil, err
		}
		d.Trees = append(d.Trees, *t)
	}
	for _, lang := range s.manifest.Languages() {
		if !slices.Contains(o.Languages, lang) {
			continue
		}
		t, err := s.Text(lang)
		if err != nil {
			return nil, err
		}
		d.Texts = append(d.Texts, *t)
	}
	if err := validateNames(d.Texts, d.Trees, allTrees); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Store) loadStats() (statsGroup, error) {
	return readGroup(s, statsFile, func(b []byte) (statsGroup, error) {
		stats, err := decodeStats(b)
		if err != nil {
			return statsGroup{}, err
		}
		index, err := statIndex(stats)
		return statsGroup{stats, index}, err
	})
}

func (s *Store) loadCharacters() ([]Character, error) {
	st, err := s.stats()
	if err != nil {
		return nil, err
	}
	cs, err := readGroup(s, charactersFile, func(b []byte) ([]Character, error) { return decodeCharacters(b, st.index) })
	if err != nil {
		return nil, err
	}
	return cs, validateCharacters(len(st.stats), cs)
}

func (s *Store) loadConstants() ([]Constant, error) {
	cs, err := readGroup(s, constantsFile, decodeConstants)
	if err != nil {
		return nil, err
	}
	return cs, validateConstants(cs)
}

func (s *Store) loadTree(variant string) (*Tree, error) {
	st, err := s.stats()
	if err != nil {
		return nil, err
	}
	chars, err := s.characters()
	if err != nil {
		return nil, err
	}
	name := treePrefix + variant + fbSuffix
	t, err := readGroup(s, name, func(b []byte) (Tree, error) { return decodeTree(b, st.index) })
	if err != nil {
		return nil, err
	}
	if t.Variant != variant {
		return nil, fmt.Errorf("%s: holds tree variant %q", name, t.Variant)
	}
	if err := validateTree(len(st.stats), chars, t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) loadText(language string) (*Text, error) {
	chars, err := s.characters()
	if err != nil {
		return nil, err
	}
	name := textPrefix + language + fbSuffix
	t, err := readGroup(s, name, decodeText)
	if err != nil {
		return nil, err
	}
	if t.Language != language {
		return nil, fmt.Errorf("%s: holds text language %q", name, t.Language)
	}
	if err := validateText(chars, t); err != nil {
		return nil, err
	}
	return &t, nil
}
