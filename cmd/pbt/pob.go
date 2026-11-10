package main

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/pob"
)

// loadBuild reads a build file: a Path of Building build when it starts with '<', else build
// JSON. A PoB build's import warnings go to notes, folded per kind unless --warnings all.
func loadBuild(in invocation, notes io.Writer, path string) (*build.Build, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read build: %w", err)
	}
	if !isPoB(data) {
		b, err := build.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return b, nil
	}
	b, ws, err := importPoB(in, notes, data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	p := &printer{w: notes}
	printImportWarnings(p, ws, in.opts["warnings"] == warningsAll)
	return b, p.err
}

func isPoB(data []byte) bool { return bytes.HasPrefix(bytes.TrimSpace(data), []byte("<")) }

func importPoB(in invocation, notes io.Writer, data []byte) (*build.Build, []build.Warning, error) {
	reg, err := in.registry(notes, in.opts["game"])
	if err != nil {
		return nil, nil, err
	}
	b, ws, err := pob.FromXML(data, reg)
	if err != nil {
		return nil, nil, fmt.Errorf("import: %w", err)
	}
	return b, ws, nil
}

// printImportWarnings prints import warnings; unless all, a kind seen more than once is one
// line with its count.
func printImportWarnings(p *printer, ws []build.Warning, all bool) {
	if all {
		printWarnings(p, ws)
		return
	}
	var order []build.Code
	count := map[build.Code]int{}
	first := map[build.Code]build.Warning{}
	for _, w := range ws {
		if count[w.Code] == 0 {
			order = append(order, w.Code)
			first[w.Code] = w
		}
		count[w.Code]++
	}
	folded := false
	for _, c := range order {
		if count[c] == 1 {
			p.printf("warning %s\n", first[c])
			continue
		}
		p.printf("warning %s: %d (%s, ...)\n", c, count[c], first[c].Subject)
		folded = true
	}
	if folded {
		p.printf("(--warnings all lists every import warning)\n")
	}
}

// buildImport imports a PoB build and writes it as build JSON; warnings go to notes, all of them.
func buildImport(in invocation) error {
	path := in.pos[2]
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read build: %w", err)
	}
	if !isPoB(data) {
		return fmt.Errorf("%s: %w", path, errNotPoB)
	}
	b, ws, err := importPoB(in, in.notes, data)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	out, err := b.Encode()
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	p := &printer{w: in.notes}
	printWarnings(p, ws)
	if p.err != nil {
		return p.err
	}
	if _, err := in.w.Write(out); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}
