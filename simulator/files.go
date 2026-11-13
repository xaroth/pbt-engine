package simulator

import (
	"cmp"
	"slices"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// The functions below name the dataset files a call reads, as the manifest m lists them, so a
// host that fetches a dataset over the network can fetch them before the call. Each list
// includes the files they are checked against on load (dataset.Manifest.Needs). A file m does
// not list, such as an optional group, is left out: the call goes on without it as it does
// when the dataset lacks it.

// ImportFiles returns the files pob.FromXML reads from the dataset it resolves the file's
// version to, for a file of the tree variant pob.FileVersion gives: the English text for class
// names, the tree of variant for the bloodline, the skills group for gems, and the items group
// and its mods for the equipped items. A file without a bloodline reads no tree, and one
// without equipped items reads no items or mods.
func ImportFiles(m dataset.Manifest, variant string) []string {
	return m.Needs(dataset.TextFile("en"), dataset.TreeFile(variant), dataset.SkillsFile, dataset.ItemsFile, dataset.ModsFile)
}

// ItemFiles returns the files Data.Item reads: the items group, its mods and the English text.
func ItemFiles(m dataset.Manifest) []string {
	return m.Needs(dataset.ItemsFile, dataset.ModsFile, dataset.TextFile("en"))
}

// SimulateFiles returns the files Simulate reads for each of builds: the tree of its variant,
// the skills group when it has gems and the items group when it has items or jewels. Compare,
// Sweep and Stats.Explain read no more than the simulations of the builds they take or make;
// Stats.Text reads the text of its language.
func SimulateFiles(m dataset.Manifest, builds ...*build.Build) []string {
	files := []string{dataset.StatsFile, dataset.CharactersFile, dataset.ConstantsFile, dataset.QuestsFile}
	for _, b := range builds {
		files = append(files, dataset.TreeFile(cmp.Or(b.Variant, build.DefaultVariant)))
		if slices.ContainsFunc(b.Skills.Groups, func(g build.SkillGroup) bool { return len(g.Gems) > 0 }) {
			files = append(files, dataset.SkillsFile)
		}
		if len(b.Items) > 0 || len(b.Jewels) > 0 {
			files = append(files, dataset.ItemsFile)
		}
	}
	return m.Needs(files...)
}
