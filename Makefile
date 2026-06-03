# Pactum local development tasks.
#
# These targets are deliberately boring: they wrap the standard Go toolchain so
# that building, testing, and installing Pactum from source is a single command.
# There is no release, packaging, or Docker automation here.

.PHONY: build test vet check install clean smoke

# Version metadata stamped into the binary. Override on the command line, e.g.
# `make build VERSION=0.1.0`.
VERSION ?= 0.1.0
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VERSION_PKG := github.com/heurema/pactum/internal/version
LDFLAGS := -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).Date=$(DATE)

# build compiles the pactum CLI into ./bin/pactum, stamping version metadata.
build:
	go build -ldflags "$(LDFLAGS)" -o bin/pactum ./cmd/pactum

# test runs the full Go test suite.
test:
	go test ./...

# vet runs go vet across all packages.
vet:
	go vet ./...

# check is the local gate: tests, vet, and a whitespace/conflict-marker check.
check: test vet
	git diff --check

# install builds and installs pactum into the Go bin directory (go env GOBIN).
install:
	go install -ldflags "$(LDFLAGS)" ./cmd/pactum

# smoke builds the binary and runs the local end-to-end smoke script.
smoke:
	scripts/smoke.sh

# clean removes locally built binaries.
clean:
	rm -rf bin
