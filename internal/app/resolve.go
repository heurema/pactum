package app

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/heurema/pactum/internal/artifacts"
)

// looksLikeRunID reports whether s is a run id (the run_* prefix). It lets prefix
// routing tell a run id from a question (q_*), finding (f_*), proposal (p_*), or
// free text. Generated run ids are run_YYYYMMDD_HHMMSS, so the prefix is a safe
// discriminator.
func looksLikeRunID(s string) bool {
	return len(s) > len("run_") && strings.HasPrefix(s, "run_")
}

// splitLeadingRunID pops a leading run id from args when present, so commands
// that also take a question/finding/proposal id can accept either
// "<secondary_id> ..." or "<run_id> <secondary_id> ...".
func splitLeadingRunID(args []string) (runID string, rest []string) {
	if len(args) > 0 && looksLikeRunID(args[0]) {
		return args[0], args[1:]
	}
	return "", args
}

// resolveRunID resolves the run a command should act on. Explicit ids win;
// otherwise --latest, then the current-run pointer, then the sole active run.
func (a App) resolveRunID(paths artifacts.Paths, explicit string, latest bool) (string, error) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		return explicit, nil
	}
	if latest {
		id, ok, err := latestRunID(paths)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", errors.New(`no runs found; create one with: pactum task new "<task>"`)
		}
		return id, nil
	}
	if id, ok := readCurrentRun(paths); ok && runExists(paths, id) {
		return id, nil
	}
	active, err := activeRunIDs(paths)
	if err != nil {
		return "", err
	}
	if len(active) == 1 {
		return active[0], nil
	}
	return "", errors.New("run id is required; use pactum task list or pactum task use <run_id>")
}

// resolveRunArgMutating ensures the workspace is initialized (erroring with
// errNotInitialized otherwise, which maps to exit 1) and resolves the run id for
// a mutating command.
func (a App) resolveRunArgMutating(explicit string, latest bool) (string, error) {
	root, workspace, err := a.resolveStatusRoot()
	if err != nil {
		return "", err
	}
	if workspace == "" {
		return "", errNotInitialized
	}
	return a.resolveRunID(artifacts.New(root), explicit, latest)
}

