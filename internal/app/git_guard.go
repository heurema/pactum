package app

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Terminal reasons for git guard outcomes; precedence highest first.
const (
	gitGuardReasonInconclusive    = "executor_git_guard_inconclusive"
	gitGuardReasonRestoreFailed   = "executor_git_restore_failed"
	gitGuardReasonHistoryMutation = "executor_git_history_mutation"
	gitGuardReasonIndexMutation   = "executor_git_index_mutation"
)

// gitGuardOutcome records what the guard detected and did.
// It is embedded in execute and review-fix result documents.
type gitGuardOutcome struct {
	TerminalReason string `json:"terminal_reason,omitempty"`
	MutationClass  string `json:"mutation_class,omitempty"`
	RestoredState  string `json:"restored_state,omitempty"`
	RestoreError   string `json:"restore_error,omitempty"`
	Detail         string `json:"detail,omitempty"`
}

// gitGuardSnapshot captures minimal git state for post-transport comparison.
// Only HEAD, HEAD reflog, and the index are tracked — refs and commit objects
// that belong to the shared common store are not snapshotted, so concurrent
// activity in other linked worktrees cannot cause false positives.
type gitGuardSnapshot struct {
	HeadSymRef    string            // symbolic ref (e.g. refs/heads/main); empty for detached HEAD
	HeadHash      string            // HEAD commit hash at snapshot time
	ReflogHints   map[string]string // HEAD reflog: "HEAD" (tip) and "HEAD@{1}" (second entry)
	HeadReflogLen int               // number of HEAD reflog entries at snapshot time; used for commit-then-reset detection
	IndexTreeHash string            // tree SHA written from the current index
}

// gitExec runs a git command under root and returns trimmed stdout.
// It does NOT use the read-only internal/gitctx wrapper; those writes are
// guard-owned operations that need access to write subcommands.
func gitExec(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...) //nolint:gosec
	out, err := cmd.Output()
	return strings.TrimRight(string(out), "\n"), err
}

