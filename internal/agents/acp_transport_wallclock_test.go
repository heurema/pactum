package agents

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// TestMain acts as the control point for helper-process tests. When the
// wall-clock transport test launches this test binary as a fake ACP adapter,
// PACTUM_ACP_WALLCLOCK_HELPER selects the helper mode instead of running tests:
//
//   - "1":          hang forever (non-responsive ACP adapter)
//   - "exit_fast":  exit immediately without speaking ACP
//   - "spawn_child": spawn a sleeping grandchild, write its PID to
//     PACTUM_CHILD_PID_FILE, then hang — lets the reap test verify that
//     killProcessGroup kills the whole process group
func TestMain(m *testing.M) {
	switch os.Getenv("PACTUM_ACP_WALLCLOCK_HELPER") {
	case "1":
		// Use time.Sleep rather than select{}: the runtime deadlock detector
		// fires on select{} (no wakeup path), causing the subprocess to exit
		// immediately — exactly the opposite of what we want.
		time.Sleep(time.Hour)
		return
	case "exit_fast":
		// Exit immediately without speaking ACP — simulates an adapter that
		// exits cleanly well before the wall-clock cap fires.
		return
	case "spawn_child":
		// Launch a grandchild process that also sleeps forever. Write its PID
		// to PACTUM_CHILD_PID_FILE so the reap test can verify it was killed
		// when killProcessGroup fires on the whole process group.
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), "PACTUM_ACP_WALLCLOCK_HELPER=1")
		if err := child.Start(); err == nil {
			if pidFile := os.Getenv("PACTUM_CHILD_PID_FILE"); pidFile != "" {
				_ = os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", child.Process.Pid)), 0o644)
			}
		}
		time.Sleep(time.Hour)
		return
	}
	os.Exit(m.Run())
}

func TestStartWallClockTimeoutFiresAndSetsFlag(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var flag atomic.Bool
	stop := startWallClockTimeout(20*time.Millisecond, cancel, &flag)
	defer stop()

	deadline := time.NewTimer(500 * time.Millisecond)
	defer deadline.Stop()
	select {
	case <-ctx.Done():
	case <-deadline.C:
		t.Fatal("wall-clock timer did not fire within 500ms")
	}
	if !flag.Load() {
		t.Fatal("flag must be true after the wall-clock timer fires")
	}
}

func TestStartWallClockTimeoutStopPreventsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var flag atomic.Bool
	stop := startWallClockTimeout(100*time.Millisecond, cancel, &flag)
	stop()

	time.Sleep(200 * time.Millisecond)
	if flag.Load() {
		t.Fatal("stopped timer must not set the flag")
	}
	select {
	case <-ctx.Done():
		t.Fatal("stopped timer must not cancel the context")
	default:
	}
}

// TestACPTransportWallClockCapKillsHangingProcess verifies that the wall-clock
// cap kills a subprocess that never responds and produces WallClockTimeout=true
// without setting TimedOut (idle).
//
// PACTUM_CLAUDE_ACP_COMMAND is overridden to this test binary; TestMain's
// PACTUM_ACP_WALLCLOCK_HELPER branch causes the subprocess to hang rather than
// run any tests — simulating a real but non-responsive ACP adapter.
func TestACPTransportWallClockCapKillsHangingProcess(t *testing.T) {
	t.Setenv("PACTUM_CLAUDE_ACP_COMMAND", os.Args[0])
	t.Setenv("PACTUM_ACP_WALLCLOCK_HELPER", "1")

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "prompt.md"), []byte("test prompt"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ACPTransport{}.Run(RunRequest{
		RepoRoot:       root,
		RunID:          "run_wct_001",
		AttemptID:      "attempt_001",
		Agent:          AgentDescriptor{Name: BuiltinClaude},
		PromptRepoPath: "prompt.md",
		ArtifactDir:    "test/attempts",
		WallClockCap:   200 * time.Millisecond,
		Timeout:        25 * time.Minute,
	})

	if err == nil {
		t.Fatal("expected an error when the wall-clock cap fires")
	}
	if !result.WallClockTimeout {
		t.Errorf("WallClockTimeout must be true when killed by the cap, got: %+v", result)
	}
	if result.TimedOut {
		t.Errorf("TimedOut (idle) must not be set when only the wall-clock cap fired, got: %+v", result)
	}
	if result.ExitCode != -1 {
		t.Errorf("ExitCode must be -1 for a wall-clock-killed attempt, got %d", result.ExitCode)
	}
}

