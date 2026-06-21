package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// mustGitG runs a git command in root and fails the test on error.
func mustGitG(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// initTestRepo creates a minimal git repo with one commit and returns its root.
func initTestRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustGitG(t, root, "init")
	mustGitG(t, root, "config", "user.email", "test@test.com")
	mustGitG(t, root, "config", "user.name", "Test")
	mustGitG(t, root, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("init"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitG(t, root, "add", "a.txt")
	mustGitG(t, root, "commit", "-m", "init")
	return root
}

func skipIfNoGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

// ---------------------------------------------------------------------------
// gitGuardPrechecks
// ---------------------------------------------------------------------------

func TestGitGuardPrechecks_CleanTreePasses(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)

	ok, reason, snap, err := gitGuardPrechecks(root, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true, got reason=%q", reason)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot when ok=true")
	}
	if snap.HeadHash == "" {
		t.Error("snapshot HeadHash is empty")
	}
	if snap.IndexTreeHash == "" {
		t.Error("snapshot IndexTreeHash is empty")
	}
}

func TestGitGuardPrechecks_DetachedHead(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)
	hash := mustGitG(t, root, "rev-parse", "HEAD")
	mustGitG(t, root, "checkout", "--detach", hash)

	ok, reason, snap, err := gitGuardPrechecks(root, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for detached HEAD")
	}
	if reason != gitGuardReasonInconclusive {
		t.Errorf("expected %q, got %q", gitGuardReasonInconclusive, reason)
	}
	if snap != nil {
		t.Error("expected nil snapshot when ok=false")
	}
}

func TestGitGuardPrechecks_DirtyTreeBlocksExecute(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)
	// Unstaged modification to a tracked file.
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, reason, _, err := gitGuardPrechecks(root, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for dirty tree on execute stage")
	}
	if reason != gitGuardReasonInconclusive {
		t.Errorf("expected %q, got %q", gitGuardReasonInconclusive, reason)
	}
}

