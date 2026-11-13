package dataset_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/jsonshape"
	"github.com/xaroth/pbt-engine/internal/realdata"
)

const artSchema = "art.schema.json"

// The published art manifests of the pinned versions read into ArtManifest with unknown fields
// rejected, encode back to the same JSON, and match the schema.
func TestArtManifestRealData(t *testing.T) {
	t.Parallel()
	for _, version := range realdata.Versions() {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			b := realdata.ArtManifest(t, version)
			m, err := dataset.ParseArtManifest(b)
			if err != nil {
				t.Fatal(err)
			}
			if len(m.Files) == 0 || len(m.Sprites) == 0 {
				t.Errorf("files %d, sprite kinds %d; want some of each", len(m.Files), len(m.Sprites))
			}
			if version == realdata.Current && len(m.ClassFaces) != 7 {
				t.Errorf("class faces %d, want 7", len(m.ClassFaces))
			}
			again, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(plain(t, b), plain(t, again)); diff != "" {
				t.Errorf("encoding the type loses or changes fields (-published +encoded):\n%s", diff)
			}
			jsonshape.Validate(t, artSchema, "ArtManifest", b)
		})
	}
}

// A manifest with every field set matches the schema and reads back unchanged.
func TestArtManifestEveryField(t *testing.T) {
	t.Parallel()
	m := artManifest()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	jsonshape.Validate(t, artSchema, "ArtManifest", b)
	got, err := dataset.ParseArtManifest(b)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(m, got); diff != "" {
		t.Errorf("round trip (-want +got):\n%s", diff)
	}
}

func TestParseArtManifestRejects(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		edit func(map[string]any)
	}{
		{"unknown field", func(m map[string]any) { m["faces"] = map[string]any{} }},
		{"unknown sheet field", func(m map[string]any) {
			m["sprites"].(map[string]any)["normalActive"].([]any)[0].(map[string]any)["scale"] = 1
		}},
		{"file listed twice", func(m map[string]any) { m["files"] = append(m["files"].([]any), m["files"].([]any)[0]) }},
		{"sheet file not listed", func(m map[string]any) {
			m["sprites"].(map[string]any)["normalActive"].([]any)[0].(map[string]any)["file"] = "missing.webp"
		}},
		{"class face not listed", func(m map[string]any) { m["classFaces"].(map[string]any)["Str"] = "missing.png" }},
		{"ascendancy face not listed", func(m map[string]any) { m["ascendancyFaces"].(map[string]any)["Juggernaut"] = "missing.png" }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b, err := json.Marshal(artManifest())
			if err != nil {
				t.Fatal(err)
			}
			m, _ := plain(t, b).(map[string]any)
			testCase.edit(m)
			if b, err = json.Marshal(m); err != nil {
				t.Fatal(err)
			}
			if _, err := dataset.ParseArtManifest(b); !errors.Is(err, dataset.ErrInvalid) {
				t.Errorf("got %v, want ErrInvalid", err)
			}
		})
	}
}

func artManifest() dataset.ArtManifest {
	hash := strings.Repeat("ab", 32)
	return dataset.ArtManifest{
		Files: []dataset.File{
			{Name: "skills-0.webp", Size: 10, SHA256: hash},
			{Name: "class-str.png", Size: 20, SHA256: hash},
			{Name: "ascendancy-juggernaut.png", Size: 30, SHA256: hash},
		},
		Sprites: map[string][]dataset.SpriteSheet{
			"normalActive": {{Zoom: 0.1246, File: "skills-0.webp", Width: 64, Height: 32, Coords: map[string]dataset.SheetRect{
				"Art/2DArt/SkillIcons/passives/life.png": {X: 1, Y: 2, W: 3, H: 4},
			}}},
		},
		ClassFaces:      map[string]string{"Str": "class-str.png"},
		AscendancyFaces: map[string]string{"Juggernaut": "ascendancy-juggernaut.png"},
	}
}

// plain decodes JSON into maps, slices and scalars.
func plain(t *testing.T, b []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}
