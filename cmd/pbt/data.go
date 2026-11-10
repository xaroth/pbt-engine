package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/dataset/remote"
)

// anyArgs marks a command that takes any number of arguments after its words.
const anyArgs = -1

// cacheDir is the directory under the user cache directory that holds pulled datasets.
const cacheDir = "pbt"

// dataPull brings the datasets of --game's leagues in pos[2:] (every league when none) up to
// date with --source.
func dataPull(in invocation) error {
	game := in.opts["game"]
	dir, err := in.dir(game)
	if err != nil {
		return err
	}
	return pull(in, in.w, dir, game, in.pos[2:]...)
}

func pull(in invocation, w io.Writer, dir, game string, leagues ...string) error {
	rs, err := remote.Pull(in.ctx, remote.HTTP{Base: in.opts["source"]}, dir, game, leagues...)
	p := &printer{w: w}
	for _, r := range rs {
		state := "current"
		if r.Updated {
			state = fmt.Sprintf("updated, %d files fetched", r.Fetched)
		}
		p.printf("%s %s: %s (%s)\n", game, r.League, r.Version, state)
	}
	if err != nil {
		return fmt.Errorf("data pull: %w", err)
	}
	return p.err
}

// dir is game's dataset directory: --root, else the cache's directory for game.
func (in invocation) dir(game string) (string, error) {
	if in.opts["root"] != "" {
		return in.opts["root"], nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("cache directory: %w", err)
	}
	return filepath.Join(base, cacheDir, game), nil
}

// root is dir, filled from --source first when it is the cache and holds no dataset yet.
func (in invocation) root(notes io.Writer, game string) (string, error) {
	dir, err := in.dir(game)
	if err != nil || in.opts["root"] != "" || hasDataset(dir) {
		return dir, err
	}
	p := &printer{w: notes}
	p.printf("pbt: no datasets in %s, pulling from %s\n", dir, in.opts["source"])
	if p.err != nil {
		return "", p.err
	}
	return dir, pull(in, notes, dir, game)
}

// hasDataset reports whether a directory of dir holds a manifest.
func hasDataset(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(dir, e.Name(), dataset.ManifestName)); e.IsDir() && !strings.HasPrefix(e.Name(), ".") && err == nil {
			return true
		}
	}
	return false
}
