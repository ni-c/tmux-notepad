BINARY  := tmux-notepad
PREFIX  ?= $(HOME)/.local
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

# The release build lives in scripts/build-dist.sh, which needs its own
# GOOS/GOARCH loop and sets -X main.version the same way this does.
RELEASE_VERSION := $(shell cat VERSION 2>/dev/null || echo 0.0.0)

.PHONY: all build test check fmt vet install dist clean

all: check build

build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/tmux-notepad

test:
	go test ./...

vet:
	go vet ./...

fmt:
	@test -z "$$(gofmt -l . | tee /dev/stderr)" || { echo 'gofmt found unformatted files'; exit 1; }

check: fmt vet test

# install -D is a GNU extension; macOS install(1) does not have it.
install: build
	mkdir -p $(PREFIX)/bin
	install -m755 $(BINARY) $(PREFIX)/bin/$(BINARY)

dist:
	scripts/build-dist.sh $(RELEASE_VERSION) dist

clean:
	rm -f $(BINARY)
	rm -rf dist
