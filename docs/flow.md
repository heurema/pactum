# Workflow

Pactum drives a task through a fixed sequence of stages. Each stage reads and
writes durable artifacts under the run directory
(`.heurema/pactum/runs/<run_id>/`) or under the shared workspace, and each stage
enforces boundaries before the next one is allowed to proceed. Nothing is
implicit: the contract, the prompt boundary, the execution attempt, the gate
report, the review, and the accepted memory are all files you can read.

## Stages at a glance

| Stage | Command | Main artifacts | Mutates state? |
| --- | --- | --- | --- |
| Init / map | `pactum init`, `pactum map refresh` | `manifest.json`, `config.yaml`, `map/` (`wiki/` pages, `repo-map.md`, `llms.txt`, `files.jsonl`, `code-items.jsonl`, `hashes.jsonl`, `search.sqlite`) | Yes |
| Status / search | `pactum status`, `pactum search "<query>"` | — (reads map + workspace) | No |
| Task | `pactum task new "<task>"`, `pactum task list`, `pactum task show`, `pactum task use` | `runs/<id>/run.json`, `task.md`, `context/repo-context.md`, `context/search-results.json`, `context/memory-context.md`, `contract/contract.json`, `contract/contract.md`, `contract/approval.json`, `cache/current-run` | `new`/`use`: Yes; `list`/`show`: No |
| Clarify | `pactum clarify add`, `pactum clarify answer`, `pactum clarify run`, `pactum clarify show` | `clarify/questions.jsonl`, `clarify/answers.jsonl`, `clarify/decisions.jsonl`, `clarify/loop-summary.json` | `add`/`answer`/`run`: Yes; `show`: No |
| Contract | `pactum contract revise`, `pactum contract approve`, `pactum contract show` | `contract/contract.json`, `contract/contract.md`, `contract/approval.json` | `revise`/`approve`: Yes; `show`: No |
| Prompt | `pactum prompt build`, `pactum prompt show` | `contract/prompt.md`, `contract/prompt-manifest.json`, `context/executor-context.md`, `context/memory-context.md`, `context/memory-selection.json` | `build`: Yes; `show`: No |
| Execute | `pactum execute plan`, `pactum execute run`, `pactum execute show` | `execute/dry-run.json`, `execute/attempts/attempt_NNN/{request,result,stdout,stderr}`, `execute/last-result.json` | `plan`/`run`: Yes; `show`: No |
| Gate | `pactum gate run`, `pactum gate show` | `gate/gate-report.json`, `gate/validation/command_NNN/{stdout,stderr,result}` | `run`: Yes; `show`: No |
| Review | `pactum review finding add`, `pactum review finding resolve`, `pactum review approve`, `pactum review status/show` | `review/review.json`, `review/findings.jsonl`, `review/resolutions.jsonl` | mutating subcommands: Yes; `status`/`show`: No |
| Reviewer rounds | `pactum review plan`, `pactum review run`, `pactum review proposal collect`, `pactum review proposal accept`, `pactum review proposal reject` | `review/reviewer-context.md`, `review/reviewer-prompt-<name>-<lens>.md`, `review/reviewer-dry-run.json`, `review/reviewer-attempts/...`, `review/proposals.jsonl`, `review/proposal-decisions.jsonl`, `review/loop-summary.json` | Yes |
| Memory | `pactum memory propose`, `pactum memory show`, `pactum memory accept`, `pactum memory search`, `pactum memory refresh`, `pactum memory stale` | `runs/<id>/memory/{memory-candidate.json,memory-candidate.md,memory-acceptance.json}`, `memory/items.jsonl`, `memory/project-memory.md`, `memory/refreshes.jsonl` | `propose`/`accept`/`refresh`: Yes; `show`/`search`/`stale`: No |
| Export | `pactum export [run_id] --output <path>` | the ZIP archive at `--output` (outside the exported run directory) | No |

"Mutates state?" means the command writes durable artifacts and/or appends to
the workspace ledger (`ledger/events.jsonl`). Read-only commands (`status`,
`search`, every `show`, `clarify show`, `review status`, `memory search`,
`memory stale`) never append ledger events.

## Stage details

### Init and the project map

