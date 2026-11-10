package poe1

import (
	"fmt"

	"github.com/xaroth/pbt-engine/internal/core"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// SetNodes turns node sources on for the allocated nodes and the chosen mastery effects and off
// for the rest, for a sheet opened over resolve.Tree. Other sources are left as they are.
func (sh *Sheet) SetNodes(nodes []uint32, masteries map[uint32]uint32) {
	on := make(map[uint32]bool, len(nodes))
	for _, n := range nodes {
		on[n] = true
	}
	for n, srcs := range sh.nodes {
		sh.setSources(srcs, on[n.ID] && (n.Effect == 0 || masteries[n.ID] == n.Effect))
	}
}

// SetNode turns node id's own sources on or off, and reports whether the sheet has the node.
// A node without stats has no sources: the sheet does not have it, and switching it changes
// nothing.
func (sh *Sheet) SetNode(id uint32, on bool) bool {
	srcs, ok := sh.nodes[resolve.NodeSource{ID: id}]
	sh.setSources(srcs, on)
	return ok
}

// SetMasteryEffect turns effect of mastery node on or off, and reports whether the sheet has it.
func (sh *Sheet) SetMasteryEffect(node, effect uint32, on bool) bool {
	srcs, ok := sh.nodes[resolve.NodeSource{ID: node, Effect: effect}]
	sh.setSources(srcs, on)
	return ok
}

func (sh *Sheet) setSources(srcs []core.SourceID, on bool) {
	for _, src := range srcs {
		// src is a source of this session, so this cannot fail.
		_ = sh.session.SetSourceEnabled(src, on)
	}
}

// Mark returns a checkpoint: switches after it can be undone with Revert. Until Commit the
// sheet refuses to add sources (ErrCheckpointOpen). Commit ends the journalling once no
// checkpoint is needed, as core.Session does.
func (sh *Sheet) Mark() core.Checkpoint {
	sh.marked = true
	return sh.session.Mark()
}

// Revert undoes every switch since cp.
func (sh *Sheet) Revert(cp core.Checkpoint) error {
	if err := sh.session.Revert(cp); err != nil {
		return fmt.Errorf("sheet: %w", err)
	}
	return nil
}

// Commit keeps every switch made so far and stops journalling; the sheet may add sources again.
func (sh *Sheet) Commit() {
	sh.marked = false
	sh.session.Commit()
}
