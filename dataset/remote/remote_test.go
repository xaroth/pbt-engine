package remote_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
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

// source is a remote source in memory that records what is fetched.
type source struct {
	mu      sync.Mutex
	files   map[string][]byte
	fetched []string
}

func (s *source) Fetch(_ context.Context, name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fetched = append(s.fetched, name)
	b, ok := s.files[name]
	if !ok {
		return nil, fmt.Errorf("%s: %w", name, fs.ErrNotExist)
	}
	return b, nil
}

// take returns and clears the names fetched since the last call, sorted, without the index.
func (s *source) take() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := slices.DeleteFunc(s.fetched, func(n string) bool { return n == remote.IndexName })
	s.fetched = nil
	slices.Sort(out)
	return out
}

func dataset1(version string, stats ...string) *dataset.Dataset {
	d := &dataset.Dataset{Manifest: dataset.Manifest{Game: "poe1", Version: version}}
	for _, s := range append([]string{"level"}, stats...) {
		d.Stats = append(d.Stats, dataset.Stat{ID: s})
	}
	return d
}

// publish writes d as league of poe1 into s and points the index at it.
func (s *source) publish(t *testing.T, league string, d *dataset.Dataset) {
	t.Helper()
	dir := t.TempDir()
	if _, err := dataset.Write(dir, d); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.files == nil {
		s.files = map[string][]byte{}
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		s.files["poe1/"+league+"/"+e.Name()] = b
	}
	idx := remote.Index{}
	if b, ok := s.files[remote.IndexName]; ok {
		if err := json.Unmarshal(b, &idx); err != nil {
			t.Fatal(err)
		}
	}
	if idx["poe1"] == nil {
		idx["poe1"] = map[string]remote.Entry{}
	}
	sum := sha256.Sum256(s.files["poe1/"+league+"/"+dataset.ManifestName])
	idx["poe1"][league] = remote.Entry{Version: d.Manifest.Version, Manifest: hex.EncodeToString(sum[:])}
	b, err := json.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}
	s.files[remote.IndexName] = b
}

func pull(t *testing.T, s remote.Fetcher, dir string, leagues ...string) []remote.Result {
	t.Helper()
	rs, err := remote.Pull(t.Context(), s, dir, "poe1", leagues...)
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

func TestPull(t *testing.T) {
	t.Parallel()
	s := &source{}
	s.publish(t, "3.28", dataset1("3.28.0.16"))
	s.publish(t, "3.29", dataset1("3.29.3.3", "strength"))
	dir := t.TempDir()

	rs := pull(t, s, dir)
	if want := []remote.Result{{"3.28", "3.28.0.16", true, 5}, {"3.29", "3.29.3.3", true, 5}}; !slices.Equal(rs, want) {
		t.Errorf("first pull: got %+v, want %+v", rs, want)
	}
	if got := versions(t, dir); !slices.Equal(got, []string{"3.28.0.16", "3.29.3.3"}) {
		t.Errorf("pulled versions %v", got)
	}
	s.take()

	if rs := pull(t, s, dir); rs[0].Updated || rs[1].Updated {
		t.Errorf("second pull updated: %+v", rs)
	}
	if got := s.take(); len(got) != 0 {
		t.Errorf("second pull fetched %v, want only the index", got)
	}

	// A new patch changes the stats; the other files are reused.
	s.publish(t, "3.29", dataset1("3.29.3.5", "strength"))
	rs = pull(t, s, dir)
	if rs[0].Updated || !rs[1].Updated || rs[1].Version != "3.29.3.5" {
		t.Errorf("patch pull: %+v", rs)
	}
	if got, want := s.take(), []string{"poe1/3.29/manifest.json"}; !slices.Equal(got, want) {
		t.Errorf("patch pull fetched %v, want %v (same files, new version)", got, want)
	}
	s.publish(t, "3.29", dataset1("3.29.3.6", "strength", "dexterity"))
	pull(t, s, dir, "3.29")
	if got, want := s.take(), []string{"poe1/3.29/manifest.json", "poe1/3.29/stats.fb"}; !slices.Equal(got, want) {
		t.Errorf("stats change fetched %v, want %v", got, want)
	}
	if got := versions(t, dir); !slices.Equal(got, []string{"3.28.0.16", "3.29.3.6"}) {
		t.Errorf("versions after updates %v", got)
	}
	if left := strayEntries(t, dir); len(left) != 0 {
		t.Errorf("left behind %v", left)
	}
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

func TestPullKeepsOldCopyOnError(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name   string
		damage func(s *source)
		want   error
	}{
		{"manifest differs from index", func(s *source) {
			m := s.files["poe1/3.29/manifest.json"]
			s.files["poe1/3.29/manifest.json"] = append(m, ' ')
		}, remote.ErrMismatch},
		{"file differs from manifest", func(s *source) {
			s.files["poe1/3.29/stats.fb"] = append([]byte("x"), s.files["poe1/3.29/stats.fb"][1:]...)
		}, remote.ErrMismatch},
		{"file missing", func(s *source) { delete(s.files, "poe1/3.29/stats.fb") }, fs.ErrNotExist},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s := &source{}
			s.publish(t, "3.29", dataset1("3.29.3.3"))
			dir := t.TempDir()
			pull(t, s, dir)
			s.publish(t, "3.29", dataset1("3.29.3.5", "strength"))
			testCase.damage(s)
			if _, err := remote.Pull(t.Context(), s, dir, "poe1"); !errors.Is(err, testCase.want) {
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

func TestPullErrors(t *testing.T) {
	t.Parallel()
	s := &source{}
	s.publish(t, "3.29", dataset1("3.29.3.3"))
	if _, err := remote.Pull(t.Context(), s, t.TempDir(), "poe1", "3.30"); !errors.Is(err, remote.ErrUnknownLeague) {
		t.Errorf("unknown league: got %v", err)
	}
	if _, err := remote.Pull(t.Context(), s, t.TempDir(), "poe1", "../x"); !errors.Is(err, remote.ErrUnknownLeague) {
		t.Errorf("league outside dir: got %v", err)
	}
	other := &source{files: maps.Clone(s.files)}
	other.files[remote.IndexName] = []byte(strings.Replace(string(s.files[remote.IndexName]), `"version":"3.29.3.3"`, `"version":"3.29.3.4"`, 1))
	if _, err := remote.Pull(t.Context(), other, t.TempDir(), "poe1"); !errors.Is(err, remote.ErrMismatch) {
		t.Errorf("index names another version: got %v", err)
	}
	broken := &source{files: map[string][]byte{remote.IndexName: []byte("{")}}
	if _, err := remote.Pull(t.Context(), broken, t.TempDir(), "poe1"); err == nil {
		t.Error("bad index: got nil error")
	}
}

func TestHTTP(t *testing.T) {
	t.Parallel()
	s := &source{}
	s.publish(t, "3.29", dataset1("3.29.3.3"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := s.Fetch(r.Context(), strings.TrimPrefix(r.URL.Path, "/data/"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	}))
	defer srv.Close()
	f := remote.HTTP{Base: srv.URL + "/data/", Client: srv.Client()}
	dir := t.TempDir()
	if rs := pull(t, f, dir); len(rs) != 1 || !rs[0].Updated {
		t.Errorf("got %+v", rs)
	}
	if _, err := f.Fetch(t.Context(), "nope"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("missing file: got %v", err)
	}
}
