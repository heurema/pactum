# Reviewer Context

## Run
- Run id: run_20260611_191330
- Run status: contract_approved

## Contract
- Goal: Two small CI hardening items from the project audit. (1) Vulnerability scanning: add govulncheck to the repository toolchain the same way deadcode is already wired — a tool directive in go.mod so the version is pinned and reproducible, a Makefile target (e.g. make vuln) running go tool govulncheck ./..., and a CI step invoking it (decide whether it joins the existing check job or runs as its own job so a slow vulndb fetch does not slow the main feedback loop; prefer a separate job). (2) Committed run-record hygiene gate: dogfood batches commit .heurema run records, and today the only protection against leaking absolute local paths or credentials from agent transcripts is a manual grep convention. Add a make target (e.g. make heurema-hygiene) that deterministically scans the tracked .heurema tree for absolute home-directory paths (/Users/, /home/, C:\Users) and credential-shaped strings (common token prefixes such as ghp_, github_pat_, sk-, AKIA, xox, -----BEGIN ... PRIVATE KEY-----, and Authorization: Bearer headers), exits nonzero listing every offending file and match, and runs as a CI step; allow a narrowly-scoped allowlist file for documented false positives if one proves necessary, but prefer zero allowlist entries. The scan must not flag the patterns inside its own definition (Makefile/script) or inside test fixtures outside .heurema. Document both targets briefly where the repo documents its make targets, and ensure make check stays unchanged (both are additive).
- In scope:
  - Pin `govulncheck` in `go.mod` using a Go tool directive, mirroring the existing `deadcode` tool wiring.
  - Add an additive `make vuln` target that runs `go tool govulncheck ./...`.
  - Add an additive `make heurema-hygiene` target that scans tracked `.heurema` files plus staged-added `.heurema` files, excluding unrelated untracked files.
  - Add CI coverage for `make vuln` as a separate blocking job and CI coverage for `make heurema-hygiene` without changing the existing `make check` target behavior.
  - Document the new `vuln` and `heurema-hygiene` make targets where repository make targets are documented.
  - Add focused automated coverage or deterministic fixtures for the `.heurema` hygiene scanner behavior.
- Out of scope:
  - Changing the contract goal or clarification answers.
  - Adding `make vuln` or `make heurema-hygiene` as dependencies of `make check`.
  - Running real Pactum agents or review agents.
  - Scanning unrelated untracked worktree files under `.heurema`.
  - Scanning detector definitions in the Makefile, helper scripts, documentation, or fixtures outside `.heurema`.
- Acceptance criteria:
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
- Validation commands:
  - make check
  - make heurema-hygiene
  - make vuln

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 5
- Fresh: 4
- Stale: 1
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: needs_review
- Execution attempt id: attempt_002
- Execution exit code: 0
- Validation command results:
  - command_001: make check (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: make heurema-hygiene (exit 0, timed out: false, result: gate/validation/command_002/result.json)
  - command_003: make vuln (exit 0, timed out: false, result: gate/validation/command_003/result.json)
- Change summary:
  - changed files:
    - .github/workflows/ci.yml
    - CHANGELOG.md
    - Makefile
    - README.md
    - docs/install.md
    - go.mod
    - go.sum
  - new files:
    - cmd/heurema-hygiene/main.go
    - cmd/heurema-hygiene/main_test.go
  - missing files:
    - none

## Existing manual review
- Review status: pending
- Current findings summary: findings=0 open=0 resolved=0 blocking_open=0
- Existing findings:
  - none
- Existing resolutions:
  - none
- Proposal summary: pending=0 accepted=0 rejected=0
- Existing proposals:
  - none

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
- Execution result: execute/last-result.json

## Reviewer guidance
- This context is not complete semantic truth.
- Use `pactum search "<term>"` and inspect files before proposing findings.
- Do not invent changes.
- Do not approve automatically.
- If you are not certain an issue is real after verification, do not flag it.