// gitExecNoOut runs a git command and discards stdout; returns stderr on error.
func gitExecNoOut(root string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...) //nolint:gosec
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// gitGuardPrechecks validates the invariants that must hold before running the
// agent transport on a write stage. Returns (ok, terminalReason, snapshot, error).
// When ok is true the snapshot is non-nil and ready for comparison. When ok is
// false the snapshot is nil and terminalReason is set.
//
// For execute stages (isReviewFix=false), the working tree must be clean
// outside .heurema/ (pactum's own state directory).
//
// For review-fix stages (isReviewFix=true), a dirty working tree is expected
// (execute-phase changes) and the clean-tree check is skipped.
func gitGuardPrechecks(root string, isReviewFix bool) (bool, string, *gitGuardSnapshot, error) {
	inconclusive := func() (bool, string, *gitGuardSnapshot, error) {
		return false, gitGuardReasonInconclusive, nil, nil
	}
	inconclusiveErr := func(err error) (bool, string, *gitGuardSnapshot, error) {
		return false, gitGuardReasonInconclusive, nil, err
	}

	// If root is not inside a git work tree, the guard is a no-op: there is no
	// history to protect and the agent cannot corrupt refs that don't exist.
	if _, err := gitExec(root, "rev-parse", "--is-inside-work-tree"); err != nil {
		return true, "", nil, nil
	}

	// Resolve the git directory for state-file checks.
	gitDir, err := gitExec(root, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return inconclusiveErr(fmt.Errorf("git guard: rev-parse --absolute-git-dir: %w", err))
	}

	// Resolve the git common dir for stale lock-file detection only. Refs and
	// commit objects in the common store are not snapshotted, so concurrent
	// activity in other linked worktrees cannot cause false positives. When
	// --git-common-dir returns a relative path (e.g. ".git" in a primary
	// worktree) it is resolved via filepath.Join(root, commonDir).
	commonDir, err := gitExec(root, "rev-parse", "--git-common-dir")
	if err != nil {
		return inconclusiveErr(fmt.Errorf("git guard: rev-parse --git-common-dir: %w", err))
	}
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(root, commonDir)
	}

	// No in-progress operations.
	for _, markerFile := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "BISECT_LOG"} {
		if _, err := os.Stat(filepath.Join(gitDir, markerFile)); err == nil {
			return inconclusive()
		}
	}
	for _, markerDir := range []string{"rebase-merge", "rebase-apply", "sequencer"} {
		if _, err := os.Stat(filepath.Join(gitDir, markerDir)); err == nil {
			return inconclusive()
		}
	}

	// No stale lock files. index.lock is per-worktree; resolved against the
	// absolute git dir. Shared-ref-store locks (refs.lock, packed-refs.lock, loose
	// ref locks under refs/) are resolved against the common dir so that detection
	// is effective in both primary and linked worktrees.
	if _, err := os.Stat(filepath.Join(gitDir, "index.lock")); err == nil {
		return inconclusive()
	}
	for _, lockFile := range []string{"refs.lock", "packed-refs.lock"} {
		if _, err := os.Stat(filepath.Join(commonDir, lockFile)); err == nil {
			return inconclusive()
		}
	}
	// No loose ref lock files (e.g. refs/heads/main.lock).
	var foundRefLock bool
	_ = filepath.WalkDir(filepath.Join(commonDir, "refs"), func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".lock") {
			foundRefLock = true
			return fs.SkipAll
		}
		return nil
	})
	if foundRefLock {
		return inconclusive()
	}

	// No submodules.
	if _, err := os.Stat(filepath.Join(root, ".gitmodules")); err == nil {
		return inconclusive()
	}

	// No sparse index.
	sparseVal, _ := gitExec(root, "config", "--bool", "--get", "extensions.sparseIndex")
	if strings.TrimSpace(sparseVal) == "true" {
		return inconclusive()
	}

	// No split index (sharedindex.* files in git dir).
	if entries, err := os.ReadDir(gitDir); err == nil {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "sharedindex.") {
				return inconclusive()
			}
		}
	}

	// No unmerged paths.
	unmergedOut, err := gitExec(root, "ls-files", "-z", "--unmerged")
	if err != nil {
		return inconclusiveErr(fmt.Errorf("git guard: ls-files --unmerged: %w", err))
	}
	if strings.TrimSpace(unmergedOut) != "" {
		return inconclusive()
	}

	// Clean working tree (execute stage only; review-fix starts dirty by design).
	if !isReviewFix {
		// Exclude pactum's own state directory from the clean-tree check. Pactum
		// writes ledger and run-record artifacts there during normal operation.
		statusOut, err := gitExec(root, "status", "--porcelain=v1", "-z", "--", ":!.heurema/")
		if err != nil {
			return inconclusiveErr(fmt.Errorf("git guard: status: %w", err))
		}
		if strings.TrimSpace(statusOut) != "" {
			return inconclusive()
		}
	}

	snap, err := gitGuardTakeSnapshot(root)
	if err != nil {
		return inconclusiveErr(fmt.Errorf("git guard: snapshot: %w", err))
	}
	return true, "", snap, nil
}

