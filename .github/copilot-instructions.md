# Codex Client Go Library - AI Agent Guide

## Project Overview
A lightweight Go client library for interacting with Codex decentralized storage nodes. The project provides:
- **Core Library**: `communities/codex_client.go` - HTTP client wrapping Codex REST API
- **CLI Tools**: `cmd/upload/` and `cmd/download/` - Command-line utilities for file operations

## Quickstart (build • run • test)
- Build CLIs: `go build -o bin/codex-upload ./cmd/upload && go build -o bin/codex-download ./cmd/download`
- Upload: `./bin/codex-upload -file test-data.bin -host localhost -port 8080`
- Download: `./bin/codex-download -cid <CID> -file out.bin -host localhost -port 8080`
- Unit tests: `go test -v ./communities`
- Integration test: `go test -v -tags=integration ./communities -run Integration`

## Architecture & Design Patterns

### Client Structure
The `CodexClient` type in `communities/codex_client.go` is the central abstraction:
- Wraps Codex HTTP API (`/api/codex/v1/data/*`) with Go-idiomatic methods
- Supports both network downloads (`/network/stream`) and local downloads (direct CID access)
- Context-aware operations for cancellation support (`DownloadWithContext`, `LocalDownloadWithContext`)
- All uploads use `application/octet-stream` with `Content-Disposition` header for filenames

### Data Flow Pattern
1. **Upload**: `file → io.Reader → HTTP POST → CID string`
2. **Download**: `CID string → HTTP GET → io.Writer → file`
3. Network vs Local: Network download uses `/network/stream` endpoint (retrieves from DHT), local uses direct `/data/{cid}`

### Key Implementation Details
- **Streaming with Cancellation**: Custom `copyWithContext` (64KB buffer) checks context cancellation between chunks
- **Default Timeout**: 60s for HTTP operations (configurable via `SetRequestTimeout`)
- **Error Handling**: Errors wrapped with `%w`; HTTP errors include status code and body
- **CID Format**: CIDv1 + multibase base58btc (prefix `z`), using the `raw` codec (`Dv`). Example: `zDvZRwzm...`

### CID format cheatsheet
- Multibase prefix `z` = base58btc; `Dv` denotes CIDv1 + raw codec
- CIDs commonly start with `zDv...` in Codex (e.g., `zDvZRwzmRigWseNB...`)
- Upload responses may include a trailing newline; client trims via `strings.TrimSpace`

## CLI Commands & Usage

### Build Commands
```bash
# Build upload tool
go build -o codex-upload ./cmd/upload

# Build download tool  
go build -o codex-download ./cmd/download

# Or build both
go build ./cmd/...
```

### Running CLI Tools
```bash
# Upload a file (defaults to localhost:8080)
go run ./cmd/upload -file test-data.bin -host localhost -port 8080

# Download by CID
go run ./cmd/download -cid <CID> -file output.bin -host localhost -port 8080
```

CLI notes:
- Download writes to an `io.Writer` (a created file). On failure, the tool deletes the partial file.
- Flags follow Go `flag` conventions; `-h/--help` prints usage.

## Development Conventions

### Module & Import Pattern
- Module name: `go-codex-client` (defined in `go.mod`)
- Internal imports use: `"go-codex-client/communities"`
- No external dependencies beyond Go stdlib (HTTP, context, io operations)

### Error Handling Style
- Use `log.Fatalf` in CLI tools for unrecoverable errors
- Return wrapped errors from library functions: `fmt.Errorf("context: %w", err)`
- HTTP errors include status codes and response body in error messages

### Flag Usage Pattern (CLI)
Standard pattern across CLI tools:
```go
var (
    host = flag.String("host", "localhost", "Codex host")
    port = flag.String("port", "8080", "Codex port")
    // tool-specific flags...
)
flag.Parse()
```

## Status & Known Issues
- Download CLI fixed: creates the output file and passes it as `io.Writer` to `client.Download`; removes the file on error.

## Integration Notes
- **Codex API Version**: Uses v1 endpoints (`/api/codex/v1/`)
- **Content Type**: Always `application/octet-stream` for uploads
- **CID Response**: Upload returns a CID string (often newline-terminated); client trims whitespace
- **Network Operations**: Assumes Codex node is running and accessible at specified host:port

## Common failure modes (and fixes)
- 404 on download: CID not found or node can’t retrieve from network; verify CID and node connectivity
- 500/Bad Gateway: upstream node error; retry or check Codex node logs
- Timeouts: increase via `client.SetRequestTimeout(...)` or set `CODEX_TIMEOUT_MS` in integration tests
- Partial file on download error: download CLI already deletes the file on failure

## Testing & Debugging
- Unit tests (stdlib) in `communities/codex_client_test.go` cover:
    - Upload success (headers validated) returning CID
    - Download success to a `bytes.Buffer`
    - Cancellation: streaming handler exits on client cancel; fast and warning-free
- Run: `go test -v ./communities`
- Integration test (requires Codex node): `communities/codex_client_integration_test.go`
    - Build tag-gated: run with `go test -v -tags=integration ./communities -run Integration`
    - Env: `CODEX_HOST` (default `localhost`), `CODEX_API_PORT` (default `8080`), optional `CODEX_TIMEOUT_MS`
    - Uses random 1KB payload, logs hex preview, uploads, downloads, and verifies equality
- Debug by observing HTTP responses and Codex node logs; client timeout defaults to 60s

## Repo Meta
- `.gitignore` excludes build artifacts (`bin/`, binaries), coverage outputs, and IDE files
- `README.md` documents build/run/test workflows
- `COPYRIGHT.md` provides MIT license
