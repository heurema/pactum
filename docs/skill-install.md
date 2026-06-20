# Installing the Pactum skill

The Pactum agent skill drives the `pactum` CLI, so the CLI must be installed
first — installing the skill does not install the CLI.

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

## 2. Install the skill

```sh
# repo-scoped (default) — installs to .<agent>/skills/pactum/ in this directory
pactum skill install --agent auto

# user-scoped — installs to $HOME/.<agent>/skills/pactum/
pactum skill install --agent auto --scope user

# target a specific agent
pactum skill install --agent claude
pactum skill install --agent codex

# install to every known alpha target
pactum skill install --agent all
```

`pactum skill install` writes the embedded skill package from the binary — no
source tree required after the CLI is installed.

## 3. Verify

```sh
pactum skill install --check --agent auto
```

Reload or restart your coding agent if the skill does not appear immediately.

## Manual install (fallback)

The embedded skill package also lives at `assets/agent-skills/pactum/` in the
source repository. To install manually without the CLI:

- **Codex:** copy `assets/agent-skills/pactum/` to `.agents/skills/pactum/`
  (or `$HOME/.agents/skills/pactum/`).
- **Claude Code:** copy `assets/agent-skills/pactum/` to `.claude/skills/pactum/`
  (or `~/.claude/skills/pactum/`).

See [`assets/agent-skills/pactum/references/install.md`](../assets/agent-skills/pactum/references/install.md)
for the full reference.

## Status

- `pactum skill install` is the supported one-command install path (alpha).
- Marketplace / plugin packaging is deferred to a later milestone.
- Real agent execution remains opt-in: the skill stops at
  `pactum execute plan` unless you explicitly approve direct, unsandboxed
  execution.
