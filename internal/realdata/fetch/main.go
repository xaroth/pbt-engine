// Command fetch puts the pinned real datasets into a directory, one per version, from pbt-data's
// bucket by dataset id. It reuses what the directory already holds.
//
//	go run ./internal/realdata/fetch DIR [VERSION...]
//
// Without versions it fetches every pinned version.
package main

import (
	"context"
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

const exitUsage = 2

var errNotPinned = errors.New("not a pinned version")

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
		versions = realdata.Versions()
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
	}
	return errors.Join(errs...)
}
