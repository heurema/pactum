# Pactum local development tasks.
#
# These targets are deliberately boring: they wrap the standard Go toolchain so
# that building, testing, and installing Pactum from source is a single command.
# There is no release, packaging, or Docker automation here.

.PHONY: build test vet check install clean smoke

# build compiles the pactum CLI into ./bin/pactum.
build:
	go build -o bin/pactum ./cmd/pactum

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
	go install ./cmd/pactum

# smoke builds the binary and runs the local end-to-end smoke script.
smoke:
	scripts/smoke.sh

# clean removes locally built binaries.
clean:
	rm -rf bin