// TestWallClockCapFiresWhileIdleTimerKeptAlive verifies that the wall-clock cap
// fires regardless of idle-timer activity: even when a goroutine continuously
// resets the idle window by sending on the activity channel, the hard ceiling
// still fires on schedule and is the only timeout flag set.
func TestWallClockCapFiresWhileIdleTimerKeptAlive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	activity := make(chan struct{}, 1)
	var idleTimedOut atomic.Bool
	var wallClockTimedOut atomic.Bool

	// Idle window much larger than the wall-clock cap; 20ms activity ticks keep
	// the idle timer from firing while the 100ms cap fires independently.
	stopIdle := startIdleTimeout(500*time.Millisecond, activity, cancel, &idleTimedOut)
	defer stopIdle()
	stopWall := startWallClockTimeout(100*time.Millisecond, cancel, &wallClockTimedOut)
	defer stopWall()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(20 * time.Millisecond):
				select {
				case activity <- struct{}{}:
				default:
				}
			}
		}
	}()

	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("context was not cancelled within 2s")
	}
	if idleTimedOut.Load() {
		t.Error("idle timer must not have fired while activity kept it alive")
	}
	if !wallClockTimedOut.Load() {
		t.Error("wall-clock cap must have fired")
	}
}

// TestACPTransportFastExitNoWallClockFlag verifies that an adapter that exits
// before the wall-clock cap fires does not produce a WallClockTimeout result.
// The ACP session fails (the helper exits without speaking ACP), but
// WallClockTimeout and TimedOut must both be false.
func TestACPTransportFastExitNoWallClockFlag(t *testing.T) {
	t.Setenv("PACTUM_CLAUDE_ACP_COMMAND", os.Args[0])
	t.Setenv("PACTUM_ACP_WALLCLOCK_HELPER", "exit_fast")

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "prompt.md"), []byte("test prompt"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, _ := ACPTransport{}.Run(RunRequest{
		RepoRoot:       root,
		RunID:          "run_fast_001",
		AttemptID:      "attempt_001",
		Agent:          AgentDescriptor{Name: BuiltinClaude},
		PromptRepoPath: "prompt.md",
		ArtifactDir:    "test/attempts",
		WallClockCap:   2 * time.Second,
		Timeout:        25 * time.Minute,
	})

	if result.WallClockTimeout {
		t.Errorf("WallClockTimeout must be false when adapter exits before the cap: %+v", result)
	}
	if result.TimedOut {
		t.Errorf("TimedOut must be false when adapter exits before both timeouts: %+v", result)
	}
}

// TestACPTransportIdleTimeoutDistinctFromWallClockCap verifies that when only
// the idle timeout fires (no wall-clock cap), WallClockTimeout stays false while
// TimedOut is true — the two outcomes are kept strictly separate.
func TestACPTransportIdleTimeoutDistinctFromWallClockCap(t *testing.T) {
	t.Setenv("PACTUM_CLAUDE_ACP_COMMAND", os.Args[0])
	t.Setenv("PACTUM_ACP_WALLCLOCK_HELPER", "1")

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "prompt.md"), []byte("test prompt"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ACPTransport{}.Run(RunRequest{
		RepoRoot:       root,
		RunID:          "run_idle_001",
		AttemptID:      "attempt_001",
		Agent:          AgentDescriptor{Name: BuiltinClaude},
		PromptRepoPath: "prompt.md",
		ArtifactDir:    "test/attempts",
		Timeout:        100 * time.Millisecond,
	})

	if err == nil {
		t.Fatal("expected an error when the idle timeout fires")
	}
	if result.WallClockTimeout {
		t.Errorf("WallClockTimeout must be false when only the idle timeout fired, got: %+v", result)
	}
	if !result.TimedOut {
		t.Errorf("TimedOut must be true when the idle timeout fired, got: %+v", result)
	}
}
