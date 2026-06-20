# Contract Review: Testability

You are reviewing a software change contract through the **acceptance-testability** lens.

Review the contract fields below using only your assigned lens checklist.
Do not flag issues that belong to other lenses.

## Contract

**Goal**: Give the CONTRACT-REVIEW loop the same operator finding-resolution that CODE-REVIEW already has, so a contract that ends `blockers_open` but is sound on operator judgment can be unblocked by resolving specific findings with a recorded, auditable reason — mirroring code-review's existing machinery, NOT a parallel system and NOT a force/bypass flag.

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

**Scope in**:
  - Persist contract-review findings under contract/reviewer/findings.jsonl with stable non-empty id and fingerprint fields while preserving category, message, and blocking.
  - Derive each contract-review finding fingerprint from the same normalized blocker key used by contract-review no-progress detection.
  - Add append-only contract/reviewer/resolutions.jsonl records for manual contract-review finding resolutions, including finding_id, fingerprint, current contract hash/version, reason, by, timestamp, and source="manual".
  - Add pactum contract review finding resolve [run_id] <id> --reason "..." --by <who> with --json support, standard next affordances, and standard error-envelope behavior.
  - Make contract approve subtract active manual contract-review resolutions from current blocking findings, where active means matching current contract hash/version.
  - Print a loud waiver summary during contract approval when operator-resolved blocking findings are accepted, including count plus each waived finding id, reason, and by.
  - Reuse or extract existing code-review resolution, deduplication, and summary helpers where practical while keeping contract-review artifacts separate under contract/reviewer/.

**Scope out**:
  - Changing the contract goal.
  - Reviewer-prompt calibration such as no-nitpicks behavior.
  - Adding a disposition enum, multi-value disposition taxonomy, or force/bypass approval flag.
  - Changing the shared pactum.reviewer_findings.v1alpha1 capture schema emitted by reviewers.
  - Implementing changed-in-that-area or partial invalidation logic for resolutions.
  - Changing code-review command behavior, code-review approval gates, or code-review artifact locations.
  - Running real contract reviewers, fixers, or other unsandboxed agents in tests.

**Acceptance criteria**:
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

**Validation commands**:
  - go test ./internal/app
  - go test ./...
  - go build ./...
  - make check

**Assumptions**:
  - contract/reviewer/findings.jsonl is the durable source of current contract-review findings for approval gating.
  - Historical contract-review findings that lack id or fingerprint do not need migration; they may require rerunning contract review or continue to fail closed.
  - The current contract hash/version can be computed at both resolution time and approval time from the existing contract artifact state.
  - Existing code-review helper extraction can be done without changing code-review CLI flags, artifact shape, or approval semantics.
  - contract approve matches an active manual resolution to a current blocking finding by FINGERPRINT (the normalized lens/category/message blocker key), NOT by finding id. Finding ids are per-write handles used to address a finding in the resolve command and need not be stable across re-runs; the fingerprint is the cross-run match key. A blocking finding re-emitted by a later review with the same fingerprint stays resolved while the contract hash is unchanged.
  - If the resolution record append to contract/reviewer/resolutions.jsonl succeeds but the subsequent ledger event append fails, the resolve command returns an error indicating partial failure; the already-written resolution record is durable and contract approve will honor it in subsequent runs since approve reads from resolutions.jsonl directly, not from the ledger index.

## Lens: Testability

Checklist:
- Is each acceptance criterion backed by or expressible as a runnable validation command (not just prose)?
- Are any criteria purely prose with no machine-checkable outcome?

## Output

State your analysis in prose. If you find issues, also include a structured block:

```json
{
  "schema": "pactum.reviewer_findings.v1alpha1",
  "findings": [
    {
      "message": "Describe the contract issue clearly.",
      "severity": "medium",
      "category": "quality",
      "blocking": true,
      "evidence": "Quote or cite the contract field that shows the issue."
    }
  ]
}
```

Rules:
- Use severity: low, medium, high, critical.
- Use category: correctness, scope, quality, validation, process, other.
- Omit file and line (not applicable for contract review).
- Set blocking=true for defects that should block approval: gaps that make the contract unexecutable or ungatable.
- Set blocking=false for advisory issues.
- If no issues, say so clearly. Do not include an empty findings block.
