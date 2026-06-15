# Contract Draft Proposal

## Status
- Run id: run_20260615_204118
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-15T20:44:10Z

## In scope
- Route every resolved claude attempt for execute, review, clarify, contract draft, and review fix through the ACP transport; the CLI transport must not launch a claude subprocess.
- Remove the built-in claude CLI descriptor and any claude-specific `-p`, `--output-format json`, `--model`, `--effort`, or `--dangerously-skip-permissions` argument construction.
- Preserve claude model and effort pinning through the ACP adapter environment mapping.
- Preserve claude write capability for execute and review fix over ACP while keeping review, clarify, and contract draft read-only.
- Remove the dead claude stdout usage parser and dispatch so claude usage is captured only from ACP `PromptResponse.Usage`.
- Add or update tests for claude ACP routing, model and effort pinning, read-only versus write-stage behavior, and ACP usage capture.

## Out of scope
- Unifying or redesigning codex transport behavior.
- Removing codex CLI usage parsing or changing codex usage accounting.
- Running real claude, codex, npx, or ACP adapter subprocesses as part of validation.
- Migrating or recomputing historical usage records.
- Changing the contract goal or answering clarification questions.

## Acceptance criteria
- No production code path can execute `claude -p` for a built-in claude agent; attempts either use ACP or fail before launching a claude CLI subprocess.
- Built-in claude executor and reviewer descriptors no longer contain CLI-only claude flags such as `-p`, `--output-format`, `--model`, `--effort`, or `--dangerously-skip-permissions`.
- A claude model spec such as model `claude-sonnet-4` and effort `high` reaches the ACP adapter as `ANTHROPIC_MODEL=claude-sonnet-4` and `CLAUDE_CODE_EFFORT_LEVEL=high`, not as CLI args.
- Claude execute and review-fix attempts pass `ReadOnly=false` and a non-nil write-scope predicate to the ACP transport; claude review, clarify, and contract-draft attempts pass `ReadOnly=true`.
- ACP write-stage clients allow scoped `WriteTextFile` operations and auto-select an allow permission option; ACP read-only clients deny writes and reject or cancel permission requests.
- `parseClaudeUsage` and the claude branch of CLI stdout usage parsing are removed; malformed or missing claude CLI stdout is no longer part of usage capture behavior.
- Claude ACP `PromptResponse.Usage` is normalized into captured `TokenUsage`, including cache read/write tokens in input totals and thought/reasoning tokens in output totals.
- Existing codex descriptors, codex model/effort handling, codex transport selection, and codex usage parsing continue to pass their existing tests.

## Validation commands
- go test ./internal/agents ./internal/app
- go test ./...
- make check

## Assumptions
- The claude ACP adapter continues to honor `ANTHROPIC_MODEL` and `CLAUDE_CODE_EFFORT_LEVEL` for session pinning.
- Unit tests and fake ACP/transport implementations are sufficient validation for this slice; live authenticated claude execution is intentionally excluded.
- Historical usage ledgers already contain normalized usage records and do not require reparsing old claude CLI stdout logs.
- The existing ACP client is the intended enforcement point for claude read-only and write-stage file behavior.

