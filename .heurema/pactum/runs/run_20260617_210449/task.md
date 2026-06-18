# Task

Make pactum's code-review loop never silently drop reviewer findings, and recover automatically when a reviewer omits the structured findings block before failing loud.

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

Generated: 2026-06-17T21:04:49Z
