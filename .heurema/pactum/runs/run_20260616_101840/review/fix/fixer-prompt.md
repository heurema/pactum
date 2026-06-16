# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260616_101840/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260616_101840/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260616_101840/review/review.json, .heurema/pactum/runs/run_20260616_101840/review/findings.jsonl, .heurema/pactum/runs/run_20260616_101840/review/resolutions.jsonl

## Approved contract
- Goal: Implement the first slice of the declarative contract-revise CLI specified in docs/contract-revise-cli-design.md. Replace the add-only flag soup with a declarative partial-update for an agent operator. Slice 1 scope: (1) 'contract show --json' returns the editable contract plus a 'version' token = the sha256 of the normalized contract content (the same hash used at approve, exposed pre-approval). (2) 'contract revise <run> --from -|<file>' reads a partial JSON document of shape {base_version, contract:{partial fields}}; every field present replaces that field's value wholesale (lists replace whole, stable-dedupe keep-first, report dropped duplicates, preserve order); fields absent are untouched; unknown fields are a hard error with a did-you-mean; null for a non-nullable scalar (goal) is rejected; [] clears a list. (3) The version check is ON BY DEFAULT: apply only if base_version equals the current version; missing or stale base_version is an atomic reject (nothing written) with contract_unchanged:true and a structured error. (4) No-op idempotence: identical content does not re-hash or reset approval (changed:false, exit 0). (5) On an approved contract a content-changing revise is rejected unless --allow-approval-reset is passed; the success result reports approval_reset / previous_approval_hash / attempts_orphaned. (6) Remove the --add-* and --goal flags from 'contract revise' entirely (pactum has no users, no deprecation); switch the internal 'contract accept' path (which today applies the drafter proposal via the same Add-field mechanism) to the new partial-replace so accept still works (on the empty skeleton, replace == the proposal). Errors: a single structured JSON object with ALL issues at once (field path, machine code, message), non-zero exit; reject unknown fields. Defer and do NOT implement in this slice: --dry-run, field-level diffs, and the --force bypass (until --force exists, a missing base_version is simply rejected). Update all affected tests. Follow the full spec and rationale in docs/contract-revise-cli-design.md.
- In scope:
  - Expose a top-level version token from pactum contract show <run> --json for the editable contract content, using the same normalized contract hash used by contract approve.
  - Replace contract revise add-style flags with pactum contract revise <run> --from -|<file> reading a JSON wrapper {base_version, contract:{...}}.
  - Implement partial-replace semantics for editable contract fields: present fields replace wholesale, absent fields are untouched, and list fields preserve submitted order.
  - Apply stable list deduplication with keep-first behavior and report dropped duplicates in the revise result.
  - Require base_version by default and reject missing or stale versions atomically without writing contract, approval, run, ledger, or rendered contract artifacts.
  - Return single structured JSON failure objects for revise validation errors, aggregating all issues with field path, machine code, and message.
  - Gate content-changing revisions to approved contracts behind --allow-approval-reset and report approval_reset, previous_approval_hash, and attempts_orphaned on success.
  - Switch contract accept so draft proposals are applied through the new partial-replace mechanism while preserving existing accept behavior and ledger events.
  - Remove --goal and all --add-* contract revise flags from CLI grammar, help output, tests, and in-repo Pactum agent/skill instructions.
- Out of scope:
  - Do not implement --dry-run for contract revise in this slice.
  - Do not implement field-level diff output in this slice.
  - Do not implement a --force or unconditional version-bypass path in this slice.
  - Do not add whole-document replace, JSON Patch, or editor-driven revise flows.
  - Do not keep deprecated compatibility aliases for the removed --goal or --add-* flags.
