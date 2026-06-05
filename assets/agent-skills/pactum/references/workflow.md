# Pactum workflow (full)

This is the detailed, command-by-command workflow referenced by `SKILL.md`.
Read `SKILL.md` first for the mandatory safety rules; they are not repeated in
full here.

## When to use

Use Pactum for non-trivial repository code-change tasks: anything where project
context, an explicit contract, and a checked prompt boundary reduce risk. Skip
it for quick questions and trivial edits, or when the user asks you not to.

## Core principle

Source files are the source of truth. Pactum's map, wiki, `code-items.jsonl`,
and memory are navigation and audit context — best-effort and incomplete by
design. Always confirm against the actual files before relying on them.

## Step-by-step

1. **Verify the CLI.** Run `which pactum`. If missing, stop and follow
   `references/install.md`. Do not continue until it succeeds.

2. **Check workspace state.** Run `pactum status`. If the workspace is not
   initialized, run `pactum init` (it creates `.heurema/pactum/` and builds the
   map). Do not re-run `init` on an already-initialized workspace.

3. **Refresh the map if stale.** If `pactum status` reports the project map as
   stale, run `pactum map refresh`.

4. **Create the task.** Run `pactum task new "<task description>"`. This creates
   a run, sets it as the current run, and assembles first-pass search context.
   Subsequent commands can omit the run id — they use the current run.

5. **Search and read.** Run targeted searches rather than one long sentence:
   - identifiers: `pactum search "formatCurrency"`
   - paths: `pactum search "apps/admin/src/lib/format.ts"`
   - domain terms: `pactum search "currency"`
   - filters: `--kind wiki`, `--kind code_item`, `--kind import`
   Then read `map/wiki/overview.md`, `structure.md`, `entrypoints.md`,
   `commands.md`, the relevant `areas/<area>.md`, and the actual source files.
   `--kind code_item` excludes imports; `--kind import` returns them.

6. **Clarify if needed.** For anything ambiguous:
   `pactum clarify ask "..." --blocking` then
   `pactum clarify answer q_001 "..."`. Blocking questions must be answered
   before the prompt can be built.

7. **Write the contract.** `pactum contract revise` with:
   - `--goal` — one clear sentence.
   - `--add-in-scope` — what will change (be specific: files/areas).
   - `--add-out-of-scope` — what will not.
   - `--add-acceptance` — observable criteria.
   - `--add-validation` — the commands that prove it (e.g. `make check`).
   Contract quality checklist: goal unambiguous, scope bounded, acceptance
   observable, validation runnable.

8. **Approve.** `pactum contract approve --by manual` — only once the scope is
   clear and clarifications are resolved. Approval pins a contract hash.

9. **Build the prompt boundary.** `pactum prompt build`, then
   `pactum prompt show`. `prompt build` re-derives the run's search context from
   the approved contract, so the executor context reflects the final scope.

10. **Safe dry-run.** `pactum execute dry-run --agent codex`. This validates the
    contract hash, map freshness, and prompt manifest, and prints the command
    that *would* run — without running a real agent. This is the default stop
    point.

11. **Report.** Summarize: current run id, the relevant files you found,
    the contract (goal/scope/acceptance/validation), the dry-run command, and
    the recommended next action.

## Current-run usage

`pactum task new` and `pactum task use` set the current run (recorded in
`.heurema/pactum/cache/current-run`). Staged commands (`clarify`, `contract`,
`prompt`, `execute`) then operate on it without a run id. Use
`pactum task list` / `pactum task current` to inspect.

## Stop conditions

- Stop at the dry-run and report. Do not run real execution unless the user
  explicitly approves it (see `references/safety.md`).
- If the contract is ambiguous, stop and ask rather than guessing scope.
- If validation would change files or run untrusted commands, confirm first.

## Reporting format

Report back with:
- Run: `<run id>` and status.
- Relevant files: the paths you inspected or will change.
- Contract: goal, in/out of scope, acceptance, validation.
- Dry-run: the exact command Pactum would run.
- Next action: what you recommend, and whether it needs the user's approval.
