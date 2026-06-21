package app

import "time"

type cli struct {
	Clarify  clarifyCmd  `cmd:"" help:"Manage manual clarification artifacts."`
	Contract contractCmd `cmd:"" help:"Inspect, revise, and approve run contracts."`
	Doctor   doctorCmd   `cmd:"" help:"Diagnose built-in agents without launching them."`
	Execute  executeCmd  `cmd:"" help:"Prepare deterministic execution artifacts."`
	Export   exportCmd   `cmd:"" help:"Export a run's full record as a single archive."`
	Gate     gateCmd     `cmd:"" help:"Run deterministic validation and scope gates."`
	Init     initCmd     `cmd:"" help:"Create a Pactum workspace and project map."`
	Map      mapCmd      `cmd:"" help:"Advanced project map commands."`
	Memory   memoryCmd   `cmd:"" help:"Propose, inspect, and accept deterministic project memory."`
	Prompt   promptCmd   `cmd:"" help:"Build and inspect executor prompt boundaries."`
	Review   reviewCmd   `cmd:"" help:"Manage manual review artifacts."`
	Search   searchCmd   `cmd:"" help:"Search the Pactum project map."`
	Skill    skillCmd    `cmd:"" help:"Install and verify the Pactum agent skill package."`
	Status   statusCmd   `cmd:"" help:"Print Pactum workspace status."`
	Task     taskCmd     `cmd:"" help:"Create and manage contract-first runs."`
	Usage    usageCmd    `cmd:"" help:"Print token usage for a run."`
	Version  versionCmd  `cmd:"" help:"Print the Pactum version."`
}

type initCmd struct {
	Path string `arg:"" optional:"" default:"." name:"path" help:"Repository path to initialize."`
}

type mapCmd struct {
	Refresh mapRefreshCmd `cmd:"" help:"Rebuild generated project map artifacts."`
}

type clarifyCmd struct {
	Add    clarifyAddCmd    `cmd:"" help:"Add a manual clarification question."`
	Answer clarifyAnswerCmd `cmd:"" help:"Record a manual clarification answer."`
	Run    clarifyRunCmd    `cmd:"" help:"Run clarifier rounds that auto-resolve high-confidence recommendations until convergence."`
	Show   clarifyShowCmd   `cmd:"" help:"Print clarification status for a run."`
}

type clarifyAddCmd struct {
	Args       []string `arg:"" optional:"" name:"args" help:"[run_id] <question>"`
	Blocking   bool     `name:"blocking" help:"Mark the question as blocking contract progress."`
	JSONOutput bool     `name:"json" help:"Print machine-readable JSON output."`
}

type clarifyAnswerCmd struct {
	Args           []string `arg:"" optional:"" name:"args" help:"[run_id] <question_id> <answer>"`
	By             string   `name:"by" default:"manual" help:"Decider name to record."`
	Recommended    bool     `name:"recommended" help:"Record the question's stored recommended answer as the answer."`
	AllRecommended bool     `name:"all-recommended" help:"Record the stored recommended answer for every open question that has one."`
	JSONOutput     bool     `name:"json" help:"Print machine-readable JSON output."`
}

type clarifyRunCmd struct {
	RunID      string        `arg:"" optional:"" name:"run_id" help:"Run id to clarify."`
	Reviewer   string        `name:"reviewer" help:"Registry name (config agents) of the clarifier. Defaults to cross-model selection against the run executor."`
	MaxRounds  int           `name:"max-rounds" help:"Maximum clarifier rounds. Defaults to clarify.max_rounds."`
	NoAuto     bool          `name:"no-auto" help:"Skip auto-resolution of high-confidence recommendations; created questions stay open for the human."`
	Timeout    time.Duration `name:"timeout" default:"0" help:"Maximum idle duration without clarifier output. Defaults to 25m when not given."`
	JSONOutput bool          `name:"json" help:"Print machine-readable JSON output."`
}

type clarifyShowCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to inspect."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type contractCmd struct {
	Show    contractShowCmd    `cmd:"" help:"Show a run contract."`
	Draft   contractDraftCmd   `cmd:"" help:"Run a read-only agent to propose contract fields."`
	Accept  contractAcceptCmd  `cmd:"" help:"Accept the latest contract draft proposal."`
	Revise  contractReviseCmd  `cmd:"" help:"Revise deterministic contract fields."`
	Review  contractReviewCmd  `cmd:"" help:"Run contract reviewer panel or manage contract-review findings."`
	Approve contractApproveCmd `cmd:"" help:"Approve a run contract."`
}

