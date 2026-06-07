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

- **Clarify commands reset approval silently** (med). `clarify ask` / `answer` /
  `suggest` add open questions via `refreshClarificationArtifacts`, which calls
  `resetApprovalIfApproved` — so running them on an already-approved/executed run
  silently regresses it to `clarifying`. Guard or warn when the run is already
  approved (pre-existing; `clarify suggest` makes bulk creation easier).

## Resolved (for reference)

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
