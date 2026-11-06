# pbt-engine

A headless build simulation engine for Path of Exile. It takes a build and a game version and returns the numbers a player cares about, each with an explanation of where it came from.

Status: early. Datasets (the game data the engine reads) can be written, read and inspected. The evaluator (`core`: stats, modifiers, contexts, actors, lazy incremental evaluation with revert) exists; no game rules yet.

## Datasets

A dataset holds one game version: stats, characters, edition constants and the passive trees (standard, ruthless, alternate), which the simulation needs, plus one text file per language (`text-<lang>.fb`) with stat description templates and display names, which only display and text parsing need. The format is in the `dataset` package. [pbt-extract](https://github.com/xaroth/pbt-extract) builds datasets from game files.

`pbt` inspects datasets stored in the subdirectories of `--root`. `dataset.Registry` holds several versions side by side and reads table groups on first use. A version resolves exactly, by prefix (`3.29` is the newest 3.29 dataset, `latest` the newest of all), or to the newest earlier patch of the same league:

```sh
go run ./cmd/pbt data show node 44297 --version 3.29.3.3 --root datasets
go run ./cmd/pbt data show character Marauder --version 3.29 --root datasets
go run ./cmd/pbt data diff 3.28.0.16 3.29.3.3 --root datasets
```

## Builds

A build file (`build` package) names a game, version, tree variant, class (`Character.ID`), ascendancy, level, allocated node ids and scenario fields (bandit, resistance penalty). `pbt build check build.json --root datasets` checks every id against the dataset the version resolves to; `pbt build resolve build.json` prints the stat bundles (game stat ids and values per source: character, level, each node) and edition constants the build resolves to:

```json
{"format": 1, "game": "poe1", "version": "3.29", "class": "Metadata/Characters/Str/Str",
 "ascendancy": "Juggernaut", "level": 90, "nodes": [47175, 17765, 31628], "scenario": {"bandit": "oak"}}
```

Text is read in English unless `--language` says otherwise.

## Develop

Requires Go 1.27.

```sh
go build ./...
go test ./...
golangci-lint run
go test ./core -run '^$' -bench .   # benchmarks, also run in CI
```

## Licence

Apache-2.0, see [LICENSE](LICENSE) and [NOTICE](NOTICE).

This product isn't affiliated with or endorsed by Grinding Gear Games in any way. Path of Exile and its game data are the property of Grinding Gear Games.
