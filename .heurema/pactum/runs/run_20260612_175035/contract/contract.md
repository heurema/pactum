# Contract Draft

## Goal
Stagger the cold start of same-model reviewer groups in the review panel fan-out to stop paying duplicate prompt-cache write premiums. Background (verified research, recorded in docs/cost-budget-design.md): Anthropic prompt-cache entries become usable only after the first response begins, parallel Claude Code sessions in the same directory read each other's cache, and the model is part of the effective cache key; today every review round launches all member-by-lens attempts simultaneously, so five concurrent claude-engine lens attempts each pay the 1.25x cache-write premium on the same shared prefix (system + tools + CLAUDE.md, roughly 25k tokens) — a staggered launch (1 write + 4 reads) instead of 5 writes saves about 74 percent of the prefix cost per claude round. Behavior: when the review fan-out spawns lens attempts, group them by the resolved registry entry's (inferred engine, model, effort); for groups whose inferred engine is claude and whose size exceeds one, launch exactly one attempt first and hold the rest; release the held attempts concurrently as soon as the first attempt's first streamed output chunk arrives (over ACP that is the first agent message text written to the attempt log — the existing transport already observes it), or immediately if the first attempt terminates before producing output, or after a hold timeout of 60 seconds so a silent first attempt can never serialize the panel. Codex-engine groups launch unchanged (no benefit: codex sets a per-thread prompt_cache_key; no cost: OpenAI charges no write premium). Single-attempt groups and the fixer are unaffected. This is built-in default behavior like the lens fan-out itself — no config knob; the live output prints one line when a group is held and when it is released so a watching operator understands the pause. The hold must not change attempt artifact naming, ordering of recorded attempts, or proposal collection semantics. Tests pin: claude-model groups launch one-then-rest on first output; the timeout and the early-termination releases both work; codex groups and single-attempt groups launch immediately; recorded attempt artifacts and review semantics are byte-compatible with the unstaggered path.

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260612_165834_02
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (0 result(s))

## Clarifications
- q_001 [blocking] — When the contract says "review panel fan-out," should the stagger apply to every `review run` reviewer lens fan-out whose grouped Claude attempts exceed one, including an explicit single Claude reviewer that expands into the five lenses, or only to configured multi-member `review.panel` runs?
  Rationale: The repo uses `review.panel` for configured reviewer rosters, but docs and code also define every resolved reviewer as expanding into five concurrent lens attempts. The cost example cites five concurrent Claude lens attempts, which can happen with a single explicit Claude reviewer, not only a multi-member panel.
  Answer: Apply the stagger to all `review run` reviewer lens attempts after roster resolution: explicit reviewer, configured panel, and empty-panel cross-model fallback. Keep `review plan`, proposal collection commands, and the fixer out of scope.
- q_002 [blocking] — If two different registry names resolve to the same Claude model and effort, should their member-times-lens attempts form one shared stagger group, or should each registry name stagger independently?
  Rationale: The repo allows two registry names backed by the same engine and model pins. The contract says group by `(inferred engine, model, effort)`, but the phrase "resolved registry entry's" could be read as preserving registry-name boundaries.
  Answer: Group across the whole review round by normalized `(engine, model, effort)` only, not by registry name. Two names with the same Claude model and effort should share one group with one lead attempt and all other attempts held.
- q_003 [blocking] — What should "recorded attempt artifacts and review semantics are byte-compatible" mean, given that staggered launches necessarily change start times, durations, and add live-output hold/release lines?
  Rationale: Attempt result JSON records timestamps and durations, so literal byte equality with the unstaggered path is not achievable. The repo already relies on stable attempt IDs, paths, request documents, summary ordering, and proposal decisions as the meaningful compatibility surface.
  Answer: Require byte-compatible schemas, artifact paths, attempt ID ordering, request prompt references, round summary ordering, proposal parsing, and proposal decision semantics. Allow timestamps, durations, usage values, scheduling order, and new live-output hold/release lines to differ.
