# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260617_210449/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260617_210449/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260617_210449/review/review.json, .heurema/pactum/runs/run_20260617_210449/review/findings.jsonl, .heurema/pactum/runs/run_20260617_210449/review/resolutions.jsonl

## Approved contract
- Goal: Make pactum's code-review loop never silently drop reviewer findings, and recover automatically when a reviewer omits the structured findings block before failing loud.

Problem: the reviewer prompt (renderReviewerPrompt in internal/app/review.go) permits findings either in prose OR in a fenced JSON block (schema pactum.reviewer_findings.v1alpha1), but the parser (parseReviewerFindingBlocks in internal/app/review_proposals.go) reads ONLY the JSON block. A reviewer (codex over ACP) that writes findings in prose has them silently dropped. The loop's clean/unparsed discriminator (internal/app/review_loop.go) only treats zero proposals as a parse-miss when a warning fired, and that warning only fires when the schema string is literally present in the text. So prose-only findings -- or findings from one lens while other lenses produced proposals -- vanish with no signal, and the review can look approvable while real findings are missing. This actually happened: two lenses found a validation-gating bug and a renderer bug, wrote them in prose, and they were silently absent from findings.jsonl.

Fix -- force the format + structural discriminator + per-lens corrective retry with escalation:

1. Prompt: make the fenced pactum.reviewer_findings.v1alpha1 JSON block MANDATORY and ALWAYS emitted -- emit "findings": [] when there are no findings. Drop "prose OR json" as equal reporting channels; prose becomes a human-readable supplement only and is never parsed. Include a worked clean example showing "findings": [].

2. Struct: change reviewerFindingBlock.Findings from []json.RawMessage to *[]json.RawMessage (or otherwise track key presence) so a block {"schema": ...} with NO findings key (nil) is distinguishable from an explicit empty array. Absent/nil => malformed parse-miss; non-nil (including empty []) => valid block, clean when empty.

3. Parser: warn whenever a reviewer attempt yields no VALID findings block (drop the guard that only warns when the schema string is present). A valid block with "findings": [] parses to one block, zero findings, zero warnings (clean). A block missing the findings key, or no block at all, is a parse-miss => warning.

4. Per-lens enforcement in the discriminator: a missing or invalid block on ANY lens's attempt must surface loudly and prevent the round from looking clean/approvable -- even when OTHER lenses produced proposals. A lens that emitted no valid block makes the round unparsed (or otherwise non-approvable), never silently partial.

5. Per-lens corrective retry with escalation (the PRIMARY recovery path, not just loud-fail): when a lens attempt yields no valid block, give the reviewer a corrective signal and let it retry, bounded by a small cap (1-2). Prefer a same-session follow-up turn if the ACP reviewer session supports a second turn ("your previous response did not include the required block; emit exactly one pactum.reviewer_findings.v1alpha1 block now, findings: [] if none; prose is ignored"); otherwise re-run the attempt with the hardened format instruction. Only after the bounded retries still yield no valid block does the round escalate to a loud reviewer_findings_unparsed terminal stop. The retry trigger is STRUCTURAL (no valid block parsed), never a prose heuristic, so a genuinely-clean reviewer -- which emits "findings": [] -- is never re-prompted (this is why forcing the format matters: it removes any need to inspect prose). The retry lives at the reviewer-attempt layer, below the outer loop's Max/Patience, so loop round accounting is unaffected.

Invariant: a round counts as clean if and only if every reviewer lens emitted a valid block whose findings array is empty -- never because zero proposals were extracted from the output.

In scope: the prompt change, the struct/presence change, the parser warning change, the per-lens discriminator change, the bounded corrective-retry-then-escalate mechanism, and focused Go tests.

Tests must cover the exact bug: a valid "findings": [] block => clean round, no warning; a clean reviewer that writes residual-risk prose AND emits "findings": [] => still clean, no false stop; a prose-only attempt with no block => corrective retry, then on persistent miss => reviewer_findings_unparsed loud stop; a retry that succeeds on the second attempt => findings captured, no stop; a mixed round (one lens emits a valid block with findings, another lens emits no block) => the missing-block lens is surfaced loudly and the round is not silently partial; a block carrying the schema but no findings key => parse-miss, not clean.