// resolveRunArgReadOnly resolves the run id for a read-only command. When the
// workspace is not initialized it prints the standard notice and returns
// ok=false so the caller exits 0 without doing work (read-only guidance).
func (a App) resolveRunArgReadOnly(stdout io.Writer, explicit string, latest bool, jsonOutput bool) (runID string, ok bool, err error) {
	root, workspace, err := a.resolveStatusRoot()
	if err != nil {
		return "", false, err
	}
	if workspace == "" {
		return "", false, notInitialized(stdout, jsonOutput)
	}
	id, err := a.resolveRunID(artifacts.New(root), explicit, latest)
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

func currentRunPointerPath(paths artifacts.Paths) string {
	return filepath.Join(paths.CacheDir, "current-run")
}

// readCurrentRun reads the local-only current-run pointer.
func readCurrentRun(paths artifacts.Paths) (string, bool) {
	data, err := activeStore.ReadBytes(currentRunPointerPath(paths))
	if err != nil {
		return "", false
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return "", false
	}
	return id, true
}

// writeCurrentRun records the current-run pointer under the (gitignored) cache
// directory. It is local convenience state, never durable/committable.
func writeCurrentRun(paths artifacts.Paths, runID string) error {
	if err := activeStore.MkdirAll(paths.CacheDir); err != nil {
		return err
	}
	return activeStore.WriteBytes(currentRunPointerPath(paths), []byte(runID+"\n"), 0o644)
}

func runExists(paths artifacts.Paths, runID string) bool {
	exists, err := storeDirExists(filepath.Join(paths.RunsDir, runID))
	return err == nil && exists
}

// listRunIDs returns every run id under the runs directory in chronological
// (ascending) order. The id format sorts lexicographically by time.
func listRunIDs(paths artifacts.Paths) ([]string, error) {
	entries, err := activeStore.ReadDir(paths.RunsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	ids := []string{}
	for _, entry := range entries {
		if entry.IsDir() && looksLikeRunID(entry.Name()) {
			ids = append(ids, entry.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func latestRunID(paths artifacts.Paths) (string, bool, error) {
	ids, err := listRunIDs(paths)
	if err != nil {
		return "", false, err
	}
	if len(ids) == 0 {
		return "", false, nil
	}
	return ids[len(ids)-1], true, nil
}

func activeRunIDs(paths artifacts.Paths) ([]string, error) {
	ids, err := listRunIDs(paths)
	if err != nil {
		return nil, err
	}
	active := make([]string, 0, len(ids))
	for _, id := range ids {
		status, ok := readPersistedRunStatus(paths, id)
		if !ok {
			// A run dir reserved by a concurrent `task new` may not have its
			// run.json yet (or it may be mid-write). Such a run is not counted
			// as active until it is fully created.
			continue
		}
		if !isTerminalRunStatus(status) {
			active = append(active, id)
		}
	}
	return active, nil
}

// readPersistedRunStatus reads a run's persisted status. ok is false when the
// run.json is missing or unreadable (for example, a run still being created
// concurrently), so callers can skip it rather than fail.
func readPersistedRunStatus(paths artifacts.Paths, runID string) (status string, ok bool) {
	var state struct {
		Status string `json:"status"`
	}
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	if err := readJSON(runPaths.RunJSON, &state); err != nil {
		return "", false
	}
	return state.Status, true
}

// deriveRunStatus reports a run's furthest-reached lifecycle stage purely from
// on-disk artifacts. It is the primary status source because persisted run.json
// status can lag behind later stages and reset operations.
//
// The lifecycle is walked from the start, stopping at the first boundary that is
// not satisfied. A downstream artifact only counts when every upstream boundary
// is still valid, so resetting an earlier stage (for example, `contract revise`
// clears the approval and prompt manifest) correctly rewinds the reported
// status even though stale gate/review/memory artifacts may still be on disk.
func deriveRunStatus(paths artifacts.Paths, runID string) string {
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	if !approvalApproved(runPaths.ApprovalJSON) {
		return "contract_draft"
	}
	if !isRegularFile(runPaths.PromptManifest) {
		return "contract_approved"
	}
	if !isRegularFile(runPaths.LastResultJSON) {
		return "prompt_built"
	}
	if !isRegularFile(runPaths.GateReportJSON) {
		return "executed"
	}
	if !isRegularFile(runPaths.ReviewJSON) {
		return "gated"
	}
	if !reviewApproved(runPaths.ReviewJSON) {
		return "review_prepared"
	}
	if !isRegularFile(runPaths.MemoryCandidateJSON) {
		return "review_approved"
	}
	if memoryAccepted(runPaths.MemoryAcceptanceJSON) {
		return "memory_accepted"
	}
	return "memory_proposed"
}

func approvalApproved(path string) bool {
	approval, err := readApprovalState(path)
	return err == nil && approval.Status == "approved"
}

func reviewApproved(path string) bool {
	var state struct {
		Status string `json:"status"`
	}
	if err := readJSON(path, &state); err != nil {
		return false
	}
	return state.Status == "approved"
}

func memoryAccepted(path string) bool {
	acceptance, exists, err := readMemoryAcceptance(path)
	return err == nil && exists && acceptance.Status == "accepted"
}

// nextCommandsForRun maps a run's derived lifecycle stage to the concrete
// runnable command(s) an agent would run next — the JSON `next` affordance.
// Stages whose real next step needs a human (answering clarifications,
// running the executor, deciding proposals) point at safe inspection or
// preparation commands instead; a finished run has no next move.
func nextCommandsForRun(paths artifacts.Paths, runID string) []string {
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	switch deriveRunStatus(paths, runID) {
	case "contract_draft":
		// Unreadable clarification records cannot prove approval is legal, so
		// they point at inspection just like open blocking questions.
		if open, err := openBlockingQuestionCount(runPaths); err != nil || open > 0 {
			return []string{"pactum clarify show " + runID}
		}
		// A config read failure means reviewers may be configured but broken;
		// point at contract review so the error surfaces there rather than
		// silently advertising approve.
		configured, err := contractReviewersConfigured(paths.Config)
		if err != nil || configured {
			return []string{"pactum contract review run " + runID}
		}
		// Even without configured reviewers, a prior contract-review run may have
		// left persisted blocking findings — approve would refuse them too. Route to
		// inspection so the guard error surfaces there rather than here.
		if isRegularFile(runPaths.ContractReviewFindingsJSONL) {
			if _, err := checkContractReviewFindingsApprovalGuard(runPaths); err != nil {
				return []string{"pactum contract show " + runID}
			}
		}
		return []string{"pactum contract approve " + runID}
	case "contract_approved":
		return []string{"pactum prompt build " + runID}
	case "prompt_built":
		return []string{"pactum execute plan " + runID}
	case "executed":
		return []string{"pactum gate run " + runID}
	case "gated", "review_prepared":
		// A gated run needs no separate preparation step: the mutating review
		// commands self-scaffold, so both stages share the review affordance.
		return nextReviewCommands(runPaths, runID)
	case "review_approved":
		// Proposals collected after approval re-block memory propose, so the
		// affordance is only legal once every proposal is decided.
		if pending, err := pendingReviewProposalCount(runPaths); err != nil || pending > 0 {
			return []string{"pactum review show " + runID}
		}
		return []string{"pactum memory propose " + runID}
	case "memory_proposed":
		// A stale candidate cannot be accepted; it must be regenerated, which
		// is only legal while the review state would still pass propose.
		if memoryCandidateFresh(runPaths) {
			return []string{"pactum memory accept " + runID}
		}
		if memoryReproposalLegal(runPaths) {
			return []string{"pactum memory propose " + runID}
		}
		return []string{"pactum review show " + runID}
	default:
		return []string{}
	}
}

// contractReviewersConfigured reports whether the workspace config has at least
// one contract reviewer registered.
func contractReviewersConfigured(configPath string) (bool, error) {
	config, err := readConfig(configPath)
	if err != nil {
		return false, err
	}
	return len(config.Pipeline.ContractReview.By) > 0, nil
}

// openBlockingQuestionCount counts open blocking clarification questions. An
// error means the records are unreadable, so the caller cannot prove the
// count is zero.
func openBlockingQuestionCount(runPaths contractRunPathSet) (int, error) {
	questions, err := readClarificationQuestions(runPaths.QuestionsJSONL)
	if err != nil {
		return 0, err
	}
	answers, err := readClarificationAnswers(runPaths.AnswersJSONL)
	if err != nil {
		return 0, err
	}
	answered := latestAnswersByQuestion(answers)
	open := 0
	for _, question := range questions {
		if _, ok := answered[question.ID]; !ok && question.Blocking {
			open++
		}
	}
	return open, nil
}

// pendingReviewProposalCount counts undecided review proposals.
func pendingReviewProposalCount(runPaths contractRunPathSet) (int, error) {
	proposals, decisions, err := readReviewProposalRecords(runPaths)
	if err != nil {
		return 0, err
	}
	return summarizeReviewProposals(buildReviewProposalViews(proposals, decisions)).Pending, nil
}

// memoryReproposalLegal reports whether `pactum memory propose` would pass
// its review preconditions against the current state: the review is still
// approved and every proposal is decided. Unreadable records cannot prove
// legality.
func memoryReproposalLegal(runPaths contractRunPathSet) bool {
	if !reviewApproved(runPaths.ReviewJSON) {
		return false
	}
	pending, err := pendingReviewProposalCount(runPaths)
	return err == nil && pending == 0
}

// nextReviewCommands picks the review affordance for a gated run: approval is
// advertised only when the gate did not fail, a completed findings artifact can
// be read, every proposal is decided, and no finding remains open. Until then
// the safe move is inspecting the review.
func nextReviewCommands(runPaths contractRunPathSet, runID string) []string {
	inspect := []string{"pactum review show " + runID}
	gateReport, err := readReviewGateReport(runPaths.GateReportJSON)
	if err != nil || gateReport.Status == "failed" {
		return inspect
	}
	if !isRegularFile(runPaths.ReviewFindingsJSONL) {
		return inspect
	}
	findings, resolutions, err := readReviewRecords(runPaths)
	if err != nil {
		return inspect
	}
	pending, err := pendingReviewProposalCount(runPaths)
	if err != nil {
		return inspect
	}
	summary := summarizeReview(findings, resolutions)
	if pending > 0 || summary.Open > 0 || !reviewFindingsCompletedForApproval(runPaths, findings) {
		return inspect
	}
	return []string{"pactum review approve " + runID}
}

func reviewFindingsCompletedForApproval(runPaths contractRunPathSet, findings []reviewFindingRecord) bool {
	if isRegularFile(runPaths.ReviewLoopSummaryJSON) {
		var loopSummary reviewLoopSummaryDocument
		if err := readJSON(runPaths.ReviewLoopSummaryJSON, &loopSummary); err != nil {
			return false
		}
		if loopSummary.OpenBlockingCount > 0 {
			return false
		}
		if len(findings) == 0 {
			return loopSummary.TerminalReason == "clean_round" || loopSummary.TerminalReason == "resolved"
		}
		return true
	}
	return len(findings) > 0
}

// legacyNextCommand keeps next_command's historical bare-command shape while
// deriving the decision from the guarded JSON `next` selector.
func legacyNextCommand(commands []string, runID string) string {
	if len(commands) == 0 {
		return ""
	}
	return strings.TrimSuffix(commands[0], " "+runID)
}

// nextCommandForStatus maps a derived lifecycle status to the command a user
// would typically run next.
func nextCommandForStatus(paths artifacts.Paths, runID string, status string) string {
	switch status {
	case "contract_draft":
		return "pactum contract revise"
	case "contract_approved":
		return "pactum prompt build"
	case "prompt_built":
		return "pactum execute plan"
	case "executed":
		return "pactum gate run"
	case "gated", "review_prepared":
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		return legacyNextCommand(nextReviewCommands(runPaths, runID), runID)
	case "review_approved":
		return "pactum memory propose"
	case "memory_proposed":
		// Same staleness gate as nextCommandsForRun: a stale candidate cannot
		// be accepted, only regenerated while the review state would still
		// pass propose.
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		if memoryCandidateFresh(runPaths) {
			return "pactum memory accept"
		}
		if memoryReproposalLegal(runPaths) {
			return "pactum memory propose"
		}
		return "pactum review show"
	default:
		return ""
	}
}
