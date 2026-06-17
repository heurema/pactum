# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260617_090147/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260617_090147/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260617_090147/review/review.json, .heurema/pactum/runs/run_20260617_090147/review/findings.jsonl, .heurema/pactum/runs/run_20260617_090147/review/resolutions.jsonl

## Approved contract
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

## Current review findings
- Summary: findings=8 open=8 resolved=0 blocking_open=7
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=medium category=correctness blocking=true status=open: Non-empty pipeline.<single-stage>.by values are parsed and validated but ignored at runtime. prepareExecution reads the config, then calls resolveExecutorEntry(config, agentName); when --agent is omitted that resolver returns config.Agents[0] and never checks config.Pipeline.Execute.By. The same old default path is used for contract_draft and clarify via resolveReviewerEntry, so configured stage performers do not take effect.
    location: internal/app/execute.go:227
  - f_002 severity=high category=correctness blocking=true status=open: Single-agent pipeline by values are parsed and validated but ignored by runtime stage resolution.
    location: internal/app/execute.go:223
  - f_003 severity=medium category=quality blocking=true status=open: Config tests do not cover two contract-required rejection paths: invalid out_of_scope values and unknown pipeline stage names.
    location: internal/app/config_test.go:104
  - f_004 severity=medium category=quality blocking=true status=open: TestResolverEquivalence does not exercise the runtime resolver or load a new-shape config, so it does not satisfy the observable equivalence test required by the contract.
    location: internal/app/config_test.go:404
  - f_006 severity=medium category=correctness blocking=true status=open: Single-stage pipeline by values are parsed and validated but ignored at runtime; execute, contract_draft, clarify, and memory keep using the old CLI/default resolver paths instead of pipeline.<stage>.by.
    location: internal/app/execute.go:227
  - f_007 severity=medium category=quality blocking=true status=open: The CLI help still documents the removed timeouts.idle config fallback for --timeout.
    location: internal/app/cli.go:58
  - f_008 severity=medium category=quality blocking=true status=open: User-facing docs still describe legacy config keys that the new reader rejects.
    location: docs/agents.md:49
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_005 severity=low category=quality blocking=false status=open: The omitted-loop fallback test exercises the contract_review resolver after checking code_review decoded nil, leaving code_review and clarify omitted-loop fallback branches untested.
    location: internal/app/config_test.go:323

## Fix boundaries
- Trace each finding to the relevant code before acting.
- Fix valid findings in place.
- For false positives, explain a concrete rebuttal instead of changing code.
- Keep changes inside the approved contract and review-finding scope.
- Do not edit `.heurema` artifacts.
- Do not run `pactum review approve`, `pactum review finding resolve`, or `pactum review run`.

## House style
- Match the surrounding code: idiom, naming, comment density.
- Comment only where the code is not self-explanatory; do not narrate the obvious.
- Search for and reuse existing helpers before writing new ones.
- Keep the diff small and focused: change only what the contract requires.
- Simplicity first: no enterprise patterns for simple problems, question every new abstraction, no premature generalization or optimization.
- Over-engineering DON'Ts: wrappers that add nothing, factories or abstractions for a single case, unused extension points, dual implementations where the old path has no callers, silent fallbacks that hide failures.
- No dead code, no commented-out code, no unused parameters.
- Handle errors per the project's existing convention; no silent failures.
- Tests verify behavior, not implementation details, and cover error paths.
- Fake-test DON'Ts: always-pass tests, hardcoded-value checks, assertions on mock behavior instead of the code under test, ignored errors, commented-out cases.

The reviewer will re-check your fixes against the discipline rules above.

## Output shape
Your final output MUST include exactly one fenced `json` block with this shape:

```json
{
  "schema": "pactum.review_fix_outcomes.v1alpha1",
  "outcomes": [
    {
      "finding_id": "f_001",
      "outcome": "fixed",
      "note": "What changed and where, or the concrete rebuttal/blocker."
    }
  ]
}
```

Rules:
- Include exactly one outcome entry for every blocking finding listed above with status open.
- Do NOT edit code for advisory (non-blocking) findings, and do NOT emit outcomes for them; they are context only.
- Use outcome fixed when you changed code to address a valid blocking finding.
- Use outcome rebutted when the blocking finding is a false positive; note must contain the concrete rebuttal.
- Use outcome blocked when concrete missing information or state prevents a fix.
- Do not include advisory or resolved findings in the outcomes list.
