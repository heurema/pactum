package app

import (
	"fmt"
	"path/filepath"

	"github.com/heurema/pactum/internal/agents"
)

// processResult is the captured outcome of a subprocess run (executor, reviewer,
// or validation command). It is embedded in each *ResultDocument so the wire
// shape stays flat while the fields and their mapping live in one place.
type processResult struct {
	StartedAt      string `json:"started_at"`
	FinishedAt     string `json:"finished_at"`
	DurationMillis int64  `json:"duration_ms"`
	ExitCode       int    `json:"exit_code"`
	TimedOut       bool   `json:"timed_out"`
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
}

func processResultFromRunResult(result agents.RunResult) processResult {
	return processResult{
		StartedAt:      result.StartedAt,
		FinishedAt:     result.FinishedAt,
		DurationMillis: result.DurationMillis,
		ExitCode:       result.ExitCode,
		TimedOut:       result.TimedOut,
		Stdout:         result.StdoutPath,
		Stderr:         result.StderrPath,
	}
}

// attemptPathSet is the on-disk layout of a single executor or reviewer attempt.
type attemptPathSet struct {
	Dir         string
	RequestJSON string
	StdoutLog   string
	StderrLog   string
	ResultJSON  string
}

func newAttemptPaths(dir string) attemptPathSet {
	return attemptPathSet{
		Dir:         dir,
		RequestJSON: filepath.Join(dir, "request.json"),
		StdoutLog:   filepath.Join(dir, "stdout.log"),
		StderrLog:   filepath.Join(dir, "stderr.log"),
		ResultJSON:  filepath.Join(dir, "result.json"),
	}
}

// processExitError reports a non-zero subprocess exit. Kind names the process
// (e.g. "agent", "reviewer") for the message.
type processExitError struct {
	Kind     string
	ExitCode int
}

func (e processExitError) Error() string {
	return fmt.Sprintf("%s process exited with code %d", e.Kind, e.ExitCode)
}