- Acceptance criteria:
  - contract show <run> --json returns a parseable JSON object containing top-level version and contract fields; version equals the normalized contract content hash that contract approve records for the same content.
  - contract revise <run> --from - and contract revise <run> --from <file> both accept a valid partial JSON document with base_version and contract fields and apply the same behavior.
  - For supported editable fields goal, scope.in, scope.out, paths_in_scope, paths_out_of_scope, acceptance_criteria, validation.commands, and assumptions, every present field replaces the stored value wholesale; absent fields remain byte-for-byte equivalent in the resulting contract content.
  - Submitting [] for a supported list field clears that list; duplicate submitted list entries are stable-deduped with the first occurrence kept, submitted order preserved, and dropped duplicates reported in the success JSON.
  - A missing or stale base_version exits non-zero with one structured JSON error object including ok:false, contract_unchanged:true, and issues[], and leaves all run artifacts unchanged.
  - Invalid revise input reports all detected issues in one structured JSON object, including unknown fields with a did-you-mean message and null goal rejected as a non-nullable scalar.
  - A successful content-changing revise returns JSON with ok, contract, base_version, new_version, changed:true, and deduped fields, updates rendered contract artifacts consistently, and records the expected contract_revised ledger event.
  - Submitting a partial update that normalizes to the existing content exits 0 with changed:false, new_version equal to the current version, no approval reset, and no changed approval hash.
  - On an approved contract, a content-changing revise without --allow-approval-reset is rejected atomically; with --allow-approval-reset it succeeds, resets approval to pending, reports previous_approval_hash and attempts_orphaned, and does not delete old attempt artifacts.
  - contract revise --help no longer advertises --goal or any --add-* option, and attempts to use those removed flags fail without mutating contract state.
  - contract accept still accepts a pending drafter proposal onto an empty skeleton contract and produces the same accepted proposal, revised contract fields, next commands, and ledger events expected by existing accept workflows.
- Validation commands:
  - go test ./internal/app -run TestContract
  - go test ./internal/app
  - make check

## Current review findings
- Summary: findings=13 open=13 resolved=0 blocking_open=11
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=high category=correctness blocking=true status=open: No-op revise on an approved contract is treated as content-changing because the code forces contract.Status to draft before comparing hashes.
    location: internal/app/contract.go:220
  - f_002 severity=medium category=correctness blocking=true status=open: Dropped duplicate list entries are omitted from the success response when deduplication normalizes the request to unchanged content.
    location: internal/app/contract.go:227
  - f_003 severity=high category=correctness blocking=true status=open: Approved no-op revisions are treated as content-changing because the revise path changes status to draft before comparing versions.
    location: internal/app/contract.go:220
  - f_004 severity=medium category=correctness blocking=true status=open: contract accept converts omitted proposal lists into empty list replacements, clearing existing fields and making the no-fields guard ineffective.
    location: internal/app/contract_draft.go:433
  - f_005 severity=medium category=scope blocking=true status=open: The mandatory Pactum skill workflow still tells agents to use removed contract revise flags.
    location: assets/agent-skills/pactum/references/workflow.md:59
  - f_008 severity=medium category=quality blocking=true status=open: New contract revise rejection paths have no direct tests.
    location: internal/app/contract.go:149
  - f_009 severity=medium category=quality blocking=true status=open: New partial-update semantics are only covered by basic file-backed happy paths.
    location: internal/app/app_test.go:1403
  - f_010 severity=medium category=correctness blocking=true status=open: contractPartialUpdateHasChanges treats an empty draft proposal as a real change because contractPartialUpdateFromDraftProposal always sets non-nil list pointers, allowing contract accept to accept and record a no-op proposal instead of preserving the existing 'no contract fields to apply' rejection.
    location: internal/app/contract_draft.go:442
  - f_011 severity=medium category=quality blocking=true status=open: README and dogfood docs tell users to feed `contract show --json` output directly into `contract revise --from`, but that JSON has `version`, `run_id`, `run_status`, and `approval`; the revise parser accepts only `base_version` and `contract`, so the documented flow fails unless the user manually rewrites the wrapper.
    location: README.md:148
  - f_012 severity=medium category=quality blocking=true status=open: The live Pactum workflow documentation still teaches the removed `--goal` and `--add-*` flags. `assets/agent-skills/pactum/SKILL.md` explicitly tells agents to read this reference before acting, so the in-repo agent instructions still point at a CLI grammar this change removed.
    location: assets/agent-skills/pactum/references/workflow.md:59
  - f_013 severity=medium category=quality blocking=true status=open: `docs/flow.md` still describes `contract revise` as append-only with `--goal` and `--add-*` flags and says revising an approved contract resets approval unconditionally. The new behavior requires `--from`, `base_version`, partial replacement, and `--allow-approval-reset`.
    location: docs/flow.md:146
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_006 severity=low category=scope blocking=false status=open: Approval-reset success JSON omits attempts_orphaned when the count is zero.
    location: internal/app/contract.go:104
  - f_007 severity=medium category=quality blocking=false status=open: contract show --json version is not asserted by tests.
    location: internal/app/app_test.go:1324

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
  "schema": "pactum.review_fix_outcomes.v1",
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
