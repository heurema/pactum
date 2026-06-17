# Contract Review Fixer Prompt

You are fixing a software change contract to address blocking review findings.

Current contract version: be84c20ce2ccdb8b395f8ddcbb6a8e2892438db253738c76ef936da2df6ce303

## Current Contract

**Goal**: Rework the pactum config to the new pipeline shape and wire it through the existing code; behaviour-preserving (no new runtime capability), config FORMAT change only. No users, so breaking the config shape is free.

NEW top-level config keys: version (string, value v1alpha1 — REPLACES the old schema: pactum.config.v1alpha1 field; the config file is standalone so renaming its version field is safe; do NOT touch the schema discriminator on any other artifact/record type); agents (unchanged: [{name, model, effort?}]); map (unchanged); out_of_scope (string, block|warn — REPLACES gate.scope_enforcement; drop the gate: wrapper); pipeline (a map of stage -> {by, loop?}). Remove the old top-level keys schema, gate, review, contract, clarify, timeouts.

pipeline stages and their value shape: clarify, contract_draft, contract_review, execute, code_review, memory. Each stage is an object. by: is the performer(s) — a scalar agent name OR a list, normalized to []string. loop: is {max, patience, settle} (optional).

VALIDATION (load-time): loop: is valid ONLY on clarify, contract_review, code_review (the loop stages); loop on contract_draft/execute/memory is a load error. A by: LIST (len>1) is valid ONLY on contract_review and code_review (the existing panels); len>1 on any other stage is a load error. Every by: agent name must resolve in agents (else load error). out_of_scope must be block or warn. Reject unknown top-level keys and unknown stage names.