func TestGitGuardPrechecks_DirtyTreeAllowedForReviewFix(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)
	// Unstaged modification — simulate execute-phase changes.
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("execute changed"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, reason, snap, err := gitGuardPrechecks(root, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true for dirty tree on review-fix stage, got reason=%q", reason)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
}

func TestGitGuardPrechecks_HeuremaExcludedFromCleanCheck(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)
	// Create an untracked file inside .heurema/ — pactum's own state dir.
	if err := os.MkdirAll(filepath.Join(root, ".heurema", "pactum"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".heurema", "pactum", "state.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, reason, snap, err := gitGuardPrechecks(root, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true: .heurema/ should be excluded from clean-tree check, got reason=%q", reason)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
}

func TestGitGuardPrechecks_MergeInProgress(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)
	// Simulate a merge in progress by creating MERGE_HEAD.
	gitDir := mustGitG(t, root, "rev-parse", "--absolute-git-dir")
	hash := mustGitG(t, root, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(gitDir, "MERGE_HEAD"), []byte(hash+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, reason, _, err := gitGuardPrechecks(root, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when MERGE_HEAD exists")
	}
	if reason != gitGuardReasonInconclusive {
		t.Errorf("expected %q, got %q", gitGuardReasonInconclusive, reason)
	}
}

func TestGitGuardPrechecks_IndexLock(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)
	gitDir := mustGitG(t, root, "rev-parse", "--absolute-git-dir")
	if err := os.WriteFile(filepath.Join(gitDir, "index.lock"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	ok, reason, _, err := gitGuardPrechecks(root, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when index.lock exists")
	}
	if reason != gitGuardReasonInconclusive {
		t.Errorf("expected %q, got %q", gitGuardReasonInconclusive, reason)
	}
}

func TestGitGuardPrechecks_Submodule(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)
	// Create a .gitmodules file without actually initialising a submodule.
	if err := os.WriteFile(filepath.Join(root, ".gitmodules"), []byte("[submodule]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, reason, _, err := gitGuardPrechecks(root, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when .gitmodules exists")
	}
	if reason != gitGuardReasonInconclusive {
		t.Errorf("expected %q, got %q", gitGuardReasonInconclusive, reason)
	}
}

// ---------------------------------------------------------------------------
// gitGuardCompareAndRestore — no mutation
// ---------------------------------------------------------------------------

func TestGitGuardCompareAndRestore_NoMutation(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)

	snap, err := gitGuardTakeSnapshot(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	outcome := gitGuardCompareAndRestore(root, snap, false, nil)
	if outcome.TerminalReason != "" {
		t.Errorf("expected no terminal reason, got %q", outcome.TerminalReason)
	}
}

// ---------------------------------------------------------------------------
// gitGuardCompareAndRestore — history mutation (agent committed)
// ---------------------------------------------------------------------------

func TestGitGuardCompareAndRestore_AgentCommit_Detected_And_Restored(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)

	snap, err := gitGuardTakeSnapshot(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	// Simulate agent making a commit.
	if err := os.WriteFile(filepath.Join(root, "agent.txt"), []byte("agent work"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitG(t, root, "add", "agent.txt")
	mustGitG(t, root, "commit", "-m", "agent commit")

	outcome := gitGuardCompareAndRestore(root, snap, false, nil)

	if outcome.TerminalReason != gitGuardReasonHistoryMutation {
		t.Errorf("expected terminal_reason=%q, got %q", gitGuardReasonHistoryMutation, outcome.TerminalReason)
	}

	// HEAD should be restored to the original hash.
	currentHead := mustGitG(t, root, "rev-parse", "HEAD")
	if currentHead != snap.HeadHash {
		t.Errorf("HEAD not restored: want %s, got %s", snap.HeadHash, currentHead)
	}

	// The agent's changes should be present as unstaged working-tree modifications.
	statusOut := mustGitG(t, root, "status", "--porcelain=v1")
	if !strings.Contains(statusOut, "agent.txt") {
		t.Errorf("expected agent.txt to appear in status after restore, got: %q", statusOut)
	}
}

// ---------------------------------------------------------------------------
// gitGuardCompareAndRestore — index-only mutation
// ---------------------------------------------------------------------------

func TestGitGuardCompareAndRestore_IndexOnly_Detected_And_Restored(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)

	snap, err := gitGuardTakeSnapshot(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	// Simulate agent staging a file without committing.
	if err := os.WriteFile(filepath.Join(root, "staged.txt"), []byte("staged"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitG(t, root, "add", "staged.txt")

	outcome := gitGuardCompareAndRestore(root, snap, false, nil)

	if outcome.TerminalReason != gitGuardReasonIndexMutation {
		t.Errorf("expected terminal_reason=%q, got %q", gitGuardReasonIndexMutation, outcome.TerminalReason)
	}

	// Index should be restored: staged.txt must no longer be in the index.
	lsOut, err := exec.Command("git", "-C", root, "ls-files", "--cached", "staged.txt").Output()
	if err != nil {
		t.Fatalf("ls-files: %v", err)
	}
	if strings.TrimSpace(string(lsOut)) != "" {
		t.Errorf("staged.txt should be removed from index after restore, but still present")
	}

	// The file should remain in the working tree.
	if _, err := os.Stat(filepath.Join(root, "staged.txt")); err != nil {
		t.Errorf("staged.txt should remain in working tree after restore: %v", err)
	}
}

// ---------------------------------------------------------------------------
// gitGuardCompareAndRestore — commit-then-reset-hard → inconclusive
// ---------------------------------------------------------------------------

func TestGitGuardCompareAndRestore_CommitThenResetHard_Inconclusive(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)

	snap, err := gitGuardTakeSnapshot(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	origHead := snap.HeadHash

	// Simulate agent: commit then reset --hard back to original HEAD.
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("secret work"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitG(t, root, "add", "secret.txt")
	mustGitG(t, root, "commit", "-m", "agent secret commit")
	mustGitG(t, root, "reset", "--hard", origHead)

	outcome := gitGuardCompareAndRestore(root, snap, false, nil)

	if outcome.TerminalReason != gitGuardReasonInconclusive {
		t.Errorf("expected terminal_reason=%q, got %q", gitGuardReasonInconclusive, outcome.TerminalReason)
	}
	// No restoration should be attempted — HEAD stays at origHead (it already was).
	currentHead := mustGitG(t, root, "rev-parse", "HEAD")
	if currentHead != origHead {
		t.Errorf("HEAD should remain at origHead %s, got %s", origHead, currentHead)
	}
}

// ---------------------------------------------------------------------------
// gitGuardCompareAndRestore — stash → inconclusive
// ---------------------------------------------------------------------------

func TestGitGuardCompareAndRestore_StashCreated_Inconclusive(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)

	snap, err := gitGuardTakeSnapshot(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	// Simulate agent stashing changes.
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("modified"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitG(t, root, "stash", "push", "-m", "agent stash")

	outcome := gitGuardCompareAndRestore(root, snap, false, nil)

	if outcome.TerminalReason != gitGuardReasonInconclusive {
		t.Errorf("expected terminal_reason=%q, got %q", gitGuardReasonInconclusive, outcome.TerminalReason)
	}
}

// ---------------------------------------------------------------------------
// gitGuardCompareAndRestore — ref mutation (non-HEAD ref changed)
// ---------------------------------------------------------------------------

func TestGitGuardCompareAndRestore_RefMutation_Detected_And_Restored(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)

	// Create an extra ref pointing to the initial commit.
	origHash := mustGitG(t, root, "rev-parse", "HEAD")
	mustGitG(t, root, "update-ref", "refs/heads/extra", origHash)

	snap, err := gitGuardTakeSnapshot(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	// Create a new commit object WITHOUT touching HEAD (so HEAD reflog does not advance).
	// git commit-tree creates a commit object directly from the current tree.
	tree := mustGitG(t, root, "write-tree")
	newCommit := mustGitG(t, root, "commit-tree", tree, "-p", origHash, "-m", "detached commit object")
	// Move the extra ref to the new commit.
	mustGitG(t, root, "update-ref", "refs/heads/extra", newCommit)

	outcome := gitGuardCompareAndRestore(root, snap, false, nil)

	if outcome.TerminalReason != gitGuardReasonRefMutation {
		t.Errorf("expected terminal_reason=%q, got %q", gitGuardReasonRefMutation, outcome.TerminalReason)
	}

	// refs/heads/extra should be restored to origHash.
	restoredRef := mustGitG(t, root, "rev-parse", "refs/heads/extra")
	if restoredRef != origHash {
		t.Errorf("refs/heads/extra not restored: want %s, got %s", origHash, restoredRef)
	}
}

// ---------------------------------------------------------------------------
// gitGuardCompareAndRestore — review-fix: commit detected + staged changes restored
// ---------------------------------------------------------------------------

func TestGitGuardCompareAndRestore_ReviewFixHistoryMutation_IndexRestored(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)

	// Stage execute-phase changes (what would be in the index before review-fix runs).
	if err := os.WriteFile(filepath.Join(root, "execute_change.txt"), []byte("execute"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitG(t, root, "add", "execute_change.txt")

	// Snapshot is taken with staged execute changes in the index.
	snap, err := gitGuardTakeSnapshot(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	// Simulate fixer agent committing both the execute change and its own change.
	if err := os.WriteFile(filepath.Join(root, "fixer.txt"), []byte("fixer"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitG(t, root, "add", "fixer.txt")
	mustGitG(t, root, "commit", "-m", "fixer commit (includes staged execute change)")

	outcome := gitGuardCompareAndRestore(root, snap, true, nil)

	if outcome.TerminalReason != gitGuardReasonHistoryMutation {
		t.Errorf("expected terminal_reason=%q, got %q", gitGuardReasonHistoryMutation, outcome.TerminalReason)
	}

	// HEAD must be restored to original.
	currentHead := mustGitG(t, root, "rev-parse", "HEAD")
	if currentHead != snap.HeadHash {
		t.Errorf("HEAD not restored: want %s, got %s", snap.HeadHash, currentHead)
	}

	// The execute-phase staged change should be back in the index.
	lsOut, err := exec.Command("git", "-C", root, "ls-files", "--cached", "execute_change.txt").Output()
	if err != nil {
		t.Fatalf("ls-files: %v", err)
	}
	if strings.TrimSpace(string(lsOut)) == "" {
		t.Error("execute_change.txt should be staged after review-fix restore (index restored to snapshot)")
	}
}

// ---------------------------------------------------------------------------
// gitGuardPrechecks — snapshot returned on success
// ---------------------------------------------------------------------------

func TestGitGuardPrechecks_SnapshotPopulatedOnSuccess(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)

	ok, _, snap, err := gitGuardPrechecks(root, false)
	if err != nil || !ok {
		t.Fatalf("expected ok=true, err=nil; got ok=%v err=%v", ok, err)
	}

	if snap.HeadSymRef == "" {
		t.Error("snapshot HeadSymRef is empty")
	}
	if snap.HeadHash == "" {
		t.Error("snapshot HeadHash is empty")
	}
	if snap.IndexTreeHash == "" {
		t.Error("snapshot IndexTreeHash is empty")
	}
	if snap.Refs == nil {
		t.Error("snapshot Refs is nil")
	}
}

// ---------------------------------------------------------------------------
// gitGuardPrechecks — loose ref lock file
// ---------------------------------------------------------------------------

func TestGitGuardPrechecks_LooseRefLock(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)
	gitDir := mustGitG(t, root, "rev-parse", "--absolute-git-dir")

	refsHeadsDir := filepath.Join(gitDir, "refs", "heads")
	if err := os.MkdirAll(refsHeadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(refsHeadsDir, "main.lock"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	ok, reason, _, err := gitGuardPrechecks(root, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when loose ref lock file exists")
	}
	if reason != gitGuardReasonInconclusive {
		t.Errorf("expected %q, got %q", gitGuardReasonInconclusive, reason)
	}
}

// ---------------------------------------------------------------------------
// gitGuardCompareAndRestore — agent errors after mutation
// ---------------------------------------------------------------------------

func TestGitGuardAgentError(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)

	snap, err := gitGuardTakeSnapshot(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	// Simulate agent making a commit before the transport errors.
	if err := os.WriteFile(filepath.Join(root, "agent.txt"), []byte("work"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitG(t, root, "add", "agent.txt")
	mustGitG(t, root, "commit", "-m", "agent commit")

	transportErr := fmt.Errorf("agent transport failed")
	outcome := gitGuardCompareAndRestore(root, snap, false, transportErr)

	// Guard still detects the history mutation despite the transport error.
	if outcome.TerminalReason != gitGuardReasonHistoryMutation {
		t.Errorf("expected terminal_reason=%q, got %q", gitGuardReasonHistoryMutation, outcome.TerminalReason)
	}
	if !strings.Contains(outcome.Detail, "agent transport failed") {
		t.Errorf("expected transport error in detail, got %q", outcome.Detail)
	}

	// HEAD is restored to original even though the transport errored.
	currentHead := mustGitG(t, root, "rev-parse", "HEAD")
	if currentHead != snap.HeadHash {
		t.Errorf("HEAD not restored despite transport error: want %s, got %s", snap.HeadHash, currentHead)
	}

	// Agent's changes remain as unstaged working-tree modifications.
	statusOut := mustGitG(t, root, "status", "--porcelain=v1")
	if !strings.Contains(statusOut, "agent.txt") {
		t.Errorf("expected agent.txt in status after restore, got: %q", statusOut)
	}
}

// ---------------------------------------------------------------------------
// gitGuardCompareAndRestore — review-fix baseline (pre-existing dirty not flagged)
// ---------------------------------------------------------------------------

func TestGitGuardReviewFixBaseline(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)

	// Simulate execute-phase dirty state: staged change + unstaged change.
	if err := os.WriteFile(filepath.Join(root, "staged.txt"), []byte("staged"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitG(t, root, "add", "staged.txt")
	if err := os.WriteFile(filepath.Join(root, "unstaged.txt"), []byte("unstaged"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Snapshot is the review-fix baseline: it includes the pre-existing dirty state.
	snap, err := gitGuardTakeSnapshot(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	// Fixer makes no further changes beyond the baseline.
	outcome := gitGuardCompareAndRestore(root, snap, true, nil)

	if outcome.TerminalReason != "" {
		t.Errorf("pre-existing dirty state should not be flagged as a mutation; got terminal_reason=%q", outcome.TerminalReason)
	}
}

// ---------------------------------------------------------------------------
// gitGuardCompareAndRestore — inconclusive suppresses restoration of restorable mutations
// ---------------------------------------------------------------------------

func TestGitGuardInconclusiveSuppressesRestore(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)

	// Create an extra ref that the agent will move (normally a restorable ref mutation).
	origHash := mustGitG(t, root, "rev-parse", "HEAD")
	mustGitG(t, root, "update-ref", "refs/heads/extra", origHash)

	snap, err := gitGuardTakeSnapshot(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	// Agent: creates a stash (inconclusive trigger) AND moves refs/heads/extra (restorable).
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("modified"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitG(t, root, "stash", "push", "-m", "agent stash")

	tree := mustGitG(t, root, "write-tree")
	newCommit := mustGitG(t, root, "commit-tree", tree, "-p", origHash, "-m", "detached commit")
	mustGitG(t, root, "update-ref", "refs/heads/extra", newCommit)

	outcome := gitGuardCompareAndRestore(root, snap, false, nil)

	// Inconclusive (stash) dominates.
	if outcome.TerminalReason != gitGuardReasonInconclusive {
		t.Errorf("expected terminal_reason=%q, got %q", gitGuardReasonInconclusive, outcome.TerminalReason)
	}

	// refs/heads/extra must NOT be restored — inconclusive suppresses all restoration.
	currentExtra := mustGitG(t, root, "rev-parse", "refs/heads/extra")
	if currentExtra != newCommit {
		t.Errorf("inconclusive should suppress ref restoration: extra should remain at %s, got %s", newCommit, currentExtra)
	}
}

// ---------------------------------------------------------------------------
// gitGuardPrechecks — linked worktree: agent commit detected and restored
// ---------------------------------------------------------------------------

func TestGitGuardPrechecks_LinkedWorktree_AgentCommitDetected(t *testing.T) {
	skipIfNoGit(t)
	primaryRoot := initTestRepo(t)

	// A branch can only be checked out in one worktree at a time; create a
	// fresh branch for the linked worktree.
	mustGitG(t, primaryRoot, "branch", "wt-branch")
	wtDir := filepath.Join(t.TempDir(), "wt")
	mustGitG(t, primaryRoot, "worktree", "add", wtDir, "wt-branch")
	t.Cleanup(func() {
		_ = exec.Command("git", "-C", primaryRoot, "worktree", "remove", "--force", wtDir).Run()
	})

	// Prechecks inside the linked worktree must succeed — multi-worktree presence
	// alone must not return executor_git_guard_inconclusive.
	ok, reason, snap, err := gitGuardPrechecks(wtDir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true in linked worktree, got reason=%q", reason)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}

	// Simulate agent making a commit inside the linked worktree.
	if err := os.WriteFile(filepath.Join(wtDir, "agent.txt"), []byte("agent work"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitG(t, wtDir, "add", "agent.txt")
	mustGitG(t, wtDir, "commit", "-m", "agent commit in linked worktree")

	outcome := gitGuardCompareAndRestore(wtDir, snap, false, nil)

	if outcome.TerminalReason != gitGuardReasonHistoryMutation {
		t.Errorf("expected terminal_reason=%q, got %q (must not be executor_git_guard_inconclusive)", gitGuardReasonHistoryMutation, outcome.TerminalReason)
	}

	// HEAD must be restored to the snapshot hash.
	currentHead := mustGitG(t, wtDir, "rev-parse", "HEAD")
	if currentHead != snap.HeadHash {
		t.Errorf("HEAD not restored: want %s, got %s", snap.HeadHash, currentHead)
	}

	// Agent's changes must be present as unstaged working-tree modifications.
	statusOut := mustGitG(t, wtDir, "status", "--porcelain=v1")
	if !strings.Contains(statusOut, "agent.txt") {
		t.Errorf("expected agent.txt in status after restore, got: %q", statusOut)
	}
}

// ---------------------------------------------------------------------------
// gitGuardPrechecks — stale lock detection uses git common dir
// ---------------------------------------------------------------------------

func TestGitGuardPrechecks_PackedRefsLock_PrimaryWorktree(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)

	// In a primary worktree --git-common-dir returns a relative path (e.g. ".git").
	// The implementation resolves it via filepath.Join(root, commonDir).
	commonDir := mustGitG(t, root, "rev-parse", "--git-common-dir")
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(root, commonDir)
	}
	if err := os.WriteFile(filepath.Join(commonDir, "packed-refs.lock"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	ok, reason, _, err := gitGuardPrechecks(root, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when packed-refs.lock exists in common dir")
	}
	if reason != gitGuardReasonInconclusive {
		t.Errorf("expected %q, got %q", gitGuardReasonInconclusive, reason)
	}
}

func TestGitGuardPrechecks_PackedRefsLock_LinkedWorktree(t *testing.T) {
	skipIfNoGit(t)
	primaryRoot := initTestRepo(t)

	mustGitG(t, primaryRoot, "branch", "wt-branch")
	wtDir := filepath.Join(t.TempDir(), "wt")
	mustGitG(t, primaryRoot, "worktree", "add", wtDir, "wt-branch")
	t.Cleanup(func() {
		_ = exec.Command("git", "-C", primaryRoot, "worktree", "remove", "--force", wtDir).Run()
	})

	// In a linked worktree --git-common-dir returns the absolute path to the
	// primary .git dir (the shared ref store).
	commonDir := mustGitG(t, wtDir, "rev-parse", "--git-common-dir")
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(wtDir, commonDir)
	}
	if err := os.WriteFile(filepath.Join(commonDir, "packed-refs.lock"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	ok, reason, _, err := gitGuardPrechecks(wtDir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when packed-refs.lock exists in common dir")
	}
	if reason != gitGuardReasonInconclusive {
		t.Errorf("expected %q, got %q", gitGuardReasonInconclusive, reason)
	}
}

func TestGitGuardPrechecks_LooseRefLock_LinkedWorktree(t *testing.T) {
	skipIfNoGit(t)
	primaryRoot := initTestRepo(t)

	mustGitG(t, primaryRoot, "branch", "wt-branch")
	wtDir := filepath.Join(t.TempDir(), "wt")
	mustGitG(t, primaryRoot, "worktree", "add", wtDir, "wt-branch")
	t.Cleanup(func() {
		_ = exec.Command("git", "-C", primaryRoot, "worktree", "remove", "--force", wtDir).Run()
	})

	// In a linked worktree --git-common-dir returns the absolute path to the
	// primary .git dir. The loose-ref lock walk must use this common dir so that
	// shared-ref-store locks are detectable from inside the linked worktree.
	commonDir := mustGitG(t, wtDir, "rev-parse", "--git-common-dir")
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(wtDir, commonDir)
	}
	refsHeadsDir := filepath.Join(commonDir, "refs", "heads")
	if err := os.MkdirAll(refsHeadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(refsHeadsDir, "wt-branch.lock"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	ok, reason, _, err := gitGuardPrechecks(wtDir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when loose ref lock file exists in common dir refs/")
	}
	if reason != gitGuardReasonInconclusive {
		t.Errorf("expected %q, got %q", gitGuardReasonInconclusive, reason)
	}
}
