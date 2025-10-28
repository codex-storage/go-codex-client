# go-codex-client

A lightweight Go client utility for interacting with Codex client.

## Project layout
- `communities/codex_client.go` — core HTTP client (upload/download, context-aware streaming)
- `cmd/upload/` — CLI to upload a file to Codex
- `cmd/download/` — CLI to download a file by CID
- `.github/copilot-instructions.md` — guidance for AI coding agents

We will be running codex client, and then use a small testing utility to check if the low level abstraction - CodexClient - correctly uploads and downloads the content.

### Integration Codex library

You need to download the library file by using: 

```sh
make fetch
```

### Building codex-upload and codex-download utilities

Use the following command to build the `codex-upload` and `codex-download` utilities:

```bash
make build-upload
make build-download
```
### Uploading content to Codex

Now, using the `codex-upload` utility, we can upload the content to Codex as follows:

```bash
~/code/local/go-codex-client
❯ ./bin/codex-upload -file test-data.bin
Uploading test-data.bin (43 bytes) to Codex
✅ Upload successful!
CID: zDvZRwzm8K7bcyPeBXcZzWD7AWc4VqNuseduDr3VsuYA1yXej49V
```

### Downloading content from Codex

Now, having the content uploaded to Codex - let's get it back using the `codex-download` utility:

```bash
~/code/local/go-codex-client
❯ ./bin/codex-download -cid zDvZRwzm8K7bcyPeBXcZzWD7AWc4VqNuseduDr3VsuYA1yXej49V -file output.bin
Downloading CID zDvZRwzm8K7bcyPeBXcZzWD7AWc4VqNuseduDr3VsuYA1yXej49V from Codex...
✅ Download successful!
Saved to: output.bin
```

You can easily compare that the downloaded content matches the original using:

```bash
~/code/local/go-codex-client
❯ openssl sha256 test-data.bin
SHA2-256(test-data.bin)= c74ce73165c288348b168baffc477b6db38af3c629b42a7725c35d99d400d992

~/code/local/go-codex-client
❯ openssl sha256 output.bin
SHA2-256(output.bin)= c74ce73165c288348b168baffc477b6db38af3c629b42a7725c35d99d400d992
```

### Running tests

We have some unit tests and a couple of integration tests.

In this section we focus on the unit tests. The integration tests are covered in the
next section.

To run all unit tests:

```bash
❯ make test
=== RUN   TestUpload_Success
--- PASS: TestUpload_Success (0.00s)
=== RUN   TestDownload_Success
--- PASS: TestDownload_Success (0.00s)
=== RUN   TestDownloadWithContext_Cancel
--- PASS: TestDownloadWithContext_Cancel (0.04s)
PASS
ok  	go-codex-client/communities	0.044s
```

To run the integration test, use `test-integration`:

```bash
make test-integration
```

### Regenerating artifacts

Everything you need comes included in the repo. But if you decide to change things,
you will need to regenerate some artifacts. There are two:

- the protobuf
- the mocks

For the first one - protobuf - you need two components:
1. **`protoc`** - the Protocol Buffer compiler itself
2. **`protoc-gen-go`** - the Go plugin for protoc that generates `.pb.go` files

#### Installing protoc

I have followed the instructions from [Protocol Buffer Compiler Installation](https://protobuf.dev/installation/).

The following bash script (Arch Linux) can come in handy:

```bash
#!/usr/bin/env bash

set -euo pipefail

echo "installing go..."

sudo pacman -S --noconfirm --needed go

echo "installing go protoc compiler"

PB_REL="https://github.com/protocolbuffers/protobuf/releases"
VERSION="32.1"
FILE="protoc-${VERSION}-linux-x86_64.zip"

# 1. create a temp dir
TMP_DIR="$(mktemp -d)"

# ensure cleanup on exit
trap 'rm -rf "$TMP_DIR"' EXIT

echo "Created temp dir: $TMP_DIR"

# 2. download file into temp dir
curl -L -o "$TMP_DIR/$FILE" "$PB_REL/download/v$VERSION/$FILE"

# 3. unzip into ~/.local/share/go
mkdir -p "$HOME/.local/share/go"
unzip -o "$TMP_DIR/$FILE" -d "$HOME/.local/share/go"

# 4. cleanup handled automatically by trap
echo "protoc $VERSION installed into $HOME/.local/share/go"
```

After that make sure that `$HOME/.local/share/go/bin` is in your path, and you should get:

```bash
protoc --version
libprotoc 32.1
```

#### Installing protoc-gen-go

The `protoc-gen-go` plugin is required to generate Go code from `.proto` files. 
Install it with:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.1
```

Make sure `$(go env GOPATH)/bin` is in your `$PATH` so protoc can find the plugin.

Verify the installation:

```bash
which protoc-gen-go
protoc-gen-go --version
# Should output: protoc-gen-go v1.34.1
```

#### Installing mockgen

In order to regenerate mocks you will need `mockgen`.

You can install it with:

```bash
go install go.uber.org/mock/mockgen
```

> Also make sure you have `$(go env GOPATH)/bin` in your PATH. Otherwise
make sure you have something like `export PATH="$PATH:$(go env GOPATH)/bin"` 
in your `~/.bashrc` (adjusted to your SHELL and OS version). 
This should be part of your standard GO installation.

If everything works well, you should see something like:

```bash
❯ which mockgen && mockgen -version
/home/<your-user-name>/go/bin/mockgen
v0.6.0
```

If everything seems to be under control, we can now proceed with actual generation.

The easiest way is to regenerate all in one go:

```bash
go generate ./...
```

If you just need to regenerate the mocks:

```bash
go generate ./communities
```

If you just need to regenerate the protobuf:

```bash
go generate ./protobuf
```
