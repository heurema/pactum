# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260609_201739
- Approval: approved
- Contract hash: 139ddc4599a86cc29aa7b86cd554eec69d97fe74e0c4369b225b4526ba0c97fb

## Goal
Make the clarify-resets-approval behavior visible instead of silent. When clarify ask/answer/suggest add or update clarifications on an already-approved run, refreshClarificationArtifacts calls resetApprovalIfApproved, which regresses the run to clarifying and writes a contract_approval_reset ledger event — but refreshClarificationArtifacts discards the reset bool, so the user is never told their approved run was regressed. Thread that existing approvalReset bool up through refreshClarificationArtifacts into the clarify ask/answer/suggest responses and warn in their output. Do NOT block the operation — re-clarifying an approved run is legitimate; only its silence is the bug.

## In scope
- refreshClarificationArtifacts (internal/app/clarify.go): capture the approvalReset bool that resetApprovalIfApproved already returns (currently discarded via 'if _, _, err :=') and add it to the function's return signature, e.g. (clarifyStatusResponse, bool, error).
- Update the three callers — ClarifyAsk (clarify.go), ClarifyAnswer (clarify.go), and recordClarifierSuggestions (clarify_suggest.go) — to receive the bool. recordClarifierSuggestions must propagate it out (extend its return signature) so ClarifySuggest can set it; note that when it records zero questions it returns early WITHOUT calling refreshClarificationArtifacts, so approvalReset is false in that path.
- Add an ApprovalReset bool field (json 'approval_reset,omitempty') to clarifyAskResponse, clarifyAnswerResponse, and clarifySuggestResponse, set from the threaded bool.
- Warn clearly in writeClarifyAskResponse, writeClarifyAnswerResponse, and writeClarifySuggest when ApprovalReset is true: state that the run was approved and is now back to clarifying (approval reset to pending), and to re-approve with 'pactum contract approve <run>' once clarifications are resolved.
- Tests (clarify_suggest_test.go / a clarify_test.go): a clarify ask/answer/suggest on an APPROVED run sets ApprovalReset=true, emits the warning, and still records the contract_approval_reset ledger event; the same on a non-approved run leaves ApprovalReset false (json omitted) with no warning and otherwise-unchanged behavior.

## Out of scope
- Do NOT change which commands reset approval, the reset mechanics (resetApprovalIfApproved), or the ledger event; do NOT block/guard the operation or add a --force flag — the operation stays allowed, only its visibility changes. The separate question of whether answering a question should reset approval is out of scope.
- Do not change files outside internal/app/clarify.go, internal/app/clarify_suggest.go, and their _test.go files; do not bump any schema-version constant.

## Paths in scope
- internal/app/clarify.go
- internal/app/clarify_suggest.go
- internal/app/clarify_suggest_test.go
- internal/app/clarify_test.go


## Acceptance criteria
- clarify ask/answer/suggest on an approved run report approval_reset=true in JSON and print a warning that approval was reset (run back to clarifying) with the re-approve hint; the run regresses to clarifying and the contract_approval_reset ledger event is still recorded.
- The same commands on a non-approved run report approval_reset false (omitted from JSON) and print no such warning; all other behavior is unchanged.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes with tests covering the approved and non-approved cases for at least one mutating command.

## Validation commands
- go build ./...
- go test ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- Surfacing the reset (warn + a response field) is the right fix rather than blocking, because re-clarifying an approved run is a legitimate operation; the reset itself and its ledger event already exist and are correct.

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
