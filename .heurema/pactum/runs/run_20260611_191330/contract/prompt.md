# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260611_191330
- Approval: approved
- Contract hash: 3abf6ae9e15899817ca38f84464fa680975b8aec10251c8c5337b07c1ba953d5

## Goal
Two small CI hardening items from the project audit. (1) Vulnerability scanning: add govulncheck to the repository toolchain the same way deadcode is already wired — a tool directive in go.mod so the version is pinned and reproducible, a Makefile target (e.g. make vuln) running go tool govulncheck ./..., and a CI step invoking it (decide whether it joins the existing check job or runs as its own job so a slow vulndb fetch does not slow the main feedback loop; prefer a separate job). (2) Committed run-record hygiene gate: dogfood batches commit .heurema run records, and today the only protection against leaking absolute local paths or credentials from agent transcripts is a manual grep convention. Add a make target (e.g. make heurema-hygiene) that deterministically scans the tracked .heurema tree for absolute home-directory paths (/Users/, /home/, C:\Users) and credential-shaped strings (common token prefixes such as ghp_, github_pat_, sk-, AKIA, xox, -----BEGIN ... PRIVATE KEY-----, and Authorization: Bearer headers), exits nonzero listing every offending file and match, and runs as a CI step; allow a narrowly-scoped allowlist file for documented false positives if one proves necessary, but prefer zero allowlist entries. The scan must not flag the patterns inside its own definition (Makefile/script) or inside test fixtures outside .heurema. Document both targets briefly where the repo documents its make targets, and ensure make check stays unchanged (both are additive).

## In scope
- Pin `govulncheck` in `go.mod` using a Go tool directive, mirroring the existing `deadcode` tool wiring.
- Add an additive `make vuln` target that runs `go tool govulncheck ./...`.
- Add an additive `make heurema-hygiene` target that scans tracked `.heurema` files plus staged-added `.heurema` files, excluding unrelated untracked files.
- Add CI coverage for `make vuln` as a separate blocking job and CI coverage for `make heurema-hygiene` without changing the existing `make check` target behavior.
- Document the new `vuln` and `heurema-hygiene` make targets where repository make targets are documented.
- Add focused automated coverage or deterministic fixtures for the `.heurema` hygiene scanner behavior.

## Out of scope
- Changing the contract goal or clarification answers.
- Adding `make vuln` or `make heurema-hygiene` as dependencies of `make check`.
- Running real Pactum agents or review agents.
- Scanning unrelated untracked worktree files under `.heurema`.
- Scanning detector definitions in the Makefile, helper scripts, documentation, or fixtures outside `.heurema`.

## Acceptance criteria
- `go.mod` contains a pinned tool directive for `golang.org/x/vuln/cmd/govulncheck`, and any required module checksum updates are committed.
- `make vuln` invokes `go tool govulncheck ./...` and returns the tool's nonzero exit status for vulnerability findings, fetch failures, or tool failures.
- The GitHub Actions workflow has a separate blocking vulnerability job that checks out the repository, sets up Go from `go.mod`, downloads modules as needed, and runs `make vuln`.
- `make heurema-hygiene` exits zero when the tracked plus staged-added `.heurema` file set contains no forbidden findings.
- `make heurema-hygiene` exits nonzero when any scanned `.heurema` file contains an absolute home-directory path matching `/Users/`, `/home/`, or `C:\Users`, and reports every finding with `file:line`, detector name, and a redacted preview.
- `make heurema-hygiene` exits nonzero when any scanned `.heurema` file contains plausible credential-shaped material for the listed detector families, including GitHub tokens, OpenAI-style `sk-` tokens, AWS `AKIA` access keys, Slack `xox` tokens, private-key blocks, or `Authorization: Bearer` headers with token material.
- The hygiene report never prints a full secret value or full absolute home-directory path in its output.
- Bare documentation examples such as `sk-`, `ghp_`, `/Users/`, and `Authorization: Bearer` without token material are not reported as credential findings.
- The hygiene scanner considers files returned by `git ls-files .heurema` and staged-added `.heurema` files, and does not consider unrelated untracked `.heurema` worktree files.
- If an allowlist file is introduced, it is narrowly scoped, documented, and contains no entries unless a concrete false positive requires one.
- `make check` remains functionally unchanged: it still runs the existing test, vet, deadcode, and whitespace/conflict-marker checks without depending on the new vulnerability or hygiene targets.
- Repository documentation that lists useful make targets includes brief entries for `make vuln` and `make heurema-hygiene`.

