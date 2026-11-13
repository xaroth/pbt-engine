package remote_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/dataset/remote"
)

// Latest gives a league's entry only in the engine's dataset format; an entry without a format
// is format 1.
func TestLatest(t *testing.T) {
	t.Parallel()
	id, art := strings.Repeat("1", 64), strings.Repeat("a", 64)
	entry := func(format string) string {
		return `{"version":"3.29.3.6","dataset":"` + id + `","art":"` + art + `"` + format + `}`
	}
	idx := `{"poe1":{"3.28":` + entry(`,"format":2`) + `,"3.29":` + entry(``) + `}}`
	f := &fake{files: map[string][]byte{remote.IndexName: []byte(idx)}}

	got, err := remote.Latest(t.Context(), f, "poe1", "3.29")
	if err != nil {
		t.Fatal(err)
	}
	if want := (remote.Entry{Version: "3.29.3.6", Dataset: id, Art: art, Format: 1}); got != want {
		t.Errorf("3.29: got %+v, want %+v", got, want)
	}
	if _, err := remote.Latest(t.Context(), f, "poe1", "3.28"); !errors.Is(err, dataset.ErrFormat) {
		t.Errorf("3.28 in format 2: got %v, want %v", err, dataset.ErrFormat)
	}
	if _, err := remote.Latest(t.Context(), f, "poe1", "3.30"); !errors.Is(err, remote.ErrUnknownLeague) {
		t.Errorf("3.30: got %v, want %v", err, remote.ErrUnknownLeague)
	}
}
