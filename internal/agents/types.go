package agents

import "time"

type AgentConfig struct {
	DefaultExecutor string                   `json:"default_executor,omitempty" yaml:"default_executor,omitempty"`
	DefaultReviewer string                   `json:"default_reviewer,omitempty" yaml:"default_reviewer,omitempty"`
	Adapters        map[string]AdapterConfig `json:"adapters,omitempty" yaml:"adapters,omitempty"`
	ExecutorModel   string                   `json:"executor_model,omitempty" yaml:"executor_model,omitempty"`
}

type AdapterConfig struct {
	Command string   `json:"command" yaml:"command"`
	Args    []string `json:"args" yaml:"args"`
	Input   string   `json:"input" yaml:"input"`
}

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
	Timeout        time.Duration
}

type RunResult struct {
	Command        string   `json:"-"`
	Args           []string `json:"-"`
	ExitCode       int      `json:"exit_code"`
	StartedAt      string   `json:"started_at"`
	FinishedAt     string   `json:"finished_at"`
	DurationMillis int64    `json:"duration_ms"`
	TimedOut       bool     `json:"timed_out"`
	StdoutPath     string   `json:"stdout"`
	StderrPath     string   `json:"stderr"`
}
