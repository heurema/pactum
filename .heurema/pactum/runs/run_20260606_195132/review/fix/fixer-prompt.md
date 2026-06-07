# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts, but it does not mark review findings resolved automatically.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260606_195132/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260606_195132/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260606_195132/review/review.json, .heurema/pactum/runs/run_20260606_195132/review/findings.jsonl, .heurema/pactum/runs/run_20260606_195132/review/resolutions.jsonl

## Approved contract
- Goal: Make 'pactum review loop' safe for long autonomous runs by adding two stop conditions beyond max_rounds: stalemate detection (stop when the fixer stops changing the working tree) and K-consecutive-clean (require K clean review rounds before declaring convergence), each with a distinct terminal reason
- In scope:
  - Stalemate-by-fingerprint: after each round compute a fingerprint of the working tree (reuse the gate's file hashing / a hash of changed files + HEAD). If N consecutive rounds in which a fix ran leave the fingerprint unchanged, terminate with terminal_reason 'stalemate'. N from config (e.g. limits.review.patience) or a flag, with a sane default (e.g. 2)
  - K-consecutive-clean: require K clean review rounds (no created proposals, no warnings) in a row before terminating as 'clean_round'; a non-clean round resets the streak. K from config/flag, default 1 (preserving current L3a behavior)
  - Record the per-round signals in the loop summary (e.g. unchanged-fingerprint streak, clean streak); add a docs/agents.md note
  - Tests with fake agents: stalemate triggers after N unchanged fix rounds; K-clean requires K consecutive clean rounds; default behavior unchanged
- Out of scope:
  - Budget/cost stop (needs token/cost accounting from the agent CLIs — a separate slice)
  - Rebuttal channel, dedup findings across rounds, severity composition, multi-reviewer panel
  - Native LLM API or model/provider abstraction
  - Touching generated .heurema artifacts
- Acceptance criteria:
  - When a fix runs but the working tree is unchanged for N consecutive rounds, the loop stops with terminal_reason 'stalemate' instead of grinding to max_rounds
  - With K>1 the loop requires K consecutive clean review rounds before terminal_reason 'clean_round'; a non-clean round resets the clean streak
  - Default behavior (no new config/flags) is unchanged from L3a; covered by tests
- Validation commands:
  - make check

## Current review findings
- Summary: findings=2 open=2 resolved=0 blocking_open=0
- Findings:
  - f_001 severity=low category=quality blocking=false status=open: Clean rounds (accepted==0) compute a full working-tree fingerprint (repo scan + git rev-parse) that is stored in the round summary but never compared against anything. This is a minor per-round cost, not a defect — it is consistent with the contract's request to record the working_tree_fingerprint signal each round. Flagged only for awareness; no change required.
    location: internal/app/review_loop.go:162
  - f_002 severity=low category=validation blocking=false status=open: The stalemate streak-reset branch is never exercised by tests. The fixer test helper (TestReviewLoopFixerHelperProcess) only reads stdin and exits — it never modifies the working tree — so the `else { unchangedFingerprintStreak = 0 }` branch at review_loop.go:210-212 (the case where a fixer actually changes files and should reset the stalemate counter) has no coverage. All stalemate-path tests rely on a no-op fixer where the fingerprint is invariant. Acceptance criteria are still met by existing tests; this is a robustness gap, not a defect: a regression that made fingerprints always-equal (e.g. accidentally hashing a constant) would pass the current suite while breaking real stalemate avoidance. Consider a test where a fixer writes a distinct file each round and asserting the streak stays 0 / no premature stalemate.
    location: internal/app/review_loop_test.go:354

## Fix boundaries
- Trace each finding to the relevant code before acting.
- Fix valid findings in place.
- For false positives, explain a concrete rebuttal instead of changing code.
- Keep changes inside the approved contract and review-finding scope.
- Do not edit `.heurema` artifacts.
- Do not run `pactum review approve`, `pactum review resolve`, or any review loop command.

## Output shape
In your final output, list each finding id with one of:
- fixed: what changed and where
- rebutted: why the finding is a false positive
- blocked: what concrete information or state is missing
