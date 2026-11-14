package resolve_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/internal/benchbuilds"
	"github.com/xaroth/pbt-engine/internal/poe1"
	"github.com/xaroth/pbt-engine/internal/resolve"
)

// BenchmarkSkills resolves each benchmark build's skills as a simulation does: the main skill
// alone, and every skill the build uses.
func BenchmarkSkills(b *testing.B) {
	d := benchbuilds.Open(b)
	for _, c := range benchbuilds.Builds(b, d) {
		checked, _, err := build.Check(c.Build, d.Store)
		if err != nil {
			b.Fatal(err)
		}
		main := c.Build.Skills.MainGroup
		b.Run(c.Name+"/main", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				r, err := resolve.NewSkillResolver(checked, poe1.GemRules())
				if err != nil {
					b.Fatal(err)
				}
				if _, _, err := r.Group(main); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(c.Name+"/all", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				r, err := resolve.NewSkillResolver(checked, poe1.GemRules())
				if err != nil {
					b.Fatal(err)
				}
				r.All()
			}
		})
	}
}
