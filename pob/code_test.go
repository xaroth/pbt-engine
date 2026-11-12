package pob_test

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/xaroth/pbt-engine/pob"
)

// shareCode encodes data as PoB writes codes: zlib at its best compression, base64 with the
// URL alphabet and padding.
func shareCode(t *testing.T, data []byte) string {
	t.Helper()
	var buf bytes.Buffer
	w, err := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return base64.URLEncoding.EncodeToString(buf.Bytes())
}

func TestDecodeCode(t *testing.T) {
	t.Parallel()
	file, err := os.ReadFile(filepath.Join("testdata", "righteous-fire-skills.xml"))
	if err != nil {
		t.Fatal(err)
	}
	code := shareCode(t, file)
	if !strings.ContainsAny(code, "-_") || !strings.HasSuffix(code, "=") {
		t.Fatalf("code %q does not test both alphabets and padding; pick another file", code)
	}
	std := strings.NewReplacer("-", "+", "_", "/").Replace(code)
	testCases := []struct {
		name, code string
	}{
		{"as PoB writes it", code},
		{"standard alphabet", std},
		{"without padding", strings.TrimRight(code, "=")},
		{"wrapped and indented", "  " + code[:40] + "\r\n" + code[40:80] + "\n\t" + code[80:] + "\n"},
		{"web paste", "\ufeff" + code[:40] + "\u00a0" + code[40:80] + "\u200b" + code[80:] + "\u00a0"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got, err := pob.DecodeCode(testCase.code)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, file) {
				t.Errorf("got %d bytes, want the %d of the file", len(got), len(file))
			}
		})
	}
}

func TestDecodeCodeErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, code string
	}{
		{"empty", ""},
		{"no base64", "!?\u00a0"},
		{"truncated base64", "eNrtf"},
		{"not zlib", base64.URLEncoding.EncodeToString([]byte("<PathOfBuilding/>"))},
		{"truncated zlib", strings.TrimRight(shareCode(t, []byte(strings.Repeat("<Build/>", 100))), "=")[:20]},
		{"over 16 MiB", shareCode(t, make([]byte, 16<<20+1))},
		{"pasted twice", strings.Repeat(strings.TrimRight(shareCode(t, []byte("<PathOfBuilding/>")), "="), 2)},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := pob.DecodeCode(testCase.code); !errors.Is(err, pob.ErrCode) {
				t.Errorf("got %v, want ErrCode", err)
			}
		})
	}
}

// TestFromCode imports a code to the build and warnings of its file.
func TestFromCode(t *testing.T) {
	t.Parallel()
	reg := registry(t)
	file := []byte(example)
	wantBuild, wantWarnings, err := pob.FromXML(file, reg)
	if err != nil {
		t.Fatal(err)
	}
	b, warnings, err := pob.FromCode(shareCode(t, file), reg)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(wantBuild, b); diff != "" {
		t.Errorf("build (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantWarnings, warnings); diff != "" {
		t.Errorf("warnings (-want +got):\n%s", diff)
	}
	if _, _, err := pob.FromCode("eNrt", reg); !errors.Is(err, pob.ErrCode) {
		t.Errorf("bad code: got %v, want ErrCode", err)
	}
}
