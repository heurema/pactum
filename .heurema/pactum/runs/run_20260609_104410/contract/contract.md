# Contract Draft

## Goal
Resolve two valid Codex review findings on the .heurema audit PRs. (A) AGENTS.md's Safety section still says 'Do not commit .heurema/' which contradicts the project's actual selective-commit policy (the durable run record under .heurema/pactum/ is versioned; only regenerable/machine-specific parts are git-ignored) — update it so it stops triggering false review flags. (B) The versioned ledger leaks the absolute local repo_root path (e.g. /path/to/repo) on every event because ledger.Event carries a RepoRoot field that is written verbatim but never read back. Remove that field so the durable ledger no longer records an absolute machine path.

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260609_104409
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

## In scope
- A: Rewrite the two stale AGENTS.md Safety bullets ('Do not commit .heurema/...' and 'Keep generated artifacts out of commits') to state the real policy: the durable .heurema/pactum/ run record (config, ledger, contracts, decisions, gate verdicts, review findings, memory) IS version-controlled; the selective .heurema/pactum/.gitignore keeps the regenerable/machine-specific parts out (map/, cache/, tmp/, locks/, runs/*/context/, and *.log transcripts); never commit absolute local paths; and feature PRs stay code-only while the run-record churn is committed separately in periodic 'audit: record runs' batches.
- B: Remove the RepoRoot field from ledger.Event in internal/ledger/events.go, and remove the 'RepoRoot: ...' argument from every ledger.Event{...} literal at the ledger.Append call sites across internal/app. The Go compiler will flag exactly those literals; removing the field must leave the build clean.
- B: Update only those tests (if any) that constructed a ledger.Event with a RepoRoot argument, as forced by the field removal.

## Out of scope
- Do NOT touch RepoRoot on any OTHER struct — RunRequest.RepoRoot (internal/agents), the run-state / workspace manifest repo_root (which is intentionally '.' and IS read), or statusResponse/report.RepoRoot (the runtime status output). Only ledger.Event.RepoRoot is removed.
- Do not change the ledger.Append signature beyond dropping the now-unused field usage; do not alter event Type/Timestamp/RunID; do not reformat or rewrite unrelated AGENTS.md sections.
- Do not edit the already-committed .heurema/pactum/ledger/events.jsonl data file (the historical scrub is handled separately outside this change).

## Paths in scope
- AGENTS.md
- internal/ledger/*.go
- internal/app/*.go


## Acceptance criteria
- ledger.Event no longer has a RepoRoot field; no ledger.Event{...} literal sets repo_root; nothing references Event.RepoRoot. RepoRoot on RunRequest, the run-state/workspace manifest, and statusResponse are untouched.
- AGENTS.md Safety accurately describes the selective .heurema-commit policy and no longer says 'Do not commit .heurema/'; the rest of AGENTS.md is unchanged.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes; the only test changes are those forced by removing ledger.Event.RepoRoot.

## Validation commands
- go build ./...
- go test ./...
- go vet ./...
- go test -race ./...

## Assumptions
- ledger.Event.RepoRoot is write-only: it is serialized into events.jsonl but never read back anywhere (verified), so removing it changes no behavior other than eliminating the absolute-path leak.
- The run-state/workspace manifest already records repo_root as the portable '.' and is the field that is actually read; the ledger's repo_root was redundant provenance.

## Open questions
- None
