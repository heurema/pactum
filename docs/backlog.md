# Backlog

Deferred work and known follow-ups, consolidated from dogfood reports and PR
reviews across M8–M10. Rough priority in parentheses.

## Execution / review UX

- **Live agent output** (high). `execute run` / `review run` / `review fix`
  capture the agent's stdout/stderr to log files but print nothing to the terminal
  during the run — a multi-minute silent black box. Stream the agent output live
  (tee to the log file *and* the operator's terminal). Caveat: it must not pollute
  `--json` output, nor the captured JSON that `review loop` parses from its
  sub-commands — stream to stderr, or only when not in JSON mode.
- **Timeout vs. completion** (med). A run killed by `--timeout` reports
  `timed_out: true` / `exit -1` even when the agent had already produced complete,
  valid work (observed when codex self-runs `make check` past the wall).
  Distinguish "timed out mid-work" from "timed out after a usable diff"; consider a
  longer default `--timeout` for large tasks.

## Review→fix loop (L3b and beyond)

- **Loop stop conditions** (high). Stalemate-by-fingerprint (reuse the gate's
  working-tree hashing — stop after N rounds with unchanged HEAD + tree); budget
  stop (`budget.max_usd`); optional K-consecutive-clean rounds.
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
- **Phase 1 — clarify loop** (high, large). The agent generates clarification
  questions and refines the contract; the human answers; loop to a precise contract,
  preserving the human approve gate.

## Hardening / cleanup

- **Mechanical scope enforcement** (med). The gate surfaces changed files and runs
  validation but does not check them against the contract's in/out-of-scope lists;
  scope adherence is human-only today. Consider flagging out-of-scope changes.
- **Lifecycle dedup** (low). `execute run` / `review run` / `review fix` share a
  near-identical attempt/result/event lifecycle. A shared helper would reduce drift
  (the L1 staleness bug came from the fork) — revisit once the shape is stable (after
  L3b), and only if it abstracts cleanly.
- **`-race` in CI** (med). `make check` runs `go test ./...` without `-race`, so the
  M10.2 live-output data race slipped through. The full suite is race-clean as of
  M10.2, so enabling `-race` (a CI step or a `make check-race` target) is now safe and
  would catch this class — at a notable test-time cost (~20× the app package).

## Resolved (for reference)

- Legacy `default_executor` / `default_reviewer` / `adapters` config removed (#44) —
  intentionally ignored, not a bug.
- `agents doctor` status `ready` → `on_path`, `clarify list` alias, log-channel docs (#38).
- Per-stage `model[:effort]` + resolved-config header (#39, #41, #42); cross-model
  review (#43); fix stage (#46); review loop driver (#47).