`pactum init` creates the `.heurema/pactum/` workspace, writes the default
`config.yaml`, and builds the project map: a deterministic scan of the
repository. `init` also writes a workspace `.gitignore` that version-controls the
durable run record — contracts, decisions, the ledger, gate verdicts, review
findings, and memory — while ignoring regenerable artifacts (`map/`, per-run
`context/`) and raw `*.log` transcripts: the run outcome lives in git history and
the learnings in memory, so the bulky agent transcripts are not committed. The map
is **wiki-first**. It produces a generated map wiki under
`map/wiki/` (`overview.md`, `structure.md`, `commands.md`, `entrypoints.md`,
`config.md`, `tests.md`, and one `areas/<area>.md` page per top-level
directory), a file inventory, a human-readable `repo-map.md`, an `llms.txt`
router, and a SQLite full-text search index. The wiki is generated from
deterministic facts (file inventory and manifests) and uses conservative,
evidence-backed language (candidate entrypoint, detected config, likely role).
Tree-sitter `code-items.jsonl` is kept only as best-effort symbol hints — it is
incomplete by design, unsupported languages/framework files may have no code
items, and source files remain the source of truth. In a git repository the
scan enumerates files via git, so the repo's `.gitignore` is honored (build
artifacts like `__pycache__/` or `dist/` are not indexed); `.heurema` is always
excluded, and non-git directories fall back to a filesystem walk that skips
`.git`, `.heurema`, and common vendor/build directories.

`pactum map refresh` rebuilds those artifacts, including the wiki. `pactum
status` reports whether the map is fresh or stale (for example, when tracked
files changed since the last scan) and points you at `pactum map refresh` when a
refresh is needed.

`pactum search "<query>"` queries the lexical index. The index covers the repo
map, the `llms.txt` router, the map wiki pages, files, code items, and imports.
Filter with `--kind` (`repo_map`, `llms`, `wiki`, `file`, `code_item`,
`import`); `--kind code_item` excludes import-like entries, which are searchable
under `--kind import`. Ranking is FTS5 bm25 with a small, deterministic polish:
in unfiltered (`any`) searches import-like entries get a penalty and the
entrypoints/commands/config wiki pages get a modest boost, and an exact
title/path-basename match gets a small boost. It is a light reordering of
near-equal matches, not a relevance model.

### Task

`pactum task new "<task>"` creates a run directory and a **draft contract**. The
draft records the goal and empty scope/acceptance/validation sections for you to
fill in, alongside deterministic context: a repository context excerpt, the
lexical search results for the task, and the accepted memory selected for the
task. The new run is recorded as the **current run** (a local-only pointer at
`cache/current-run`), so the staged commands below can omit the run id.

`pactum task new "<task>" --clarify` additionally runs autonomous
clarifier rounds (see [agents.md](agents.md)) against the new run as soon as it
is created: suggest → auto-resolve high-confidence recommendations → re-suggest,
until converged, `needs_human`, or the round cap. The command then prints the
created run, the loop summary, and a "questions awaiting you" section with the
open blocking questions automation could not resolve — each with its kind,
confidence, and recommended answer — so you answer only those and proceed to
`pactum contract approve`. `--reviewer`, `--max-rounds`, and the idle
`--timeout` pass through to the loop. A loop failure never rolls back the run: it
stays created (and current), and you can re-run `pactum clarify run` on it.
Without `--clarify`, `task new` only creates the run and the draft contract as
described above.

`pactum task list` lists every run with its derived lifecycle status, `pactum
task show [run_id|--latest]` shows one run and its next step, and `pactum task
use <run_id>` changes the current run. `pactum status` reports the current run,
and `pactum task list` marks it with `*`. When a
staged command's run id is omitted, Pactum resolves it to the current run, or to
the sole active run if no current run is set; otherwise it asks you to pick one.

### Clarify

`pactum clarify add <run_id> "Question?"` appends a question; add `--blocking` to
mark it as blocking contract approval. `pactum clarify answer <run_id> q_001
"Answer"` records the answer and decision. `pactum clarify show <run_id>`
summarizes open and answered questions. Open **blocking** questions prevent
contract approval and prompt build.

`pactum clarify run <run_id>` runs autonomous clarifier rounds: a read-only
clarifier proposes questions, high-confidence recommendations are auto-resolved,
and the loop repeats until converged, `needs_human`, or the round cap
(`--max-rounds`). `--no-auto` disables auto-resolution entirely — created
questions stay open for the human, so a single manual clarifier pass is
`pactum clarify run --no-auto --max-rounds 1`. Every run writes
`clarify/loop-summary.json`.

A clarifier question stores a recommended answer. `pactum clarify answer
<run_id> q_001 --recommended` records that stored recommendation as the answer —
it errors when the question is already answered, has no recommendation, or
depends on unanswered questions. `pactum clarify answer <run_id>
--all-recommended` answers every open question that carries a recommendation in
dependency order and reports the skipped ones (no recommendation, or
dependencies unanswered). Both record provenance distinguishable from a typed
answer (`manual_recommended` / `manual_all_recommended`).

