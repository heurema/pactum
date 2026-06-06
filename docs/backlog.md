# Backlog

Deferred work and known follow-ups, consolidated from dogfood reports and PR
reviews across M8–M10. Rough priority in parentheses.

## Execution / review UX

- **Timeout vs. completion** (med). A run killed by `--timeout` reports
  `timed_out: true` / `exit -1` even when the agent had already produced complete,
  valid work (observed when codex self-runs `make check` past the wall).
  Distinguish "timed out mid-work" from "timed out after a usable diff"; consider a
  longer default `--timeout` for large tasks.
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
- **Dedup findings across rounds** (med). An unfixed issue re-proposed each round is
  accepted as a *new* finding; counts inflate and the ledger fills with duplicates.
  Dedup proposals against open findings; fix the round-summary `open_findings` field
  (it currently equals the per-round accept count, not the live open count).
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

## Hardening / cleanup

- **Mechanical scope enforcement** (med). The gate surfaces changed files and runs
  validation but does not check them against the contract's in/out-of-scope lists;
  scope adherence is human-only today. Consider flagging out-of-scope changes.
- **Clarify commands reset approval silently** (med). `clarify ask` / `answer` /
  `suggest` add open questions via `refreshClarificationArtifacts`, which calls
  `resetApprovalIfApproved` — so running them on an already-approved/executed run
  silently regresses it to `clarifying`. Guard or warn when the run is already
  approved (pre-existing; `clarify suggest` makes bulk creation easier).
- **`-race` in CI** (med). `make check` runs `go test ./...` without `-race`, so the
  M10.2 live-output data race slipped through. The full suite is race-clean as of
  M10.2, so enabling `-race` (a CI step or a `make check-race` target) is now safe and
  would catch this class — at a notable test-time cost (~20× the app package).

## Resolved (for reference)

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
