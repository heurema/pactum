# Contract Review Fixer Prompt

You are fixing a software change contract to address blocking review findings.

Current contract version: f6a929883cf219c232f4186c2ff1090208fa0be7dfcce7dec0ccb6f89bfc35b6

## Current Contract

**Goal**: Add a safe-by-construction read-only git wrapper and migrate internal read-only git calls to it. Create a new package internal/gitctx that runs read-only git commands safely: always use exec.CommandContext (never a shell), always set GIT_OPTIONAL_LOCKS=0 in the command environment, and validate arguments before running. Validation: allowlist read-only subcommands only (ls-files, rev-parse --verify HEAD, show-ref, for-each-ref, status --porcelain, diff --name-only and diff --name-status); drop branch and tag entirely (their read and write modes are indistinguishable by argument — callers must use for-each-ref/show-ref instead); reject filesystem-reading and write flags (--no-index, --contents, --ignore-revs-file, --output, -o, -c, --git-dir, --work-tree, --exec-path), absolute path arguments, .. path traversal, NUL bytes, and pathspec magic, until a real caller needs them. Migrate the existing production read-only git calls to the wrapper: gitCandidateFiles in internal/projectmap/scan.go and reviewLoopGitHead in internal/app/review_loop.go. Test helpers that mutate git fixtures may stay raw. This wrapper does NOT replace the ACP write guard or the gate; it only makes internal read-only git construction auditable and deterministic. Scope: new internal/gitctx package (with tests), internal/projectmap/scan.go, internal/app/review_loop.go, and their tests. Do NOT change provider/transport, agent scope, or unrelated production behavior. Acceptance: internal/gitctx exposes a read-only git runner with an allow/deny validation matrix covered by table-driven tests; branch and tag are rejected; absolute paths, .., --no-index, --output/-o are rejected; built commands include GIT_OPTIONAL_LOCKS=0; gitCandidateFiles and reviewLoopGitHead use the wrapper and keep current behavior (existing scan/review_loop tests still pass); make check passes.

**Scope in**:
  - Add a new internal/gitctx package that runs git through exec.CommandContext with a context-aware API, no shell invocation, GIT_OPTIONAL_LOCKS=0 in the command environment, and validation before execution.
  - Implement gitctx validation for the allowed read-only command shapes needed by this contract: ls-files, rev-parse --verify HEAD, show-ref, for-each-ref, status --porcelain, diff --name-only, and diff --name-status.
  - Implement gitctx rejection for branch, tag, --no-index, --contents, --ignore-revs-file, --output, -o, -c, --git-dir, --work-tree, --exec-path, NUL bytes, absolute path arguments, .. traversal, and pathspec magic.
  - Migrate internal/projectmap/scan.go gitCandidateFiles to use internal/gitctx while preserving git ls-files -z --cached --others --exclude-standard behavior, path normalization, duplicate removal, and fallback-on-git-error behavior.
  - Migrate internal/app/review_loop.go reviewLoopGitHead to use internal/gitctx while preserving trimmed HEAD output and the existing unavailable result on git failure or empty output.
  - Add or update tests for internal/gitctx, internal/projectmap/scan.go, and internal/app/review_loop.go behavior touched by the migration.

**Scope out**:
  - Changing the contract goal or answering clarification questions.
  - Replacing or weakening the ACP write guard, gate behavior, provider/transport behavior, agent execution behavior, or agent scope rules.
  - Migrating test helpers that intentionally run mutating git commands to build fixture repositories.
  - Migrating cmd/heurema-hygiene raw git calls or adding support for git cat-file, rev-parse --show-toplevel, or other command shapes not named in the contract goal.
  - Adding support for pathspec magic, filesystem-reading flags, write flags, branch, or tag.
  - Changing unrelated project map scanning, review loop, gate, store, prompt, or contract behavior.

