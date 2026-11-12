package pob

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrCode is a share code DecodeCode cannot read.
var ErrCode = errors.New("pob: not a Path of Building share code")

// maxCodeXML caps the build file a share code inflates to, so a small code cannot expand
// without bound.
const maxCodeXML = 16 << 20

// DecodeCode returns the build file a PoB share code holds, for FromXML. A share code is the
// file compressed with zlib and base64-encoded with the URL alphabet. As PoB reads codes
// (ImportTab.lua), it accepts the standard alphabet too, with or without padding, and ignores
// whitespace, such as a code wrapped over lines. Other characters, data that is not zlib and
// a file over 16 MiB are ErrCode. Links to a code on pobb.in or poe.ninja are the host's to
// fetch.
func DecodeCode(code string) ([]byte, error) {
	s := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r':
			return -1
		case '-':
			return '+'
		case '_':
			return '/'
		}
		return r
	}, code)
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "="))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCode, err)
	}
	z, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCode, err)
	}
	xml, err := io.ReadAll(io.LimitReader(z, maxCodeXML+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCode, err)
	}
	if len(xml) > maxCodeXML {
		return nil, fmt.Errorf("%w: build file over %d bytes", ErrCode, maxCodeXML)
	}
	return xml, nil
}
