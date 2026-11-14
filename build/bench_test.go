package build_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/benchbuilds"
)

// BenchmarkCheck checks each benchmark build against the pinned current dataset, as every
// simulation and host call does (target: < 0.2 ms native).
func BenchmarkCheck(b *testing.B) {
	d := benchbuilds.Open(b)
	for _, c := range benchbuilds.Builds(b, d) {
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
