# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260609_163022
- Approval: approved
- Contract hash: dc45d5b3b69f565d884056cad96ebe863bae20a57294008cc6bdba9a7886a0d9

## Goal
Add dependency-ordered questioning to clarify (grill-me slice 2). The clarifier orders its proposed questions foundational-first and declares, for any question whose framing or answer hinges on another, a depends_on referencing the earlier question(s) it depends on. Because pactum assigns the q_NNN ids only after parsing, the agent references earlier questions by their 1-based position in the emitted sequence; pactum resolves those positions to the assigned question ids, and marks a question 'blocked' when any prerequisite is still unanswered. Additive and behavior-compatible (no schema-version bump). Captures + displays the dependency structure; this slice does not auto-defer or hide dependent questions in the loop.

## In scope
- Add depends_on to the clarifier suggestion input (clarifierSuggestionInput in clarify_suggest.go) as a list of 1-based positions ([]int, json depends_on), referencing EARLIER questions in the same emitted sequence; add DependsOn []string (resolved question ids, json depends_on,omitempty) to the persisted clarificationQuestionRecord (clarify.go) and to the clarifyQuestionStatus view, plus a Blocked bool on the view.
- In recordClarifierSuggestions (clarify_suggest.go), resolve depends_on as questions are created: maintain a per-attempt map from each emitted question's 1-based position to its assigned id (or a skipped sentinel). For the question at position p, resolve each depends_on entry d by: dropping it (with a 'dependency dropped' style warning, keeping the question) if d < 1, d >= p (not strictly earlier / self / forward reference), d is out of range, or position d was skipped; otherwise add the id at position d to the record's DependsOn. depends_on is optional; a question may declare none.
- Keep clarificationQuestionFromSuggestion focused on the existing per-field validation (text/blocking/rationale/recommended_answer/confidence) — resolve DependsOn in the caller (recordClarifierSuggestions) where the position context lives — and set it on the record.
- In buildClarificationStatus (clarify.go): pass DependsOn through to clarifyQuestionStatus, and set Blocked=true for an open question that has at least one prerequisite (depends_on id) whose question is not answered. Do NOT change the open/answered status values or the Open/Answered/BlockingOpen counters (a blocked question is still counted as open/blocking); Blocked is an additional display flag.
- Display the dependency in writeClarifyStatus and renderClarifierContext: under each question show its depends_on ids and, when Blocked, annotate that it is waiting on unanswered prerequisites. Keep the existing question ordering (ids are already foundational-first because the agent orders them that way).
- Update renderClarifierPrompt: instruct the clarifier to (a) order questions foundational-first, and (b) for any question that depends on another in the same block, set depends_on to that earlier question's 1-based position; add an optional depends_on field to the example JSON with a one-line explanation that it lists 1-based positions of earlier questions.
- Update the clarify tests (clarify_suggest_test.go, agent_output_test.go as needed) to cover depends_on resolution (valid earlier reference resolves to the right id; a forward/self/out-of-range reference is dropped with a warning while the question is still recorded; a question depending on an unanswered one is marked Blocked). Update docs/backlog.md to mark grill-me slice 2 shipped.

## Out of scope
- Do NOT auto-defer, hide, or gate dependent questions in the clarify loop or change which questions the clarifier is asked to emit per round — this slice only captures, resolves, and displays the dependency + blocked flag.
- Do not implement grill-me slices 3-5 (terminology, edge-cases, convergence); do not bump the clarification schema-version constants; do not change files outside internal/app/clarify.go, internal/app/clarify_suggest.go (and their _test.go), and docs/backlog.md.

## Paths in scope
- internal/app/clarify.go
- internal/app/clarify_suggest.go
- internal/app/clarify_suggest_test.go
- internal/app/agent_output_test.go
- docs/backlog.md


## Acceptance criteria
- A clarifier question's depends_on (1-based earlier positions) is resolved to the assigned question ids on the persisted record; a forward/self/out-of-range/skipped reference is dropped with a warning and the question is still recorded.
- clarify status and the clarifier-context list show each question's resolved depends_on ids and flag a question Blocked when a prerequisite is unanswered; the Open/Answered/BlockingOpen counters and the open/answered status values are unchanged.
- renderClarifierPrompt instructs foundational-first ordering and depends_on by 1-based earlier position, and its example JSON includes the optional depends_on field.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes with the clarify tests covering depends_on resolution + the Blocked flag; docs/backlog.md marks slice 2 shipped.

## Validation commands
- go build ./...
- go test ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- Referencing earlier questions by 1-based emission position (resolved by pactum to ids) is necessary because ids are assigned only after parsing; restricting refs to strictly-earlier positions makes the dependency graph acyclic and resolvable in a single forward pass.
- Marking blocked is display-only this slice; the loop's convergence still treats a blocked blocking question as an open blocking question.

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
