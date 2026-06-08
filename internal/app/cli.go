package app

import "time"

type cli struct {
	Agents   agentsCmd   `cmd:"" help:"Diagnose built-in agents."`
	Clarify  clarifyCmd  `cmd:"" help:"Manage manual clarification artifacts."`
	Contract contractCmd `cmd:"" help:"Inspect, revise, and approve run contracts."`
	Execute  executeCmd  `cmd:"" help:"Prepare deterministic execution artifacts."`
	Gate     gateCmd     `cmd:"" help:"Run deterministic validation and scope gates."`
	Init     initCmd     `cmd:"" help:"Create a Pactum workspace and project map."`
	Map      mapCmd      `cmd:"" help:"Advanced project map commands."`
	Memory   memoryCmd   `cmd:"" help:"Propose, inspect, and accept deterministic project memory."`
	Prompt   promptCmd   `cmd:"" help:"Build and inspect executor prompt boundaries."`
	Review   reviewCmd   `cmd:"" help:"Manage manual review artifacts."`
	Search   searchCmd   `cmd:"" help:"Search the Pactum project map."`
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
	Ask     clarifyAskCmd     `cmd:"" help:"Add a manual clarification question."`
	Answer  clarifyAnswerCmd  `cmd:"" help:"Record a manual clarification answer."`
	Suggest clarifySuggestCmd `cmd:"" help:"Run a read-only clarifier agent and record proposed questions."`
	Status  clarifyStatusCmd  `cmd:"" aliases:"list" help:"Print clarification status for a run (alias: list)."`
}

type clarifyAskCmd struct {
	Args       []string `arg:"" optional:"" name:"args" help:"[run_id] <question>"`
	Blocking   bool     `name:"blocking" help:"Mark the question as blocking contract progress."`
	JSONOutput bool     `name:"json" help:"Print machine-readable JSON output."`
}

type clarifyAnswerCmd struct {
	Args       []string `arg:"" optional:"" name:"args" help:"[run_id] <question_id> <answer>"`
	JSONOutput bool     `name:"json" help:"Print machine-readable JSON output."`
}

type clarifySuggestCmd struct {
	RunID      string        `arg:"" optional:"" name:"run_id" help:"Run id to suggest clarifications for."`
	Reviewer   string        `name:"reviewer" help:"Built-in clarifier agent name. Defaults to the configured reviewer unless cross-model review selects another built-in."`
	Timeout    time.Duration `name:"timeout" default:"10m" help:"Maximum idle duration without clarifier output."`
	Yes        bool          `name:"yes" help:"Skip the interactive confirmation (required in non-interactive use)."`
	JSONOutput bool          `name:"json" help:"Print machine-readable JSON output."`
}

type clarifyStatusCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to inspect."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type contractCmd struct {
	Show        contractShowCmd        `cmd:"" help:"Show a run contract."`
	Draft       contractDraftCmd       `cmd:"" help:"Run a read-only agent to propose contract fields."`
	ShowDraft   contractShowDraftCmd   `cmd:"show-draft" help:"Show the latest contract draft proposal."`
	AcceptDraft contractAcceptDraftCmd `cmd:"accept-draft" help:"Accept the latest contract draft proposal."`
	Revise      contractReviseCmd      `cmd:"" help:"Revise deterministic contract fields."`
	Approve     contractApproveCmd     `cmd:"" help:"Approve a run contract."`
}

type promptCmd struct {
	Build promptBuildCmd `cmd:"" help:"Build deterministic executor prompt artifacts."`
	Show  promptShowCmd  `cmd:"" help:"Show a built executor prompt."`
}

type executeCmd struct {
	DryRun executeDryRunCmd `cmd:"dry-run" help:"Prepare execution artifacts without running an agent."`
	Run    executeRunCmd    `cmd:"run" help:"Run the selected built-in agent directly in the repository."`
	Show   executeShowCmd   `cmd:"show" help:"Show captured execution attempt artifacts."`
	Status executeStatusCmd `cmd:"status" help:"Summarize captured execution artifacts."`
}

type gateCmd struct {
	Run  gateRunCmd  `cmd:"run" help:"Run deterministic validation and scope checks."`
	Show gateShowCmd `cmd:"show" help:"Show the latest gate report."`
}

