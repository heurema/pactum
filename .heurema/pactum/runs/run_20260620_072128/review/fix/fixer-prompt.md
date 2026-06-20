# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260620_072128/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260620_072128/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260620_072128/review/review.json, .heurema/pactum/runs/run_20260620_072128/review/findings.jsonl, .heurema/pactum/runs/run_20260620_072128/review/resolutions.jsonl

## Approved contract
- Goal: Give the CONTRACT-REVIEW loop the same operator finding-resolution that CODE-REVIEW already has, so a contract that ends `blockers_open` but is sound on operator judgment can be unblocked by resolving specific findings with a recorded, auditable reason — mirroring code-review's existing machinery, NOT a parallel system and NOT a force/bypass flag.

Background: contract review can terminate `blockers_open` with open blocking findings. Today `contract approve` fail-closes on those and there is NO way to resolve a contract-review finding (the only action is re-running `contract review`, which re-stalemates). Code-review already solved this: `review finding resolve`, a resolutions store, fingerprint dedup, and an approval gate of `BlockingOpen == 0` (current blocking findings minus active resolutions). This slice brings the same to contract-review. The reviewer-prompt calibration ("no nitpicks") is a SEPARATE prompt-only slice and is out of scope here.

In scope:
1. Persist contract-review findings with a stable ID and a fingerprint. The current `contract/reviewer/findings.jsonl` entry carries only category/message/blocking; add an `id` and a `fingerprint` (the normalized blocker key already used for no-progress detection, e.g. normalized lens/category/message) to each persisted finding.
2. Add an append-only `contract/reviewer/resolutions.jsonl`, mirroring code-review's `review/resolutions.jsonl` record shape. A manual resolution record carries: finding id, fingerprint, the contract hash/version at resolution time, reason, by (principal), timestamp, source="manual".
3. Add `pactum contract review finding resolve [run_id] <id> --reason "..." --by <who>`: append a manual resolution for a blocking contract-review finding. `--reason` and `--by` are REQUIRED. Mirror the command family and flag shape of code-review's `review finding resolve`. Emit JSON output (`--json`) consistent with the rest of the CLI (carry `next`; use the error envelope on failure). Append a ledger event as an index; the detailed reason/by live in the resolutions JSONL, not the ledger event payload.
4. Make `contract approve` resolution-aware: a blocking finding is cleared if it is fixed (absent from the current findings) OR has an ACTIVE manual resolution. A resolution is active only if its recorded contract hash/version matches the CURRENT contract hash — any change to the contract invalidates prior resolutions (conservative; do not implement "changed in that area" detection). The gate mirrors code-review's "no open blocking findings remain". Keep the existing fail-closed behavior on absent/unreadable/malformed artifacts. When approving while one or more blockers are operator-resolved (waived), print a LOUD waiver summary: a count and the list of waived findings (id + reason + by).
5. Reuse, do not duplicate: extract or share the resolution-record / dedup / summary helpers that code-review already has (in review.go / review_loop.go) rather than building a parallel mechanism for contract-review. Keep the STORES separate — contract-review findings/resolutions live under contract/reviewer/, NOT under review/ (do not conflate pre-approval contract review with post-execution code review).

Out of scope (do NOT do here): the reviewer-prompt "no nitpicks" calibration (separate prompt-only slice); any `--disposition` enum or multi-value disposition taxonomy; any change to the shared `pactum.reviewer_findings.v1alpha1` capture schema; "changed in that area" partial resolution invalidation; any change to code-review's own commands or gate; a `contract approve --force` flag.

Tests (helper-process/temp; no real agents): resolving a blocking contract-review finding lets `contract approve` succeed and prints the waiver summary; approve still fails-closed while any unresolved blocking finding remains; after a resolution exists, changing the contract (new hash) invalidates it so approve refuses again until re-resolved; `--reason`/`--by` are required; the resolution is written to contract/reviewer/resolutions.jsonl and a ledger event is appended; malformed/missing artifacts still fail closed.

