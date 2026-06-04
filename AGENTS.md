# Agent guidance

Instructions for coding agents working in this repository.

## Use Pactum for non-trivial changes

- For non-trivial code changes, use Pactum's contract-first workflow.
- Canonical Pactum agent skill package: `assets/agent-skills/pactum/` (start at
  its `SKILL.md`). Human overview: `docs/agent-skill.md`.

## Safety

- Do not run real agents (`pactum execute run`, `pactum review run`) without
  explicit approval — agent execution is unsandboxed.
- The default stop point for the Pactum workflow is `pactum execute dry-run`.
- Do not commit `.heurema/`; it is generated, machine-specific workspace state.
- Keep generated artifacts out of commits.

## Before reporting code changes

- Run `make check` (tests, vet, and the whitespace/conflict-marker check).
- Report failures honestly with their output; do not claim code changed unless
  it actually changed.
