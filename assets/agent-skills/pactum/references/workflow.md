# Pactum workflow (full reference)

This is optional enrichment for `SKILL.md`. `SKILL.md` is self-sufficient for
the safe path; read these details when you want deeper context on a step.

## When to use

Use Pactum for non-trivial repository code-change tasks: anything where project
context, an explicit contract, and a checked prompt boundary reduce risk. Skip
it for quick questions and trivial edits, or when the user asks you not to.

## Core principle

Source files are the source of truth. Pactum's map, wiki, and memory are
navigation and audit context — best-effort and incomplete by design. Always
confirm against the actual files before relying on them.

## Step-by-step

1. **Verify the CLI.** Run `which pactum`. If missing, stop and follow
   `references/install.md`. Do not continue until it succeeds.

2. **Check workspace state.** Run `pactum status --json`. If the workspace is
   not initialized, run `pactum init` (it creates `.heurema/pactum/` and builds
   the map). Do not re-run `init` on an already-initialized workspace.

3. **Refresh the map if stale.** If `pactum status` reports the project map as
   stale, run `pactum map refresh --json`.

4. **Create the task.** Run `pactum task new "<task description>" --json`. This
   creates a run, sets it as the current run, and assembles first-pass search
   context. Subsequent commands can omit the run id — they use the current run.

5. **Search and read.** Run targeted searches rather than one long sentence:
   - identifiers: `pactum search "formatCurrency" --json`
   - paths: `pactum search "apps/admin/src/lib/format.ts" --json`
   - domain terms: `pactum search "currency" --json`
   - filters: `--kind wiki`, `--kind file`
   Then read `map/wiki/overview.md`, `structure.md`, `entrypoints.md`,
   `commands.md`, the relevant `areas/<area>.md`, and the actual source files.

6. **Clarify if needed.** For anything ambiguous:
   `pactum clarify add "..." --blocking --json`, then record the answer. A
   typed `pactum clarify answer q_001 "..." --by manual` is the explicit human
   answer; when a question carries a stored recommended answer the human agrees
   with, relay it with `pactum clarify answer q_001 --recommended --by manual`,
   or answer every open recommended question at once with
   `pactum clarify answer --all-recommended --by manual`.
   Blocking questions must be answered before the prompt can be built.
   (`pactum clarify run` launches a clarifier agent and is not part of the safe
   default flow; `--no-auto --max-rounds 1` is its single manual pass.)

7. **Write the contract.** `pactum contract revise --from - --json` reads a
   partial JSON document from stdin (`--from <file>` for a file). Include only
   the fields you want to set — absent fields are left untouched. Wrap the
   fields in `{"base_version": "<version>", "contract": {<fields>}}`. Obtain
   `<version>` from `pactum contract show --json` (the `version` field).

   ```sh
   VERSION=$(pactum contract show --json | jq -r '.version')
   pactum contract revise --from - --json <<EOF
   {
     "base_version": "$VERSION",
     "contract": {
       "goal": "one clear sentence",
       "scope": {"in": ["what will change"], "out": ["what will not"]},
       "acceptance_criteria": ["observable criteria"],
       "validation": {"commands": ["make check"]}
     }
   }
   EOF
   ```

   Alternatively: `pactum contract show --json > contract-draft.json`, edit the
   `contract` sub-object, then
   `pactum contract revise --from contract-draft.json --json`.
   Contract quality checklist: goal unambiguous, scope bounded, acceptance
   observable, validation runnable.

8. **Approve.** `pactum contract approve --by manual --json` — only once the
   scope is clear and clarifications are resolved. Approval pins a contract hash.

9. **Build the prompt boundary.** `pactum prompt build --json`, then
   `pactum prompt show --json`. `prompt build` re-derives the run's search
   context from the approved contract, so the executor context reflects the
   final scope. A stale project map does not block it: the build refreshes the
   map itself and records the self-heal (`map_refresh` in `--json` output and
   the prompt manifest).

10. **Safe plan step.** `pactum execute plan --agent <agent> --json`. This
    validates the contract hash, map freshness, and prompt manifest, and prints
    the command that *would* run — without running a real agent. This is the
    default stop point. Use the configured executor name for `<agent>` (codex,
    claude, or whatever is registered for this workspace).

11. **Report.** Summarize: current run id, contract status (approved / not
    approved), the plan command, files likely touched, and an explicit
    "stopped at execute plan — awaiting your approval to proceed" statement.

## After execution (when the user approved it)

`pactum gate run --json` runs the contract's validation commands
deterministically. Once a gate report exists, the review stage needs no
preparation step: `review run`, `review finding add`, and `review approve`
self-scaffold the review record (commands acting on existing records —
`finding resolve`, `proposal accept`/`reject`, `fix run`/`apply` — still
require those records).

- `pactum review show --json` / `pactum review status --json` — read-only
  inspection (a gated run with no review record shows the derived empty pending
  state).
- `pactum review finding add` / `pactum review finding resolve` — manual
  findings.
- `pactum review run` — autonomous reviewer/fixer rounds (agent-running, needs
  the same explicit approval as execution; `--no-fix --max-rounds 1` is a
  single reviewer panel pass without the write-enabled fixer).
- `pactum review approve --by manual --json` — the human approval gate.
- `pactum memory propose --json` / `pactum memory accept --by manual --json` —
  capture reusable project memory from the reviewed run. Candidates are pinned
  to the review state they were proposed from: if the review changed since,
  `accept` refuses the stale candidate and `error.fix` says to re-propose.

## JSON affordances (`--json`)

Add `--json` to all workflow commands and read the state machine:

- Workflow commands that advance a run, plus `pactum status` and
  `pactum task show`, return a top-level `next` array of complete pactum
  command strings (run id filled in) — the legal next moves. An empty `next`
  means there is no safe scriptable move: either the next step needs a human
  decision or the run is at a terminal state; `pactum execute run` is never
  suggested.
- A failed command returns a `pactum.error.v1alpha1` envelope:
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
run id. Use `pactum status --json` (reports the current run) or
`pactum task list --json` (the current run is marked with `*`) to inspect.

## Run-record policy for `.heurema/pactum/`

The `.heurema/pactum/` workspace is durable, version-controlled run-record
state. Treat it as audit data:
- **Do not delete or revert** `.heurema/pactum/` content.
- **Do not mix** `.heurema/pactum/` churn into feature commits — it belongs in
  a separate periodic `audit: record runs` commit.
- **Report** the `.heurema/pactum/` changes separately in your summary unless
  the user explicitly asks for an audit-record commit.

## Stop conditions

- Stop at the plan step and report. Do not run real execution unless the user
  explicitly approves it (see `references/safety.md`).
- After `pactum execute plan`, do NOT implement the plan yourself — that
  bypasses the Pactum safety boundary.
- If the contract is ambiguous, stop and ask rather than guessing scope.
- If validation would change files or run untrusted commands, confirm first.

## Reporting format

Report back with:
- Run: `<run id>` and contract status (approved / not approved).
- Files likely touched: the paths you inspected or identified during search.
- Contract: goal, in/out of scope, acceptance, validation commands.
- Plan: the exact command `pactum execute plan` printed.
- Status: "stopped at execute plan — awaiting your approval to proceed."
