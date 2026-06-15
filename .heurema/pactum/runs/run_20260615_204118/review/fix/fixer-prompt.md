# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260615_204118/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260615_204118/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260615_204118/review/review.json, .heurema/pactum/runs/run_20260615_204118/review/findings.jsonl, .heurema/pactum/runs/run_20260615_204118/review/resolutions.jsonl

## Approved contract
- Goal: Route all claude agent invocation through the ACP transport and remove the 'claude -p' CLI path. Today the built-in claude descriptor in internal/agents/config.go runs a CLI process (claude -p --output-format json --dangerously-skip-permissions) for the executor, while reviewers already run claude over ACP via internal/agents/acp_transport.go. This split captures token usage two different ways (CLI parseClaudeUsage over stdout vs ACP PromptResponse.Usage) and is the root cause of usage not being recorded consistently. Make claude always use the ACP adapter (the same transport the reviewers use), remove the claude -p CLI descriptor and its args, and remove the now-dead CLI claude usage parser (parseClaudeUsage and its dispatch in internal/agents/usage.go) so claude usage comes only from ACP PromptResponse.Usage. Requirements: (1) preserve model and effort pinning (today --model/--effort; over ACP these map to the CLAUDE_CODE_* session env already handled in acp_transport.go); (2) the executor and review fixer must RETAIN file-edit capability over ACP (the CLI path used --dangerously-skip-permissions; ensure the ACP session grants write/edit for write-stage roles, not read-only); (3) keep codex transport unchanged in THIS slice (codex-over-ACP unification is a tracked follow-up); (4) add or update tests for the claude ACP path and usage capture. Do not break the executor, reviewer, drafter, or fixer claude paths.
- In scope:
  - Route every resolved claude attempt for execute, review, clarify, contract draft, and review fix through the ACP transport; the CLI transport must not launch a claude subprocess.
  - Remove the built-in claude CLI descriptor and any claude-specific `-p`, `--output-format json`, `--model`, `--effort`, or `--dangerously-skip-permissions` argument construction.
  - Preserve claude model and effort pinning through the ACP adapter environment mapping.
  - Preserve claude write capability for execute and review fix over ACP while keeping review, clarify, and contract draft read-only.
  - Remove the dead claude stdout usage parser and dispatch so claude usage is captured only from ACP `PromptResponse.Usage`.
  - Add or update tests for claude ACP routing, model and effort pinning, read-only versus write-stage behavior, and ACP usage capture.
- Out of scope:
  - Unifying or redesigning codex transport behavior.
  - Removing codex CLI usage parsing or changing codex usage accounting.
  - Running real claude, codex, npx, or ACP adapter subprocesses as part of validation.
  - Migrating or recomputing historical usage records.
  - Changing the contract goal or answering clarification questions.
- Acceptance criteria:
  - No production code path can execute `claude -p` for a built-in claude agent; attempts either use ACP or fail before launching a claude CLI subprocess.
  - Built-in claude executor and reviewer descriptors no longer contain CLI-only claude flags such as `-p`, `--output-format`, `--model`, `--effort`, or `--dangerously-skip-permissions`.
  - A claude model spec such as model `claude-sonnet-4` and effort `high` reaches the ACP adapter as `ANTHROPIC_MODEL=claude-sonnet-4` and `CLAUDE_CODE_EFFORT_LEVEL=high`, not as CLI args.
  - Claude execute and review-fix attempts pass `ReadOnly=false` and a non-nil write-scope predicate to the ACP transport; claude review, clarify, and contract-draft attempts pass `ReadOnly=true`.
  - ACP write-stage clients allow scoped `WriteTextFile` operations and auto-select an allow permission option; ACP read-only clients deny writes and reject or cancel permission requests.
  - `parseClaudeUsage` and the claude branch of CLI stdout usage parsing are removed; malformed or missing claude CLI stdout is no longer part of usage capture behavior.
  - Claude ACP `PromptResponse.Usage` is normalized into captured `TokenUsage`, including cache read/write tokens in input totals and thought/reasoning tokens in output totals.
  - Existing codex descriptors, codex model/effort handling, codex transport selection, and codex usage parsing continue to pass their existing tests.
- Validation commands:
  - go test ./internal/agents ./internal/app
  - go test ./...
  - make check

## Current review findings
- Summary: findings=2 open=2 resolved=0 blocking_open=2
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=medium category=quality blocking=true status=open: The review-fix write-stage transport test only runs codex, leaving the required claude ACP review-fix path untested for ReadOnly=false, non-nil WritePathAllowed, model/effort propagation, and no CLI args.
    location: internal/app/agent_attempt_transport_test.go:114
  - f_002 severity=medium category=quality blocking=true status=open: User-facing docs still document the removed Claude CLI transport path. The code now makes the built-in Claude descriptor ACP-only with no CLI command or args, but docs still say Claude runs via `claude -p` under CLI transport.
    location: docs/agents.md:11
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
