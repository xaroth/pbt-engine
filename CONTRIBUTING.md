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

## Dataset format

`dataset/schema/*.fbs` is the dataset schema. After changing it, run `go generate ./dataset` with `flatc` 25.12.19 on `PATH` (the version must match the `github.com/google/flatbuffers` module) and commit the generated `dataset/internal/fb`. `dataset.FormatVersion` stays at 1 until the first release. After that, a change that older datasets cannot read bumps it.

## Licence

Contributions are accepted under Apache-2.0 (see [LICENSE](LICENSE), section 5).
