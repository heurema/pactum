# Memory Candidate

## Run
- Run id: run_20260617_090147
- Source: deterministic

## Contract
- Goal: Rework the pactum config to the new pipeline shape and wire it through the existing code; behaviour-preserving (no new runtime capability), config FORMAT change only. No users, so breaking the config shape is free.

NEW top-level config keys: version (string, value v1alpha1 — REPLACES the old schema: pactum.config.v1alpha1 field; the config file is standalone so renaming its version field is safe; do NOT touch the schema discriminator on any other artifact/record type); agents (unchanged: [{name, model, effort?}]); map (unchanged); out_of_scope (string, block|warn — REPLACES gate.scope_enforcement; drop the gate: wrapper); pipeline (a map of stage -> {by, loop?}). Remove the old top-level keys schema, gate, review, contract, clarify, timeouts.

pipeline stages and their value shape: clarify, contract_draft, contract_review, execute, code_review, memory. Each stage is an object. by: is the performer(s) — a scalar agent name OR a list, normalized to []string. loop: is {max, patience, settle} (optional).

VALIDATION (load-time): loop: is valid ONLY on clarify, contract_review, code_review (the loop stages); loop on contract_draft/execute/memory is a load error. A by: LIST (len>1) is valid ONLY on contract_review and code_review (the existing panels); len>1 on any other stage is a load error. Every by: agent name must resolve in agents (else load error). out_of_scope must be block or warn. Reject unknown top-level keys and unknown stage names.

