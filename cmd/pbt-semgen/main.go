// Command pbt-semgen generates a semantics table and its coverage report for one dataset.
//
//	pbt-semgen <dataset dir> <modcache.json> <overrides.json> <pob commit> <table out> <report out>
//
// modcache.json comes from modcache.lua run against the PoB checkout at the given commit. The
// table keeps only entries that feed the sheet (Table.SheetOnly); the report covers all.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/semantics"
	"github.com/xaroth/pbt-engine/statdesc"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "pbt-semgen:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 6 {
		return errors.New("usage: pbt-semgen <dataset dir> <modcache.json> <overrides.json> <pob commit> <table out> <report out>")
	}
	v, err := dataset.Open(os.DirFS(args[0]))
	if err != nil {
		return err
	}
	stats, err := v.Stats()
	if err != nil {
		return err
	}
	tree, err := v.Tree("standard")
	if err != nil {
		return err
	}
	text, err := v.Text("en")
	if err != nil {
		return err
	}
	set, err := statdesc.NewSet(text.Descriptions)
	if err != nil {
		return err
	}
	var cache []semantics.PoBLine
	if err := readJSON(args[1], &cache); err != nil {
		return err
	}
	var overrides []semantics.Entry
	if err := readJSON(args[2], &overrides); err != nil {
		return err
	}
	t, err := semantics.Generate(semantics.Input{
		Game: v.Game(), Version: v.Name(), PoB: args[3],
		Stats: stats, Tree: tree, Text: set, ModCache: cache, Overrides: overrides,
	})
	if err != nil {
		return err
	}
	out, err := t.SheetOnly().Encode()
	if err != nil {
		return err
	}
	if err := os.WriteFile(args[4], out, 0o644); err != nil {
		return err
	}
	return os.WriteFile(args[5], []byte(semantics.Coverage(t)), 0o644)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
