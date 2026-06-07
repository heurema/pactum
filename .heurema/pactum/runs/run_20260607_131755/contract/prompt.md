# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260607_131755
- Approval: approved
- Contract hash: 56873ee463c6335a1475923e093cf950cd987672cd00341d85fdb017da272c92

## Goal
Make the project-map scan honor the repository's .gitignore so build artifacts (e.g. __pycache__/*.pyc, dist/, target/, and anything the repo ignores) are NOT indexed. This is the root cause of spurious 'project map is stale': regenerating gitignored artifacts changes the map hashes and blocks prompt build/execute. The fix lives in projectmap.Scan (internal/projectmap/scan.go), which is shared by both the map build and the freshness check (hashStaleReasons), so one fix closes both. Found by the foreign-repo generality test (a Python repo's .pyc got indexed because the hardcoded ignore list omits __pycache__)

## In scope
- projectmap.Scan: when root is a git work tree, enumerate candidate files via git (run 'git -C <root> ls-files -z --cached --others --exclude-standard'), which respects .gitignore, .git/info/exclude, and the global excludes. Process that file set (hashing, language inference, code-items, inventory, top-dirs, important files) exactly as today
- Always exclude the pactum workspace '.heurema/' regardless of gitignore (git --others would otherwise include it on a repo that has not gitignored .heurema yet), and keep the existing binary-extension and max-file-size skips on the git-enumerated set
- Fallback: when root is not a git work tree, or git is unavailable / errors, fall back to the current filepath.WalkDir + hardcoded ignoredDirs behavior, preserving today's output exactly
- Deterministic results: sort the enumerated files so Scan output (Files/Hashes/CodeItems order) is stable regardless of enumeration path
- Tests: a git-backed temp repo whose .gitignore ignores e.g. '__pycache__/', '*.pyc', and an 'ignored/' dir — assert Scan excludes those and includes tracked/non-ignored files; '.heurema/' is excluded even when NOT gitignored; a non-git temp dir still scans via the walk fallback (existing behavior preserved); git-unavailable falls back without error

## Out of scope
- Removing the hardcoded ignoredDirs/ignoredBinaryExts — they remain as the non-git fallback plus the always-on .heurema/binary/size guards
- Changing the map wiki/render, code-index, or any downstream consumer of Scan
- The .heurema version-control policy / init .gitignore (done separately)
- Native LLM API or provider abstraction; editing generated .heurema run artifacts

## Paths in scope
- internal/projectmap/**
- internal/app/**
- docs/**


## Acceptance criteria
- On a git repo, files matched by .gitignore (build artifacts like __pycache__/*.pyc, dist/) are NOT in the map's Files/Hashes/CodeItems, so regenerating them does not make the map stale
- '.heurema/' is never indexed, even on a repo that has not gitignored it
- Non-git repositories scan exactly as before via the walk fallback; git-unavailable degrades to the fallback without erroring
- Scan output is deterministic (stable file ordering)
- make check is green (incl. deadcode); go test -race ./... is clean

## Validation commands
- make check

## Assumptions
TBD

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
