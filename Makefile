# Destination folder for the downloaded libraries
LIBS_DIR := $(abspath ./libs)

# Flags for CGO to find the headers and the shared library
UNAME_S := $(shell uname -s)
CGO_CFLAGS  := -I$(LIBS_DIR)
CGO_LDFLAGS := -L$(LIBS_DIR) -lcodex -Wl,-rpath,$(LIBS_DIR)

ifeq ($(OS),Windows_NT)
  BIN_NAME := codex-go.exe
else
  BIN_NAME := codex-go
endif

# Configuration for fetching the right binary
OS ?= "linux"
ARCH ?= "amd64"
VERSION ?= "v0.0.22"
DOWNLOAD_URL := "https://github.com/codex-storage/codex-go-bindings/releases/download/$(VERSION)/codex-${OS}-${ARCH}.zip"		

fetch: 
	@echo "Fetching libcodex from GitHub Actions from: ${DOWNLOAD_URL}"
	curl -fSL --create-dirs -o $(LIBS_DIR)/codex-${OS}-${ARCH}.zip ${DOWNLOAD_URL}
	unzip -o -qq $(LIBS_DIR)/codex-${OS}-${ARCH}.zip -d $(LIBS_DIR)
	rm -f $(LIBS_DIR)/*.zip

build-upload:
	CGO_ENABLED=1 CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" go build -o bin/codex-upload ./cmd/upload

build-download:
	CGO_ENABLED=1 CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" go build -o bin/codex-download ./cmd/download

build: build-upload build-download

test:
	@echo "Running unit tests..."
	CGO_ENABLED=1 CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" gotestsum --packages="./communities" -f standard-verbose -- -v

test-ci:
	@echo "Running unit tests..."
	CGO_ENABLED=1 CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" gotestsum --packages="./communities" -f standard-verbose -- -race -v

test-integration:
	@echo "Running tests..."
	CGO_ENABLED=1 CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" gotestsum --packages="./communities" -f standard-verbose -- -tags=codex_integration -run Integration -timeout 60s

coverage: 
	@echo "Running unit tests with coverage..."
	CGO_ENABLED=1 CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" go test -coverprofile=coverage.out ./communities
	go tool cover -func=coverage.out

clean:
	rm -f $(BIN_NAME)
	rm -Rf $(LIBS_DIR)/*