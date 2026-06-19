//go:build unix

package agents

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// TestACPTransportWallClockCapReapsChildProcess verifies that the process-group
// kill triggered by the wall-clock cap reaps processes spawned by the adapter,
// not only the adapter itself. The spawn_child helper launches a sleeping
// grandchild process and writes its PID to a temp file; after the cap fires and
// killProcessGroup kills the whole group, the grandchild must no longer exist.
func TestACPTransportWallClockCapReapsChildProcess(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("PACTUM_CLAUDE_ACP_COMMAND", os.Args[0])
	t.Setenv("PACTUM_ACP_WALLCLOCK_HELPER", "spawn_child")
	t.Setenv("PACTUM_CHILD_PID_FILE", pidFile)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "prompt.md"), []byte("test prompt"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ACPTransport{}.Run(RunRequest{
		RepoRoot:       root,
		RunID:          "run_reap_001",
		AttemptID:      "attempt_001",
		Agent:          AgentDescriptor{Name: BuiltinClaude},
		PromptRepoPath: "prompt.md",
		ArtifactDir:    "test/attempts",
		// Generous enough for the helper to spawn the grandchild and write
		// the PID file, short enough to keep the test fast.
		WallClockCap: 500 * time.Millisecond,
		Timeout:      25 * time.Minute,
	})

	if err == nil {
		t.Fatal("expected an error when the wall-clock cap fires")
	}
	if !result.WallClockTimeout {
		t.Errorf("WallClockTimeout must be true, got: %+v", result)
	}

	// Allow a brief moment for the OS to fully reap the process group.
	time.Sleep(50 * time.Millisecond)

	pidBytes, readErr := os.ReadFile(pidFile)
	if readErr != nil {
		// The helper may have been killed before it wrote the PID file (e.g. on
		// a very fast machine where 500ms is not enough). Skip rather than fail.
		t.Skipf("grandchild PID file not written (helper killed before it could write): %v", readErr)
	}

	childPID, parseErr := strconv.Atoi(string(pidBytes))
	if parseErr != nil || childPID <= 0 {
		t.Fatalf("could not parse grandchild PID from %q: %v", string(pidBytes), parseErr)
	}

	// signal(pid, 0) checks existence without sending a real signal.
	if err := syscall.Kill(childPID, 0); err == nil {
		t.Errorf("grandchild process %d survived killProcessGroup; process-group kill did not reap the whole tree", childPID)
	}
}