**Acceptance criteria**:
  - internal/gitctx exposes a read-only git runner API that requires a context and repository root and rejects invalid command arguments before starting git.
  - internal/gitctx exposes an inspectable command-building seam — for example, a Build function that constructs and returns a *exec.Cmd without executing it — so that the resulting command's binary path and environment are unit-testable without spawning git. A dedicated unit test inspects the *exec.Cmd produced by this seam, asserting that Cmd.Path resolves to the git binary (not a shell such as /bin/sh or /bin/bash), that GIT_OPTIONAL_LOCKS=0 appears in the command's effective environment, and that the repository root is reflected in the built command's working context — either Cmd.Dir equals the supplied root, or the root appears as the argument to -C in Cmd.Args. At runtime, internal/gitctx uses exec.CommandContext, never invokes a shell, and sets GIT_OPTIONAL_LOCKS=0 on every built command while preserving the rest of the process environment.
  - Table-driven gitctx tests cover allowed command cases and enforce the exact permitted argument shapes per subcommand: ls-files accepts any combination of flags from {-z, --cached, --others, --exclude-standard} and no other flags or non-flag arguments; rev-parse accepts the exact argument vector [--verify, HEAD] and nothing else; show-ref accepts no flags and at most one non-flag ref-name argument; for-each-ref accepts no flags and at most one non-flag ref-pattern argument; status accepts the single flag --porcelain with no other flags or arguments; diff accepts exactly one flag from {--name-only, --name-status} and optionally at most two non-flag commit-ish arguments. Any unrecognized flag, any flag not listed for the given subcommand, or any argument count exceeding the stated per-subcommand limit is rejected by the validator. Any subcommand not in the enumerated allowlist is also rejected by the validator, regardless of its arguments.
  - Table-driven gitctx tests cover rejected cases for: the explicitly blocked subcommands branch and tag; write and unknown subcommands including at minimum commit, push, checkout, reset, clean, and stash; an empty or missing subcommand (argv of length zero); and the blocked flags and argument patterns --no-index, --contents, --ignore-revs-file, --output, -o, -c, --git-dir, --work-tree, --exec-path, NUL bytes, absolute path arguments, .. traversal, and pathspec magic.
  - internal/projectmap/scan.go no longer constructs git directly in gitCandidateFiles; it calls internal/gitctx and still passes existing scan tests, including git worktree ignore handling and non-git fallback behavior.
  - internal/app/review_loop.go no longer constructs git directly in reviewLoopGitHead; it calls internal/gitctx and still preserves review-loop working-tree fingerprint behavior.
  - Raw git calls in fixture-mutating test helpers remain allowed.
  - make check passes.

**Validation commands**:
  - go test ./internal/gitctx ./internal/projectmap ./internal/app
  - make check

**Assumptions**:
  - The repository root parameter passed to the wrapper may be absolute; the absolute-path rejection applies to git subcommand arguments and pathspec-like inputs, not to the trusted root used for -C.
  - The existing gitCandidateFiles ls-files flags -z, --cached, --others, and --exclude-standard are permitted as part of the ls-files read-only command shape.
  - Tests that require the git executable may keep the existing skip-when-git-is-missing pattern.
  - No current production caller requires pathspec magic, arbitrary file path arguments, or additional git subcommands beyond those listed in the contract goal.

## Blocking Findings to Address

1. [codex-xhigh/testability] Some structural acceptance criteria are prose-only unless the contract adds an explicit static check or unit test that gates them.
   Evidence: Acceptance criteria require that internal/gitctx uses exec.CommandContext, never invokes a shell, and that internal/projectmap/scan.go and internal/app/review_loop.go no longer construct git directly, but the validation commands only list go test and make check without specifying a static or behavioral test for those implementation-shape requirements.

## Fixer Instructions

- Address each blocking finding by updating the relevant contract field.
- Do NOT change the goal field — it is out of scope for the fixer.
- Only include the contract fields you are changing in the output.
- base_version must exactly match the version shown above.

## Output

Output your reasoning, then a single JSON block with the revise payload:

```json
{
  "schema": "pactum.contract_revise.v1alpha1",
  "base_version": "f6a929883cf219c232f4186c2ff1090208fa0be7dfcce7dec0ccb6f89bfc35b6",
  "contract": {
    "acceptance_criteria": ["...updated criteria..."],
    "validation": {"commands": ["...updated commands..."]}
  }
}
```

Omit any contract field you are not changing. Do not include the goal field.
