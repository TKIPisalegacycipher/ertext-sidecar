# ertext-sidecar

The Google Messages backend for [Ertext](https://github.com/TKIPisalegacycipher/Ertext), a Windows
Google Messages client. Ertext starts this process and talks to it over stdio using the protocol in
`proto/ertext/v1/ertext.proto`. The process wraps
[mautrix-gmessages](https://github.com/mautrix/gmessages)' `libgm` and keeps no state on disk.

## License

- This program: **AGPL-3.0** (see `LICENSE`), because it links libgm (AGPL-3.0).
- `proto/`: **MIT** (see `proto/LICENSE`), so other programs can implement the protocol.

## Build

    go build -trimpath -ldflags "-X main.version=$(git describe --always --dirty)" -o ertext-sidecar.exe ./cmd/ertext-sidecar

## Develop

    go test ./...
    go tool buf lint
    go tool buf generate   # after editing the .proto; commit the regenerated internal/pb

## Protocol

Frames: a u32 little-endian length, then a protobuf `ertext.v1.Envelope`, with a 16 MiB maximum.
The sidecar first writes `Event{hello}`, then answers every `Request` with exactly one `Response`
carrying the same `id`, in any order, and sends `Event`s at any time. Logs go to stderr.
Golden frames for cross-language tests are in `proto/testdata/`.
