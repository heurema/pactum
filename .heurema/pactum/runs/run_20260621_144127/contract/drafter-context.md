# Contract Drafter Context

## Run
- Run id: run_20260621_144127
- Run status: contract_draft

## Contract goal
Fix the git-history guard so it runs inside a linked git worktree instead of blanket-blocking. Currently gitGuardPrechecks in internal/app/git_guard.go runs `git worktree list --porcelain` and returns executor_git_guard_inconclusive whenever more than one worktree exists (around lines 134-146). This blocks the write-enabled execute and review-fix stages in ANY git worktree checkout — and even in the primary checkout when any sibling worktree exists — which breaks the common worktree-based workflow (pactum's own dogfooding runs in worktrees, so the just-merged guard immediately blocked it).

The guard's snapshot/restore operates correctly per-worktree: HEAD (symbolic-ref + hash), the index (write-tree), and `git reset --mixed` / `git update-ref` all work in a linked worktree, scoped to that worktree's own HEAD/index. So a linked worktree is NOT a reason to bail.

First read internal/app/git_guard.go (the gitGuardPrechecks function and the multi-worktree check) and internal/app/git_guard_test.go to understand the existing precondition tests and helpers.

Fix: remove the blanket multi-worktree inconclusive precondition so the guard runs normally in a linked (or primary-with-siblings) worktree. KEEP every other precondition unchanged (attached HEAD, clean tree excluding .heurema, no unmerged paths, no in-progress merge/rebase/cherry-pick/revert/bisect/sequencer, no stale lock files, no submodules, no sparse/split index). Document the residual shared-ref edge in a code comment: refs are shared across linked worktrees, so (a) a branch checked out in ANOTHER worktree cannot be force-moved from this one — if the guard ever needs to restore such a ref and git refuses, that already surfaces loudly as executor_git_restore_failed; and (b) concurrent git activity in another worktree could in theory perturb shared refs mid-run — this is an accepted, documented residual for now and must NOT block.

Scope: internal/app/git_guard.go (remove/replace only the multi-worktree precondition, plus the explanatory comment) and internal/app/git_guard_test.go (add a test that the guard runs its normal snapshot/compare/restore inside a linked worktree created with `git worktree add` — e.g. an agent commit made inside the worktree is detected and restored as executor_git_history_mutation with the work left unstaged, NOT executor_git_guard_inconclusive). Do NOT weaken the other preconditions, change the snapshot/restore logic, or alter the terminal-reason set.

Acceptance: a write-stage guard run inside a linked git worktree (created via `git worktree add`) does NOT return executor_git_guard_inconclusive merely because other worktrees exist; the guard performs its normal snapshot/compare/restore there, and an in-worktree agent commit is detected and restored as executor_git_history_mutation leaving the work unstaged; all other preconditions (detached HEAD, dirty non-.heurema tree, in-progress op, submodules, sparse/split index) still return executor_git_guard_inconclusive; a code comment documents the linked-worktree support and the residual shared-ref edge; make check passes.

## Current contract fields
- In scope:
  - none
- Out of scope:
  - none
- Acceptance criteria:
  - none
- Validation commands:
  - none
- Assumptions:
  - none

## Answered clarifications
- None

## Repository context
# Repository Context

Generated: 2026-06-21T14:41:27Z

Map run: map_20260620_115144
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

Project map is unavailable at .heurema/pactum/map/repo-map.md.

## Search results
{
  "query": "Fix the git-history guard so it runs inside a linked git worktree instead of blanket-blocking. Currently gitGuardPrechecks in internal/app/git_guard.go runs `git worktree list --porcelain` and returns executor_git_guard_inconclusive whenever more than one worktree exists (around lines 134-146). This blocks the write-enabled execute and review-fix stages in ANY git worktree checkout — and even in the primary checkout when any sibling worktree exists — which breaks the common worktree-based workflow (pactum's own dogfooding runs in worktrees, so the just-merged guard immediately blocked it).\n\nThe guard's snapshot/restore operates correctly per-worktree: HEAD (symbolic-ref + hash), the index (write-tree), and `git reset --mixed` / `git update-ref` all work in a linked worktree, scoped to that worktree's own HEAD/index. So a linked worktree is NOT a reason to bail.\n\nFirst read internal/app/git_guard.go (the gitGuardPrechecks function and the multi-worktree check) and internal/app/git_guard_test.go to understand the existing precondition tests and helpers.\n\nFix: remove the blanket multi-worktree inconclusive precondition so the guard runs normally in a linked (or primary-with-siblings) worktree. KEEP every other precondition unchanged (attached HEAD, clean tree excluding .heurema, no unmerged paths, no in-progress merge/rebase/cherry-pick/revert/bisect/sequencer, no stale lock files, no submodules, no sparse/split index). Document the residual shared-ref edge in a code comment: refs are shared across linked worktrees, so (a) a branch checked out in ANOTHER worktree cannot be force-moved from this one — if the guard ever needs to restore such a ref and git refuses, that already surfaces loudly as executor_git_restore_failed; and (b) concurrent git activity in another worktree could in theory perturb shared refs mid-run — this is an accepted, documented residual for now and must NOT block.\n\nScope: internal/app/git_guard.go (remove/replace only the multi-worktree precondition, plus the explanatory comment) and internal/app/git_guard_test.go (add a test that the guard runs its normal snapshot/compare/restore inside a linked worktree created with `git worktree add` — e.g. an agent commit made inside the worktree is detected and restored as executor_git_history_mutation with the work left unstaged, NOT executor_git_guard_inconclusive). Do NOT weaken the other preconditions, change the snapshot/restore logic, or alter the terminal-reason set.\n\nAcceptance: a write-stage guard run inside a linked git worktree (created via `git worktree add`) does NOT return executor_git_guard_inconclusive merely because other worktrees exist; the guard performs its normal snapshot/compare/restore there, and an in-worktree agent commit is detected and restored as executor_git_history_mutation leaving the work unstaged; all other preconditions (detached HEAD, dirty non-.heurema tree, in-progress op, submodules, sparse/split index) still return executor_git_guard_inconclusive; a code comment documents the linked-worktree support and the residual shared-ref edge; make check passes.",
  "queries": [
    "internal/app/git_guard.go",
    "snapshot/restore",
    "/",
    "HEAD/index",
    "internal/app/git_guard_test.go",
    "merge/rebase/cherry-pick/revert/bisect/sequencer",
    "sparse/split",
    "remove/replace"
  ],
  "query_source": "task",
  "results": [],
  "warnings": [
    "Search index is stale. Run: pactum map refresh."
  ]
}

## Drafter guidance
- Propose only additions to the contract fields listed in the prompt.
- Do not change or restate the contract goal.
- Do not answer clarification questions.
- Do not edit files.
- Treat repository map/search context as navigation hints, not semantic truth.
