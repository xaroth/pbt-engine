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

// With SkipHash FS checks a file's size and not its hash; without, both.
func TestFSSkipHash(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name    string
		blob    func(good []byte) []byte
		opts    []remote.Option
		wantErr bool
	}{
		{name: "wrong bytes, hashed", blob: flipped, wantErr: true},
		{name: "wrong size, hashed", blob: grown, wantErr: true},
		{name: "wrong bytes, skip hash", blob: flipped, opts: []remote.Option{remote.SkipHash()}},
		{name: "wrong size, skip hash", blob: grown, opts: []remote.Option{remote.SkipHash()}, wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			f := &fake{files: map[string][]byte{}}
			m := f.upload(t, dataset1("3.29.3.6"))
			want := testCase.blob(f.files[blob(statsSHA(m))])
			f.files[blob(statsSHA(m))] = want
			fsys, err := remote.FS(t.Context(), f, m.ID, testCase.opts...)
			if err != nil {
				t.Fatal(err)
			}
			got, err := fs.ReadFile(fsys, "stats.fb")
			switch {
			case testCase.wantErr && !errors.Is(err, remote.ErrMismatch):
				t.Errorf("got %v, want %v", err, remote.ErrMismatch)
			case !testCase.wantErr && (err != nil || !bytes.Equal(got, want)):
				t.Errorf("got %d bytes and error %v, want the blob's %d bytes", len(got), err, len(want))
			}
		})
	}
}

func flipped(b []byte) []byte {
	out := bytes.Clone(b)
	out[len(out)-1] ^= 0xff
	return out
}

func grown(b []byte) []byte { return append(bytes.Clone(b), 0) }
