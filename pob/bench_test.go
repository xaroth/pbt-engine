package pob_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/internal/benchbuilds"
	"github.com/xaroth/pbt-engine/pob"
)

// BenchmarkFromXML imports each benchmark build's Path of Building file on the pinned current
// dataset.
func BenchmarkFromXML(b *testing.B) {
	d := benchbuilds.Open(b)
	for _, c := range benchbuilds.Builds(b, d) {
		b.Run(c.Name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, _, err := pob.FromXML(c.XML, d.Registry); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
