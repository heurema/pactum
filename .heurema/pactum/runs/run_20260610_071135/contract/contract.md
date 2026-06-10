# Contract Draft

## Goal
Outline next steps for test coverage

## Current status
Contract status: draft
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260610_065943
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (0 result(s))

## Clarifications
- q_001 [blocking] — Which concrete 'test coverage' is the goal about? Candidates in this repo: (a) Go unit-test coverage of the pactum codebase (go test -cover ./... — no coverage tooling exists today in the Makefile or CI); (b) end-to-end/behavioral coverage via scripts/smoke.sh and dogfood runs; (c) Pactum's own clarification 'coverage by dimension' signal (M15.5, clarify status), which is the only 'coverage' the backlog currently mentions.
  Rationale: The goal's only noun phrase is overloaded. docs/backlog.md uses 'coverage' exclusively for the clarification-dimension signal, while the codebase has zero Go coverage tooling and several packages with no test files at all (internal/store, internal/ledger, internal/artifacts, internal/version, cmd/pactum). Which meaning is intended changes the entire deliverable.
  Answer: pending
- q_002 [blocking] — What artifact should 'outline next steps' produce, and where should it live? Options: a new section or milestone candidates appended to docs/backlog.md (the existing planning doc, M-numbered format); a new standalone doc such as docs/test-coverage.md; or only the run report. And is the run outline-only, or should it also implement any of the steps (write tests, add a Makefile cover target)?
  Rationale: The contract has empty in/out scope, so the executor cannot tell a docs-only change from a code change. The repo convention is that docs/backlog.md carries planned work in milestone form; there is no existing testing-strategy doc. Whether implementation is in scope changes the stage permissions and the size of the run.
  Answer: pending
- q_003 [blocking] — What acceptance criteria make the outline 'done'? For example: must it (i) name concrete packages and gaps (internal/store, internal/ledger, internal/artifacts, internal/version, and cmd/pactum currently have no *_test.go files), (ii) include measured per-package coverage numbers, and (iii) prioritize the steps with rationale? And what validation command should the contract carry for a docs-only change — is 'make check passes' sufficient?
  Rationale: acceptance_criteria and validation.commands are both empty, so there is no way to gate completion. An 'outline' can range from three bullet points to a measured, prioritized plan; without criteria the reviewer has nothing to verify against.
  Answer: pending
- q_004 — May the executing agent run go test -coverprofile=cover.out ./... during the run to ground the outline in measured numbers, or must the outline be qualitative (derived only from reading the tree)? If measurement is allowed, should the coverprofile artifact stay uncommitted?
  Rationale: The contract is silent on whether executing the test suite is permitted/expected. The repo has no stored coverage baseline, so numbers exist only if the run produces them. make test (go test ./...) is the standard local gate and is safe to run, but this is an unstated premise worth confirming since it shapes acceptance criterion (ii).
  Answer: pending
- q_005 — How should the outline treat code that unit tests structurally cannot exercise in CI: real-agent subprocess execution (CI explicitly runs 'no real Codex/Claude' per ci.yml, so paths in internal/agents/runner.go and the live transports), and platform-specific files (internal/agents/process_windows.go, acp_transport_unix.go vs acp_transport_other.go on a linux/darwin-only CI)? Should these count as coverage gaps to fix, or be classified as covered-by-design via smoke.sh/dogfood and platform exclusion?
  Rationale: Raw go test -cover numbers will permanently undercount these files, and an outline that flags them as 'gaps' would propose unfixable work. The repo already has a deliberate split: hermetic unit tests plus scripts/smoke.sh for the deterministic command surface plus dogfood runs for real agents.
  Answer: pending
- q_006 — Should the outlined next steps include CI/tooling proposals — e.g. adding a 'make cover' target or a numeric coverage threshold gate in .github/workflows/ci.yml — or stay purely descriptive about test gaps? The current gate set is deliberately minimal (test, vet, deadcode, race, smoke; the Makefile calls its targets 'deliberately boring').
  Rationale: Whether a coverage gate is desirable is product intent the repo cannot settle: the Makefile and CI express a philosophy of few, strict gates, and a numeric threshold is a trade-off (guards regressions but invites low-value tests). This decides what the outline is allowed to recommend.
  Answer: pending

## In scope
TBD

## Out of scope
TBD

## Acceptance criteria
TBD

## Validation commands
TBD

## Assumptions
TBD

## Open questions
- Which concrete 'test coverage' is the goal about? Candidates in this repo: (a) Go unit-test coverage of the pactum codebase (go test -cover ./... — no coverage tooling exists today in the Makefile or CI); (b) end-to-end/behavioral coverage via scripts/smoke.sh and dogfood runs; (c) Pactum's own clarification 'coverage by dimension' signal (M15.5, clarify status), which is the only 'coverage' the backlog currently mentions.
- What artifact should 'outline next steps' produce, and where should it live? Options: a new section or milestone candidates appended to docs/backlog.md (the existing planning doc, M-numbered format); a new standalone doc such as docs/test-coverage.md; or only the run report. And is the run outline-only, or should it also implement any of the steps (write tests, add a Makefile cover target)?
- What acceptance criteria make the outline 'done'? For example: must it (i) name concrete packages and gaps (internal/store, internal/ledger, internal/artifacts, internal/version, and cmd/pactum currently have no *_test.go files), (ii) include measured per-package coverage numbers, and (iii) prioritize the steps with rationale? And what validation command should the contract carry for a docs-only change — is 'make check passes' sufficient?
- May the executing agent run go test -coverprofile=cover.out ./... during the run to ground the outline in measured numbers, or must the outline be qualitative (derived only from reading the tree)? If measurement is allowed, should the coverprofile artifact stay uncommitted?
- How should the outline treat code that unit tests structurally cannot exercise in CI: real-agent subprocess execution (CI explicitly runs 'no real Codex/Claude' per ci.yml, so paths in internal/agents/runner.go and the live transports), and platform-specific files (internal/agents/process_windows.go, acp_transport_unix.go vs acp_transport_other.go on a linux/darwin-only CI)? Should these count as coverage gaps to fix, or be classified as covered-by-design via smoke.sh/dogfood and platform exclusion?
- Should the outlined next steps include CI/tooling proposals — e.g. adding a 'make cover' target or a numeric coverage threshold gate in .github/workflows/ci.yml — or stay purely descriptive about test gaps? The current gate set is deliberately minimal (test, vet, deadcode, race, smoke; the Makefile calls its targets 'deliberately boring').
