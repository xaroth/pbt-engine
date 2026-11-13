# pbt-engine

A headless build simulation engine for Path of Exile. It takes a build and a game version and returns the numbers a player cares about, each with an explanation of where it came from.

Status: early. The engine simulates the character sheet of a Path of Exile 1 build from its passive tree, campaign rewards and equipped items; skills are not simulated yet. It imports and exports Path of Building builds and reads item text. What each package does is in its doc comment (`go doc ./<package>`).

## Layout

Hosts and tools import the packages at the top level: `simulator` (the API), `poe1` (the Path of Exile 1 output keys, config keys and config values, such as `poe1.Life` and `poe1.Bandits`), `build`, `config`, `output`, `pob`, `power`, `dataset` (with `dataset/remote`), `statdesc` and `items`. The pipeline behind them is under `internal/`, so it can change without breaking anyone: `core` (the game-neutral evaluator), `resolve` (build to stat bundles), `semantics` (the table format) and one package per game, `poe1` (rules, output catalogue and semantics table), whose names for hosts the top-level package of the same name re-exports.

## Datasets

A dataset holds one game version as one FlatBuffers file per table group plus a JSON manifest. The format is in the `dataset` package; [pbt-extract](https://github.com/xaroth/pbt-extract) builds datasets from game files and [pbt-data](https://github.com/xaroth/pbt-data) publishes them to `https://pbt-data.xaroth.nl/`, from where `dataset/remote` fetches them.

| Files | Holds |
|---|---|
| `stats.fb`, `characters.fb`, `constants.fb`, `quests.fb`, `tree-<variant>.fb` | Stats, characters, edition constants, quest rewards and the passive trees (standard, ruthless, alternate): what every simulation reads |
| `text-<lang>.fb`, `names-<lang>.fb` | Per language: stat description templates and display names, for display and text parsing; the names file holds character, ascendancy and node names alone |
| `layout-<variant>.fb` | Where a tree variant's nodes and art are drawn, for drawing the tree |
| `items.fb`, `mods.fb` (optional) | Base items, bench crafts, essences and item mods |
| `skills.fb` (optional) | Gems, granted effects and active skills |
| `buffs.fb` (optional) | Buffs, utility flask buffs and tinctures |
| `monsters.fb` (optional, needs skills) | Monster varieties, types and tables, and the links from skills to monsters |

The art that goes with a dataset (sprite sheets and class and ascendancy faces) is listed in an art manifest, published beside it and named by its SHA-256: `dataset.ArtManifest`, whose JSON `dataset/art.schema.json` describes.

A manifest can list capabilities: the rule version a mechanic needs in that dataset (`dataset.Capabilities`). Rules choose formulas by these versions, never by game version.

## Develop

Requires Go 1.27.

```sh
go build ./...
go test ./...
golangci-lint run
```

Tests that need real datasets (sheets, goldens, node power, the semantics table and its fixtures, the dataset groups, the walkthrough, the API sketch) read the datasets under `$PBT_DATASETS` by exact version and dataset id, pinned in `internal/realdata` (3.28.0.16 and 3.29.3.6), and the art manifests published with them. They skip without `$PBT_DATASETS` and fail when it lacks a pinned dataset. `internal/realdata/fetch` fetches them by id from pbt-data's bucket, one directory per version and the art manifests under `art/`, reusing what the directory holds:

```sh
go run ./internal/realdata/fetch /tmp/pbt-datasets
PBT_DATASETS=/tmp/pbt-datasets go test ./...
```

Generated test files are rewritten with `-update` after a deliberate change:

```sh
PBT_DATASETS=/tmp/pbt-datasets go test ./internal/poe1 ./statdesc -update   # fixtures, coverage reports, golden lists, explanation and read goldens
go test ./simulator -run Wire -update     # wire goldens; change simulator/wire.schema.json to match
go test ./simulator -run Sketch -update   # the API sketch parts that cannot run yet
```

`internal/poe1/testdata` holds, per dataset, the stats whose text names a sheet value but that the table's predicate rejects, for review, and `coverage-<version>.txt`: the stats that feed the sheet with no table entry, the unknown ones with their reason, and the authored ones not yet validated or with effects left out, most used first.

Each package benchmarks its own pieces with plain Go benchmarks; CI runs tests only. Each benchmark's doc comment names its target time.

```sh
go test -run '^$' -bench . -benchmem ./...          # every benchmark
go test -run '^$' -bench . -count 10 ./internal/core > new.txt # then benchstat old.txt new.txt
PBT_DATASETS=/tmp/pbt-datasets go test -run '^$' -bench Nodes ./power   # node power on the real tree
```

The engine also builds for `GOOS=js GOARCH=wasm`. `TestProbe` in `simulator` imports a Path of Building file and simulates it on the pinned 3.29 dataset, writing the result to `$PBT_PROBE_OUT`; CI runs it natively and as WebAssembly under Node and fails when the two results differ. To run it as WebAssembly locally (Node on `PATH`):

```sh
GOOS=js GOARCH=wasm go test -c -o /tmp/simulator.test.wasm ./simulator
cd simulator && PBT_DATASETS=/tmp/pbt-datasets "$(go env GOROOT)/lib/wasm/go_js_wasm_exec" /tmp/simulator.test.wasm -test.run '^TestProbe$' -test.v
```

## Licence

Apache-2.0, see [LICENSE](LICENSE) and [NOTICE](NOTICE).

This product isn't affiliated with or endorsed by Grinding Gear Games in any way. Path of Exile and its game data are the property of Grinding Gear Games.
