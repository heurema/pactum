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
- The durable `.heurema/pactum/` run record (config, ledger, contracts,
  decisions, gate verdicts, review findings, memory) is version-controlled; the
  selective `.heurema/pactum/.gitignore` keeps the regenerable/machine-specific
  parts out (`map/`, `cache/`, `tmp/`, `locks/`, `runs/*/context/`, and `*.log`
  transcripts).
- Never commit absolute local paths.
- Feature PRs stay code-only; the run-record churn is committed separately in
  periodic `audit: record runs` batches.

## Before reporting code changes

- Run `make check` (tests, vet, and the whitespace/conflict-marker check).
- Report failures honestly with their output; do not claim code changed unless
  it actually changed.
