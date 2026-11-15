package build_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/benchbuilds"
	"github.com/xaroth/pbt-engine/items"
)

// BenchmarkCheck checks each benchmark build against the pinned current dataset, as every
// simulation and host call does (target: < 0.2 ms native), and a level 90 Marauder with a large
// cluster jewel in a socket.
func BenchmarkCheck(b *testing.B) {
	d := benchbuilds.Open(b)
	cases := benchbuilds.Builds(b, d)
	r, err := items.NewReader(d.Store)
	if err != nil {
		b.Fatal(err)
	}
	cases = append(cases, benchbuilds.Case{Name: "cluster-jewel", Build: clusterBuild(b, d.Store, readJewel(b, r, largeJewel))})
	for _, c := range cases {
		b.Run(c.Name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, _, err := build.Check(c.Build, d.Store); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
