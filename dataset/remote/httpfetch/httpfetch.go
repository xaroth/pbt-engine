// Package httpfetch fetches a source's files over HTTP, as a remote.Fetcher. It is apart from
// remote so a host that brings its own fetcher, such as a WebAssembly host, does not link
// net/http.
package httpfetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrStatus is a response with a status other than 200.
var ErrStatus = errors.New("unexpected HTTP status")

// Source fetches from Base, a URL ending in a slash, with Client, or without one a client that
// gives up on a request after fetchTimeout.
type Source struct {
	Base   string
	Client *http.Client
}

// fetchTimeout bounds one file's request: the largest is a few MB.
const fetchTimeout = 2 * time.Minute

// Fetch gets Base+name and fails on any status but 200.
func (s Source) Fetch(ctx context.Context, name string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Base+name, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", name, err)
	}
	c := s.Client
	if c == nil {
		c = &http.Client{Timeout: fetchTimeout}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: %w %s", s.Base+name, ErrStatus, resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", name, err)
	}
	return b, nil
}
