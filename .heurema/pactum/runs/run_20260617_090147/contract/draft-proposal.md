# Contract Draft Proposal

## Status
- Run id: run_20260617_090147
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-17T09:04:40Z

## In scope
- Replace the Pactum config reader, in-memory config structs, and default-config writer with the new top-level shape: version, agents, map, out_of_scope, and pipeline.
- Model the pipeline stages clarify, contract_draft, contract_review, execute, code_review, and memory with by plus optional loop settings where allowed.
- Normalize scalar and list by values to []string and validate all referenced agent names against the agents registry.
- Wire existing config consumers to the new fields while preserving current role assignment, panel, loop-limit, scope-enforcement, and timeout fallback behavior where the new shape still represents it.
- Update config-focused tests and affected app tests for the new config shape and validation rules.

## Out of scope
- Changing any non-config artifact or record schema discriminator named schema.
- Adding backward-compatible support or migration for the old config top-level keys schema, gate, review, contract, clarify, or timeouts.
- Changing internal/loop, loop algorithms, review_loop.go, contract_review.go, or clarify_loop.go behavior beyond config field plumbing.
- Adding multi-model execution, best-of-N behavior, new runtime capabilities, or new agent transports.
- Documentation-only rewrites or unrelated refactors.

## Acceptance criteria
- The generated default config contains version: v1alpha1, agents, map, out_of_scope, and pipeline, and does not emit the removed top-level keys schema, gate, review, contract, clarify, or timeouts.
- readConfig accepts the new config shape and rejects unknown top-level keys, unknown pipeline stage names, invalid version values, invalid out_of_scope values, and all removed legacy top-level keys.
- loop is accepted only on clarify, contract_review, and code_review; loop on contract_draft, execute, or memory is a load-time error.
- A by list with more than one agent is accepted only on contract_review and code_review; a multi-agent by list on any other stage is a load-time error.
- Every non-empty by agent name must refer to a registered agent, and failures identify the offending config path and agent name.
- Resolver behavior matches the old config mapping: review.panel maps to pipeline.code_review.by, review max_rounds/patience/clean_rounds map to pipeline.code_review.loop max/patience/settle, contract.reviewers maps to pipeline.contract_review.by, contract review loop limits come from pipeline.contract_review.loop with the same defaults, clarify.max_rounds maps to pipeline.clarify.loop.max, and gate.scope_enforcement maps to out_of_scope.
- Single-agent stages clarify, contract_draft, execute, and memory use their pipeline by values while preserving the current default role assignment verified against the existing role-resolution code.
- Empty/default contract_review.by preserves the current disabled-contract-review behavior, and empty/default code_review.by preserves the current code-review default reviewer behavior.
- Runtime code no longer dereferences config.Schema, config.Review, config.Contract, config.Gate, config.Clarify, or config.Timeouts outside tests that intentionally assert legacy-key rejection.
- Tests cover default-config round-trip, scalar by parsing, list by parsing, loop validation, multi-agent validation, unknown-key rejection, unknown-stage rejection, unresolved-agent rejection, and old-shape rejection.

## Validation commands
- go test ./internal/app -run TestReadConfig
- go build ./...
- go test ./...
- make check

## Assumptions
- No existing user configs need migration; rejecting the old shape is acceptable.
- The top-level version field is config-local and must not alter JSON schema fields on other Pactum artifacts.
- Removing the top-level timeouts config means legacy config-level idle timeout overrides are intentionally dropped; existing CLI timeout overrides and built-in fallback behavior remain.
- The pipeline stage set is closed to exactly clarify, contract_draft, contract_review, execute, code_review, and memory.

