# Contract Draft

## Goal
Add a caching layer to speed things up

## Current status
Contract status: draft
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260609_111238
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (0 result(s))

## Clarifications
- q_001 [blocking] — Which operation is the caching layer meant to speed up — the project-map build (map/scan + tree-sitter code extraction, rebuilt in full on every `pactum map refresh`), the search-index rebuild, agent/LLM subprocess calls, or something else?
  Rationale: The goal names no target. The repo has no computation cache today (`cache/` only holds the current-run pointer; 'cache read ratio' is LLM-token accounting). The map/scan phase is the strongest fit: it fully rebuilds every run, is deterministic, and already emits `hashes.jsonl` as a natural content-hash cache key. Agent/LLM responses are nondeterministic and a poor caching target. Without this answer the contract scope cannot be written.
  Answer: pending
- q_002 — Should the cache persist on disk across CLI invocations (e.g. under the existing gitignored `cache/` workspace dir, keyed by per-file content hash and invalidated when the hash changes), or is an in-process/in-memory cache for a single command sufficient?
  Rationale: Pactum runs as discrete CLI invocations, so an in-memory cache gives almost no cross-run benefit — the speedup only materializes if results survive between runs. The repo already has a gitignored `cache/` dir and content hashes, making a persistent, hash-keyed cache the natural and low-risk design. Confirming this fixes the core architecture before implementation.
  Answer: pending
- q_003 [blocking] — What concrete, measurable outcome defines success for 'speed things up,' and which validation command(s) should the contract gate on (the repo currently has empty acceptance criteria and no validation commands)?
  Rationale: 'Speed things up' is unmeasurable as written, and both acceptance_criteria and validation.commands are empty — the gate/validation phase has nothing to run. Correctness must be preserved (cached output must match a clean rebuild), and the speedup must be demonstrable, otherwise the change can't be verified or accepted.
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
- Which operation is the caching layer meant to speed up — the project-map build (map/scan + tree-sitter code extraction, rebuilt in full on every `pactum map refresh`), the search-index rebuild, agent/LLM subprocess calls, or something else?
- Should the cache persist on disk across CLI invocations (e.g. under the existing gitignored `cache/` workspace dir, keyed by per-file content hash and invalidated when the hash changes), or is an in-process/in-memory cache for a single command sufficient?
- What concrete, measurable outcome defines success for 'speed things up,' and which validation command(s) should the contract gate on (the repo currently has empty acceptance criteria and no validation commands)?