type promptCmd struct {
	Build promptBuildCmd `cmd:"" help:"Build deterministic executor prompt artifacts."`
	Show  promptShowCmd  `cmd:"" help:"Show a built executor prompt."`
}

type executeCmd struct {
	Plan executePlanCmd `cmd:"plan" help:"Prepare execution artifacts without running an agent."`
	Run  executeRunCmd  `cmd:"run" help:"Run the selected built-in agent directly in the repository."`
	Show executeShowCmd `cmd:"show" help:"Summarize execution, or show a captured attempt's artifacts."`
}

type gateCmd struct {
	Run  gateRunCmd  `cmd:"run" help:"Run deterministic validation and scope checks."`
	Show gateShowCmd `cmd:"show" help:"Show the latest gate report."`
}

type reviewCmd struct {
	Status   reviewStatusCmd   `cmd:"" help:"Show manual review status."`
	Show     reviewShowCmd     `cmd:"" help:"Show manual review findings."`
	Finding  reviewFindingCmd  `cmd:"" help:"Manage manual review findings."`
	Approve  reviewApproveCmd  `cmd:"" help:"Approve a manual review."`
	Plan     reviewPlanCmd     `cmd:"plan" help:"Prepare reviewer artifacts without running a reviewer."`
	Run      reviewRunCmd      `cmd:"run" help:"Run reviewer/fixer rounds until a clean review round or max rounds."`
	Fix      reviewFixCmd      `cmd:"fix" help:"Run a write-enabled fixer and apply its outcomes."`
	Proposal reviewProposalCmd `cmd:"" help:"Manage pending review finding proposals."`
}

type reviewFindingCmd struct {
	Add     reviewFindingAddCmd     `cmd:"" help:"Append a manual review finding."`
	Resolve reviewFindingResolveCmd `cmd:"" help:"Resolve a manual review finding."`
}

type reviewFixCmd struct {
	Run   reviewFixRunCmd   `cmd:"run" help:"Run a write-enabled fixer against current review findings."`
	Apply reviewFixApplyCmd `cmd:"apply" help:"Parse fixer output into review resolutions."`
}

type reviewProposalCmd struct {
	Collect reviewProposalCollectCmd `cmd:"" help:"Parse reviewer output into pending finding proposals."`
	Accept  reviewProposalAcceptCmd  `cmd:"" help:"Accept a pending review finding proposal."`
	Reject  reviewProposalRejectCmd  `cmd:"" help:"Reject a pending review finding proposal."`
}

type memoryCmd struct {
	Propose memoryProposeCmd `cmd:"" help:"Create a deterministic memory candidate for a reviewed run."`
	Show    memoryShowCmd    `cmd:"" help:"Show a run memory candidate."`
	Accept  memoryAcceptCmd  `cmd:"" help:"Accept a run memory candidate into project memory."`
	Search  memorySearchCmd  `cmd:"" help:"Search accepted project memory deterministically."`
	Refresh memoryRefreshCmd `cmd:"" help:"Refresh accepted memory freshness metadata."`
	Stale   memoryStaleCmd   `cmd:"" help:"Show stale and unknown accepted memory items."`
}

type contractShowCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to inspect."`
	Draft      bool   `name:"draft" help:"Show the latest contract draft proposal instead of the contract."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type contractDraftCmd struct {
	RunID      string        `arg:"" optional:"" name:"run_id" help:"Run id to draft contract fields for."`
	Reviewer   string        `name:"reviewer" help:"Registry name (config agents) of the read-only drafter. Defaults to cross-model selection against the run executor."`
	Timeout    time.Duration `name:"timeout" default:"0" help:"Maximum idle duration without drafter output. Defaults to 25m when not given."`
	JSONOutput bool          `name:"json" help:"Print machine-readable JSON output."`
}

type contractAcceptCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id whose latest draft proposal should be accepted."`
	By         string `name:"by" default:"manual" help:"Acceptance name to record."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type contractReviseCmd struct {
	RunID              string `arg:"" optional:"" name:"run_id" help:"Run id to revise."`
	From               string `name:"from" required:"" help:"Path to a partial JSON update document, or - to read from stdin."`
	AllowApprovalReset bool   `name:"allow-approval-reset" help:"Permit resetting an existing approval when content changes."`
	JSONOutput         bool   `name:"json" help:"Print machine-readable JSON output."`
}

type contractApproveCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to approve."`
	By         string `name:"by" default:"manual" help:"Approver name to record."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type contractReviewCmd struct {
	Run     contractReviewRunCmd     `cmd:"run" help:"Run the configured contract reviewer panel against the contract."`
	Finding contractReviewFindingCmd `cmd:"" help:"Manage contract-review findings."`
}

type contractReviewRunCmd struct {
	RunID      string        `arg:"" optional:"" name:"run_id" help:"Run id whose contract to review."`
	Timeout    time.Duration `name:"timeout" default:"0" help:"Maximum idle duration without reviewer output. Defaults to 25m when not given."`
	JSONOutput bool          `name:"json" help:"Print machine-readable JSON output."`
}

type contractReviewFindingCmd struct {
	Resolve contractReviewFindingResolveCmd `cmd:"" help:"Resolve a blocking contract-review finding."`
}

type contractReviewFindingResolveCmd struct {
	Args       []string `arg:"" optional:"" name:"args" help:"[run_id] <finding_id>"`
	Reason     string   `name:"reason" required:"" help:"Reason for resolving the finding (required)."`
	By         string   `name:"by" required:"" help:"Principal resolving the finding (required)."`
	JSONOutput bool     `name:"json" help:"Print machine-readable JSON output."`
}

type promptBuildCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to build prompt artifacts for."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type promptShowCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to inspect."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type executePlanCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to prepare for execution."`
	Agent      string `name:"agent" help:"Registry name (config agents) of the executor. Defaults to the first registry entry."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type executeRunCmd struct {
	RunID      string        `arg:"" optional:"" name:"run_id" help:"Run id to execute."`
	Agent      string        `name:"agent" help:"Registry name (config agents) of the executor. Defaults to the first registry entry."`
	Timeout    time.Duration `name:"timeout" default:"0" help:"Maximum idle duration without agent output. Defaults to 25m when not given."`
	JSONOutput bool          `name:"json" help:"Print machine-readable JSON output."`
}

type executeShowCmd struct {
	Args       []string `arg:"" optional:"" name:"args" help:"[run_id] [attempt_id]"`
	Logs       bool     `name:"logs" help:"Include bounded stdout/stderr excerpts."`
	JSONOutput bool     `name:"json" help:"Print machine-readable JSON output."`
}

type exportCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to export."`
	Output     string `name:"output" required:"" help:"Archive file to create (must not already exist)."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type gateRunCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to inspect."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type gateShowCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to inspect."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type reviewStatusCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to inspect."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type reviewShowCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to inspect."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type reviewFindingAddCmd struct {
	Args       []string `arg:"" optional:"" name:"args" help:"[run_id] <message>"`
	Severity   string   `name:"severity" default:"medium" enum:"low,medium,high,critical" help:"Finding severity."`
	Category   string   `name:"category" default:"other" enum:"correctness,scope,quality,validation,process,other" help:"Finding category."`
	File       string   `name:"file" help:"Repo-relative file path."`
	Line       int      `name:"line" help:"Optional line number."`
	Blocking   bool     `name:"blocking" help:"Block review approval until resolved."`
	JSONOutput bool     `name:"json" help:"Print machine-readable JSON output."`
}

type reviewFindingResolveCmd struct {
	Args       []string `arg:"" optional:"" name:"args" help:"[run_id] <finding_id>"`
	Note       string   `name:"note" help:"Resolution note."`
	JSONOutput bool     `name:"json" help:"Print machine-readable JSON output."`
}

type reviewApproveCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to review."`
	By         string `name:"by" default:"manual" help:"Approver name to record."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type reviewPlanCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to prepare reviewer artifacts for."`
	Reviewer   string `name:"reviewer" help:"Registry name (config agents) of the reviewer. Defaults to cross-model selection against the run executor."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type reviewRunCmd struct {
	RunID       string        `arg:"" optional:"" name:"run_id" help:"Run id to review."`
	Reviewer    string        `name:"reviewer" help:"Registry name (config agents) of the reviewer. Defaults to the review panel, falling back to cross-model selection."`
	Agent       string        `name:"agent" help:"Registry name (config agents) of the fixer. Defaults to the first registry entry."`
	MaxRounds   int           `name:"max-rounds" help:"Maximum review rounds. Defaults to review.max_rounds."`
	Patience    int           `name:"patience" help:"Consecutive no-change fixer rounds before stopping as stalemate. Defaults to review.patience."`
	CleanRounds int           `name:"clean-rounds" help:"Consecutive clean review rounds required before convergence. Defaults to review.clean_rounds."`
	NoFix       bool          `name:"no-fix" help:"Never invoke the fixer; stop after the first round that leaves open blocking findings."`
	Timeout     time.Duration `name:"timeout" default:"0" help:"Maximum idle duration without reviewer or fixer output. Defaults to 25m when not given."`
	JSONOutput  bool          `name:"json" help:"Print machine-readable JSON output."`
}

type reviewFixRunCmd struct {
	RunID      string        `arg:"" optional:"" name:"run_id" help:"Run id whose review findings should be fixed."`
	Agent      string        `name:"agent" help:"Registry name (config agents) of the fixer. Defaults to the first registry entry."`
	Timeout    time.Duration `name:"timeout" default:"0" help:"Maximum idle duration without fixer output. Defaults to 25m when not given."`
	JSONOutput bool          `name:"json" help:"Print machine-readable JSON output."`
}

type reviewProposalCollectCmd struct {
	Args       []string `arg:"" optional:"" name:"args" help:"[run_id] [reviewer_attempt_id]"`
	JSONOutput bool     `name:"json" help:"Print machine-readable JSON output."`
}

type reviewFixApplyCmd struct {
	Args       []string `arg:"" optional:"" name:"args" help:"[run_id] [fixer_attempt_id]"`
	JSONOutput bool     `name:"json" help:"Print machine-readable JSON output."`
}

type reviewProposalAcceptCmd struct {
	Args       []string `arg:"" optional:"" name:"args" help:"[run_id] <proposal_id>"`
	By         string   `name:"by" default:"manual" help:"Decider name to record."`
	JSONOutput bool     `name:"json" help:"Print machine-readable JSON output."`
}

type reviewProposalRejectCmd struct {
	Args       []string `arg:"" optional:"" name:"args" help:"[run_id] <proposal_id>"`
	Reason     string   `name:"reason" help:"Reason for rejecting the proposal."`
	By         string   `name:"by" default:"manual" help:"Decider name to record."`
	JSONOutput bool     `name:"json" help:"Print machine-readable JSON output."`
}

type doctorCmd struct {
	Agent      string `name:"agent" help:"Built-in agent name to inspect."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type memoryProposeCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to propose memory for."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type memoryShowCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to inspect."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type memoryAcceptCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to accept memory for."`
	By         string `name:"by" default:"manual" help:"Acceptance name to record."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type memorySearchCmd struct {
	Query      string `arg:"" name:"query" help:"Accepted memory search query."`
	Limit      int    `name:"limit" help:"Maximum number of memory items." default:"5"`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type memoryRefreshCmd struct {
	JSONOutput bool `name:"json" help:"Print machine-readable JSON output."`
}

type memoryStaleCmd struct {
	JSONOutput bool `name:"json" help:"Print machine-readable JSON output."`
}

type mapRefreshCmd struct {
	JSONOutput bool `name:"json" help:"Print machine-readable JSON output."`
}

type statusCmd struct {
	JSONOutput bool `name:"json" help:"Print machine-readable JSON output."`
}

type usageCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to inspect."`
	All        bool   `name:"all" help:"Aggregate token usage across every run in the workspace."`
	By         string `name:"by" default:"stage" enum:"stage,model,agent,provider" help:"Grouping dimension: stage, model, agent, or provider."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type versionCmd struct {
	JSONOutput bool `name:"json" help:"Print machine-readable JSON output."`
}

type searchCmd struct {
	Query      string `arg:"" optional:"" name:"query" help:"Search query."`
	Limit      int    `help:"Maximum number of results." default:"10"`
	Kind       string `help:"Document kind filter." default:"any" enum:"any,repo_map,llms,wiki,file"`
	Symbol     string `name:"symbol" hidden:"" help:"Removed: symbol search was removed when tree-sitter was dropped."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type skillCmd struct {
	Install skillInstallCmd `cmd:"" help:"Install the embedded Pactum skill package for a coding agent."`
}

type skillInstallCmd struct {
	Agent      string `name:"agent" default:"auto" enum:"claude,codex,auto,all" help:"Target agent: claude, codex, auto (detect), or all."`
	Scope      string `name:"scope" default:"repo" enum:"user,repo" help:"Install scope: repo (.<agent>/skills) or user ($HOME/.<agent>/skills)."`
	Check      bool   `name:"check" help:"Verify the skill is installed without writing files."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}