Out of scope: routing or defaulting the reviewer role to a more reliable emitter (Claude) where a cross-model reviewer exists -- that is a separate later slice. Parsing prose findings into proposals (rejected by design as unsafe given proposals auto-accept to the fixer). Changing the fixer or the proposal auto-accept path.

Validation: go test ./internal/app -run Review, go test ./..., go build ./..., make check.
- In scope:
  - Harden `renderReviewerPrompt` so every reviewer lens prompt makes the fenced `pactum.reviewer_findings.v1alpha1` JSON block mandatory, always emitted, and the only parsed reporting channel.
  - Include a worked clean reviewer-output example with `"findings": []` and state that prose is supplemental only and ignored by the parser.
  - Change reviewer finding block parsing so an explicit empty `findings` array is distinguishable from a missing `findings` key.
  - Treat no valid reviewer findings block, malformed block JSON, or a schema block missing `findings` as a structural parse miss with a warning.
  - Enforce the valid-block requirement per reviewer lens attempt, including mixed rounds where some lenses emit proposals and another lens emits no valid block.
  - Add a bounded per-lens corrective retry for missing or invalid findings blocks before escalating to `reviewer_findings_unparsed`.
  - Keep corrective retry accounting below the outer review-loop round accounting so `MaxRounds`, patience, and clean-round logic still count logical review rounds.
  - Update focused Go tests and review-loop helper fixtures so clean reviewer output emits an explicit `"findings": []` block.
  - Harden the parser against structurally ambiguous reviewer output: more than one fenced pactum.reviewer_findings.v1alpha1 block in an attempt is a parse miss; per-finding field validation excludes entries missing required fields (message/severity/category) with a warning while still capturing valid sibling findings in the same block.
- Out of scope:
  - Changing the contract goal.
  - Routing or defaulting review to a different reviewer agent or model.
  - Parsing prose findings into proposals.
  - Changing fixer behavior, gate behavior, or proposal auto-accept semantics beyond preventing silently partial review rounds.
  - Running real reviewer or fixer agents as part of validation.