Every decision verb — `clarify answer`, `contract accept`, `contract approve`,
`review proposal accept`/`reject`, `review approve`, `memory accept` — takes an
optional `--by <principal>` (default `manual`) naming whose decision is being
relayed; it is recorded in the decision artifact (`decided_by`/`accepted_by`/
`approved_by`). Automatic loop decisions never carry a principal — their
`source` field is the provenance.

### Contract

`pactum contract revise <run_id>` appends to the deterministic contract fields
(`--goal`, `--add-in-scope`, `--add-out-of-scope`, `--add-acceptance`,
`--add-path-in-scope`, `--add-path-out-of-scope`, `--add-validation`,
`--add-assumption`). Path scope entries are repo-relative slash globs; `*`
matches within one path segment, while `**` matches any number of path segments.
`pactum contract approve <run_id> --by manual` approves the contract and pins the
approval to the contract's SHA-256 hash. Revising an already-approved contract
resets the approval, because the recorded hash no longer matches.

### Prompt

`pactum prompt build <run_id>` builds the executor **prompt boundary**: a
deterministic `prompt.md` plus a `prompt-manifest.json` that records the
approved contract hash, the project map run, and the accepted-memory boundary
(content and source hashes). Build requires the contract to be approved and no
open blocking clarifications. A stale project map is not a failure: `prompt
build` refreshes it itself, names the new map run id in its output, and records
the self-heal as a `map_refresh` object (`{"triggered": false}` when no refresh
was needed) in both the `--json` response and the prompt manifest. `pactum
prompt show` prints the built prompt.

### Execute

Always inspect `pactum execute plan` before `pactum execute run`. `plan`
writes the exact command Pactum would run (`execute/dry-run.json`) without
launching anything. `pactum execute run` runs the agent as a subprocess and
captures the attempt (request, result, stdout, stderr) under
`execute/attempts/`. An omitted `--agent` runs the first entry of the config
agents registry; `--agent <name>` picks another registered entry. Direct
execution is unsandboxed and `execute run` never prompts — running it is
itself the recorded decision.
Both first re-verify the boundaries recorded at prompt build: the
contract hash still matches the approval, the project map is still fresh and
matches the prompt manifest, and the accepted-memory boundary is unchanged.
`pactum execute show` inspects captured attempts: with no attempt id it shows
the run-level execution summary, and `pactum execute show [run_id]
<attempt_id>` shows one attempt in detail. See [agents.md](agents.md) for the
execution model.

### Gate

`pactum gate run <run_id>` deterministically compares the working tree against
the project map hashes to report changed/new/missing files, summarizes the
latest matching execution attempt, and runs the validation commands from the
contract. When the approved contract declares `paths_in_scope` and/or
`paths_out_of_scope`, the gate also compares changed and new files against those
globs. The project config controls enforcement:

- `gate.scope_enforcement: block` (the default; missing or empty also means
  `block`) records `scope.status: blocked` for undeclared or explicitly
  out-of-scope files and makes the overall gate status `failed`.
- `gate.scope_enforcement: warn` preserves the M11.5 advisory behavior:
  violations are recorded as `scope.status: warnings` and do not directly fail
  the gate.

When no path globs are declared, the scope section is omitted and scope has no
effect on the gate. The gate status is `passed`, `needs_review`, or `failed`.
`pactum gate show` prints the latest report.

### Review

There is no separate preparation step: once a gate report exists, the mutating
review commands (`review run`, `review finding add`, `review approve`)
self-scaffold `review/review.json` and the findings/resolutions records, and
`review status`/`review show` derive the empty pending state read-only. `pactum
review finding add` appends a finding with a severity and category; `--blocking`
blocks approval until resolved. `pactum review finding resolve <run_id> f_001`
records a resolution. `pactum review approve <run_id> --by manual` approves the
review, which requires that the gate did not fail and that no blocking findings
remain. Adding a finding to an approved review resets the approval.

### Reviewer rounds

Reviewer agents are optional and never trusted to approve anything. `pactum
review plan` prepares the reviewer prompt/context without launching. `pactum
review run <run_id>` runs autonomous reviewer/fixer rounds: each round fans the
reviewer panel out across the fixed lenses, parses structured finding proposals
from the captured output, accepts valid ones into review findings, and — while
open **blocking** findings remain — runs the write-enabled fixer, applies its
outcomes, and re-runs the gate. `--reviewer` overrides the panel, `--agent`
picks the fixer, and `--max-rounds`, `--patience`, and `--clean-rounds` bound
the loop. `--no-fix` never invokes the fixer and stops after the first round
that leaves open blocking findings (`findings_open`), so a single manual panel
pass is `pactum review run --no-fix --max-rounds 1`. Every run writes
`review/loop-summary.json`.