// gitGuardTakeSnapshot captures the current git state as a mutation-detection baseline.
// Only this worktree's HEAD, HEAD reflog, and the index are recorded — shared refs and
// the commit-object store are intentionally omitted so concurrent activity in other
// linked worktrees does not cause false positives.
func gitGuardTakeSnapshot(root string) (*gitGuardSnapshot, error) {
	snap := &gitGuardSnapshot{}

	// HEAD symbolic ref; empty for detached HEAD, which is a valid initial state.
	symRef, _ := gitExec(root, "symbolic-ref", "--quiet", "HEAD")
	snap.HeadSymRef = strings.TrimSpace(symRef)

	// HEAD commit hash.
	headHash, err := gitExec(root, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git guard: rev-parse HEAD: %w", err)
	}
	snap.HeadHash = strings.TrimSpace(headHash)

	// HEAD reflog hints: tip and second entry for commit-then-reset detection.
	snap.ReflogHints = make(map[string]string)
	if tip, err := gitExec(root, "reflog", "--format=%H", "-n", "1", "HEAD"); err == nil && strings.TrimSpace(tip) != "" {
		snap.ReflogHints["HEAD"] = strings.TrimSpace(tip)
	}
	if second, err := gitExec(root, "rev-parse", "HEAD@{1}"); err == nil && strings.TrimSpace(second) != "" {
		snap.ReflogHints["HEAD@{1}"] = strings.TrimSpace(second)
	}

	// Count HEAD reflog entries so compare can identify entries added after snapshot.
	if reflogOut, err := gitExec(root, "reflog", "--format=%H", "HEAD"); err == nil {
		for _, line := range strings.Split(reflogOut, "\n") {
			if strings.TrimSpace(line) != "" {
				snap.HeadReflogLen++
			}
		}
	}

	// Index tree hash.
	indexTree, err := gitExec(root, "write-tree")
	if err != nil {
		return nil, fmt.Errorf("git guard: write-tree: %w", err)
	}
	snap.IndexTreeHash = strings.TrimSpace(indexTree)

	return snap, nil
}

