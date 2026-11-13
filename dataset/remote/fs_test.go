package remote_test

import (
	"bytes"
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/dataset/remote"
)

// FS fetches the manifest when made and a file by its hash when read, and opens as a dataset.
func TestFS(t *testing.T) {
	t.Parallel()
	f := &fake{files: map[string][]byte{}}
	m := f.upload(t, dataset1("3.29.3.6", "strength"))
	fsys, err := remote.FS(t.Context(), f, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{manifest(m.ID)}; !slices.Equal(f.fetched, want) {
		t.Errorf("made: fetched %v, want %v", f.fetched, want)
	}
	got, err := fs.ReadFile(fsys, "stats.fb")
	if err != nil {
		t.Fatal(err)
	}
	if want := f.files[blob(statsSHA(m))]; !bytes.Equal(got, want) {
		t.Errorf("stats.fb: got %d bytes, want the blob's %d", len(got), len(want))
	}
	store, err := dataset.Open(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := store.HasStat("strength"); err != nil || !ok {
		t.Errorf("the dataset read through FS has strength: %v, %v", ok, err)
	}
}

func TestFSErrors(t *testing.T) {
	t.Parallel()
	f := &fake{files: map[string][]byte{}}
	m := f.upload(t, dataset1("3.29.3.6"))
	other := f.upload(t, dataset1("3.29.3.7"))

	f.files[blob(statsSHA(m))] = []byte("not the stats")
	fsys, err := remote.FS(t.Context(), f, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.ReadFile(fsys, "stats.fb"); !errors.Is(err, remote.ErrMismatch) {
		t.Errorf("blob that does not match its hash: got %v, want %v", err, remote.ErrMismatch)
	}
	if _, err := fs.ReadFile(fsys, "nothing.fb"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("file not in the manifest: got %v, want %v", err, fs.ErrNotExist)
	}

	f.files[manifest(m.ID)] = f.files[manifest(other.ID)]
	if _, err := remote.FS(t.Context(), f, m.ID); !errors.Is(err, remote.ErrMismatch) {
		t.Errorf("manifest of another id: got %v, want %v", err, remote.ErrMismatch)
	}
	if _, err := remote.FS(t.Context(), f, strings.Repeat("A", 64)); !errors.Is(err, dataset.ErrInvalid) {
		t.Errorf("id not a hash: got %v, want %v", err, dataset.ErrInvalid)
	}
}
