# Contributing

## Before a pull request

- `gofmt`, `go vet`, `go test ./...` and `golangci-lint run` pass.
- New behaviour has tests. Engine changes that affect results include a case showing the expected value and its source.
- One concern per pull request.

## Engine rules

- `internal/core` imports nothing from this module.
- Everything a game defines (rules, catalogue, semantics table) lives in its package under `internal/`, such as `internal/poe1`. Public packages expose what hosts need through `simulator`.
- No globals and no I/O in the engine. Same input gives the same bits on every platform.
- Compute from game ids (stat ids, mod ids, node hashes), not from display text.
- Add a package when a task needs it, not before.

## Benchmarks

Each package benchmarks its own pieces with plain Go benchmarks. Each benchmark's doc comment names its target time, where it has one. CI runs every benchmark once, so a broken one fails, and does not judge the times.

The benchmarks of whole operations run on the builds in `internal/benchbuilds`, Path of Building files made for benchmarking: a level 96 Deadeye with Lightning Arrow and a level 97 Elementalist with Arc, each with a full tree, five or six skill groups with supports and auras, and ten items, some adding gem levels. They and the timeless and cluster benchmarks read the pinned current dataset under `$PBT_DATASETS` (see the README) and skip without it.

```sh
go run ./internal/realdata/fetch /tmp/pbt-datasets
export PBT_DATASETS=/tmp/pbt-datasets
go test -run '^$' -bench . -benchmem ./...                          # every benchmark
go test -run '^$' -bench 'Real$' -benchmem ./simulator              # Simulate, Sweep and Compare on the builds
go test -run '^$' -bench . -count 10 ./simulator > new.txt          # then benchstat old.txt new.txt
go test -run '^$' -bench Nodes ./power                              # node power on the real tree
```

As WebAssembly under Node (Node on `PATH`), from the package directory so the test finds its files:

```sh
GOOS=js GOARCH=wasm go test -c -o /tmp/simulator.test.wasm ./simulator
cd simulator && "$(go env GOROOT)/lib/wasm/go_js_wasm_exec" /tmp/simulator.test.wasm -test.run '^$' -test.bench 'Real$' -test.benchmem
```

### Reference numbers

Median of 6 runs on an AMD Ryzen 5 5600X, Go 1.27.4, Linux, at commit 39bb3e0, on dataset 3.29.3.6. WebAssembly under Node 24.22, one run. Repeated runs differ by up to about 10%.

| Benchmark | What it measures | Deadeye | Elementalist | Allocated | WebAssembly |
|---|---|---|---|---|---|
| `simulator` `SimulateReal` | `Simulate` on a warm sim | 0.63 ms | 0.67 ms | 0.8 to 1.0 MB | 3.1 to 3.3 ms |
| `simulator` `SweepReal` | One `Sweep` variant, every output: a hover preview | 0.98 ms | 1.09 ms | 1.3 to 1.6 MB | 4.6 to 5.7 ms |
| `simulator` `CompareReal` | `Compare` with a path allocated: applying a click | 1.75 ms | 1.43 ms | 1.6 to 1.9 MB | 13 to 16 ms |
| `build` `Check` | `build.Check` | 0.11 ms | 0.10 ms | 39 KB | |
| `internal/resolve` `Skills` | Resolving the main skill alone / every skill | 0.04 / 0.07 ms | 0.04 / 0.09 ms | 86 to 115 KB | |
| `pob` `FromXML` | Path of Building import | 46 ms | 47 ms | 41 to 43 MB | |

| Benchmark | What it measures | Time | Allocated |
|---|---|---|---|
| `poe1/timeless` `Conquer/<version>/forward` | One seed over the 100 passives of the fullest Large radius | 0.012 to 0.021 ms | 0 |
| `poe1/timeless` `Conquer/<version>/reverse` | Every seed of the version over that radius | 47 ms (Kalguuran, 3,901 seeds) to 163 ms (Vaal) | 0 |
| `internal/poe1/cluster` `Generator/setup` | The cluster generator's setup, once per tree variant | 3.1 ms | 2.2 MB |
| `internal/poe1/cluster` `Generator/generate` | One large cluster jewel | 0.011 ms | 5 KB |

## Dataset format

`dataset/schema/*.fbs` is the dataset schema. After changing it, run `go generate ./dataset` with `flatc` 25.12.19 on `PATH` (the version must match the `github.com/google/flatbuffers` module) and commit the generated `dataset/internal/fb`. `dataset.FormatVersion` stays at 1 until the first release. After that, a change that older datasets cannot read bumps it.

## Licence

Contributions are accepted under Apache-2.0 (see [LICENSE](LICENSE), section 5).
