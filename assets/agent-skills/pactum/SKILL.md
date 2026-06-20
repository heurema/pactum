---
name: pactum
description: Use Pactum's contract-first workflow for non-trivial repository code changes. Use when a code task needs project-map context, contract approval, a prompt boundary, or a safe plan step. Do not use for quick questions, trivial edits, or when the user asks not to use Pactum.
---

# Pactum — contract-first agent workflow

Pactum turns a repository code task into an auditable, contract-first run:
project map → task → clarification → approved contract → prompt boundary →
execution plan. Drive it entirely with shell commands and `--json` output.

The reference files in `references/` are optional enrichment; this file is
self-sufficient for the safe path. Consult them for deeper detail:
- `references/workflow.md` — full command-by-command sequence and JSON affordances
- `references/install.md` — CLI installation and manual skill copy instructions
- `references/safety.md` — execution safety rules

## Mandatory safety rules (inline, do not skip)

- **Verify the CLI first.** Run `which pactum`. If it is not found, stop; see
  `references/install.md`. Do not continue until `which pactum` succeeds.
- **Do not run `pactum execute run` by default.** It launches a real, unsandboxed
  agent. The default stop point is `pactum execute plan`.
- **Do not run `pactum review run` by default.** It drives reviewer rounds and,
  unless `--no-fix` is set, a **write-enabled fixer**.
- Agent-running commands never prompt — running one IS the approval. Run them
  only after the user has explicitly approved unsandboxed execution.
- Do NOT implement source changes by hand after `pactum execute plan`. Pactum
  stops at the plan step; implementing the plan yourself bypasses the safety
  boundary unless the user explicitly exits Pactum or approves unsandboxed
  execution.
- **`.heurema/pactum/` run records are durable.** Do not delete or revert them.
  Do not mix `.heurema/pactum` churn into feature commits — report them
  separately unless the user asks for an audit-record commit.
- Source files are the source of truth. Pactum's map, wiki, and code-items are
  navigation context — verify against the actual files before relying on them.
- Never hide a non-zero command exit; report failures with their output.

## Machine affordances (use `next` and `error.fix` — never memorize stage order)

Run all Pactum workflow commands with `--json` and read the response:

- **`next` array** — the complete, directly runnable commands legal at this
  stage. Run ONLY commands from `next`. If `next` is empty, STOP and report to
  the user — the next step requires a human decision.
- **`error.fix`** — when a command fails, the exact remedial command. Run it.
  If there is no `error.fix`, read `next` for safe inspection alternatives.
- **`error.code`** — stable machine-readable failure reason
  (`contract_not_approved`, `prompt_not_built`, `project_map_stale`, …).

Decision commands (`clarify answer`, `contract accept`, `contract approve`,
`review proposal accept`/`reject`, `review approve`, `memory accept`) relay an
explicit human decision — pass `--by <principal>` to record who decided.

## Safe workflow (self-contained)

Run each step with `--json`. After each command, read `next` and run only those
commands. If `next` is empty, stop and report. On failure, use `error.fix`.

**1. Verify CLI**
```sh
which pactum
pactum version
```
If `pactum` is not found, stop and follow `references/install.md`.

**2. Check or initialize the workspace**
```sh
pactum status --json
# If not initialized:
pactum init
```

**3. Refresh map if stale**
```sh
pactum map refresh --json
```
Run only if `pactum status` reports the project map stale.

**4. Create the task (sets the current run)**
```sh
pactum task new "<task description>" --json
```
Subsequent commands omit the run id — they use the current run.

**5. Search and read (non-optional — this is Pactum's value)**

Use Pactum's project map for context discovery. Do not substitute raw `rg`/`cat`
unless Pactum directs you to via `next` or `error.fix`.

```sh
# Identifiers and domain terms
pactum search "<identifier or term>" --json
# Exact symbol lookup (no positional query needed)
pactum search --symbol <SymbolName> --json
# Kind filters
pactum search "<term>" --kind wiki --json
pactum search "<term>" --kind code_item --json
```

Read the returned file paths and line ranges. Also read the relevant
`map/wiki/` pages (overview, structure, areas) and the actual source files.

**6. Clarify ambiguities**
```sh
pactum clarify add "<question>" --blocking --json
# Type the answer:
pactum clarify answer q_001 "<answer>" --by manual --json
# Or relay a stored recommendation:
pactum clarify answer q_001 --recommended --by manual --json
# Or answer all open recommended questions at once:
pactum clarify answer --all-recommended --by manual --json
```
Blocking questions must be answered before the prompt can be built.

**7. Write and revise the contract**
```sh
# Get current version token
pactum contract show --json

# Revise with a partial JSON document
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

**8. Approve the contract**
```sh
pactum contract approve --by manual --json
```
Only after the scope is clear and all blocking clarifications are resolved.

**9. Build the prompt boundary**
```sh
pactum prompt build --json
pactum prompt show --json
```
A stale map does not block this step — `prompt build` self-heals and reports it.

**10. Execute plan (the safe stop point)**
```sh
pactum execute plan --agent <agent> --json
```
Replace `<agent>` with the configured executor name (detect whether you are
Codex or Claude, or ask the user which executor is configured). This validates
the contract hash, map freshness, and prompt manifest and prints the command
that *would* run — without running a real agent.

**STOP HERE.** Do not proceed to `pactum execute run` or implement the plan
yourself. Report and wait for explicit user approval.

## Final report format (required)

After `pactum execute plan`, report exactly:

- **Run:** `<run_id>` and contract status (approved / not approved)
- **Files likely touched:** the paths identified during search and map reading
- **Contract:** goal, in/out of scope, acceptance criteria, validation commands
- **Plan command:** the exact command `pactum execute plan` printed
- **Status:** explicitly state "stopped at execute plan — awaiting your approval
  to proceed"

## Executor selection

Do not hardcode `--agent codex`. Detect which executor is configured:
- If running as Codex, use `--agent codex` (or the configured registry name).
- If running as Claude Code, use `--agent claude` (or the configured name).
- If uncertain, ask the user: "Which executor is configured for this workspace?"

Run `pactum doctor` to inspect built-in agent availability without launching
any real agent.
