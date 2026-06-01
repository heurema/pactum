package agents

type AgentConfig struct {
	DefaultExecutor string                   `json:"default_executor" yaml:"default_executor"`
	Adapters        map[string]AdapterConfig `json:"adapters" yaml:"adapters"`
}

type AdapterConfig struct {
	Command string   `json:"command" yaml:"command"`
	Args    []string `json:"args" yaml:"args"`
	Input   string   `json:"input" yaml:"input"`
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

type DryRunAgent struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Input   string   `json:"input"`
}

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
}
