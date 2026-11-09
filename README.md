# pbt-engine

A headless build simulation engine for Path of Exile. It takes a build and a game version and returns the numbers a player cares about, each with an explanation of where it came from.

Status: early. Datasets (the game data the engine reads) can be written, read and inspected. The evaluator (`core`: stats, modifiers, contexts, actors, lazy incremental evaluation with revert) and the character sheet rules for a build without items (`rules/poe1`) exist.

## Datasets

A dataset holds one game version: stats, characters, edition constants and the passive trees (standard, ruthless, alternate), which the simulation needs, plus one text file per language (`text-<lang>.fb`) with stat description templates and display names, which only display and text parsing need. The format is in the `dataset` package. [pbt-extract](https://github.com/xaroth/pbt-extract) builds datasets from game files.

A manifest can list capabilities: the rule version a mechanic needs in that dataset, such as `{"armour": 2}`. A mechanic not listed uses version 1. Rules choose formulas by these versions, never by game version, and read them only after checking the dataset against the versions they implement (`Capabilities.Check`), which rejects a dataset that needs one they do not.

`dataset.Registry` holds several versions side by side and reads table groups on first use. A version resolves exactly, by prefix (`3.29` is the newest 3.29 dataset, `latest` the newest of all), or, for a patch without a dataset, to the newest earlier patch of the same league, else the newest patch of that league; leagues never fall back to each other. A subdirectory whose dataset cannot be opened, such as one in an older format, is skipped. `pbt` inspects the datasets in the subdirectories of `--root` and resolves `--version` of `--game` (default `poe1`) that way; fallbacks and skipped datasets are noted on stderr:

```sh
go run ./cmd/pbt data show node 44297 --version 3.29.3.3 --root datasets
go run ./cmd/pbt data show character Marauder --version 3.29 --root datasets
go run ./cmd/pbt data diff 3.28.0.16 3.29.3.3 --root datasets
```

Text is read in English unless `--language` says otherwise.

## Builds

A build file (`build` package) names a game, version, tree variant, class (`Character.ID`), ascendancy, level, allocated node ids and config values by key. The poe1 keys are `campaign` (`complete`, the default, `acts-6-10` or `acts-1-5`, split where Kitava is defeated) and `bandits` (`kill-all`, `help-alira`, `help-kraityn`, `help-oak`, or none). `pbt build check build.json --root datasets` checks every id against the dataset the version resolves to; `pbt build resolve build.json --root datasets` prints the stat bundles the build resolves to (game stat ids and values per source: character, level, every quest reward with stats, each node; the rules pick the rewards the config grants), the edition constants and the dataset's capabilities:

```json
{"format": 1, "game": "poe1", "version": "3.29", "class": "Metadata/Characters/Str/Str",
 "ascendancy": "Juggernaut", "level": 90, "nodes": [47175, 17765, 31628], "config": {"bandits": "help-oak"}}
```

## Benchmarks

Each package benchmarks its own pieces with plain Go benchmarks; CI runs tests only. Each benchmark's doc comment names the target from `plan/01-goals.md` it measures.

```sh
go test -run '^$' -bench . -benchmem ./...          # every benchmark
go test -run '^$' -bench . -count 10 ./core > new.txt # then benchstat old.txt new.txt
PBT_DATASETS=../pbt-archive/datasets go test -run '^$' -bench Nodes ./power   # node power on the real tree
```

## Semantics

What a game stat id means in engine terms is authored by us from game data: stat ids, stat description text, object templates. The `semantics` package holds the table format: entries keyed by stat id and scope, with optional game version ranges, a status (`authored`, `unknown` or `display-only`), the sources each was authored from, and for unknown entries a typed reason (a scenario condition, equipped items, a rule still to write, and so on).

The Path of Exile 1 table (`semantics/poe1/table.json`) has an entry for every stat on standard tree nodes, characters and campaign rewards that its predicate, which reads the English description text, finds can change a character sheet value: attributes, life, mana, energy shield, armour, evasion, resistances and their maximums, life and mana regeneration. Stats whose text names a sheet value but that the predicate rejects (enemy and minion stats, costs, reservation, penetration, recovery) are listed per dataset in `semantics/poe1/testdata` for review. `pbt semantics coverage --version 3.29 --root datasets` lists the stats that feed the sheet with no entry, the unknown ones with their reason, and the authored ones not yet validated or with effects left out, most used first.

## Rules

`rules/poe1` computes the character sheet of a resolved build: `Open(resolved, table)` turns each bundle stat into engine modifiers through the semantics table, adds the rules' own (attribute bonuses, conversions, resistance caps) and returns a `Sheet`. `Sheet.Output` reads the named outputs of `Outputs()`, the catalogue consumers render from: attributes, life, mana, energy shield, armour, evasion rating, resistances with their maximums and uncapped totals, and regeneration per second. Stats the table cannot apply are listed in `Sheet.Notes`, not errors. Where neither game text nor game data states a formula detail, such as rounding, it follows Path of Building as a starting point and is marked unvalidated.

## Develop

Requires Go 1.27.

```sh
go build ./...
go test ./...
golangci-lint run
go test ./core -run '^$' -bench .   # benchmarks
PBT_DATASETS=datasets go test ./rules/poe1   # also compute sheets on real datasets
PBT_DATASETS=datasets go test ./semantics/poe1   # also check the table against real datasets
PBT_DATASETS=datasets go test ./semantics/poe1 -update   # rewrite its fixtures and golden lists
```

## Licence

Apache-2.0, see [LICENSE](LICENSE) and [NOTICE](NOTICE).

This product isn't affiliated with or endorsed by Grinding Gear Games in any way. Path of Exile and its game data are the property of Grinding Gear Games.
