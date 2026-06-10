# Contract Draft

## Goal
Redesign the workspace config (.heurema/pactum/config.yaml) into the agreed stage-centric shape. An audit found over half the current keys are dead (read by nothing): default_profile, project_map.refresh, limits.clarify.*, limits.execute.*, budget.max_usd, the entire memory section. Others are misplaced or redundant: review limits live under a generic limits section, budget is enforced only by the review loop but sits top-level, agents.cross_model_review duplicates what an empty panel can express, agents.transport should not be a config knob, and model pins were global strings that broke multi-agent panels. The new shape: schema (value stays pactum.config.v1 — pre-release, no version bump), map {max_file_bytes, code_index}, gate {scope_enforcement}, execute {models: [{agent, model, effort}]}, review {max_rounds, patience, clean_rounds, budget {mode, max_tokens}, panel: [{agent, model, effort}]}. Unknown keys in the file must fail loudly so dead keys can never accumulate silently again.

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260609_221559
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (0 result(s))

## Clarifications
- None

## In scope
- Rewrite the config schema structs (internal/app/config.go): top-level sections exactly schema, map (renamed from project_map; fields max_file_bytes, code_index — refresh dropped), gate (scope_enforcement), execute (models: list), review (max_rounds, patience, clean_rounds, budget {mode, max_tokens}, panel: list). Delete default_profile, the limits section (review limits move into review), the top-level budget section (moves under review.budget, max_usd dropped), the memory section, and the agents section entirely. Update defaultConfigFile and writeDefaultConfigIfMissing to the new shape.
- Introduce ONE shared entry type for execute.models and review.panel items: {agent string (required), model string (optional), effort string (optional)}. Validation, applied when config is read: agent must be a resolvable built-in agent name (clear error naming the bad value); duplicate agent within one list is an error; a model value containing ':' is an error telling the user to use the separate effort key. An entry maps directly to agents.ModelSpec{Model, Effort} for ApplyModelSpec — ParseModelSpec colon-splitting is no longer used for config values.
- Strict parsing: readConfig must reject unknown keys (yaml.v3 decoder with KnownFields(true)) with an error naming the file, so removed/mistyped keys fail loudly instead of sitting dead.
- Rewire the read sites. Executor stages (execute.go, review_fix.go fixer): look the invoked agent up in execute.models; found -> apply its model/effort, absent -> unpinned. Review loop (review_loop.go) and single review (review.go): panel comes from review.panel entries — names AND per-member model/effort from the same entry; an explicit --reviewer name overrides the roster and takes pins from its panel entry when present (absent -> unpinned). Empty/missing panel = cross-model default (single reviewer = the agent other than the run's executor — today's cross_model_review=true fallback, now always-on). Clarifier (clarify_suggest.go) and drafter (contract_draft.go): resolve the agent as today minus the flag (explicit --reviewer, else cross-model default), then take pins from that agent's review.panel entry when present. Remove the cross_model_review flag and the agents.AgentConfig type (with AgentConfig.Transport, the config branch of acpTransportEnabled goes away — PACTUM_AGENT_TRANSPORT env stays the only switch, default remains cli for now); review limits readers (resolveReviewLoopSettings, defaults) move from config.Limits.Review/config.Budget to config.Review.*.
- Update all tests that construct or assert on the old config shape; add coverage for: unknown-key rejection, entry validation (unknown agent, duplicate agent, colon-in-model), executor pin applied only to the matching invoked agent, per-panel-member pins (one member pinned, the other unpinned), and the empty-panel cross-model default.
- Docs: update docs/agents.md (config examples: executor_model/reviewer_model/cross_model_review/review_panel/transport references) to the new shape with a two-member panel example; sweep other docs for references to removed keys (grep executor_model, reviewer_model, cross_model_review, default_profile, max_usd, project_map, limits:) and update; docs/backlog.md notes the config redesign and drops the now-resolved notes (shared reviewer model pin; workspace-config model-pin leak example stays as history if referenced).

## Out of scope
- Do NOT touch the ACP transport implementation or its gaps (model-over-ACP, read-only write guard — next milestone); do NOT flip the transport default to acp (stays cli; env-only switching); do NOT rewrite the workspace .heurema/pactum/config.yaml itself (committed separately right after merge); no backward compatibility or migration code for old keys (strict parsing intentionally rejects them); do not change ApplyModelSpec CLI-flag formatting, gate semantics, review-loop convergence, or any stage behavior beyond config plumbing.

## Paths in scope
- internal/agents/*.go
- internal/app/*.go
- docs/*.md


## Acceptance criteria
- The config schema has exactly the five sections (schema, map, gate, execute, review) with the agreed fields; every previously-dead key is gone from structs and defaults; an unknown key in config.yaml fails loudly naming the file.
- execute.models and review.panel share one {agent, model, effort} entry type with validation (unknown agent / duplicate agent / colon-in-model are clear errors); the executor pin applies only to the matching invoked agent; panel members carry per-member pins; clarifier/drafter take pins from the resolved agent's panel entry; empty panel yields the cross-model single-reviewer default; cross_model_review and the agents section no longer exist.
- PACTUM_AGENT_TRANSPORT env remains the only transport switch (config knob removed, default cli); review limits and budget are read from review.* and nothing reads the old locations.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes; docs contain no references to removed keys outside historical backlog notes.

## Validation commands
- go build ./...
- go test ./internal/agents/... ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- Pre-release: schema value stays pactum.config.v1 despite the shape change, and old keys get no migration — strict parsing surfaces them immediately and the workspace file is rewritten by hand right after merge.
- The agent is chosen at invocation time (--agent / cross-model resolution), so stage model pins must be per-agent entry lists, never single strings; review.panel doubles as the reviewer-role model registry for clarify/draft.

## Open questions
- None
