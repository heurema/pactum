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

## Install this skill (repo-local, manual)

The skill package currently lives inside the Pactum repository at
`assets/agent-skills/pactum/`. To make an agent discover it, copy the package
into the agent's skills directory:

- **Codex:** copy `assets/agent-skills/pactum/` to `.agents/skills/pactum/`
  (repo-scoped) or `$HOME/.agents/skills/pactum/` (user-scoped).
- **Claude Code:** copy `assets/agent-skills/pactum/` to
  `.claude/skills/pactum/` (project-scoped) or `~/.claude/skills/pactum/`
  (user-scoped).

Both ecosystems load a skill from its `SKILL.md`; the `references/` files are
read on demand by the skill.

Marketplace and plugin packaging (a Claude plugin and a Codex plugin) are
deferred to a later milestone. There is no marketplace-based install and no CLI
self-install command yet — install the CLI and copy the skill manually, as
above.
