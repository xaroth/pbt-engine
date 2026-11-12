package simulator_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/xaroth/pbt-engine/build"
	"github.com/xaroth/pbt-engine/dataset"
	"github.com/xaroth/pbt-engine/internal/realdata"
	"github.com/xaroth/pbt-engine/pob"
	"github.com/xaroth/pbt-engine/poe1"
	"github.com/xaroth/pbt-engine/simulator"
)

// probeResult is everything the probe computes. Floats encode as the shortest text that reads
// back to the same bits, so two results are byte-identical only when every value is.
type probeResult struct {
	Build          *build.Build             `json:"build"`
	ImportWarnings []build.Warning          `json:"importWarnings"`
	Outputs        []probeOutput            `json:"outputs"`
	Warnings       []build.Warning          `json:"warnings"`
	Explanations   []*simulator.Explanation `json:"explanations"`
}

type probeOutput struct {
	Key   string  `json:"key"`
	Value float64 `json:"value"`
}

// TestProbe imports a Path of Building file and simulates it on the pinned 3.29 dataset as a
// host does, and writes the result to $PBT_PROBE_OUT when set. CI runs it natively and as
// js/wasm and fails when the two files differ, so WebAssembly computes what native does.
func TestProbe(t *testing.T) {
	t.Parallel()
	realdata.Open(t, realdata.Current)
	xml, err := os.ReadFile(filepath.Join("testdata", "chieftain-L90-3.29.xml"))
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	reg, err := dataset.OpenRoot(os.DirFS(realdata.Root(t)))
	if reg == nil {
		t.Fatal(err)
	}
	b, importWarnings, err := pob.FromXML(xml, reg)
	if err != nil {
		t.Fatal(err)
	}
	res, err := reg.Resolve(b.Game, b.Version)
	if err != nil {
		t.Fatal(err)
	}
	sim, err := simulator.New(res.Store)
	if err != nil {
		t.Fatal(err)
	}
	imported := time.Now()
	stats, err := sim.Simulate(b)
	if err != nil {
		t.Fatal(err)
	}
	simulated := time.Now()

	got := probeResult{Build: b, ImportWarnings: importWarnings, Warnings: stats.Warnings()}
	for _, k := range stats.Keys() {
		v, _ := stats.Get(k)
		got.Outputs = append(got.Outputs, probeOutput{Key: string(k), Value: v})
	}
	for _, v := range []simulator.View{simulator.Proportional, simulator.Marginal} {
		ex, err := stats.Explain(poe1.Life, v)
		if err != nil {
			t.Fatal(err)
		}
		got.Explanations = append(got.Explanations, ex)
	}
	explained := time.Now()

	if life, _ := stats.Get(poe1.Life); life < 1000 {
		t.Errorf("life %v, want a level 90 Chieftain's thousands", life)
	}
	t.Logf("probe %s/%s: import %s, first simulate %s, explain %s",
		runtime.GOOS, runtime.GOARCH, ms(imported.Sub(start)), ms(simulated.Sub(imported)), ms(explained.Sub(simulated)))

	out := os.Getenv("PBT_PROBE_OUT")
	if out == "" {
		return
	}
	data, err := json.MarshalIndent(got, "", "\t")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func ms(d time.Duration) string {
	return strconv.FormatFloat(float64(d.Microseconds())/1000, 'f', 1, 64) + " ms"
}
