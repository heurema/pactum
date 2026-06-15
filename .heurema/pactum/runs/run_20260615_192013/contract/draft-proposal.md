# Contract Draft Proposal

## Status
- Run id: run_20260615_192013
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-15T19:22:14Z

## In scope
- Update the Makefile check gate to run a gofmt listing check over tracked Go files.
- Make the gofmt check fail with a nonzero exit when gofmt -l reports any tracked Go file.
- Run gofmt -w on existing tracked Go files to remove current formatting drift.

## Out of scope
- Changing the contract goal.
- Changing unrelated Makefile targets or existing check steps such as test, vet, deadcode, or git diff --check.
- Changing CI workflow behavior except through CI's existing use of make check.
- Making non-formatting source behavior changes.

## Acceptance criteria
- make check still runs the existing test, vet, deadcode, and git diff --check gates.
- make check includes a gofmt -l based gate over tracked Go files and exits nonzero when that output is non-empty.
- After the change, all tracked Go files are gofmt-formatted and the direct gofmt listing check produces no output.
- The Makefile change is limited to adding gofmt enforcement to the build gate.

## Validation commands
- make check
- test -z "$(gofmt -l $(git ls-files '*.go'))"

## Assumptions
- Tracked Go files means the files returned by git ls-files '*.go' at validation time.
- The repository's active Go toolchain gofmt is the formatting authority.
- CI already invokes make check, so updating the Makefile is sufficient to enforce this in CI.

