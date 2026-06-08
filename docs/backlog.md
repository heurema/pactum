# Backlog

Deferred work and known follow-ups, consolidated from dogfood reports and PR
reviews across M8–M10. Rough priority in parentheses.

## Execution / review UX

- **Timeout follow-ups** (med). `--timeout` is now idle-based (M11.6). Still open:
  (a) **completion-aware finalize** — when the idle timeout fires but the agent
  already produced a usable diff, report it as completed-with-warning instead of
  `timed_out: true` / `exit -1` (a killed-but-complete run currently looks failed);
  (b) an optional **absolute total cap** (off by default) as a CI backstop for an
  agent that keeps emitting output yet never finishes (the idle timer never fires);
  (c) **per-project config defaults** for the idle window (and any cap) so a project
  sets it once instead of passing `--timeout` on every run.
- **Workspace config leaks across runs/branches** (med). `executor_model` (and other
  `agents.*` config) lives in the gitignored `.heurema/pactum/config.yaml`, which
  persists across branches. An M10.2 `executor_model: claude-opus-4-8:xhigh` leaked
  into M10.3 and broke a codex run (pactum passes model flags blind; codex rejected a
  claude model). Consider warning on agent/model mismatch and/or scoping model pins.

## Review→fix loop (L3b and beyond)

- **Review-loop convergence** (high). **Slice 1 (resolve-on-fix) shipped in M12.8** —
  root cause (a) below is addressed; (b) severity-gate and (c) the anti-meta-churn reviewer
  prompt remain. The first real dogfood of the autonomous `review
  loop` (cross-model panel on the M12.6 store-port) did NOT converge — it hit `max_rounds`
  with open findings accumulating monotonically (4→7→10→12, never a clean round). Root
  causes: (a) **findings are never resolved** — a fixed finding stays "open", so
  `open_findings` only grows and a 0-new-proposals clean round is the only path to
  convergence; a successful fix should mark its finding resolved (or a re-review should).
  (b) **No severity gate** — most findings were `low` design-observations; low/subjective
  findings drive fixer churn. Converge on blocking/critical+major only (the L2 pass) and
  defer low. (c) **Meta-churn** — by rounds 3-4 the panel began critiquing the FIXER'S OWN
  changes (a method it added, "the working tree changed", "validation wasn't re-run"),
  which never clears; the reviewer prompt should judge the contract's diff, not process
  meta-commentary, and avoid re-reviewing already-accepted unchanged regions. (d) The
  cross-model panel itself worked (disjoint real findings — it caught real mis-routing the
  manual pass missed — 0 bad merges) but doubled finding volume and amplified churn. Net:
  the loop is strong at FINDING but cannot yet CONVERGE on a refactor; today its output is
  a draft to curate, not an auto-merge.
- **Strengthen the executor + reviewer prompts with house style + best practices** (med).
  The built-in prompt templates carry only generic guidance — the executor prompt
  (`renderApprovedPromptMD`) says "follow the contract / no out-of-scope / search before
  creating", and the reviewer prompt (`renderReviewerPrompt`) gives the findings schema +
  "do not apply patches". Neither encodes the project's minimal style guide or engineering
  best practices. Bake them into both: for the executor — match surrounding idiom, naming,
  and comment density; prefer existing helpers; small focused diffs; conventional error
  handling; no dead code; behavior-preserving where asked. For the reviewer — also flag
  style / best-practice / readability violations against the same ruleset, not only
  correctness. Decide sourcing: a short minimal ruleset baked into the templates vs.
  referencing `AGENTS.md` / `CLAUDE.md` (already read by the agents). Applies to BOTH the
  write and review stages. (Relates to the dropped custom review-guidelines knob — this is
  the built-in-prompt version.)
- **Loop stop conditions.** Stalemate-by-fingerprint and K-consecutive-clean —
  **done (M10.3)**. Token-native `max_tokens` budget stop is **done** with a
  `budget_exceeded` terminal. Remaining cost-layer follow-ups live in
  [`cost-budget-design.md`](cost-budget-design.md).
