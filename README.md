# pbt-engine

A headless build simulation engine for Path of Exile. It takes a build and a game version and returns the numbers a player cares about, each with an explanation of where it came from.

Status: early. Datasets (the game data the engine reads) can be written, read and inspected. The evaluator (`core`: stats, modifiers, contexts, actors, lazy incremental evaluation with revert) exists; no game rules yet.

## Datasets

A dataset holds one game version: stats, characters, edition constants and the passive trees (standard, ruthless, alternate), which the simulation needs, plus one text file per language (`text-<lang>.fb`) with stat description templates and display names, which only display and text parsing need. The format is in the `dataset` package. [pbt-extract](https://github.com/xaroth/pbt-extract) builds datasets from game files.

`dataset.Registry` holds several versions side by side and reads table groups on first use. A version resolves exactly, by prefix (`3.29` is the newest 3.29 dataset, `latest` the newest of all), or, for a patch without a dataset, to the newest earlier patch of the same league, else the newest patch of that league; leagues never fall back to each other. A subdirectory whose dataset cannot be opened, such as one in an older format, is skipped. `pbt` inspects the datasets in the subdirectories of `--root` and resolves `--version` of `--game` (default `poe1`) that way; fallbacks and skipped datasets are noted on stderr:

```sh
go run ./cmd/pbt data show node 44297 --version 3.29.3.3 --root datasets
go run ./cmd/pbt data show character Marauder --version 3.29 --root datasets
go run ./cmd/pbt data diff 3.28.0.16 3.29.3.3 --root datasets
```

Text is read in English unless `--language` says otherwise.

## Builds

A build file (`build` package) names a game, version, tree variant, class (`Character.ID`), ascendancy, level, allocated node ids and a scenario: the campaign stage (`complete`, the default, `acts-6-10` or `acts-1-5`, split where Kitava is defeated) and the outcome of the bandit quest (`kill-all`, `help-alira`, `help-kraityn`, `help-oak`, or none). `pbt build check build.json --root datasets` checks every id against the dataset the version resolves to; `pbt build resolve build.json --root datasets` prints the stat bundles the build resolves to (game stat ids and values per source: character, level, the quest rewards of the campaign stage, each node) and the edition constants:

```json
{"format": 1, "game": "poe1", "version": "3.29", "class": "Metadata/Characters/Str/Str",
 "ascendancy": "Juggernaut", "level": 90, "nodes": [47175, 17765, 31628], "scenario": {"bandits": "help-oak"}}
```

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
