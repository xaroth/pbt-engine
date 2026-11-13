package dataset

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
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
	quests     func() ([]Quest, error)
	items      func() (*Items, error)
	mods       func() ([]Mod, error)
	skills     func() (*Skills, error)
	buffs      func() (*Buffs, error)
	monsters   func() (*Monsters, error)
	// trees, texts, names and layouts are keyed by variant and language, fixed by the manifest.
	trees   map[string]func() (*Tree, error)
	texts   map[string]func() (*Text, error)
	names   map[string]func() (*Names, error)
	layouts map[string]func() (*Layout, error)
}

type statsGroup struct {
	stats []Stat
	index map[string]uint32
}

// Open reads the manifest in fsys and reads no table group yet. It fails when ReadManifest
// does, or the manifest lists an unknown file, misses stats, characters, constants or quests,
// lists only one of the items group's two files, lists monsters without skills, or lists a
// layout without its tree.
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
		names:    map[string]func() (*Names, error){},
		layouts:  map[string]func() (*Layout, error){},
	}
	for _, f := range m.Files {
		if err := registerFile(s, f); err != nil {
			return nil, err
		}
	}
	for _, name := range []string{CharactersFile, ConstantsFile, QuestsFile, StatsFile} {
		if _, ok := s.files[name]; !ok {
			return nil, fmt.Errorf("%s: missing file %s: %w", ManifestName, name, ErrInvalid)
		}
	}
	for v := range s.layouts {
		if _, ok := s.trees[v]; !ok {
			return nil, fmt.Errorf("%s: layout %s has no tree of that variant: %w", ManifestName, v, ErrInvalid)
		}
	}
	s.stats = sync.OnceValues(s.loadStats)
	s.characters = sync.OnceValues(s.loadCharacters)
	s.constants = sync.OnceValues(s.loadConstants)
	s.quests = sync.OnceValues(s.loadQuests)
	_, hasItems := s.files[ItemsFile]
	if _, hasMods := s.files[ModsFile]; hasItems != hasMods {
		return nil, fmt.Errorf("%s: items group needs both %s and %s: %w", ManifestName, ItemsFile, ModsFile, ErrInvalid)
	}
	s.items = sync.OnceValues(s.loadItems)
	s.mods = sync.OnceValues(s.loadMods)
	s.skills = sync.OnceValues(s.loadSkills)
	s.buffs = sync.OnceValues(s.loadBuffs)
	if err := s.checkMonstersFiles(); err != nil {
		return nil, err
	}
	s.monsters = sync.OnceValues(s.loadMonsters)
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
		return out, fmt.Errorf("read %s: %w", name, err)
	}
	sum := sha256.Sum256(b)
	if len(b) != f.Size || hex.EncodeToString(sum[:]) != f.SHA256 {
		return out, fmt.Errorf("%s: size or sha256 differs from %s: %w", name, ManifestName, ErrInvalid)
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

// HasStat reports whether the stats group has stat id.
func (s *Store) HasStat(id string) (bool, error) {
	g, err := s.stats()
	_, ok := g.index[id]
	return ok && err == nil, err
}

// Characters returns the characters group, shared and read-only.
func (s *Store) Characters() ([]Character, error) { return s.characters() }

// Constants returns the edition constants group, shared and read-only.
func (s *Store) Constants() ([]Constant, error) { return s.constants() }

// Quests returns the quest rewards group, shared and read-only.
func (s *Store) Quests() ([]Quest, error) { return s.quests() }

// Tree returns the tree of one variant, such as "standard", shared and read-only.
func (s *Store) Tree(variant string) (*Tree, error) {
	load, ok := s.trees[variant]
	if !ok {
		return nil, fmt.Errorf("dataset %s has no %s tree: %w", s.manifest.Version, variant, ErrNotInDataset)
	}
	return load()
}

// Text returns the display text of one language, such as "en", shared and read-only. Its node,
// ascendancy, item, skill, buff and monster names are not checked against the groups they
// name, which it does not read; Write checks them.
func (s *Store) Text(language string) (*Text, error) {
	load, ok := s.texts[language]
	if !ok {
		return nil, fmt.Errorf("dataset %s has no %s text: %w", s.manifest.Version, language, ErrNotInDataset)
	}
	return load()
}

// registerFile records f and, for a file of a group with one file per variant or language, how
// to load it.
func registerFile(s *Store, f File) error {
	s.files[f.Name] = f
	switch {
	case isSingleFile(f.Name):
	case isGroup(f.Name, treePrefix):
		variant := groupTag(f.Name, treePrefix)
		s.trees[variant] = sync.OnceValues(func() (*Tree, error) { return s.loadTree(variant) })
	case isGroup(f.Name, layoutPrefix):
		variant := groupTag(f.Name, layoutPrefix)
		s.layouts[variant] = sync.OnceValues(func() (*Layout, error) { return s.loadLayout(variant) })
	case isGroup(f.Name, textPrefix):
		lang := groupTag(f.Name, textPrefix)
		s.texts[lang] = sync.OnceValues(func() (*Text, error) { return s.loadText(lang) })
	case isGroup(f.Name, namesPrefix):
		lang := groupTag(f.Name, namesPrefix)
		s.names[lang] = sync.OnceValues(func() (*Names, error) { return s.loadNames(lang) })
	default:
		return fmt.Errorf("%s: unknown file %s: %w", ManifestName, f.Name, ErrInvalid)
	}
	return nil
}

// isSingleFile reports whether name is the file of a group that has one file per dataset.
func isSingleFile(name string) bool {
	switch name {
	case StatsFile, CharactersFile, ConstantsFile, QuestsFile, ItemsFile, ModsFile, SkillsFile, BuffsFile, MonstersFile:
		return true
	}
	return false
}

// Needs returns the files a Store reads to load the groups in files: those and the files each
// is checked against on load, such as stats.fb and characters.fb for a tree, in manifest order.
// Files m does not list are left out, and so are the files only they would need.
func (m Manifest) Needs(files ...string) []string {
	listed := map[string]bool{}
	for _, f := range m.Files {
		listed[f.Name] = true
	}
	need := map[string]bool{}
	var add func(string)
	add = func(name string) {
		if need[name] || !listed[name] {
			return
		}
		need[name] = true
		for _, f := range loadsFirst(name) {
			add(f)
		}
	}
	for _, f := range files {
		add(f)
	}
	var out []string
	for _, f := range m.Files {
		if need[f.Name] {
			out = append(out, f.Name)
		}
	}
	return out
}

// loadsFirst returns the files the loader of name reads before it, as the load methods do.
func loadsFirst(name string) []string {
	switch {
	case name == CharactersFile, name == QuestsFile, name == SkillsFile:
		return []string{StatsFile}
	case name == ModsFile, name == BuffsFile:
		return []string{StatsFile, ItemsFile}
	case name == MonstersFile:
		return []string{StatsFile, SkillsFile}
	case isGroup(name, treePrefix):
		return []string{StatsFile, CharactersFile}
	case isGroup(name, textPrefix):
		return []string{CharactersFile}
	case isGroup(name, layoutPrefix):
		return []string{TreeFile(groupTag(name, layoutPrefix))}
	}
	return nil
}

func (s *Store) loadStats() (statsGroup, error) {
	return readGroup(s, StatsFile, func(b []byte) (statsGroup, error) {
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
	cs, err := readGroup(s, CharactersFile, func(b []byte) ([]Character, error) { return decodeCharacters(b, st.index) })
	if err != nil {
		return nil, err
	}
	return cs, validateCharacters(len(st.stats), cs)
}

func (s *Store) loadConstants() ([]Constant, error) {
	cs, err := readGroup(s, ConstantsFile, decodeConstants)
	if err != nil {
		return nil, err
	}
	return cs, validateConstants(cs)
}

func (s *Store) loadQuests() ([]Quest, error) {
	st, err := s.stats()
	if err != nil {
		return nil, err
	}
	qs, err := readGroup(s, QuestsFile, func(b []byte) ([]Quest, error) { return decodeQuests(b, st.index) })
	if err != nil {
		return nil, err
	}
	return qs, validateQuests(len(st.stats), qs)
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
	name := TreeFile(variant)
	t, err := readGroup(s, name, func(b []byte) (Tree, error) { return decodeTree(b, st.index) })
	if err != nil {
		return nil, err
	}
	if t.Variant != variant {
		return nil, fmt.Errorf("%s: holds tree variant %q: %w", name, t.Variant, ErrInvalid)
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
	name := TextFile(language)
	t, err := readGroup(s, name, decodeText)
	if err != nil {
		return nil, err
	}
	if t.Language != language {
		return nil, fmt.Errorf("%s: holds text language %q: %w", name, t.Language, ErrInvalid)
	}
	if err := validateText(chars, t); err != nil {
		return nil, err
	}
	return &t, nil
}