- **Cost/budget remaining slices** (see [`cost-budget-design.md`](cost-budget-design.md)).
  Slices 1-2 (write- and read-stage token accounting) done (M12.0, M12.1);
  Slice 4 (`max_tokens` budget stop) done. Remaining: cost ($) overlay and
  estimation. Also: harden the claude usage parser to tolerate incidental
  leading/trailing stdout (today any non-JSON stdout degrades claude capture to
  `captured=false`; safe but brittle).
- **Cross-run / workspace usage stats command** (med). Per-task stats exist
  (`pactum usage <run_id>`). Add a workspace-wide aggregate — `pactum usage` with no
  run id (or `--all`) — that scans all runs and reports total tokens, by-run / by-stage
  / by-agent / by-model breakdowns, cache-hit ratios, and trend over time. This is the
  cross-run rollup the design doc reserves and that no agentic CLI currently offers
  (they're all per-task only). Reads the per-run `usage.jsonl` ledgers; derived, never
  the source of truth.
- **Rebuttal channel** (med). Feed the fixer's rebuttal of a false positive back to
  the reviewer on the next round so it re-judges instead of re-reporting.
- **Semantic review-finding dedup** (low). The autonomous review loop now dedups
  proposals against currently open findings only by the exact stored
  `(file, line, message)` tuple. Reworded messages, line-number drift, and other
  semantic duplicates are still treated as distinct findings; add fuzzy/semantic
  reconciliation only after the exact-match behavior has settled.
- **JSON sub-command contract** (low). The loop json-parses sub-command stdout; a
  mid-loop not-ready condition could inject human text into the parsed buffer. Make
  the contract explicit.

## Loop phases not yet built

- **L2 — severity by composition** (med). A broad fix pass, then a critical/major-
  only final gate (no per-finding severity schema).
- **Phase 1 — clarify loop** (high, large). Slices done: `clarify suggest` (agent
  proposes questions, M11.0) and `contract draft` (agent drafts contract fields from
  answers → proposal → `accept-draft`, M11.1). Remaining: the **loop driver**
  (suggest → answer → draft → repeat to a precise contract, with caps). Open edges:
  re-proposed-question dedup; the clarifier/drafter fuse run+parse so a non-zero exit
  discards valid output (decouple like `review propose-findings`); an all-empty
  `contract draft` records a pending proposal that `accept-draft` then rejects
  (dead-end — report "no additions" instead); re-running `contract draft` after accept
  clobbers the accepted proposal's audit fields; `accept-draft` hardcodes
  `accepted_by:"manual"` (no `--by`).
- **Sharper clarify questioning** (med). The `clarify suggest` prompt currently
  produces soft, surface-level questions. Strengthen it to interrogate the
  requester adversarially — probe hidden assumptions, force concrete acceptance
  criteria, surface edge cases, and push back on vague answers — so the resulting
  contract is precise rather than agreeable. A "grill the requester" questioning
  style, not a polite checklist.

## Hardening / cleanup

- **Clarify commands reset approval silently** (med). `clarify ask` / `answer` /
  `suggest` add open questions via `refreshClarificationArtifacts`, which calls
  `resetApprovalIfApproved` — so running them on an already-approved/executed run
  silently regresses it to `clarifying`. Guard or warn when the run is already
  approved (pre-existing; `clarify suggest` makes bulk creation easier).

## Resolved (for reference)

- Review-loop convergence slice 1 — resolve-on-fix (M12.8) — the fixer now emits a
  structured `pactum.review_fix_outcomes.v1` fenced-JSON block (one outcome per finding:
  `fixed` / `rebutted` / `blocked`), parsed decoupled and best-effort (mirroring `review
  propose-findings`) by the new `review apply-fix-outcomes [run_id] [fixer_attempt_id]`.
  `fixed` and `rebutted` findings become resolved (with a new `outcome` field on the
  resolution record); `blocked` stays open. The autonomous loop applies outcomes after
  each fix round, so `open_findings` shrinks as work completes and the existing terminals
  (`clean_round` / `stalemate`) finally fire — a fully-fixed round lets the next reviewer
  round converge instead of churning to `max_rounds`. A finding resolved as `rebutted` (a
  false positive) is suppressed if a later round re-proposes the same `(file, line,
  message)`; `fixed`/manual resolutions stay re-acceptable (a re-raise may mean the fix
  did not hold). Best-effort: a missing/malformed block warns and never errors, so the
  loop still tolerates a fixer that emits no outcomes. Slices 2 (severity-gated
  convergence) and 3 (anti-meta-churn reviewer prompt) remain.
- Storage port (M12.6) — new leaf package `internal/store` (`Store` interface + an `FS`
  filesystem implementation byte-for-byte equivalent to the prior `os.*` calls). A
  package-level `activeStore` in `internal/app` routes the workspace durable-record I/O
  through the port: the JSON/JSONL/YAML primitives, the workspace `os.*` calls, the
  prompt-manifest removal (`store.Remove`), the run-dir reservation (`store.Mkdir`, kept
  non-recursive/fail-if-exists for the atomic claim), and all `ledger.Append` sites
  (`ledger.Append` gained a `store.Store` parameter). A map-backed in-memory `Store`
  swapped in a test proves the backend is swappable. Direct `os.*` is intentionally kept
  for non-record I/O: workspace discovery (`findUp`, `os.Getwd`, `filepath.Abs`), repo-tree
  reads, temp files, the project-map files (`search.sqlite`, `map/`), and transcript `*.log`
  (agent + gate-validation stdout/stderr, which stream live). Surfaced by review: the first
  manual adversarial pass + the dogfooded `gate` → cross-model `review loop` together caught
  inconsistent routing (run-local memory hashes → `storeFileSHA256`, attempt-dir checks →
  `storeDirExists`, prompt-manifest delete → `store.Remove`, map-file existence → direct
  `filesystemRegularFile`, run reservation → store) — all fixed. Opens the door (per the
  storage research) to a future regenerable, gitignored SQLite index for cross-run queries;
  no binary DB in git, no API server (YAGNI). Not ported: `internal/agents` transcripts and
  `internal/projectmap` (regenerable/transcript, out of scope).
- Cross-model review panel (M12.4) — `agents.review_panel` lists two or more reviewer
  agents; each autonomous review-loop round runs them CONCURRENTLY against the same
  diff/contract (goroutine fan-out, sequential per-attempt proposal parsing in panel
  order), then the existing `(file, line, message)` fingerprint dedup merges cross-
  reviewer duplicates (first accepted, rest recorded as `duplicate` decisions, so
  corroboration stays in the audit). Severity is reconciled to the max: a duplicate
  proposal that outranks the open finding upgrades it (`low<medium<high<critical`) and
  emits a `review_finding_severity_upgraded` event. An explicit `--reviewer` disables
  the panel; empty/absent `review_panel` is byte-for-byte the single-reviewer path.
  Concurrency is race-clean: a package-level mutex guards only the shared lifecycle
  sections (attempt-id allocation + mkdir, the events/usage ledger appends, the shared
  last-result write) so the agent subprocess still runs lock-free; concurrent reviewers
  share a synchronized live-output writer. Validated under `go test -race`. The N×
  per-round token cost is already bounded by the M12.2 budget stop. Deferred: an
  agreement-count field and severity-threshold gating (require K reviewers); per-panel-
  member model pins (all members share `agents.reviewer_model` today).
- Token accounting — slices 1-2 (M12.0, M12.1) — executor/fixer agents and read-stage
  reviewer/clarifier/drafter agents now run with structured output (`codex exec --json`
  / `claude -p --output-format json`, with read-stage Codex kept read-only); the
  runner parses token usage per agent (best-effort, never fatal), normalized per
  [`cost-budget-design.md`](cost-budget-design.md), recorded as a `UsageRecord` to the
  per-run `ledger/usage.jsonl` via the shared lifecycle, and surfaced as tokens-per-task
  in `status` and a new `pactum usage` command. Tokens are the unit; cost/budget/
  estimation are later slices. (codex `--json` usage validated against real CLI output.)
- Blocking path-scope enforcement (M11.11) — `gate.scope_enforcement` defaults to
  `block`, so changed/new files that are undeclared by `paths_in_scope` or matched
  by `paths_out_of_scope` now produce `scope.status: blocked` and an overall
  failed gate. `gate.scope_enforcement: warn` preserves the M11.5 advisory warning
  behavior. In the autonomous review loop this composes with M11.8: the failed
  gate stops the loop with terminal reason `gate_failed` and records the gate
  report artifact for escalation.
- Project map honors `.gitignore` (M11.10) — in a git repo the scan enumerates files
  via `git ls-files --cached --others --exclude-standard`, so the repo's ignore rules
  are respected and build artifacts (`__pycache__/*.pyc`, `dist/`, …) are no longer
  indexed or churning the map; `.heurema` is always excluded; non-git dirs fall back to
  the filesystem walk. Fixes the real root cause of "spurious project-map staleness"
  on non-Go repos (found by the foreign-repo generality test; verified end-to-end:
  regenerating a `.pyc` no longer makes the map stale).
- `.heurema` version-control policy (M11.9) — `init` now writes a `.gitignore` that
  versions the durable run record (contracts, decisions, the ledger/audit timeline,
  gate verdicts, review findings, memory) and ignores only regenerable artifacts
  (`map/`, `cache/`, `runs/*/context/`) and raw `*.log` transcripts. The outcome lives
  in git history and the learnings in memory, so the bulky agent transcripts are not
  committed. Found via the foreign-repo generality test, which showed the old split
  committed generated context + gate logs while ignoring the ledger.
- `-race` in CI (M11.7) — a `make test-race` target (`go test -race ./...`) plus a
  dedicated `race` CI job run the race detector on every PR, catching the data-race
  class (e.g. the M10.2 live-output race) that a non-race `make check` misses. Local
  `make check` stays fast; the race run is CI/pre-merge only.

- Gate-failure-in-loop policy (M11.8) — a fixer-induced failed gate now stops the
  autonomous review loop with terminal reason `gate_failed`, records the failed
  gate status and report artifact in the round summary, and returns cleanly for
  escalation. Infrastructure gate errors still terminate as `error`.

- Idle agent timeout (M11.6) — `--timeout` is now an idle (no-output) safety
  timeout: a subprocess is killed only after the window passes with no stdout/stderr
  (default 10m), so long actively-streaming runs are never cut mid-work; the fixed
  total wall-clock cap is removed. Channel-based watchdog, race-clean.
- Legacy `default_executor` / `default_reviewer` / `adapters` config removed (#44) —
  intentionally ignored, not a bug.
- `agents doctor` status `ready` → `on_path`, `clarify list` alias, log-channel docs (#38).
- Per-stage `model[:effort]` + resolved-config header (#39, #41, #42); cross-model
  review (#43); fix stage (#46); review loop driver (#47); live agent output (#49);
  loop stop conditions — stalemate + K-consecutive-clean (M10.3); clarify suggest
  (M11.0); contract drafting (M11.1).
- Lifecycle dedup (#53) — the five agent-run commands (`execute run`, `review run`,
  `review fix`, `clarify suggest`, `contract draft`) now share one attempt-lifecycle
  helper (`runAgentAttemptLifecycle`); behavior-preserving, net −137 LOC (M11.2).
- Dead-code gate (M11.3) — `make check` now runs `go tool deadcode` (golang.org/x/
  tools, pinned via the go.mod `tool` directive), which flags unused package-level
  functions including production code reachable only from tests — the class `go vet`
  misses (and how the M11.2 refactor left orphaned wrappers). Removed the 7 dead funcs
  it found; tree is clean and the gate keeps it that way.
- Review-loop correctness (M11.4) — round-summary `open_findings` now reports the live
  open-finding count (was the per-round accept count); redundant `total_open_findings`
  removed. The loop dedups a re-proposed currently-open finding by exact
  `(file, line, message)` instead of minting a duplicate finding — it records a
  `duplicate` proposal-decision + one `review_proposal_duplicate` event. Resolved/
  rejected re-proposals are not suppressed; the M10.1 unparsed-findings guard is intact.
- Mechanical path-scope warnings (M11.5) — contracts can now carry
  `paths_in_scope` / `paths_out_of_scope` slash globs, `contract revise` can append
  them, and the gate reports advisory warnings for changed/new files that are
  undeclared or explicitly out of scope. Hard-fail/blocking behavior is intentionally
  left as a separate follow-up.
