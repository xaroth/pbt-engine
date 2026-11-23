package pob

import (
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/items"
)

// itemWriter writes a build's items as PoB's items and item sets.
type itemWriter struct {
	ex *exporter
	// names are the English names of the bases by id.
	names map[string]string
	out   outItems
	// equipped is the id of the character's item in each slot.
	equipped map[build.Slot]int
}

// newItemWriter returns a writer of b's items with the item set of the character's, which is
// active, and the names of the bases of text.
func (ex *exporter) newItemWriter(text *dataset.Text) *itemWriter {
	names := make(map[string]string, len(text.BaseNames))
	for _, n := range text.BaseNames {
		names[n.ID] = n.Name
	}
	return &itemWriter{ex: ex, names: names, out: outItems{Active: 1, Sets: []outItemSet{{ID: 1}}}, equipped: map[build.Slot]int{}}
}

// character writes the character's items in item set 1, each with its abyss jewels in their
// abyssal sockets, and returns the jewels in their tree sockets.
func (w *itemWriter) character() []outSocket {
	for _, s := range slices.Sorted(maps.Keys(w.ex.b.Items)) {
		name := pobItemSlot(s)
		if name == "" {
			w.ex.warnf(ItemNotExported, string(s), "PoB has no slot %s; not exported", s)
			continue
		}
		it := w.ex.b.Items[s]
		id, ok := w.write(string(s), it)
		if !ok {
			continue
		}
		w.equipped[s] = id
		set := &w.out.Sets[0]
		set.Slots = append(set.Slots, outSlot{Name: name, Item: id})
		for _, i := range slices.Sorted(maps.Keys(it.AbyssJewels)) {
			if id, ok := w.write(fmt.Sprintf("%s abyssal socket %d", s, i), it.AbyssJewels[i]); ok {
				set.Slots = append(set.Slots, outSlot{Name: fmt.Sprintf("%s Abyssal Socket %d", name, i), Item: id})
			}
		}
	}
	var sockets []outSocket
	for _, node := range slices.Sorted(maps.Keys(w.ex.b.Jewels)) {
		it := w.ex.b.Jewels[node]
		if id, ok := w.add(it); ok {
			sockets = append(sockets, outSocket{Node: node, Item: id})
		} else {
			w.ex.warnNodef(ItemNotExported, node, baseUnnamed, it.Base)
		}
	}
	return sockets
}

// minionSet writes gear in an item set of its own and returns the set's id.
func (w *itemWriter) minionSet(group int, gear build.Items) int {
	set := outItemSet{ID: len(w.out.Sets) + 1}
	for _, s := range slices.Sorted(maps.Keys(gear)) {
		subject, name := fmt.Sprintf("skill group %d minion %s", group+1, s), pobItemSlot(s)
		if name == "" {
			w.ex.warnf(ItemNotExported, subject, "PoB has no slot %s; not exported", s)
			continue
		}
		if id, ok := w.write(subject, gear[s]); ok {
			set.Slots = append(set.Slots, outSlot{Name: name, Item: id})
		}
	}
	w.out.Sets = append(w.out.Sets, set)
	return set.ID
}

// source is PoB's source of a group the item in slot s grants, and false when that item is not
// written.
func (w *itemWriter) source(s build.Slot) (string, bool) {
	id, ok := w.equipped[s]
	if !ok {
		return "", false
	}
	it := w.ex.b.Items[s]
	return "Item:" + strconv.Itoa(id) + ":" + pobItemName(it, w.names[it.Base]), true
}

const baseUnnamed = "base %q has no English name; not exported"

// write adds it as an item and returns its id, or warns about subject when its base has no
// English name.
func (w *itemWriter) write(subject string, it build.Item) (int, bool) {
	id, ok := w.add(it)
	if !ok {
		w.ex.warnf(ItemNotExported, subject, baseUnnamed, it.Base)
	}
	return id, ok
}

// add adds it as an item and returns its id, or false when its base has no English name.
func (w *itemWriter) add(it build.Item) (int, bool) {
	base, ok := w.names[it.Base]
	if !ok {
		return 0, false
	}
	id := len(w.out.Items) + 1
	w.out.Items = append(w.out.Items, outItem{ID: id, Text: "\n" + items.PoBText(it, base)})
	return id, true
}

// pobItemName is PoB's name of an item: the name and base of a rare or unique item, else the base
// or a magic item's full name.
func pobItemName(it build.Item, base string) string {
	switch {
	case it.Rarity == build.Rare || it.Rarity == build.Unique:
		return it.Name + ", " + base
	case it.Name != "":
		return it.Name
	}
	return base
}

// pobItemSlot is the inverse of itemSlots.
func pobItemSlot(s build.Slot) string {
	for name, slot := range itemSlots() {
		if slot == s {
			return name
		}
	}
	return ""
}

type outItems struct {
	Active int          `xml:"activeItemSet,attr"`
	Items  []outItem    `xml:"Item"`
	Sets   []outItemSet `xml:"ItemSet"`
}

type outItem struct {
	ID   int    `xml:"id,attr"`
	Text string `xml:",chardata"`
}

type outItemSet struct {
	ID    int       `xml:"id,attr"`
	Slots []outSlot `xml:"Slot"`
}

type outSlot struct {
	Name string `xml:"name,attr"`
	Item int    `xml:"itemId,attr"`
}

type outSocket struct {
	Node uint32 `xml:"nodeId,attr"`
	Item int    `xml:"itemId,attr"`
}
