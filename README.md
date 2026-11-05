# pbt-engine

A headless build simulation engine for Path of Exile. It takes a build and a game version and returns the numbers a player cares about, each with an explanation of where it came from.

Status: early. Nothing usable yet.

## Develop

Requires Go 1.27.

```sh
go build ./...
go test ./...
golangci-lint run
```

## Licence

Apache-2.0, see [LICENSE](LICENSE) and [NOTICE](NOTICE).

This product isn't affiliated with or endorsed by Grinding Gear Games in any way. Path of Exile and its game data are the property of Grinding Gear Games.
