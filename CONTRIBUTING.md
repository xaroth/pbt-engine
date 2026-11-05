# Contributing

## Before a pull request

- `gofmt`, `go vet`, `go test ./...` and `golangci-lint run` pass.
- New behaviour has tests. Engine changes that affect results include a case showing the expected value and its source.
- One concern per pull request.

## Engine rules

- `core` imports nothing from this module.
- No globals and no I/O in the engine. Same input gives the same bits on every platform.
- Compute from game ids (stat ids, mod ids, node hashes), not from display text.
- Add a package when a task needs it, not before.

## Licence

Contributions are accepted under Apache-2.0 (see [LICENSE](LICENSE), section 5).
