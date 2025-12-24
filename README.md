# MapStore for Go

[![License: MIT](https://img.shields.io/badge/License-MIT-brightgreen.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/ppipada/mapstore-go)](https://goreportcard.com/report/github.com/ppipada/mapstore-go)
[![lint](https://github.com/ppipada/mapstore-go/actions/workflows/lint.yml/badge.svg?branch=main)](https://github.com/ppipada/mapstore-go/actions/workflows/lint.yml)
[![test](https://github.com/ppipada/mapstore-go/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/ppipada/mapstore-go/actions/workflows/test.yml)

MapStore is a local, filesystem‑backed map database with pluggable codecs (JSON or custom), optional per‑key encryption via the OS keyring, and optional full‑text search via SQLite FTS5.

## Features

- File store

  - It keeps a `map[string]any` in sync with files on disk, the file can be encoded as JSON (inbuilt), or any format using a custom file encoder/decoder.
  - It is a thread-safe map store with atomic file writes and optimistic concurrency.
  - Pluggable codecs for both keys and values inside the map, including an encrypted string encoder backed by `github.com/zalando/go-keyring`.
  - Listener hooks so callers can observe every mutation written to disk.
  - Optional SQLite FTS5 integration for fast search, with helpers for incremental sync.

- Directory store: A convenience manager that partitions data across subdirectories and paginates listings.

- Pure Go implementation with no cgo, compatible with Go 1.25+.

## Capabilities and Extensibility

- **File encoders**

  - Supply your own `IOEncoderDecoder` via `WithFileEncoderDecoder`.
  - _JSON file encode/decode_ - use the inbuilt `jsonencdec.JSONEncoderDecoder` to encode/decode files as JSON.

- **Encode key or value at sub-path**

  - Override encoding of specific keys or values with `WithKeyEncDecGetter` or `WithValueEncDecGetter`.
  - _Value encryption_ - use the inbuilt `keyringencdec.EncryptedStringValueEncoderDecoder` to transparently store sensitive string values through the OS keyring.

- **Directory Partitioning**

  - Swap in your own `PartitionProvider` to control directory layout.
  - _Month based partitioning_ - use the inbuilt `dirpartition.MonthPartitionProvider` to split files across month based directories.

- **File naming**

  - The file store is opaque to filenames, allowing for any naming scheme.
  - The directory store uses a `FileKey` based design to allow for control of encoding and decoding of data inside file names for efficient traversal.
  - _UUIDv7 based filename provider_ - use the inbuilt UUIDv7 based provider to derive and use, collision free and semantic data based filenames.

- **File change events**

  - Custom listeners can be plugged into the file store to observe file events.
  - Pluggable _Full text search_
    - Inbuilt, pure go, sqlite backed (via [glebarez driver](https://github.com/glebarez/go-sqlite) + [modernc sqlite](https://pkg.go.dev/modernc.org/sqlite)), fts engine.
    - Pluggable iterator utility `ftsengine.SyncIterToFTS` for efficient, incremental index updates.

## Installation

```bash
go get github.com/ppipada/mapstore-go
```

## Quick Start

- Single file store: see [ExampleMapFileStore](internal/integration/filestore_example_test.go).
- Managing files inside a directory: see [ExampleMapDirectoryStore](internal/integration/dirstore_example_test.go).
- A full store with JSON encode decode, month partitioning, uuid filenames and FTS enabled: see [ExampleMapDirectoryStore_full](internal/integration/conversation_fts_example_test.go).
- Additional samples can be seen in integration tests under [integration tests](internal/integration).

## Packages

- `github.com/ppipada/mapstore-go` — file and directory stores, options, events, and types.
- `github.com/ppipada/mapstore-go/jsonencdec` — JSON file encoder/decoder.
- `github.com/ppipada/mapstore-go/keyringencdec` — per-value encryption (AES‑256‑GCM; key in OS keyring).
- `github.com/ppipada/mapstore-go/dirpartition` — partitioning strategies (month/no-op).
- `github.com/ppipada/mapstore-go/uuidv7filename` — UUIDv7‑backed filename helpers.
- `github.com/ppipada/mapstore-go/ftsengine` — FTS5 engine and sync helpers.

## Concurrency Model

MapStore uses optimistic concurrency when writing files:

- Writes create a deep copy of the in‑memory map, encode it (value encoding first, then key encoding), and atomically rename a temp file into place.
- Before writing, the store compares current file `stat` to a remembered snapshot. If it changed, `SetAll` and `DeleteFile` return a conflict error. `SetAll` retries a few times automatically; key‑level mutations (`SetKey`/`DeleteKey`) honor the `AutoFlush` setting and propagate conflict errors from the internal flush.
- This is best‑effort across processes. Two writers racing between the pre‑write CAS check and `rename` may still result in last‑writer‑wins. If you need stronger cross‑process guarantees, coordinate at the application level.

## Keyring Notes

- `keyringencdec.EncryptedStringValueEncoderDecoder` uses the OS keyring to store the AES‑256 key.
- In headless or container environments, ensure a compatible keyring backend is available, or avoid using the keyring‑based encoder.

## Development

- Formatting follows `gofumpt` and `golines` via `golangci-lint`, which is also used for linting. All rules are in [.golangci.yml](.golangci.yml).
- Useful scripts are defined in `taskfile.yml`; requires [Task](https://taskfile.dev/).
