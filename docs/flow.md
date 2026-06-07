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
| Task | `pactum task new "<task>"`, `pactum task list`, `pactum task show`, `pactum task use`, `pactum task current` | `runs/<id>/run.json`, `task.md`, `context/repo-context.md`, `context/search-results.json`, `context/memory-context.md`, `contract/contract.json`, `contract/contract.md`, `contract/approval.json`, `cache/current-run` | `new`/`use`: Yes; `list`/`show`/`current`: No |
| Clarify | `pactum clarify ask`, `pactum clarify answer`, `pactum clarify status` | `clarify/questions.jsonl`, `clarify/answers.jsonl`, `clarify/decisions.jsonl` | `ask`/`answer`: Yes; `status`: No |
| Contract | `pactum contract revise`, `pactum contract approve`, `pactum contract show` | `contract/contract.json`, `contract/contract.md`, `contract/approval.json` | `revise`/`approve`: Yes; `show`: No |
| Prompt | `pactum prompt build`, `pactum prompt show` | `contract/prompt.md`, `contract/prompt-manifest.json`, `context/executor-context.md`, `context/memory-context.md`, `context/memory-selection.json` | `build`: Yes; `show`: No |
| Execute | `pactum execute dry-run`, `pactum execute run`, `pactum execute show/status` | `execute/dry-run.json`, `execute/attempts/attempt_NNN/{request,result,stdout,stderr}`, `execute/last-result.json` | `dry-run`/`run`: Yes; `show`/`status`: No |
| Gate | `pactum gate run`, `pactum gate show` | `gate/gate-report.json`, `gate/validation/command_NNN/{stdout,stderr,result}` | `run`: Yes; `show`: No |
| Review | `pactum review prepare`, `pactum review add-finding`, `pactum review resolve`, `pactum review approve`, `pactum review status/show` | `review/review.json`, `review/findings.jsonl`, `review/resolutions.jsonl` | mutating subcommands: Yes; `status`/`show`: No |
| Reviewer proposals | `pactum review dry-run`, `pactum review run`, `pactum review propose-findings`, `pactum review accept-proposal`, `pactum review reject-proposal` | `review/reviewer-context.md`, `review/reviewer-prompt.md`, `review/reviewer-dry-run.json`, `review/reviewer-attempts/...`, `review/proposals.jsonl`, `review/proposal-decisions.jsonl` | Yes |
| Memory | `pactum memory propose`, `pactum memory show`, `pactum memory accept`, `pactum memory search`, `pactum memory refresh`, `pactum memory stale` | `runs/<id>/memory/{memory-candidate.json,memory-candidate.md,memory-acceptance.json}`, `memory/items.jsonl`, `memory/project-memory.md`, `memory/refreshes.jsonl` | `propose`/`accept`/`refresh`: Yes; `show`/`search`/`stale`: No |

"Mutates state?" means the command writes durable artifacts and/or appends to
the workspace ledger (`ledger/events.jsonl`). Read-only commands (`status`,
`search`, every `show`, `clarify status`, `review status`, `memory search`,
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

`pactum task list` lists every run with its derived lifecycle status, `pactum
task show [run_id|--latest]` shows one run and its next step, `pactum task use
<run_id>` changes the current run, and `pactum task current` prints it. When a
staged command's run id is omitted, Pactum resolves it to the current run, or to
the sole active run if no current run is set; otherwise it asks you to pick one.

### Clarify

`pactum clarify ask <run_id> "Question?"` appends a question; add `--blocking` to
mark it as blocking contract approval. `pactum clarify answer <run_id> q_001
"Answer"` records the answer and decision. `pactum clarify status <run_id>`
summarizes open and answered questions. Open **blocking** questions prevent
contract approval and prompt build.

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
(content and source hashes). Build requires the contract to be approved, the
project map fresh, and no open blocking clarifications. `pactum prompt show`
prints the built prompt.

### Execute

Always inspect `pactum execute dry-run --agent codex` before `pactum execute
run`. `dry-run` writes the exact command Pactum would run
(`execute/dry-run.json`) without launching anything. `pactum execute run --agent
codex` runs the agent as a subprocess and captures the attempt (request, result,
stdout, stderr) under `execute/attempts/`. Because direct execution is
unsandboxed, `execute run` asks for confirmation on an interactive terminal and
**requires `--yes`** for non-interactive/automated use; `dry-run` never needs
`--yes`. Both first re-verify the boundaries recorded at prompt build: the
contract hash still matches the approval, the project map is still fresh and
matches the prompt manifest, and the accepted-memory boundary is unchanged.
`pactum execute status/show` inspect captured attempts. See
[agents.md](agents.md) for the execution model.

### Gate

`pactum gate run <run_id>` deterministically compares the working tree against
the project map hashes to report changed/new/missing files, and summarizes the
latest matching execution attempt. Validation commands from the contract run
**only** when you pass `--allow-commands`; without it, Pactum refuses to run
them and says so. When the approved contract declares `paths_in_scope` and/or
`paths_out_of_scope`, the gate also compares changed and new files against those
globs and records advisory scope warnings for undeclared or explicitly
out-of-scope files. Scope warnings do not make the gate fail; promotion to a
blocking policy is a deliberate follow-up. The gate status is `passed`,
`needs_review`, or `failed`. `pactum gate show` prints the latest report.

### Review

`pactum review prepare <run_id>` creates the manual review (requires a gate
report). `pactum review add-finding` appends a finding with a severity and
category; `--blocking` blocks approval until resolved. `pactum review resolve
<run_id> f_001` records a resolution. `pactum review approve <run_id> --by
manual` approves the review, which requires that the gate did not fail and that
no blocking findings remain. Adding a finding to an approved review resets the
approval.

### Reviewer proposals

Reviewer agents are optional and never trusted automatically. `pactum review
dry-run` prepares the reviewer prompt/context without launching. `pactum review
run` runs a reviewer subprocess and captures its output. `pactum review
propose-findings <run_id>` parses optional fenced-JSON finding blocks from the
reviewer's captured stdout into **pending proposals**. A human then runs
`pactum review accept-proposal <run_id> p_001` (which creates a real finding) or
`pactum review reject-proposal <run_id> p_001 --reason "..."`. Pending proposals
must be decided before memory can be proposed.

### Review loop terminal reasons

`pactum review loop` writes `review/loop-summary.json` with a
`terminal_reason` so operators can tell why the autonomous reviewer/fixer loop
stopped:

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
