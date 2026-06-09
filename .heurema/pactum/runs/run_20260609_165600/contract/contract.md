# Contract Draft

## Goal
Add a plugin system so users can register custom agents beyond the built-in codex and claude

## Current status
Contract status: draft
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260609_163022
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (0 result(s))

## Clarifications
- q_001 [blocking] — What is the registration mechanism for the plugin system — declarative custom-agent descriptors in config, or dynamically loaded code / an adapter SDK?
  Rationale: This is the foundational scope decision; everything else (schema, validation, docs, tests) hinges on it. The repo strongly favors the declarative path: AgentDescriptor{Name,Command,Args,Input} is already generic, BuildCommand (internal/agents/executor.go:22) already runs any descriptor, agents.Registry is an interface with an injection seam on App (app.go:31/229), and the agents: config block already exists (internal/app/config.go:33). The project philosophy is deterministic and no-dynamic-loading (no Docker, no Go plugins, lexical search). A full Go-plugin/.so or external adapter SDK would be a large new subsystem at odds with that. Confirm the lightweight path so this stays a config-driven slice, not a runtime-loading project.
  Answer: pending
- q_002 [blocking] — Should a custom agent be definable for both the executor (write) and reviewer (read-only) roles, or executor-only for the first slice?
  Rationale: Built-ins ship two distinct descriptors per agent: a write-enabled executor (ListBuiltins, write-bypass flags) and a read-only reviewer (reviewerBuiltins, internal/agents/config.go:70, sandbox/no-bypass). ResolveExecutor and ResolveReviewer are separate registry methods, and clarify suggest / contract draft / review all resolve via ResolveReviewer. So the config schema must decide whether a custom entry carries one invocation or two (executor + read-only reviewer). This shapes the YAML shape and how the registry's ResolveExecutor vs ResolveReviewer treat custom names. The repo cannot settle the desired role coverage — it's a product-priority call.
  Answer: pending
- q_003 — How should a custom agent whose name collides with a built-in (`codex` or `claude`) be handled — reject it, or let it override the built-in?
  Rationale: resolveFrom (internal/agents/config.go:91) matches by exact name, and a layered registry must decide precedence when a custom name equals a built-in. Override would let users retune built-in flags (a real use case) but makes the two built-ins no longer stable/guaranteed; reject keeps built-ins authoritative and surfaces config mistakes early. Safe default exists, so non-blocking, but it's a genuine trade-off the repo doesn't decide.
  Answer: pending
- q_004 — Which transport(s) must custom agents support in the first slice — CLI only, or also ACP?
  Rationale: The CLI transport runs any descriptor (prompt piped to stdin), but the ACP transport launches specific external adapters (claude-agent-acp / codex-acp via npx, see docs/agents.md) keyed to the built-in agents. A custom CLI agent has no ACP adapter unless the user also supplies one, which would widen the config schema and the transport-selection logic. There's a strong default (CLI-only first), so non-blocking, but confirm to avoid scope creep into ACP adapter wiring.
  Answer: pending

## In scope
TBD

## Out of scope
TBD

## Acceptance criteria
TBD

## Validation commands
TBD

## Assumptions
TBD

## Open questions
- What is the registration mechanism for the plugin system — declarative custom-agent descriptors in config, or dynamically loaded code / an adapter SDK?
- Should a custom agent be definable for both the executor (write) and reviewer (read-only) roles, or executor-only for the first slice?
- How should a custom agent whose name collides with a built-in (`codex` or `claude`) be handled — reject it, or let it override the built-in?
- Which transport(s) must custom agents support in the first slice — CLI only, or also ACP?
