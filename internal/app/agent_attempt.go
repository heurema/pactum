package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/ledger"
)

var agentAttemptLifecycleMu sync.Mutex

type agentAttemptLifecycle[Prepared any, Request any, Result any, Response any] struct {
	Stdout     io.Writer
	LiveOutput io.Writer
	JSONOutput bool

	Root        string
	EventsJSONL string
	RunID       string
	Stage       string

	AttemptsDir     string
	AttemptIDPrefix string
	LastResultJSON  string

	// AgentName is the registry name the stage was invoked under; it feeds the
	// usage record's agent_name. Agent is the underlying built-in's descriptor,
	// which attempt artifacts keep recording.
	AgentName string
	Agent     agents.AgentDescriptor
	// Model is the stage's resolved model pin (the same spec shown in the
	// Resolved block). It feeds the usage record and is passed through to the
	// RunRequest so the ACP transport can thread the pin to the adapter.
	Model          agents.ModelSpec
	PromptRepoPath string
	ArtifactDir    string
	Timeout        time.Duration
	WallClockCap   time.Duration

	// WritePathAllowed, when non-nil, is passed through to the RunRequest so the
	// ACP transport can enforce the contract path-scope at the file-write
	// boundary. Write stages (execute, review fix) populate it; read-only stages
	// leave it nil.
	WritePathAllowed func(repoRelPath string) bool
	// ReadOnly marks read-only stages (review, clarifier round, contract draft):
	// the ACP transport denies the agent's writes and permission requests. Write
	// stages (execute, review fix) leave it false.
	ReadOnly bool

	// OnFirstOutput is threaded to the RunRequest so a caller can observe the
	// attempt's first visible output. Only the lead attempt of a staggered
	// same-model Claude review group sets it; every other attempt leaves it nil.
	OnFirstOutput func()

	StartedEvent  string
	FinishedEvent string

	ExitKind       string
	TimeoutMessage func(time.Duration) string

	Prepare       func(createdAt string) (Prepared, error)
	BuildRequest  func(agentAttemptContext[Prepared]) (Request, error)
	BuildResult   func(agentAttemptContext[Prepared], agents.RunResult) Result
	ProcessResult func(Result) processResult
	RenderRunOnly func(io.Writer, Request, Result)
	// WrapRunOnly wraps the result document for the run-only --json response so
	// a stage can attach CLI-only affordances (next) without writing them into
	// attempt artifacts. Nil leaves the result document as the response.
	WrapRunOnly   func(Result) any
	AfterSuccess  func(agentAttemptContext[Prepared], Request, Result, time.Time) (Response, error)
	RenderSuccess func(io.Writer, Response, Request)
}

type agentAttemptContext[Prepared any] struct {
	Prepared     Prepared
	RunID        string
	AttemptID    string
	CreatedAt    string
	AttemptPaths attemptPathSet
}

