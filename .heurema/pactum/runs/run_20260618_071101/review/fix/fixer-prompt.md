# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260618_071101/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260618_071101/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260618_071101/review/review.json, .heurema/pactum/runs/run_20260618_071101/review/findings.jsonl, .heurema/pactum/runs/run_20260618_071101/review/resolutions.jsonl

## Approved contract
- Goal: Plan-DAG slice 3: the plan immune system (entry) — static non-vacuous validation + a single-pass `plan_review` pipeline stage. This is slice 3 of the plan-DAG arc (see docs/contract-plan-dag-design.md, build plan item 3, and the "Validation is the immune system" section). It is the blocking prerequisite that makes a plan DAG trustworthy BEFORE any unattended execution loop (slice 4). STATIC ENFORCEMENT + REVIEW HOOK ONLY: no execution change, no topological loop, no tasks-state, no context-pack.

Context: slice 1 put plan.tasks[] on the hashed contract and validates it structurally (duplicate ids, cycles, unresolved depends_on, expected_files outside paths_in_scope, empty acceptance/validation) at contract load and revise. Slice 2 lets the drafter emit a plan and added `pactum plan show`. The plan's per-task validation is the most exploitable seam: a weak executor under retry pressure can fake green by weakening a check rather than doing the work. This slice adds the parts of the immune system that are enforceable statically (now), and a review hook over the DAG.

In scope:
1. Non-vacuous per-task validation (extend validateContractPlan, enforced at contract load AND revise like the existing slice-1 rules): a task whose expected_files is non-empty must have at least one validation command that is SCOPED to its expected_files — i.e. at least one validation command string references one of the task's expected_files by path or by an enclosing directory/package segment of that path. Reject with a new actionable issue code (e.g. VACUOUS_VALIDATION) a task whose validation commands are all unscoped/global (none reference any expected_file path or its parent dir), since a check that a global no-op like `go build ./...` or `make check` satisfies regardless is not a real check for that task. A task with empty expected_files is exempt from this rule (cannot be checked). Keep the rule a conservative substring/path-segment match (low false positives); a task MAY also carry extra global commands as long as at least one scoped command exists. Add focused tests: scoped validation accepted; all-global validation rejected with VACUOUS_VALIDATION; mixed scoped+global accepted; empty expected_files exempt.

2. New `plan_review` pipeline stage, mirroring `contract_review`: add a PlanReview pipelineStage field to the pipeline config (yaml: plan_review) reusing the existing stageBy (by: scalar-or-list of agent names); empty/absent by means the stage is a human-gate-only no-op (no automated plan review). Add a `pactum plan review [run_id] [--json]` command that runs a SINGLE-PASS reviewer panel (NOT a convergence loop, no fixer) over the contract's plan.tasks[] DAG. Reviewer lenses for the plan: granularity (the DAG earns nodes only on real intra-contract fan-in or independently-validatable surfaces; target 3-10 leaves; a leaf is one independently reviewable patch, not one edit), dependency-correctness (depends_on edges are sensible and acyclic — structural cycle/missing-dep is already hard-rejected, this lens judges logical correctness), testability (each task's validation is falsifiable and scoped to its expected_files), non-vacuity (no task validation is a global no-op), and scope-fidelity (the plan's expected_files stay within paths_in_scope and cover the contract's goal). Reuse the EXISTING reviewer-findings capture machinery (the mandatory pactum.reviewer_findings.v1alpha1 block + parse-miss + corrective-retry path shipped in the reviewer-findings-capture change) so plan-review findings are captured structurally, never silently dropped. Persist plan-review findings as artifacts under the run (e.g. plan-review/). When the contract has no plan, `plan review` is a clear no-op (prints that there is no plan to review, exits 0). Single-pass: collect and report structured findings; do NOT auto-fix or loop — the operator addresses findings by revising the plan and re-approving.

3. Tests: non-vacuous validation cases (above); plan review on a contract with a plan produces structured findings artifacts; plan review on a plan-less contract is a clear no-op exiting 0; an absent/empty plan_review.by makes automated plan review a no-op; the plan-review reviewer prompt documents the lenses and the mandatory findings block; plan-review uses the structured capture (a reviewer that omits the block triggers the existing parse-miss/corrective-retry path rather than silently passing).

