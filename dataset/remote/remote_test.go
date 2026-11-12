package remote_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/dataset/remote"
)

// bucket is a directory in the bucket's layout, served over HTTP, that records the paths
// fetched from it.
type bucket struct {
	dir     string
	src     remote.HTTP
	mu      sync.Mutex
	fetched []string
}

func newBucket(t *testing.T) *bucket {
	t.Helper()
	b := &bucket{dir: t.TempDir()}
	files := http.FileServer(http.Dir(b.dir))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		b.fetched = append(b.fetched, strings.TrimPrefix(r.URL.Path, "/"))
		b.mu.Unlock()
		files.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	b.src = remote.HTTP{Base: srv.URL + "/", Client: srv.Client()}
	return b
}

// take returns and clears the paths fetched since the last call, sorted, without the index.
func (b *bucket) take() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := slices.DeleteFunc(b.fetched, func(n string) bool { return n == remote.IndexName })
	b.fetched = nil
	slices.Sort(out)
	return out
}

func (b *bucket) write(t *testing.T, name string, content []byte) {
	t.Helper()
	p := filepath.Join(b.dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func (b *bucket) read(t *testing.T, name string) []byte {
	t.Helper()
	c, err := os.ReadFile(filepath.Join(b.dir, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func dataset1(version string, stats ...string) *dataset.Dataset {
	d := &dataset.Dataset{Manifest: dataset.Manifest{Game: "poe1", Version: version}}
	for _, s := range append([]string{"level"}, stats...) {
		d.Stats = append(d.Stats, dataset.Stat{ID: s})
	}
	return d
}

// upload puts d's files under blobs/ and its manifest under manifests/, as a publish does
// before it touches the index.
func (b *bucket) upload(t *testing.T, d *dataset.Dataset) dataset.Manifest {
	t.Helper()
	dir := t.TempDir()
	m, err := dataset.Write(dir, d)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range m.Files {
		c, err := os.ReadFile(filepath.Join(dir, f.Name))
		if err != nil {
			t.Fatal(err)
		}
		b.write(t, blob(f.SHA256), c)
	}
	mb, err := os.ReadFile(filepath.Join(dir, dataset.ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	b.write(t, manifest(m.ID), mb)
	return m
}

// publish uploads d and makes it the latest of league.
func (b *bucket) publish(t *testing.T, league string, d *dataset.Dataset) dataset.Manifest {
	t.Helper()
	m := b.upload(t, d)
	idx := remote.Index{}
	if c, err := os.ReadFile(filepath.Join(b.dir, remote.IndexName)); err == nil {
		if err := json.Unmarshal(c, &idx); err != nil {
			t.Fatal(err)
		}
	}
	if idx["poe1"] == nil {
		idx["poe1"] = map[string]remote.Entry{}
	}
	idx["poe1"][league] = remote.Entry{Version: m.Version, Dataset: m.ID, Art: strings.Repeat("a", 64)}
	c, err := json.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}
	b.write(t, remote.IndexName, c)
	return m
}

func blob(sha string) string    { return "blobs/" + sha }
func manifest(id string) string { return "manifests/" + id + ".json" }

func statsSHA(m dataset.Manifest) string {
	for _, f := range m.Files {
		if f.Name == "stats.fb" {
			return f.SHA256
		}
	}
	panic("stats.fb not in manifest")
}

func pull(t *testing.T, b *bucket, dir string, leagues ...string) []remote.Result {
	t.Helper()
	rs, err := remote.Pull(t.Context(), b.src, dir, "poe1", leagues...)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

// versions opens dir as a registry and lists its poe1 versions.
func versions(t *testing.T, dir string) []string {
	t.Helper()
	reg, err := dataset.OpenRoot(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, v := range []string{"3.28", "3.29"} {
		if res, err := reg.Resolve("poe1", v); err == nil {
			out = append(out, res.Store.Manifest().Version)
		}
	}
	return out
}

func strayEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out
}

// Pull follows the latest dataset of each league and fetches only what changed.
func TestPull(t *testing.T) {
	t.Parallel()
	b := newBucket(t)
	b.publish(t, "3.28", dataset1("3.28.0.16"))
	b.publish(t, "3.29", dataset1("3.29.3.3", "strength"))
	dir := t.TempDir()

	rs := pull(t, b, dir)
	if want := []remote.Result{{"3.28", "3.28.0.16", true, 5}, {"3.29", "3.29.3.3", true, 5}}; !slices.Equal(rs, want) {
		t.Errorf("first pull: got %+v, want %+v", rs, want)
	}
	if got := versions(t, dir); !slices.Equal(got, []string{"3.28.0.16", "3.29.3.3"}) {
		t.Errorf("pulled versions %v", got)
	}
	b.take()

	if rs := pull(t, b, dir); rs[0].Updated || rs[1].Updated {
		t.Errorf("second pull updated: %+v", rs)
	}
	if got := b.take(); len(got) != 0 {
		t.Errorf("second pull fetched %v, want only the index", got)
	}

	// A new patch with the same files fetches only its manifest.
	m := b.publish(t, "3.29", dataset1("3.29.3.5", "strength"))
	rs = pull(t, b, dir)
	if rs[0].Updated || !rs[1].Updated || rs[1].Version != "3.29.3.5" {
		t.Errorf("patch pull: %+v", rs)
	}
	if got, want := b.take(), []string{manifest(m.ID)}; !slices.Equal(got, want) {
		t.Errorf("patch pull fetched %v, want %v", got, want)
	}
	m = b.publish(t, "3.29", dataset1("3.29.3.6", "strength", "dexterity"))
	pull(t, b, dir, "3.29")
	want := []string{blob(statsSHA(m)), manifest(m.ID)}
	slices.Sort(want)
	if got := b.take(); !slices.Equal(got, want) {
		t.Errorf("stats change fetched %v, want %v", got, want)
	}
	if got := versions(t, dir); !slices.Equal(got, []string{"3.28.0.16", "3.29.3.6"}) {
		t.Errorf("versions after updates %v", got)
	}
	if left := strayEntries(t, dir); len(left) != 0 {
		t.Errorf("left behind %v", left)
	}
}

// PullID fetches the dataset pinned by id, whatever the index says is latest.
func TestPullID(t *testing.T) {
	t.Parallel()
	b := newBucket(t)
	pinned := b.publish(t, "3.29", dataset1("3.29.3.3"))
	latest := b.publish(t, "3.29", dataset1("3.29.3.5", "strength"))
	dir := filepath.Join(t.TempDir(), "pinned")

	r, err := remote.PullID(t.Context(), b.src, dir, pinned.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := (remote.Result{Version: "3.29.3.3", Updated: true, Fetched: 5}); r != want {
		t.Errorf("got %+v, want %+v", r, want)
	}
	if got := versions(t, filepath.Dir(dir)); !slices.Equal(got, []string{"3.29.3.3"}) {
		t.Errorf("pulled versions %v", got)
	}
	b.take()

	if r, err := remote.PullID(t.Context(), b.src, dir, pinned.ID); err != nil || r.Updated {
		t.Errorf("second pull: %+v, %v", r, err)
	}
	if got := b.take(); len(got) != 0 {
		t.Errorf("second pull fetched %v, want nothing", got)
	}

	// Moving the pin reuses the files the two share.
	if _, err := remote.PullID(t.Context(), b.src, dir, latest.ID); err != nil {
		t.Fatal(err)
	}
	if got, want := b.take(), []string{blob(statsSHA(latest)), manifest(latest.ID)}; !slices.Equal(got, want) {
		t.Errorf("moving the pin fetched %v, want %v", got, want)
	}
}

// A failed pull keeps the old copy and leaves nothing behind.
func TestPullKeepsOldCopyOnError(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name   string
		damage func(t *testing.T, b *bucket, m dataset.Manifest)
		want   error
	}{
		{"blob does not match its hash", func(t *testing.T, b *bucket, m dataset.Manifest) {
			t.Helper()
			name := blob(statsSHA(m))
			c := b.read(t, name)
			b.write(t, name, append([]byte{c[0] ^ 1}, c[1:]...))
		}, remote.ErrMismatch},
		{"blob missing", func(t *testing.T, b *bucket, m dataset.Manifest) {
			t.Helper()
			if err := os.Remove(filepath.Join(b.dir, "blobs", statsSHA(m))); err != nil {
				t.Fatal(err)
			}
		}, remote.ErrStatus},
		{"manifest of another dataset under the id", func(t *testing.T, b *bucket, m dataset.Manifest) {
			t.Helper()
			other := b.upload(t, dataset1("3.29.3.5", "dexterity"))
			b.write(t, manifest(m.ID), b.read(t, manifest(other.ID)))
		}, remote.ErrMismatch},
		{"manifest altered", func(t *testing.T, b *bucket, m dataset.Manifest) {
			t.Helper()
			c := strings.Replace(string(b.read(t, manifest(m.ID))), statsSHA(m), strings.Repeat("0", 64), 1)
			b.write(t, manifest(m.ID), []byte(c))
		}, dataset.ErrInvalid},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			b := newBucket(t)
			b.publish(t, "3.29", dataset1("3.29.3.3"))
			dir := t.TempDir()
			pull(t, b, dir)
			m := b.publish(t, "3.29", dataset1("3.29.3.5", "strength"))
			testCase.damage(t, b, m)
			if _, err := remote.Pull(t.Context(), b.src, dir, "poe1"); !errors.Is(err, testCase.want) {
				t.Fatalf("got %v, want %v", err, testCase.want)
			}
			if got := versions(t, dir); !slices.Equal(got, []string{"3.29.3.3"}) {
				t.Errorf("after a failed pull: %v, want the old copy", got)
			}
			if left := strayEntries(t, dir); len(left) != 0 {
				t.Errorf("left behind %v", left)
			}
		})
	}
}

// A league that fails does not stop the others.
func TestPullContinuesPastFailure(t *testing.T) {
	t.Parallel()
	b := newBucket(t)
	m := b.publish(t, "3.28", dataset1("3.28.0.16", "dexterity"))
	b.publish(t, "3.29", dataset1("3.29.3.5"))
	if err := os.Remove(filepath.Join(b.dir, "blobs", statsSHA(m))); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	rs, err := remote.Pull(t.Context(), b.src, dir, "poe1", "3.27", "3.28", "3.29")
	if !errors.Is(err, remote.ErrUnknownLeague) || !errors.Is(err, remote.ErrStatus) {
		t.Errorf("got %v, want the unknown league and the missing blob joined", err)
	}
	if len(rs) != 1 || rs[0].League != "3.29" || !rs[0].Updated {
		t.Errorf("results %+v, want 3.29 updated", rs)
	}
	if got := versions(t, dir); !slices.Equal(got, []string{"3.29.3.5"}) {
		t.Errorf("pulled versions %v", got)
	}
}

// What an interrupted pull left behind is removed by the next pull of the league, also when
// the league is current.
func TestPullRemovesLeftovers(t *testing.T) {
	t.Parallel()
	b := newBucket(t)
	b.publish(t, "3.29", dataset1("3.29.3.5"))
	dir := t.TempDir()
	pull(t, b, dir)
	for _, left := range []string{".3.29-123", ".3.29-456.old"} {
		if err := os.MkdirAll(filepath.Join(dir, left), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	pull(t, b, dir)
	if left := strayEntries(t, dir); len(left) != 0 {
		t.Errorf("left behind %v", left)
	}
}

func TestPullErrors(t *testing.T) {
	t.Parallel()
	b := newBucket(t)
	b.publish(t, "3.29", dataset1("3.29.3.3"))
	if _, err := remote.Pull(t.Context(), b.src, t.TempDir(), "poe1", "3.30"); !errors.Is(err, remote.ErrUnknownLeague) {
		t.Errorf("unknown league: got %v", err)
	}
	if _, err := remote.Pull(t.Context(), b.src, t.TempDir(), "poe1", "../x"); !errors.Is(err, remote.ErrUnknownLeague) {
		t.Errorf("league outside dir: got %v", err)
	}
	if _, err := remote.PullID(t.Context(), b.src, t.TempDir(), "../x"); !errors.Is(err, dataset.ErrInvalid) {
		t.Errorf("id outside manifests: got %v", err)
	}
	_, err := remote.PullID(t.Context(), b.src, t.TempDir(), strings.Repeat("0", 64))
	if !errors.Is(err, remote.ErrStatus) || !strings.Contains(err.Error(), "404") {
		t.Errorf("unknown id: got %v", err)
	}

	idx := string(b.read(t, remote.IndexName))
	b.write(t, remote.IndexName, []byte(strings.Replace(idx, `"version":"3.29.3.3"`, `"version":"3.29.3.4"`, 1)))
	if _, err := remote.Pull(t.Context(), b.src, t.TempDir(), "poe1"); !errors.Is(err, remote.ErrMismatch) {
		t.Errorf("index names another version: got %v", err)
	}
	b.write(t, remote.IndexName, []byte("{"))
	if _, err := remote.Pull(t.Context(), b.src, t.TempDir(), "poe1"); err == nil {
		t.Error("bad index: got nil error")
	}
}
