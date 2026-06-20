# Installing Pactum and this skill

This skill drives the `pactum` CLI. The CLI must be installed separately — the
skill does not install it for you.

## Install the Pactum CLI

From a clone of the Pactum repository:

```sh
make build
./bin/pactum version
```

Or install it onto your `PATH` with the Go toolchain:

```sh
go install ./cmd/pactum
```

After tagged releases are published, this will also work from anywhere:

```sh
go install github.com/heurema/pactum/cmd/pactum@latest
```

Verify:

```sh
which pactum
pactum version
```

`npm`/`npx` and `uvx` install paths are planned/being considered. They are
**not implemented yet** — do not assume they exist.

## Install this skill (one command)

Once the CLI is installed, install the skill package with:

```sh
# repo-scoped (default) — installs to .<agent>/skills/pactum/ in the working dir
pactum skill install --agent <claude|codex|auto|all>

# user-scoped — installs to $HOME/.<agent>/skills/pactum/
pactum skill install --agent <claude|codex|auto|all> --scope user
```

- `auto` detects which agents are present (CLI on PATH or known dot-directories)
  and installs to all detected targets.
- `all` installs to every known alpha target regardless of detection.
- Default scope is `repo` so a global skill does not trigger on every repository
  you open.

Verify after install:

```sh
pactum skill install --check --agent <claude|codex|auto|all>
```

Reload or restart your coding agent if the skill does not appear immediately.

## Install this skill (manual fallback)

The skill package lives inside the Pactum repository at
`assets/agent-skills/pactum/`. To install manually, copy it into the agent's
skills directory:

- **Codex:** copy `assets/agent-skills/pactum/` to `.agents/skills/pactum/`
  (repo-scoped) or `$HOME/.agents/skills/pactum/` (user-scoped).
- **Claude Code:** copy `assets/agent-skills/pactum/` to
  `.claude/skills/pactum/` (project-scoped) or `~/.claude/skills/pactum/`
  (user-scoped).

Both ecosystems load a skill from its `SKILL.md`; the `references/` files are
read on demand by the skill.

Marketplace and plugin packaging (a Claude plugin and a Codex plugin) are
deferred to a later milestone. The `pactum skill install` command is the
supported one-command install path.
