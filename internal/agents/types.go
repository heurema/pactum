package agents

import (
	"context"
	"encoding/json"
	"io"
	"time"
)

type AgentDescriptor struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Input   string   `json:"input"`
}

type Registry interface {
	DefaultExecutor() string
	DefaultReviewer() string
	ResolveExecutor(name string) (AgentDescriptor, error)
	ResolveReviewer(name string) (AgentDescriptor, error)
	ListBuiltins() []AgentDescriptor
}

type DryRunPlan struct {
	Schema    string          `json:"schema"`
	RunID     string          `json:"run_id"`
	CreatedAt string          `json:"created_at"`
	Agent     DryRunAgent     `json:"agent"`
	Checks    DryRunChecks    `json:"checks"`
	Artifacts DryRunArtifacts `json:"artifacts"`
	WouldRun  DryRunCommand   `json:"would_run"`
}

// DryRunAgent describes the agent a dry-run plan would launch. It is the same
// shape as AgentDescriptor.
type DryRunAgent = AgentDescriptor

type DryRunChecks struct {
	PromptManifestReady         bool `json:"prompt_manifest_ready"`
	ContractHashMatchesApproval bool `json:"contract_hash_matches_approval"`
}

type DryRunArtifacts struct {
	Prompt          string `json:"prompt"`
	ExecutorContext string `json:"executor_context"`
	PromptManifest  string `json:"prompt_manifest"`
}

type DryRunCommand struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Stdin   string   `json:"stdin,omitempty"`
	Env     []string `json:"env,omitempty"`
}

type RunRequest struct {
	// Context cancels the in-flight transport from the caller side, for example
	// when the CLI receives an interrupt. Nil means context.Background().
	Context        context.Context
	RepoRoot       string
	RunID          string
	AttemptID      string
	Agent          AgentDescriptor
	PromptRepoPath string
	ArtifactDir    string
	// Timeout is an idle safety timeout: the process is cancelled only after
	// this duration passes without stdout or stderr output.
	Timeout time.Duration
	// WallClockCap, when positive, is the absolute maximum duration for the
	// attempt from start to finish regardless of output activity. When it fires
	// the run context is cancelled so the existing process-group kill/reap path
	// handles cleanup. Unlike Timeout it never resets on streamed output or
	// inbound protocol callbacks.
	WallClockCap time.Duration
	// LiveOutput, when set, receives a copy of the agent's stdout and stderr as
	// the process runs, in addition to the per-attempt log files. Callers pass
	// the operator's stderr so multi-minute runs are not a silent black box.
	// Leaving it nil preserves capture-only behavior.
	LiveOutput io.Writer
	// OnFirstOutput, when set, is invoked exactly once the moment the attempt
	// produces its first visible output: over ACP the first non-empty agent
	// message chunk written to the attempt stdout.log, over the CLI the first
	// non-empty stdout or stderr write. The review fan-out uses it to release a
	// staggered same-model Claude group once the lead has warmed the prompt
	// cache. Leaving it nil is a no-op for every other caller.
	OnFirstOutput func()
	// WritePathAllowed, when non-nil, is consulted by the ACP transport at the
	// file-write boundary: it reports whether a write to the given repo-relative
	// slash path is within the contract scope. The ACP transport denies (errors,
	// no disk write) any write the predicate rejects, giving a real-time scope
	// guard in addition to the post-hoc gate. Leaving it nil preserves allow-all
	// behavior for every caller.
	WritePathAllowed func(repoRelPath string) bool
	// Model is the resolved model pin for this attempt. The ACP transport
	// threads it to the adapter (codex: -c overrides, claude: env vars) via
	// ApplyModelSpec. A zero spec means unpinned.
	Model ModelSpec
	// ReadOnly marks the attempt as a read-only stage (review, clarifier round,
	// contract draft). The ACP transport denies WriteTextFile and refuses
	// permission requests instead of auto-approving; for codex the adapter is
	// also launched with sandbox_mode="read-only" so native patch writes are
	// blocked at the adapter level.
	ReadOnly bool
}

type RunResult struct {
	ExitCode       int    `json:"exit_code"`
	StartedAt      string `json:"started_at"`
	FinishedAt     string `json:"finished_at"`
	DurationMillis int64  `json:"duration_ms"`
	TimedOut       bool   `json:"timed_out"`
	// CompletedDespiteTimeout marks an idle-killed attempt whose captured output
	// carries the agent's successful terminal marker: the watchdog did fire
	// (TimedOut stays true for the record), but the agent had already finished,
	// so the attempt is finalized as completed with a warning (ExitCode 0).
	CompletedDespiteTimeout bool `json:"completed_despite_timeout,omitempty"`
	// WallClockTimeout marks an attempt killed by the absolute wall-clock cap
	// rather than the idle watchdog (TimedOut). Distinct from a generic
	// transport error: the process hung past the hard ceiling and was reaped by
	// the cap timer, not by an ACP protocol failure or idle silence.
	WallClockTimeout bool       `json:"wall_clock_timeout,omitempty"`
	StdoutPath       string     `json:"stdout"`
	StderrPath       string     `json:"stderr"`
	Usage            TokenUsage `json:"usage"`
}

// TokenUsage is a provider-normalized view of one agent subprocess call's
// usage. Counts follow the OTel-inclusive convention: input includes cache, and
// output includes reasoning.
type TokenUsage struct {
	InputTokens         int64           `json:"input_tokens"`
	OutputTokens        int64           `json:"output_tokens"`
	TotalTokens         int64           `json:"total_tokens"`
	CacheReadTokens     int64           `json:"cache_read_tokens,omitempty"`
	CacheCreationTokens int64           `json:"cache_creation_tokens,omitempty"`
	ReasoningTokens     int64           `json:"reasoning_tokens,omitempty"`
	Captured            bool            `json:"captured"`
	Raw                 json.RawMessage `json:"raw,omitempty"`

	CaptureWarning string `json:"-"`
}
