package remote_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
)

// fake is a source held in memory that records what is fetched from it.
type fake struct {
	files   map[string][]byte
	fetched []string
}

func (f *fake) Fetch(_ context.Context, name string) ([]byte, error) {
	f.fetched = append(f.fetched, name)
	b, ok := f.files[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return b, nil
}

// upload puts d's files under blobs/ and its manifest under manifests/.
func (f *fake) upload(t *testing.T, d *dataset.Dataset) dataset.Manifest {
	t.Helper()
	dir := t.TempDir()
	m, err := dataset.Write(dir, d)
	if err != nil {
		t.Fatal(err)
	}
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, file := range m.Files {
		f.files[blob(file.SHA256)] = read(file.Name)
	}
	f.files[manifest(m.ID)] = read(dataset.ManifestName)
	return m
}
