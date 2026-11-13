package poe1

import (
	"slices"
	"testing"

	"github.com/xaroth/pbt-engine/internal/realdata"
)

// Every skill type a table can name is an ActiveSkillType id, so a misspelled one cannot load
// and never match.
func TestSkillTypesInDataset(t *testing.T) {
	t.Parallel()
	for _, v := range realdata.Versions() {
		sk, err := realdata.Open(t, v).Skills()
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range skillTypes() {
			if !slices.Contains(sk.Types, id) {
				t.Errorf("%s: %s is not an ActiveSkillType", v, id)
			}
		}
	}
}
