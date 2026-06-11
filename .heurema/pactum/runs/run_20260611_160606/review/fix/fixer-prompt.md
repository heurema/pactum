# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260611_160606/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260611_160606/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260611_160606/review/review.json, .heurema/pactum/runs/run_20260611_160606/review/findings.jsonl, .heurema/pactum/runs/run_20260611_160606/review/resolutions.jsonl

## Approved contract
- Goal: Smooth the pipeline so no command is pure ritual, then compress the agent skill against the final grammar. This is the last grammar break; hard removals, no aliases. (1) review run absorbs the loop: pactum review run becomes today's review loop semantics — implicit prepare when the gate report exists, panel-times-lenses rounds, fixer, severity-gated convergence; the single panel round without a fixer remains available as review run --no-fix --max-rounds 1; the review loop command is removed (parser error). (2) review prepare is removed: review run prepares implicitly, and manual review mutations (review finding add) self-scaffold the review when a gate report exists; the review_not_prepared error code disappears — the remaining precondition is gate_report_missing, and affordance fix/next values must reflect that. (3) prompt build self-heals a stale project map: when the map is stale it runs the refresh itself, says so in human output, exposes it in --json, and records it in the prompt manifest; the project_map_stale failure disappears from prompt build (other commands keep it). (4) clarify suggest is removed: its semantics fold into clarify run --no-auto --max-rounds 1 (new --no-auto flag disables auto-resolution of high-confidence recommendations). (5) clarify answer gains --recommended: pactum clarify answer <q_id> --recommended records that question's stored recommended answer as the answer (error when the question has no recommendation), and pactum clarify answer --all-recommended answers every open question that carries a recommendation, skipping and reporting those without; both are decision verbs honoring --by and recording answer provenance distinguishable from a typed answer. (6) The skill is rewritten against the final grammar: assets/agent-skills/pactum/SKILL.md, references/workflow.md, references/safety.md, and docs/agent-skill.md compress to the final convention — every stage exposes run and show, decision verbs relay explicit human decisions with --by, agents read next and error.fix instead of memorizing stage order, execute run is unsandboxed per SECURITY.md. (7) All Next: hints, affordance next/fix command strings, docs (flow.md, agents.md, README if affected), and tests speak only the new surface; negative parser coverage for the removed spellings (review loop, review prepare, clarify suggest) and forbidden-phrase guards in the docs/skill tests. Existing JSON schema names and artifact paths unchanged; review plan and review fix run/apply stay as surgical commands.
- In scope:
  - Remove the `pactum review loop`, `pactum review prepare`, and `pactum clarify suggest` command spellings from the CLI grammar so they fail as parser errors with no aliases.
  - Make `pactum review run` absorb the existing review loop behavior, including reviewer panel and lenses rounds, fixer invocation, convergence over open blocking findings, and support for `--reviewer`, `--agent`, `--max-rounds`, `--patience`, `--clean-rounds`, `--timeout`, `--json`, and new `--no-fix`.
  - Make `review run`, `review finding add`, and `review approve` auto-scaffold `review/review.json` when `gate/report.json` exists; keep `review status/show` read-only and show a derived empty pending review state when no review artifact exists.
  - Remove `review_not_prepared` as a user-facing error path and update remaining review preconditions, affordance `next`, and affordance `fix` values to use `gate_report_missing` or ordinary missing-record errors as appropriate.
  - Make `pactum prompt build` refresh a stale project map itself, report the refresh in human output, expose a `map_refresh` object in `--json`, and write the same additive object to `contract/prompt-manifest.json` while preserving existing schema names and artifact paths.
  - Replace `clarify suggest` behavior with `pactum clarify run --no-auto --max-rounds 1`, using the clarify-loop output and `clarify/loop-summary.json` artifact surface.
  - Add `pactum clarify answer <q_id> --recommended` and `pactum clarify answer --all-recommended` decision verbs that honor `--by`, record recommended answers, preserve typed/manual and auto-loop provenance, and persist distinguishable recommended-answer provenance.
  - Rewrite `assets/agent-skills/pactum/SKILL.md`, `assets/agent-skills/pactum/references/workflow.md`, `assets/agent-skills/pactum/references/safety.md`, and `docs/agent-skill.md` to describe only the final grammar and current safety model.
  - Update `docs/flow.md`, `docs/agents.md`, `README.md` if affected, CLI help, `Next:` hints, affordance command strings, and tests so they mention only the new command surface.
