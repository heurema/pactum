package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/version"
)

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

func (c *usageCmd) Run(r *runner) error {
	if c.All {
		if strings.TrimSpace(c.RunID) != "" {
			return errors.New("usage: pass either a run_id or --all, not both")
		}
		return r.App.UsageAll(r.Stdout, c.JSONOutput)
	}
	runID, ok, err := r.App.resolveRunArgReadOnly(r.Stdout, c.RunID, false, c.JSONOutput)
	if err != nil || !ok {
		return err
	}
	return r.App.Usage(r.Stdout, runID, c.JSONOutput)
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

func (c *clarifySuggestCmd) Run(r *runner) error {
	runID, err := r.App.resolveRunArgMutating(c.RunID, false)
	if err != nil {
		return err
	}
	return r.App.ClarifySuggest(r.Stdout, r.Stderr, runID, c.Reviewer, c.Timeout, c.Yes, c.JSONOutput)
}

func (c *clarifyLoopCmd) Run(r *runner) error {
	runID, err := r.App.resolveRunArgMutating(c.RunID, false)
	if err != nil {
		return err
	}
	return r.App.ClarifyLoop(r.Stdout, r.Stderr, runID, clarifyLoopOptions{
		Reviewer:   c.Reviewer,
		MaxRounds:  c.MaxRounds,
		Timeout:    c.Timeout,
		Yes:        c.Yes,
		JSONOutput: c.JSONOutput,
	})
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

func (c *contractDraftCmd) Run(r *runner) error {
	runID, err := r.App.resolveRunArgMutating(c.RunID, false)
	if err != nil {
		return err
	}
	return r.App.ContractDraft(r.Stdout, r.Stderr, runID, c.Reviewer, c.Timeout, c.Yes, c.JSONOutput)
}

func (c *contractShowDraftCmd) Run(r *runner) error {
	runID, ok, err := r.App.resolveRunArgReadOnly(r.Stdout, c.RunID, false, c.JSONOutput)
	if err != nil || !ok {
		return err
	}
	return r.App.ContractShowDraft(r.Stdout, runID, c.JSONOutput)
}

func (c *contractAcceptDraftCmd) Run(r *runner) error {
	runID, err := r.App.resolveRunArgMutating(c.RunID, false)
	if err != nil {
		return err
	}
	return r.App.ContractAcceptDraft(r.Stdout, runID, c.JSONOutput)
}

func (c *contractReviseCmd) Run(r *runner) error {
	runID, err := r.App.resolveRunArgMutating(c.RunID, false)
	if err != nil {
		return err
	}
	revision := contractRevision{
		Goal:              c.Goal,
		AddInScope:        c.AddInScope,
		AddOutOfScope:     c.AddOutOfScope,
		AddPathInScope:    c.AddPathInScope,
		AddPathOutOfScope: c.AddPathOutOfScope,
		AddAcceptance:     c.AddAcceptance,
		AddValidation:     c.AddValidation,
		AddAssumption:     c.AddAssumption,
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

func (c *reviewApplyFixOutcomesCmd) Run(r *runner) error {
	explicitRun, rest := splitLeadingRunID(c.Args)
	if len(rest) > 1 {
		return errors.New("usage: pactum review apply-fix-outcomes [run_id] [fixer_attempt_id]")
	}
	attemptID := ""
	if len(rest) == 1 {
		attemptID = rest[0]
	}
	runID, err := r.App.resolveRunArgMutating(explicitRun, false)
	if err != nil {
		return err
	}
	return r.App.ReviewApplyFixOutcomes(r.Stdout, runID, attemptID, c.JSONOutput)
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
