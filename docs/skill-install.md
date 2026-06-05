# Installing the Pactum skill

The Pactum agent skill drives the `pactum` CLI, so the CLI must be installed
separately — installing the skill does not install the CLI.

This is the human summary; the authoritative steps live in
[`assets/agent-skills/pactum/references/install.md`](../assets/agent-skills/pactum/references/install.md).

## 1. Install the Pactum CLI

From a clone of this repository:

```sh
make build
./bin/pactum version
```

Or with the Go toolchain:

```sh
go install ./cmd/pactum
```

After tagged releases are published, `go install
github.com/heurema/pactum/cmd/pactum@latest` will also work. `npm`/`npx` and
`uvx` install paths are **planned/being considered, not implemented** — do not
assume they exist.

## 2. Install the skill (repo-local, manual)

The skill package lives at `assets/agent-skills/pactum/`. Copy it into your
agent's skills directory:

- **Codex:** copy `assets/agent-skills/pactum/` to `.agents/skills/pactum/`
  (or `$HOME/.agents/skills/pactum/`).
- **Claude Code:** copy `assets/agent-skills/pactum/` to `.claude/skills/pactum/`
  (or `~/.claude/skills/pactum/`).

## 3. Verify

```sh
which pactum
pactum version
```

## Status

- The skill is **repo-local** for now; marketplace / plugin packaging is
  deferred to a later milestone.
- Real agent execution remains opt-in: the skill stops at
  `pactum execute dry-run` unless you explicitly approve direct, unsandboxed
  execution.
