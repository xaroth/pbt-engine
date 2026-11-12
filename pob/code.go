package pob

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
)

// ErrCode is a share code DecodeCode cannot read.
var ErrCode = errors.New("pob: not a Path of Building share code")

// maxCodeXML caps the build file a share code inflates to, so a small code cannot expand
// without bound.
const maxCodeXML = 16 << 20

// DecodeCode returns the build file a PoB share code holds, for FromXML. A share code is the
// file compressed with zlib and base64-encoded with the URL alphabet. It reads codes as PoB
// does (ImportTab.lua): either alphabet, and every byte outside it dropped, such as padding,
// line breaks or the non-breaking and zero-width spaces of a web paste. Data that is not one
// zlib stream, data after the stream, such as a code pasted twice, and a file over 16 MiB are
// ErrCode. Links to a code on pobb.in or poe.ninja are the host's to fetch.
func DecodeCode(code string) ([]byte, error) {
	b64 := base64Only(code)
	raw := make([]byte, base64.RawStdEncoding.DecodedLen(len(b64)))
	n, err := base64.RawStdEncoding.Decode(raw, b64)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCode, err)
	}
	r := bytes.NewReader(raw[:n])
	z, err := zlib.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCode, err)
	}
	xml, err := io.ReadAll(io.LimitReader(z, maxCodeXML+1))
	switch {
	case err != nil:
		return nil, fmt.Errorf("%w: %w", ErrCode, err)
	case len(xml) > maxCodeXML:
		return nil, fmt.Errorf("%w: build file over %d bytes", ErrCode, maxCodeXML)
	case r.Len() > 0:
		return nil, fmt.Errorf("%w: %d bytes after the build file", ErrCode, r.Len())
	}
	return xml, nil
}

// base64Only keeps the bytes of code in either base64 alphabet, as the standard one.
func base64Only(code string) []byte {
	out := make([]byte, 0, len(code))
	for _, c := range []byte(code) {
		switch {
		case c == '-':
			out = append(out, '+')
		case c == '_':
			out = append(out, '/')
		case c == '+', c == '/', '0' <= c && c <= '9', 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z':
			out = append(out, c)
		}
	}
	return out
}

// FromCode imports the build of a PoB share code: DecodeCode, then FromXML.
func FromCode(code string, reg *dataset.Registry) (*build.Build, []build.Warning, error) {
	data, err := DecodeCode(code)
	if err != nil {
		return nil, nil, err
	}
	return FromXML(data, reg)
}
