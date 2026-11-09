package main

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/xaroth/pbt-engine/output"
	"github.com/xaroth/pbt-engine/power"
	"github.com/xaroth/pbt-engine/resolve"
	"github.com/xaroth/pbt-engine/rules/poe1"
	"github.com/xaroth/pbt-engine/simulator"
)

var errMode = errors.New("--mode wants alone or path")

// Modes of pbt power, and the decimals its gains show.
const (
	modeAlone  = "alone"
	modePath   = "path"
	gainPlaces = 2
)

// powerArgs are pbt power's options, checked.
type powerArgs struct {
	by         output.Key
	mode       string
	top, limit int
}

func readPowerArgs(opts map[string]string) (powerArgs, error) {
	a := powerArgs{by: output.Key(opts["by"]), mode: opts["mode"]}
	if a.mode != modeAlone && a.mode != modePath {
		return a, fmt.Errorf("%w: %q", errMode, a.mode)
	}
	var err error
	if a.top, err = strconv.Atoi(opts["top"]); err != nil || a.top < 0 {
		return a, fmt.Errorf("--top %q: %w", opts["top"], errLimit)
	}
	if a.limit, err = strconv.Atoi(cmp.Or(opts["max-distance"], "0")); err != nil || a.limit < 0 {
		return a, fmt.Errorf("--max-distance %q: %w", opts["max-distance"], errLimit)
	}
	return a, nil
}

// nodePower ranks the candidates of a build by the change of one output: alone, or along the
// shortest path by gain per point spent.
func nodePower(in invocation) error {
	a, err := readPowerArgs(in.opts)
	if err != nil {
		return err
	}
	b, s, err := simFor(in, in.pos[1], in.opts["version"])
	if err != nil {
		return err
	}
	by, mode, top := a.by, a.mode, a.top
	nps, err := power.Nodes(s, b, power.Outputs(by), power.MaxDistance(a.limit))
	if err != nil {
		return fmt.Errorf("%s: %w", in.pos[1], err)
	}
	score := func(np power.NodePower) float64 {
		if mode == modeAlone {
			return np.Alone.Changes[by].Abs
		}
		return np.ViaPath.Changes[by].Abs / float64(max(np.Points, 1))
	}
	slices.SortStableFunc(nps, func(a, c power.NodePower) int { return cmp.Compare(score(c), score(a)) })
	nm, err := namer(s)
	if err != nil {
		return err
	}
	p := &printer{w: in.w}
	p.printf("%s by %s, %s, %d candidates on dataset %s\n", in.pos[1], by, modeLabel(mode), len(nps), s.Store().Manifest().Version)
	for i, np := range nps[:min(top, len(nps))] {
		p.printf("%3d  %10s", i+1, number(score(np), gainPlaces))
		if mode == modePath {
			p.printf("  %10s in %2d", number(np.ViaPath.Changes[by].Abs, gainPlaces), np.Points)
		}
		p.printf("  %s\n", candidateName(s, nm, np))
	}
	return p.err
}

func modeLabel(mode string) string {
	if mode == modePath {
		return "gain per point along the shortest path (gain, points)"
	}
	return "the node alone"
}

func namer(s *simulator.Sim) (*poe1.TextNamer, error) {
	text, err := s.Store().Text("en")
	if err != nil {
		return nil, fmt.Errorf("names: %w", err)
	}
	nm, err := poe1.NewNamer(text)
	if err != nil {
		return nil, fmt.Errorf("names: %w", err)
	}
	return nm, nil
}

// candidateName names a node, or a mastery and its effect's text.
func candidateName(s *simulator.Sim, nm *poe1.TextNamer, np power.NodePower) string {
	src := poe1.Source{Bundle: &resolve.Bundle{Source: resolve.NodeSource{ID: np.Node}}}
	name := nm.Name(src)
	if np.Effect == 0 {
		return name
	}
	bd, err := resolve.MasteryEffectBundle(s.Store(), "standard", np.Node, np.Effect)
	if err != nil {
		return fmt.Sprintf("%s effect %d", name, np.Effect)
	}
	return fmt.Sprintf("%s: %s", name, strings.Join(nm.Lines(poe1.Source{Bundle: &bd}), " / "))
}
