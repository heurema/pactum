package app

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/ledger"
)

const promptManifestSchema = "pactum.executor_prompt.v1"

type promptManifest struct {
	Schema         string                  `json:"schema"`
	RunID          string                  `json:"run_id"`
	BuiltAt        string                  `json:"built_at"`
	Status         string                  `json:"status"`
	ContractSHA256 string                  `json:"contract_sha256"`
	ApprovalStatus string                  `json:"approval_status"`
	ApprovedBy     string                  `json:"approved_by"`
	ApprovedAt     string                  `json:"approved_at"`
	MapRunID       string                  `json:"map_run_id"`
	Artifacts      promptManifestArtifacts `json:"artifacts"`
	Memory         *promptManifestMemory   `json:"memory"`
	Checks         promptManifestChecks    `json:"checks"`
}

type promptManifestArtifacts struct {
	Prompt          string `json:"prompt"`
	ExecutorContext string `json:"executor_context"`
	Contract        string `json:"contract"`
	Approval        string `json:"approval"`
	RepoContext     string `json:"repo_context"`
	SearchResults   string `json:"search_results"`
}

type promptManifestMemory struct {
	Context         string                       `json:"context"`
	Selection       string                       `json:"selection"`
	ContextSHA256   string                       `json:"context_sha256"`
	SelectionSHA256 string                       `json:"selection_sha256"`
	SourceSHA256    string                       `json:"source_sha256"`
	Selected        promptManifestMemorySelected `json:"selected"`
	LatestRefresh   *promptManifestMemoryRefresh `json:"latest_refresh"`
}

type promptManifestMemorySelected struct {
	Total   int `json:"total"`
	Fresh   int `json:"fresh"`
	Stale   int `json:"stale"`
	Unknown int `json:"unknown"`
}

type promptManifestMemoryRefresh struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
}

type promptManifestChecks struct {
	ContractApproved            bool `json:"contract_approved"`
	ContractHashMatchesApproval bool `json:"contract_hash_matches_approval"`
	ProjectMapFresh             bool `json:"project_map_fresh"`
	BlockingClarificationsOpen  int  `json:"blocking_clarifications_open"`
	MemoryContextReady          bool `json:"memory_context_ready"`
	MemorySourceCurrent         bool `json:"memory_source_current"`
}

type promptBuildResponse struct {
	RunID     string         `json:"run_id"`
	RunStatus string         `json:"run_status"`
	Manifest  promptManifest `json:"manifest"`
}

type promptShowResponse struct {
	RunID     string         `json:"run_id"`
	RunStatus string         `json:"run_status"`
	Manifest  promptManifest `json:"manifest"`
	Prompt    string         `json:"prompt"`
}

type promptShowNotBuiltResponse struct {
	RunID            string `json:"run_id"`
	RunStatus        string `json:"run_status"`
	Ready            bool   `json:"ready"`
	Message          string `json:"message"`
	SuggestedCommand string `json:"suggested_command"`
}

