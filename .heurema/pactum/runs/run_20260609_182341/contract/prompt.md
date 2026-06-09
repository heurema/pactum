# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260609_182341
- Approval: approved
- Contract hash: 70dc7b001bf9f33b2645d1f2b1140f84ec08b1eb0c23d0d50eab4e2d0d02963a

## Goal
Add terminology / domain challenge to clarify (grill-me slice 3). Introduce a 'kind' classification on every clarification question and instruct the clarifier to challenge vague or overloaded domain terms — when the contract uses an ambiguous term, ask which concrete concept is meant (anchored on the repository's real concepts/identifiers, with the candidate interpretations), tagging such questions kind=terminology. The kind field also categorizes the other questions and is the structural basis for later slices (edge-case probing, coverage signal). Additive and behavior-compatible (no schema-version bump).

## In scope
- Add a 'kind' field (string) to the clarifier suggestion input (clarifierSuggestionInput in clarify_suggest.go), the persisted clarificationQuestionRecord (clarify.go), and the clarifyQuestionStatus view. Allowed kinds: terminology, scope, acceptance, edge_case, assumption, other. Validate in clarificationQuestionFromSuggestion (add isValidClarificationKind): require kind to be one of the allowed set, else skip the question with a clear 'question skipped: kind must be one of ...' warning (mirroring the confidence check).
- Map kind through buildClarificationStatus into clarifyQuestionStatus, and display it to the human in writeClarifyStatus (per question) and in renderClarifierContext's existing-questions list.
- Update renderClarifierPrompt: (a) add a '## Challenge vague terminology' section instructing the clarifier to flag ambiguous or overloaded domain terms in the contract (goal/scope/acceptance), ask which concrete meaning is intended with the candidate interpretations named, anchor the challenge on the repository's actual concepts/identifiers (from the repo context/search), and tag such questions kind=terminology; (b) instruct that EVERY question must carry a kind from the allowed set (use 'other' when none fits); (c) add the kind field to the example JSON and a one-line note listing the allowed kinds.
- Update the clarify tests (clarify_suggest_test.go, agent_output_test.go as needed) so existing clarifier suggestions carry a valid kind and add coverage that an invalid/missing kind is rejected and that kind is persisted + surfaced in status. Update docs/backlog.md to mark grill-me slice 3 shipped.

## Out of scope
- Do NOT add structured term/candidate sub-fields — the ambiguous term and its candidate meanings live in the question text and recommended answer; only the kind tag is structured. Do not change how questions are emitted/ordered, the depends_on resolution, or the recommended_answer/confidence logic.
- Do not implement slices 4-5 (edge-case probing behavior, coverage/convergence signal) — only introduce the kind field they will build on. Do not bump the clarification schema-version constants. Do not change files outside internal/app/clarify.go, internal/app/clarify_suggest.go (and their _test.go), and docs/backlog.md.

## Paths in scope
- internal/app/clarify.go
- internal/app/clarify_suggest.go
- internal/app/clarify_suggest_test.go
- internal/app/agent_output_test.go
- docs/backlog.md


## Acceptance criteria
- Every recorded clarification question carries a kind from {terminology, scope, acceptance, edge_case, assumption, other}; a suggestion with a missing or invalid kind is skipped with a clear warning and the others are still recorded.
- clarify status and the clarifier-context list show each question's kind; the open/answered status, depends_on/Blocked behavior, and the Open/Answered/BlockingOpen counters are unchanged.
- renderClarifierPrompt instructs challenging vague terminology (anchored on repo concepts, tag kind=terminology) and a kind on every question, and its example JSON includes the kind field with the allowed set documented.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes with the clarify tests covering kind validation + surfacing; docs/backlog.md marks slice 3 shipped.

## Validation commands
- go build ./...
- go test ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- A small closed kind enum (terminology/scope/acceptance/edge_case/assumption/other) is enough to categorize clarification questions and is forward-compatible with edge-case probing (slice 4) and a per-dimension coverage signal (slice 5).
- The ambiguous term and its candidate meanings are adequately expressed in the question text + recommended answer, so a kind tag (not structured term fields) is the right level for this slice.

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
