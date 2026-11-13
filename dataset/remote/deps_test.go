package remote_test

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// A WebAssembly host brings its own fetcher, so remote must not link an HTTP client into it.
func TestNoHTTPInWasm(t *testing.T) {
	t.Parallel()
	cmd := exec.CommandContext(t.Context(), "go", "list", "-deps", ".")
	cmd.Env = append(cmd.Environ(), "GOOS=js", "GOARCH=wasm")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	deps := strings.Fields(string(out))
	for _, pkg := range []string{"net/http", "crypto/tls"} {
		if slices.Contains(deps, pkg) {
			t.Errorf("remote links %s for js/wasm", pkg)
		}
	}
}
