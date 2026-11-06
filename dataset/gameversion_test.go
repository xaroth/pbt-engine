package dataset_test

import (
	"testing"

	"github.com/xaroth/pbt-engine/dataset"
)

func TestGameVersion(t *testing.T) {
	t.Parallel()
	parse := func(s string) dataset.GameVersion {
		t.Helper()
		v, err := dataset.ParseGameVersion(s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	testCases := []struct {
		a, b        string
		compare     int
		aHasPrefixB bool
	}{
		{"3.29.3.3", "3.29.3.3", 0, true},
		{"3.29.3.3", "3.29", 1, true},
		{"3.29", "3.29.0.0", -1, false},
		{"3.28.0.16", "3.29", -1, false},
		{"3.10", "3.9", 1, false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.a+" "+testCase.b, func(t *testing.T) {
			t.Parallel()
			a, b := parse(testCase.a), parse(testCase.b)
			if got := a.Compare(b); got != testCase.compare {
				t.Errorf("Compare: got %d, want %d", got, testCase.compare)
			}
			if got := a.HasPrefix(b); got != testCase.aHasPrefixB {
				t.Errorf("HasPrefix: got %v, want %v", got, testCase.aHasPrefixB)
			}
			if a.String() != testCase.a {
				t.Errorf("String: got %q, want %q", a.String(), testCase.a)
			}
		})
	}
	if got := parse("3.29.3.3").League(); got != parse("3.29") || got.String() != "3.29" {
		t.Errorf("League: got %q, want 3.29", got)
	}
	if a, b := parse("3.29.3.3"), parse("3.29.3.3"); a != b || parse("3.29") == parse("3.29.0") {
		t.Error("==: want equal versions equal and a prefix unequal to its extension")
	}
	seen := map[dataset.GameVersion]string{parse("3.28.0.16"): "a", parse("3.29"): "b"}
	if seen[parse("3.29")] != "b" || seen[parse("3.29.0")] != "" {
		t.Errorf("map key: got %v, want 3.29 found and 3.29.0 not", seen)
	}
	var zero dataset.GameVersion
	if !zero.IsZero() || zero.String() != "" || zero.Compare(parse("0")) >= 0 {
		t.Errorf("zero value: IsZero %v, String %q, want true, \"\", ordering first", zero.IsZero(), zero.String())
	}
}

func TestParseGameVersionRejects(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"", "3.x", "3..29", "3.029", "-1.2", "3.29.", "3.29.3.3.1", "3.65536"} {
		t.Run(s, func(t *testing.T) {
			t.Parallel()
			if _, err := dataset.ParseGameVersion(s); err == nil {
				t.Errorf("ParseGameVersion(%q): got no error", s)
			}
		})
	}
}
