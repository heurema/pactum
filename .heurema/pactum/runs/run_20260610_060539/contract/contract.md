# Contract Draft

## Goal
Close the two gaps that block making ACP the default transport. (1) Model pins never reach ACP: ApplyModelSpec puts model/effort into the agent CLI args, but ACPTransport only uses Agent.Name to pick the npx adapter and ignores Args — so the per-agent pins from execute.models/review.panel are silently dropped over ACP. (2) Read-only stages over ACP have no guard: only execute/review-fix populate WritePathAllowed (nil = allow all), and acpClient.RequestPermission auto-approves everything — so a reviewer/clarifier/drafter over ACP could write to the repo, weaker than the CLI transport where codex runs in a --sandbox read-only. Carry the pins and a read-only flag through RunRequest: codex-acp accepts the same -c overrides as the codex CLI (-c model=..., -c model_reasoning_effort=...; verified via its --help); claude-agent-acp launches Claude Code, which honors the ANTHROPIC_MODEL and CLAUDE_CODE_EFFORT_LEVEL env vars for the launched session (per Claude Code model-config docs). On read-only stages the ACP client must deny WriteTextFile and deny permission requests instead of auto-approving.

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260610_060539
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

## In scope
- internal/agents/types.go: add Model agents-package ModelSpec field and ReadOnly bool to RunRequest. The CLI transport ignores both (model is already applied to the CLI args via ApplyModelSpec; read-only is already in the reviewer descriptors).
- internal/agents/acp_transport.go: thread the pin to the adapter per agent — for codex append '-c model=<model in TOML-quoted form>' and '-c model_reasoning_effort=<effort>' to the codex-acp adapter args (same forms ApplyModelSpec uses for the codex CLI); for claude set ANTHROPIC_MODEL=<model> and CLAUDE_CODE_EFFORT_LEVEL=<effort> in the adapter subprocess env (cmd.Env). Empty model/effort add nothing. Add/extract a small helper so the mapping is unit-testable without spawning the adapter.
- internal/agents/acp_transport.go acpClient: honor ReadOnly — (a) WriteTextFile returns a clear 'acp write denied: read-only stage' error before touching disk, regardless of WritePathAllowed; (b) RequestPermission, instead of auto-approving, selects a reject/deny option when one exists and otherwise returns the cancelled outcome, so the agent's write/exec tool calls are refused; (c) keep ReadTextFile working. Advertise the same client capabilities as today (do not drop the write capability — the agent must route writes through the client where they are denied, not fall back to native writes).
- internal/app: populate the new RunRequest fields at the agent-attempt lifecycle call sites — execute/review-fix keep ReadOnly=false and existing WritePathAllowed; review (single + loop), clarify suggest, and contract draft set ReadOnly=true; every site passes its already-resolved ModelSpec (the same one shown in the Resolved block) so the pin reaches ACP.
- Unit tests: the adapter command/env mapping for codex (both -c overrides, TOML-quoted model) and claude (both env vars), including empty-spec adds nothing; acpClient read-only behavior — WriteTextFile denied with the read-only error and no file created, RequestPermission returns reject/cancelled (and the existing allow-path still works when ReadOnly is false); app-level: read stages set ReadOnly and pass the model spec, write stages do not set ReadOnly.
- Docs: docs/agents.md ACP section — model pins now reach ACP (how: codex -c overrides, claude env) and read-only stages deny writes + permission requests over ACP; update the transport paragraph's 'remaining gaps' wording (the two named gaps are closed; the shell-command gating limitation for WRITE stages remains and stays documented); docs/backlog.md: update the ACP shell-gating item to reflect the narrowed remaining surface and note the M16.1 closure of the two gaps.

## Out of scope
- Do not flip the transport default (stays cli; PACTUM_AGENT_TRANSPORT env only — that is M16.2); do not implement shell-command/tool-call scope gating for WRITE stages (documented limitation); do not change the CLI transport behavior, ApplyModelSpec, the config schema, or stage selection logic.

## Paths in scope
- internal/agents/*.go
- internal/app/*.go
- docs/agents.md
- docs/backlog.md


## Acceptance criteria
- Over ACP, a codex run with a model/effort pin launches codex-acp with -c model=... and -c model_reasoning_effort=...; a claude run launches claude-agent-acp with ANTHROPIC_MODEL and CLAUDE_CODE_EFFORT_LEVEL set; unpinned runs add neither (covered by unit tests on the mapping helper).
- Over ACP, read-only stages (review, clarify suggest, contract draft) deny WriteTextFile with a clear read-only error and refuse permission requests; write stages keep today's behavior (scope-guarded writes, auto-approve); covered by tests at both the agents and app layers.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes; docs describe the closed gaps and the remaining write-stage shell limitation.

## Validation commands
- go build ./...
- go test ./internal/agents/... ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- codex-acp accepts codex CLI -c config overrides (verified via npx codex-acp --help); claude-agent-acp launches Claude Code, which honors ANTHROPIC_MODEL and CLAUDE_CODE_EFFORT_LEVEL for the launched session (per the Claude Code model-config documentation; the env var takes precedence over other effort sources).
- Keeping the write capability advertised while denying each write/permission is the correct read-only mechanism: dropping the capability could push the agent to native writes that bypass the client entirely.

## Open questions
- None
