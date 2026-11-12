# pbt-engine

A headless build simulation engine for Path of Exile. It takes a build and a game version and returns the numbers a player cares about, each with an explanation of where it came from.

Status: early. Datasets (the game data the engine reads) can be written, read and inspected. The evaluator (`internal/core`: stats, modifiers, contexts, actors, lazy incremental evaluation with revert) and the character sheet rules for a build without items (`internal/poe1`) exist.

## Layout

Hosts and tools import the packages at the top level: `simulator` (the API), `poe1` (the Path of Exile 1 output keys, config keys and config values, such as `poe1.Life` and `poe1.Bandits`), `build`, `config`, `output`, `pob`, `power`, `dataset` (with `dataset/remote`), `statdesc` and `items`. The pipeline behind them is under `internal/`, so it can change without breaking anyone: `core` (the game-neutral evaluator), `resolve` (build to stat bundles), `semantics` (the table format) and one package per game, `poe1` (rules, output catalogue and semantics table), whose names for hosts the top-level package of the same name re-exports.

## Datasets

A dataset holds one game version: stats, characters, edition constants and the passive trees (standard, ruthless, alternate), which the simulation needs, plus one text file per language (`text-<lang>.fb`) with stat description templates and display names, which only display and text parsing need. The format is in the `dataset` package. [pbt-extract](https://github.com/xaroth/pbt-extract) builds datasets from game files.

The items group is optional: `items.fb` holds the equippable base items with their item classes, mod domains, tags, defences, block, weapon and flask values and implicit mods, and the bench crafts that add a mod with the item classes each applies to, and `mods.fb` the item mods with their stat ranges, generation type, level, families, tags, spawn weights and game mode, read only when asked for. Their display names are in the text files. A dataset without the group reads as before and returns a `dataset.MissingGroupError` when asked for items.

The skills group is optional too: `skills.fb` holds gems, granted effects from every source (gems, items, the tree, monsters) with their per-level rows, stat sets with constant and per-level stats, and active skills with their skill types, totem and mine ids. Gem and skill names and the skill stat descriptions are in the text files, and a dataset without the group returns a `dataset.MissingGroupError` when asked for skills.

The buffs group is optional as well: `buffs.fb` holds the game's buff definitions (the stats a buff carries, its category, stack limit and, for charges, the stats holding its maximum and current count), the buffs of utility flasks with their values, and the tinctures. Its flasks and tinctures refer to the items group. Buff names are in the text files. The game files do not link skills to the buffs they grant; the engine keeps those links in its rules.

The monsters group is optional and needs the skills group: `monsters.fb` holds every monster variety with its type, life and damage multipliers, attack time, tags, granted effects and mods, the monster types with their defences and resistances, the per-level monster tables, the base stats of `Monster.ot`, the mods of the monster domain, and the links from skills to monsters: display minion types, totem variations, spectre overrides and minion limit stats. A link the game data does not hold, such as the totem Spell Totem Support uses, is an edition entry with its source. Monster names are in the text files.

A manifest can list capabilities: the rule version a mechanic needs in that dataset, such as `{"armour": 2}`. A mechanic not listed uses version 1. Rules choose formulas by these versions, never by game version, and read them only after checking the dataset against the versions they implement (`Capabilities.Check`), which rejects a dataset that needs one they do not.

Published datasets live in [pbt-data](https://github.com/xaroth/pbt-data), one per league (its latest patch). `dataset/remote` keeps local copies current: `Pull` reads the source's `index.json`, and for each league whose manifest changed fetches the manifest and only the files whose SHA-256 differs, checking each against the index and the manifest, then swaps the league's directory in whole.

`dataset.Registry` holds several versions side by side and reads table groups on first use. A version resolves exactly, by prefix (`3.29` is the newest 3.29 dataset, `latest` the newest of all), or, for a patch without a dataset, to the newest earlier patch of the same league, else the newest patch of that league; leagues never fall back to each other. A subdirectory whose dataset cannot be opened, such as one in an older format, is skipped.

## Builds

A build file (`build` package) names a game, version, tree variant, class (`Character.ID`), ascendancy, level, allocated node ids, config values by key and skill groups: each group's gems by id (`Gem.ID`, plus the `GemEffect.ID` of a transfigured variant) with level, quality, whether each is a support or disabled, the group's active gem and the item slot it is socketed in, and which group is the main skill. A build holds ids; `Sim.Data().Gem("Fireball")` turns an English gem name into one, through the dataset's skills group and text; where the game's files hold more than one gem of a name (Royale gems, replaced items), it takes the current regular gem by the rule `dataset.Store.GemByName` states, and a name that rule cannot decide is an error. `build.Check` checks the gems against the skills group (unknown gems dropped, levels and quality clamped, each with a warning); against a dataset without one it warns once and keeps them. Skills are not simulated yet; `Simulate` warns when a build has gems. Equipped items are held by slot as plain data: the base id (`ItemBase.ID`), rarity, name, item level, quality, sockets, the defences the item's text gives, and each mod line's text with the stats it reads as; `build.Equip(slot, item)` and `build.Unequip(slot)` are edits. `Sim.Data().Item(text)` reads Path of Building's item text into one through the dataset's items group and stat descriptions (`items.Reader`); of bases of one name, such as the three Two-Stone Rings, it takes the one whose implicits read the item's implicit lines; where a line reads as more than one stat, it keeps the first in the game's order with a warning; a line nothing reads is a warning. `build.Check` leaves out items in no item slot, of an unknown base or of a class the slot does not take, and warns about lines without stats. Items are not simulated yet; `Simulate` warns when a build has them. The poe1 keys are `campaign` (`complete`, the default, `acts-6-10` or `acts-1-5`, split where Kitava is defeated) and `bandits` (`kill-all`, `help-alira`, `help-kraityn`, `help-oak`, or none). `build.Check` checks every id against a dataset. Inside the engine, the resolve stage turns a checked build into stat bundles (game stat ids and values per source: character, level, every quest reward with stats, each node; the rules pick the rewards the config grants), the edition constants and the dataset's capabilities. A build file looks like this:

```json
{"format": 1, "game": "poe1", "version": "3.29", "class": "Metadata/Characters/Str/Str",
 "ascendancy": "Juggernaut", "level": 90, "nodes": [47175, 17765, 31628], "config": {"bandits": "help-oak"}}
```

Path of Building build files (PoE1) import with `pob.FromXML`: the active tree's version, variant, nodes and mastery choices, class and ascendancy (names mapped through the dataset's English text), level, bandits and the campaign stage from PoB's resistance penalty. The active skill set's groups import with their slot, gems (resolved by gem id, the gem granting the skill id, or English name through the dataset's skills group, in the order PoB loads them), levels, quality, enabled state, the group's main active gem and the main group. Everything else in the file gives a `pob-*` warning: other trees, cluster jewel nodes (dropped), jewels, tattoos, config inputs, items, pantheon, and of the skills: groups an item or the tree grants, gems the dataset lacks (dropped), names the dataset cannot choose between, a main skill group that is not imported, slots without sockets, the switches of a Vaal gem's two skills, and every group when the dataset has no skills group.

## Benchmarks

Each package benchmarks its own pieces with plain Go benchmarks; CI runs tests only. Each benchmark's doc comment names its target time.

```sh
go test -run '^$' -bench . -benchmem ./...          # every benchmark
go test -run '^$' -bench . -count 10 ./internal/core > new.txt # then benchstat old.txt new.txt
PBT_DATASETS=/tmp/pbt-data/poe1 go test -run '^$' -bench Nodes ./power   # node power on the real tree (see Develop)
```

## Semantics

What a game stat id means in engine terms is authored by us from game data: stat ids, stat description text, object templates. The `internal/semantics` package holds the table format: entries keyed by stat id and scope, with optional game version ranges, a status (`authored`, `unknown` or `display-only`), the sources each was authored from, and for unknown entries a typed reason (a scenario condition, equipped items, a rule still to write, and so on).

The Path of Exile 1 table (`internal/poe1/table.json`) has an entry for every stat on standard tree nodes, characters and campaign rewards that its predicate, which reads the English description text, finds can change a character sheet value: attributes, life, mana, energy shield, armour, evasion, resistances and their maximums, life and mana regeneration. Stats whose text names a sheet value but that the predicate rejects (enemy and minion stats, costs, reservation, penetration, recovery) are listed per dataset in `internal/poe1/testdata` for review. The coverage report per dataset, `internal/poe1/testdata/coverage-<version>.txt`, lists the stats that feed the sheet with no entry, the unknown ones with their reason, and the authored ones not yet validated or with effects left out, most used first; `go test ./internal/poe1 -update` rewrites it (see Develop).

## Rules

`internal/poe1` computes the character sheet of a resolved build: `Open(resolved, table)` turns each bundle stat into engine modifiers through the semantics table, adds the rules' own (attribute bonuses, conversions, resistance caps) and returns a `Sheet`. `Sheet.Output` reads the named outputs of `Outputs()`, the catalogue consumers render from: attributes, life, mana, energy shield, armour, evasion rating, resistances with their maximums and uncapped totals, and regeneration per second. Stats the table cannot apply are listed in `Sheet.Notes`, not errors. Where neither game text nor game data states a formula detail, such as rounding, it follows Path of Building as a starting point and is marked unvalidated.

## Develop

Requires Go 1.27.

```sh
go build ./...
go test ./...
golangci-lint run
go test ./internal/core -run '^$' -bench .   # benchmarks
```

Tests that need real datasets (sheets, goldens, node power, the semantics table and its fixtures, the dataset groups, the walkthrough, the API sketch) read the datasets under `$PBT_DATASETS` by exact version and dataset id, pinned in `internal/realdata` (3.28.0.16 and 3.29.3.6). They skip without `$PBT_DATASETS` and fail when it lacks a pinned dataset. Check out the pbt-data commit that holds them:

```sh
git clone https://github.com/xaroth/pbt-data /tmp/pbt-data
git -C /tmp/pbt-data checkout 4221dc07cd12412079d4f69f22dc702168fb3774
PBT_DATASETS=/tmp/pbt-data/poe1 go test ./...
PBT_DATASETS=/tmp/pbt-data/poe1 go test ./internal/poe1 ./statdesc -update   # rewrite the fixtures, coverage reports, golden lists and explanation and read goldens
go test ./simulator -run Sketch -update   # rewrite the list of API sketch parts that cannot run yet
```

## Licence

Apache-2.0, see [LICENSE](LICENSE) and [NOTICE](NOTICE).

This product isn't affiliated with or endorsed by Grinding Gear Games in any way. Path of Exile and its game data are the property of Grinding Gear Games.
