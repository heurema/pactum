package agents

import (
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
	ProjectMapFresh             bool `json:"project_map_fresh"`
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
}

type RunRequest struct {
	RepoRoot       string
	RunID          string
	AttemptID      string
	Agent          AgentDescriptor
	PromptRepoPath string
	ArtifactDir    string
	// Timeout is an idle safety timeout: the process is cancelled only after
	// this duration passes without stdout or stderr output.
	Timeout time.Duration
	// LiveOutput, when set, receives a copy of the agent's stdout and stderr as
	// the process runs, in addition to the per-attempt log files. Callers pass
	// the operator's stderr so multi-minute runs are not a silent black box.
	// Leaving it nil preserves capture-only behavior.
	LiveOutput io.Writer
	// WritePathAllowed, when non-nil, is consulted by the ACP transport at the
	// file-write boundary: it reports whether a write to the given repo-relative
	// slash path is within the contract scope. The ACP transport denies (errors,
	// no disk write) any write the predicate rejects, giving a real-time scope
	// guard in addition to the post-hoc gate. The CLI transport ignores this
	// field. Leaving it nil preserves allow-all behavior for every caller.
	WritePathAllowed func(repoRelPath string) bool
	// Model is the resolved model pin for this attempt. The ACP transport
	// threads it to the adapter (codex: -c overrides, claude: env vars); the CLI
	// transport ignores it — the pin is already applied to the agent CLI args
	// via ApplyModelSpec. A zero spec means unpinned.
	Model ModelSpec
	// ReadOnly marks the attempt as a read-only stage (review, clarifier round,
	// contract draft). The ACP transport denies WriteTextFile and refuses
	// permission requests instead of auto-approving; the CLI transport ignores
	// it — read-only-ness is already baked into the reviewer descriptors (e.g.
	// codex --sandbox read-only).
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
	CompletedDespiteTimeout bool       `json:"completed_despite_timeout,omitempty"`
	StdoutPath              string     `json:"stdout"`
	StderrPath              string     `json:"stderr"`
	Usage                   TokenUsage `json:"usage"`
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
