BINARY  := tmux-notepad
PREFIX  ?= $(HOME)/.local
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build test check fmt vet install clean

all: check build

build:
	go build -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/tmux-notepad

test:
	go test ./...

vet:
	go vet ./...

fmt:
	@test -z "$$(gofmt -l . | tee /dev/stderr)" || { echo 'gofmt found unformatted files'; exit 1; }

check: fmt vet test

install: build
	install -Dm755 $(BINARY) $(PREFIX)/bin/$(BINARY)

clean:
	rm -f $(BINARY)
