package dataset

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"sync"
)

// Version is one dataset opened from its manifest. Table groups are read, checked and decoded
// on first use, then shared; a Version is safe for concurrent use and its results must not be
// modified.
type Version struct {
	manifest Manifest
	fsys     fs.FS
	files    map[string]File
	variants []string
	langs    []string

	stats      func() ([]Stat, error)
	characters func() ([]Character, error)
	constants  func() ([]Constant, error)
	allTrees   func() ([]Tree, error)

	mu    sync.Mutex
	trees map[string]func() (*Tree, error)
	texts map[string]func() (*Text, error)
}

// Open reads and checks the manifest in fsys. No table group is read yet.
func Open(fsys fs.FS) (*Version, error) {
	mb, err := fs.ReadFile(fsys, ManifestName)
	if err != nil {
		return nil, err
	}
	v := &Version{fsys: fsys, files: map[string]File{}}
	dec := json.NewDecoder(bytes.NewReader(mb))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v.manifest); err != nil {
		return nil, fmt.Errorf("%s: %w", ManifestName, err)
	}
	m := &v.manifest
	if m.Format != FormatVersion {
		return nil, fmt.Errorf("dataset format %d, want %d", m.Format, FormatVersion)
	}
	for _, f := range m.Files {
		if _, dup := v.files[f.Name]; dup {
			return nil, fmt.Errorf("%s: file %s listed twice", ManifestName, f.Name)
		}
		v.files[f.Name] = f
		switch {
		case f.Name == statsFile || f.Name == charactersFile || f.Name == constantsFile:
		case isGroup(f.Name, treePrefix):
			v.variants = append(v.variants, groupTag(f.Name, treePrefix))
		case isGroup(f.Name, textPrefix):
			v.langs = append(v.langs, groupTag(f.Name, textPrefix))
		default:
			return nil, fmt.Errorf("%s: unknown file %s", ManifestName, f.Name)
		}
	}
	for _, name := range []string{statsFile, charactersFile, constantsFile} {
		if _, ok := v.files[name]; !ok {
			return nil, fmt.Errorf("%s: missing file %s", ManifestName, name)
		}
	}
	v.stats = sync.OnceValues(func() ([]Stat, error) { return readGroup(v, statsFile, decodeStats) })
	v.characters = sync.OnceValues(v.loadCharacters)
	v.constants = sync.OnceValues(v.loadConstants)
	v.allTrees = sync.OnceValues(v.loadAllTrees)
	v.trees = map[string]func() (*Tree, error){}
	v.texts = map[string]func() (*Text, error){}
	return v, nil
}

func isGroup(name, prefix string) bool {
	return strings.HasPrefix(name, prefix) && strings.HasSuffix(name, fbSuffix)
}

func groupTag(name, prefix string) string {
	return strings.TrimSuffix(strings.TrimPrefix(name, prefix), fbSuffix)
}

