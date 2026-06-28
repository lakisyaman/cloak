.PHONY: test test-integration build install dist

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

test:
	go test ./...

test-integration:
	go test -tags=integration ./test/integration -count=1 -v

# Build a version-stamped binary into bin/cloak.
build:
	go build -ldflags "$(LDFLAGS)" -o bin/cloak ./cmd/cloak

# Install a version-stamped binary into $GOBIN / $GOPATH/bin.
install:
	go install -ldflags "$(LDFLAGS)" ./cmd/cloak

# Build release artifacts locally without publishing (requires goreleaser).
dist:
	goreleaser release --snapshot --clean