Validation: go test ./internal/app, go test ./..., go build ./..., make check.
- In scope:
  - Persist contract-review findings under contract/reviewer/findings.jsonl with stable non-empty id and fingerprint fields while preserving category, message, and blocking.
  - Derive each contract-review finding fingerprint from the same normalized blocker key used by contract-review no-progress detection.
  - Add append-only contract/reviewer/resolutions.jsonl records for manual contract-review finding resolutions, including finding_id, fingerprint, current contract hash/version, reason, by, timestamp, and source="manual".
  - Add pactum contract review finding resolve [run_id] <id> --reason "..." --by <who> with --json support, standard next affordances, and standard error-envelope behavior.
  - Make contract approve subtract active manual contract-review resolutions from current blocking findings, where active means matching current contract hash/version.
  - Print a loud waiver summary during contract approval when operator-resolved blocking findings are accepted, including count plus each waived finding id, reason, and by.
  - Reuse or extract existing code-review resolution, deduplication, and summary helpers where practical while keeping contract-review artifacts separate under contract/reviewer/.
- Out of scope:
  - Changing the contract goal.
  - Reviewer-prompt calibration such as no-nitpicks behavior.
  - Adding a disposition enum, multi-value disposition taxonomy, or force/bypass approval flag.
  - Changing the shared pactum.reviewer_findings.v1alpha1 capture schema emitted by reviewers.
  - Implementing changed-in-that-area or partial invalidation logic for resolutions.
  - Changing code-review command behavior, code-review approval gates, or code-review artifact locations.
  - Running real contract reviewers, fixers, or other unsandboxed agents in tests.
- Acceptance criteria:
  - A contract-review loop that writes findings produces contract/reviewer/findings.jsonl entries with non-empty id and fingerprint fields in addition to category, message, and blocking.
  - Resolving an existing blocking contract-review finding with pactum contract review finding resolve <run_id> <id> --reason "operator accepted" --by alice --json appends exactly one manual resolution record to contract/reviewer/resolutions.jsonl and returns JSON containing the resolution and next commands.
  - The resolution command rejects missing --reason, missing --by, unknown finding ids, and non-blocking finding ids without appending a resolution.
  - A successful manual resolution appends a ledger event that indexes the resolution action without storing the detailed reason or by value in the ledger payload.
  - contract approve succeeds when every current blocking contract-review finding is either absent from current findings or has an active manual resolution for the current contract hash/version.
  - contract approve fails when any unresolved blocking contract-review finding remains, including when reviewers have been removed from config after findings were written.
  - Changing the contract after a manual resolution invalidates the previous resolution because the current contract hash/version no longer matches, so approval fails until the finding is resolved again.
  - contract approve remains fail-closed for absent, unreadable, or malformed contract-review findings or resolutions artifacts.
  - Approval output that relies on one or more manual resolutions includes a loud waiver summary with the waived finding count and each waived finding id, reason, and by.
  - Helper-process or temp-directory tests cover resolution success, unresolved blocker failure, hash invalidation, required flags, resolution artifact writing, ledger event writing, and malformed or missing artifact failure.
  - A blocking finding re-emitted by a subsequent contract review (same fingerprint, possibly different id) remains cleared by its existing active manual resolution — matched by fingerprint, not id — so contract approve still succeeds without re-resolving, provided the contract hash is unchanged.
  - When the same finding fingerprint is resolved more than once, all resolution records are retained in contract/reviewer/resolutions.jsonl; the approval gate treats any active resolution for a fingerprint as sufficient to clear that fingerprint, and the waiver summary deduplicates by fingerprint so each waived fingerprint appears at most once in the output regardless of how many resolution records exist for it.
  - Multiple current blocking findings that share the same fingerprint are collectively cleared by a single active manual resolution for that fingerprint; the waiver summary treats them as one waived entry rather than one entry per matching finding instance.
- Validation commands:
  - go test ./internal/app
  - go test ./...
  - go build ./...
  - make check

