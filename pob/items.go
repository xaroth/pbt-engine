package pob

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"strconv"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/items"
	"github.com/xaroth/pbt-engine/pob/internal/pobxml"
)

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

// items equips the active item set's items in the equipment slots, weapon swap included, each
// read through the dataset's items group. Every other item warns: a jewel in the tree, an item
// in a slot the build does not have (flasks, abyssal sockets, grafts), one the set does not
// equip, and one that does not read. The reader is built only when an item is to be read.
func (im *importer) items(store *dataset.Store) (build.Items, error) {
	jewels := map[int]uint32{}
	for _, s := range im.spec.Sockets {
		if s.Item != 0 {
			jewels[s.Item] = s.Node
		}
	}
	in := map[int][]string{}
	for _, e := range im.file.Equipped {
		in[e.Item] = append(in[e.Item], e.Slot)
	}
	var out build.Items
	read := itemReader(store)
	for _, it := range im.file.Items {
		if node, ok := jewels[it.ID]; ok {
			im.warnf(JewelIgnored, nodeSubject(node), "jewel %q not imported", it.Name)
			continue
		}
		if len(in[it.ID]) == 0 {
			im.warnf(ItemIgnored, itemSubject(it), "not equipped in the active item set; not imported")
			continue
		}
		for _, name := range in[it.ID] {
			if err := im.equip(&out, it, name, read); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// equip reads it into out at PoB's slot name, or warns.
func (im *importer) equip(out *build.Items, it pobxml.Item, name string, read func(pobxml.Item) (build.Item, error)) error {
	slot, ok := itemSlots()[name]
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
