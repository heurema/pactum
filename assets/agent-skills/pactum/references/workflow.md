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
   - known symbols: `pactum search --symbol formatCurrency` (exact,
     case-insensitive; no positional query needed)
   - paths: `pactum search "apps/admin/src/lib/format.ts"`
   - domain terms: `pactum search "currency"`
   - filters: `--kind wiki`, `--kind code_item`, `--kind import`
   Code-item results carry a `path:start-end` range and signature — read that
   line range directly instead of scanning the whole file (line numbers hold
   until you edit the file). Then read `map/wiki/overview.md`,
   `structure.md`, `entrypoints.md`, `commands.md`, the relevant
   `areas/<area>.md`, and the actual source files. `--kind code_item`
   excludes imports; `--kind import` returns them.

6. **Clarify if needed.** For anything ambiguous:
   `pactum clarify add "..." --blocking`, then record the answer. A typed
   `pactum clarify answer q_001 "..."` is the explicit human answer; when a
   question carries a stored recommended answer the human agrees with, relay it
   with `pactum clarify answer q_001 --recommended`, or answer every open
   recommended question at once with `pactum clarify answer --all-recommended`.
   Blocking questions must be answered before the prompt can be built.
   (`pactum clarify run` launches a clarifier agent and is not part of the safe
   default flow; `--no-auto --max-rounds 1` is its single manual pass.)

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
   the approved contract, so the executor context reflects the final scope. A
   stale project map does not block it: the build refreshes the map itself and
   records the self-heal (`map_refresh` in `--json` and the prompt manifest).

10. **Safe plan step.** `pactum execute plan --agent codex`. This validates the
    contract hash, map freshness, and prompt manifest, and prints the command
    that *would* run — without running a real agent. This is the default stop
    point.

11. **Report.** Summarize: current run id, the relevant files you found,
    the contract (goal/scope/acceptance/validation), the plan command, and
    the recommended next action.

## After execution (when the user approved it)

`pactum gate run` runs the contract's validation commands deterministically.
Once a gate report exists, the review stage needs no preparation step:
`review run`, `review finding add`, and `review approve` self-scaffold the
review record (commands acting on existing records — `finding resolve`,
`proposal accept`/`reject`, `fix run`/`apply` — still require those records).

- `pactum review show` / `pactum review status` — read-only inspection (a gated
  run with no review record shows the derived empty pending state).
- `pactum review finding add` / `pactum review finding resolve` — manual
  findings.
- `pactum review run` — autonomous reviewer/fixer rounds (agent-running, needs
  the same explicit approval as execution; `--no-fix --max-rounds 1` is a
  single reviewer panel pass without the write-enabled fixer).
- `pactum review approve --by manual` — the human approval gate.
- `pactum memory propose` / `pactum memory accept --by manual` — capture
  reusable project memory from the reviewed run. Candidates are pinned to the
  review state they were proposed from: if the review changed since, `accept`
  refuses the stale candidate and `error.fix` says to re-propose.

## JSON affordances (`--json`)

Add `--json` to read Pactum's state machine instead of memorizing it:

- Workflow commands that advance a run, plus `pactum status` and
  `pactum task show`, return a top-level `next` array of complete pactum
  command strings (run id filled in) — the legal next moves. An empty `next`
  means there is no safe scriptable move: either the next step needs a human
  decision or the run is at a terminal state; `pactum execute run` is never
  suggested.
- A failed command returns a `pactum.error.v1` envelope:
  `error.code` is a stable machine-readable reason, `error.message` the human
  text, and `error.fix` (when present) the single exact command that remedies
  the failure — never a placeholder. When there is no single fix (for example,
  open blocking clarifications), the envelope's `next` lists safe inspection
  commands such as `pactum clarify show <run_id>`.
- Read-only `show` commands whose artifact does not exist yet return
  `pactum.not_ready.v1` with exit 0 and the remedial command in `fix`.

Read `next` and `error.fix` instead of memorizing stage order — they always
speak the current grammar.

## Current-run usage

`pactum task new` and `pactum task use` set the current run (recorded in
`.heurema/pactum/cache/current-run`). Staged commands (`clarify`, `contract`,
`prompt`, `execute`, `gate`, `review`, `memory`) then operate on it without a
run id. Use `pactum status` (reports the current run) or `pactum task list`
(the current run is marked with `*`) to inspect.

## Stop conditions

- Stop at the plan step and report. Do not run real execution unless the user
  explicitly approves it (see `references/safety.md`).
- If the contract is ambiguous, stop and ask rather than guessing scope.
- If validation would change files or run untrusted commands, confirm first.

## Reporting format

Report back with:
- Run: `<run id>` and status.
- Relevant files: the paths you inspected or will change.
- Contract: goal, in/out of scope, acceptance, validation.
- Plan: the exact command Pactum would run.
- Next action: what you recommend, and whether it needs the user's approval.
