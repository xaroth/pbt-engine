package simulator

import (
	"cmp"
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1/cluster"
)

// The functions below name the dataset files a call reads, as the manifest m lists them, so a
// host that fetches a dataset over the network can fetch them before the call. Each list
// includes the files they are checked against on load (dataset.Manifest.Needs). A file m does
// not list, such as an optional group, is left out: the call goes on without it as it does
// when the dataset lacks it.

// ImportFiles returns the files pob.FromXML reads from the dataset it resolves the file's
// version to, for a file of the tree variant pob.FileVersion gives: the English text for class
// names, the tree of variant for the bloodline, the skills group for gems, and the items group,
// its mods and the standard tree for the equipped items (ItemFiles). A file without a
// bloodline reads no tree of its variant, and one without equipped items reads no items, mods
// or standard tree.
func ImportFiles(m dataset.Manifest, variant string) []string {
	return m.Needs(dataset.TextFile("en"), dataset.TreeFile(variant), dataset.SkillsFile, dataset.ItemsFile, dataset.ModsFile,
		dataset.TreeFile(build.DefaultVariant))
}

// ItemFiles returns the files Data.Item reads: the items group, its mods, the English text and
// the standard tree, for the names of the passives lines allocate.
func ItemFiles(m dataset.Manifest) []string {
	return m.Needs(dataset.ItemsFile, dataset.ModsFile, dataset.TextFile("en"), dataset.TreeFile(build.DefaultVariant))
}

// SimulateFiles returns the files Simulate reads for each of builds: the tree of its variant,
// the skills group, for its gems or else the default attack, the items group when it has items or jewels, its mods
// when it has gems and an item with stats in a slot with sockets, which can grant supports, or
// a body armour with stats, whose Life Modifiers a mastery looks at (poe1.ModTags), the
// layout of its variant and the English text when it has a cluster jewel, whose lines say
// which passives it adds, and the layout when it has a jewel that transforms the attributes of
// the passives in its radius (build.TransformsAttributes) or otherwise changes them
// (build.ChangesRadiusPassives). Compare, Sweep and Stats.Explain read no more than the simulations
// of the builds they take or make; Stats.Text reads the text of its language.
func SimulateFiles(m dataset.Manifest, builds ...*build.Build) []string {
	files := []string{dataset.StatsFile, dataset.CharactersFile, dataset.ConstantsFile, dataset.QuestsFile, dataset.SkillsFile}
	for _, b := range builds {
		files = append(files, dataset.TreeFile(cmp.Or(b.Variant, build.DefaultVariant)))
		if len(b.Items) > 0 || len(b.Jewels) > 0 {
			files = append(files, dataset.ItemsFile)
		}
		if slices.ContainsFunc(b.Skills.Groups, func(g build.SkillGroup) bool { return len(g.Gems) > 0 }) && socketedStats(b.Items) ||
			hasStats(b.Items[build.SlotBodyArmour]) {
			files = append(files, dataset.ModsFile)
		}
		files = append(files, jewelFiles(b)...)
	}
	return m.Needs(files...)
}

// jewelFiles returns the files b's jewels read beside the items group.
func jewelFiles(b *build.Build) []string {
	var clusterJewel, transform bool
	for _, it := range b.Jewels {
		clusterJewel = clusterJewel || cluster.IsJewelBase(it.Base)
		transform = transform || build.TransformsAttributes(&it) || build.ChangesRadiusPassives(&it)
	}
	layout := dataset.LayoutFile(cmp.Or(b.Variant, build.DefaultVariant))
	switch {
	case clusterJewel:
		return []string{layout, dataset.TextFile("en")}
	case transform:
		return []string{layout}
	}
	return nil
}

// socketedStats reports whether an item in a slot with sockets has stats.
func socketedStats(items build.Items) bool {
	return slices.ContainsFunc(build.Slots(), func(s build.Slot) bool { return hasStats(items[s]) })
}

// hasStats reports whether a mod line of it has stats.
func hasStats(it build.Item) bool {
	return slices.ContainsFunc(it.Mods, func(m build.ModLine) bool { return len(m.Stats) > 0 })
}
