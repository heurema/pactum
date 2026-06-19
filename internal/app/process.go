package app

import (
	"fmt"
	"io"
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
	// CompletedDespiteTimeout records the completed-with-warning finalize: the
	// idle watchdog fired (TimedOut stays true), but the agent's output carried
	// a successful terminal marker, so the attempt counts as a success.
	CompletedDespiteTimeout bool `json:"completed_despite_timeout,omitempty"`
	// WallClockTimeout records an attempt killed by the absolute wall-clock cap
	// rather than the idle watchdog.
	WallClockTimeout bool   `json:"wall_clock_timeout,omitempty"`
	Stdout           string `json:"stdout"`
	Stderr           string `json:"stderr"`
}

func processResultFromRunResult(result agents.RunResult) processResult {
	return processResult{
		StartedAt:               result.StartedAt,
		FinishedAt:              result.FinishedAt,
		DurationMillis:          result.DurationMillis,
		ExitCode:                result.ExitCode,
		TimedOut:                result.TimedOut,
		CompletedDespiteTimeout: result.CompletedDespiteTimeout,
		WallClockTimeout:        result.WallClockTimeout,
		Stdout:                  result.StdoutPath,
		Stderr:                  result.StderrPath,
	}
}

// writeCompletedDespiteTimeoutWarning surfaces the completed-with-warning
// finalize in the human result output, right under the timed-out line.
func writeCompletedDespiteTimeoutWarning(stdout io.Writer, result processResult) {
	if !result.CompletedDespiteTimeout {
		return
	}
	fmt.Fprintln(stdout, "  warning: idle timeout fired after the agent completed; treated as completed")
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
