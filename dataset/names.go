package dataset

import (
	"errors"
	"fmt"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/xaroth/pbt-engine/dataset/internal/fb"
)

const namesPrefix = "names-"

// NamesFile returns the file name of the names of language, such as "en".
func NamesFile(language string) string { return namesPrefix + language + fbSuffix }

// Names are the character, ascendancy, node, override and timeless replacement names of one
// language: the part of its Text a host needs to show a class, a node, an override or a
// timeless jewel's passive by name without the stat descriptions. Write writes them beside each text as names-<language>.fb.
type Names struct {
	Language string
	// NodeNames are sorted by ID.
	NodeNames []NodeName
	// CharacterNames are keyed by Character.ID.
	CharacterNames []Name
	// AscendancyNames are keyed by tree variant and Ascendancy.ID.
	AscendancyNames []AscendancyName
	// OverrideNames are keyed by NodeOverride.ID.
	OverrideNames []Name
	// TimelessNames are keyed by TimelessReplacement.ID.
	TimelessNames []Name
}

// Names returns the names of t, sharing its slices.
func (t *Text) Names() Names {
	return Names{
		Language: t.Language, NodeNames: t.NodeNames, CharacterNames: t.CharacterNames,
		AscendancyNames: t.AscendancyNames, OverrideNames: t.OverrideNames, TimelessNames: t.TimelessNames,
	}
}

// Names returns the names of one language, such as "en", shared and read-only. It reads the
// names file and no other, so its names are not checked against the characters and trees they
// name; Write checks them. A dataset without a names file for the language gives the names of
// its text.
func (s *Store) Names(language string) (*Names, error) {
	if l, ok := s.names[language]; ok {
		return l.get(func() (*Names, error) { return s.loadNames(language) })
	}
	t, err := s.Text(language)
	if err != nil {
		if errors.Is(err, ErrNotInDataset) {
			return nil, fmt.Errorf("dataset %s has no %s names: %w", s.manifest.Version, language, ErrNotInDataset)
		}
		return nil, err
	}
	n := t.Names()
	return &n, nil
}

func (s *Store) loadNames(language string) (*Names, error) {
	name := NamesFile(language)
	n, err := readGroup(s, name, decodeNamesGroup)
	if err != nil {
		return nil, err
	}
	if n.Language != language {
		return nil, fmt.Errorf("%s: holds names language %q: %w", name, n.Language, ErrInvalid)
	}
	if err := checkNodeNamesSorted(name, n.NodeNames); err != nil {
		return nil, err
	}
	return &n, nil
}

func encodeNamesGroup(n Names) []byte {
	b := flatbuffers.NewBuilder(0)
	nodes := nodeNameVector(b, fb.NamesStartNodeNamesVector, n.NodeNames)
	chars := nameVector(b, fb.NamesStartCharacterNamesVector, n.CharacterNames)
	ascs := ascendancyNameVector(b, fb.NamesStartAscendancyNamesVector, n.AscendancyNames)
	addOverrides := optionalNames(b, nameList{fb.NamesStartOverrideNamesVector, fb.NamesAddOverrideNames, n.OverrideNames})
	addTimeless := optionalNames(b, nameList{fb.NamesStartTimelessNamesVector, fb.NamesAddTimelessNames, n.TimelessNames})
	lang := b.CreateString(n.Language)
	fb.NamesStart(b)
	fb.NamesAddLanguage(b, lang)
	fb.NamesAddNodeNames(b, nodes)
	fb.NamesAddCharacterNames(b, chars)
	fb.NamesAddAscendancyNames(b, ascs)
	addOverrides()
	addTimeless()
	fb.FinishNamesBuffer(b, fb.NamesEnd(b))
	return b.FinishedBytes()
}

func decodeNamesGroup(buf []byte) (Names, error) {
	if !fb.NamesBufferHasIdentifier(buf) {
		return Names{}, errIdentifier
	}
	root := fb.GetRootAsNames(buf, 0)
	return Names{
		Language:        string(root.Language()),
		NodeNames:       decodeNodeNames(root.NodeNamesLength(), root.NodeNames),
		CharacterNames:  decodeNames(root.CharacterNamesLength(), root.CharacterNames),
		AscendancyNames: decodeAscendancyNames(root.AscendancyNamesLength(), root.AscendancyNames),
		OverrideNames:   decodeNames(root.OverrideNamesLength(), root.OverrideNames),
		TimelessNames:   decodeNames(root.TimelessNamesLength(), root.TimelessNames),
	}, nil
}
