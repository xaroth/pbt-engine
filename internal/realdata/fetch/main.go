// Command fetch puts the pinned real datasets into a directory, one per version, and their art
// manifests under art/, from pbt-data's bucket by id. It reuses what the directory already
// holds.
//
//	go run ./internal/realdata/fetch DIR [VERSION...]
//
// Without versions it fetches every pinned version.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/xaroth/pbt-engine/dataset/remote"
	"github.com/xaroth/pbt-engine/dataset/remote/httpfetch"
	"github.com/xaroth/pbt-engine/internal/realdata"
)

const (
	exitUsage = 2
	dirPerm   = 0o755
	filePerm  = 0o644
)

var (
	errNotPinned = errors.New("not a pinned version")
	errHash      = errors.New("content does not match its id")
)

func main() {
	flag.Usage = func() { fmt.Fprintln(os.Stderr, "usage: fetch DIR [VERSION...]") }
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(exitUsage)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := run(ctx, flag.Arg(0), flag.Args()[1:])
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, dir string, versions []string) error {
	if len(versions) == 0 {
		versions = realdata.Pinned()
	}
	src := httpfetch.Source{Base: remote.DefaultSource}
	var errs []error
	for _, v := range versions {
		id := realdata.ID(v)
		if id == "" {
			errs = append(errs, fmt.Errorf("%s: %w", v, errNotPinned))
			continue
		}
		r, err := remote.PullID(ctx, src, filepath.Join(dir, v), id)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		fmt.Fprintf(os.Stdout, "%s %s: %d files fetched\n", v, id, r.Fetched)
		if err := fetchArt(ctx, src, dir, realdata.ArtID(v)); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", v, err))
		}
	}
	return errors.Join(errs...)
}

// fetchArt puts the art manifest with id at its path under dir, unless it is there already.
func fetchArt(ctx context.Context, src remote.Fetcher, dir, id string) error {
	path := realdata.ArtPath(dir, id)
	if b, err := os.ReadFile(path); err == nil && hash(b) == id {
		return nil
	}
	b, err := src.Fetch(ctx, "manifests/"+id+".json")
	if err != nil {
		return fmt.Errorf("art %s: %w", id, err)
	}
	if hash(b) != id {
		return fmt.Errorf("art %s: %w", id, errHash)
	}
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("art %s: %w", id, err)
	}
	if err := os.WriteFile(path, b, filePerm); err != nil {
		return fmt.Errorf("art %s: %w", id, err)
	}
	fmt.Fprintf(os.Stdout, "art %s fetched\n", id)
	return nil
}

func hash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
