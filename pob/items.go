package pob

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/items"
	"github.com/xaroth/pbt-engine/pob/internal/pobxml"
)

// ItemSlot returns the build slot of Path of Building's equipment slot name, such as "Ring 3",
// and false for a name that is not one of them.
func ItemSlot(name string) (build.Slot, bool) {
	s, ok := itemSlots()[name]
	return s, ok
}

// itemSlots maps PoB's names of the equipment slots: those with sockets, the amulet, three rings
// and the belt.
func itemSlots() map[string]build.Slot {
	out := slots()
	maps.Copy(out, map[string]build.Slot{
		"Amulet": build.SlotAmulet, "Ring 1": build.SlotRing1, "Ring 2": build.SlotRing2, "Ring 3": build.SlotRing3,
		"Belt": build.SlotBelt,
	})
	return out
}

// abyssalSlot returns the item slot and index of Path of Building's name of an abyssal socket,
// such as "Belt Abyssal Socket 1", and false for a name that is not one.
func abyssalSlot(name string) (build.Slot, int, bool) {
	item, n, ok := strings.Cut(name, " Abyssal Socket ")
	if !ok {
		return "", 0, false
	}
	slot, ok := ItemSlot(item)
	i, err := strconv.Atoi(n)
	if !ok || err != nil || i < 1 {
		return "", 0, false
	}
	return slot, i, true
}

// abyssJewel is a jewel PoB puts in an abyssal socket, read and waiting for its item.
type abyssJewel struct {
	slot  build.Slot
	index int
	name  string
	item  build.Item
}

// items equips the active item set's items in the equipment slots, weapon swap included, with
// the jewels in their abyssal sockets, and puts the active spec's jewels in their tree sockets,
// each read through the dataset's items group. Every other item warns: a jewel in a socket of a
// cluster jewel, an abyss jewel whose item is not imported, an item in a slot the build does
// not have (flasks, grafts), one the set does not equip, and one that does not read. The reader
// is built only when an item is to be read.
func (im *importer) items(store *dataset.Store) (build.Items, build.Jewels, error) {
	jewels := im.socketNodes()
	in := map[int][]string{}
	for _, e := range im.file.Equipped {
		in[e.Item] = append(in[e.Item], e.Slot)
	}
	var out build.Items
	var socketed build.Jewels
	var abyss []abyssJewel
	read := itemReader(store)
	for _, it := range im.file.Items {
		if node, ok := jewels[it.ID]; ok {
			if err := im.socket(&socketed, it, node, read); err != nil {
				return nil, nil, err
			}
			continue
		}
		if len(in[it.ID]) == 0 {
			im.warnf(ItemIgnored, itemSubject(it), "not equipped in the active item set; not imported")
			continue
		}
		for _, name := range in[it.ID] {
			var err error
			if slot, i, ok := abyssalSlot(name); ok {
				err = im.readAbyssJewel(&abyss, it, slot, i, read)
			} else {
				err = im.equip(&out, it, name, read)
			}
			if err != nil {
				return nil, nil, err
			}
		}
	}
	for _, j := range abyss {
		im.setAbyssJewel(out, j)
	}
	return out, socketed, nil
}

// socketNodes returns the tree socket node of each item id the active spec puts in one.
func (im *importer) socketNodes() map[int]uint32 {
	out := map[int]uint32{}
	for _, s := range im.spec.Sockets {
		if s.Item != 0 {
			out[s.Item] = s.Node
		}
	}
	return out
}

// readAbyssJewel reads it, in abyssal socket i of slot, into out, or warns.
func (im *importer) readAbyssJewel(out *[]abyssJewel, it pobxml.Item, slot build.Slot, i int, read func(pobxml.Item) (build.Item, error)) error {
	item, err := read(it)
	switch {
	case err == nil:
		*out = append(*out, abyssJewel{slot: slot, index: i, name: itemSubject(it), item: item})
	case ignorable(err):
		im.warnf(ItemIgnored, itemSubject(it), "not read: %v; not imported", err)
	default:
		return fmt.Errorf("pob: %w", err)
	}
	return nil
}

// setAbyssJewel puts j in its item in out, or warns when that item is not imported.
func (im *importer) setAbyssJewel(out build.Items, j abyssJewel) {
	host, ok := out[j.slot]
	if !ok {
		im.warnf(ItemIgnored, j.name, "in an abyssal socket of %s, which holds no imported item; not imported", j.slot)
		return
	}
	if host.AbyssJewels == nil {
		host.AbyssJewels = map[int]build.Item{}
	}
	host.AbyssJewels[j.index] = j.item
	out[j.slot] = host
}

// socket reads it into out at the socket node, or warns.
func (im *importer) socket(out *build.Jewels, it pobxml.Item, node uint32, read func(pobxml.Item) (build.Item, error)) error {
	if node > maxTreeNode {
		im.warnf(JewelIgnored, nodeSubject(node), "jewel %q is in a cluster jewel's socket; not imported", it.Name)
		return nil
	}
	item, err := read(it)
	switch {
	case err == nil:
		if *out == nil {
			*out = build.Jewels{}
		}
		(*out)[node] = item
	case ignorable(err):
		im.warnf(JewelIgnored, nodeSubject(node), "jewel %q not read: %v; not imported", it.Name, err)
	default:
		return fmt.Errorf("pob: %w", err)
	}
	return nil
}

// equip reads it into out at PoB's slot name, or warns.
func (im *importer) equip(out *build.Items, it pobxml.Item, name string, read func(pobxml.Item) (build.Item, error)) error {
	slot, ok := ItemSlot(name)
	if !ok {
		im.warnf(ItemIgnored, itemSubject(it), "slot %q is not an equipment slot; not imported", name)
		return nil
	}
	item, err := read(it)
	switch {
	case err == nil:
		if *out == nil {
			*out = build.Items{}
		}
		(*out)[slot] = item
	case ignorable(err):
		im.warnf(ItemIgnored, itemSubject(it), "not read: %v; not imported", err)
	default:
		return fmt.Errorf("pob: %w", err)
	}
	return nil
}

func itemSubject(it pobxml.Item) string { return cmp.Or(it.Name, "item "+strconv.Itoa(it.ID)) }

// itemReader returns a function that reads an item's text, building the reader on first use.
func itemReader(store *dataset.Store) func(pobxml.Item) (build.Item, error) {
	var r *items.Reader
	var rerr error
	return func(it pobxml.Item) (build.Item, error) {
		if r == nil && rerr == nil {
			r, rerr = items.NewReader(store)
		}
		if rerr != nil {
			return build.Item{}, fmt.Errorf("%w", rerr)
		}
		item, _, err := r.Parse(it.Text)
		if err != nil {
			return build.Item{}, fmt.Errorf("%w", err)
		}
		return item, nil
	}
}

// ignorable is an error that leaves an item out: text the reader does not take, or a dataset
// that cannot read items.
func ignorable(err error) bool {
	_, missing := errors.AsType[*dataset.MissingGroupError](err)
	return missing || errors.Is(err, items.ErrFormat) || errors.Is(err, items.ErrUnknownBase) || errors.Is(err, items.ErrNoCrafting)
}
