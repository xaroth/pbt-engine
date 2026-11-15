package build

import (
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/poe1/radius"
	"github.com/xaroth/pbt-engine/internal/poe1/timeless"
)

// Load makes now what Check makes from s the first time a build needs it, for every tree variant
// of s, so that first Check is as fast as later ones: the index of jewel radii, the alternate
// trees timeless jewels conquer with and the cluster jewel generator. One that fails to be made
// fails Check as it would have without Load.
func Load(s *dataset.Store) {
	for _, v := range s.Manifest().Trees() {
		_, _ = radius.Of(s, v)
		_, _ = timeless.VersionsOf(s, v)
		_, _ = clusterGenerator(s, v)
	}
}
