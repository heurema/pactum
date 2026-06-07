# Reviewer Context

## Run
- Run id: run_20260606_195132
- Run status: contract_approved

## Contract
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

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 0
- Fresh: 0
- Stale: 0
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: needs_review
- Execution attempt id: attempt_002
- Execution exit code: 0
- Validation command results:
  - command_001: make check (exit 0, timed out: false, result: gate/validation/command_001/result.json)
- Change summary:
  - changed files:
    - docs/agents.md
    - internal/app/app.go
    - internal/app/review_loop.go
    - internal/app/review_loop_test.go
  - new files:
    - none
  - missing files:
    - none

## Existing manual review
- Review status: pending
- Current findings summary: findings=0 open=0 resolved=0 blocking_open=0
- Existing findings:
  - none
- Existing resolutions:
  - none
- Proposal summary: pending=0 accepted=0 rejected=0
- Existing proposals:
  - none

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
- Execution result: execute/last-result.json

## Reviewer guidance
- This context is not complete semantic truth.
- Use `pactum search "<term>"` and inspect files before proposing findings.
- Do not invent changes.
- Do not approve automatically.
- If uncertain, propose a blocking finding that asks for clarification.