The surgical commands remain for working outside the rounds: `pactum review
proposal collect <run_id>` parses optional fenced-JSON finding blocks from the
captured stdout of every completed reviewer attempt (all lenses; `--attempt`
narrows to one) into **pending proposals**. A human then runs
`pactum review proposal accept <run_id> p_001` (which creates a real finding) or
`pactum review proposal reject <run_id> p_001 --reason "..."`. Pending proposals
must be decided before memory can be proposed.

The fixer reports a structured outcome per finding: `pactum review fix apply
<run_id>` parses a `pactum.review_fix_outcomes.v1` fenced-JSON
block from the fixer's captured stdout (best-effort — a missing or malformed
block warns, never errors) and **resolves** findings accordingly: `fixed` and
`rebutted` findings become resolved, `blocked` findings stay open. In the
autonomous loop this runs automatically after each fix round, so `open_findings`
shrinks as work completes; a finding the fixer resolved as `rebutted` (a false
positive) is suppressed if a later round re-proposes the same `(file, line,
message)`.

### Review run terminal reasons

`pactum review run` writes `review/loop-summary.json` with a
`terminal_reason` so operators can tell why the autonomous reviewer/fixer rounds
stopped.

Convergence is gated on **blocking** findings. Each round records
`open_blocking_findings` (open findings with `blocking: true`, the same
`blocking_open` count that gates review approval) alongside `open_findings`. The
fixer runs only while open blocking findings remain, and is scoped to them:
non-blocking findings are still accepted and recorded as advisory, but they never
drive the fixer or keep the loop running. This is what stops low/subjective
finding churn from running the loop to `max_rounds` without converging.

- `resolved` — the primary success terminal: after a round accepted its
  proposals (and, when a fixer ran, applied its fix outcomes), no open blocking
  findings remain (`open_blocking_findings == 0`). Advisory (non-blocking)
  findings may still be open and recorded. A round whose accepted proposals are
  all non-blocking converges `resolved` without invoking the fixer.
- `findings_open` — `--no-fix` was set and a round left open blocking findings.
  Nothing can change the tree without the fixer, so the run stops instead of
  churning reviewer-only rounds; the findings await the human in
  `pactum review show`.
- `clean_round` — the configured number of consecutive reviewer rounds reported
  no findings or warnings.
- `stalemate` — fixer rounds repeatedly left the working-tree fingerprint
  unchanged for the configured patience.
- `max_rounds` — the configured round cap was reached.
- `reviewer_findings_unparsed` — the reviewer emitted finding-like output that
  Pactum could not turn into accepted proposals.
- `gate_failed` — a fixer round completed, the gate ran, and the gate report
  status was `failed`. The loop stops cleanly and records the gate report
  artifact for human escalation.
- `budget_exceeded` — `budget.max_tokens` is set, `budget.mode` is `block`, and
  the run's cumulative captured token total reached the configured ceiling
  before the next round began. The loop stops cleanly; the summary records the
  budget mode, `max_tokens`, and captured token total. With `budget.mode: warn`,
  the same condition is recorded as a budget warning and the loop continues.
- `error` — Pactum could not run or record part of the loop, such as a missing
  or unreadable execution artifact. This remains an infrastructure/tooling
  failure and returns a command error.

### Memory

`pactum memory propose <run_id>` builds a deterministic memory candidate from a
reviewed run (requires the contract approved, a gate report, an approved review,
and no pending reviewer proposals). `pactum memory show <run_id>` previews it.
`pactum memory accept <run_id> --by manual` appends an accepted item to project
memory. `pactum memory search`, `pactum memory refresh`, and `pactum memory
stale` are covered in [memory.md](memory.md).

### Export

`pactum export [run_id] --output <path> [--json]` packs a run's full record
into a single deterministic ZIP archive. An omitted `run_id` resolves like the
other read-only run commands (current run, else the sole active run). The
archive contains every regular file under `runs/<run_id>/` — including `.log`
transcripts when present — rooted at `pactum-run-<run_id>/`, plus a generated
`ledger/events.filtered.jsonl` sidecar holding only the workspace ledger
events for that run. Entries are sorted, slash-separated, and normalized
(fixed timestamps, `0644`/`0755` modes), so repeated exports of an unchanged
run are byte-for-byte identical.

Export is read-only Pactum state: the only write is the archive itself,
created via a temporary sibling file and an atomic rename. The command fails —
removing any partial archive — if the output path already exists, its parent
directory is missing, the path points inside the exported run directory, a
selected file cannot be read, the workspace ledger is missing or malformed, or
the run record contains symlinks or other non-regular entries.
