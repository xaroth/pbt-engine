package build

import (
	"fmt"
	"maps"
	"slices"

	"github.com/xaroth/pbt-engine/dataset"
)

// Warning codes of Check for jewels.
const (
	JewelNotSocket    Code = "jewel-not-socket"
	JewelNotAllocated Code = "jewel-not-allocated"
	JewelKindMismatch Code = "jewel-kind-mismatch"
	JewelTooLarge     Code = "jewel-too-large"
)

// jewelClasses are the ItemClass.IDs a jewel socket takes. Cluster jewels are of class Jewel.
func jewelClasses() []string { return []string{"Jewel", "AbyssJewel"} }

// checkJewels leaves out jewels in a node that is not a jewel socket and, with data, jewels of
// an unknown base or of a class a socket does not take, each with a warning, and warns about
// jewels kept in a socket that is not allocated. Without data, bases go unchecked. A cluster
// jewel larger than its socket, or in a socket that takes none on a tree that names its
// cluster sockets, is left out with a warning.
func (c *Checked) checkJewels(data *dataset.Items) {
	for _, id := range slices.Sorted(maps.Keys(c.build.Jewels)) {
		it := c.build.Jewels[id]
		if !c.checkJewel(id, &it, data) {
			delete(c.build.Jewels, id)
			continue
		}
		c.build.Jewels[id] = it
	}
	if len(c.build.Jewels) == 0 {
		c.build.Jewels = nil
	}
}

// checkJewel fixes the jewel it in node id as the dataset says, or reports false to leave it
// out.
func (c *Checked) checkJewel(id uint32, it *Item, data *dataset.Items) bool {
	subject := jewelSubject(id)
	if w, bad := c.socketProblem(id); bad {
		c.warnings = append(c.warnings, w)
		return false
	}
	if data != nil {
		base := c.itemBase(Warning{Subject: subject}, it, data)
		if base == nil {
			return false
		}
		if !slices.Contains(jewelClasses(), base.Class) {
			c.warnf(JewelKindMismatch, subject, "a %s does not fit a jewel socket; left out", base.Class)
			return false
		}
		c.checkMods(Warning{Subject: subject}, it, c.clusterRead[id])
	}
	if !c.clusterFits(id, it) {
		return false
	}
	if c.allocated(id) == nil {
		c.warnf(JewelNotAllocated, subject, "socket not allocated; kept, it counts once allocated")
	}
	return true
}

func jewelSubject(id uint32) string { return fmt.Sprintf("jewel %d", id) }

// socketProblem returns the warning for a jewel in node id when the node is not a jewel socket.
func (c *Checked) socketProblem(id uint32) (Warning, bool) {
	if n := c.node(id); n != nil && n.Kind == dataset.JewelSocket {
		return Warning{}, false
	}
	return Warning{
		Code: JewelNotSocket, Subject: jewelSubject(id),
		Message: fmt.Sprintf("node %d is not a jewel socket of the %s tree; jewel left out", id, c.build.Variant),
	}, true
}

// clusterFits reports whether the jewel it, if a cluster jewel, fits socket id, or warns.
func (c *Checked) clusterFits(id uint32, it *Item) bool {
	cj := c.clusterJewel(it.Base)
	if cj == nil {
		return true
	}
	sock, ok := c.tree.ClusterSocket(id)
	switch {
	case !ok && len(c.tree.Clusters.Sockets) > 0:
		c.warnf(JewelKindMismatch, jewelSubject(id), "a cluster jewel does not fit a socket that takes none; left out")
		return false
	case ok && cj.Size > sock.Size:
		c.warnf(JewelTooLarge, jewelSubject(id), "a %s cluster jewel does not fit a %s socket; left out", cj.Size, sock.Size)
		return false
	}
	return true
}