type reviewCmd struct {
	Prepare          reviewPrepareCmd          `cmd:"" help:"Prepare manual review artifacts."`
	Status           reviewStatusCmd           `cmd:"" help:"Show manual review status."`
	Show             reviewShowCmd             `cmd:"" help:"Show manual review findings."`
	AddFinding       reviewAddFindingCmd       `cmd:"add-finding" help:"Append a manual review finding."`
	Resolve          reviewResolveCmd          `cmd:"" help:"Resolve a manual review finding."`
	Approve          reviewApproveCmd          `cmd:"" help:"Approve a manual review."`
	DryRun           reviewDryRunCmd           `cmd:"dry-run" help:"Prepare reviewer artifacts without running a reviewer."`
	Run              reviewRunCmd              `cmd:"run" help:"Run a built-in reviewer and capture attempt artifacts."`
	Fix              reviewFixCmd              `cmd:"fix" help:"Run a write-enabled fixer against current review findings."`
	Loop             reviewLoopCmd             `cmd:"loop" help:"Run reviewer/fixer rounds until a clean review round or max rounds."`
	ProposeFindings  reviewProposeFindingsCmd  `cmd:"propose-findings" help:"Parse reviewer output into pending finding proposals."`
	ApplyFixOutcomes reviewApplyFixOutcomesCmd `cmd:"apply-fix-outcomes" help:"Parse fixer output into review resolutions."`
	AcceptProposal   reviewAcceptProposalCmd   `cmd:"accept-proposal" help:"Accept a pending review finding proposal."`
	RejectProposal   reviewRejectProposalCmd   `cmd:"reject-proposal" help:"Reject a pending review finding proposal."`
}

