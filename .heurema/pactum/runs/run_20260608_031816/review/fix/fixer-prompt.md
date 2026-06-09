# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts, but it does not mark review findings resolved automatically.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260608_031816/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260608_031816/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260608_031816/review/review.json, .heurema/pactum/runs/run_20260608_031816/review/findings.jsonl, .heurema/pactum/runs/run_20260608_031816/review/resolutions.jsonl

## Approved contract
- Goal: Introduce a storage port so the workspace durable-record backend is swappable. Add a new leaf package internal/store with a Store interface and an FS (filesystem) implementation that is byte-for-byte behavior-identical to todays os.* calls, then route ALL of internal/app workspace (.heurema) durable-record I/O through it: the seven structured-record primitives, the workspace-targeting direct os.* calls, and the events ledger. This is behavior-preserving; FS does exactly what the current code does.
- In scope:
  - New package internal/store (leaf; imports only the stdlib): a Store interface with the minimal methods needed to cover the call sites — WriteBytes(path string, data []byte, perm fs.FileMode) error (MkdirAll the parent then write/truncate), ReadBytes(path string) ([]byte, error), AppendBytes(path string, data []byte) error (MkdirAll the parent then O_APPEND|O_CREATE|O_WRONLY), Exists(path string) bool (true only for a regular file), MkdirAll(path string) error, ReadDir(path string) ([]os.DirEntry, error), Open(path string) (io.ReadCloser, error). Plus an FS struct implementing each with the SAME flags, perms, and error semantics the current code uses.
  - internal/app: a package-level active store, var activeStore store.Store = store.FS{}, with an unexported test helper to swap it and restore via t.Cleanup. Route the seven structured-record primitives through activeStore (keep their signatures and keep the json/yaml marshalling inside the primitives; only the raw byte read/write/append/exists goes to the store): writeJSON, readJSON, appendJSONLine, readJSONLines, writeJSONLines, writeYAML, isRegularFile.
  - internal/app: replace every direct workspace-targeting os.* call with the matching activeStore method, preserving exact behavior: os.WriteFile->WriteBytes, os.ReadFile->ReadBytes, os.MkdirAll->MkdirAll, os.ReadDir->ReadDir, os.Open(for streaming reads)->Open, os.OpenFile(O_APPEND)->AppendBytes, and os.Stat-used-for-existence->Exists. Workspace-targeting means paths derived from the .heurema workspace (artifacts.Paths, RunPaths/contractRunPaths, the workspace root).
  - internal/ledger: Append gains a leading store.Store parameter and writes the event line through the store (AppendBytes); update all call sites in internal/app to pass activeStore. internal/ledger imports internal/store (no cycle: store is a leaf).
  - Add a test that substitutes an in-memory Store implementation (a simple map-backed fake) via the swap helper and exercises a representative command end-to-end (e.g. init then write+read a run/contract artifact), asserting the command works without any filesystem writes — proving the backend is swappable without touching call sites.
- Out of scope:
  - Workspace-DISCOVERY and non-record I/O stay direct os.* (do NOT route them): findUp, os.Getwd, filepath.Abs, reading the user repo tree, os.MkdirTemp/temp files, and the project map files (internal/projectmap, search.sqlite). These are not the durable record and cannot live in a non-filesystem backend.
  - internal/agents (subprocess stdout/stderr transcript log streaming), internal/projectmap, internal/search, internal/codeindex — their I/O is out of scope.
  - Any behavior, format, schema, path, perm, or output change. New real backends (SQLite/HTTP) beyond the in-memory test fake. Threading the store through App as a constructor dependency (the package-level active store is intentional for this slice). Changing the events JSON format.
- Acceptance criteria:
  - internal/store defines Store + FS; FS preserves exact os.* semantics (flags, perms, regular-file existence, parent MkdirAll on write/append).
  - No direct os.WriteFile/os.ReadFile/os.MkdirAll/os.ReadDir/os.Open/os.OpenFile or os.Stat-for-existence remain in internal/app for workspace paths (only inside store.FS); the seven primitives and all 41 ledger.Append sites go through a store.Store; workspace-discovery/repo/temp/map I/O is left direct as specified.
  - A test swaps in an in-memory Store and a representative command works against it with zero real filesystem writes, proving swappability.
  - make check (build, vet, deadcode, git diff --check) and make test-race pass; all pre-existing tests pass UNCHANGED (behavior-preserving) apart from any that legitimately adopt the store-swap helper; artifacts, schemas, output, and events are byte-for-byte identical.
- Validation commands:
  - make build
  - make check
  - make test-race

