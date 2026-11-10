package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/dataset/remote"
)

// serveSource serves a source laid out as pbt-data with leagues 3.28 and 3.29 of poe1, and
// returns its URL.
func serveSource(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	idx := remote.Index{"poe1": {}}
	for league, d := range map[string]*dataset.Dataset{"3.28": testDataset("3.28.0.16", 10), "3.29": testDataset("3.29.3.5", 20)} {
		dir := filepath.Join(root, "poe1", league)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := dataset.Write(dir, d); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(dir, dataset.ManifestName))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		idx["poe1"][league] = remote.Entry{Version: d.Manifest.Version, Manifest: hex.EncodeToString(sum[:])}
	}
	b, err := json.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, remote.IndexName), b, 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(root)))
	t.Cleanup(srv.Close)
	return srv.URL + "/"
}

func TestDataPull(t *testing.T) {
	t.Parallel()
	src, root := serveSource(t), t.TempDir()
	var out, notes bytes.Buffer
	if err := run(t.Context(), []string{"data", "pull", "--root", root, "--source", src}, &out, &notes); err != nil {
		t.Fatal(err)
	}
	want := "poe1 3.28: 3.28.0.16 (updated, 7 files fetched)\npoe1 3.29: 3.29.3.5 (updated, 7 files fetched)\n"
	if out.String() != want {
		t.Errorf("first pull:\n%s\nwant:\n%s", out.String(), want)
	}
	out.Reset()
	if err := run(t.Context(), []string{"data", "pull", "3.29", "--root", root, "--source", src}, &out, &notes); err != nil {
		t.Fatal(err)
	}
	if want := "poe1 3.29: 3.29.3.5 (current)\n"; out.String() != want {
		t.Errorf("second pull: %q, want %q", out.String(), want)
	}
	out.Reset()
	if err := run(t.Context(), []string{"data", "show", "stat", "base_strength", "--version", "3.29", "--root", root}, &out, &notes); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "base_strength") {
		t.Errorf("show from pulled data: %q", out.String())
	}
	if err := run(t.Context(), []string{"data", "pull", "3.30", "--root", root, "--source", src}, &out, &notes); err == nil {
		t.Error("pull of an unknown league: got nil error")
	}
}

// Without --root, commands use the cache and fill it on first use.
func TestCachePulledOnFirstUse(t *testing.T) {
	src := serveSource(t)
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("HOME", cache)
	for i, wantNote := range []bool{true, false} {
		var out, notes bytes.Buffer
		if err := run(t.Context(), []string{"data", "show", "stat", "base_strength", "--version", "3.28", "--source", src}, &out, &notes); err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(notes.String(), "pulling from"); got != wantNote {
			t.Errorf("run %d: pull note %v, want %v; notes:\n%s", i, got, wantNote, notes.String())
		}
	}
	base, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if !hasDataset(filepath.Join(base, cacheDir, "poe1")) {
		t.Errorf("no datasets in the cache %s", base)
	}
}