Out of scope (explicitly later slices): baseline-red enforcement (running each task's validation against the pre-change tree to confirm it fails — this is runtime and lands with the executor loop in slice 4); frozen-edit detection (auto-blocking a node whose validation was edited in the same commit as its implementation — runtime, slice 4); the topological execute loop; execute.loop.max config; context-pack resolution; tasks-state.json; single-writer lease; --task. Do not change execute, gate, code_review, or memory behavior. Do not make plan_review a convergence loop with a fixer (single-pass only this slice).

Validation: go test ./internal/app -run 'Contract|Plan|Config|Review', go test ./..., go build ./..., make check.
- In scope:
  - Extend contract plan validation so any plan task with non-empty expected_files must have at least one validation command scoped to an expected file path or an enclosing path/package segment.
  - Reject unscoped per-task validation with an actionable VACUOUS_VALIDATION issue during both contract load and contract revise.
  - Add pipeline.plan_review configuration using the existing stageBy scalar-or-list parsing; absent or empty by means no automated plan review.
  - Add pactum plan review [run_id] [--json] as a single-pass plan DAG reviewer over contract.plan.tasks, distinct from the existing pactum review plan code-review dry-run command.
  - Persist plan-review prompts, attempts, results, and structured findings under a run-local plan-review artifact area.
  - Use the existing reviewer_findings structured-output capture behavior, including parse-miss detection and corrective retry, so prose-only reviewer output is never silently treated as clean.
- Out of scope:
  - Do not implement baseline-red validation enforcement.
  - Do not implement frozen-edit detection for task validation changes.
  - Do not change execute run behavior, add a topological execute loop, add tasks-state.json, context-pack resolution, execute.loop.max, single-writer leases, or --task.
  - Do not change gate, code_review, memory, or existing pactum review plan behavior.
  - Do not make plan_review a convergence loop and do not invoke any fixer from plan_review.
- Acceptance criteria:
  - A plan task with expected_files and only global validation such as go build ./... or make check is rejected with VACUOUS_VALIDATION on load and revise.
  - A plan task with expected_files is accepted when at least one validation command references an expected file path, an enclosing directory, or an enclosing package segment; mixed global plus scoped validation is accepted.
  - Scoped validation matching normalizes expected file paths by stripping a leading './' and collapsing to forward-slash separators (e.g., './internal/app/foo.go' normalizes to 'internal/app/foo.go'). A command string is considered scoped to a task if it contains, as a literal case-sensitive substring, at least one of: (a) the full normalized expected file path — rule (a) always qualifies regardless of whether the normalized path contains a '/' separator, so root-level files such as 'main.go' or 'go.mod' are fully covered by this rule; (b) any directory prefix of that path that itself contains at least one '/' separator (e.g., 'internal/app' is a valid prefix-match for 'internal/app/foo.go', but the bare basename 'foo.go' is not); or (c) a Go wildcard pattern that covers the file's directory, i.e., a string of the form '<dir>/...' or './<dir>/...' where '<dir>' is a directory prefix matching rule (b). The match is performed against the raw unprocessed command string, so paths that appear inside shell quotes in the command are still matched by substring. A substring with no '/' separator that is NOT the complete normalized path of the file being checked does not qualify as scoped; this prevents a bare filename component of a deeper-path file (e.g., 'foo.go' extracted from 'internal/app/foo.go') from being treated as a scoped reference.
  - Root-level expected files (normalized path has no '/' separator, e.g., 'main.go', 'go.mod', 'Makefile') are matched exclusively by rule (a): a task with expected_files ['go.mod'] and a validation command containing the substring 'go.mod' is accepted as scoped; a task with expected_files ['go.mod'] and only commands that do not contain 'go.mod' as a substring is rejected with VACUOUS_VALIDATION. A focused test covers both cases.
  - A plan task with empty expected_files remains exempt from the non-vacuous validation rule.
  - Config parsing accepts pipeline.plan_review.by as a scalar or list of registered agents, allows multiple agents as a reviewer panel, normalizes empty entries away, and treats absent or empty by as a no-op.
  - pactum plan review exits 0 with a clear no-plan message when the approved contract has no plan.
  - With no configured plan_review.by, pactum plan review exits 0 without launching reviewer attempts.
  - With configured reviewers and a plan, pactum plan review runs one single-pass reviewer panel over the plan DAG, reports the plan-review lenses, and does not revise the contract or run a fixer. The following artifacts must be written under the run-local plan-review artifact area and their presence must be verified in tests: (a) for each configured reviewer agent, a prompt artifact (e.g., plan-review/<agent>/prompt.txt) containing the exact prompt text sent to that reviewer; (b) for each attempt made by each reviewer, a raw-response artifact (e.g., plan-review/<agent>/attempt-<n>.txt) containing the unprocessed agent response text; (c) a structured findings artifact (e.g., plan-review/findings.json) containing the aggregated findings as a JSON array, written regardless of finding count — an empty array when the panel reports no findings. pactum plan review exits 1 when the panel produces one or more findings (any severity) and exits 0 when the panel produces zero findings, making plan review gatable from pipeline scripts.
  - The plan-review prompt documents the granularity, dependency-correctness, testability, non-vacuity, and scope-fidelity lenses and requires exactly one pactum.reviewer_findings.v1alpha1 JSON block.
  - A plan-review reviewer response missing the required findings block triggers the existing parse-miss/corrective-retry behavior instead of silently passing.
  - With --json, pactum plan review writes to stdout a single JSON object with the following top-level fields: no_plan (bool, true when the contract has no plan), no_reviewers (bool, true when plan_review.by is absent or empty), and findings (array of finding objects, empty array when no findings). Each finding object has string fields: agent (name of the reviewing agent), lens (one of: granularity, dependency-correctness, testability, non-vacuity, scope-fidelity), title, description, and severity (blocking or suggestion). The exit code follows the same rule regardless of whether --json is passed.
- Validation commands:
  - go test ./internal/app -run 'Contract|Plan|Config|Review'
  - go test ./...
  - go build ./...
  - make check

## Current review findings
- Summary: findings=9 open=9 resolved=0 blocking_open=9
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=high category=correctness blocking=true status=open: Plan review can exit clean after a reviewer still fails to emit the mandatory findings block on the corrective retry.
    location: internal/app/plan_review.go:284
  - f_002 severity=high category=correctness blocking=true status=open: Plan review silently treats a reviewer response that still lacks the mandatory findings block after corrective retry as a clean review.
    location: internal/app/plan_review.go:284
  - f_003 severity=high category=correctness blocking=true status=open: The plan-review prompt contradicts the contract by instructing no-issue reviewers not to include the mandatory findings block.
    location: internal/app/plan_review.go:485
  - f_004 severity=medium category=correctness blocking=true status=open: Plan-review findings do not enforce the promised lens enum before writing JSON output.
    location: internal/app/plan_review.go:388
  - f_005 severity=medium category=quality blocking=true status=open: The no-findings plan-review test treats a missing structured findings block as a clean review, instead of requiring an empty pactum.reviewer_findings.v1alpha1 block.
    location: internal/app/plan_review_test.go:224
  - f_006 severity=medium category=quality blocking=true status=open: There is no regression test that pipeline.plan_review.by accepts and runs multiple configured reviewer agents.
    location: internal/app/config_test.go:245
  - f_007 severity=medium category=correctness blocking=true status=open: Plan review silently treats missing structured reviewer output as a clean review.
    location: internal/app/plan_review.go:260
  - f_008 severity=medium category=quality blocking=true status=open: The new plan-review CLI and pipeline stage are user-visible but are not documented in the user-facing workflow/config docs.
    location: internal/app/cli.go:79
  - f_009 severity=medium category=quality blocking=true status=open: The new VACUOUS_VALIDATION scoped-validation rule is not documented in the user-facing plan documentation.
    location: internal/app/contract.go:917
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - none

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
