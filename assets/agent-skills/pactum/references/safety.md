# Execution safety

Read this before running anything beyond `pactum execute plan`.

## Real agent execution is not sandboxed

`pactum execute run` and `pactum review run` launch a real agent (Codex or
Claude) as a direct subprocess against your repository. This is **not
sandboxed** — the agent can read, write, and run commands. Pactum is honest
about this; the skill must be too.

## Rules

- The default stop point is `pactum execute plan --agent <name>`. Stop there
  and report.
- Run `pactum execute run` **only after** the user has explicitly approved
  unsandboxed, direct agent execution for this task. The same applies to
  `pactum review run`. These commands never prompt — running one is itself the
  decision, so do not run them without that explicit approval.
- Never hide a non-zero exit code. If a command fails, report it with its
  output rather than continuing.
- Do not commit `.heurema/`. It is generated, machine-specific workspace state.
- When working inside the Pactum repository itself, run `make check` before
  reporting that code changed, and report failures honestly.
- Do not claim code changed unless it actually changed. Source files remain the
  source of truth; verify before reporting.

## Why plan first

`pactum execute plan` validates the approved contract hash, the project
map's freshness, and the prompt manifest, and prints the exact command that
*would* run — giving you and the user a reviewable plan before any real,
unsandboxed execution.
