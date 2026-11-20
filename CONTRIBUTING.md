# Contributing

## Before a pull request

- `gofmt`, `go vet`, `go test ./...` and `golangci-lint run` pass.
- New behaviour has tests. Engine changes that affect results include a case showing the expected value and its source.
- One concern per pull request.

## Generated test files after a rebase

The files the README's `-update` commands write are generated, mostly from `internal/poe1/table.json`: the fixtures under `internal/poe1/testdata/fixture`, the reports and goldens beside them, the `statdesc` read goldens and the `simulator` wire and sketch goldens. Two branches that change the table always conflict there. Do not merge them by hand: take main's copy and regenerate. `.gitattributes` marks the fixtures binary, so git neither diffs nor merges them. CI fails on a stale one.

```sh
git rebase origin/main
# on a conflict in a generated file, take main's copy (in a rebase, --ours is main), resolve other files as usual
git checkout --ours -- <generated files>
git add <generated files>
git rebase --continue
# once the rebase is done
export PBT_DATASETS=/tmp/pbt-datasets
go test ./internal/poe1 -run '^TestFixtures$' -update   # first: until then main's manifests do not match the branch's .fb files
go test ./internal/poe1 ./statdesc -update
go test ./simulator -run 'Wire|Sketch' -update
git add -A internal/poe1/testdata statdesc/testdata simulator/testdata
git commit -m "test: regenerate generated test files"
go test ./...
```

## Engine rules

- `internal/core` imports nothing from this module.
- Everything a game defines (rules, catalogue, semantics table) lives in its package under `internal/`, such as `internal/poe1`. Public packages expose what hosts need through `simulator`.
- No globals and no I/O in the engine. Same input gives the same bits on every platform.
- Compute from game ids (stat ids, mod ids, node hashes), not from display text.
- Add a package when a task needs it, not before.

## Benchmarks

Each package benchmarks its own pieces with plain Go benchmarks. Each benchmark's doc comment names its target time, where it has one. CI runs every benchmark once, so a broken one fails, and does not judge the times.

The benchmarks of whole operations run on the builds in `internal/benchbuilds`, Path of Building files made for benchmarking: a level 96 Deadeye with Lightning Arrow and a level 97 Elementalist with Arc, each with a full tree, five or six skill groups with supports and auras, and ten items, some adding gem levels; and a level 100 Hierophant with Righteous Fire of Arcane Devotion, with six skill groups, ten items and twelve jewels, four of them cluster jewels (two large, two medium) whose passives are in the tree. They and the timeless and cluster benchmarks read the pinned current dataset under `$PBT_DATASETS` (see the README) and skip without it.

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

Median of 6 runs on an AMD Ryzen 5 5600X, Go 1.27.4, Linux, at commit 23effd2, on dataset 3.29.3.6. WebAssembly under Node 24.22, median of 2 runs. Repeated runs differ by up to about 10%.

| Benchmark | What it measures | Deadeye | Elementalist | Hierophant | Allocated | WebAssembly |
|---|---|---|---|---|---|---|
| `simulator` `SimulateReal` | `Simulate` on a warm sim | 0.57 ms | 0.57 ms | 0.87 ms | 0.5 to 0.7 MB | 2.9 to 4.4 ms |
| `simulator` `SweepReal` | One `Sweep` variant, every output: a hover preview | 1.00 ms | 0.91 ms | 1.31 ms | 0.6 to 0.8 MB | 4.0 to 5.9 ms |
| `simulator` `CompareReal` | `Compare` with a path allocated: applying a click | 1.24 ms | 1.23 ms | 1.88 ms | 0.9 to 1.4 MB | 5.7 to 8.7 ms |
| `build` `Check` | `build.Check` | 0.13 ms | 0.12 ms | 0.23 ms | 41 to 89 KB | 0.5 to 1.1 ms |
| `internal/resolve` `Skills` | Resolving the main skill alone / every skill | 0.06 / 0.08 ms | 0.06 / 0.09 ms | 0.07 / 0.10 ms | 106 to 147 KB | |
| `pob` `FromXML` | Path of Building import | 48 ms | 50 ms | 31 ms | 24 to 43 MB | |

| Benchmark | What it measures | Time | Allocated |
|---|---|---|---|
| `poe1/timeless` `Conquer/<version>/forward` | One seed over the 100 passives of the fullest Large radius | 0.011 to 0.020 ms | 0 |
| `poe1/timeless` `Conquer/<version>/reverse` | Every seed of the version over that radius | 88 ms (Maraketh, 7,501 seeds) to 176 ms (Vaal, 7,901 seeds) | 0 |
| `poe1/timeless` `Search/<version>` | A search of every seed of the version in that socket for a named notable and a stat of each kind of passive, best 50 with what each gives | 31 ms (Eternal) to 164 ms (Vaal); 242 ms for Vaal as WebAssembly in Chrome 156 | 1.8 to 2.0 MB |
| `internal/poe1/cluster` `Generator/setup` | The cluster generator's setup, once per tree variant | 3.0 ms | 2.2 MB |
| `internal/poe1/cluster` `Generator/generate` | One large cluster jewel | 0.010 ms | 5 KB |

## Dataset format

`dataset/schema/*.fbs` is the dataset schema. After changing it, run `go generate ./dataset` with `flatc` 25.12.19 on `PATH` (the version must match the `github.com/google/flatbuffers` module) and commit the generated `dataset/internal/fb`. `dataset.FormatVersion` stays at 1 until the first release. After that, a change that older datasets cannot read bumps it.

## Licence

Contributions are accepted under Apache-2.0 (see [LICENSE](LICENSE), section 5).