## Current review findings
- Summary: findings=8 open=8 resolved=0 blocking_open=8
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=medium category=correctness blocking=true status=open: The next affordance still emits the old `pactum contract review <run_id>` spelling after the CLI was changed to require `pactum contract review run <run_id>`, so JSON/status guidance can direct operators to an invalid command.
    location: internal/app/resolve.go:268
  - f_002 severity=medium category=correctness blocking=true status=open: The waiver summary is not deduplicated by fingerprint: duplicate current findings with the same fingerprint are cleared by one resolution but still produce multiple waiver entries and an inflated warning count.
    location: internal/app/contract_review.go:1057
  - f_003 severity=medium category=correctness blocking=true status=open: contract approve does not fail closed when the contract-review resolutions artifact is absent.
    location: internal/app/contract_review.go:1015
  - f_004 severity=medium category=correctness blocking=true status=open: Waiver summaries are not deduplicated by fingerprint.
    location: internal/app/contract_review.go:1057
  - f_005 severity=medium category=correctness blocking=true status=open: The lifecycle next affordance still points to the old contract review command spelling.
    location: internal/app/resolve.go:268
  - f_006 severity=medium category=quality blocking=true status=open: The waiver-summary deduplication test does not actually assert deduplication.
    location: internal/app/contract_review_resolve_test.go:395
  - f_007 severity=medium category=correctness blocking=true status=open: The contract-review approval path silently treats a missing contract/reviewer/resolutions.jsonl as an empty resolution set, so missing durable resolution state is not fail-closed.
    location: internal/app/contract_review.go:1015
  - f_008 severity=medium category=quality blocking=true status=open: The contract-review documentation still advertises the old `contract review <run>` command and does not document the new resolution workflow, even though the CLI now exposes `pactum contract review run` plus `pactum contract review finding resolve`.
    location: docs/contract-review-design.md:76
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - none

## Fix boundaries
- Trace each finding to the relevant code before acting.
- Fix valid findings in place.
- For false positives, explain a concrete rebuttal instead of changing code.
- Keep changes inside the approved contract and review-finding scope.
- Do not edit `.heurema` artifacts.
- Do not run `pactum review approve`, `pactum review finding resolve`, or `pactum review run`.

## House style
- Match the surrounding code: idiom, naming, comment density.
- Comment only where the code is not self-explanatory; do not narrate the obvious.
- Search for and reuse existing helpers before writing new ones.
- Keep the diff small and focused: change only what the contract requires.
- Simplicity first: no enterprise patterns for simple problems, question every new abstraction, no premature generalization or optimization.
- Over-engineering DON'Ts: wrappers that add nothing, factories or abstractions for a single case, unused extension points, dual implementations where the old path has no callers, silent fallbacks that hide failures.
- No dead code, no commented-out code, no unused parameters.
- Handle errors per the project's existing convention; no silent failures.
- Tests verify behavior, not implementation details, and cover error paths.
- Fake-test DON'Ts: always-pass tests, hardcoded-value checks, assertions on mock behavior instead of the code under test, ignored errors, commented-out cases.

The reviewer will re-check your fixes against the discipline rules above.

## Output shape
Your final output MUST include exactly one fenced `json` block with this shape:

```json
{
  "schema": "pactum.review_fix_outcomes.v1alpha1",
  "outcomes": [
    {
      "finding_id": "f_001",
      "outcome": "fixed",
      "note": "What changed and where, or the concrete rebuttal/blocker."
    }
  ]
}
```

Rules:
- Include exactly one outcome entry for every blocking finding listed above with status open.
- Do NOT edit code for advisory (non-blocking) findings, and do NOT emit outcomes for them; they are context only.
- Use outcome fixed when you changed code to address a valid blocking finding.
- Use outcome rebutted when the blocking finding is a false positive; note must contain the concrete rebuttal.
- Use outcome blocked when concrete missing information or state prevents a fix.
- Do not include advisory or resolved findings in the outcomes list.