- q_004 — Should the first-output release trigger be implemented for the CLI transport debug path as well as the default ACP transport?
  Rationale: ACP is the default and already observes agent message chunks. The repo still supports `PACTUM_AGENT_TRANSPORT=cli`, whose visible output may not correspond to ACP agent-message chunks and may arrive only at process completion for structured Claude output.
  Answer: Implement the stagger through a transport-agnostic first-visible-output callback. ACP should fire it on the first non-empty agent message chunk written to `stdout.log`; CLI should fire it on the first non-empty stdout or stderr write. Completion-before-output and the fixed 60-second hold timeout remain fallback releases.
- q_005 — Should the contract explicitly include documentation updates for the changed reviewer launch behavior?
  Rationale: `docs/agents.md` currently says each review round launches lens attempts concurrently, and `docs/cost-budget-design.md` describes staggering as a planned future slice. The feature changes user-visible timing and live output.
  Answer: Include docs updates in scope: update `docs/agents.md` to describe same-model Claude stagger behavior and update `docs/cost-budget-design.md` from planned slice to implemented behavior.

## In scope
- Implement `review run` reviewer lens scheduling after roster resolution for explicit reviewers, configured panels, and empty-panel cross-model fallback.
- Group reviewer lens attempts across the whole review round by normalized `(engine, model, effort)`, independent of registry name.
- For Claude groups with more than one attempt, launch exactly one lead attempt, hold the rest, and release held attempts concurrently on first visible output, lead completion before output, or a 60 second hold timeout.
- Add a transport-agnostic first-visible-output callback: ACP fires on the first non-empty agent message chunk written to `stdout.log`; CLI fires on the first non-empty stdout or stderr write.
- Emit live output lines when a Claude group is held and when it is released.
- Add tests covering Claude first-output release, timeout release, completion-before-output release, cross-registry grouping by normalized model and effort, Codex immediate launch, and single-attempt immediate launch.
- Update `docs/agents.md` and `docs/cost-budget-design.md` to describe the implemented same-model Claude review stagger behavior.

## Out of scope
- `review plan`, proposal collection commands, proposal accept/reject commands, fixer execution, execute stages, clarify stages, and contract-draft stages.
- Adding a config knob, environment flag, or user-facing option for enabling or disabling staggered review launches.
- Changing prompt contents, attempt artifact naming, attempt ID allocation order, reviewer lens set, model resolution rules, or Codex prompt cache key behavior.
- Running real `pactum review run` agent subprocesses as validation without explicit human approval.

## Acceptance criteria
- A `review run` with a multi-attempt Claude group starts exactly one transport invocation for that normalized `(engine, model, effort)` group before any held attempts start.
- Held Claude attempts are not invoked until the lead attempt produces first visible output, exits before visible output, or the 60 second timeout elapses.
- When release is triggered, all held attempts in the Claude group are launched without intentional serialization.
- Two different reviewer registry names resolving to the same Claude model and effort share one stagger group with one lead attempt.
- Codex groups, non-Claude groups, and single-attempt groups launch immediately with no stagger hold.
- Artifact schemas, artifact paths, attempt ID ordering, request prompt references, round summary ordering, proposal parsing, and proposal decision semantics remain compatible with the unstaggered path; timestamps, durations, usage values, scheduling order, and new live-output hold/release lines may differ.
- `docs/agents.md` no longer describes all review lens attempts as always launching concurrently without qualification, and `docs/cost-budget-design.md` describes the Claude stagger as implemented rather than only planned.

## Validation commands
- go test ./internal/app ./internal/agents
- make check

## Assumptions
- Existing test doubles can observe transport invocation ordering without launching real agents.
- A fixed 60 second production timeout can be tested through an injectable clock or timeout duration so tests do not actually wait 60 seconds.
- Normalized effort uses the same resolved value currently recorded for reviewer attempts and registry/model inference.

## Open questions
- None
