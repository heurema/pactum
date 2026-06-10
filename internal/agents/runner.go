package agents

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func RunSubprocess(request RunRequest) (RunResult, error) {
	return runSubprocessWithRunner(request, osProcessRunner{})
}

// lockedWriter serializes concurrent writes to an underlying writer. os/exec
// copies a command's stdout and stderr on separate goroutines, so a live writer
// shared by both streams must be guarded.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

type activityWriter struct {
	w        io.Writer
	activity chan<- struct{}
}

func (w activityWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		select {
		case w.activity <- struct{}{}:
		default:
		}
	}
	return w.w.Write(p)
}

type processRunner interface {
	Run(ctx context.Context, spec processSpec) error
}

type processSpec struct {
	Command string
	Args    []string
	Dir     string
	Env     []string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
}

func runSubprocessWithRunner(request RunRequest, runner processRunner) (RunResult, error) {
	if runner == nil {
		runner = osProcessRunner{}
	}
	if err := validateRunRequest(request); err != nil {
		return RunResult{}, err
	}
	executor, err := newPromptExecutor(request.Agent)
	if err != nil {
		return RunResult{}, err
	}

	command := executor.command(request.PromptRepoPath)
	promptPath := filepath.Join(request.RepoRoot, filepath.FromSlash(request.PromptRepoPath))
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		return RunResult{}, err
	}

	layout, err := attemptArtifactLayout(request)
	if err != nil {
		return RunResult{}, err
	}
	stdoutPath := layout.stdoutPath
	stderrPath := layout.stderrPath

	stdout, stderr, err := createAttemptLogs(layout)
	if err != nil {
		return RunResult{}, err
	}
	defer stdout.Close()
	defer stderr.Close()

	// Capture to the per-attempt log files always; when a live writer is set,
	// also tee both streams to it (the operator's stderr) so the run is visible
	// as it happens. The agent's stdout is teed to the same live writer, keeping
	// the caller's own stdout free for the clean result channel.
	var stdoutWriter io.Writer = stdout
	var stderrWriter io.Writer = stderr
	if request.LiveOutput != nil {
		// os/exec copies stdout and stderr on separate goroutines, so the shared
		// live writer must be synchronized to avoid concurrent writes (data race).
		live := &lockedWriter{w: request.LiveOutput}
		stdoutWriter = io.MultiWriter(stdout, live)
		stderrWriter = io.MultiWriter(stderr, live)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var idleTimedOut atomic.Bool
	stopIdleTimeout := func() {}
	if request.Timeout > 0 {
		activity := make(chan struct{}, 1)
		stdoutWriter = activityWriter{w: stdoutWriter, activity: activity}
		stderrWriter = activityWriter{w: stderrWriter, activity: activity}
		stopIdleTimeout = startIdleTimeout(request.Timeout, activity, cancel, &idleTimedOut)
	}
	defer cancel()

	started := time.Now().UTC()
	err = runner.Run(ctx, processSpec{
		Command: command.Command,
		Args:    cloneArgs(command.Args),
		Dir:     request.RepoRoot,
		Env:     executor.env(os.Environ()),
		Stdin:   bytes.NewReader(prompt),
		Stdout:  stdoutWriter,
		Stderr:  stderrWriter,
	})
	finished := time.Now().UTC()
	stopIdleTimeout()

	exitCode := 0
	timedOut := idleTimedOut.Load()
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = -1
			fmt.Fprintln(stderr, err.Error())
		}
	}
	completedDespiteTimeout := false
	if timedOut {
		exitCode, completedDespiteTimeout = finalizeTimedOutAttempt(request.Agent, stdoutPath, false, stderr, request.LiveOutput)
	}
	usage := captureUsageFromArtifacts(request.Agent, stdoutPath, stderrPath)
	writeUsageWarning(usage, stderr, request.LiveOutput)

	return RunResult{
		ExitCode:                exitCode,
		StartedAt:               started.Format(time.RFC3339Nano),
		FinishedAt:              finished.Format(time.RFC3339Nano),
		DurationMillis:          finished.Sub(started).Milliseconds(),
		TimedOut:                timedOut,
		CompletedDespiteTimeout: completedDespiteTimeout,
		StdoutPath:              layout.stdoutArtifact,
		StderrPath:              layout.stderrArtifact,
		Usage:                   usage,
	}, err
}

const completedDespiteTimeoutNotice = "idle timeout fired after the agent completed; treating as completed with warning"

// finalizeTimedOutAttempt resolves an idle-killed attempt against the agent's
// completion signal: when the captured stdout carries a successful terminal
// marker (or the caller already observed completion, e.g. a recorded ACP
// prompt response), the attempt is finalized as completed-with-warning — exit
// code 0, the warning appended to the attempt stderr and the live writer.
// TimedOut stays true on the result for the record. Without the marker the
// timed-out failure stands (exit -1).
// An empty stdoutPath skips the captured-output detection: over ACP the
// attempt log is free-streamed agent text where the CLI terminal markers
// cannot legitimately appear (an agent merely QUOTING one must not convert a
// stalled turn into success), so the recorded prompt response is the
// protocol's only completion signal.
func finalizeTimedOutAttempt(agent AgentDescriptor, stdoutPath string, alreadyCompleted bool, stderr io.Writer, live io.Writer) (exitCode int, completed bool) {
	completed = alreadyCompleted
	if !completed && stdoutPath != "" {
		stdout, err := os.ReadFile(stdoutPath)
		completed = err == nil && agentRunCompleted(agent, stdout)
	}
	if !completed {
		return -1, false
	}
	line := completedDespiteTimeoutNotice + "\n"
	_, _ = io.WriteString(stderr, line)
	if live != nil {
		_, _ = io.WriteString(live, line)
	}
	return 0, true
}

func captureUsageFromArtifacts(agent AgentDescriptor, stdoutPath string, stderrPath string) TokenUsage {
	if !structuredUsageEnabled(agent) {
		return TokenUsage{}
	}
	stdout, err := os.ReadFile(stdoutPath)
	if err != nil {
		return TokenUsage{CaptureWarning: fmt.Sprintf("usage capture failed: read stdout: %v", err)}
	}
	stderr, err := os.ReadFile(stderrPath)
	if err != nil {
		return TokenUsage{CaptureWarning: fmt.Sprintf("usage capture failed: read stderr: %v", err)}
	}
	return parseAgentUsage(agent, stdout, stderr)
}

func writeUsageWarning(usage TokenUsage, stderr io.Writer, live io.Writer) {
	if strings.TrimSpace(usage.CaptureWarning) == "" {
		return
	}
	line := "usage capture warning: " + usage.CaptureWarning + "\n"
	_, _ = io.WriteString(stderr, line)
	if live != nil {
		_, _ = io.WriteString(live, line)
	}
}

func startIdleTimeout(timeout time.Duration, activity <-chan struct{}, cancel context.CancelFunc, timedOut *atomic.Bool) func() {
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)

		timer := time.NewTimer(timeout)
		defer timer.Stop()

		for {
			select {
			case <-timer.C:
				timedOut.Store(true)
				cancel()
				return
			case <-activity:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(timeout)
			case <-done:
				return
			}
		}
	}()

	return func() {
		close(done)
		<-stopped
	}
}
