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
// version to: the English text for class names, and the skills group for gems.
func ImportFiles(m dataset.Manifest) []string {
	return m.Needs(dataset.TextFile("en"), dataset.SkillsFile)
}

// ItemFiles returns the files Data.Item reads: the items group, its mods and the English text.
func ItemFiles(m dataset.Manifest) []string {
	return m.Needs(dataset.ItemsFile, dataset.ModsFile, dataset.TextFile("en"))
}

// NodeFiles returns the files Data.Node and Data.Classes read for the tree variant: its tree
// and the English text.
func NodeFiles(m dataset.Manifest, variant string) []string {
	return m.Needs(dataset.TreeFile(variant), dataset.TextFile("en"))
}

// SimulateFiles returns the files Simulate reads for each of builds: the tree of its variant,
// the skills group when it has gems and the items group when it has items. Compare, Sweep and
// Stats.Explain read no more than the simulations of the builds they take or make; Stats.Text
// reads the text of its language.
func SimulateFiles(m dataset.Manifest, builds ...*build.Build) []string {
	files := []string{dataset.StatsFile, dataset.CharactersFile, dataset.ConstantsFile, dataset.QuestsFile}
	for _, b := range builds {
		files = append(files, dataset.TreeFile(cmp.Or(b.Variant, build.DefaultVariant)))
		if slices.ContainsFunc(b.Skills.Groups, func(g build.SkillGroup) bool { return len(g.Gems) > 0 }) {
			files = append(files, dataset.SkillsFile)
		}
		if len(b.Items) > 0 {
			files = append(files, dataset.ItemsFile)
		}
	}
	return m.Needs(files...)
}