type agentsCmd struct {
	Doctor agentsDoctorCmd `cmd:"" help:"Diagnose built-in agents without launching them."`
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
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type contractDraftCmd struct {
	RunID      string        `arg:"" optional:"" name:"run_id" help:"Run id to draft contract fields for."`
	Reviewer   string        `name:"reviewer" help:"Built-in read-only drafter name. Defaults to the configured reviewer unless cross-model review selects another built-in."`
	Timeout    time.Duration `name:"timeout" default:"10m" help:"Maximum idle duration without drafter output."`
	Yes        bool          `name:"yes" help:"Required confirmation for direct drafter execution."`
	JSONOutput bool          `name:"json" help:"Print machine-readable JSON output."`
}

type contractShowDraftCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to inspect."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type contractAcceptDraftCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id whose latest draft proposal should be accepted."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type contractReviseCmd struct {
	RunID             string   `arg:"" optional:"" name:"run_id" help:"Run id to revise."`
	Goal              string   `name:"goal" help:"Replace the contract goal."`
	AddInScope        []string `name:"add-in-scope" sep:"none" help:"Append an in-scope item."`
	AddOutOfScope     []string `name:"add-out-of-scope" sep:"none" help:"Append an out-of-scope item."`
	AddPathInScope    []string `name:"add-path-in-scope" sep:"none" help:"Append a repo-relative path glob that is in scope."`
	AddPathOutOfScope []string `name:"add-path-out-of-scope" sep:"none" help:"Append a repo-relative path glob that is out of scope."`
	AddAcceptance     []string `name:"add-acceptance" sep:"none" help:"Append an acceptance criterion."`
	AddValidation     []string `name:"add-validation" sep:"none" help:"Append a validation command."`
	AddAssumption     []string `name:"add-assumption" sep:"none" help:"Append an assumption."`
	JSONOutput        bool     `name:"json" help:"Print machine-readable JSON output."`
}

type contractApproveCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to approve."`
	By         string `name:"by" default:"manual" help:"Approver name to record."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type promptBuildCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to build prompt artifacts for."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type promptShowCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to inspect."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type executeDryRunCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to prepare for execution."`
	Agent      string `name:"agent" help:"Built-in agent name. Defaults to codex."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type executeRunCmd struct {
	RunID      string        `arg:"" optional:"" name:"run_id" help:"Run id to execute."`
	Agent      string        `name:"agent" help:"Built-in agent name. Defaults to codex."`
	Timeout    time.Duration `name:"timeout" default:"10m" help:"Maximum idle duration without agent output."`
	Yes        bool          `name:"yes" help:"Skip the interactive confirmation (required in non-interactive use)."`
	JSONOutput bool          `name:"json" help:"Print machine-readable JSON output."`
}

type executeStatusCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to inspect."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type executeShowCmd struct {
	Args       []string `arg:"" optional:"" name:"args" help:"[run_id] [attempt_id]"`
	Logs       bool     `name:"logs" help:"Include bounded stdout/stderr excerpts."`
	JSONOutput bool     `name:"json" help:"Print machine-readable JSON output."`
}

type gateRunCmd struct {
	RunID         string `arg:"" optional:"" name:"run_id" help:"Run id to inspect."`
	AllowCommands bool   `name:"allow-commands" help:"Required safety flag before running validation commands."`
	JSONOutput    bool   `name:"json" help:"Print machine-readable JSON output."`
}

type gateShowCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to inspect."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type reviewPrepareCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to review."`
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

type reviewAddFindingCmd struct {
	Args       []string `arg:"" optional:"" name:"args" help:"[run_id] <message>"`
	Severity   string   `name:"severity" default:"medium" enum:"low,medium,high,critical" help:"Finding severity."`
	Category   string   `name:"category" default:"other" enum:"correctness,scope,quality,validation,process,other" help:"Finding category."`
	File       string   `name:"file" help:"Repo-relative file path."`
	Line       int      `name:"line" help:"Optional line number."`
	Blocking   bool     `name:"blocking" help:"Block review approval until resolved."`
	JSONOutput bool     `name:"json" help:"Print machine-readable JSON output."`
}

type reviewResolveCmd struct {
	Args       []string `arg:"" optional:"" name:"args" help:"[run_id] <finding_id>"`
	Note       string   `name:"note" help:"Resolution note."`
	JSONOutput bool     `name:"json" help:"Print machine-readable JSON output."`
}

type reviewApproveCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to review."`
	By         string `name:"by" default:"manual" help:"Approver name to record."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type reviewDryRunCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to prepare reviewer artifacts for."`
	Reviewer   string `name:"reviewer" help:"Built-in reviewer name. Defaults to the configured reviewer unless cross-model review selects another built-in."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type reviewRunCmd struct {
	RunID      string        `arg:"" optional:"" name:"run_id" help:"Run id to review."`
	Reviewer   string        `name:"reviewer" help:"Built-in reviewer name. Defaults to the configured reviewer unless cross-model review selects another built-in."`
	Timeout    time.Duration `name:"timeout" default:"10m" help:"Maximum idle duration without reviewer output."`
	Yes        bool          `name:"yes" help:"Skip the interactive confirmation (required in non-interactive use)."`
	JSONOutput bool          `name:"json" help:"Print machine-readable JSON output."`
}

type reviewFixCmd struct {
	RunID      string        `arg:"" optional:"" name:"run_id" help:"Run id whose review findings should be fixed."`
	Agent      string        `name:"agent" help:"Built-in fixer agent name. Defaults to codex."`
	Timeout    time.Duration `name:"timeout" default:"10m" help:"Maximum idle duration without fixer output."`
	Yes        bool          `name:"yes" help:"Skip the interactive confirmation (required in non-interactive use)."`
	JSONOutput bool          `name:"json" help:"Print machine-readable JSON output."`
}

type reviewLoopCmd struct {
	RunID       string        `arg:"" optional:"" name:"run_id" help:"Run id to review."`
	Reviewer    string        `name:"reviewer" help:"Built-in reviewer name. Defaults to the configured reviewer unless cross-model review selects another built-in."`
	Agent       string        `name:"agent" help:"Built-in fixer agent name. Defaults to codex."`
	MaxRounds   int           `name:"max-rounds" help:"Maximum review rounds. Defaults to limits.review.max_rounds."`
	Patience    int           `name:"patience" help:"Consecutive no-change fixer rounds before stopping as stalemate. Defaults to limits.review.patience."`
	CleanRounds int           `name:"clean-rounds" help:"Consecutive clean review rounds required before convergence. Defaults to limits.review.clean_rounds."`
	Timeout     time.Duration `name:"timeout" default:"10m" help:"Maximum idle duration without reviewer or fixer output."`
	Yes         bool          `name:"yes" help:"Required confirmation for direct reviewer/fixer execution."`
	JSONOutput  bool          `name:"json" help:"Print machine-readable JSON output."`
}

type reviewProposeFindingsCmd struct {
	Args       []string `arg:"" optional:"" name:"args" help:"[run_id] [reviewer_attempt_id]"`
	JSONOutput bool     `name:"json" help:"Print machine-readable JSON output."`
}

type reviewApplyFixOutcomesCmd struct {
	Args       []string `arg:"" optional:"" name:"args" help:"[run_id] [fixer_attempt_id]"`
	JSONOutput bool     `name:"json" help:"Print machine-readable JSON output."`
}

type reviewAcceptProposalCmd struct {
	Args       []string `arg:"" optional:"" name:"args" help:"[run_id] <proposal_id>"`
	JSONOutput bool     `name:"json" help:"Print machine-readable JSON output."`
}

type reviewRejectProposalCmd struct {
	Args       []string `arg:"" optional:"" name:"args" help:"[run_id] <proposal_id>"`
	Reason     string   `name:"reason" help:"Reason for rejecting the proposal."`
	JSONOutput bool     `name:"json" help:"Print machine-readable JSON output."`
}

type agentsDoctorCmd struct {
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
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type versionCmd struct {
	JSONOutput bool `name:"json" help:"Print machine-readable JSON output."`
}

type searchCmd struct {
	Query      string `arg:"" name:"query" help:"Search query."`
	Limit      int    `help:"Maximum number of results." default:"10"`
	Kind       string `help:"Document kind filter." default:"any" enum:"any,repo_map,llms,wiki,file,code_item,import"`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}
