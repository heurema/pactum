# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260610_151132
- Approval: approved
- Contract hash: 257ba55c10402c4a47eb324fc1f95096c79cfec0a7fd5eabe0dd5fc51a206b5f

## Goal
Simplify the agent registry entry to {name, model, effort}. The agent field is removed: the underlying engine is inferred SOLELY from the model, which therefore becomes required on every entry. The name is a free reference (path-safe, unique) used by --agent/--reviewer/review.panel exactly as today. Inference rules live in the agents package: a model starting with claude, or equal to one of the claude aliases (opus, sonnet, haiku, fable), runs on claude; a model starting with gpt or codex, or matching o<digit> (o3, o4-mini, ...), runs on codex; anything else fails loudly at config read with an error naming the entry and the recognized forms. The at-least-one-agent rule stays. The workspace config.yaml ships atomically in the new shape.

## In scope
- internal/agents: an exported inference helper (e.g. InferAgentFromModel(model string) (string, bool)) with the rule table above (trimmed, case-insensitive prefixes; the alias set exact-match case-insensitive), unit-tested for each rule and for unknowns.
- internal/app config: agentRegistryEntry loses Agent; model is required (empty model -> error naming the entry: the engine is inferred from the model); validation infers and caches/normalizes nothing extra — the resolution layer calls the inference where it needs the engine. Keep: ≥1 entry, unique path-safe names, no ':' in model (effort separate). Update writeDefaultConfigIfMissing: the generated default registers a single claude entry whose model is a sensible pinned claude model (use the claude-opus-4-8 id) so the default config round-trips inference.
- Resolution layer (agent_resolve.go): the engine for a registry entry comes from InferAgentFromModel; resolveAgentForRole passes the inferred engine to ResolveExecutor/ResolveReviewer; cross-model reviewer default compares INFERRED engines (first entry whose engine differs from the run executor's engine, else first entry); everything else (first-entry executor default, registry-only names, lens fan-out, per-member artifacts, usage agent=engine + agent_name=registry name) unchanged.
- The workspace .heurema/pactum/config.yaml ships atomically: agents [fable (model claude-fable-5), codex (model gpt-5.5, effort high — matching the local codex config)] with review.panel [fable, codex]; the default executor becomes the first entry (fable), matching the operator's current claude CLI default model.
- Tests: inference table (claude prefix, each alias, gpt/codex prefixes, o3/o4-style, unknown -> false); config validation (missing model error names the entry; unknown-model-form error; agent field now an unknown key rejected by strict parsing — pin that an old-shape config with agent: fails loudly); resolution uses the inferred engine (a free-named claude-model entry resolves claude descriptors; cross-model picks the different-engine entry); update existing tests from the old shape. Docs: agents.md registry section rewritten for {name, model, effort} + inference rules; backlog notes the simplification.

## Out of scope
- No changes to the lens fan-out, prompts, transports, ledger schema, or panel semantics beyond engine inference; no model-existence validation (a wrong-but-recognizable model still fails at the provider, as today); no compat for the removed agent field (strict parsing rejects it; the workspace file is rewritten here).

## Paths in scope
- internal/agents/*.go
- internal/app/*.go
- docs/*.md


## Acceptance criteria
- agentRegistryEntry is {name, model, effort} with model required; the engine is inferred solely from the model via the agents-package helper with the documented rules; unknown forms and missing models fail loudly naming the entry; an old-shape config carrying agent: is rejected by strict parsing.
- Cross-model and all stage resolution work off inferred engines; the default config generates a single claude entry with a pinned model and round-trips; the workspace config.yaml in the new shape parses and the first entry (fable) is the default executor.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes; docs describe the new entry shape and inference rules.

## Validation commands
- go build ./...
- go test ./internal/agents/... ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- Inferring the engine from the model is reliable because the two vendors' model families are prefix-distinct (claude-*/aliases vs gpt-*/codex-*/o<digit>), and a failed inference is a loud config error rather than a guess.
- Requiring model on every entry is the deliberate consequence of model-only inference: an inherit-the-CLI-default entry can no longer exist, and the workspace pins fable/gpt-5.5 to match the operator's current CLI defaults.

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
