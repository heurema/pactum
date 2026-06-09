# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260608_223329
- Approval: approved
- Contract hash: 42642263f293ae3eab5f6ee6ec566a08d757fc73458c5d1600842aeee03367c3

## Goal
Harden the ACP agent transport: bring its token-usage normalization to parity with the OTel-inclusive convention the CLI parsers use, and make the process-group cleanup cross-platform with build tags. Two small, focused fixes plus docs; no behavior change to the CLI transport or the ACP session flow.

## In scope
- Usage normalization parity (internal/agents/acp_transport.go, acpClient.tokenUsage): map PromptResponse.Usage to the OTel-INCLUSIVE convention documented in docs/cost-budget-design.md and used by parseClaudeUsage/parseCodexUsage — InputTokens = u.InputTokens + cache_read + cache_write (input includes cache); OutputTokens = u.OutputTokens + thought (output includes reasoning); ReasoningTokens = thought; TotalTokens = u.TotalTokens; CacheReadTokens = u.CachedReadTokens; CacheCreationTokens = u.CachedWriteTokens. Keep Captured=true when usage is present, the no-usage warning path unchanged. Today the mapping omits cache from input and reasoning from output, under-counting vs the CLI path.
- Cross-platform process-group cleanup: extract the Unix-only syscall.SysProcAttr{Setpgid:true} setup and the syscall.Kill(-pid, SIGKILL) group-kill out of acp_transport.go into build-tagged helpers — e.g. setProcessGroup(cmd) and killProcessGroup(cmd) in acp_transport_unix.go (//go:build unix) doing the process-group behavior, and acp_transport_other.go (//go:build !unix) with a no-op setProcessGroup and a killProcessGroup fallback that uses cmd.Process.Kill + Wait. acp_transport.go calls the helpers and no longer references syscall directly, so the package compiles on every GOOS while keeping the process-tree reaping on Unix.
- Tests: update the acpClient usage mapping test to assert the OTel-inclusive result (input includes cache, output includes reasoning, total preserved). Keep the package compiling and the existing ACP tests passing.
- Docs: in docs/agents.md, note the ACP usage normalization (OTel-inclusive parity with the CLI path) and reiterate the file-write-boundary scope-guard limitation (shell-command writes bypass it). In docs/backlog.md, record the remaining ACP follow-ups: shell-command / tool-call scope gating (deeper, currently a documented limitation) and an optional consolidated ACP design note.

## Out of scope
- Shell-command or tool-call write gating (deeper; the agent can still write out of scope via a shell command — keep it a documented limitation, do not attempt to solve it here).
- Changing the CLI usage parsers (parseClaudeUsage/parseCodexUsage), the gate, the ACP session driving, or the config/opt-in. Native LLM API.

## Paths in scope
- internal/agents/*.go
- docs/*.md


## Acceptance criteria
- acpClient.tokenUsage returns OTel-inclusive counts (InputTokens includes cache_read+cache_write; OutputTokens includes reasoning), matching the CLI parsers; a unit test asserts this.
- The ACP transport process-group setup and kill live in build-tagged files; acp_transport.go no longer imports syscall directly; the package compiles for unix and non-unix GOOS, preserving the Unix process-tree reaping.
- make check (incl. deadcode + git diff --check) and make test-race pass; docs/agents.md and docs/backlog.md updated.

## Validation commands
- make build
- make check
- make test-race

## Assumptions
- The OTel-inclusive convention (InputTokens includes cache, OutputTokens includes reasoning) is the canonical normalization in docs/cost-budget-design.md, already applied by parseClaudeUsage and parseCodexUsage; mirror it for ACP.
- syscall.SysProcAttr.Setpgid and syscall.Kill are Unix-only; the //go:build unix and //go:build !unix split is the idiomatic way to keep the cross-platform build green.
- No backward-compatibility constraints; the usage numbers becoming cache-inclusive is the intended correction.

## Clarifications
- None

## Project context
- Executor context: context/executor-context.md
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json
- Accepted memory context: context/memory-context.md

## Accepted memory

Memory context:
- context/memory-context.md

Selected memory:
- total: 0
- fresh: 0
- stale: 0
- unknown: 0

Items:
- none

Rules:
- Accepted memory is context, not semantic truth.
- Stale memory may be outdated; verify before using.
- Use `pactum search "<term>"` and inspect current source files before relying on memory.
- Do not implement from memory alone.

## Instructions for future executor
- Follow the approved contract.
- Do not implement out-of-scope work.
- Search before creating new code.
- Prefer existing code items when applicable.
- If the contract is ambiguous, stop and request clarification.
- Use the listed validation commands as expected checks.
- Pactum gate can run approved validation commands after execution.