// gitGuardCompareAndRestore compares the post-transport git state against the
// snapshot and restores what it can. It always runs (even when the transport
// errored) so that agent-caused git mutations are detected and reported.
//
// transportErr is the error (if any) from the agent transport; it informs
// detail fields but does not suppress detection or restoration.
//
// Only mutations attributable to this worktree's agent are classified:
// HEAD symbolic-ref change (branch switch), HEAD hash change (commit),
// HEAD reflog advance while hash is unchanged (commit-then-reset), and
// index-only staging. Changes to other refs, tags, remote refs, stash, or
// commit objects in the shared store are not examined — they may originate
// from concurrent activity in other linked worktrees.
//
// Precedence (highest first):
//
//	inconclusive > restore_failed > history_mutation > index_mutation
//
// An index-only mutation is benign: it is detected and restored, but
// TerminalReason is left empty so the attempt completes.
func gitGuardCompareAndRestore(root string, snap *gitGuardSnapshot, isReviewFix bool, transportErr error) gitGuardOutcome {
	outcome := gitGuardOutcome{}
	if transportErr != nil {
		outcome.Detail = "transport error: " + transportErr.Error()
	}

	// Branch-switch / HEAD-attachment detection.
	if snap.HeadSymRef != "" {
		// Started on a branch: detect detach or switch to another branch.
		curSymRef, err := gitExec(root, "symbolic-ref", "--quiet", "HEAD")
		if err != nil {
			// Agent detached HEAD after starting on a branch — inconclusive.
			outcome.TerminalReason = gitGuardReasonInconclusive
			outcome.MutationClass = "detached_head"
			return outcome
		}
		if strings.TrimSpace(curSymRef) != snap.HeadSymRef {
			outcome.TerminalReason = gitGuardReasonInconclusive
			outcome.MutationClass = "branch_switched"
			return outcome
		}
	} else {
		// Started in detached HEAD; if HEAD is now attached the agent checked out
		// or created a branch — we cannot safely attribute that branch's state.
		if curSymRef, err := gitExec(root, "symbolic-ref", "--quiet", "HEAD"); err == nil && strings.TrimSpace(curSymRef) != "" {
			outcome.TerminalReason = gitGuardReasonInconclusive
			outcome.MutationClass = "branch_attached"
			return outcome
		}
	}

	// Current HEAD hash.
	curHeadHash, err := gitExec(root, "rev-parse", "HEAD")
	if err != nil {
		outcome.TerminalReason = gitGuardReasonInconclusive
		outcome.MutationClass = "head_unresolvable"
		return outcome
	}
	curHeadHash = strings.TrimSpace(curHeadHash)

	// Current index tree.
	curIndexTree, err := gitExec(root, "write-tree")
	if err != nil {
		outcome.TerminalReason = gitGuardReasonInconclusive
		outcome.MutationClass = "index_unresolvable"
		return outcome
	}
	curIndexTree = strings.TrimSpace(curIndexTree)

	headHashChanged := curHeadHash != snap.HeadHash

	// Commit-then-reset detection: HEAD ended at the same hash but the reflog
	// gained entries since the snapshot. Scan only the new entries (those added
	// after the snapshot) and flag any entry whose hash differs from snap.HeadHash:
	// that indicates the agent committed to a new hash and then reset back.
	// Entries equal to snap.HeadHash (e.g. from `git reset --hard HEAD`) are
	// harmless and do not trigger this check.
	if !headHashChanged && snap.HeadReflogLen > 0 {
		if reflogOut, err := gitExec(root, "reflog", "--format=%H", "HEAD"); err == nil {
			var curEntries []string
			for _, line := range strings.Split(reflogOut, "\n") {
				if h := strings.TrimSpace(line); h != "" {
					curEntries = append(curEntries, h)
				}
			}
			newCount := len(curEntries) - snap.HeadReflogLen
			for i := 0; i < newCount && i < len(curEntries); i++ {
				if curEntries[i] != snap.HeadHash {
					outcome.TerminalReason = gitGuardReasonInconclusive
					outcome.MutationClass = "head_unchanged_reflog_advanced"
					return outcome
				}
			}
		}
	}

	// Collect detected mutation classes.
	type mutKind int
	const (
		mutHistory mutKind = iota
		mutIndex
	)
	var detectedMuts []mutKind

	if headHashChanged {
		detectedMuts = append(detectedMuts, mutHistory)
	}
	if curIndexTree != snap.IndexTreeHash && !headHashChanged {
		// Only flag index as a separate mutation class when HEAD didn't move;
		// if HEAD moved, the index change is expected (reset during restore).
		detectedMuts = append(detectedMuts, mutIndex)
	}

	if len(detectedMuts) == 0 {
		return gitGuardOutcome{}
	}

	// ------- Restore -------

	dominantClass := func() string {
		for _, m := range []mutKind{mutHistory, mutIndex} {
			for _, d := range detectedMuts {
				if d == m {
					switch m {
					case mutHistory:
						return gitGuardReasonHistoryMutation
					case mutIndex:
						return gitGuardReasonIndexMutation
					}
				}
			}
		}
		return ""
	}

	var restoreErrs []string
	var restored []string

	// Restore history mutation: reset HEAD to the original commit.
	if headHashChanged {
		if err := gitExecNoOut(root, "reset", "--mixed", snap.HeadHash); err != nil {
			restoreErrs = append(restoreErrs, "reset --mixed: "+err.Error())
		} else {
			restored = append(restored, "branch reset to "+snap.HeadHash)
			// For review-fix: restore the index to the pre-fixer snapshot state
			// (execute-phase staged changes) rather than leaving it at origHash's tree.
			if isReviewFix {
				if err := gitExecNoOut(root, "read-tree", snap.IndexTreeHash); err != nil {
					restoreErrs = append(restoreErrs, "read-tree (review-fix index restore): "+err.Error())
				} else {
					restored = append(restored, "index restored to snapshot tree "+snap.IndexTreeHash)
				}
			}
		}
	}

	// Restore index-only mutation (no HEAD change): restore the index to snapshot state.
	if !headHashChanged && curIndexTree != snap.IndexTreeHash {
		if err := gitExecNoOut(root, "read-tree", snap.IndexTreeHash); err != nil {
			restoreErrs = append(restoreErrs, "read-tree (index restore): "+err.Error())
		} else {
			restored = append(restored, "index restored to snapshot tree "+snap.IndexTreeHash)
		}
	}

	outcome.MutationClass = dominantClass()
	if len(restoreErrs) > 0 {
		outcome.TerminalReason = gitGuardReasonRestoreFailed
		outcome.RestoreError = strings.Join(restoreErrs, "; ")
	} else if outcome.MutationClass == gitGuardReasonIndexMutation {
		// Index-only staging is benign and does NOT void the attempt.
		outcome.TerminalReason = ""
	} else {
		outcome.TerminalReason = dominantClass()
	}
	if len(restored) > 0 {
		outcome.RestoredState = strings.Join(restored, "; ")
	}

	return outcome
}
