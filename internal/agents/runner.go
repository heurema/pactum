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
	"time"

	"github.com/heurema/pactum/internal/artifacts"
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
	if strings.TrimSpace(request.RepoRoot) == "" {
		return RunResult{}, errors.New("repo root is required")
	}
	if strings.TrimSpace(request.RunID) == "" {
		return RunResult{}, errors.New("run id is required")
	}
	if strings.TrimSpace(request.AttemptID) == "" {
		return RunResult{}, errors.New("attempt id is required")
	}
	executor, err := newPromptExecutor(request.Agent)
	if err != nil {
		return RunResult{}, err
	}
	if strings.TrimSpace(request.PromptRepoPath) == "" {
		return RunResult{}, errors.New("prompt path is required")
	}
	artifactDir := strings.Trim(strings.TrimSpace(request.ArtifactDir), "/")
	if artifactDir == "" {
		artifactDir = filepath.ToSlash(filepath.Join("execute", "attempts"))
	}

	command := executor.command(request.PromptRepoPath)
	promptPath := filepath.Join(request.RepoRoot, filepath.FromSlash(request.PromptRepoPath))
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		return RunResult{}, err
	}

	attemptDir := filepath.Join(request.RepoRoot, artifacts.WorkspaceRel, "runs", request.RunID, filepath.FromSlash(artifactDir), request.AttemptID)
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return RunResult{}, err
	}

	stdoutArtifact := filepath.ToSlash(filepath.Join(artifactDir, request.AttemptID, "stdout.log"))
	stderrArtifact := filepath.ToSlash(filepath.Join(artifactDir, request.AttemptID, "stderr.log"))
	stdoutPath := filepath.Join(attemptDir, "stdout.log")
	stderrPath := filepath.Join(attemptDir, "stderr.log")

	stdout, err := os.Create(stdoutPath)
	if err != nil {
		return RunResult{}, err
	}
	defer stdout.Close()

	stderr, err := os.Create(stderrPath)
	if err != nil {
		return RunResult{}, err
	}
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
	if request.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, request.Timeout)
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

	exitCode := 0
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = -1
			fmt.Fprintln(stderr, err.Error())
		}
	}
	if timedOut {
		exitCode = -1
	}

	return RunResult{
		Command:        command.Command,
		Args:           cloneArgs(command.Args),
		ExitCode:       exitCode,
		StartedAt:      started.Format(time.RFC3339Nano),
		FinishedAt:     finished.Format(time.RFC3339Nano),
		DurationMillis: finished.Sub(started).Milliseconds(),
		TimedOut:       timedOut,
		StdoutPath:     stdoutArtifact,
		StderrPath:     stderrArtifact,
	}, err
}
