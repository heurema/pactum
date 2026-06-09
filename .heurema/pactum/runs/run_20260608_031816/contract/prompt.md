# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260608_031816
- Approval: approved
- Contract hash: a8304d09954a3a483497905955946c29941dccfc61bf23daf99c8fef6434c558

## Goal
Introduce a storage port so the workspace durable-record backend is swappable. Add a new leaf package internal/store with a Store interface and an FS (filesystem) implementation that is byte-for-byte behavior-identical to todays os.* calls, then route ALL of internal/app workspace (.heurema) durable-record I/O through it: the seven structured-record primitives, the workspace-targeting direct os.* calls, and the events ledger. This is behavior-preserving; FS does exactly what the current code does.

## In scope
- New package internal/store (leaf; imports only the stdlib): a Store interface with the minimal methods needed to cover the call sites — WriteBytes(path string, data []byte, perm fs.FileMode) error (MkdirAll the parent then write/truncate), ReadBytes(path string) ([]byte, error), AppendBytes(path string, data []byte) error (MkdirAll the parent then O_APPEND|O_CREATE|O_WRONLY), Exists(path string) bool (true only for a regular file), MkdirAll(path string) error, ReadDir(path string) ([]os.DirEntry, error), Open(path string) (io.ReadCloser, error). Plus an FS struct implementing each with the SAME flags, perms, and error semantics the current code uses.
- internal/app: a package-level active store, var activeStore store.Store = store.FS{}, with an unexported test helper to swap it and restore via t.Cleanup. Route the seven structured-record primitives through activeStore (keep their signatures and keep the json/yaml marshalling inside the primitives; only the raw byte read/write/append/exists goes to the store): writeJSON, readJSON, appendJSONLine, readJSONLines, writeJSONLines, writeYAML, isRegularFile.
- internal/app: replace every direct workspace-targeting os.* call with the matching activeStore method, preserving exact behavior: os.WriteFile->WriteBytes, os.ReadFile->ReadBytes, os.MkdirAll->MkdirAll, os.ReadDir->ReadDir, os.Open(for streaming reads)->Open, os.OpenFile(O_APPEND)->AppendBytes, and os.Stat-used-for-existence->Exists. Workspace-targeting means paths derived from the .heurema workspace (artifacts.Paths, RunPaths/contractRunPaths, the workspace root).
- internal/ledger: Append gains a leading store.Store parameter and writes the event line through the store (AppendBytes); update all call sites in internal/app to pass activeStore. internal/ledger imports internal/store (no cycle: store is a leaf).
- Add a test that substitutes an in-memory Store implementation (a simple map-backed fake) via the swap helper and exercises a representative command end-to-end (e.g. init then write+read a run/contract artifact), asserting the command works without any filesystem writes — proving the backend is swappable without touching call sites.

## Out of scope
- Workspace-DISCOVERY and non-record I/O stay direct os.* (do NOT route them): findUp, os.Getwd, filepath.Abs, reading the user repo tree, os.MkdirTemp/temp files, and the project map files (internal/projectmap, search.sqlite). These are not the durable record and cannot live in a non-filesystem backend.
- internal/agents (subprocess stdout/stderr transcript log streaming), internal/projectmap, internal/search, internal/codeindex — their I/O is out of scope.
- Any behavior, format, schema, path, perm, or output change. New real backends (SQLite/HTTP) beyond the in-memory test fake. Threading the store through App as a constructor dependency (the package-level active store is intentional for this slice). Changing the events JSON format.

## Paths in scope
- internal/store/*.go
- internal/app/*.go
- internal/ledger/*.go


## Acceptance criteria
- internal/store defines Store + FS; FS preserves exact os.* semantics (flags, perms, regular-file existence, parent MkdirAll on write/append).
- No direct os.WriteFile/os.ReadFile/os.MkdirAll/os.ReadDir/os.Open/os.OpenFile or os.Stat-for-existence remain in internal/app for workspace paths (only inside store.FS); the seven primitives and all 41 ledger.Append sites go through a store.Store; workspace-discovery/repo/temp/map I/O is left direct as specified.
- A test swaps in an in-memory Store and a representative command works against it with zero real filesystem writes, proving swappability.
- make check (build, vet, deadcode, git diff --check) and make test-race pass; all pre-existing tests pass UNCHANGED (behavior-preserving) apart from any that legitimately adopt the store-swap helper; artifacts, schemas, output, and events are byte-for-byte identical.

## Validation commands
- make build
- make check
- make test-race

## Assumptions
- internal/app tests are not parallel (0 t.Parallel), so a package-level active store is safe; the swap helper restores the FS default via t.Cleanup.
- internal/store is a leaf (stdlib only) so internal/ledger and internal/app importing it creates no cycle.
- FS is behavior-identical to the replaced os.* calls, so the existing test suite passes unchanged; the deadcode gate and unchanged tests are the regression guards.

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
