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
- **Spurious project-map staleness** (low). `prompt build` / `execute dry-run`
  intermittently report "project map is stale" with a clean working tree, needing a
  `map refresh` + `prompt build` again. Investigate what invalidates freshness when
  nothing tracked changed.

## Review→fix loop (L3b and beyond)

- **Loop stop conditions.** Stalemate-by-fingerprint and K-consecutive-clean —
  **done (M10.3)**. Remaining: **budget stop** (`budget.max_usd`) (med) — needs
  token/cost accounting from the agent CLIs (parse usage + pricing), a separate
  prerequisite slice.
- **Rebuttal channel** (med). Feed the fixer's rebuttal of a false positive back to
  the reviewer on the next round so it re-judges instead of re-reporting.
- **Semantic review-finding dedup** (low). The autonomous review loop now dedups
  proposals against currently open findings only by the exact stored
  `(file, line, message)` tuple. Reworded messages, line-number drift, and other
  semantic duplicates are still treated as distinct findings; add fuzzy/semantic
  reconciliation only after the exact-match behavior has settled.
- **Gate-failure-in-loop policy** (med). A fixer that breaks `make check` makes the
  loop abort (summary recorded, as an error). Define a meaningful terminal — stop +
  escalate — distinct from infrastructure errors.
- **JSON sub-command contract** (low). The loop json-parses sub-command stdout; a
  mid-loop not-ready condition could inject human text into the parsed buffer. Make
  the contract explicit.

## Loop phases not yet built

- **L2 — severity by composition** (med). A broad fix pass, then a critical/major-
  only final gate (no per-finding severity schema).
- **Cross-model panel** (med). Run N reviewers (codex + claude) per round; merge and
  de-duplicate findings; reconcile severity.
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

- **Blocking path-scope enforcement** (med). The gate now emits advisory warnings
  for changed/new files outside declared path globs, but it intentionally does not
  hard-fail on those warnings. Decide whether and how to promote path-scope
  violations to a blocking gate policy.
- **Clarify commands reset approval silently** (med). `clarify ask` / `answer` /
  `suggest` add open questions via `refreshClarificationArtifacts`, which calls
  `resetApprovalIfApproved` — so running them on an already-approved/executed run
  silently regresses it to `clarifying`. Guard or warn when the run is already
  approved (pre-existing; `clarify suggest` makes bulk creation easier).
- **`-race` in CI** (med). `make check` runs `go test ./...` without `-race`, so the
  M10.2 live-output data race slipped through. The full suite is race-clean as of
  M10.2, so enabling `-race` (a CI step or a `make check-race` target) is now safe and
  would catch this class — at a notable test-time cost (~20× the app package). The
  `tool` directive added for `deadcode` (M11.3) is the pattern to reuse here.

## Resolved (for reference)

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