MAPPING (the resolver must reproduce today's behaviour exactly): review.panel -> pipeline.code_review.by; review.{max_rounds,patience,clean_rounds} -> pipeline.code_review.loop.{max,patience,settle}; contract.reviewers -> pipeline.contract_review.by; contract_review now ALSO gets loop knobs via pipeline.contract_review.loop.{max,patience,settle} (today it reuses the review limits — keep that resolution, just sourced from pipeline.contract_review.loop, falling back to the same defaults); clarify.max_rounds -> pipeline.clarify.loop.max; gate.scope_enforcement -> out_of_scope. Single-agent stages contract_draft/execute/memory and clarify each name their agent via by:; these must reproduce today's resolved per-stage agent assignment (verify against run.go role-resolution before hardcoding the default config's by: values).

Update the default-config writer to emit the new shape. Update all config call sites (config.Schema, config.Review.*, config.Contract.*, config.Gate.*, config.Clarify.*, config.Timeouts.*) to read from the new structs. Update config tests.

Constraints: behaviour-preserving (same agents/limits resolve as today); do NOT change internal/loop, the loop bodies (review_loop.go/contract_review.go/clarify_loop.go logic), or add multi-model/best-of-N. Validation: go build ./..., go test ./..., make check, and go test ./internal/app -run TestReadConfig.
- In scope:
  - Replace the Pactum config reader, in-memory config structs, and default-config writer with the new top-level shape: version, agents, map, out_of_scope, and pipeline.
  - Model the pipeline stages clarify, contract_draft, contract_review, execute, code_review, and memory with by plus optional loop settings where allowed.
  - Normalize scalar and list by values to []string and validate all referenced agent names against the agents registry.
  - Wire existing config consumers to the new fields while preserving current role assignment, panel, loop-limit, scope-enforcement, and timeout fallback behavior where the new shape still represents it.
  - Update config-focused tests and affected app tests for the new config shape and validation rules.
- Out of scope:
  - Changing any non-config artifact or record schema discriminator named schema.
  - Adding backward-compatible support or migration for the old config top-level keys schema, gate, review, contract, clarify, or timeouts.
  - Changing internal/loop, loop algorithms, review_loop.go, contract_review.go, or clarify_loop.go behavior beyond config field plumbing.
  - Adding multi-model execution, best-of-N behavior, new runtime capabilities, or new agent transports.
  - Documentation-only rewrites or unrelated refactors.
- Acceptance criteria:
  - The generated default config contains version: v1alpha1, agents, map, out_of_scope, and pipeline, and does not emit the removed top-level keys schema, gate, review, contract, clarify, or timeouts.
  - readConfig accepts the new config shape and rejects: unknown top-level keys, unknown pipeline stage names, version values other than v1alpha1, out_of_scope values other than block or warn, and all removed legacy top-level keys (schema, gate, review, contract, clarify, timeouts).
  - loop is accepted only on clarify, contract_review, and code_review; loop on contract_draft, execute, or memory is a load-time error.
  - A by list with more than one agent is accepted only on contract_review and code_review; a multi-agent by list on any other stage is a load-time error.
  - Every non-empty by agent name must refer to a registered agent in the agents list, and failures identify the offending config path and agent name.
  - Resolver behavior matches the old config mapping: review.panel maps to pipeline.code_review.by; review.{max_rounds,patience,clean_rounds} map to pipeline.code_review.loop.{max,patience,settle}; contract.reviewers maps to pipeline.contract_review.by; contract-review loop limits come from pipeline.contract_review.loop.{max,patience,settle} with the same defaults as today; clarify.max_rounds maps to pipeline.clarify.loop.max; gate.scope_enforcement maps to out_of_scope.
  - Single-agent stages (clarify, contract_draft, execute, memory) use their pipeline.<stage>.by values and preserve the current default role assignment verified against existing role-resolution code in run.go.
  - An absent or empty pipeline.contract_review.by — absent key, empty list [], or empty string — is a valid config value that disables contract review, preserving today's default-disabled behavior; readConfig must not reject it as a missing-required field. An absent or empty pipeline.code_review.by — absent key, empty list [], or empty string — is a valid config value that causes the runtime to use its current default-reviewer fallback, not a load error. An absent or empty by on clarify, contract_draft, execute, or memory is also valid and causes the runtime to use its existing default role assignment for that stage.
  - Runtime code no longer dereferences config.Schema, config.Review, config.Contract, config.Gate, config.Clarify, or config.Timeouts outside of tests that intentionally assert legacy-key rejection; the static grep validation command enforces this gate.
  - Tests cover: default-config round-trip; scalar by parsing; list by parsing; loop validation (accepted on clarify, contract_review, code_review; rejected on contract_draft, execute, memory); multi-agent by validation (accepted on contract_review, code_review; rejected on all other stages); unknown-key rejection; unknown-stage rejection; unresolved-agent rejection; old-shape (legacy top-level key) rejection; invalid version value rejection; invalid out_of_scope value rejection; empty/default by behavior (empty contract_review.by disables contract review, empty code_review.by uses default-reviewer fallback, empty by on single-agent stages uses default role assignment); resolver-equivalence (asserting that resolved per-stage agent names, panel lists, and loop limits — max, patience, settle — for all six pipeline stages match today's values when using the default config, including that pipeline.contract_review.loop resolves to the same limits as pipeline.code_review.loop); and default role assignment preservation for single-agent stages verified against run.go role-resolution.
  - Behaviour-preservation is pinned by an OBSERVABLE equivalence test: a test loads a config in the new shape that is equivalent to todays defaults and asserts the resolved values match the pre-rework resolved values by equality — code_review reviewers == old review.panel, code_review loop {max,patience,settle} == old review {max_rounds,patience,clean_rounds}, contract_review reviewers == old contract.reviewers, clarify loop.max == old clarify.max_rounds, out_of_scope == old gate.scope_enforcement, and each single-agent stage (contract_draft, execute, memory, clarify) resolves to the same agent run.go resolves today. The resolver reproducing todays behaviour must be verified by value equality, not only by the existing suite still passing.
  - Omitted/empty-field semantics are specified and tested: an absent out_of_scope key defaults to block (matching todays explicit default); an absent or empty by on ANY stage is a load error (every stage names its performer; empty is invalid, not disabled); an omitted loop on ANY of the three loop stages (clarify, contract_review, code_review) falls back to the same in-code limit defaults used today (not only contract_review). Tests cover the absent-out_of_scope default, the empty-by load error, and at least one omitted-loop fallback.
- Validation commands:
  - go test ./internal/app -run TestReadConfig
  - go build ./...
  - go test ./...
  - make check
  - [ $(grep -rEn --include='*.go' 'config\.(Schema|Review|Contract|Gate|Clarify|Timeouts)' internal/ | grep -v _test\.go | wc -l) -eq 0 ]

## Outcome
- Gate status: needs_review
- Review status: approved
- Execution exit code: 0
- Validation passed: true
- Changes need review: true

## Changes
- Changed files:
  - docs/agents.md
  - internal/app/agent_attempt_transport_test.go
  - internal/app/agent_resolve.go
  - internal/app/app.go
  - internal/app/app_test.go
  - internal/app/clarify_loop.go
  - internal/app/clarify_loop_test.go
  - internal/app/clarify_round.go
  - internal/app/cli.go
  - internal/app/config.go
  - internal/app/config_test.go
  - internal/app/contract_draft.go
  - internal/app/contract_review.go
  - internal/app/contract_review_test.go
  - internal/app/execute.go
  - internal/app/gate.go
  - internal/app/gate_test.go
  - internal/app/resolve.go
  - internal/app/review.go
  - internal/app/review_fix.go
  - internal/app/review_loop.go
  - internal/app/review_loop_test.go
- New files: none
- Missing files: none

## Clarifications
- None

## Review Decisions
- f_001 [medium] resolved internal/app/execute.go:227: Non-empty pipeline.<single-stage>.by values are parsed and validated but ignored at runtime. prepareExecution reads the config, then calls resolveExecutorEntry(config, agentName); when --agent is omitted that resolver returns config.Agents[0] and never checks config.Pipeline.Execute.By. The same old default path is used for contract_draft and clarify via resolveReviewerEntry, so configured stage performers do not take effect.
  Resolution: Updated resolveExecutorEntry in agent_resolve.go to accept a stageBy stageBy parameter; when --agent is omitted and stageBy is non-empty, it resolves stageBy[0] instead of defaulting to config.Agents[0]. Updated execute.go:227 to pass config.Pipeline.Execute.By. The same change covers review_fix.go:205 and contract_review.go:552 (both also pass config.Pipeline.Execute.By).
- f_002 [high] resolved internal/app/execute.go:223: Single-agent pipeline by values are parsed and validated but ignored by runtime stage resolution.
  Resolution: Same root cause as f_001. resolveExecutorEntry and resolveReviewerEntry both now consult the configured stage by before falling through to their respective defaults. All seven call sites updated: execute.go, review_fix.go, contract_review.go, review.go, review_loop.go, clarify_round.go, contract_draft.go.
- f_003 [medium] resolved internal/app/config_test.go:104: Config tests do not cover two contract-required rejection paths: invalid out_of_scope values and unknown pipeline stage names.
  Resolution: Added TestReadConfigRejectsUnknownPipelineStage in config_test.go (the invalid-out_of_scope rejection was already covered by TestReadConfigRejectsInvalidOutOfScope in app_test.go, so only the missing unknown-stage test was added).
- f_004 [medium] resolved internal/app/config_test.go:404: TestResolverEquivalence does not exercise the runtime resolver or load a new-shape config, so it does not satisfy the observable equivalence test required by the contract.
  Resolution: Extended TestResolverEquivalence in config_test.go to write the default config to a temp file, load it with readConfig, and call resolveExecutorEntry for the execute stage and resolveReviewerEntry for clarify and contract_draft stages, asserting all three resolve to config.Agents[0] ("claude"). Also calls resolveContractReviewLoopLimits and asserts it returns the same limits as the default config loop struct fields.
- f_005 [low] resolved internal/app/config_test.go:323: The omitted-loop fallback test exercises the contract_review resolver after checking code_review decoded nil, leaving code_review and clarify omitted-loop fallback branches untested.
  Resolution: Advisory test-ordering nit; omitted-loop fallback is covered. Acknowledged.
- f_006 [medium] resolved internal/app/execute.go:227: Single-stage pipeline by values are parsed and validated but ignored at runtime; execute, contract_draft, clarify, and memory keep using the old CLI/default resolver paths instead of pipeline.<stage>.by.
  Resolution: Same fix as f_001/f_002. resolveReviewerEntry in agent_resolve.go now accepts stageBy and consults it before cross-model selection. clarify_round.go passes config.Pipeline.Clarify.By and contract_draft.go passes config.Pipeline.ContractDraft.By. Memory stage has no agent invocation in the current codebase (MemoryPropose is deterministic), so there is no resolver call site to update for memory.
- f_007 [medium] resolved internal/app/cli.go:58: The CLI help still documents the removed timeouts.idle config fallback for --timeout.
  Resolution: Replaced all six occurrences of 'Defaults to timeouts.idle in the workspace config (25m when unset).' with 'Defaults to 25m when not given.' in cli.go (lines for clarify run, contract draft, contract review, execute run, review run, and review fix run timeout flags).
- f_008 [medium] resolved docs/agents.md:49: User-facing docs still describe legacy config keys that the new reader rejects.
  Resolution: Updated docs/agents.md: replaced review.panel references with pipeline.code_review.by in the registry description, the config YAML example block (lines 41-51), and the reviewer-round roster paragraph; replaced the timeouts.idle resolution description with the simpler built-in-25m text; replaced clarify.max_rounds with pipeline.clarify.loop.max; replaced review.max_rounds/review.clean_rounds/review.patience with pipeline.code_review.loop.{max,settle,patience}.
- Proposal summary: pending=0 accepted=8 rejected=0

## Reusable Project Knowledge
- scope: in scope: Replace the Pactum config reader, in-memory config structs, and default-config writer with the new top-level shape: version, agents, map, out_of_scope, and pipeline.
- scope: in scope: Model the pipeline stages clarify, contract_draft, contract_review, execute, code_review, and memory with by plus optional loop settings where allowed.
- scope: in scope: Normalize scalar and list by values to []string and validate all referenced agent names against the agents registry.
- scope: in scope: Wire existing config consumers to the new fields while preserving current role assignment, panel, loop-limit, scope-enforcement, and timeout fallback behavior where the new shape still represents it.
- scope: in scope: Update config-focused tests and affected app tests for the new config shape and validation rules.
- scope: out of scope: Changing any non-config artifact or record schema discriminator named schema.
- scope: out of scope: Adding backward-compatible support or migration for the old config top-level keys schema, gate, review, contract, clarify, or timeouts.
- scope: out of scope: Changing internal/loop, loop algorithms, review_loop.go, contract_review.go, or clarify_loop.go behavior beyond config field plumbing.
- scope: out of scope: Adding multi-model execution, best-of-N behavior, new runtime capabilities, or new agent transports.
- scope: out of scope: Documentation-only rewrites or unrelated refactors.
- review_resolution: f_001 resolved: Non-empty pipeline.<single-stage>.by values are parsed and validated but ignored at runtime. prepareExecution reads the config, then calls resolveExecutorEntry(config, agentName); when --agent is omitted that resolver returns config.Agents[0] and never checks config.Pipeline.Execute.By. The same old default path is used for contract_draft and clarify via resolveReviewerEntry, so configured stage performers do not take effect.; resolution: Updated resolveExecutorEntry in agent_resolve.go to accept a stageBy stageBy parameter; when --agent is omitted and stageBy is non-empty, it resolves stageBy[0] instead of defaulting to config.Agents[0]. Updated execute.go:227 to pass config.Pipeline.Execute.By. The same change covers review_fix.go:205 and contract_review.go:552 (both also pass config.Pipeline.Execute.By).
- review_resolution: f_002 resolved: Single-agent pipeline by values are parsed and validated but ignored by runtime stage resolution.; resolution: Same root cause as f_001. resolveExecutorEntry and resolveReviewerEntry both now consult the configured stage by before falling through to their respective defaults. All seven call sites updated: execute.go, review_fix.go, contract_review.go, review.go, review_loop.go, clarify_round.go, contract_draft.go.
- review_resolution: f_003 resolved: Config tests do not cover two contract-required rejection paths: invalid out_of_scope values and unknown pipeline stage names.; resolution: Added TestReadConfigRejectsUnknownPipelineStage in config_test.go (the invalid-out_of_scope rejection was already covered by TestReadConfigRejectsInvalidOutOfScope in app_test.go, so only the missing unknown-stage test was added).
- review_resolution: f_004 resolved: TestResolverEquivalence does not exercise the runtime resolver or load a new-shape config, so it does not satisfy the observable equivalence test required by the contract.; resolution: Extended TestResolverEquivalence in config_test.go to write the default config to a temp file, load it with readConfig, and call resolveExecutorEntry for the execute stage and resolveReviewerEntry for clarify and contract_draft stages, asserting all three resolve to config.Agents[0] ("claude"). Also calls resolveContractReviewLoopLimits and asserts it returns the same limits as the default config loop struct fields.
- review_resolution: f_005 resolved: The omitted-loop fallback test exercises the contract_review resolver after checking code_review decoded nil, leaving code_review and clarify omitted-loop fallback branches untested.; resolution: Advisory test-ordering nit; omitted-loop fallback is covered. Acknowledged.
- review_resolution: f_006 resolved: Single-stage pipeline by values are parsed and validated but ignored at runtime; execute, contract_draft, clarify, and memory keep using the old CLI/default resolver paths instead of pipeline.<stage>.by.; resolution: Same fix as f_001/f_002. resolveReviewerEntry in agent_resolve.go now accepts stageBy and consults it before cross-model selection. clarify_round.go passes config.Pipeline.Clarify.By and contract_draft.go passes config.Pipeline.ContractDraft.By. Memory stage has no agent invocation in the current codebase (MemoryPropose is deterministic), so there is no resolver call site to update for memory.
- review_resolution: f_007 resolved: The CLI help still documents the removed timeouts.idle config fallback for --timeout.; resolution: Replaced all six occurrences of 'Defaults to timeouts.idle in the workspace config (25m when unset).' with 'Defaults to 25m when not given.' in cli.go (lines for clarify run, contract draft, contract review, execute run, review run, and review fix run timeout flags).
- review_resolution: f_008 resolved: User-facing docs still describe legacy config keys that the new reader rejects.; resolution: Updated docs/agents.md: replaced review.panel references with pipeline.code_review.by in the registry description, the config YAML example block (lines 41-51), and the reviewer-round roster paragraph; replaced the timeouts.idle resolution description with the simpler built-in-25m text; replaced clarify.max_rounds with pipeline.clarify.loop.max; replaced review.max_rounds/review.clean_rounds/review.patience with pipeline.code_review.loop.{max,settle,patience}.
- review_resolution: proposal p_001 accepted as f_001
- review_resolution: proposal p_002 accepted as f_002
- review_resolution: proposal p_003 accepted as f_003
- review_resolution: proposal p_004 accepted as f_004
- review_resolution: proposal p_005 accepted as f_005
- review_resolution: proposal p_006 accepted as f_006
- review_resolution: proposal p_007 accepted as f_007
- review_resolution: proposal p_008 accepted as f_008
- validation: go test ./internal/app -run TestReadConfig passed
- validation: go build ./... passed
- validation: go test ./... passed
- validation: make check passed
- validation: [ $(grep -rEn --include='*.go' 'config\.(Schema|Review|Contract|Gate|Clarify|Timeouts)' internal/ | grep -v _test\.go | wc -l) -eq 0 ] passed

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
