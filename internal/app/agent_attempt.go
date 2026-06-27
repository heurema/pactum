package app

import (
	"encoding/json"
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

// agentAttemptGitGuard wires the git guard into the lifecycle for a write
// stage. Outcome is filled by the guard before BuildResult is called; callers
// can capture the pointer in their BuildResult closures.
type agentAttemptGitGuard struct {
	Root        string
	IsReviewFix bool             // skip clean-tree precondition
	Outcome     *gitGuardOutcome // filled before BuildResult runs
}

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

	// GitGuard, when non-nil, wraps the transport call with the git-history guard.
	// Set only for write-enabled stages (execute, review-fix).
	GitGuard *agentAttemptGitGuard

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

// maxTransportCalls is the total number of transport calls allowed per lifecycle
// attempt (initial call + at most maxTransportCalls-1 retries). Only read-only
// stages are retried; write-enabled stages use one call only.
const maxTransportCalls = 3

// transportRetryDelay returns the delay before the next transport call.
// callNum is the one-based number of the call that just failed.
// jitter is a multiplicative factor for the exponential base (1s, 2s, …);
// pass 1.0 for deterministic delays (used by tests to lock delay_ms values).
func transportRetryDelay(callNum int, jitter float64) time.Duration {
	base := time.Duration(1<<uint(callNum-1)) * time.Second
	return time.Duration(float64(base) * jitter)
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

	runReq := agents.RunRequest{
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
	}
	retryDecisionsPath := filepath.Join(attemptPaths.Dir, "retry-decisions.jsonl")

	var runResult agents.RunResult
	var runErr error
	var retryArtifactErr error

	// Git guard: preconditions + snapshot. On precondition failure, skip the
	// transport entirely and synthesize a failed RunResult so the lifecycle
	// still produces a complete attempt artifact.
	var guardSnap *gitGuardSnapshot
	var guardSkipTransport bool
	if cfg.GitGuard != nil {
		ok, reason, snap, err := gitGuardPrechecks(cfg.GitGuard.Root, cfg.GitGuard.IsReviewFix)
		if err != nil {
			return err
		}
		if !ok {
			*cfg.GitGuard.Outcome = gitGuardOutcome{
				TerminalReason: reason,
				Detail:         gitGuardPrecheckDetail(reason),
			}
			guardSkipTransport = true
			nowNano := a.nowUTC().Format(time.RFC3339Nano)
			runResult = agents.RunResult{
				StartedAt:  nowNano,
				FinishedAt: nowNano,
				ExitCode:   1,
			}
			runErr = fmt.Errorf("git guard: %s", reason)
		} else {
			guardSnap = snap
		}
	}

	if !guardSkipTransport {
		for callNum := 1; callNum <= maxTransportCalls; callNum++ {
			runResult, runErr = a.agentTransport().Run(runReq)

			// Pre-start failure: the transport could not launch the process at all.
			// Preserve the existing early-return behavior so no partial artifacts are
			// written. Retries are not attempted for pre-start failures.
			if runErr != nil && runResult.StartedAt == "" {
				break
			}

			// Wall-clock cap or completed-despite-timeout suppress retries: the result
			// is conclusive and must be returned as-is on any stage.
			if runResult.WallClockTimeout || runResult.CompletedDespiteTimeout {
				break
			}

			isLastCall := callNum >= maxTransportCalls
			// Write stages (ReadOnly=false) are never retried: retrying after a
			// possible write risks duplicate writes. No retry-decision artifact is
			// written for write stages.
			if !cfg.ReadOnly {
				break
			}
			// Last call on a read-only stage: no more retries are available.
			if isLastCall {
				if runErr != nil || isTransportOutputEmpty(attemptPaths.StdoutLog) {
					retryArtifactErr = appendRetryDecision(retryDecisionsPath, cfg.Stage, attemptID, runErr, runResult, cfg.ReadOnly, callNum, 0)
				}
				break
			}

			// Determine whether this failure/empty output is retryable.
			var cls agents.TransportErrorClass
			if runErr != nil {
				cls = agents.ClassifyTransportError(runErr)
				// An idle timeout sets RunResult.TimedOut and cancels the context; the
				// resulting context.Canceled is retryable as an idle timeout regardless of
				// what the classifier returns for context.Canceled alone.
				if runResult.TimedOut && !runResult.WallClockTimeout {
					cls.Retryable = true
					cls.Kind = "idle_timeout"
					cls.Reason = "idle timeout: agent process killed after period of no output"
				}
			} else if isTransportOutputEmpty(attemptPaths.StdoutLog) {
				cls = agents.TransportErrorClass{Retryable: true, Kind: "empty_output", Reason: "transport returned no output"}
			} else {
				break // success
			}

			delay := transportRetryDelay(callNum, a.jitter())
			if !cls.Retryable {
				delay = 0
			}
			if retryArtifactErr = appendRetryDecision(retryDecisionsPath, cfg.Stage, attemptID, runErr, runResult, cfg.ReadOnly, callNum, delay); retryArtifactErr != nil {
				break
			}
			if !cls.Retryable {
				break
			}
			a.sleep(delay)
		}

		// Git guard: compare post-transport state to snapshot and restore if needed.
		// Runs even when the transport errored, so mutations are always detected.
		if cfg.GitGuard != nil && guardSnap != nil {
			outcome := gitGuardCompareAndRestore(cfg.GitGuard.Root, guardSnap, cfg.GitGuard.IsReviewFix, runErr)
			*cfg.GitGuard.Outcome = outcome
			if outcome.TerminalReason != "" && runErr == nil {
				// Transport succeeded but guard found a mutation; fail the attempt.
				runErr = fmt.Errorf("git guard: %s", outcome.TerminalReason)
			}
		}
	}

	if retryArtifactErr != nil {
		return retryArtifactErr
	}

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
		// Git guard terminal reason takes precedence over generic exit-code messages.
		if cfg.GitGuard != nil && cfg.GitGuard.Outcome.TerminalReason != "" {
			return agentAttemptFailedError{msg: "git guard: " + cfg.GitGuard.Outcome.TerminalReason}
		}
		process := cfg.ProcessResult(result)
		if process.WallClockTimeout {
			return agentAttemptFailedError{msg: fmt.Sprintf("%s process killed after wall-clock cap", cfg.ExitKind)}
		}
		if process.TimedOut {
			return agentAttemptFailedError{msg: cfg.TimeoutMessage(cfg.Timeout)}
		}
		return agentAttemptFailedError{msg: processExitError{Kind: cfg.ExitKind, ExitCode: process.ExitCode}.Error()}
	}

	// Guard terminal reason on an otherwise-successful transport: the transport
	// exited 0 but the guard detected and restored a git mutation — still fail.
	if cfg.GitGuard != nil && cfg.GitGuard.Outcome.TerminalReason != "" {
		if err := writeAgentAttemptRunOnly(cfg, request, result); err != nil {
			return err
		}
		return agentAttemptFailedError{msg: "git guard: " + cfg.GitGuard.Outcome.TerminalReason}
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

func gitGuardPrecheckDetail(reason string) string {
	switch reason {
	case gitGuardReasonUnbornHead:
		return `the repository has no commits yet — the git guard requires a baseline commit; run: git add -A && git commit -m "initial commit"`
	case gitGuardReasonInconclusive:
		return "the git guard could not establish a safe clean baseline before transport; if the intended baseline has dirty worktree or index changes, checkpoint them with a commit/stash or clean them up, resolve in-progress git operations or stale lock files if present, rebuild the prompt if HEAD changes, then rerun execute"
	default:
		return "git guard precondition failed before transport; establish a clean baseline, checkpoint or clean up intended worktree/index changes, then rerun execute"
	}
}

// isTransportOutputEmpty reports whether the transport's stdout artifact is
// absent or contains only whitespace. A nil-error transport result with no
// non-whitespace output is eligible for retry on read-only stages.
func isTransportOutputEmpty(stdoutPath string) bool {
	data, err := os.ReadFile(stdoutPath)
	if err != nil {
		return true // file absent → treat as empty
	}
	return len(strings.TrimSpace(string(data))) == 0
}

// retryDecisionRecord is the JSONL schema for a single transport retry decision.
type retryDecisionRecord struct {
	Schema        string `json:"schema"`
	Stage         string `json:"stage"`
	AttemptID     string `json:"attempt_id"`
	Kind          string `json:"kind"`
	Reason        string `json:"reason"`
	Retryable     bool   `json:"retryable"`
	ReadOnly      bool   `json:"read_only"`
	AttemptNumber int    `json:"attempt_number"`
	DelayMS       int64  `json:"delay_ms"`
}

// appendRetryDecision appends a single retry-decision record to the JSONL
// artifact file and returns any write error. The caller must propagate write
// failures: a durable record is required before scheduling or skipping a retry.
func appendRetryDecision(path string, stage string, attemptID string, runErr error, runResult agents.RunResult, readOnly bool, callNum int, delay time.Duration) error {
	var cls agents.TransportErrorClass
	if runErr != nil {
		cls = agents.ClassifyTransportError(runErr)
		// Idle timeout: context.Canceled from the watchdog kill is retryable
		// regardless of what the classifier returns for context.Canceled alone.
		if runResult.TimedOut && !runResult.WallClockTimeout {
			cls.Retryable = true
			cls.Kind = "idle_timeout"
			cls.Reason = "idle timeout: agent process killed after period of no output"
		}
	} else {
		cls = agents.TransportErrorClass{Retryable: true, Kind: "empty_output", Reason: "transport returned no output"}
	}
	rec := retryDecisionRecord{
		Schema:        "pactum.agent_retry_decision.v1alpha1",
		Stage:         stage,
		AttemptID:     attemptID,
		Kind:          cls.Kind,
		Reason:        cls.Reason,
		Retryable:     cls.Retryable,
		ReadOnly:      readOnly,
		AttemptNumber: callNum,
		DelayMS:       delay.Milliseconds(),
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return activeStore.AppendBytes(path, data)
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