func (a App) PromptBuild(stdout io.Writer, runID string, jsonOutput bool) error {
	context, ok, err := a.loadContractContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}

	status, err := buildClarificationStatus(context.RunPaths, context.State)
	if err != nil {
		return err
	}
	if status.BlockingOpen > 0 {
		return fmt.Errorf("cannot build executor prompt: blocking clarification questions remain")
	}
	hash, err := verifyApprovedContract(context.RunPaths, context.Contract, context.Approval, "build executor prompt")
	if err != nil {
		return err
	}

	report, err := a.workspaceStatus(context.Root)
	if err != nil {
		return err
	}
	if report.ProjectMap.Status != "fresh" {
		if !jsonOutput {
			fmt.Fprintln(stdout, "Project map is stale")
			fmt.Fprintln(stdout)
			fmt.Fprintln(stdout, "Suggested:")
			fmt.Fprintln(stdout, "  pactum map refresh")
		}
		return fmt.Errorf("cannot build executor prompt: project map is stale")
	}

	now := a.nowUTC()
	selection, err := writeAcceptedMemoryContext(context.Paths, context.RunPaths, runID, memoryQueryFromContract(context.Contract), "contract", defaultMemorySelectionLimit, now)
	if err != nil {
		return err
	}
	memory, err := buildPromptManifestMemory(context.Paths, context.RunPaths, selection)
	if err != nil {
		return err
	}
	manifest := buildPromptManifest(context, hash, report.ProjectMap.RunID, now, memory)
	// Refresh the run's search context from the approved contract: clarification
	// and revision usually make the work more precise than the initial task
	// sentence, so re-derive targeted queries from goal/scope/acceptance/validation.
	searchResults := buildRunSearchResults(context.Paths, report.ProjectMap, "contract", memoryQueryFromContract(context.Contract))
	if err := os.WriteFile(context.RunPaths.SearchResults, mustMarshalJSON(searchResults), 0o644); err != nil {
		return err
	}
	decisions, err := readClarificationDecisions(context.RunPaths.DecisionsJSONL)
	if err != nil {
		return err
	}

	if err := os.WriteFile(context.RunPaths.ExecutorContext, renderExecutorContext(context.State, report.ProjectMap.RunID, hash, searchResults, decisions, memory.Selected), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(context.RunPaths.PromptMD, renderApprovedPromptMD(context.Contract, context.State.RunID, hash, selection), 0o644); err != nil {
		return err
	}
	if err := writeJSON(context.RunPaths.PromptManifest, manifest); err != nil {
		return err
	}
	if err := ensurePromptArtifactRefs(context.RunPaths, context.State); err != nil {
		return err
	}
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "executor_prompt_built", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	response := promptBuildResponse{RunID: runID, RunStatus: context.State.Status, Manifest: manifest}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writePromptBuild(stdout, response)
	return nil
}

func (a App) PromptShow(stdout io.Writer, runID string, jsonOutput bool) error {
	context, ok, err := a.loadContractContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	if !isRegularFile(context.RunPaths.PromptManifest) {
		message := "Executor prompt has not been built. Run: pactum prompt build " + runID
		if jsonOutput {
			return writeJSONResponse(stdout, promptShowNotBuiltResponse{
				RunID:            runID,
				RunStatus:        context.State.Status,
				Ready:            false,
				Message:          message,
				SuggestedCommand: "pactum prompt build " + runID,
			})
		}
		fmt.Fprintf(stdout, "Executor prompt has not been built. Run: pactum prompt build %s\n", runID)
		return nil
	}

	manifest, err := readPromptManifest(context.RunPaths.PromptManifest)
	if err != nil {
		return err
	}
	prompt, err := os.ReadFile(context.RunPaths.PromptMD)
	if err != nil {
		return err
	}
	response := promptShowResponse{
		RunID:     runID,
		RunStatus: context.State.Status,
		Manifest:  manifest,
		Prompt:    string(prompt),
	}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writePromptShow(stdout, response)
	return nil
}

func buildPromptManifest(context runContext, contractSHA256 string, mapRunID string, builtAt time.Time, memory promptManifestMemory) promptManifest {
	approvedBy := ""
	if context.Approval.ApprovedBy != nil {
		approvedBy = *context.Approval.ApprovedBy
	}
	approvedAt := ""
	if context.Approval.ApprovedAt != nil {
		approvedAt = *context.Approval.ApprovedAt
	}
	memoryCopy := memory
	return promptManifest{
		Schema:         promptManifestSchema,
		RunID:          context.State.RunID,
		BuiltAt:        builtAt.Format(time.RFC3339),
		Status:         "ready",
		ContractSHA256: contractSHA256,
		ApprovalStatus: "approved",
		ApprovedBy:     approvedBy,
		ApprovedAt:     approvedAt,
		MapRunID:       mapRunID,
		Artifacts: promptManifestArtifacts{
			Prompt:          "contract/prompt.md",
			ExecutorContext: "context/executor-context.md",
			Contract:        "contract/contract.json",
			Approval:        "contract/approval.json",
			RepoContext:     "context/repo-context.md",
			SearchResults:   "context/search-results.json",
		},
		Memory: &memoryCopy,
		Checks: promptManifestChecks{
			ContractApproved:            true,
			ContractHashMatchesApproval: true,
			ProjectMapFresh:             true,
			BlockingClarificationsOpen:  0,
			MemoryContextReady:          true,
			MemorySourceCurrent:         true,
		},
	}
}

