# Contract Draft

## Goal
Add gofmt enforcement to the build gate. Update the Makefile so that 'make check' fails when any tracked Go file is not gofmt-formatted (add a gofmt -l step that errors on non-empty output), and apply gofmt -w to fix any existing formatting drift. Do not change unrelated gate steps.

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260613_090236
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (0 result(s))

## Clarifications
- None

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

## Open questions
- None
