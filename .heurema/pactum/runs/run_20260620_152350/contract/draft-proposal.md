# Contract Draft Proposal

## Status
- Run id: run_20260620_152350
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-20T15:26:00Z

## In scope
- Add a new internal/gitctx package that runs git through exec.CommandContext with a context-aware API, no shell invocation, GIT_OPTIONAL_LOCKS=0 in the command environment, and validation before execution.
- Implement gitctx validation for the allowed read-only command shapes needed by this contract: ls-files, rev-parse --verify HEAD, show-ref, for-each-ref, status --porcelain, diff --name-only, and diff --name-status.
- Implement gitctx rejection for branch, tag, --no-index, --contents, --ignore-revs-file, --output, -o, -c, --git-dir, --work-tree, --exec-path, NUL bytes, absolute path arguments, .. traversal, and pathspec magic.
- Migrate internal/projectmap/scan.go gitCandidateFiles to use internal/gitctx while preserving git ls-files -z --cached --others --exclude-standard behavior, path normalization, duplicate removal, and fallback-on-git-error behavior.
- Migrate internal/app/review_loop.go reviewLoopGitHead to use internal/gitctx while preserving trimmed HEAD output and the existing unavailable result on git failure or empty output.
- Add or update tests for internal/gitctx, internal/projectmap/scan.go, and internal/app/review_loop.go behavior touched by the migration.

## Out of scope
- Changing the contract goal or answering clarification questions.
- Replacing or weakening the ACP write guard, gate behavior, provider/transport behavior, agent execution behavior, or agent scope rules.
- Migrating test helpers that intentionally run mutating git commands to build fixture repositories.
- Migrating cmd/heurema-hygiene raw git calls or adding support for git cat-file, rev-parse --show-toplevel, or other command shapes not named in the contract goal.
- Adding support for pathspec magic, filesystem-reading flags, write flags, branch, or tag.
- Changing unrelated project map scanning, review loop, gate, store, prompt, or contract behavior.

## Acceptance criteria
- internal/gitctx exposes a read-only git runner API that requires a context and repository root and rejects invalid command arguments before starting git.
- internal/gitctx constructs git processes with exec.CommandContext, does not invoke a shell, and sets GIT_OPTIONAL_LOCKS=0 on every built command while preserving the rest of the command environment.
- Table-driven gitctx tests cover allowed command cases for ls-files, rev-parse --verify HEAD, show-ref, for-each-ref, status --porcelain, diff --name-only, and diff --name-status.
- Table-driven gitctx tests cover rejected cases for branch, tag, --no-index, --contents, --ignore-revs-file, --output, -o, -c, --git-dir, --work-tree, --exec-path, NUL bytes, absolute path arguments, .. traversal, and pathspec magic.
- internal/projectmap/scan.go no longer constructs git directly in gitCandidateFiles; it calls internal/gitctx and still passes existing scan tests, including git worktree ignore handling and non-git fallback behavior.
- internal/app/review_loop.go no longer constructs git directly in reviewLoopGitHead; it calls internal/gitctx and still preserves review-loop working-tree fingerprint behavior.
- Raw git calls in fixture-mutating test helpers remain allowed.
- make check passes.

## Validation commands
- go test ./internal/gitctx ./internal/projectmap ./internal/app
- make check

## Assumptions
- The repository root parameter passed to the wrapper may be absolute; the absolute-path rejection applies to git subcommand arguments and pathspec-like inputs, not to the trusted root used for -C.
- The existing gitCandidateFiles ls-files flags -z, --cached, --others, and --exclude-standard are permitted as part of the ls-files read-only command shape.
- Tests that require the git executable may keep the existing skip-when-git-is-missing pattern.
- No current production caller requires pathspec magic, arbitrary file path arguments, or additional git subcommands beyond those listed in the contract goal.

