package build

// Jewels are a build's jewels by the id of the jewel socket node they are in.
type Jewels map[uint32]Item

// Edit ops of jewels.
const (
	OpSocketJewel   EditOp = "socket-jewel"
	OpUnsocketJewel EditOp = "unsocket-jewel"
)

// SocketJewel puts it in the jewel socket node, replacing what is there. Whether it fits is
// not checked here; Check warns.
func SocketJewel(node uint32, it Item) Edit {
	return Edit{Op: OpSocketJewel, Node: node, Item: it.Clone()}
}

// UnsocketJewel empties the jewel socket node.
func UnsocketJewel(node uint32) Edit { return Edit{Op: OpUnsocketJewel, Node: node} }

func (b *Build) applyJewel(e Edit) {
	if e.Op == OpUnsocketJewel {
		delete(b.Jewels, e.Node)
		return
	}
	if b.Jewels == nil {
		b.Jewels = Jewels{}
	}
	var it Item
	if e.Item != nil {
		it = *e.Item.Clone()
	}
	b.Jewels[e.Node] = it
}
