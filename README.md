# pbt-engine

A headless build simulation engine for Path of Exile. It takes a build and a game version and returns the numbers a player cares about, each with an explanation of where it came from.

Status: early. Datasets (the game data the engine reads) can be written, read and inspected. The evaluator (`core`: stats, modifiers, contexts, actors, lazy incremental evaluation with revert) exists; no game rules yet.

## Datasets

A dataset holds one game version: stats, characters, edition constants and the passive trees (standard, ruthless, alternate), which the simulation needs, plus one text file per language (`text-<lang>.fb`) with stat description templates and display names, which only display and text parsing need. The format is in the `dataset` package. [pbt-extract](https://github.com/xaroth/pbt-extract) builds datasets from game files.

`pbt` inspects datasets stored as `<root>/<version>/`:

```sh
go run ./cmd/pbt data show node 44297 --version 3.29.3.3 --root datasets
go run ./cmd/pbt data show character Marauder --version 3.29.3.3 --root datasets
go run ./cmd/pbt data diff 3.28.0.16 3.29.3.3 --root datasets
```

Text is read in English unless `--language` says otherwise.

## Develop

Requires Go 1.27.

```sh
go build ./...
go test ./...
golangci-lint run
go test ./core -run '^$' -bench .   # benchmarks
```

## Licence

Apache-2.0, see [LICENSE](LICENSE) and [NOTICE](NOTICE).

This product isn't affiliated with or endorsed by Grinding Gear Games in any way. Path of Exile and its game data are the property of Grinding Gear Games.