## Validation commands
- make check
- make heurema-hygiene
- make vuln

## Assumptions
- The preferred `govulncheck` tool module path is `golang.org/x/vuln/cmd/govulncheck`.
- CI has network access to fetch the Go vulnerability database, and a transient fetch failure should fail the separate vulnerability job.
- Focused scanner tests may create temporary git repositories or fixtures outside `.heurema` as long as the production target only scans the specified `.heurema` file set.

## Clarifications
- q_001 [blocking] For `make heurema-hygiene`, should 'tracked .heurema tree' mean only files already returned by `git ls-files .heurema`, staged additions under `.heurema`, or every non-ignored worktree file under `.heurema`?
  Rationale: The contract says 'tracked .heurema tree', while docs/backlog discuss committed durable run records and staged/changed `.heurema` content. This affects whether local pre-commit use catches newly added run files before they are committed, and whether untracked scratch files are considered.
  Decision: Scan tracked `.heurema` files plus staged-added `.heurema` files in local runs; in CI this naturally scans the checked-out tracked `.heurema` files. Do not scan unrelated untracked worktree files.
- q_002 [blocking] Should 'credential-shaped strings' mean literal occurrences of listed prefixes like `sk-`, `ghp_`, `AKIA`, and `Authorization: Bearer`, or only plausible secrets with those prefixes plus enough token material or header value to look real?
  Rationale: The current `.heurema` run record contains descriptive examples of the detector terms. Literal prefix matching would flag the contract/task text itself, while the stated intent is to catch leaked credentials from committed run records.
  Decision: Treat the list as detector families, not literal forbidden substrings: flag plausible secret-shaped values with enough following token material or a private-key block, but do not flag bare documentation examples such as `sk-`, `ghp_`, `/Users/`, or `Authorization: Bearer` without a token.
- q_003 [blocking] When the hygiene gate reports a credential or absolute home path, should it print the full offending match, or redact sensitive parts while still listing the file, line, and detector?
  Rationale: The contract says to list every offending file and match, but printing a real secret or full local path into CI logs can create a second leak. The implementation needs a concrete reporting rule.
  Decision: Report `file:line`, detector name, and a redacted match preview that preserves enough context to find the issue without printing full credentials or full home-directory paths.
- q_004 If `govulncheck` cannot fetch vulnerability data in CI because of a transient network or database error, should the separate vulnerability job fail the PR or be allowed to pass with a warning?
  Rationale: The contract asks for a CI step and prefers a separate job because vulndb fetches may be slow, but it does not state failure policy for fetch/tool errors versus actual vulnerability findings.
  Decision: Run `govulncheck` in a separate blocking CI job; any nonzero `govulncheck` result, including fetch/tool errors, fails the job so failures are visible and retried explicitly.

## Project context
- Executor context: context/executor-context.md
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json
- Accepted memory context: context/memory-context.md

## Accepted memory

Memory context:
- context/memory-context.md

Selected memory:
- total: 5
- fresh: 4
- stale: 1
- unknown: 0

Items:
- mem_005 [fresh] score=39 — Make the CLI announce legal moves so an agent never guesses the pipeline stat...
- mem_007 [fresh] score=38 — Fix three valid external review findings. (1) pactum export must preserve its...
- mem_002 [stale] score=34 — Normalize the CLI command grammar for agent-first use: every stage exposes a ...
  reason: missing file internal/app/agents_doctor.go
  reason: missing file internal/app/agents_doctor_test.go
- mem_001 [fresh] score=28 — Add an export command that dumps a run's full record as a single archive
- mem_006 [fresh] score=24 — Smooth the pipeline so no command is pure ritual, then compress the agent ski...

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
