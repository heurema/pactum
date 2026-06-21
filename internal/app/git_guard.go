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
	gitGuardReasonRefMutation     = "executor_git_ref_mutation"
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
type gitGuardSnapshot struct {
	HeadSymRef    string            // symbolic ref (e.g. refs/heads/main)
	HeadHash      string            // HEAD commit hash at snapshot time
	BranchRef     string            // the current branch ref (same as HeadSymRef)
	Refs          map[string]string // all refs: refname -> object hash
	ReflogHints   map[string]string // per-ref reflog tip hash: ref key -> top-entry hash
	CommitObjects map[string]bool   // SHA set of all commit objects (including dangling)
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

	// HEAD must be a symbolic ref (attached to a branch, not detached).
	if _, err := gitExec(root, "symbolic-ref", "--quiet", "HEAD"); err != nil {
		return inconclusive()
	}

	// Resolve the git directory for state-file checks.
	gitDir, err := gitExec(root, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return inconclusiveErr(fmt.Errorf("git guard: rev-parse --absolute-git-dir: %w", err))
	}

	// Resolve the git common dir for shared-ref-store state checks. Linked
	// worktrees are supported — refs are shared across all linked worktrees via
	// the common dir, so:
	//   (a) a branch checked out in another worktree cannot be force-moved from
	//       this one — if the guard must restore such a ref and git refuses, it
	//       surfaces as executor_git_restore_failed;
	//   (b) concurrent git activity in another worktree could perturb shared refs
	//       mid-run — this is an accepted, documented residual and does not block.
	//
	// Stale lock path resolution uses the common dir for shared-ref-store locks
	// (refs.lock, packed-refs.lock, loose-ref locks under refs/). When
	// --git-common-dir returns a relative path (e.g. ".git" in a primary worktree)
	// it is resolved via filepath.Join(root, commonDir), where root is the
	// directory used with "git -C root", because git's relative output is rooted
	// at root, not the process CWD. index.lock remains resolved against the
	// per-worktree git dir (always absolute, from --absolute-git-dir) because each
	// worktree has its own index.
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

// gitGuardSnapshot captures the current git state as a mutation-detection baseline.
func gitGuardTakeSnapshot(root string) (*gitGuardSnapshot, error) {
	snap := &gitGuardSnapshot{}

	// HEAD symbolic ref.
	symRef, err := gitExec(root, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git guard: symbolic-ref HEAD: %w", err)
	}
	snap.HeadSymRef = strings.TrimSpace(symRef)
	snap.BranchRef = snap.HeadSymRef

	// HEAD commit hash.
	headHash, err := gitExec(root, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git guard: rev-parse HEAD: %w", err)
	}
	snap.HeadHash = strings.TrimSpace(headHash)

	// All refs.
	refsOut, err := gitExec(root, "for-each-ref", "--format=%(refname) %(objectname)")
	if err != nil {
		return nil, fmt.Errorf("git guard: for-each-ref: %w", err)
	}
	snap.Refs = parseForEachRef(refsOut)

	// Reflog tip hints: HEAD (tip + second entry) for commit-then-reset-hard
	// detection, and tip-only for every other ref to catch advance-and-retract.
	snap.ReflogHints = make(map[string]string)
	if tip, err := gitExec(root, "reflog", "--format=%H", "-n", "1", "HEAD"); err == nil && strings.TrimSpace(tip) != "" {
		snap.ReflogHints["HEAD"] = strings.TrimSpace(tip)
	}
	if second, err := gitExec(root, "rev-parse", "HEAD@{1}"); err == nil && strings.TrimSpace(second) != "" {
		snap.ReflogHints["HEAD@{1}"] = strings.TrimSpace(second)
	}
	// Capture reflog tip for every non-HEAD ref (including stash).
	for ref := range snap.Refs {
		if ref == snap.HeadSymRef {
			continue // HEAD's reflog is already captured above
		}
		if tip, err := gitExec(root, "reflog", "--format=%H", "-n", "1", ref); err == nil && strings.TrimSpace(tip) != "" {
			snap.ReflogHints[ref] = strings.TrimSpace(tip)
		}
	}

	// Commit object set: all commit objects in the object store, including
	// dangling ones not reachable from any ref. git commit-tree can create
	// these without touching HEAD, any ref, or the reflog.
	snap.CommitObjects = collectCommitObjects(root)

	// Index tree hash. git write-tree creates a tree object from the current
	// index without creating a commit; the object is loose and harmless.
	indexTree, err := gitExec(root, "write-tree")
	if err != nil {
		return nil, fmt.Errorf("git guard: write-tree: %w", err)
	}
	snap.IndexTreeHash = strings.TrimSpace(indexTree)

	return snap, nil
}

// collectCommitObjects returns the SHA set of all commit objects in the object
// store, including dangling commits not reachable from any ref.
func collectCommitObjects(root string) map[string]bool {
	out, err := gitExec(root, "cat-file", "--batch-check=%(objecttype) %(objectname)", "--batch-all-objects")
	if err != nil {
		return map[string]bool{}
	}
	commits := make(map[string]bool)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "commit ") {
			sha := strings.TrimPrefix(line, "commit ")
			if sha != "" {
				commits[sha] = true
			}
		}
	}
	return commits
}