MAPPING (the resolver must reproduce today's behaviour exactly): review.panel -> pipeline.code_review.by; review.{max_rounds,patience,clean_rounds} -> pipeline.code_review.loop.{max,patience,settle}; contract.reviewers -> pipeline.contract_review.by; contract_review now ALSO gets loop knobs via pipeline.contract_review.loop.{max,patience,settle} (today it reuses the review limits — keep that resolution, just sourced from pipeline.contract_review.loop, falling back to the same defaults); clarify.max_rounds -> pipeline.clarify.loop.max; gate.scope_enforcement -> out_of_scope. Single-agent stages contract_draft/execute/memory and clarify each name their agent via by:; these must reproduce today's resolved per-stage agent assignment (verify against run.go role-resolution before hardcoding the default config's by: values).

Update the default-config writer to emit the new shape. Update all config call sites (config.Schema, config.Review.*, config.Contract.*, config.Gate.*, config.Clarify.*, config.Timeouts.*) to read from the new structs. Update config tests.

Constraints: behaviour-preserving (same agents/limits resolve as today); do NOT change internal/loop, the loop bodies (review_loop.go/contract_review.go/clarify_loop.go logic), or add multi-model/best-of-N. Validation: go build ./..., go test ./..., make check, and go test ./internal/app -run TestReadConfig.

**Scope in**:
  - Replace the Pactum config reader, in-memory config structs, and default-config writer with the new top-level shape: version, agents, map, out_of_scope, and pipeline.
  - Model the pipeline stages clarify, contract_draft, contract_review, execute, code_review, and memory with by plus optional loop settings where allowed.
  - Normalize scalar and list by values to []string and validate all referenced agent names against the agents registry.
  - Wire existing config consumers to the new fields while preserving current role assignment, panel, loop-limit, scope-enforcement, and timeout fallback behavior where the new shape still represents it.
  - Update config-focused tests and affected app tests for the new config shape and validation rules.

**Scope out**:
  - Changing any non-config artifact or record schema discriminator named schema.
  - Adding backward-compatible support or migration for the old config top-level keys schema, gate, review, contract, clarify, or timeouts.
  - Changing internal/loop, loop algorithms, review_loop.go, contract_review.go, or clarify_loop.go behavior beyond config field plumbing.
  - Adding multi-model execution, best-of-N behavior, new runtime capabilities, or new agent transports.
  - Documentation-only rewrites or unrelated refactors.

**Acceptance criteria**:
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

**Validation commands**:
  - go test ./internal/app -run TestReadConfig
  - go build ./...
  - go test ./...
  - make check
  - [ $(grep -rEn --include='*.go' 'config\.(Schema|Review|Contract|Gate|Clarify|Timeouts)' internal/ | grep -v _test\.go | wc -l) -eq 0 ]

**Assumptions**:
  - No existing user configs need migration; rejecting the old shape is acceptable.
  - The top-level version field is config-local and must not alter JSON schema fields on other Pactum artifacts.
  - Removing the top-level timeouts config means legacy config-level idle timeout overrides are intentionally dropped; existing CLI timeout overrides and built-in fallback behavior remain.
  - The pipeline stage set is closed to exactly clarify, contract_draft, contract_review, execute, code_review, and memory.
  - The default config must emit pipeline.contract_review.loop values (max, patience, settle) equal to today's effective contract-review limits, which are identical to today's code_review defaults. The new format allows independent contract_review.loop tuning as an incidental consequence of the format change, not a new runtime capability; the behaviour-preservation guarantee applies to the default config and is machine-checkable via the resolver-equivalence test. An executor that emits divergent contract_review.loop defaults silently breaks this guarantee.
  - An absent or empty by value — absent key, empty list [], or empty string — is valid for every pipeline stage and is never a load error. For contract_review an absent/empty by means contract review is disabled (today's default). For code_review an absent/empty by means the runtime uses its existing default-reviewer fallback. For clarify, contract_draft, execute, and memory an absent/empty by means the runtime uses its existing default role assignment for that stage. Validation errors on by are limited to: unresolved non-empty agent names, list length > 1 on non-panel stages, and loop on non-loop stages.

## Blocking Findings to Address

1. [opus/testability] The resolver-equivalence test — designated by the contract as THE machine-check for the behaviour-preservation guarantee — has no specified independent oracle. The old config code is removed in this same change, so no old-vs-new in-build comparison exists, and 'match today's values' is undefined. As written the test can pass tautologically (hand-transcribed expected constants compared against the same hardcoded new defaults), so the core guarantee is not actually gatable. Specify the oracle: capture pre-change resolved values as a golden fixture, or require concrete literal expectations cross-checked against run.go.
   Evidence: Acceptance: 'resolver-equivalence (asserting that resolved per-stage agent names, panel lists, and loop limits — max, patience, settle — for all six pipeline stages match today's values...)'; Assumption: 'the behaviour-preservation guarantee applies to the default config and is machine-checkable via the resolver-equivalence test. An executor that emits divergent contract_review.loop defaults silently breaks this guarantee.'
2. [opus/testability] The resolver-equivalence test — designated by the contract as THE machine-check for the behaviour-preservation guarantee — has no specified independent oracle. The old config code is removed in this same change, so no old-vs-new in-build comparison exists, and 'match today's values' is undefined. As written the test can pass tautologically (hand-transcribed expected constants compared against the same hardcoded new defaults), so the core guarantee is not actually gatable. Specify the oracle: capture pre-change resolved values as a golden fixture, or require concrete literal expectations cross-checked against run.go.
   Evidence: Acceptance: 'resolver-equivalence (asserting that resolved per-stage agent names, panel lists, and loop limits — max, patience, settle — for all six pipeline stages match today's values...)'; Assumption: 'the behaviour-preservation guarantee applies to the default config and is machine-checkable via the resolver-equivalence test. An executor that emits divergent contract_review.loop defaults silently breaks this guarantee.'
3. [codex-xhigh/testability] The behavior-preservation criteria for default resolver values and single-agent role assignment are not fully pinned to machine-checkable expected values. The contract says to match today's behavior and verify against run.go, but it does not require captured baseline values, fixtures, or explicit expected resolved agents/limits for the tests to assert.
   Evidence: Acceptance criteria: "Resolver behavior matches the old config mapping..." and "Single-agent stages ... preserve the current default role assignment verified against existing role-resolution code in run.go."
4. [codex-xhigh/validation-soundness] The static grep validation command is a shell expression rather than a gate-runnable command.
   Evidence: Validation commands: "[ $(grep -rEn --include='*.go' 'config\.(Schema|Review|Contract|Gate|Clarify|Timeouts)' internal/ | grep -v _test\.go | wc -l) -eq 0 ]"
5. [codex-xhigh/scope-fidelity] The scope includes independently configurable contract-review loop limits, which contradicts the stated goal of a behavior-preserving config format change with no new runtime capability.
   Evidence: Goal: "behaviour-preserving (no new runtime capability), config FORMAT change only"; Mapping: "contract_review now ALSO gets loop knobs via pipeline.contract_review.loop.{max,patience,settle}"; Assumptions: "The new format allows independent contract_review.loop tuning as an incidental consequence of the format change, not a new runtime capability".
6. [codex-xhigh/assumptions-surfaced] The contract explicitly assumes no user config migration is needed, but does not state whether existing repo-owned config artifacts, fixtures, examples, or version-controlled Pactum run records may still use the old shape and whether the executor should update them or leave them untouched.
   Evidence: Assumptions: "No existing user configs need migration; rejecting the old shape is acceptable." Scope out: "Documentation-only rewrites or unrelated refactors." Acceptance criteria require legacy top-level keys to be rejected.

## Fixer Instructions

- Address each blocking finding by updating the relevant contract field.
- Do NOT change the goal field — it is out of scope for the fixer.
- Only include the contract fields you are changing in the output.
- base_version must exactly match the version shown above.

## Output

Output your reasoning, then a single JSON block with the revise payload:

```json
{
  "schema": "pactum.contract_revise.v1alpha1",
  "base_version": "be84c20ce2ccdb8b395f8ddcbb6a8e2892438db253738c76ef936da2df6ce303",
  "contract": {
    "acceptance_criteria": ["...updated criteria..."],
    "validation": {"commands": ["...updated commands..."]}
  }
}
```

Omit any contract field you are not changing. Do not include the goal field.