- Acceptance criteria:
  - Generated reviewer prompts no longer describe structured finding proposals as optional and require exactly one fenced pactum.reviewer_findings.v1alpha1 block for both finding and no-finding outcomes. A focused Go test asserts that the text returned by renderReviewerPrompt (a) uses mandatory language for the fenced pactum.reviewer_findings.v1alpha1 block, (b) states that prose is supplemental-only and ignored by the parser, and (c) includes a worked example with "findings": []; this test must pass under go test ./internal/app -run Review.
  - A reviewer output containing residual-risk prose plus a valid fenced block with "findings": [] is treated as clean: zero proposals, zero parse warnings, and no corrective retry.
  - A prose-only reviewer attempt with no valid findings block triggers a corrective retry. The run terminates with terminal_reason set to reviewer_findings_unparsed when every corrective retry yields either (a) no valid findings block at all, or (b) a valid findings block with "findings": [] — an empty block on a fresh corrective retry is unverifiable and escalates identically to a missing block. These are the only two structural conditions that cause the corrective retry path to escalate to termination; a valid non-empty retry block is a successful recovery and does not trigger reviewer_findings_unparsed.
  - A reviewer attempt whose fenced block has the correct schema but omits the findings key is a parse miss, not a clean empty review. A block whose findings key is present but set to null or to a non-array value (for example a string, number, or object) is also a parse miss; both cases trigger the corrective retry path identically to a missing findings key and emit a warning with lens=<name> and attempt=<n> tokens. A focused Go test covers the null and non-array cases and asserts that each is a parse miss, not a clean round, and that the warning tokens are present. Array entries within a structurally valid findings array that are not JSON objects are treated as field-invalid entries: each non-object entry is excluded from captured proposals, a warning is emitted identifying the entry index and the structural error, the containing block is not treated as a parse miss, no corrective retry is triggered, and other valid object entries in the same block are still captured and produce proposals normally; a focused Go test covers a findings array containing at least one non-object entry alongside a valid finding object and asserts that only the valid object entry produces a proposal.
  - A reviewer attempt whose fenced block contains the correct schema string but is not valid JSON (malformed block JSON) is a parse miss: it triggers a corrective retry, emits a warning with lens=<name> and attempt=<n> tokens, and if every retry also yields a malformed or missing block, the run terminates loudly with terminal_reason set to reviewer_findings_unparsed; a focused test covers this scenario and asserts the lens and attempt tokens are present in the warning.
  - If a reviewer attempt emits more than one fenced pactum.reviewer_findings.v1alpha1 block — regardless of whether any individual block is valid JSON or contains a valid findings key — the attempt is treated as a parse miss (malformed output). A corrective retry is triggered; the corrective prompt states that exactly one block is required. The warning emitted for the corrective retry includes lens=<name> and attempt=<n> tokens. If every corrective retry also yields multiple blocks or fails to yield exactly one valid block, the run terminates with terminal_reason set to reviewer_findings_unparsed. A focused Go test covers this scenario and asserts that the presence of multiple blocks triggers the corrective retry path and that the round is neither clean nor silently partial; this test must pass under go test ./internal/app -run Review.
  - A reviewer attempt with a valid block and one or more valid findings still creates review proposals from those findings.
  - If a first reviewer attempt has no valid block and the bounded retry emits a valid block with findings, those findings are captured and the run does not stop as reviewer_findings_unparsed because of the first attempt.
  - If one lens emits valid findings and another lens persistently emits no valid block after bounded retries, the round terminates with terminal_reason set to reviewer_findings_unparsed; the already-captured findings from the valid lens are not written to findings.jsonl, and the round is not approvable; a focused test covers this scenario and asserts that terminal_reason is reviewer_findings_unparsed and that the valid lens's findings do not appear in findings.jsonl.
  - A clean round is counted only when every reviewer lens in the round ultimately produces a valid pactum.reviewer_findings.v1alpha1 findings block — after any applicable corrective retries — and every such block's findings array is empty. A lens attempt is 'successful' if and only if it completes without transport error and returns a non-empty response (even if that response requires a corrective retry to yield a valid block). A lens attempt that fails with a transport error, crashes, or produces empty stdout is a hard failure that is not eligible for the corrective retry path; any such failure causes the round to terminate immediately with terminal_reason set to reviewer_findings_unparsed and prevents the round from being counted as clean. A lens that is successful but exhausts its corrective retries without producing a valid findings block also prevents the round from being counted as clean and makes the round non-approvable.
  - Warnings for unparsed reviewer output include the reviewer lens name and attempt index as identifiable fields or string tokens (e.g., lens=<name> and attempt=<n> in structured log output or inline in the warning message); tests assert the presence of these tokens for each warn-triggering scenario covered by the test suite.
  - The test suite includes at least one test that explicitly exercises proposal field validation: (a) a finding containing the required fields (message, severity, and category) is accepted and produces a valid proposal; (b) a finding missing at least one of these required fields (message, severity, or category) is rejected: it is excluded from the captured proposals, a warning is emitted that identifies the finding index and the missing field(s), the containing block is not treated as a parse miss, and no corrective retry is triggered — other valid findings in the same block are still captured and produce proposals normally; (c) the remaining fields (file path, blocking, confidence, and evidence) are optional and their absence does not reject a finding; these tests must pass under go test ./internal/app -run Review.
  - The corrective retry mechanism uses a fresh reviewer attempt in all cases, because the ACP transport (driveACPSession) is strictly single-turn and same-session follow-up is architecturally unavailable for this slice; no capability detection is performed. When a corrective retry is launched, the system emits a warning indicating that findings expressed only in prose during the first attempt may not be recoverable in the fresh attempt. If the fresh attempt returns a valid block with a non-empty findings array, those findings are captured and the run proceeds normally. If the fresh attempt returns a valid block with "findings": [], the result is unverifiable — an empty fresh-retry block cannot be distinguished from a lost-finding scenario — so the run must terminate with terminal_reason=reviewer_findings_unparsed rather than treating the round as clean. A focused test covers this entire path: (i) a lens produces no valid block on the first attempt; (ii) the corrective retry is a fresh reviewer attempt and the warning is present; (iii) when the fresh retry returns a non-empty findings array, those findings are captured and the run proceeds normally; (iv) when the fresh retry instead returns "findings": [], the run terminates with terminal_reason=reviewer_findings_unparsed; this test must pass under go test ./internal/app -run Review.
  - Corrective retries within a single lens attempt do not increment the outer review-loop round counter. MaxRounds, patience, and clean-round logic account only for logical review rounds; one logical round corresponds to one outer-loop iteration, regardless of how many intra-attempt corrective retries occurred for any lens within that round. A focused Go test exercises a review run in which the first lens attempt lacks a valid findings block and a corrective retry emits a valid findings block, and verifies that the review loop records exactly one logical round, not two; this test must pass under go test ./internal/app -run Review.
  - A valid findings block with a non-empty findings array in which every entry fails field validation (each finding is missing at least one required field) produces zero captured proposals. This outcome is neither clean — the findings array was non-empty — nor a parse miss — the block was structurally valid — and triggers no corrective retry. For loop disposition, the round is treated as having produced no actionable proposals; patience accounting applies as if zero proposals were captured, and the run may terminate via patience exhaustion or MaxRounds if the reviewer persistently emits only field-invalid findings. A warning is emitted for each field-rejected finding as specified in the field-validation criterion. A focused Go test covers this scenario and verifies: the round is not counted as clean, no corrective retry is triggered, and warnings are emitted for each rejected finding; this test must pass under go test ./internal/app -run Review.
  - When a corrective retry returns a valid block with a non-empty findings array, those findings are captured and treated as the authoritative output for that lens. Because the fresh reviewer has no memory of the first attempt's prose, findings that appeared only in the first attempt's prose and were not re-expressed in the retry's structured block are permanently unrecoverable. This partial-recovery risk is a known design limitation: the run does not emit an additional warning, halt, or treat partial recovery as an error; the retry's non-empty captured findings are accepted as-is and the run proceeds normally.
