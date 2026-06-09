package agents

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/heurema/pactum/internal/artifacts"
)

// validateRunRequest checks the fields both transports require before any
// filesystem work: the repo root, the run/attempt identifiers, and the prompt
// path. It returns the first failure using the shared error message so the CLI
// and ACP transports reject malformed requests identically.
func validateRunRequest(request RunRequest) error {
	if strings.TrimSpace(request.RepoRoot) == "" {
		return errors.New("repo root is required")
	}
	if strings.TrimSpace(request.RunID) == "" {
		return errors.New("run id is required")
	}
	if strings.TrimSpace(request.AttemptID) == "" {
		return errors.New("attempt id is required")
	}
	if strings.TrimSpace(request.PromptRepoPath) == "" {
		return errors.New("prompt path is required")
	}
	return nil
}

// attemptLayout is the on-disk and recorded-artifact path layout for a single
// attempt. Both transports derive their log paths from one layout so the
// attempt directory and the RunResult artifact paths can never drift.
type attemptLayout struct {
	// stdoutPath and stderrPath are the absolute paths of the attempt log files.
	stdoutPath string
	stderrPath string
	// stdoutArtifact and stderrArtifact are the slash-relative artifact paths
	// recorded in RunResult.
	stdoutArtifact string
	stderrArtifact string
}

// attemptArtifactLayout computes the attempt's path layout and creates the
// attempt directory. The artifact dir defaults to "execute/attempts" when the
// request leaves it empty; the absolute attempt directory lives under the
// .heurema runs tree, and the slash-relative artifact paths recorded in
// RunResult are derived from the same artifact dir.
func attemptArtifactLayout(request RunRequest) (attemptLayout, error) {
	artifactDir := strings.Trim(strings.TrimSpace(request.ArtifactDir), "/")
	if artifactDir == "" {
		artifactDir = filepath.ToSlash(filepath.Join("execute", "attempts"))
	}
	attemptDir := filepath.Join(request.RepoRoot, artifacts.WorkspaceRel, "runs", request.RunID, filepath.FromSlash(artifactDir), request.AttemptID)
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return attemptLayout{}, err
	}
	return attemptLayout{
		stdoutPath:     filepath.Join(attemptDir, "stdout.log"),
		stderrPath:     filepath.Join(attemptDir, "stderr.log"),
		stdoutArtifact: filepath.ToSlash(filepath.Join(artifactDir, request.AttemptID, "stdout.log")),
		stderrArtifact: filepath.ToSlash(filepath.Join(artifactDir, request.AttemptID, "stderr.log")),
	}, nil
}

// createAttemptLogs creates the attempt's stdout.log and stderr.log files. The
// caller owns the returned handles and is responsible for closing them (the
// usual deferred Close). If the stderr file cannot be created, the already-open
// stdout file is closed so the caller is never left holding a dangling handle.
func createAttemptLogs(layout attemptLayout) (stdout *os.File, stderr *os.File, err error) {
	stdout, err = os.Create(layout.stdoutPath)
	if err != nil {
		return nil, nil, err
	}
	stderr, err = os.Create(layout.stderrPath)
	if err != nil {
		stdout.Close()
		return nil, nil, err
	}
	return stdout, stderr, nil
}
