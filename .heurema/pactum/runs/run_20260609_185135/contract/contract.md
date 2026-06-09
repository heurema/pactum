# Contract Draft

## Goal
Add a command that deletes run records older than N days

## Current status
Contract status: draft
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260609_184335
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (0 result(s))

## Clarifications
- q_001 [blocking] — By "run records", which tree do you mean: the durable contract-first run directories under `.heurema/pactum/runs/` (the `RunsDir` entries named `run_YYYYMMDD_HHMMSS`, each holding contract/clarify/gate/review/memory/ledger), or the regenerable map runs under `.heurema/pactum/map/runs/` (`MapRunsDir`)? These are the only two 'runs' trees in this repo and deleting the wrong one is destructive.
  Rationale: artifacts.Paths defines both RunsDir (.heurema/pactum/runs) and MapRunsDir (.heurema/pactum/map/runs). The backlog item 'Committed .heurema run-record growth' mitigation (B) ('prune old runs/ dirs while keeping the ledger + memory'), the run-record-batching memory note, and the new run_* dirs in git status all point to RunsDir; map runs are gitignored/regenerable. This is foundational because every other answer (age basis, preservation, safety) is defined relative to the chosen target.
  Answer: pending
- q_002 — Where should the command live and what is its surface? Candidates: a subcommand of the existing run-management group, `pactum task prune --older-than <days>`, vs a new top-level group such as `pactum runs prune` or `pactum prune`. Which flag carries N?
  Rationale: internal/app/cli.go registers top-level groups via Kong struct tags; taskCmd (task.go) already owns the run lifecycle (new/list/show/use/current) and is the natural home. There is no existing runs/prune group. Commands conventionally expose --json and destructive ones --yes.
  Answer: pending
- q_003 — What timestamp defines a run's age? Candidates: the `created_at` field in each run's `run.json` (RFC3339), the timestamp embedded in the run id (`run_YYYYMMDD_HHMMSS`), or filesystem mtime. Note run dirs are committed to git, which does not preserve mtimes, so mtime would make every record look freshly created after a clone.
  Rationale: contractRunState.CreatedAt in run.json is the authoritative creation time, and reserveContractRunDir builds the id from createdAt.Format("20060102_150405"). The M11.9 policy commits run dirs to git, so mtime is unreliable after a clone/checkout.
  Answer: pending
- q_004 — When a run is pruned, should anything outside its `runs/<id>/` directory be deleted? Specifically, should the append-only workspace ledger (`.heurema/pactum/ledger/events.jsonl`), accepted project memory (`.heurema/pactum/memory/`), and the project map be left fully intact?
  Rationale: Backlog mitigation (B) is explicit: 'prune old runs/ dirs while keeping the ledger + memory as the live queryable summary (history retains the detail)'. The per-run ledger (runDir/ledger/usage.jsonl) lives inside the run dir and is removed with it; the workspace ledger and memory dir are separate top-level paths in artifacts.Paths.
  Answer: pending
- q_005 — What about a run older than N days that is still active (status not in completed/cancelled/failed per isTerminalRunStatus) or is the current-run pointer target — should an age-only filter delete an in-progress run?
  Rationale: deriveRunStatus/isTerminalRunStatus distinguish active vs terminal runs; the current-run pointer lives in the gitignored cache and is already treated as stale-safe (TaskCurrent reports 'no current run' when its target is gone). Deleting an in-progress run purely by age could discard live work.
  Answer: pending
- q_006 — How should invalid or boundary `--older-than` values behave: 0, negative, or non-integer? `--older-than 0` would match every run (all ages > 0) and wipe all history.
  Rationale: The goal phrase 'older than N days' is silent on bounds, and the operation is destructive, so 0/negative is a likely footgun.
  Answer: pending
- q_007 — Since deletion is destructive, should the command require a `--yes` confirmation and/or offer a dry-run preview listing the runs it would prune, and what should it do when no run is older than N?
  Rationale: Repo convention gates destructive/agent commands on --yes (execute run, review run, review fix, clarify suggest, contract draft) and gate run on --allow-commands. The bare contract is silent on confirmation and on the empty-match case.
  Answer: pending

## In scope
TBD

## Out of scope
TBD

## Acceptance criteria
TBD

## Validation commands
TBD

## Assumptions
TBD

## Open questions
- By "run records", which tree do you mean: the durable contract-first run directories under `.heurema/pactum/runs/` (the `RunsDir` entries named `run_YYYYMMDD_HHMMSS`, each holding contract/clarify/gate/review/memory/ledger), or the regenerable map runs under `.heurema/pactum/map/runs/` (`MapRunsDir`)? These are the only two 'runs' trees in this repo and deleting the wrong one is destructive.
- Where should the command live and what is its surface? Candidates: a subcommand of the existing run-management group, `pactum task prune --older-than <days>`, vs a new top-level group such as `pactum runs prune` or `pactum prune`. Which flag carries N?
- What timestamp defines a run's age? Candidates: the `created_at` field in each run's `run.json` (RFC3339), the timestamp embedded in the run id (`run_YYYYMMDD_HHMMSS`), or filesystem mtime. Note run dirs are committed to git, which does not preserve mtimes, so mtime would make every record look freshly created after a clone.
- When a run is pruned, should anything outside its `runs/<id>/` directory be deleted? Specifically, should the append-only workspace ledger (`.heurema/pactum/ledger/events.jsonl`), accepted project memory (`.heurema/pactum/memory/`), and the project map be left fully intact?
- What about a run older than N days that is still active (status not in completed/cancelled/failed per isTerminalRunStatus) or is the current-run pointer target — should an age-only filter delete an in-progress run?
- How should invalid or boundary `--older-than` values behave: 0, negative, or non-integer? `--older-than 0` would match every run (all ages > 0) and wipe all history.
- Since deletion is destructive, should the command require a `--yes` confirmation and/or offer a dry-run preview listing the runs it would prune, and what should it do when no run is older than N?
