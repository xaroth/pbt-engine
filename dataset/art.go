package dataset

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// ArtManifest lists the art files that go with a dataset: where each passive tree icon is on
// the sprite sheets, and the face image of each class and ascendancy. art.schema.json
// describes its JSON. The dataset format covers its shape.
type ArtManifest struct {
	Files []File `json:"files"`
	// Sprites holds per kind, such as "normalActive", one sheet per zoom level, by ascending
	// zoom.
	Sprites map[string][]SpriteSheet `json:"sprites"`
	// ClassFaces names the file of each base class's face by character id.
	ClassFaces map[string]string `json:"classFaces,omitempty"`
	// AscendancyFaces names the file of each ascendancy's face by ascendancy id.
	AscendancyFaces map[string]string `json:"ascendancyFaces,omitempty"`
}

// SpriteSheet places the icons of one kind at one zoom level on File, which Files lists. At
// Zoom, one tree unit takes Zoom sheet pixels.
type SpriteSheet struct {
	Zoom   float64              `json:"zoom"`
	File   string               `json:"file"`
	Width  int                  `json:"width"`
	Height int                  `json:"height"`
	Coords map[string]SheetRect `json:"coords"`
}

// SheetRect is an icon's rectangle on a sheet, in sheet pixels.
type SheetRect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// ParseArtManifest reads an art manifest from its JSON. It fails on a field the type does not
// have, a file listed twice, and a sheet or face whose file is not listed.
func ParseArtManifest(b []byte) (ArtManifest, error) {
	var m ArtManifest
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, fmt.Errorf("art manifest: %w: %w", err, ErrInvalid)
	}
	listed := make(map[string]bool, len(m.Files))
	for _, f := range m.Files {
		if listed[f.Name] {
			return m, fmt.Errorf("art manifest: file %s listed twice: %w", f.Name, ErrInvalid)
		}
		listed[f.Name] = true
	}
	for _, kind := range slices.Sorted(maps.Keys(m.Sprites)) {
		for _, s := range m.Sprites[kind] {
			if !listed[s.File] {
				return m, fmt.Errorf("art manifest: sprite %s at zoom %g: file %s not listed: %w", kind, s.Zoom, s.File, ErrInvalid)
			}
		}
	}
	for _, faces := range []map[string]string{m.ClassFaces, m.AscendancyFaces} {
		for _, id := range slices.Sorted(maps.Keys(faces)) {
			if !listed[faces[id]] {
				return m, fmt.Errorf("art manifest: face of %s: file %s not listed: %w", id, faces[id], ErrInvalid)
			}
		}
	}
	return m, nil
}