// buildPromptManifestMemory computes the memory boundary metadata recorded in
// the prompt manifest. It hashes the run-local memory artifacts and the global
// accepted-memory source files so execution can detect drift after prompt build.
func buildPromptManifestMemory(paths artifacts.Paths, runPaths contractRunPathSet, selection memorySelectionDocument) (promptManifestMemory, error) {
	contextHash, err := fileSHA256(runPaths.MemoryContextMD)
	if err != nil {
		return promptManifestMemory{}, err
	}
	selectionHash, err := fileSHA256(runPaths.MemorySelectionJSON)
	if err != nil {
		return promptManifestMemory{}, err
	}
	sourceHash, err := memorySourceSHA256(paths)
	if err != nil {
		return promptManifestMemory{}, err
	}
	memory := promptManifestMemory{
		Context:         "context/memory-context.md",
		Selection:       "context/memory-selection.json",
		ContextSHA256:   contextHash,
		SelectionSHA256: selectionHash,
		SourceSHA256:    sourceHash,
		Selected:        memorySelectionCounts(selection),
	}
	refresh, ok, err := latestMemoryRefresh(paths.MemoryRefreshes)
	if err != nil {
		return promptManifestMemory{}, err
	}
	if ok {
		memory.LatestRefresh = &promptManifestMemoryRefresh{ID: refresh.ID, CreatedAt: refresh.CreatedAt}
	}
	return memory, nil
}

func readPromptManifest(path string) (promptManifest, error) {
	var manifest promptManifest
	return manifest, readJSON(path, &manifest)
}

func ensurePromptArtifactRefs(runPaths contractRunPathSet, state contractRunState) error {
	updated := false
	if state.Artifacts.ExecutorContext == "" {
		state.Artifacts.ExecutorContext = "context/executor-context.md"
		updated = true
	}
	if state.Artifacts.PromptManifest == "" {
		state.Artifacts.PromptManifest = "contract/prompt-manifest.json"
		updated = true
	}
	if !updated {
		return nil
	}
	return writeJSON(runPaths.RunJSON, state)
}

