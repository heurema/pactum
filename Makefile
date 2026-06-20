# Pactum local development tasks.
#
# These targets are deliberately boring: they wrap the standard Go toolchain so
# that building, testing, and installing Pactum from source is a single command.
# There is no release, packaging, or Docker automation here.

.PHONY: build test vet deadcode test-race vuln heurema-hygiene check install clean smoke

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

# test-race runs the suite under the race detector. It is ~20x slower on the app
# package, so it is a CI / pre-merge gate rather than part of the fast `make
# check`. It catches the data-race class the plain test run cannot — e.g. the
# M10.2 live-output race that slipped through a non-race `make check`.
test-race:
	go test -race -timeout 20m ./...

# deadcode flags functions unreachable from any main entry point (golang.org/x/
# tools, pinned via the go.mod tool directive). It catches what `go vet` cannot:
# unused package-level functions, including production code reachable only from
# tests. Any finding fails the gate; the tree is expected to stay empty.
deadcode:
	@out="$$(go tool deadcode ./...)"; \
	if [ -n "$$out" ]; then \
		echo "$$out"; \
		echo "deadcode: unreachable functions found (above); remove them"; \
		exit 1; \
	fi

# vuln scans dependencies and stdlib usage for known vulnerabilities
# (golang.org/x/vuln, pinned via the go.mod tool directive). CI runs it as its
# own blocking job so a slow vulndb fetch never delays the main check loop;
# any nonzero exit — finding, fetch failure, or tool failure — fails the gate.
vuln:
	go tool govulncheck ./...

# heurema-hygiene is the deterministic leak gate for the committed .heurema
# run record: it scans tracked and staged-added .heurema files (the git index,
# never unrelated untracked files) for absolute home-directory paths and
# credential-shaped strings, and fails listing file:line, detector name, and a
# redacted preview for every finding. Detectors live in cmd/heurema-hygiene,
# which is itself outside the scanned tree.
heurema-hygiene:
	go run ./cmd/heurema-hygiene

# check is the local gate: tests, vet, dead-code, gofmt formatting, and a whitespace/conflict-marker check.
check: test vet deadcode
	@out="$$(gofmt -l $$(git ls-files '*.go'))"; \
	if [ -n "$$out" ]; then \
		echo "$$out"; \
		echo "gofmt: unformatted files above; run gofmt -w on them"; \
		exit 1; \
	fi
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