func runAgentAttemptLifecycle[Prepared any, Request any, Result any, Response any](a App, cfg agentAttemptLifecycle[Prepared, Request, Result, Response]) error {
	now := a.nowUTC()
	createdAt := now.Format(time.RFC3339)
	prepared, err := cfg.Prepare(createdAt)
	if err != nil {
		return err
	}

	agentAttemptLifecycleMu.Lock()
	attemptID, err := nextAgentAttemptID(cfg.AttemptsDir, cfg.AttemptIDPrefix)
	if err != nil {
		agentAttemptLifecycleMu.Unlock()
		return err
	}
	attemptPaths := newAttemptPaths(filepath.Join(cfg.AttemptsDir, attemptID))
	if err := activeStore.MkdirAll(attemptPaths.Dir); err != nil {
		agentAttemptLifecycleMu.Unlock()
		return err
	}
	agentAttemptLifecycleMu.Unlock()

	context := agentAttemptContext[Prepared]{
		Prepared:     prepared,
		RunID:        cfg.RunID,
		AttemptID:    attemptID,
		CreatedAt:    createdAt,
		AttemptPaths: attemptPaths,
	}
	request, err := cfg.BuildRequest(context)
	if err != nil {
		return err
	}
	if err := writeJSON(attemptPaths.RequestJSON, request); err != nil {
		return err
	}
	agentAttemptLifecycleMu.Lock()
	err = ledger.Append(activeStore, cfg.EventsJSONL, ledger.Event{Type: cfg.StartedEvent, Timestamp: now, RunID: cfg.RunID})
	agentAttemptLifecycleMu.Unlock()
	if err != nil {
		return err
	}

	runResult, runErr := a.agentTransport().Run(agents.RunRequest{
		RepoRoot:         cfg.Root,
		RunID:            cfg.RunID,
		AttemptID:        attemptID,
		Agent:            cfg.Agent,
		PromptRepoPath:   cfg.PromptRepoPath,
		ArtifactDir:      cfg.ArtifactDir,
		Timeout:          cfg.Timeout,
		WallClockCap:     cfg.WallClockCap,
		LiveOutput:       cfg.LiveOutput,
		WritePathAllowed: cfg.WritePathAllowed,
		Model:            cfg.Model,
		ReadOnly:         cfg.ReadOnly,
		OnFirstOutput:    cfg.OnFirstOutput,
	})
	if runErr != nil && runResult.StartedAt == "" {
		return runErr
	}
	result := cfg.BuildResult(context, runResult)
	if err := writeJSON(attemptPaths.ResultJSON, result); err != nil {
		return err
	}
	agentAttemptLifecycleMu.Lock()
	err = writeJSON(cfg.LastResultJSON, result)
	if err == nil {
		appendUsageRecordBestEffort(cfg, attemptID, runResult)
		err = ledger.Append(activeStore, cfg.EventsJSONL, ledger.Event{Type: cfg.FinishedEvent, Timestamp: agentAttemptFinishedAt(cfg.ProcessResult(result), now), RunID: cfg.RunID})
	}
	agentAttemptLifecycleMu.Unlock()
	if err != nil {
		return err
	}

	// A completed-despite-timeout attempt is a success with a warning: the idle
	// kill error does not fail the run, and the attempt proceeds below exactly
	// like a success (AfterSuccess runs, artifacts already written).
	if runErr != nil && !cfg.ProcessResult(result).CompletedDespiteTimeout {
		if err := writeAgentAttemptRunOnly(cfg, request, result); err != nil {
			return err
		}
		// The run-only document above is the payload (like a gate failure's
		// report): the typed error tells the --json path not to append a
		// second JSON document (the error envelope) to the same stdout.
		process := cfg.ProcessResult(result)
		if process.WallClockTimeout {
			return agentAttemptFailedError{msg: fmt.Sprintf("%s process killed after wall-clock cap", cfg.ExitKind)}
		}
		if process.TimedOut {
			return agentAttemptFailedError{msg: cfg.TimeoutMessage(cfg.Timeout)}
		}
		return agentAttemptFailedError{msg: processExitError{Kind: cfg.ExitKind, ExitCode: process.ExitCode}.Error()}
	}

	if cfg.AfterSuccess == nil {
		return writeAgentAttemptRunOnly(cfg, request, result)
	}
	response, err := cfg.AfterSuccess(context, request, result, now)
	if err != nil {
		return err
	}
	if cfg.JSONOutput {
		return writeJSONResponse(cfg.Stdout, response)
	}
	cfg.RenderSuccess(cfg.Stdout, response, request)
	return nil
}

// agentAttemptFailedError marks a failed agent attempt whose run-only result
// document was already written to stdout — the --json path must not append an
// error envelope after it (mirrors gateProcessError for gate failures).
type agentAttemptFailedError struct {
	msg string
}

func (e agentAttemptFailedError) Error() string { return e.msg }

func writeAgentAttemptRunOnly[Prepared any, Request any, Result any, Response any](cfg agentAttemptLifecycle[Prepared, Request, Result, Response], request Request, result Result) error {
	if cfg.JSONOutput {
		if cfg.WrapRunOnly != nil {
			return writeJSONResponse(cfg.Stdout, cfg.WrapRunOnly(result))
		}
		return writeJSONResponse(cfg.Stdout, result)
	}
	cfg.RenderRunOnly(cfg.Stdout, request, result)
	return nil
}

func nextAgentAttemptID(attemptsDir string, prefix string) (string, error) {
	entries, err := activeStore.ReadDir(attemptsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("%s_%03d", prefix, 1), nil
		}
		return "", err
	}
	maxAttempt := 0
	pattern := prefix + "_%03d"
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var number int
		if _, err := fmt.Sscanf(entry.Name(), pattern, &number); err == nil && number > maxAttempt {
			maxAttempt = number
		}
	}
	return fmt.Sprintf(pattern, maxAttempt+1), nil
}

func agentAttemptPaths(attemptsDir string, attemptID string) attemptPathSet {
	return newAttemptPaths(filepath.Join(attemptsDir, attemptID))
}

func agentAttemptFinishedAt(result processResult, fallback time.Time) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, result.FinishedAt); err == nil {
		return parsed
	}
	return fallback
}

func appendUsageRecordBestEffort[Prepared any, Request any, Result any, Response any](cfg agentAttemptLifecycle[Prepared, Request, Result, Response], attemptID string, runResult agents.RunResult) {
	createdAt := runResult.FinishedAt
	if strings.TrimSpace(createdAt) == "" {
		createdAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := appendUsageRecord(cfg.Root, cfg.RunID, attemptID, cfg.Stage, cfg.AgentName, cfg.Model.Model, cfg.Agent, runResult.Usage, createdAt); err != nil && cfg.LiveOutput != nil {
		_, _ = fmt.Fprintf(cfg.LiveOutput, "usage capture warning: append usage ledger: %v\n", err)
	}
}

func agentDescriptorDocument(agent agents.AgentDescriptor) agents.AgentDescriptor {
	return agents.AgentDescriptor{
		Name:    agent.Name,
		Command: agent.Command,
		Args:    append([]string{}, agent.Args...),
		Input:   agent.Input,
	}
}