func removePromptReadinessArtifacts(runPaths contractRunPathSet) error {
	if err := os.Remove(runPaths.PromptManifest); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func renderExecutorContext(state contractRunState, mapRunID string, contractSHA256 string, searchResults runSearchResults, decisions []clarificationDecisionRecord, memory promptManifestMemorySelected) []byte {
	var buffer bytes.Buffer
	fmt.Fprintln(&buffer, "# Executor Context")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Run")
	fmt.Fprintf(&buffer, "- Run id: %s\n", state.RunID)
	fmt.Fprintf(&buffer, "- Run status: %s\n", state.Status)
	fmt.Fprintf(&buffer, "- Map run id: %s\n", mapRunID)
	fmt.Fprintf(&buffer, "- Contract hash: %s\n", contractSHA256)
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Project map")
	fmt.Fprintf(&buffer, "- Repo map: %s\n", artifacts.WorkspaceRel+"/map/repo-map.md")
	fmt.Fprintf(&buffer, "- LLM map pointer: %s\n", artifacts.WorkspaceRel+"/map/llms.txt")
	fmt.Fprintf(&buffer, "- Search index: %s\n", artifacts.WorkspaceRel+"/map/search.sqlite")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Retrieval guidance")
	fmt.Fprintln(&buffer, "- Use `pactum search \"<term>\"` before adding new code.")
	fmt.Fprintln(&buffer, "- Prefer existing code items when applicable.")
	fmt.Fprintln(&buffer, "- If ownership is unclear, stop and ask for clarification.")
	fmt.Fprintln(&buffer, "- Do not rely on this map as complete semantic truth.")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Accepted memory")
	fmt.Fprintln(&buffer, "- Memory context: context/memory-context.md")
	fmt.Fprintln(&buffer, "- Selection: context/memory-selection.json")
	fmt.Fprintf(&buffer, "- Selected items: %d\n", memory.Total)
	fmt.Fprintf(&buffer, "- Fresh: %d\n", memory.Fresh)
	fmt.Fprintf(&buffer, "- Stale: %d\n", memory.Stale)
	fmt.Fprintf(&buffer, "- Unknown: %d\n", memory.Unknown)
	fmt.Fprintln(&buffer, "- Treat memory as context, not semantic truth.")
	fmt.Fprintln(&buffer, "- Stale memory must be verified before use.")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Relevant search results")
	if searchResults.QuerySource != "" {
		fmt.Fprintf(&buffer, "Query source: %s\n", searchResults.QuerySource)
	}
	if len(searchResults.Queries) > 0 {
		fmt.Fprintln(&buffer, "Targeted queries:")
		for _, query := range searchResults.Queries {
			fmt.Fprintf(&buffer, "- %s\n", query)
		}
	}
	if len(searchResults.Results) == 0 {
		fmt.Fprintln(&buffer, "No relevant results. Run `pactum search \"<term>\"` with narrower terms.")
	} else {
		fmt.Fprintln(&buffer, "Results:")
		for i, result := range searchResults.Results {
			line := fmt.Sprintf("%d. %s %s", i+1, result.Kind, result.Path)
			if result.Kind == "code_item" && result.Title != "" {
				line += " (" + result.Title + ")"
			}
			fmt.Fprintln(&buffer, line)
		}
	}
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Clarification decisions")
	if len(decisions) == 0 {
		fmt.Fprintln(&buffer, "- None")
	} else {
		for _, decision := range decisions {
			fmt.Fprintf(&buffer, "- %s: %s\n", decision.QuestionID, decision.Decision)
		}
	}
	return buffer.Bytes()
}

func renderApprovedPromptMD(contract draftContract, runID string, contractSHA256 string, selection memorySelectionDocument) []byte {
	var buffer bytes.Buffer
	fmt.Fprintln(&buffer, "# Executor Prompt")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "This prompt is prepared from an approved Pactum contract.")
	fmt.Fprintln(&buffer, "This prompt is prepared for the selected built-in agent when `pactum execute run` is used.")
	fmt.Fprintln(&buffer, "Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Contract status")
	fmt.Fprintf(&buffer, "- Run: %s\n", runID)
	fmt.Fprintln(&buffer, "- Approval: approved")
	fmt.Fprintf(&buffer, "- Contract hash: %s\n", contractSHA256)
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Goal")
	writeMarkdownValue(&buffer, contract.Goal)
	fmt.Fprintln(&buffer)
	writeMarkdownListSection(&buffer, "In scope", contract.Scope.In)
	fmt.Fprintln(&buffer)
	writeMarkdownListSection(&buffer, "Out of scope", contract.Scope.Out)
	fmt.Fprintln(&buffer)
	writeMarkdownListSection(&buffer, "Acceptance criteria", contract.AcceptanceCriteria)
	fmt.Fprintln(&buffer)
	writeMarkdownListSection(&buffer, "Validation commands", contract.Validation.Commands)
	fmt.Fprintln(&buffer)
	writeMarkdownListSection(&buffer, "Assumptions", contract.Assumptions)
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Clarifications")
	if len(contract.Clarifications.Questions) == 0 {
		fmt.Fprintln(&buffer, "- None")
	} else {
		for _, question := range contract.Clarifications.Questions {
			blocking := ""
			if question.Blocking {
				blocking = " [blocking]"
			}
			fmt.Fprintf(&buffer, "- %s%s %s\n", question.ID, blocking, question.Question)
			if question.Rationale != "" {
				fmt.Fprintf(&buffer, "  Rationale: %s\n", question.Rationale)
			}
			decision := question.Answer
			if strings.TrimSpace(decision) == "" {
				decision = "pending"
			}
			fmt.Fprintf(&buffer, "  Decision: %s\n", decision)
		}
	}
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Project context")
	fmt.Fprintln(&buffer, "- Executor context: context/executor-context.md")
	fmt.Fprintf(&buffer, "- Repo map: %s\n", artifacts.WorkspaceRel+"/map/repo-map.md")
	fmt.Fprintln(&buffer, "- Search results: context/search-results.json")
	fmt.Fprintln(&buffer, "- Accepted memory context: context/memory-context.md")
	fmt.Fprintln(&buffer)
	writeApprovedPromptMemorySection(&buffer, selection)
	fmt.Fprintln(&buffer, "## Instructions for future executor")
	fmt.Fprintln(&buffer, "- Follow the approved contract.")
	fmt.Fprintln(&buffer, "- Do not implement out-of-scope work.")
	fmt.Fprintln(&buffer, "- Search before creating new code.")
	fmt.Fprintln(&buffer, "- Prefer existing code items when applicable.")
	fmt.Fprintln(&buffer, "- If the contract is ambiguous, stop and request clarification.")
	fmt.Fprintln(&buffer, "- Use the listed validation commands as expected checks.")
	fmt.Fprintln(&buffer, "- Pactum gate can run approved validation commands after execution.")
	return buffer.Bytes()
}