- Out of scope:
  - Do not add broad new stage aliases or unrelated command families such as `pactum contract run`, `pactum prompt run`, or `pactum memory run`.
  - Do not rename unrelated existing stage verbs such as `prompt build`, `execute plan`, `gate run`, `memory propose`, or `memory accept`.
  - Do not change existing JSON schema names or artifact paths except for additive fields required by this contract.
  - Do not introduce a new severity threshold for review convergence; convergence and fixer invocation remain based on open findings with `blocking=true`.
  - Do not change `review plan` or the surgical `review fix run/apply` commands except where needed for help text, precondition errors, and removed `review prepare` references.
  - Do not preserve compatibility aliases for `review loop`, `review prepare`, or `clarify suggest`.
- Acceptance criteria:
  - `pactum review loop`, `pactum review prepare`, and `pactum clarify suggest` fail at parse time, and negative CLI grammar tests cover all three removed spellings.
  - `pactum review run` accepts the former loop control flags plus `--no-fix`, runs loop semantics by default, writes the loop summary, and converges based on open blocking findings rather than raw severity values.
  - `pactum review run --no-fix --max-rounds 1` runs one reviewer panel/lenses pass, accepts valid finding proposals into review findings, skips fixer execution, exits 0 when findings remain open, records a terminal reason such as `findings_open` or `no_fix`, and points `next` to `pactum review show <run_id>`.
  - `pactum review run --no-fix --max-rounds 2` stops after the first round that leaves open blocking findings instead of continuing reviewer-only churn.
  - `review run`, `review finding add`, and `review approve` create the review scaffold when `gate/report.json` exists; `review finding resolve`, proposal accept/reject, and `review fix run/apply` still require relevant existing review records.
  - `review status` and `review show` do not mutate files and show a derived empty pending review state for a gated run with no review artifact.
  - `project_map_stale` is no longer a `prompt build` failure; when a stale map is detected, `prompt build` refreshes the map, human output names the refresh and new map run id, `--json` includes `map_refresh`, and `contract/prompt-manifest.json` includes the same additive `map_refresh` object.
  - `map_refresh` is `{ "triggered": false }` when no prompt-build refresh was needed, and `{ "triggered": true, "reason": "project_map_stale", "previous_map_run_id": "...", "run_id": "..." }` when a refresh occurred.
  - `pactum clarify run --no-auto --max-rounds 1` runs one clarifier round, records created questions, performs no auto-resolution, writes `clarify/loop-summary.json`, and reports `terminal_reason: needs_human` when open blocking questions remain.
  - `pactum clarify answer <q_id> --recommended` errors when the question is not currently open, has no non-empty stored recommendation, or is blocked by unanswered dependencies; it records `source: manual_recommended` in `answers.jsonl` and `source: manual_recommended_answer` in `decisions.jsonl` when successful.
  - `pactum clarify answer --all-recommended` answers currently open recommended questions in dependency order, skips open questions without recommendations and dependency-blocked questions, reports skipped IDs in human and JSON output, and records `source: manual_all_recommended` / `manual_all_recommended_answer` for each recorded answer.
  - All recommended-answer decision paths normalize and persist `decided_by` from `--by`.
  - Docs and agent skill files contain no instructions to use `review loop`, `review prepare`, or `clarify suggest`, and describe agents reading `next` and `error.fix` rather than memorizing stage order.
  - `execute run` safety language in the skill and docs matches `SECURITY.md` and states that real agent execution is unsandboxed.
- Validation commands:
  - make check
  - go test ./...
  - rg "review loop|review prepare|clarify suggest|review_not_prepared|project_map_stale" docs assets/agent-skills README.md internal/app

