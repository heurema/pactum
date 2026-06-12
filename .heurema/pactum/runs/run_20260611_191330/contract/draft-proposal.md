# Contract Draft Proposal

## Status
- Run id: run_20260611_191330
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-11T19:18:56Z

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