func writeApprovedPromptMemorySection(buffer *bytes.Buffer, selection memorySelectionDocument) {
	counts := memorySelectionCounts(selection)
	fmt.Fprintln(buffer, "## Accepted memory")
	fmt.Fprintln(buffer)
	fmt.Fprintln(buffer, "Memory context:")
	fmt.Fprintln(buffer, "- context/memory-context.md")
	fmt.Fprintln(buffer)
	fmt.Fprintln(buffer, "Selected memory:")
	fmt.Fprintf(buffer, "- total: %d\n", counts.Total)
	fmt.Fprintf(buffer, "- fresh: %d\n", counts.Fresh)
	fmt.Fprintf(buffer, "- stale: %d\n", counts.Stale)
	fmt.Fprintf(buffer, "- unknown: %d\n", counts.Unknown)
	fmt.Fprintln(buffer)
	fmt.Fprintln(buffer, "Items:")
	if len(selection.Selected) == 0 {
		fmt.Fprintln(buffer, "- none")
	} else {
		for _, item := range selection.Selected {
			status := normalizeMemoryFreshnessStatus(item.Freshness.Status)
			fmt.Fprintf(buffer, "- %s [%s] score=%d — %s\n", item.ID, status, item.Score, compactMemoryContextText(item.Title))
			if status != memoryFreshnessFresh {
				for _, reason := range item.Freshness.Reasons {
					fmt.Fprintf(buffer, "  reason: %s\n", reason)
				}
			}
		}
	}
	fmt.Fprintln(buffer)
	fmt.Fprintln(buffer, "Rules:")
	fmt.Fprintln(buffer, "- Accepted memory is context, not semantic truth.")
	fmt.Fprintln(buffer, "- Stale memory may be outdated; verify before using.")
	fmt.Fprintln(buffer, "- Use `pactum search \"<term>\"` and inspect current source files before relying on memory.")
	fmt.Fprintln(buffer, "- Do not implement from memory alone.")
	fmt.Fprintln(buffer)
}

func writePromptBuild(stdout io.Writer, response promptBuildResponse) {
	fmt.Fprintln(stdout, "Executor prompt built")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", response.RunStatus)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Checks:")
	fmt.Fprintln(stdout, "  contract approved: yes")
	fmt.Fprintln(stdout, "  contract hash: ok")
	fmt.Fprintln(stdout, "  project map: fresh")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  prompt: %s\n", runArtifactRepoRel(response.RunID, response.Manifest.Artifacts.Prompt))
	fmt.Fprintf(stdout, "  executor context: %s\n", runArtifactRepoRel(response.RunID, response.Manifest.Artifacts.ExecutorContext))
	fmt.Fprintf(stdout, "  prompt manifest: %s\n", runArtifactRepoRel(response.RunID, "contract/prompt-manifest.json"))
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Next:")
	fmt.Fprintln(stdout, "  pactum execute dry-run --agent codex")
}

func writePromptShow(stdout io.Writer, response promptShowResponse) {
	fmt.Fprintln(stdout, "Executor Prompt")
	fmt.Fprintf(stdout, "Run: %s\n", response.RunID)
	fmt.Fprintf(stdout, "Status: %s\n", response.RunStatus)
	if response.Manifest.Memory != nil {
		selected := response.Manifest.Memory.Selected
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Memory:")
		fmt.Fprintf(stdout, "  selected: %d\n", selected.Total)
		fmt.Fprintf(stdout, "  fresh: %d\n", selected.Fresh)
		fmt.Fprintf(stdout, "  stale: %d\n", selected.Stale)
		fmt.Fprintf(stdout, "  unknown: %d\n", selected.Unknown)
	}
	fmt.Fprintln(stdout)
	fmt.Fprint(stdout, response.Prompt)
	if !strings.HasSuffix(response.Prompt, "\n") {
		fmt.Fprintln(stdout)
	}
}
