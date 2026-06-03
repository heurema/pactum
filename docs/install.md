# Installing Pactum

Pactum is a Go module (`github.com/heurema/pactum`). There are no packaged
releases or prebuilt binaries yet — you build and install it from source. This
page covers the prerequisites, the local build/install paths, and a first-repo
smoke check.

## Prerequisites

- **Go 1.26+** — the module targets `go 1.26` (see `go.mod`). Check with
  `go version`.
- **Git** — Pactum operates inside a Git repository and uses Git to detect
  repository changes during gating.

### Optional: agent CLIs

Pactum can drive two built-in agents, but only when you ask it to *execute*
(`pactum execute run`). The packaging and smoke steps below never launch an
agent, so the agent CLIs are optional for installation:

- `codex` — used by the `codex` built-in agent (`codex exec`).
- `claude` — used by the `claude` built-in agent (`claude -p`).

Pactum does **not** install or authenticate these tools. You install and log in
to them yourself; Pactum only looks for them on your `PATH`. See
[docs/agents.md](agents.md) for the execution model.

## Build from source

From a clone of the repository:

```sh
# Run directly without building a binary.
go run ./cmd/pactum --help

# Or build a local binary with the Makefile.
make build
./bin/pactum --help
```

`make build` compiles the CLI to `./bin/pactum`. It stamps the build with version
metadata via `-ldflags`; override the version when building a release binary:

```sh
make build VERSION=0.1.0   # sets `pactum version` output
```

Other useful targets:

```sh
make test     # go test ./...
make vet      # go vet ./...
make check    # test + vet + git diff --check
make smoke    # build + run scripts/smoke.sh
make clean    # remove ./bin
```

## Install into your Go bin

```sh
go install ./cmd/pactum
pactum --help
```

This installs `pactum` into your Go bin directory (`go env GOBIN`, or
`$(go env GOPATH)/bin` if `GOBIN` is unset). Make sure that directory is on your
`PATH`.

Once releases are tagged, you will also be able to install a specific version
directly from the module path:

```sh
# Available after releases are tagged (no tags exist yet).
go install github.com/heurema/pactum/cmd/pactum@latest
```

## Verify the install

```sh
pactum --help
```

Check the build metadata with:

```sh
pactum version          # also: pactum version --json
```

You should also see the top-level command groups (`init`, `status`, `task`,
`contract`, `execute`, `gate`, `review`, `memory`, `agents`, `version`, ...).

## First-repo smoke check

Inside any Git repository you want Pactum to manage:

```sh
pactum init             # create .heurema/pactum/ and build the project map
pactum status           # show workspace + project map status
pactum task new "demo"  # create a contract-first run (becomes the current run)
pactum agents doctor    # check whether codex/claude are on PATH
```

Notes:

- `pactum init` creates the workspace at `.heurema/pactum/` and builds a
  deterministic project map and search index. It does not run any agent.
- `pactum task new` creates a run and records it as the current run, so the
  staged commands (`contract approve`, `prompt build`, ...) can omit the run id.
- `pactum agents doctor` only checks your `PATH` for the agent commands. It
  does **not** launch the agents and does **not** authenticate them; a
  `missing_command` status simply means the CLI isn't installed yet.

To exercise the whole safe surface in a throwaway repository automatically, run
the bundled smoke script from a clone:

```sh
scripts/smoke.sh
```

It builds `bin/pactum`, creates a temporary Git repo, and runs `version`,
`init`, `status`, `task new`, and `agents doctor` — never a real agent — then
cleans up.

## Continuous integration

GitHub Actions (`.github/workflows/ci.yml`) runs the same checks on every pull
request and push to `main`: `make check`, `make build`, and `scripts/smoke.sh`.
CI does not need `codex`/`claude` installed and never runs a real agent.

See [CHANGELOG.md](../CHANGELOG.md) for notable changes. Everything is currently
**Unreleased** — there are no packaged releases yet.

## What is not included yet

- No packaged releases or prebuilt binaries.
- No Docker image.
- No release-publishing automation.
- No Windows smoke script (the smoke script targets Linux and macOS).