- Validation commands:
  - go test ./internal/app -run Review
  - go test ./...
  - go build ./...
  - make check

## Current review findings
- Summary: findings=10 open=10 resolved=0 blocking_open=10
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=high category=correctness blocking=true status=open: A reviewer lens transport failure can still be counted as a clean round when the remaining lenses emit valid empty findings blocks.
    location: internal/app/review_loop.go:637
  - f_002 severity=high category=correctness blocking=true status=open: The corrective retry prompt omits the reviewer context and artifact paths, so a fresh retry has no basis to re-review the task.
    location: internal/app/review.go:1271
  - f_003 severity=medium category=correctness blocking=true status=open: The multiple-block parse-miss check ignores malformed or missing-findings schema blocks when one sibling block is valid.
    location: internal/app/review_proposals.go:443
  - f_004 severity=high category=quality blocking=true status=open: The parser test still encodes the old silent-drop behavior for prose-only reviewer output.
    location: internal/app/review_proposals_test.go:57
  - f_005 severity=high category=quality blocking=true status=open: The review-loop tests do not exercise the new corrective retry path for successful reviewer attempts with structurally invalid output.
    location: internal/app/review_loop_test.go:1902
  - f_006 severity=medium category=quality blocking=true status=open: The prompt contract test does not assert the mandatory-output requirements added by this change.
    location: internal/app/review_test.go:2207
  - f_007 severity=medium category=quality blocking=true status=open: Proposal field-validation tests do not explicitly cover the required-field rejection contract.
    location: internal/app/review_test.go:1855
  - f_008 severity=high category=correctness blocking=true status=open: Mixed reviewer rounds can still proceed after a per-lens parse miss when another lens produced proposals.
    location: internal/app/review_loop.go:267
  - f_009 severity=medium category=quality blocking=true status=open: The generated reviewer prompt still describes structured proposal blocks as optional, contradicting the new mandatory findings-block contract.
    location: internal/app/review.go:1161
  - f_010 severity=medium category=quality blocking=true status=open: The operator documentation still says reviewer structured findings are optional, but review runs now require a valid findings block and retry or escalate when it is missing.
    location: docs/agents.md:545
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