## Current review findings
- Summary: findings=12 open=12 resolved=0 blocking_open=5
- Findings:
  - f_001 severity=medium category=scope blocking=true status=open: Repository-file existence checks now go through activeStore via isRegularFile, so non-filesystem workspace stores will make real repo files appear missing during gate/review-loop change detection. Repo-tree I/O is explicitly out of scope and should remain direct filesystem I/O.
    location: internal/app/gate.go:404
  - f_002 severity=low category=scope blocking=false status=open: Store interface adds a Remove method beyond the contract's enumerated list (WriteBytes/ReadBytes/AppendBytes/Exists/MkdirAll/ReadDir/Open). It is used to route removePromptReadinessArtifacts' workspace os.Remove(PromptManifest). This is a contract-vs-implementation delta, not a defect: FS.Remove == os.Remove, the !os.IsNotExist guard is preserved, and routing it is consistent with the goal of routing ALL workspace durable-record I/O. No action needed.
    location: internal/store/store.go:18
  - f_003 severity=low category=correctness blocking=false status=open: storeDirExists answers directory-existence via Exists-then-ReadDir because the contracted minimal interface has no IsDir method. It is equivalent to the original os.Stat+IsDir for all realistic run-dir paths, diverging only for the impossible case of a non-regular, non-directory special file (socket/fifo) at a run-dir path, where the original returned (false,nil) but ReadDir returns ENOTDIR. Negligible; a consequence of the contracted interface, not a defect.
    location: internal/app/store.go:14
  - f_004 severity=low category=quality blocking=false status=open: Within the map subsystem, map manifest/run-record writes route through the store (writeJSON at map.go:161/179/238, an unavoidable consequence of routing the shared writeJSON primitive), while the existence probe in nextMapRunID (os.Stat, map.go:220) and the map output files stay direct. This matches acceptance criterion 'map I/O is left direct as specified' and is byte-identical for the FS backend, but creates a write-vs-probe asymmetry that would only matter for a future non-FS backend (explicitly out of scope). Flagging for future awareness only.
    location: internal/app/map.go:220
  - f_005 severity=medium category=scope blocking=true status=open: Gate validation still writes workspace stdout/stderr artifacts directly with os.Create after creating the validation directory through activeStore, so non-filesystem stores cannot run validation commands and the logs bypass the storage port.
    location: internal/app/gate.go:539
  - f_006 severity=medium category=scope blocking=true status=open: Project-map artifact existence checks use activeStore even though map/search files are explicitly out of scope and remain on the filesystem.
    location: internal/app/status.go:237
  - f_007 severity=low category=scope blocking=false status=open: The project-map file map/hashes.jsonl is written directly (projectmap.WriteJSONL, out of scope) but read through the store via the shared readJSONLines primitive at gate.go:375 and existence-probed through the store via isRegularFile at status.go:260, while also read directly via readHashRecords (os.Open) at status.go:292. Byte-identical for the FS backend so acceptance criteria hold; for a future non-FS backend the gate's expected-hash read would return empty, silently breaking repo change-detection. Distinct, more concrete instance of the f_004 map write-vs-probe asymmetry (a written-direct file read-through-store). Future awareness only.
    location: internal/app/gate.go:375
  - f_008 severity=medium category=scope blocking=true status=open: Run reservation still bypasses the storage port: task new reaches reserveContractRunDir, which creates the .heurema/pactum/runs/run_* directory with os.Mkdir after only creating the parent through activeStore. A non-filesystem store cannot create a run without real filesystem writes or filesystem parent directories.
    location: internal/app/run.go:345
  - f_009 severity=low category=quality blocking=false status=open: Gate validation stdout/stderr are now fully buffered in memory and written once via activeStore.WriteBytes, rather than streamed to disk. Final log artifacts are byte-identical for the FS backend so acceptance criteria hold; this is the intended consequence of routing through the WriteBytes-only Store interface and resolves f_005/p_005. Flagging the streaming->buffered behavior delta for human awareness given the contract's 'no behavior change' clause: very large validation output is now held entirely in memory, and partial logs are no longer flushed live during a long-running or killed command.
    location: internal/app/gate.go:544
  - f_010 severity=low category=process blocking=false status=open: The current working tree has been revised past the state the existing findings describe: repo/map existence probes moved from activeStore to direct filesystemRegularFile, repo/map hashing to direct fileSHA256/os.Open, and validation logs routed through the store. This resolves blocking findings f_001/f_005/f_006 and non-blocking f_007. I could not execute make build/check/test-race in this review sandbox to confirm the revised tree still passes; the latest gate-report.json shows exit 0, but the tree is uncommitted and diverged from the findings. Recommend re-running validation against the current tree before approval, then marking the resolved findings.
    location: internal/app/gate.go:407
  - f_011 severity=medium category=process blocking=true status=open: Validation has not been re-run against the current working tree. Acceptance criterion #4 requires `make check` and `make test-race` to pass against the final tree, but the only validation evidence (gate-report.json, exit 0) predates the working-tree revisions that resolved f_001/f_005/f_006/f_007/f_008. The tree is uncommitted and diverged from the gated state. This escalates the existing non-blocking f_010: static analysis confirms consistent signatures and no deadcode (storeFileSHA256/storeDirExists still used for workspace paths; new direct fileSHA256/filesystemRegularFile used at repo-probe sites), so risk is low, but the criterion mandates actual execution. Re-run validation against the current tree, then mark f_001/f_005/f_006/f_007/f_008 resolved.
    location: internal/app/gate.go:407
  - f_012 severity=low category=scope blocking=false status=open: The Store interface adds a Mkdir method beyond the contract's enumerated list (WriteBytes/ReadBytes/AppendBytes/Exists/MkdirAll/ReadDir/Open) — a second un-enumerated addition alongside Remove (f_002), not currently represented by any finding. It routes reserveContractRunDir's os.Mkdir and is the mechanism that resolves f_008. FS.Mkdir == os.Mkdir(path, 0o755) and the os.IsExist guard in the run-dir reservation loop is preserved, so this is a contract-vs-implementation delta, not a defect. No action needed; flagged for completeness/symmetry with f_002.
    location: internal/store/store.go:15

## Fix boundaries
- Trace each finding to the relevant code before acting.
- Fix valid findings in place.
- For false positives, explain a concrete rebuttal instead of changing code.
- Keep changes inside the approved contract and review-finding scope.
- Do not edit `.heurema` artifacts.
- Do not run `pactum review approve`, `pactum review resolve`, or any review loop command.

## Output shape
In your final output, list each finding id with one of:
- fixed: what changed and where
- rebutted: why the finding is a false positive
- blocked: what concrete information or state is missing