// parseForEachRef parses "for-each-ref --format=%(refname) %(objectname)" output.
func parseForEachRef(out string) map[string]string {
	refs := make(map[string]string)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			refs[parts[0]] = parts[1]
		}
	}
	return refs
}

// gitGuardCompareAndRestore compares the post-transport git state against the
// snapshot and restores what it can. It always runs (even when the transport
// errored) so that agent-caused git mutations are detected and reported.
//
// transportErr is the error (if any) from the agent transport; it informs
// detail fields but does not suppress detection or restoration.
//
// The restore logic follows the precedence rule (highest first):
//
//	inconclusive > restore_failed > history_mutation > ref_mutation > index_mutation
//
// When inconclusive is detected, all restoration is suppressed.
func gitGuardCompareAndRestore(root string, snap *gitGuardSnapshot, isReviewFix bool, transportErr error) gitGuardOutcome {
	outcome := gitGuardOutcome{}
	if transportErr != nil {
		outcome.Detail = "transport error: " + transportErr.Error()
	}

	// Current HEAD symbolic ref.
	curSymRef, err := gitExec(root, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		// Detached HEAD after the agent ran — inconclusive.
		outcome.TerminalReason = gitGuardReasonInconclusive
		outcome.MutationClass = "detached_head"
		return outcome
	}
	curSymRef = strings.TrimSpace(curSymRef)

	// Current HEAD hash.
	curHeadHash, err := gitExec(root, "rev-parse", "HEAD")
	if err != nil {
		outcome.TerminalReason = gitGuardReasonInconclusive
		outcome.MutationClass = "head_unresolvable"
		return outcome
	}
	curHeadHash = strings.TrimSpace(curHeadHash)

	// Current reflog entries for HEAD (tip and second-most-recent).
	curReflogTip, _ := gitExec(root, "reflog", "--format=%H", "-n", "1", "HEAD")
	curReflogTip = strings.TrimSpace(curReflogTip)
	curReflogSecond, _ := gitExec(root, "rev-parse", "HEAD@{1}")
	curReflogSecond = strings.TrimSpace(curReflogSecond)

	// Current index tree.
	curIndexTree, err := gitExec(root, "write-tree")
	if err != nil {
		outcome.TerminalReason = gitGuardReasonInconclusive
		outcome.MutationClass = "index_unresolvable"
		return outcome
	}
	curIndexTree = strings.TrimSpace(curIndexTree)

	// Current refs.
	refsOut, err := gitExec(root, "for-each-ref", "--format=%(refname) %(objectname)")
	if err != nil {
		outcome.TerminalReason = gitGuardReasonInconclusive
		outcome.MutationClass = "refs_unresolvable"
		return outcome
	}
	curRefs := parseForEachRef(refsOut)

	// ------- Classify mutations -------

	// Stash created or changed → inconclusive.
	prevStash := snap.Refs["refs/stash"]
	curStash := curRefs["refs/stash"]
	if curStash != prevStash && (curStash != "" || prevStash != "") {
		outcome.TerminalReason = gitGuardReasonInconclusive
		outcome.MutationClass = "stash_created_or_changed"
		return outcome
	}

	// HEAD branch changed (agent switched branches) → inconclusive.
	if curSymRef != snap.HeadSymRef {
		outcome.TerminalReason = gitGuardReasonInconclusive
		outcome.MutationClass = "branch_switched"
		return outcome
	}

	// HEAD hash unchanged but reflog advanced → commit-then-reset-hard or similar.
	// We check both @{0} (tip) and @{1} (second entry): after a commit+reset-hard the
	// tip hash reverts to the original value but @{1} now holds the agent's commit.
	snapReflogTip := snap.ReflogHints["HEAD"]
	snapReflogSecond := snap.ReflogHints["HEAD@{1}"]
	headHashChanged := curHeadHash != snap.HeadHash
	reflogAdvanced := (curReflogTip != snapReflogTip || curReflogSecond != snapReflogSecond) && curReflogTip != ""

	if !headHashChanged && reflogAdvanced {
		// The reflog advanced while HEAD ended at the same hash. This is the
		// commit-then-reset-hard (or similar) pattern. The agent's committed
		// work is in dangling commits accessible via the reflog.
		outcome.TerminalReason = gitGuardReasonInconclusive
		outcome.MutationClass = "head_unchanged_reflog_advanced"
		return outcome
	}

	// Non-HEAD ref reflog check: detect refs that were moved and then restored
	// (advance-and-retract). A ref whose SHA is back to the snapshot value but
	// whose reflog tip changed means it was modified mid-run.
	for ref, snapTip := range snap.ReflogHints {
		if ref == "HEAD" || ref == "HEAD@{1}" {
			continue // HEAD reflog is already handled above
		}
		// Only check refs whose SHA is still in curRefs and unchanged.
		snapSHA, hadSHA := snap.Refs[ref]
		curSHA, stillExists := curRefs[ref]
		if !hadSHA || !stillExists || curSHA != snapSHA {
			continue // SHA-level change handled by ref mutation detection below
		}
		curTip, _ := gitExec(root, "reflog", "--format=%H", "-n", "1", ref)
		curTip = strings.TrimSpace(curTip)
		if curTip != "" && curTip != snapTip {
			outcome.TerminalReason = gitGuardReasonInconclusive
			outcome.MutationClass = "ref_reflog_advanced"
			return outcome
		}
	}

	// Detect changed non-HEAD refs.
	var changedRefs, addedRefs, deletedRefs []string
	for ref, sha := range curRefs {
		if ref == snap.BranchRef {
			continue // the current branch ref is handled via HEAD hash check
		}
		if ref == "refs/stash" {
			continue // handled above
		}
		prevSHA, existed := snap.Refs[ref]
		if !existed {
			addedRefs = append(addedRefs, ref)
		} else if sha != prevSHA {
			changedRefs = append(changedRefs, ref)
		}
	}
	for ref := range snap.Refs {
		if ref == snap.BranchRef || ref == "refs/stash" {
			continue
		}
		if _, ok := curRefs[ref]; !ok {
			deletedRefs = append(deletedRefs, ref)
		}
	}
	hasRefMutation := len(changedRefs)+len(addedRefs)+len(deletedRefs) > 0

	// Collect all detected mutation classes to apply precedence.
	type mutKind int
	const (
		mutHistory mutKind = iota
		mutRef
		mutIndex
	)
	var detectedMuts []mutKind

	if headHashChanged {
		detectedMuts = append(detectedMuts, mutHistory)
	}
	if hasRefMutation {
		detectedMuts = append(detectedMuts, mutRef)
	}
	if curIndexTree != snap.IndexTreeHash && !headHashChanged {
		// Only flag index as a separate mutation class when HEAD didn't move;
		// if HEAD moved, the index change is expected (reset during restore).
		detectedMuts = append(detectedMuts, mutIndex)
	}

	if len(detectedMuts) == 0 {
		// No history, ref, or index mutations detected. Check for new commit
		// objects created via low-level commands (e.g. git commit-tree) that
		// bypass refs entirely, leaving dangling commits in the object store.
		// This check only applies when HEAD didn't move; if HEAD moved the new
		// commits are already accounted for by the history mutation path above.
		if !headHashChanged {
			curCommitObjects := collectCommitObjects(root)
			for sha := range curCommitObjects {
				if !snap.CommitObjects[sha] {
					outcome.TerminalReason = gitGuardReasonInconclusive
					outcome.MutationClass = "new_commit_objects"
					return outcome
				}
			}
		}
		return gitGuardOutcome{}
	}

	// ------- Restore -------

	// dominantClass returns the highest-precedence class label.
	dominantClass := func() string {
		for _, m := range []mutKind{mutHistory, mutRef, mutIndex} {
			for _, d := range detectedMuts {
				if d == m {
					switch m {
					case mutHistory:
						return gitGuardReasonHistoryMutation
					case mutRef:
						return gitGuardReasonRefMutation
					case mutIndex:
						return gitGuardReasonIndexMutation
					}
				}
			}
		}
		return ""
	}

	var restoreErrs []string
	restored := []string{}

	// Restore history mutation: reset the branch to the original commit.
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

	// Restore changed non-HEAD refs to their snapshot values.
	for _, ref := range changedRefs {
		if err := gitExecNoOut(root, "update-ref", ref, snap.Refs[ref]); err != nil {
			restoreErrs = append(restoreErrs, fmt.Sprintf("update-ref %s: %s", ref, err.Error()))
		} else {
			restored = append(restored, "ref "+ref+" restored to "+snap.Refs[ref])
		}
	}
	// Delete refs that the agent added.
	for _, ref := range addedRefs {
		if err := gitExecNoOut(root, "update-ref", "-d", ref); err != nil {
			restoreErrs = append(restoreErrs, fmt.Sprintf("update-ref -d %s: %s", ref, err.Error()))
		} else {
			restored = append(restored, "ref "+ref+" deleted")
		}
	}
	// Restore refs that the agent deleted.
	for _, ref := range deletedRefs {
		if err := gitExecNoOut(root, "update-ref", ref, snap.Refs[ref]); err != nil {
			restoreErrs = append(restoreErrs, fmt.Sprintf("update-ref (restore deleted) %s: %s", ref, err.Error()))
		} else {
			restored = append(restored, "ref "+ref+" re-created at "+snap.Refs[ref])
		}
	}

	outcome.MutationClass = dominantClass()
	if len(restoreErrs) > 0 {
		outcome.TerminalReason = gitGuardReasonRestoreFailed
		outcome.RestoreError = strings.Join(restoreErrs, "; ")
	} else {
		outcome.TerminalReason = dominantClass()
	}
	if len(restored) > 0 {
		outcome.RestoredState = strings.Join(restored, "; ")
	}

	return outcome
}