## Current review findings
- Summary: findings=11 open=11 resolved=0 blocking_open=1
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_010 severity=medium category=correctness blocking=true status=open: prompt build human output still prints `pactum execute plan --agent codex` without the run id, while the final affordance surface requires concrete runnable Next commands such as `pactum execute plan <run_id> --agent codex`.
    location: internal/app/prompt.go:556
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_001 severity=medium category=validation blocking=false status=open: Gate validation runner tokenizes commands with strings.Fields (quote-blind), so the contract's quoted rg validation command executes as pattern '"review' with the remaining pattern fragments treated as nonexistent paths. rg exits 2 every time, the gate permanently reports failed for this contract, and review approval is blocked. The intended forbidden-phrase check never actually ran.
    location: internal/app/gate.go:535
  - f_002 severity=medium category=validation blocking=false status=open: Gate validation command_003 never tested the contract's intent: the gate splits commands with strings.Fields (no shell-quote handling), so the quoted rg pattern fragmented — rg searched for the literal pattern '"review' and treated 'loop|review', 'prepare|clarify', 'suggest|review_not_prepared|project_map_stale"' as file paths, exiting 2 with 'No such file or directory'. Independently, the command's intent is zero matches, but rg exits 1 on zero matches and the gate treats nonzero exit as failure — so this command structurally fails on a clean tree. The run cannot converge through a passing gate with this command as written; replace it (e.g. invert the exit code) or accept the gate manually.
    location: internal/app/gate.go:535
  - f_003 severity=low category=scope blocking=false status=open: Removed-grammar phrases remain in docs outside the test guards: docs/real-agent-execution-dogfood.md lines 65/119/122 still name 'pactum review prepare' and 'review prepare' (file untouched by this change), and docs/backlog.md lines 154/190/379/559/562 keep 'clarify suggest' plus 'review loop' concept mentions (file edited by this change, mentions kept as historical record). The contract's rg validation sweeps all of docs and contract assumption 3 expects a human review of remaining matches — which never happened because command_003 misfired. Human should accept these as intentional historical fixtures or scope the validation command to the guarded doc set.
    location: docs/real-agent-execution-dogfood.md:65
  - f_004 severity=medium category=quality blocking=false status=open: Acceptance criterion 'review finding resolve, proposal accept/reject, and review fix run/apply still require relevant existing review records' has no test at the new boundary. requireReviewPrepared was removed from these paths and the old *BeforeReviewPreparedFails tests were deleted without replacements that run these commands on a gated run with no review artifacts. All current tests pre-scaffold via scaffoldReviewForTest, so nothing pins that these commands fail with ordinary missing-record errors and do not self-scaffold review/review.json; switching them to ensureReviewRecord would regress the criterion with tests green.
    location: internal/app/review.go:483
  - f_005 severity=low category=quality blocking=false status=open: Four new hand-written CLI error paths in clarifyAnswerCmd.Run are untested: the --recommended/--all-recommended conflict (line 69), the --all-recommended usage string (line 73), the --recommended usage string (line 83), and the q_ prefix check in the --recommended branch (line 87). TestUsageErrorsAdvertiseNewGrammar (cli_grammar_test.go:169) exists to pin every hand-written usage-error string but was not extended with the new strings.
    location: internal/app/commands.go:69
  - f_006 severity=low category=quality blocking=false status=open: Constant reviewerLastResultArtifact is dead code: its only consumer (writeReviewRun, deleted with the old reviewer-only ReviewRun) is gone. Unused package-level constants are not caught by go vet or the deadcode gate, so it will linger silently. Delete it.
    location: internal/app/review.go:27
  - f_007 severity=low category=quality blocking=false status=open: Constant clarifierLastResultArtifact is dead code: its only consumer (writeClarifySuggest, deleted when clarify suggest was removed and the round primitive became JSON-only) is gone. Delete it.
    location: internal/app/clarify_round.go:20
  - f_008 severity=low category=quality blocking=false status=open: README quick-start says 'Mutating review commands self-scaffold the review record once the gate report exists', but only review run, review finding add, and review approve scaffold (ensureReviewRecord call sites in internal/app/review.go); review finding resolve — shown one line below — and proposal accept/reject require existing records per the contract's q_001 answer and acceptance criteria. docs/flow.md:197-199 states the precise list; the README contradicts it.
    location: README.md:173
  - f_009 severity=low category=quality blocking=false status=open: The agent-skill workflow reference says 'the mutating review commands self-scaffold the review record' and then lists review finding resolve among them; resolve does not scaffold and requires existing records. Agents act on this file, so the overgeneralization can mislead an agent into expecting resolve/proposal-accept to work on a gated run with no review data.
    location: assets/agent-skills/pactum/references/workflow.md:85
  - f_011 severity=low category=quality blocking=false status=open: `clarify run` still routes its only internal clarifier round through a stdout JSON serialization/unmarshal adapter after removing the user-facing `clarify suggest` command.
    location: internal/app/clarify_loop.go:242

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
  "schema": "pactum.review_fix_outcomes.v1",
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
