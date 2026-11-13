package remote

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"path"
	"time"

	"github.com/xaroth/pbt-engine/dataset"
)

// FS reads the dataset with id from f without a local copy. It fetches the manifest now and
// checks it against id; each file the manifest lists is fetched from its blob when read, and
// checked against its size and SHA-256. Nothing is cached: read each file once, as a
// dataset.Store does. Every read uses ctx.
func FS(ctx context.Context, f Fetcher, id string) (fs.FS, error) {
	if !validHash(id) {
		return nil, fmt.Errorf("dataset id %q: %w", id, dataset.ErrInvalid)
	}
	mb, err := f.Fetch(ctx, "manifests/"+id+".json")
	if err != nil {
		return nil, fmt.Errorf("manifest %s: %w", id, err)
	}
	b := &blobFS{ctx: ctx, f: f, manifest: mb}
	// ReadManifest checks the id against the content, so a manifest with the right id is the right one.
	m, err := dataset.ReadManifest(b)
	if err != nil {
		return nil, fmt.Errorf("manifest %s: %w", id, err)
	}
	if m.ID != id {
		return nil, fmt.Errorf("manifest %s has id %s: %w", id, m.ID, ErrMismatch)
	}
	b.files = make(map[string]dataset.File, len(m.Files))
	for _, file := range m.Files {
		if !validHash(file.SHA256) {
			return nil, fmt.Errorf("manifest %s: file %q, SHA-256 %q: %w", id, file.Name, file.SHA256, dataset.ErrInvalid)
		}
		b.files[file.Name] = file
	}
	return b, nil
}

// blobFS is a dataset at a source: its manifest, and each file it lists read from its blob.
type blobFS struct {
	ctx      context.Context //nolint:containedctx // fs.FS methods take no context
	f        Fetcher
	manifest []byte
	files    map[string]dataset.File
}

func (b *blobFS) ReadFile(name string) ([]byte, error) {
	if name == dataset.ManifestName {
		return bytes.Clone(b.manifest), nil
	}
	file, ok := b.files[name]
	if !ok {
		return nil, readError(name, fs.ErrNotExist)
	}
	c, err := b.f.Fetch(b.ctx, "blobs/"+file.SHA256)
	if err != nil {
		return nil, readError(name, err)
	}
	if !matches(c, file) {
		return nil, readError(name, ErrMismatch)
	}
	return c, nil
}

func readError(name string, err error) error {
	return &fs.PathError{Op: "read", Path: name, Err: err}
}

func (b *blobFS) Open(name string) (fs.File, error) {
	c, err := b.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return &memFile{Reader: bytes.NewReader(c), info: fileInfo{name: path.Base(name), size: int64(len(c))}}, nil
}

// readOnly is the mode of a file read from a source.
const readOnly fs.FileMode = 0o444

// memFile is a file read whole.
type memFile struct {
	*bytes.Reader

	info fileInfo
}

func (f *memFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (*memFile) Close() error                 { return nil }

type fileInfo struct {
	name string
	size int64
}

func (i fileInfo) Name() string     { return i.name }
func (i fileInfo) Size() int64      { return i.size }
func (fileInfo) Mode() fs.FileMode  { return readOnly }
func (fileInfo) ModTime() time.Time { return time.Time{} }
func (fileInfo) IsDir() bool        { return false }
func (fileInfo) Sys() any           { return nil }
