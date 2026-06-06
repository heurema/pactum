package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/codeindex"
	"github.com/heurema/pactum/internal/ledger"
	"github.com/heurema/pactum/internal/projectmap"
	searchpkg "github.com/heurema/pactum/internal/search"
	"github.com/heurema/pactum/internal/version"
	"gopkg.in/yaml.v3"
)

const (
	workspaceSchema = "pactum.workspace.v1"
	configSchema    = "pactum.config.v1"
)

type App struct {
	WorkingDir    string
	Now           func() time.Time
	AgentRegistry agents.Registry
}

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
	Version  versionCmd  `cmd:"" help:"Print the Pactum version."`
}

type initCmd struct {
	Path string `arg:"" optional:"" default:"." name:"path" help:"Repository path to initialize."`
}

type mapCmd struct {
	Refresh mapRefreshCmd `cmd:"" help:"Rebuild generated project map artifacts."`
}

type clarifyCmd struct {
	Ask    clarifyAskCmd    `cmd:"" help:"Add a manual clarification question."`
	Answer clarifyAnswerCmd `cmd:"" help:"Record a manual clarification answer."`
	Status clarifyStatusCmd `cmd:"" aliases:"list" help:"Print clarification status for a run (alias: list)."`
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

type clarifyStatusCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to inspect."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type contractCmd struct {
	Show    contractShowCmd    `cmd:"" help:"Show a run contract."`
	Revise  contractReviseCmd  `cmd:"" help:"Revise deterministic contract fields."`
	Approve contractApproveCmd `cmd:"" help:"Approve a run contract."`
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
	Prepare         reviewPrepareCmd         `cmd:"" help:"Prepare manual review artifacts."`
	Status          reviewStatusCmd          `cmd:"" help:"Show manual review status."`
	Show            reviewShowCmd            `cmd:"" help:"Show manual review findings."`
	AddFinding      reviewAddFindingCmd      `cmd:"add-finding" help:"Append a manual review finding."`
	Resolve         reviewResolveCmd         `cmd:"" help:"Resolve a manual review finding."`
	Approve         reviewApproveCmd         `cmd:"" help:"Approve a manual review."`
	DryRun          reviewDryRunCmd          `cmd:"dry-run" help:"Prepare reviewer artifacts without running a reviewer."`
	Run             reviewRunCmd             `cmd:"run" help:"Run a built-in reviewer and capture attempt artifacts."`
	Fix             reviewFixCmd             `cmd:"fix" help:"Run a write-enabled fixer against current review findings."`
	Loop            reviewLoopCmd            `cmd:"loop" help:"Run reviewer/fixer rounds until a clean review round or max rounds."`
	ProposeFindings reviewProposeFindingsCmd `cmd:"propose-findings" help:"Parse reviewer output into pending finding proposals."`
	AcceptProposal  reviewAcceptProposalCmd  `cmd:"accept-proposal" help:"Accept a pending review finding proposal."`
	RejectProposal  reviewRejectProposalCmd  `cmd:"reject-proposal" help:"Reject a pending review finding proposal."`
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

type contractReviseCmd struct {
	RunID         string   `arg:"" optional:"" name:"run_id" help:"Run id to revise."`
	Goal          string   `name:"goal" help:"Replace the contract goal."`
	AddInScope    []string `name:"add-in-scope" sep:"none" help:"Append an in-scope item."`
	AddOutOfScope []string `name:"add-out-of-scope" sep:"none" help:"Append an out-of-scope item."`
	AddAcceptance []string `name:"add-acceptance" sep:"none" help:"Append an acceptance criterion."`
	AddValidation []string `name:"add-validation" sep:"none" help:"Append a validation command."`
	AddAssumption []string `name:"add-assumption" sep:"none" help:"Append an assumption."`
	JSONOutput    bool     `name:"json" help:"Print machine-readable JSON output."`
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
	Timeout    time.Duration `name:"timeout" default:"10m" help:"Maximum duration for the built-in agent process."`
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
	Timeout    time.Duration `name:"timeout" default:"10m" help:"Maximum duration for the reviewer process."`
	Yes        bool          `name:"yes" help:"Skip the interactive confirmation (required in non-interactive use)."`
	JSONOutput bool          `name:"json" help:"Print machine-readable JSON output."`
}

type reviewFixCmd struct {
	RunID      string        `arg:"" optional:"" name:"run_id" help:"Run id whose review findings should be fixed."`
	Agent      string        `name:"agent" help:"Built-in fixer agent name. Defaults to codex."`
	Timeout    time.Duration `name:"timeout" default:"10m" help:"Maximum duration for the fixer process."`
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
	Timeout     time.Duration `name:"timeout" default:"10m" help:"Maximum duration for each reviewer or fixer process."`
	Yes         bool          `name:"yes" help:"Required confirmation for direct reviewer/fixer execution."`
	JSONOutput  bool          `name:"json" help:"Print machine-readable JSON output."`
}

type reviewProposeFindingsCmd struct {
	Args       []string `arg:"" optional:"" name:"args" help:"[run_id] [reviewer_attempt_id]"`
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

type versionCmd struct {
	JSONOutput bool `name:"json" help:"Print machine-readable JSON output."`
}

type searchCmd struct {
	Query      string `arg:"" name:"query" help:"Search query."`
	Limit      int    `help:"Maximum number of results." default:"10"`
	Kind       string `help:"Document kind filter." default:"any" enum:"any,repo_map,llms,wiki,file,code_item,import"`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type runner struct {
	App    App
	Stdout io.Writer
	// Stderr is the operator's stderr. Agent-running commands pass it as the live
	// output writer so the agent's stdout/stderr stream there as it runs, keeping
	// Stdout the clean result channel (human summary or --json).
	Stderr io.Writer
}

type workspaceManifest struct {
	Schema        string    `json:"schema"`
	Tool          string    `json:"tool"`
	ToolVersion   string    `json:"tool_version"`
	RepoRoot      string    `json:"repo_root"`
	InitializedAt time.Time `json:"initialized_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Map           struct {
		CurrentRunID string `json:"current_run_id"`
	} `json:"map"`
	Status string `json:"status"`
}

type configFile struct {
	Schema         string             `yaml:"schema"`
	DefaultProfile string             `yaml:"default_profile"`
	ProjectMap     projectMapConfig   `yaml:"project_map"`
	Agents         agents.AgentConfig `yaml:"agents,omitempty"`
	Limits         limitsConfig       `yaml:"limits"`
	Budget         budgetConfig       `yaml:"budget"`
	Memory         memoryConfig       `yaml:"memory"`
}

type projectMapConfig struct {
	Refresh      string `yaml:"refresh"`
	MaxFileBytes int    `yaml:"max_file_bytes"`
	CodeIndex    string `yaml:"code_index"`
}

type limitsConfig struct {
	Clarify iterationLimits `yaml:"clarify"`
	Execute executeLimits   `yaml:"execute"`
	Review  reviewLimits    `yaml:"review"`
}

type iterationLimits struct {
	MaxIterations        int `yaml:"max_iterations"`
	MaxQuestionsPerRound int `yaml:"max_questions_per_round"`
}

type executeLimits struct {
	MaxIterations int `yaml:"max_iterations"`
}

type reviewLimits struct {
	MaxRounds   int `yaml:"max_rounds"`
	Patience    int `yaml:"patience"`
	CleanRounds int `yaml:"clean_rounds"`
}

type budgetConfig struct {
	Mode   string   `yaml:"mode"`
	MaxUSD *float64 `yaml:"max_usd"`
}

type memoryConfig struct {
	Enabled      bool   `yaml:"enabled"`
	IncludeStale string `yaml:"include_stale"`
}

func Run(args []string, stdout, stderr io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "pactum: %v\n", err)
		return 1
	}
	return App{WorkingDir: wd, Now: time.Now}.Run(args, stdout, stderr)
}

// kongExit carries the exit code kong requests (for example from --help) out
// through a panic so App.Run can return it instead of letting kong terminate the
// process with os.Exit. This keeps help/parse exits testable in-process and lets
// Pactum be embedded without the parser killing the host process.
type kongExit struct{ code int }

func (a App) Run(args []string, stdout, stderr io.Writer) (code int) {
	if a.Now == nil {
		a.Now = time.Now
	}
	if a.WorkingDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "pactum: %v\n", err)
			return 1
		}
		a.WorkingDir = wd
	}

	defer func() {
		if r := recover(); r != nil {
			exit, ok := r.(kongExit)
			if !ok {
				panic(r)
			}
			code = exit.code
		}
	}()

	var command cli
	parser, err := kong.New(
		&command,
		kong.Name("pactum"),
		kong.Description("Contract-first CLI for agentic software work."),
		kong.UsageOnError(),
		kong.Writers(stdout, stderr),
		kong.Exit(func(c int) { panic(kongExit{code: c}) }),
	)
	if err != nil {
		fmt.Fprintf(stderr, "pactum: %v\n", err)
		return 1
	}
	ctx, err := parser.Parse(args)
	if err != nil {
		// Do not exit silently: report the parse error and a help pointer on
		// stderr, keeping stdout clean for scripts. Parser errors exit 2.
		fmt.Fprintf(stderr, "pactum: %v\n", err)
		fmt.Fprintln(stderr, "Run 'pactum --help' for usage.")
		return 2
	}
	if err := ctx.Run(&runner{App: a, Stdout: stdout, Stderr: stderr}); err != nil {
		// Command errors exit 1. With --json, emit a machine-readable error
		// envelope on stdout (stderr stays empty); otherwise a human line on
		// stderr.
		if jsonRequested(args) {
			if encErr := writeErrorEnvelope(stdout, err); encErr == nil {
				return 1
			}
		}
		fmt.Fprintf(stderr, "pactum %s: %v\n", ctx.Command(), err)
		return 1
	}
	return 0
}

func (c *initCmd) Run(r *runner) error {
	root, err := r.App.resolveInitRoot(c.Path)
	if err != nil {
		return err
	}
	if err := r.App.Init(root); err != nil {
		return err
	}
	fmt.Fprintf(r.Stdout, "Initialized Pactum workspace at %s\n", artifacts.New(root).Workspace)
	return nil
}

func (c *statusCmd) Run(r *runner) error {
	return r.App.Status(r.Stdout, c.JSONOutput)
}

func (c *versionCmd) Run(r *runner) error {
	if c.JSONOutput {
		return writeJSONResponse(r.Stdout, version.Current())
	}
	info := version.Current()
	fmt.Fprintln(r.Stdout, "Pactum version")
	fmt.Fprintf(r.Stdout, "  version: %s\n", info.Version)
	fmt.Fprintf(r.Stdout, "  commit: %s\n", info.Commit)
	fmt.Fprintf(r.Stdout, "  date: %s\n", info.Date)
	return nil
}

// ensureInitialized returns errNotInitialized when no workspace exists. Mutating
// commands without a run id argument use it so they exit 1 (not 0) before init.
func (a App) ensureInitialized() error {
	_, workspace, err := a.resolveStatusRoot()
	if err != nil {
		return err
	}
	if workspace == "" {
		return errNotInitialized
	}
	return nil
}

func (c *clarifyAskCmd) Run(r *runner) error {
	explicitRun, rest := splitLeadingRunID(c.Args)
	if len(rest) != 1 {
		return errors.New("usage: pactum clarify ask [run_id] <question>")
	}
	runID, err := r.App.resolveRunArgMutating(explicitRun, false)
	if err != nil {
		return err
	}
	return r.App.ClarifyAsk(r.Stdout, runID, rest[0], c.Blocking, c.JSONOutput)
}

func (c *clarifyAnswerCmd) Run(r *runner) error {
	explicitRun, rest := splitLeadingRunID(c.Args)
	if len(rest) != 2 {
		return errors.New("usage: pactum clarify answer [run_id] <question_id> <answer>")
	}
	questionID, answer := rest[0], rest[1]
	if !strings.HasPrefix(questionID, "q_") {
		return fmt.Errorf("expected a question id (q_...), got %q", questionID)
	}
	runID, err := r.App.resolveRunArgMutating(explicitRun, false)
	if err != nil {
		return err
	}
	return r.App.ClarifyAnswer(r.Stdout, runID, questionID, answer, c.JSONOutput)
}

func (c *clarifyStatusCmd) Run(r *runner) error {
	runID, ok, err := r.App.resolveRunArgReadOnly(r.Stdout, c.RunID, false, c.JSONOutput)
	if err != nil || !ok {
		return err
	}
	return r.App.ClarifyStatus(r.Stdout, runID, c.JSONOutput)
}

func (c *contractShowCmd) Run(r *runner) error {
	runID, ok, err := r.App.resolveRunArgReadOnly(r.Stdout, c.RunID, false, c.JSONOutput)
	if err != nil || !ok {
		return err
	}
	return r.App.ContractShow(r.Stdout, runID, c.JSONOutput)
}

func (c *contractReviseCmd) Run(r *runner) error {
	runID, err := r.App.resolveRunArgMutating(c.RunID, false)
	if err != nil {
		return err
	}
	revision := contractRevision{
		Goal:          c.Goal,
		AddInScope:    c.AddInScope,
		AddOutOfScope: c.AddOutOfScope,
		AddAcceptance: c.AddAcceptance,
		AddValidation: c.AddValidation,
		AddAssumption: c.AddAssumption,
	}
	return r.App.ContractRevise(r.Stdout, runID, revision, c.JSONOutput)
}

func (c *contractApproveCmd) Run(r *runner) error {
	runID, err := r.App.resolveRunArgMutating(c.RunID, false)
	if err != nil {
		return err
	}
	return r.App.ContractApprove(r.Stdout, runID, c.By, c.JSONOutput)
}

func (c *promptBuildCmd) Run(r *runner) error {
	runID, err := r.App.resolveRunArgMutating(c.RunID, false)
	if err != nil {
		return err
	}
	return r.App.PromptBuild(r.Stdout, runID, c.JSONOutput)
}

func (c *promptShowCmd) Run(r *runner) error {
	runID, ok, err := r.App.resolveRunArgReadOnly(r.Stdout, c.RunID, false, c.JSONOutput)
	if err != nil || !ok {
		return err
	}
	return r.App.PromptShow(r.Stdout, runID, c.JSONOutput)
}

func (c *executeDryRunCmd) Run(r *runner) error {
	runID, err := r.App.resolveRunArgMutating(c.RunID, false)
	if err != nil {
		return err
	}
	return r.App.ExecuteDryRun(r.Stdout, runID, c.Agent, c.JSONOutput)
}

func (c *executeRunCmd) Run(r *runner) error {
	runID, err := r.App.resolveRunArgMutating(c.RunID, false)
	if err != nil {
		return err
	}
	return r.App.ExecuteRun(r.Stdout, r.Stderr, runID, c.Agent, c.Timeout, c.Yes, c.JSONOutput)
}

func (c *executeStatusCmd) Run(r *runner) error {
	runID, ok, err := r.App.resolveRunArgReadOnly(r.Stdout, c.RunID, false, c.JSONOutput)
	if err != nil || !ok {
		return err
	}
	return r.App.ExecuteStatus(r.Stdout, runID, c.JSONOutput)
}

func (c *executeShowCmd) Run(r *runner) error {
	explicitRun, rest := splitLeadingRunID(c.Args)
	if len(rest) > 1 {
		return errors.New("usage: pactum execute show [run_id] [attempt_id]")
	}
	attemptID := ""
	if len(rest) == 1 {
		attemptID = rest[0]
	}
	runID, ok, err := r.App.resolveRunArgReadOnly(r.Stdout, explicitRun, false, c.JSONOutput)
	if err != nil || !ok {
		return err
	}
	return r.App.ExecuteShow(r.Stdout, runID, attemptID, c.Logs, c.JSONOutput)
}

func (c *gateRunCmd) Run(r *runner) error {
	runID, err := r.App.resolveRunArgMutating(c.RunID, false)
	if err != nil {
		return err
	}
	return r.App.GateRun(r.Stdout, runID, c.AllowCommands, c.JSONOutput)
}

func (c *gateShowCmd) Run(r *runner) error {
	runID, ok, err := r.App.resolveRunArgReadOnly(r.Stdout, c.RunID, false, c.JSONOutput)
	if err != nil || !ok {
		return err
	}
	return r.App.GateShow(r.Stdout, runID, c.JSONOutput)
}

func (c *reviewPrepareCmd) Run(r *runner) error {
	runID, err := r.App.resolveRunArgMutating(c.RunID, false)
	if err != nil {
		return err
	}
	return r.App.ReviewPrepare(r.Stdout, runID, c.JSONOutput)
}

func (c *reviewStatusCmd) Run(r *runner) error {
	runID, ok, err := r.App.resolveRunArgReadOnly(r.Stdout, c.RunID, false, c.JSONOutput)
	if err != nil || !ok {
		return err
	}
	return r.App.ReviewStatus(r.Stdout, runID, c.JSONOutput)
}

func (c *reviewShowCmd) Run(r *runner) error {
	runID, ok, err := r.App.resolveRunArgReadOnly(r.Stdout, c.RunID, false, c.JSONOutput)
	if err != nil || !ok {
		return err
	}
	return r.App.ReviewShow(r.Stdout, runID, c.JSONOutput)
}

func (c *reviewAddFindingCmd) Run(r *runner) error {
	explicitRun, rest := splitLeadingRunID(c.Args)
	if len(rest) != 1 {
		return errors.New("usage: pactum review add-finding [run_id] <message>")
	}
	runID, err := r.App.resolveRunArgMutating(explicitRun, false)
	if err != nil {
		return err
	}
	finding := reviewFindingInput{
		Message:  rest[0],
		Severity: c.Severity,
		Category: c.Category,
		File:     c.File,
		Line:     c.Line,
		Blocking: c.Blocking,
	}
	return r.App.ReviewAddFinding(r.Stdout, runID, finding, c.JSONOutput)
}

func (c *reviewResolveCmd) Run(r *runner) error {
	explicitRun, rest := splitLeadingRunID(c.Args)
	if len(rest) != 1 {
		return errors.New("usage: pactum review resolve [run_id] <finding_id>")
	}
	findingID := rest[0]
	if !strings.HasPrefix(findingID, "f_") {
		return fmt.Errorf("expected a finding id (f_...), got %q", findingID)
	}
	runID, err := r.App.resolveRunArgMutating(explicitRun, false)
	if err != nil {
		return err
	}
	return r.App.ReviewResolve(r.Stdout, runID, findingID, c.Note, c.JSONOutput)
}

func (c *reviewApproveCmd) Run(r *runner) error {
	runID, err := r.App.resolveRunArgMutating(c.RunID, false)
	if err != nil {
		return err
	}
	return r.App.ReviewApprove(r.Stdout, runID, c.By, c.JSONOutput)
}

func (c *reviewDryRunCmd) Run(r *runner) error {
	runID, err := r.App.resolveRunArgMutating(c.RunID, false)
	if err != nil {
		return err
	}
	return r.App.ReviewDryRun(r.Stdout, runID, c.Reviewer, c.JSONOutput)
}

func (c *reviewRunCmd) Run(r *runner) error {
	runID, err := r.App.resolveRunArgMutating(c.RunID, false)
	if err != nil {
		return err
	}
	return r.App.ReviewRun(r.Stdout, r.Stderr, runID, c.Reviewer, c.Timeout, c.Yes, c.JSONOutput)
}

func (c *reviewFixCmd) Run(r *runner) error {
	runID, err := r.App.resolveRunArgMutating(c.RunID, false)
	if err != nil {
		return err
	}
	return r.App.ReviewFix(r.Stdout, r.Stderr, runID, c.Agent, c.Timeout, c.Yes, c.JSONOutput)
}

func (c *reviewLoopCmd) Run(r *runner) error {
	runID, err := r.App.resolveRunArgMutating(c.RunID, false)
	if err != nil {
		return err
	}
	return r.App.ReviewLoop(r.Stdout, r.Stderr, runID, reviewLoopOptions{
		Reviewer:    c.Reviewer,
		Agent:       c.Agent,
		MaxRounds:   c.MaxRounds,
		Patience:    c.Patience,
		CleanRounds: c.CleanRounds,
		Timeout:     c.Timeout,
		Yes:         c.Yes,
		JSONOutput:  c.JSONOutput,
	})
}

func (c *reviewProposeFindingsCmd) Run(r *runner) error {
	explicitRun, rest := splitLeadingRunID(c.Args)
	if len(rest) > 1 {
		return errors.New("usage: pactum review propose-findings [run_id] [reviewer_attempt_id]")
	}
	attemptID := ""
	if len(rest) == 1 {
		attemptID = rest[0]
	}
	runID, err := r.App.resolveRunArgMutating(explicitRun, false)
	if err != nil {
		return err
	}
	return r.App.ReviewProposeFindings(r.Stdout, runID, attemptID, c.JSONOutput)
}

func (c *reviewAcceptProposalCmd) Run(r *runner) error {
	explicitRun, rest := splitLeadingRunID(c.Args)
	if len(rest) != 1 {
		return errors.New("usage: pactum review accept-proposal [run_id] <proposal_id>")
	}
	proposalID := rest[0]
	if !strings.HasPrefix(proposalID, "p_") {
		return fmt.Errorf("expected a proposal id (p_...), got %q", proposalID)
	}
	runID, err := r.App.resolveRunArgMutating(explicitRun, false)
	if err != nil {
		return err
	}
	return r.App.ReviewAcceptProposal(r.Stdout, runID, proposalID, c.JSONOutput)
}

func (c *reviewRejectProposalCmd) Run(r *runner) error {
	explicitRun, rest := splitLeadingRunID(c.Args)
	if len(rest) != 1 {
		return errors.New("usage: pactum review reject-proposal [run_id] <proposal_id>")
	}
	proposalID := rest[0]
	if !strings.HasPrefix(proposalID, "p_") {
		return fmt.Errorf("expected a proposal id (p_...), got %q", proposalID)
	}
	runID, err := r.App.resolveRunArgMutating(explicitRun, false)
	if err != nil {
		return err
	}
	return r.App.ReviewRejectProposal(r.Stdout, runID, proposalID, c.Reason, c.JSONOutput)
}

func (c *agentsDoctorCmd) Run(r *runner) error {
	return r.App.AgentsDoctor(r.Stdout, c.Agent, c.JSONOutput)
}

func (c *memoryProposeCmd) Run(r *runner) error {
	runID, err := r.App.resolveRunArgMutating(c.RunID, false)
	if err != nil {
		return err
	}
	return r.App.MemoryPropose(r.Stdout, runID, c.JSONOutput)
}

func (c *memoryShowCmd) Run(r *runner) error {
	runID, ok, err := r.App.resolveRunArgReadOnly(r.Stdout, c.RunID, false, c.JSONOutput)
	if err != nil || !ok {
		return err
	}
	return r.App.MemoryShow(r.Stdout, runID, c.JSONOutput)
}

func (c *memoryAcceptCmd) Run(r *runner) error {
	runID, err := r.App.resolveRunArgMutating(c.RunID, false)
	if err != nil {
		return err
	}
	return r.App.MemoryAccept(r.Stdout, runID, c.By, c.JSONOutput)
}

func (c *memorySearchCmd) Run(r *runner) error {
	return r.App.MemorySearch(r.Stdout, c.Query, c.Limit, c.JSONOutput)
}

func (c *memoryRefreshCmd) Run(r *runner) error {
	if err := r.App.ensureInitialized(); err != nil {
		return err
	}
	return r.App.MemoryRefresh(r.Stdout, c.JSONOutput)
}

func (c *memoryStaleCmd) Run(r *runner) error {
	return r.App.MemoryStale(r.Stdout, c.JSONOutput)
}

func (c *searchCmd) Run(r *runner) error {
	return r.App.Search(r.Stdout, c.Query, c.Limit, c.Kind, c.JSONOutput)
}

func (c *mapRefreshCmd) Run(r *runner) error {
	root, workspace, err := r.App.resolveStatusRoot()
	if err != nil {
		return err
	}
	if workspace == "" {
		return errNotInitialized
	}
	result, err := r.App.RefreshMap(root)
	if err != nil {
		return err
	}
	if c.JSONOutput {
		return writeJSONResponse(r.Stdout, result)
	}
	writeMapRefreshResult(r.Stdout, result)
	return nil
}

func (a App) Init(root string) error {
	if err := projectmap.ValidateRoot(root); err != nil {
		return err
	}
	paths := artifacts.New(root)
	if err := ensureDirs(paths.Dirs()); err != nil {
		return err
	}

	now := a.nowUTC()
	runID := "map_" + now.Format("20060102_150405")

	if err := ledger.Append(paths.EventsJSONL, ledger.Event{Type: "init_started", Timestamp: now, RunID: runID, RepoRoot: root}); err != nil {
		return err
	}
	if err := writeStaticWorkspaceFiles(paths); err != nil {
		return err
	}
	manifest := workspaceManifest{
		Schema:        workspaceSchema,
		Tool:          artifacts.ToolName,
		ToolVersion:   version.Version,
		RepoRoot:      ".",
		InitializedAt: now,
		UpdatedAt:     now,
		Status:        "initialized",
	}
	if err := writeJSON(paths.Manifest, manifest); err != nil {
		return err
	}
	result, err := a.refreshMap(root, now)
	if err != nil {
		return err
	}
	return ledger.Append(paths.EventsJSONL, ledger.Event{Type: "init_finished", Timestamp: result.FinishedAt, RunID: result.RunID, RepoRoot: root})
}

func (a App) Status(stdout io.Writer, jsonOutput bool) error {
	root, _, ok, err := a.requireWorkspace(stdout, jsonOutput)
	if err != nil || !ok {
		return err
	}

	report, err := a.workspaceStatus(root)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSONResponse(stdout, report)
	}
	writeWorkspaceStatus(stdout, report)
	return nil
}

func (a App) nowUTC() time.Time {
	if a.Now == nil {
		return time.Now().UTC()
	}
	return a.Now().UTC()
}

func writeWorkspaceStatus(stdout io.Writer, report statusResponse) {
	fmt.Fprintln(stdout, "Pactum status")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Repository:")
	fmt.Fprintf(stdout, "  root: %s\n", report.RepoRoot)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Workspace:")
	fmt.Fprintf(stdout, "  path: %s\n", report.Workspace)
	fmt.Fprintln(stdout, "  initialized: yes")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Project map:")
	fmt.Fprintf(stdout, "  status: %s\n", report.ProjectMap.Status)
	fmt.Fprintf(stdout, "  run: %s\n", report.ProjectMap.RunID)
	fmt.Fprintf(stdout, "  files indexed: %d\n", report.ProjectMap.FilesIndexed)
	fmt.Fprintf(stdout, "  code items: %d\n", report.ProjectMap.CodeItems)
	fmt.Fprintf(stdout, "  search index: %s\n", report.ProjectMap.SearchIndex)
	if len(report.ProjectMap.StaleReasons) > 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Stale reasons:")
		for _, reason := range report.ProjectMap.StaleReasons {
			fmt.Fprintf(stdout, "  - %s\n", reason)
		}
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Suggested:")
		fmt.Fprintln(stdout, "  pactum map refresh")
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Runs:")
	fmt.Fprintf(stdout, "  active: %d\n", report.Runs.Active)
	if report.Runs.LatestRunID != "" {
		fmt.Fprintf(stdout, "  latest: %s\n", report.Runs.LatestRunID)
		fmt.Fprintf(stdout, "  latest status: %s\n", report.Runs.LatestStatus)
	}
	if report.Runs.CurrentRunID != "" {
		fmt.Fprintf(stdout, "  current: %s\n", report.Runs.CurrentRunID)
	}
	if report.Runs.NextCommand != "" {
		fmt.Fprintf(stdout, "  next: %s\n", report.Runs.NextCommand)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Memory:")
	fmt.Fprintf(stdout, "  items: %d\n", report.Memory.Items)
	fmt.Fprintf(stdout, "  stale: %d\n", report.Memory.Stale)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Usage:")
	fmt.Fprintf(stdout, "  total tokens: %d\n", report.Usage.TotalTokens)
	fmt.Fprintf(stdout, "  estimated cost: $%.2f\n", report.Usage.EstimatedCostUSD)
}

func (a App) Search(stdout io.Writer, query string, limit int, kind string, jsonOutput bool) error {
	_, paths, ok, err := a.requireWorkspace(stdout, false)
	if err != nil || !ok {
		return err
	}

	response, err := searchpkg.Query(paths.SearchSQLite, searchpkg.QueryOptions{
		Query: query,
		Limit: limit,
		Kind:  kind,
	})
	if err != nil {
		if searchpkg.IsMissingIndex(err) {
			fmt.Fprintln(stdout, "Search index is missing. Run: pactum map refresh")
			return nil
		}
		return err
	}

	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}

	writeSearchResults(stdout, response)
	return nil
}

func (a App) resolveInitRoot(target string) (string, error) {
	path := target
	if !filepath.IsAbs(path) {
		path = filepath.Join(a.WorkingDir, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", err
	}
	if root, ok := findUp(abs, ".git"); ok {
		return root, nil
	}
	return abs, nil
}

func (a App) resolveStatusRoot() (root string, workspace string, err error) {
	start, err := filepath.Abs(a.WorkingDir)
	if err != nil {
		return "", "", err
	}
	if root, ok := findUp(start, ".git"); ok {
		paths := artifacts.New(root)
		if isDir(paths.Workspace) {
			return root, paths.Workspace, nil
		}
		return root, "", nil
	}
	for current := start; ; current = filepath.Dir(current) {
		paths := artifacts.New(current)
		if isDir(paths.Workspace) {
			return current, paths.Workspace, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return start, "", nil
}

// requireWorkspace resolves the repository root and confirms that a Pactum
// workspace has been initialized. When it has not, it emits the standard notice
// (in the requested format) and returns ok=false so callers can return their
// zero value immediately.
func (a App) requireWorkspace(stdout io.Writer, jsonOutput bool) (root string, paths artifacts.Paths, ok bool, err error) {
	root, workspace, err := a.resolveStatusRoot()
	if err != nil {
		return "", artifacts.Paths{}, false, err
	}
	if workspace == "" {
		return "", artifacts.Paths{}, false, notInitialized(stdout, jsonOutput)
	}
	return root, artifacts.New(root), true, nil
}

// notInitialized writes the standard "workspace not initialized" notice. The
// returned error is the encoder error (nil for the text form), so callers can
// return it directly as a graceful exit.
func notInitialized(stdout io.Writer, jsonOutput bool) error {
	if jsonOutput {
		return writeStatusNotInitialized(stdout)
	}
	fmt.Fprintln(stdout, "Pactum is not initialized. Run: pactum init")
	return nil
}

func findUp(start, marker string) (string, bool) {
	for current := start; ; current = filepath.Dir(current) {
		if _, err := os.Stat(filepath.Join(current, marker)); err == nil {
			return current, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
	}
}

func ensureDirs(paths []string) error {
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func writeStaticWorkspaceFiles(paths artifacts.Paths) error {
	if err := writeDefaultConfigIfMissing(paths.Config); err != nil {
		return err
	}
	files := map[string][]byte{
		paths.Gitignore: []byte(strings.TrimSpace(`
locks/
map/
ledger/
cache/
tmp/
runs/*/ledger/
runs/*/execute/
runs/*/review/
`) + "\n"),
		paths.ProjectMemory: []byte("# Project Memory\n\nNo project memory has been extracted yet.\n"),
		paths.MemoryItems:   nil,
		paths.StaleReport:   []byte("{\"stale\":0,\"items\":[]}\n"),
		paths.UsageJSONL:    nil,
		paths.CostJSON:      []byte("{\"total_tokens\":0,\"estimated_cost_usd\":0}\n"),
	}
	for path, content := range files {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func writeDefaultConfigIfMissing(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeYAML(path, defaultConfigFile())
}

func defaultConfigFile() configFile {
	return configFile{
		Schema:         configSchema,
		DefaultProfile: "balanced",
		ProjectMap: projectMapConfig{
			Refresh:      "auto",
			MaxFileBytes: 500000,
			CodeIndex:    codeindex.ModeAuto,
		},
		Limits: limitsConfig{
			Clarify: iterationLimits{
				MaxIterations:        5,
				MaxQuestionsPerRound: 5,
			},
			Execute: executeLimits{
				MaxIterations: 10,
			},
			Review: reviewLimits{
				MaxRounds:   4,
				Patience:    2,
				CleanRounds: 1,
			},
		},
		Budget: budgetConfig{
			Mode:   "warn",
			MaxUSD: nil,
		},
		Memory: memoryConfig{
			Enabled:      true,
			IncludeStale: "warn",
		},
	}
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// writeJSONResponse encodes value as indented JSON to stdout. It is the shared
// formatter for every command's --json output.
func writeJSONResponse(stdout io.Writer, value any) error {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeYAML(path string, value any) error {
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(value); err != nil {
		_ = encoder.Close()
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buffer.Bytes(), 0o644)
}

func readWorkspaceManifest(path string) (workspaceManifest, error) {
	var manifest workspaceManifest
	if err := readJSON(path, &manifest); err != nil {
		return workspaceManifest{}, err
	}
	if manifest.Schema == "" || manifest.RepoRoot == "" {
		return workspaceManifest{}, errors.New("workspace manifest is incomplete")
	}
	return manifest, nil
}

func readConfig(path string) (configFile, error) {
	var config configFile
	data, err := os.ReadFile(path)
	if err != nil {
		return configFile{}, err
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return configFile{}, err
	}
	if config.Schema == "" {
		return configFile{}, errors.New("config is incomplete")
	}
	return config, nil
}

func (a App) agentRegistry() agents.Registry {
	if a.AgentRegistry != nil {
		return a.AgentRegistry
	}
	return agents.BuiltinRegistry{}
}

func readMapManifest(path string) (projectmap.Manifest, error) {
	var manifest projectmap.Manifest
	if err := readJSON(path, &manifest); err != nil {
		return projectmap.Manifest{}, err
	}
	if manifest.RunID == "" {
		return projectmap.Manifest{}, errors.New("project map manifest is incomplete")
	}
	return manifest, nil
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func writeSearchResults(stdout io.Writer, response searchpkg.Response) {
	fmt.Fprintf(stdout, "Search results for: %s\n\n", response.Query)
	if len(response.Results) == 0 {
		fmt.Fprintln(stdout, "No results found.")
		return
	}
	for _, result := range response.Results {
		fmt.Fprintf(stdout, "%d. %s %s\n", result.Rank, result.Kind, result.Path)
		switch result.Kind {
		case searchpkg.KindCodeItem, searchpkg.KindImport:
			fmt.Fprintf(stdout, "   kind: %s\n", result.CodeKind)
			fmt.Fprintf(stdout, "   name: %s\n", result.Title)
			if result.Language != "" {
				fmt.Fprintf(stdout, "   language: %s\n", result.Language)
			}
		case searchpkg.KindFile:
			if result.Language != "" {
				fmt.Fprintf(stdout, "   language: %s\n", result.Language)
			}
			if result.CodeKind != "" {
				fmt.Fprintf(stdout, "   kind: %s\n", result.CodeKind)
			}
		default:
			fmt.Fprintf(stdout, "   title: %s\n", result.Title)
		}
		if result.Rank < len(response.Results) {
			fmt.Fprintln(stdout)
		}
	}
}