// readGroup reads a file listed in the manifest, checks its size and hash, and decodes it.
func readGroup[T any](v *Version, name string, decode func([]byte) (T, error)) (T, error) {
	var zero T
	f := v.files[name]
	b, err := fs.ReadFile(v.fsys, name)
	if err != nil {
		return zero, err
	}
	sum := sha256.Sum256(b)
	if len(b) != f.Size || hex.EncodeToString(sum[:]) != f.SHA256 {
		return zero, fmt.Errorf("%s: size or sha256 differs from %s", name, ManifestName)
	}
	var out T
	err = decodeSafely(func(b []byte) (err error) { out, err = decode(b); return err }, b)
	if err != nil {
		return zero, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

// Manifest returns the dataset's manifest.
func (v *Version) Manifest() Manifest { return v.manifest }

// Game returns the edition, such as "poe1".
func (v *Version) Game() string { return v.manifest.Game }

// Name returns the game version, such as "3.29.3.3".
func (v *Version) Name() string { return v.manifest.Version }

// Variants returns the tree variants in the dataset, in manifest order.
func (v *Version) Variants() []string { return slices.Clone(v.variants) }

// Languages returns the text languages in the dataset, in manifest order.
func (v *Version) Languages() []string { return slices.Clone(v.langs) }

// Stats returns the stats group.
func (v *Version) Stats() ([]Stat, error) { return v.stats() }

// Characters returns the characters group, checked against the stats.
func (v *Version) Characters() ([]Character, error) { return v.characters() }

// Constants returns the edition constants group.
func (v *Version) Constants() ([]Constant, error) { return v.constants() }

func (v *Version) loadCharacters() ([]Character, error) {
	stats, err := v.stats()
	if err != nil {
		return nil, err
	}
	cs, err := readGroup(v, charactersFile, decodeCharacters)
	if err != nil {
		return nil, err
	}
	d := Dataset{Stats: stats, Characters: cs}
	return cs, d.validate()
}

func (v *Version) loadConstants() ([]Constant, error) {
	cs, err := readGroup(v, constantsFile, decodeConstants)
	if err != nil {
		return nil, err
	}
	d := Dataset{Constants: cs}
	return cs, d.validate()
}

// Tree returns the tree of one variant, such as "standard", checked against the stats and
// characters.
func (v *Version) Tree(variant string) (*Tree, error) {
	if !slices.Contains(v.variants, variant) {
		return nil, fmt.Errorf("dataset %s has no %s tree", v.manifest.Version, variant)
	}
	v.mu.Lock()
	load, ok := v.trees[variant]
	if !ok {
		load = sync.OnceValues(func() (*Tree, error) { return v.loadTree(variant) })
		v.trees[variant] = load
	}
	v.mu.Unlock()
	return load()
}

func (v *Version) loadTree(variant string) (*Tree, error) {
	stats, err := v.stats()
	if err != nil {
		return nil, err
	}
	chars, err := v.characters()
	if err != nil {
		return nil, err
	}
	t, err := readGroup(v, treePrefix+variant+fbSuffix, decodeTree)
	if err != nil {
		return nil, err
	}
	if t.Variant != variant {
		return nil, fmt.Errorf("%s%s%s: holds variant %q", treePrefix, variant, fbSuffix, t.Variant)
	}
	d := Dataset{Stats: stats, Characters: chars, Trees: []Tree{t}}
	if err := d.validate(); err != nil {
		return nil, err
	}
	return &t, nil
}

func (v *Version) loadAllTrees() ([]Tree, error) {
	var out []Tree
	for _, variant := range v.variants {
		t, err := v.Tree(variant)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, nil
}

// Text returns the display text of one language, such as "en". Text names nodes of every
// tree, so it reads every tree to check them.
func (v *Version) Text(language string) (*Text, error) {
	if !slices.Contains(v.langs, language) {
		return nil, fmt.Errorf("dataset %s has no %s text", v.manifest.Version, language)
	}
	v.mu.Lock()
	load, ok := v.texts[language]
	if !ok {
		load = sync.OnceValues(func() (*Text, error) { return v.loadText(language) })
		v.texts[language] = load
	}
	v.mu.Unlock()
	return load()
}

func (v *Version) loadText(language string) (*Text, error) {
	stats, err := v.stats()
	if err != nil {
		return nil, err
	}
	chars, err := v.characters()
	if err != nil {
		return nil, err
	}
	trees, err := v.allTrees()
	if err != nil {
		return nil, err
	}
	t, err := readGroup(v, textPrefix+language+fbSuffix, decodeText)
	if err != nil {
		return nil, err
	}
	if t.Language != language {
		return nil, fmt.Errorf("%s%s%s: holds language %q", textPrefix, language, fbSuffix, t.Language)
	}
	d := Dataset{Stats: stats, Characters: chars, Trees: trees, Texts: []Text{t}}
	if err := d.validateTexts(); err != nil {
		return nil, err
	}
	return &t, nil
}

// Dataset returns every table group, with the display text of the given languages.
func (v *Version) Dataset(languages ...string) (*Dataset, error) {
	d := &Dataset{Manifest: v.manifest}
	var err error
	if d.Stats, err = v.Stats(); err != nil {
		return nil, err
	}
	if d.Characters, err = v.Characters(); err != nil {
		return nil, err
	}
	if d.Constants, err = v.Constants(); err != nil {
		return nil, err
	}
	if d.Trees, err = v.allTrees(); err != nil {
		return nil, err
	}
	for _, l := range languages {
		t, err := v.Text(l)
		if err != nil {
			return nil, err
		}
		d.Texts = append(d.Texts, *t)
	}
	return d, nil
}
