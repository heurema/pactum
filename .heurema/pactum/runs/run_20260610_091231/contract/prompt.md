# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260610_091231
- Approval: approved
- Contract hash: 82cac496795a7cc7f40a6276c5fab0ccdf6f89d75e9a6bd6a8aa8112b0198e32

## Goal
Introduce an agent registry as the config's source of truth for agents. A required top-level 'agents' list replaces the per-stage agent/model entries: each entry is {name (how it is referenced), agent (the built-in it runs on, claude|codex, defaulting to name), model, effort (optional pins)}. Everything references registry names: review.panel becomes a plain list of names, and --agent/--reviewer accept names. The execute config section disappears — a name carries its agent+model+effort wherever it is used, so pins stop duplicating across stages, and a name is decoupled from the built-in (two entries can run the same agent with different models; a panel may hold two claude-backed entries, which the old duplicate-agent rule forbade). The default config generates agents: [{name: claude}]; an empty or missing registry is a loud error because at least one agent must be registered.

## In scope
- Config schema (internal/app/config.go): add the required top-level agents list (entry fields: name, agent, model, effort; yaml omitempty for the optional three); delete the execute section (executeConfig and its models); review.panel becomes []string of registry names. Strict parsing stays. Validation at readConfig: registry non-empty ('config agents: at least one agent must be registered'), names non-empty/unique, entry.agent (after defaulting to name) must be a built-in, model must not contain ':' (effort is its own key); panel names must reference registered names (error naming the bad reference). writeDefaultConfigIfMissing generates the registry with the single claude entry.
- Resolution layer (internal/app): a helper resolving a registry name + role -> (built-in descriptor for that role via the agents package, ModelSpec from the entry) — the role still picks the executor vs read-only reviewer descriptor variant; the entry's model/effort feed ApplyModelSpec and RunRequest.Model exactly as today. The --agent and --reviewer values resolve ONLY against the registry (an unregistered name, including a bare built-in name that is not registered, is a clear error). An omitted --agent defaults to the FIRST registry entry (replacing the hardcoded codex default for executor selection at the app layer); an omitted --reviewer keeps cross-model semantics against UNDERLYING agents: pick the first registry entry whose underlying agent differs from the run executor's underlying agent, else fall back to the first registry entry (the Resolved block already shows what was picked). The review panel runs the named entries, each with its own pins — duplicate NAMES are invalid but two names backed by the same built-in are allowed and run as separate panel members.
- Stage rewiring: execute/review-fix lose the execute.models lookup (the resolved entry's ModelSpec travels instead); review single+loop take roster AND pins from the named registry entries; clarify suggest / contract draft resolve their reviewer name (explicit or cross-model rule above) through the registry. The usage ledger record gains an agent_name field (the registry name) alongside the existing agent (underlying built-in) and request_model; execution/attempt records keep recording the underlying built-in so cross-model comparison semantics are unchanged.
- CLI: the agent and reviewer flag help texts say registry name; no new flags. Tests: registry validation (empty registry, duplicate/blank names, unknown built-in in entry.agent, colon-in-model, panel referencing unregistered name); name resolution (default executor = first entry; explicit registry name; unregistered name errors); two entries on the same built-in with different models resolve to different pins (and may sit in one panel together); cross-model picks the first different-underlying entry and falls back to the first entry; usage record carries agent_name; default-config generation and round-trip through the strict reader. Update existing tests from the old shape.
- The workspace .heurema/pactum/config.yaml ships in this change atomically (the strict parser rejects the old shape): registry entries [claude (unpinned), fable (agent claude, model claude-fable-5), codex] with review.panel: [fable, codex] — preserving today's behavior (unpinned claude executor as the default first entry, fable-5 claude review, codex review unpinned) while exercising the same-built-in-twice feature. Docs: agents.md (registry section with the example, name resolution rules, defaults, cross-model under the registry), backlog (mark the registry item shipped; note the agents-doctor registry view as a small follow-up), README config example if it shows the old shape.

## Out of scope
- agents doctor stays as-is (a registry-aware doctor view is a recorded follow-up); no custom agents (entry.agent limited to built-ins; command/args fields are future work); no changes to transports, the ACP legs, gate, or loop semantics; no backward compatibility for the old execute/panel shapes (strict parsing rejects them and the workspace file is rewritten in this change).

## Paths in scope
- internal/agents/*.go
- internal/app/*.go
- docs/*.md
- README.md


## Acceptance criteria
- The config has a required agents registry validated as specified; the execute section is gone; review.panel is a list of registry names validated against the registry; the default config generates the single-claude registry and round-trips the strict reader.
- --agent/--reviewer resolve only registry names (clear error otherwise); omitted --agent uses the first registry entry; omitted --reviewer applies the cross-model rule against underlying agents with the first-entry fallback; panel members each run with their entry's pins, including two entries backed by the same built-in; the usage ledger records agent_name alongside agent.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes; the workspace config.yaml in the new shape parses and preserves today's pins (unpinned claude default executor, fable-5 claude review, codex review).

## Validation commands
- go build ./...
- go test ./internal/agents/... ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- The registry is the single source of truth: bare built-in names are not implicitly available — referencing an unregistered name errors, and the generated default (claude) keeps the out-of-box experience working.
- The first registry entry is the natural default executor (replacing the hardcoded codex default at the app layer), and cross-model independence is judged by UNDERLYING agents, not registry names.

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
