---
name: pactum
description: Use Pactum's contract-first workflow for non-trivial repository code changes. Use when a code task needs project-map context, contract approval, a prompt boundary, a safe plan step, or the gate/review/memory workflow. Do not use for quick questions, for trivial edits, or when the user asks not to use Pactum.
---

# Pactum — contract-first agent workflow

Pactum turns a repository code task into an auditable, contract-first run:
project map → task → clarification → approved contract → prompt boundary →
execution plan. It is a CLI you drive with shell commands.

This file is the portable skill. The full procedure and rationale live in the
reference files next to it — **read them with your file tools before acting**:

- `references/workflow.md` — the full command-by-command workflow, search
  strategy, current-run usage, and reporting format. Read this before starting.
- `references/install.md` — installing the `pactum` CLI and this skill.
- `references/safety.md` — the rules for real (unsandboxed) agent execution.
  Read this before running anything beyond the plan step.

## Scope

- Use for non-trivial repository code-change tasks that benefit from map
  context, an approved contract, and a checked prompt boundary.
- Do not use for quick questions, trivial one-line edits, or when the user
  asks you not to use Pactum.

## Mandatory safety rules (do not skip)

- Verify the CLI first: run `which pactum`. If it is not found, stop and follow
  `references/install.md`; do not continue until `which pactum` succeeds.
- **Do not run `pactum execute run` by default.** The default stop point is
  `pactum execute plan`.
- **Do not run `pactum review run` by default.**
- Do not pass `--yes` unless the user has explicitly approved unsandboxed,
  direct agent execution. Pactum runs agents as real subprocesses; this is not
  sandboxed.
- Do not commit `.heurema/` — it is generated, machine-specific workspace state.
- Source files are the source of truth. Pactum's map, wiki, code-items, and
  memory are navigation and audit context, not semantic truth — verify against
  the actual files before relying on them.
- Never hide a non-zero command exit; report failures with their output.

## Safe workflow (skeleton)

Read `references/workflow.md` for the detailed version. The safe default flow:

1. `which pactum` — verify the CLI (else stop, see `references/install.md`).
2. `pactum init` (only if not already initialized) and `pactum status`.
3. `pactum map refresh` if the map is stale.
4. `pactum task new "<task description>"` — creates the run and sets it current.
5. Targeted `pactum search "<term>"` (identifiers, paths, domain terms; with
   `--kind wiki`, `--kind code_item`, `--kind import`) and read the relevant
   `map/wiki/` pages and source files.
6. `pactum clarify add "..." --blocking` / `pactum clarify answer q_001 "..."`
   if anything is ambiguous.
7. `pactum contract revise` with goal, in-scope, out-of-scope, acceptance
   criteria, and validation commands.
8. `pactum contract approve --by manual` — only after the scope is clear.
9. `pactum prompt build` then `pactum prompt show`.
10. `pactum execute plan --agent codex` — the safe stop point.
11. Report: current run id, relevant files, contract summary, the plan
    command, and the recommended next action.

Stop at step 10 and report. Only proceed to real execution after the user
explicitly approves it (see `references/safety.md`).
