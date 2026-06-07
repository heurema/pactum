# Contract Draft

## Goal
Fix two correctness bugs in the autonomous review->fix loop (internal/app/review_loop.go and the proposal/finding accept path). BUG 1: the per-round summary 'open_findings' field misreports — it is set to the per-round proposals_accepted count instead of the live count of currently-open findings. BUG 2: when an unfixed finding is re-proposed in a later round, the loop accepts it as a brand-new finding (new finding id), inflating findings.jsonl and the ledger with duplicates. Dedup re-proposed findings against currently-open findings so a standing issue stays a single open finding

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260606_222441
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

## In scope
- open_findings: set the round summary's open_findings to the live count of currently-open findings (already computed via reviewLoopTotalOpenFindings). Remove the now-redundant total_open_findings struct field + JSON key — it holds the same live count, is never rendered, and would be identical after the fix
- Dedup: in ReviewLoop, BEFORE auto-accepting a round's proposals, skip any proposal whose (file, line, message) matches a currently-OPEN finding (accepted and not resolved). Instead of creating a duplicate finding, record a proposal decision with decision "duplicate" linked to the existing finding id, emitting a single ledger event. Apply dedup within a single round too (two identical proposals in one round yield one finding)
- Add a finding fingerprint helper over the (file, line, message) tuple (normalize nothing else); reuse it for the dedup comparison
- Tests: (a) a re-proposed open finding in a later round is deduped — no new finding record, findings count stable, a 'duplicate' decision recorded; (b) a genuinely NEW finding in a later round is still accepted; (c) open_findings equals the live open count across rounds (a round with 0 new accepts but 1 still open reports open_findings=1); (d) a finding whose original was RESOLVED, re-proposed later, is NOT suppressed (it is re-accepted). Update existing review_loop tests to the corrected open_findings semantics and removed total_open_findings field

## Out of scope
- Semantic or fuzzy dedup (reworded messages, line-number drift across rounds). Use exact (file,line,message) match only and document this limitation in a code comment and docs/backlog.md
- Rebuttal channel; auto-marking findings resolved when the fixer fixes them; cost/budget stop (separate backlog items)
- Changing the manual 'review accept-proposal' command behavior — dedup is confined to the autonomous loop driver
- Native LLM API or provider abstraction; editing generated .heurema run artifacts

## Acceptance criteria
- open_findings reports the live open-finding count each round and the human round-summary line is correct; total_open_findings field/JSON key is removed
- A standing unfixed finding stays a single open finding across rounds: no duplicate finding records in findings.jsonl, no duplicate accepted/finding_added ledger events; a 'duplicate' decision is recorded instead
- Genuinely new findings are still accepted; a resolved-then-re-proposed finding is re-accepted. Dedup NEVER suppresses a finding that is not currently open (no silent suppression of real findings)
- make check is green (includes the deadcode gate); go test ./... passes

## Validation commands
- make check

## Assumptions
TBD

## Open questions
- None
