.PHONY: build test install clean release

VERSION ?= 0.1.0
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/ganjar/ecorouter/internal/version.Version=$(VERSION) \
	-X github.com/ganjar/ecorouter/internal/version.Commit=$(COMMIT) \
	-X github.com/ganjar/ecorouter/internal/version.BuildDate=$(DATE)

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/eco ./cmd/eco

test:
	CGO_ENABLED=0 go test ./...

install: build
	install -m 755 bin/eco /usr/local/bin/eco

clean:
	rm -rf bin/

release:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/eco-linux-amd64 ./cmd/eco
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/eco-linux-arm64 ./cmd/eco
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/eco-windows-amd64.exe ./cmd/eco
