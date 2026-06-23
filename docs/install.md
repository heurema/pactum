# Installing Pactum

Pactum ships as an npm package, as prebuilt binaries on GitHub Releases, and as
a Go module you can build from source. Most users want npm.

## npm (recommended)

```sh
npm i -g @heurema/pactum     # then: pactum --help
# or run ad-hoc, no install:
npx @heurema/pactum --help
```

Supported: macOS (arm64/x64) and Linux (amd64/arm64, glibc). On first run the
launcher downloads the prebuilt binary for your platform from the matching GitHub
Release, verifies its checksum against the manifest baked into the package, caches
it under `~/.cache/pactum/<version>/`, and runs it — no build toolchain required.
See [docs/install-npm.md](install-npm.md). Windows and Alpine/musl are not yet
supported (the launcher exits with a clear message).

## Prebuilt binary

Download the archive for your platform from
[Releases](https://github.com/heurema/pactum/releases), verify it against
`checksums.txt`, and put `pactum` on your `PATH`. Each archive also bundles the
docs and the agent skill.

## Build from source

The rest of this page covers building from source (Go 1.26+) and a first-repo smoke check.

## Prerequisites

- **Go 1.26+** — the module targets `go 1.26` (see `go.mod`). Check with
  `go version`.
- **Git** — Pactum operates inside a Git repository and uses Git to detect
  repository changes during gating.

### Optional: ACP adapter packages

Pactum can drive two built-in agents, but only when you ask it to *execute*
(`pactum execute run`). The packaging and smoke steps below never launch an
agent, so the adapter packages are optional for installation:

- `@heurema/codex-acp@latest` — ACP adapter for the `codex` built-in agent.
- `@agentclientprotocol/claude-agent-acp@latest` — ACP adapter for the `claude` built-in agent.

Pactum downloads and runs these via `npx` on demand. Set
`PACTUM_CODEX_ACP_COMMAND` or `PACTUM_CLAUDE_ACP_COMMAND` to override the
launch command. See [docs/agents.md](agents.md) for the execution model.

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
make test             # go test ./...
make vet              # go vet ./...
make check            # test + vet + deadcode + git diff --check
make vuln             # govulncheck ./... (pinned via the go.mod tool directive)
make heurema-hygiene  # scan committed .heurema run records for local paths/credentials
make smoke            # build + run scripts/smoke.sh
make clean            # remove ./bin
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
`contract`, `execute`, `gate`, `review`, `memory`, `doctor`, `version`, ...).

## First-repo smoke check

Inside any Git repository you want Pactum to manage:

```sh
pactum init             # create .heurema/pactum/ workspace
pactum status           # show workspace status
pactum task new "demo"  # create a contract-first run (becomes the current run)
pactum doctor           # check whether codex/claude are on PATH
```

Notes:

- `pactum init` creates the workspace at `.heurema/pactum/`. It does not run any agent.
- `pactum task new` creates a run and records it as the current run, so the
  staged commands (`contract approve`, `prompt build`, ...) can omit the run id.
- `pactum doctor` checks your `PATH` for the ACP adapter launcher (`npx` for
  built-in agents). It does **not** launch the agents and does **not**
  authenticate them; a `missing_command` status means the launcher is not on
  your `PATH`.

To exercise the whole safe surface in a throwaway repository automatically, run
the bundled smoke script from a clone:

```sh
scripts/smoke.sh
```

It builds `bin/pactum`, creates a temporary Git repo, and runs `version`,
`init`, `status`, `task new`, and `doctor` — never a real agent — then
cleans up.

## Continuous integration

GitHub Actions (`.github/workflows/ci.yml`) runs the same checks on every pull
request and push to `main`: `make check`, `make heurema-hygiene`, `make build`,
and `scripts/smoke.sh`, plus separate `make test-race` and `make vuln` jobs.
CI does not need ACP adapter packages installed and never runs a real agent.

See [CHANGELOG.md](../CHANGELOG.md) for notable changes. Everything is currently
**Unreleased** — there are no packaged releases yet.

## npm / prebuilt binary

Once releases are tagged, you can also install the prebuilt binary via npm:

```sh
npm i -g @heurema/pactum
```

See [docs/install-npm.md](install-npm.md) for the full npm install guide,
supported platforms, and the binary cache location.

## What is not included yet

- No packaged releases or prebuilt binaries.
- No Docker image.
- No release-publishing automation.
- No Windows smoke script (the smoke script targets Linux and macOS).
