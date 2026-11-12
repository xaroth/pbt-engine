// Package realdata names the real datasets the tests read from $PBT_DATASETS and opens them.
// The README names the pbt-data commit that holds them.
package realdata

import (
	"os"
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
)

// The pinned poe1 datasets: Previous is the league before Current.
const (
	Previous = "3.28.0.16"
	Current  = "3.29.3.6"
)

// ids are the dataset ids of the pinned versions.
//
//nolint:gochecknoglobals // a constant table
var ids = map[string]string{
	Previous: "fd0a479473e295844fc9e4d896c06c9d646694b8d75a6513b8ac0cb46731ce07",
	Current:  "5df73a424e257851d202b776d94271ae29831fdfa6e04ccdf2784b9de279ae75",
}

// Versions are the pinned versions, oldest first.
func Versions() []string {
	return []string{Previous, Current}
}

// Root is $PBT_DATASETS, and skips tb without it.
func Root(tb testing.TB) string {
	tb.Helper()
	root := os.Getenv("PBT_DATASETS")
	if root == "" {
		tb.Skip("PBT_DATASETS not set")
	}
	return root
}

// Open opens the pinned dataset of exactly version among the datasets under $PBT_DATASETS (one
// per directory: pulled from pbt-data, or one per version). It skips without $PBT_DATASETS and
// fails when the dataset is missing or is not the pinned one, so a stale checkout cannot pass
// by skipping.
func Open(tb testing.TB, version string) *dataset.Store {
	tb.Helper()
	want, ok := ids[version]
	if !ok {
		tb.Fatalf("realdata: %s is not a pinned version", version)
	}
	reg, err := dataset.OpenRoot(os.DirFS(Root(tb)))
	if reg == nil {
		tb.Fatal(err)
	}
	res, err := reg.Resolve("poe1", version)
	if err != nil {
		tb.Fatalf("dataset %s: %v", version, err)
	}
	m := res.Store.Manifest()
	if m.Version != version || m.ID != want {
		tb.Fatalf("PBT_DATASETS has poe1 %s (id %s), want %s (id %s): check out the pbt-data commit the README names", m.Version, m.ID, version, want)
	}
	return res.Store
}
