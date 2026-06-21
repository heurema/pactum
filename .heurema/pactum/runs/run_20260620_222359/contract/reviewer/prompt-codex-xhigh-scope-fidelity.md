# Contract Review: Scope fidelity

You are reviewing a software change contract through the **scope-fidelity** lens.

Review the contract fields below using only your assigned lens checklist.
Do not flag issues that belong to other lenses.

## Contract

**Goal**: Add a deterministic git-history guard around write-enabled agent stages so the executor cannot mask or corrupt git history. This is the reliable, adapter-independent mechanism researched by a multi-model council: observe the EFFECT (history/refs/reflog/index changed), not the command — because command-level interception (ACP permission denylist, hooks, PATH shim) is bypassable by indirection (e.g. `bash commit.sh`) and adapter auto-approve modes.

CONTEXT: pactum runs coding agents over ACP during the write-enabled `execute` (and `review-fix`) stages; the agent edits the WORKING TREE and pactum's `gate`/`review` inspect the working-tree state. PROBLEM: an execute agent ran `git commit` itself, packaging its work into history instead of leaving it as unstaged working-tree changes, which breaks the workflow model (and could even lose work via `commit && reset --hard`).

DESIGN (v1, conservative — auto-restore the simple cases; bail loudly on ambiguous ones; NEVER silently lose the agent's work):

1. New package/file internal/app/git_guard.go with a guard that SNAPSHOTS git state before the agent runs and RESTORES + reports if mutated after. Use a dedicated controlled WRITE-git path (plain exec of git for the guard's own mutations) — NOT the read-only internal/gitctx wrapper (that is read-only by design).

2. PRECONDITION checks at execute START — if any fails, do NOT run the agent; finish the attempt with terminal_reason `executor_git_guard_inconclusive`: HEAD is attached to a branch (not detached); working tree is clean (no staged, unstaged, or untracked changes — `git status --porcelain=v1 -z --untracked-files=all` empty); no unmerged paths; no in-progress merge/rebase/cherry-pick/revert/bisect/sequencer; no stale index/ref lock files; no submodules or linked worktrees (treat as inconclusive until explicitly supported); ordinary (non-sparse/non-split) index.

3. SNAPSHOT (minimal-but-complete) captured at start: HEAD symbolic ref + HEAD commit hash; all refs via `git for-each-ref` (branches, tags, remotes, notes, refs/stash); reflog state of HEAD and all refs (line count or hash — this catches `git commit && git reset --hard` where HEAD ends unchanged but the reflog advanced); the set of existing commit object ids (`git cat-file --batch-all-objects` filtered to commits — catches dangling commits); the index tree hash (`git write-tree`).

4. After the agent returns, recompute and compare. If nothing changed -> no-op, proceed normally. If a mutation is detected:
   - Simple, safely-restorable cases (agent committed N commits, amended, staged the index, and/or moved/created the current branch, with the start preconditions having held): run `git reset --mixed <origHash>` so the branch pointer rewinds and the agent's file changes remain UNSTAGED in the working tree for the gate to scan; restore any other changed refs and their reflogs from the snapshot LAST (the restore commands themselves write reflogs). Set terminal_reason `executor_git_history_mutation` (commit/amend/rebase/commit-then-reset), `executor_git_ref_mutation` (branch/tag/ref moved), or `executor_git_index_mutation` (only the index changed) as appropriate.
   - reset-hard / stash / ambiguous (reflog advanced while HEAD hash unchanged; new stash entry; more than one candidate recovery tip; objects pruned; restore would conflict): do NOT auto-restore in v1 — the work is preserved in the reflog/dangling commits; finish loud with terminal_reason `executor_git_guard_inconclusive` for a human (or a future v2 recovery) to handle.
   - If a restore command itself errors: terminal_reason `executor_git_restore_failed`.

5. Integrate in runAgentAttemptLifecycle (internal/app/agent_attempt.go) wrapping the single agentTransport().Run call, enabled only for WRITE-enabled stages (execute, review-fix) — read-only stages are unaffected. Record the guard outcome (the terminal_reason and what was detected/restored) in the execution result document (internal/app/execute.go) and the analogous review-fix result. The execute `next` affordances and the gate must require an EMPTY terminal_reason (a git-guard terminal_reason blocks proceeding, loudly).

6. Be honest about scope in code comments: this does NOT primarily exist because the gate would miss a committed change (pactum's gate compares project-map file hashes, so a committed-but-present change may already be detected) — it protects the workflow invariant (executor work ends as unstaged working-tree changes with original history/refs/index intact), recovers `reset --hard`-lost work where unambiguous (deferred to inconclusive in v1), and fails loud instead of silently corrupting the run model.

SCOPE: new internal/app/git_guard.go (+ tests), integration edits in internal/app/agent_attempt.go and internal/app/execute.go (and the review-fix result path), and tests. Do NOT add the ACP RequestPermission command denylist in this slice (it is a bypassable fast-fail optimization — a separate follow-up). Do NOT change the read-only internal/gitctx wrapper, the transport/provider selection, or the loop engine.

ACCEPTANCE: precondition failures (dirty tree, detached HEAD, in-progress op, submodule) finish with `executor_git_guard_inconclusive` without running the agent; an agent that commits on a clean-start write stage is detected and restored so the commit's changes end as unstaged working-tree changes and HEAD is back at the original commit, with terminal_reason `executor_git_history_mutation`; an index-only change is unstaged (`executor_git_index_mutation`); a `reset --hard`/stash case is detected and finishes `executor_git_guard_inconclusive` WITHOUT losing the work; a non-empty git-guard terminal_reason blocks the execute `next`/gate; read-only stages are unaffected; tests cover snapshot/restore/preconditions using temporary real git repositories; make check passes.

**Scope in**:
  - Add `internal/app/git_guard.go` and focused tests for git state preconditions, snapshots, mutation detection, restoration, and terminal reason classification using temporary real git repositories.
  - Integrate the guard in `runAgentAttemptLifecycle` around the single `agentTransport().Run` call for write-enabled stages only, covering execute and review-fix while leaving read-only stages unchanged.
  - Extend execute and review-fix result documents, last-result artifacts, and JSON run responses with a machine-readable git-guard outcome including `terminal_reason`, detected mutation class, restored state, and restore error details when present.
  - Block execute `next` affordances, review-fix apply affordances, and gate progression whenever the relevant attempt result has a non-empty git-guard `terminal_reason`.
  - Use a dedicated controlled git execution path for guard-owned write operations such as `git reset --mixed <origHash>` and ref restoration; do not reuse the read-only `internal/gitctx` wrapper for those writes.
  - Add or update tests around execute, review-fix, and gate behavior so git-guard terminal reasons are observable through attempt artifacts and CLI JSON responses.

**Scope out**:
  - Do not add an ACP RequestPermission command denylist in this change.
  - Do not change the read-only contract of `internal/gitctx` or broaden its allowed git commands to perform writes.
  - Do not change transport/provider selection, ACP adapter behavior, retry policy, or the review loop engine except where necessary to pass guard outcomes through existing lifecycle result handling.
  - Do not add v1 support for submodules, linked worktrees, sparse indexes, split indexes, or multi-worktree recovery; these cases must be treated as inconclusive precondition failures.
  - Do not auto-restore ambiguous reset-hard, stash, pruned-object, or multi-candidate recovery cases in v1.
  - Do not commit or otherwise mutate repository history as part of normal execute or review-fix agent handling.

**Acceptance criteria**:
  - On an execute stage, a dirty working tree (any staged, unstaged, or untracked changes), detached HEAD, unmerged paths, in-progress merge/rebase/cherry-pick/revert/bisect/sequencer, stale git lock files, submodules, linked worktrees, sparse index, or split index is detected before the transport runs; the agent transport is not called and the attempt records `terminal_reason` `executor_git_guard_inconclusive`.
  - On a review-fix stage, detached HEAD, unmerged paths, in-progress merge/rebase/cherry-pick/revert/bisect/sequencer, stale git lock files, submodules, linked worktrees, sparse index, or split index is detected before the transport runs and the attempt records `terminal_reason` `executor_git_guard_inconclusive` without calling the transport; the clean-working-tree check is omitted because pre-existing unstaged and staged changes from the execute stage are expected at review-fix start.
  - On a review-fix stage that passes preconditions, the guard snapshots the full working-tree and git state — including all pre-existing unstaged and staged execute-phase changes — as the mutation-detection baseline before the fixer transport runs; only fixer-introduced history, ref, or index mutations that exceed that baseline are flagged.
  - On a clean-start write-enabled stage where the agent makes only ordinary working-tree file edits, the guard records an empty `terminal_reason`, preserves existing successful execute/review-fix behavior, and exposes the normal next affordance.
  - If the agent creates commits, amends commits, or rebases such that HEAD ends at a different commit hash than the snapshot hash, the guard detects the history mutation, restores HEAD and the current branch to the original commit with `git reset --mixed <origHash>`, leaves the agent's file changes as unstaged working-tree changes, and records `terminal_reason` `executor_git_history_mutation`. The case where HEAD ends at the same commit hash as the snapshot — for example, commit-then-reset-hard where HEAD reverts to the original hash — is not classified as `executor_git_history_mutation`; it falls under `executor_git_guard_inconclusive` per the HEAD-unchanged-reflog-advanced criterion.
  - If the agent only stages index changes from a clean start, the guard restores the original index tree while preserving working-tree content and records `terminal_reason` `executor_git_index_mutation`.
  - If the agent creates, deletes, or moves non-current refs such as branches, tags, notes, remotes, or stash refs, the guard detects and restores safely restorable refs and records `terminal_reason` `executor_git_ref_mutation` unless the case is ambiguous.
  - If HEAD hash ends unchanged but reflog state or reachable/dangling commit objects changed — including commit-then-reset-hard — the guard detects the mutation and records `terminal_reason` `executor_git_guard_inconclusive` without silently proceeding and without attempting auto-restoration.
  - If a new stash entry is created, the guard detects it, does not auto-restore in v1, and records `terminal_reason` `executor_git_guard_inconclusive`; the guard takes no action (such as running `git gc` or dropping the stash) that would make the stashed or dangling work harder to recover — the work remains accessible via `git reflog` and `git fsck --unreachable` until an operator decides what to do.
  - If any guard restore command fails, the attempt records `terminal_reason` `executor_git_restore_failed` and includes enough restore error detail for a human to diagnose the repository state, unless a concurrent `executor_git_guard_inconclusive` condition is also detected — in which case `executor_git_guard_inconclusive` takes precedence per the severity ranking; restore error detail is still recorded in the outcome fields for diagnostic purposes regardless of which terminal_reason is dominant.
  - When a single agent run produces mutations spanning more than one class, the recorded `terminal_reason` is the most severe class by the following precedence (highest first): `executor_git_guard_inconclusive` > `executor_git_restore_failed` > `executor_git_history_mutation` > `executor_git_ref_mutation` > `executor_git_index_mutation`; all detected mutations that are classifiable as restorable are still restored before the dominant reason is recorded.
  - If the wrapped agent transport returns an error or non-zero exit after mutating git state, the guard still detects mutations and executes the same restoration logic as for a successful agent return before propagating the failure result; the attempt records both the git-guard `terminal_reason` and the transport error; a non-empty git-guard `terminal_reason` supersedes any would-be next affordance regardless of whether the agent transport returned success or failure.
  - For review-fix stages where a history or index mutation is detected and a history-mutation restore is performed: after `git reset --mixed <origHash>` rewinds the branch pointer and clears the index to origHash's tree, the guard must additionally restore the index to the pre-fixer snapshot index state — specifically the index tree hash captured at guard start, which includes the execute-phase staged changes — rather than leaving the index reflecting origHash's clean tree; this ensures that execute-phase staged changes are returned to their pre-fixer staging status and are not inadvertently unstaged by the reset.
  - Execute and review-fix attempt `result.json`, run-level `last-result.json`, and `--json` CLI output expose the same git-guard terminal reason and outcome fields.
  - Any non-empty git-guard `terminal_reason` suppresses execute `next`, suppresses review-fix apply `next`, and prevents `pactum gate run` from treating the attempt as a completed successful execution.
  - Read-only lifecycle stages still run without the git-history guard and preserve existing read-only retry behavior.
  - Tests assert all of the following scenarios using temporary real git repositories: execute precondition no-run behavior (dirty tree, detached HEAD, in-progress op, submodule, linked worktree, sparse/split index); review-fix precondition no-run behavior (detached HEAD, in-progress op) without requiring a clean tree; review-fix baseline-snapshot behavior where pre-existing execute-phase dirty changes are not flagged but fixer-introduced mutations are; clean no-op behavior; commit restore where HEAD hash changes (executor_git_history_mutation); index-only restore (executor_git_index_mutation); ref mutation handling (executor_git_ref_mutation); reset-hard or stash inconclusive handling (executor_git_guard_inconclusive) including the explicit classification of commit-then-reset-hard as inconclusive rather than executor_git_history_mutation; agent-errors-after-mutation (transport returns error after mutating git — restoration still runs and terminal_reason is recorded); review-fix staging preservation (index restored to pre-fixer snapshot tree, not origHash tree, after history-mutation restore); combined-mutation terminal_reason precedence including executor_git_guard_inconclusive taking precedence over executor_git_restore_failed; result artifact fields on attempt result.json, last-result.json, and CLI output; next suppression on execute and review-fix; gate blocking; and read-only stage non-interference. All git-guard test functions must use the TestGitGuard prefix (e.g., TestGitGuardPreconditions, TestGitGuardCommitRestore, TestGitGuardAgentError, TestGitGuardReviewFixBaseline, TestGitGuardReviewFixStagingPreservation, TestGitGuardInconclusiveResetHard) so that targeted validation commands can locate and run them.

**Validation commands**:
  - grep -rq 'func TestGitGuard' internal/app/
  - go test -v -count=1 ./internal/app -run 'TestGitGuard'
  - go test -count=1 ./internal/app ./internal/gitctx
  - make check

**Assumptions**:
  - The implementation environment has a `git` binary available for tests and guard operations.
  - Tests may configure `user.name` and `user.email` inside temporary git repositories without modifying global git config.
  - Rejecting execute-stage agent execution when the repository has any pre-existing staged, unstaged, or untracked change is acceptable for v1; review-fix stages are exempt from this restriction because they begin on a dirty working tree by design, and the guard uses the pre-fixer dirty state as the mutation-detection baseline.
  - It is acceptable to add new result JSON fields for git-guard outcome data as long as existing process result fields remain compatible.
  - The guard's own restoration commands may create reflog entries; detection is based on the post-agent, pre-restore comparison, and restoration metadata should distinguish guard-caused writes from agent-caused mutations.

## Lens: Scope fidelity

Checklist:
- Is scope.in coherent with and proportionate to the goal?
- Is scope.out coherent and not contradictory with scope.in?
- Is the scope neither over-broad nor under-broad for the stated goal?

## Output

Report likely-real defects (recall-first), then gate on precision before marking blocking.
Use state=candidate with explicit uncertainty when you believe a finding is real but have not fully confirmed it.

State your analysis in prose. If you find issues, also include a structured block:

```json
{
  "schema": "pactum.contract_reviewer_result.v1alpha1",
  "findings": [
    {
      "message": "Describe the contract issue clearly.",
      "severity": "medium",
      "category": "quality",
      "blocking": true,
      "evidence": "Quote or cite the contract field that shows the issue.",
      "material_impact": "Concrete way this spec defect would make the implementation wrong, ambiguous, or stuck.",
      "fix_direction": "What the contract author should change to resolve this.",
      "uncertainty": "Any doubt about this finding — omit if confident.",
      "state": "candidate"
    }
  ]
}
```

Rules:
- Use severity: low, medium, high, critical.
- Use category: correctness, scope, quality, validation, process, other.
- Omit file and line (not applicable for contract review).
- Set state=candidate when likely real but not fully confirmed; set state=confirmed when certain.
- HARD RULE: blocking=true is allowed ONLY for a material spec defect that would make the implementation wrong, ambiguous, or stuck.
- Wording, style, naming, redundancy, and completeness/thoroughness preferences MUST be blocking=false (advisory).
- Every blocking finding MUST include a concrete material_impact explaining the implementation consequence.
- If you cannot state a concrete material_impact, mark the finding blocking=false (advisory).
- Set blocking=false for advisory issues.
- If no issues, say so clearly. Do not include an empty findings block.
