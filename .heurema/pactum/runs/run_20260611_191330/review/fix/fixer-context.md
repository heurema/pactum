# Review Fixer Context

## Run
- Run id: run_20260611_191330
- Run status: contract_approved

## Approved contract
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

## Current review findings
- Summary: findings=9 open=9 resolved=0 blocking_open=3
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_006 severity=medium category=correctness blocking=true status=open: `heurema-hygiene` scans the worktree copy of index-selected files, so it can miss secrets that are staged for commit.
    location: cmd/heurema-hygiene/main.go:121
  - f_007 severity=medium category=correctness blocking=true status=open: The hygiene scanner selects staged .heurema paths from the Git index but scans worktree bytes, so staged secret content can be missed when the worktree differs from the index.
    location: cmd/heurema-hygiene/main.go:121
  - f_008 severity=medium category=correctness blocking=true status=open: The hygiene scanner silently skips git-index .heurema files that are missing from the worktree, so a staged-added file can still be committed while the gate reports clean if the worktree copy was deleted before running the target.
    location: cmd/heurema-hygiene/main.go:121
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_001 severity=low category=correctness blocking=false status=open: Local hygiene gate selects files from the git index (git ls-files) but scans worktree content via os.ReadFile and skips worktree-deleted files, so staged content that diverges from the worktree is not what is verified. A staged secret whose worktree copy was edited or deleted afterward passes `make heurema-hygiene` locally and gets committed; CI only catches it after the secret has been pushed to the remote. Scanning staged blob content (e.g. git show :<path> or git grep --cached) would close the gap.
    location: cmd/heurema-hygiene/main.go:121
  - f_002 severity=low category=correctness blocking=false status=open: Slack detector regexp xox[abprs]- omits the real Slack token prefixes xoxc- (client/browser), xoxd- (browser cookie), and xoxe- (rotated tokens), so such tokens in a committed run record pass the gate despite the family-level acceptance criterion for Slack xox tokens. Widening the class (e.g. xox[a-z]-) preserves the 10+-char plausibility requirement and the bare-prefix doc-example behavior.
    location: cmd/heurema-hygiene/main.go:55
  - f_003 severity=low category=quality blocking=false status=open: No automated test exercises the command-level nonzero exit path. Acceptance criteria 5-6 say 'make heurema-hygiene exits nonzero when...', but tests only assert scan()'s finding counts; the findings>0 -> os.Exit(1) mapping and the exit-2 error branches (repoRoot failure, heuremaFiles git failure, non-ENOENT read error) are uncovered (package coverage 54.2%, all uncovered statements are in main/repoRoot/error branches). A regression dropping the os.Exit(1) would pass make check and the gate's clean-tree validation, silently disarming the leak gate.
    location: cmd/heurema-hygiene/main.go:84
  - f_004 severity=low category=quality blocking=false status=open: This change implements two 'Hardening / cleanup' backlog items — 'govulncheck in CI' and 'Committed run-record hygiene gate' — but docs/backlog.md is not updated, so both still read as open to-do items. Repo convention marks entries shipped in the implementing PR (M24.1's PR rewrote its own Slice 4 entry to '(M24.1, shipped)').
    location: docs/backlog.md:361
  - f_005 severity=low category=quality blocking=false status=open: Pre-existing: docs/install.md:53, README.md:224, and AGENTS.md:27 describe `make check` as test + vet + git diff --check, omitting the deadcode gate that `check: test vet deadcode` runs. The install.md line was re-aligned by this change but the stale description was carried over.
    location: docs/install.md:53
  - f_009 severity=low category=quality blocking=false status=open: docs/backlog.md still lists the newly implemented govulncheck CI job and committed run-record hygiene gate as open backlog work.
    location: docs/backlog.md:361

## Artifacts
- Contract: contract/contract.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Gate report: gate/gate-report.json
- Execution result: execute/last-result.json

## Fixer guidance
- Source files are the source of truth.
- Use `pactum search "<term>"` and inspect current source files before relying on this context.
- For each current review finding, trace the finding to the code.
- If a finding is valid, fix it in place within the approved contract scope.
- If a finding is a false positive, leave code unchanged for that finding and explain the rebuttal in your final output.
- Do not approve the review or mutate review findings/resolutions/proposals.
- Do not modify generated `.heurema` artifacts.
