package dataset_test

import (
	"os"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/dataset"
)

func TestCapabilitiesVersion(t *testing.T) {
	t.Parallel()
	c := dataset.Capabilities{"armour": 3}
	for name, want := range map[string]int{"armour": 3, "life-from-strength": 1} {
		if got := c.Version(name); got != want {
			t.Errorf("Version(%q): got %d, want %d", name, got, want)
		}
	}
	if got := dataset.Capabilities(nil).Version("armour"); got != 1 {
		t.Errorf("nil Version: got %d, want 1", got)
	}
}

func TestCapabilitiesSupported(t *testing.T) {
	t.Parallel()
	known := map[string]int{"armour": 2, "evasion": 1}
	testCases := []struct {
		name string
		c    dataset.Capabilities
		want string // error text; empty means supported
	}{
		{name: "none", c: nil},
		{name: "implemented version", c: dataset.Capabilities{"armour": 2}},
		{name: "newer version", c: dataset.Capabilities{"armour": 3}, want: "unsupported capabilities: armour 3 (implemented up to 2)"},
		{name: "unknown mechanic", c: dataset.Capabilities{"block": 2}, want: "unsupported capabilities: block 2 (unknown mechanic)"},
		{
			name: "several, sorted", c: dataset.Capabilities{"evasion": 2, "armour": 3},
			want: "unsupported capabilities: armour 3 (implemented up to 2), evasion 2 (implemented up to 1)",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := testCase.c.Supported(known)
			if testCase.want == "" {
				if err != nil {
					t.Errorf("got %v, want nil", err)
				}
				return
			}
			checkError(t, err, dataset.ErrUnsupported, testCase.want)
		})
	}
}

func TestCapabilitiesRoundTrip(t *testing.T) {
	t.Parallel()
	d := sample()
	d.Manifest.Capabilities = dataset.Capabilities{"armour": 2, "life-from-strength": 3}
	dir := t.TempDir()
	if _, err := dataset.Write(dir, d); err != nil {
		t.Fatalf("Write: %v", err)
	}
	m, err := dataset.ReadManifest(os.DirFS(dir))
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if diff := gocmp.Diff(d.Manifest.Capabilities, m.Capabilities); diff != "" {
		t.Errorf("capabilities (-want +got):\n%s", diff)
	}
}

func TestCapabilitiesID(t *testing.T) {
	t.Parallel()
	id := func(c dataset.Capabilities) string {
		d := sample()
		d.Manifest.Capabilities = c
		m, err := dataset.Write(t.TempDir(), d)
		if err != nil {
			t.Fatalf("Write: %v", err)
		}
		return m.ID
	}
	// The id of sample() without capabilities; it must not change when capabilities are absent.
	const plain = "2df3527070a7cc1be4b95d780d8defba5b4fcb0760a4046e9072788986400fe5"
	if got := id(nil); got != plain {
		t.Errorf("no capabilities: got id %s, want %s", got, plain)
	}
	if got := id(dataset.Capabilities{}); got != plain {
		t.Errorf("empty capabilities: got id %s, want %s", got, plain)
	}
	a, b := id(dataset.Capabilities{"armour": 2}), id(dataset.Capabilities{"armour": 3})
	if a == plain || b == plain || a == b {
		t.Errorf("ids not distinct: plain %s, armour 2 %s, armour 3 %s", plain, a, b)
	}
}

func TestCapabilitiesInvalid(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, want string
		c          dataset.Capabilities
	}{
		{name: "version 1", c: dataset.Capabilities{"armour": 1}, want: `capability "armour": version 1, want 2 or more`},
		{name: "version 0", c: dataset.Capabilities{"armour": 0}, want: `capability "armour": version 0, want 2 or more`},
		{name: "empty name", c: dataset.Capabilities{"": 2}, want: `capability "": name is not lowercase kebab-case`},
		{name: "upper case", c: dataset.Capabilities{"Armour": 2}, want: `capability "Armour": name is not`},
		{name: "underscore", c: dataset.Capabilities{"life_regen": 2}, want: `capability "life_regen": name is not`},
		{name: "double hyphen", c: dataset.Capabilities{"life--regen": 2}, want: `capability "life--regen": name is not`},
		{name: "trailing hyphen", c: dataset.Capabilities{"life-": 2}, want: `capability "life-": name is not`},
		{name: "leading digit", c: dataset.Capabilities{"3-30": 2}, want: `capability "3-30": name is not`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			d := sample()
			d.Manifest.Capabilities = testCase.c
			_, err := dataset.Write(t.TempDir(), d)
			checkError(t, err, dataset.ErrInvalid, testCase.want)
		})
	}
}
