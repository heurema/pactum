package agents

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
)

func RunSubprocess(request RunRequest) (RunResult, error) {
	if strings.TrimSpace(request.RepoRoot) == "" {
		return RunResult{}, errors.New("repo root is required")
	}
	if strings.TrimSpace(request.RunID) == "" {
		return RunResult{}, errors.New("run id is required")
	}
	if strings.TrimSpace(request.AttemptID) == "" {
		return RunResult{}, errors.New("attempt id is required")
	}
	if strings.TrimSpace(request.Agent.Command) == "" {
		return RunResult{}, errors.New("agent command is required")
	}
	if request.Agent.Input != InputPromptFile {
		return RunResult{}, fmt.Errorf("unsupported agent input mode: %s", request.Agent.Input)
	}
	if strings.TrimSpace(request.PromptRepoPath) == "" {
		return RunResult{}, errors.New("prompt path is required")
	}

	args := append([]string{}, request.Agent.Args...)
	promptPath := filepath.Join(request.RepoRoot, filepath.FromSlash(request.PromptRepoPath))
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		return RunResult{}, err
	}

	attemptDir := filepath.Join(request.RepoRoot, artifacts.WorkspaceRel, "runs", request.RunID, "execute", "attempts", request.AttemptID)
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return RunResult{}, err
	}

	stdoutArtifact := filepath.ToSlash(filepath.Join("execute", "attempts", request.AttemptID, "stdout.log"))
	stderrArtifact := filepath.ToSlash(filepath.Join("execute", "attempts", request.AttemptID, "stderr.log"))
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

	ctx := context.Background()
	var cancel context.CancelFunc
	if request.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, request.Timeout)
		defer cancel()
	}

	started := time.Now().UTC()
	command := exec.CommandContext(ctx, request.Agent.Command, args...)
	command.Dir = request.RepoRoot
	command.Env = os.Environ()
	command.Stdout = stdout
	command.Stderr = stderr
	command.Stdin = bytes.NewReader(prompt)

	err = command.Run()
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
	if timedOut && exitCode == 0 {
		exitCode = -1
	}

	return RunResult{
		Command:        request.Agent.Command,
		Args:           args,
		ExitCode:       exitCode,
		StartedAt:      started.Format(time.RFC3339Nano),
		FinishedAt:     finished.Format(time.RFC3339Nano),
		DurationMillis: finished.Sub(started).Milliseconds(),
		TimedOut:       timedOut,
		StdoutPath:     stdoutArtifact,
		StderrPath:     stderrArtifact,
	}, err
}
