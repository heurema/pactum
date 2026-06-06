package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/ledger"
	searchpkg "github.com/heurema/pactum/internal/search"
)

const maxRepoMapContextBytes = 20000

const (
	runSchema      = "pactum.run.v1"
	contractSchema = "pactum.contract.v1"
)

type contractRunState struct {
	Schema    string               `json:"schema"`
	RunID     string               `json:"run_id"`
	Status    string               `json:"status"`
	Task      string               `json:"task"`
	CreatedAt time.Time            `json:"created_at"`
	UpdatedAt time.Time            `json:"updated_at"`
	RepoRoot  string               `json:"repo_root"`
	Workspace string               `json:"workspace"`
	MapRunID  string               `json:"map_run_id"`
	Artifacts contractRunArtifacts `json:"artifacts"`
}

type contractRunArtifacts struct {
	Task            string `json:"task"`
	RepoContext     string `json:"repo_context"`
	SearchResults   string `json:"search_results"`
	ExecutorContext string `json:"executor_context"`
	ContractJSON    string `json:"contract_json"`
	ContractMD      string `json:"contract_md"`
	Prompt          string `json:"prompt"`
	PromptManifest  string `json:"prompt_manifest"`
	Approval        string `json:"approval"`
}

type runSearchResults struct {
	Query       string                `json:"query"`
	Queries     []string              `json:"queries,omitempty"`
	QuerySource string                `json:"query_source,omitempty"`
	Results     []runSearchResultItem `json:"results"`
	Warnings    []string              `json:"warnings,omitempty"`
}

// runSearchResultItem is a single combined run-context search hit. It embeds the
// search result and records which targeted query surfaced it.
type runSearchResultItem struct {
	searchpkg.Result
	SourceQuery string `json:"source_query,omitempty"`
}

type draftContract struct {
	Schema             string             `json:"schema"`
	RunID              string             `json:"run_id"`
	Status             string             `json:"status"`
	Goal               string             `json:"goal"`
	Scope              draftContractScope `json:"scope"`
	AcceptanceCriteria []string           `json:"acceptance_criteria"`
	Validation         draftValidation    `json:"validation"`
	Assumptions        []string           `json:"assumptions"`
	OpenQuestions      []string           `json:"open_questions"`
	Clarifications     contractClarifySet `json:"clarifications,omitempty"`
	MemoryContext      draftMemoryContext `json:"memory_context"`
}

type draftContractScope struct {
	In  []string `json:"in"`
	Out []string `json:"out"`
}

type draftValidation struct {
	Commands []string `json:"commands"`
}

type draftMemoryContext struct {
	UsedItems []string `json:"used_items"`
}

type approvalState struct {
	Schema         string  `json:"schema"`
	Status         string  `json:"status"`
	ApprovedAt     *string `json:"approved_at"`
	ApprovedBy     *string `json:"approved_by"`
	ContractSHA256 *string `json:"contract_sha256"`
}

func (a App) createContractOnlyRun(root string, task string) (contractRunState, error) {
	paths := artifacts.New(root)
	report, err := a.workspaceStatus(root)
	if err != nil {
		return contractRunState{}, err
	}

	createdAt := a.nowUTC()
	runID, runDir, err := reserveContractRunDir(createdAt, paths.RunsDir)
	if err != nil {
		return contractRunState{}, err
	}
	runPaths := contractRunPaths(runDir)
	if err := ensureDirs([]string{
		runPaths.ContextDir,
		runPaths.ClarifyDir,
		runPaths.ContractDir,
		runPaths.ExecuteDir,
		runPaths.ReviewDir,
		runPaths.MemoryDir,
		runPaths.LedgerDir,
	}); err != nil {
		return contractRunState{}, err
	}

	state := contractRunState{
		Schema:    runSchema,
		RunID:     runID,
		Status:    "contract_draft",
		Task:      task,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
		RepoRoot:  ".",
		Workspace: artifacts.WorkspaceRel,
		MapRunID:  report.ProjectMap.RunID,
		Artifacts: contractRunArtifacts{
			Task:            "task.md",
			RepoContext:     "context/repo-context.md",
			SearchResults:   "context/search-results.json",
			ExecutorContext: "context/executor-context.md",
			ContractJSON:    "contract/contract.json",
			ContractMD:      "contract/contract.md",
			Prompt:          "contract/prompt.md",
			PromptManifest:  "contract/prompt-manifest.json",
			Approval:        "contract/approval.json",
		},
	}

	searchResults := buildRunSearchResults(paths, report.ProjectMap, "task", task)
	contract := draftContractFor(runID, task)
	memorySelection, err := buildAcceptedMemorySelection(paths, runID, task, "task", defaultMemorySelectionLimit, createdAt.Format(time.RFC3339))
	if err != nil {
		return contractRunState{}, err
	}
	files := map[string][]byte{
		runPaths.TaskMD:              renderTaskMD(task, createdAt),
		runPaths.RepoContext:         renderRepoContext(root, paths, report.ProjectMap.RunID, createdAt),
		runPaths.MemoryContextMD:     []byte(renderMemoryContextMD(memorySelection)),
		runPaths.QuestionsJSONL:      nil,
		runPaths.AnswersJSONL:        nil,
		runPaths.DecisionsJSONL:      nil,
		runPaths.ContractMD:          renderContractMDFromDraft(contract, report.ProjectMap.RunID, len(searchResults.Results)),
		runPaths.PromptMD:            renderPromptMDFromDraft(contract),
		runPaths.MemorySelectionJSON: mustMarshalJSON(memorySelection),
	}
	files[runPaths.SearchResults] = mustMarshalJSON(searchResults)
	files[runPaths.ContractJSON] = mustMarshalJSON(contract)
	files[runPaths.ApprovalJSON] = mustMarshalJSON(pendingApprovalState())
	files[runPaths.RunJSON] = mustMarshalJSON(state)

	for _, path := range sortedKeys(files) {
		if err := os.WriteFile(path, files[path], 0o644); err != nil {
			return contractRunState{}, err
		}
	}

	if err := ledger.Append(paths.EventsJSONL, ledger.Event{Type: "run_created", Timestamp: createdAt, RunID: runID, RepoRoot: root}); err != nil {
		return contractRunState{}, err
	}
	if err := ledger.Append(paths.EventsJSONL, ledger.Event{Type: "contract_draft_created", Timestamp: createdAt, RunID: runID, RepoRoot: root}); err != nil {
		return contractRunState{}, err
	}

	return state, nil
}

type contractRunPathSet struct {
	ContextDir  string
	ClarifyDir  string
	ContractDir string
	ExecuteDir  string
	GateDir     string
	ReviewDir   string
	MemoryDir   string
	LedgerDir   string

	RunJSON string
	TaskMD  string

	RepoContext         string
	SearchResults       string
	MemoryContextMD     string
	MemorySelectionJSON string

	ExecutorContext string

	QuestionsJSONL string
	AnswersJSONL   string
	DecisionsJSONL string

	ClarifierContextMD      string
	ClarifierPromptMD       string
	ClarifierAttemptsDir    string
	ClarifierLastResultJSON string

	ContractJSON   string
	ContractMD     string
	PromptMD       string
	PromptManifest string
	ApprovalJSON   string

	DryRunJSON     string
	AttemptsDir    string
	LastResultJSON string

	GateReportJSON    string
	GateValidationDir string

	MemoryCandidateJSON  string
	MemoryCandidateMD    string
	MemoryAcceptanceJSON string

	ReviewJSON                   string
	ReviewFindingsJSONL          string
	ReviewResolutionsJSONL       string
	ReviewProposalsJSONL         string
	ReviewProposalDecisionsJSONL string
	ReviewContextMD              string
	ReviewPromptMD               string
	ReviewDryRunJSON             string
	ReviewAttemptsDir            string
	ReviewLastResultJSON         string
	ReviewLoopSummaryJSON        string
	ReviewFixDir                 string
	ReviewFixContextMD           string
	ReviewFixPromptMD            string
	ReviewFixDryRunJSON          string
	ReviewFixAttemptsDir         string
	ReviewFixLastResultJSON      string
}

func contractRunPaths(runDir string) contractRunPathSet {
	contextDir := filepath.Join(runDir, "context")
	clarifyDir := filepath.Join(runDir, "clarify")
	contractDir := filepath.Join(runDir, "contract")
	executeDir := filepath.Join(runDir, "execute")
	gateDir := filepath.Join(runDir, "gate")
	reviewDir := filepath.Join(runDir, "review")
	return contractRunPathSet{
		ContextDir:                   contextDir,
		ClarifyDir:                   clarifyDir,
		ContractDir:                  contractDir,
		ExecuteDir:                   executeDir,
		GateDir:                      gateDir,
		ReviewDir:                    reviewDir,
		MemoryDir:                    filepath.Join(runDir, "memory"),
		LedgerDir:                    filepath.Join(runDir, "ledger"),
		RunJSON:                      filepath.Join(runDir, "run.json"),
		TaskMD:                       filepath.Join(runDir, "task.md"),
		RepoContext:                  filepath.Join(contextDir, "repo-context.md"),
		SearchResults:                filepath.Join(contextDir, "search-results.json"),
		MemoryContextMD:              filepath.Join(contextDir, "memory-context.md"),
		MemorySelectionJSON:          filepath.Join(contextDir, "memory-selection.json"),
		ExecutorContext:              filepath.Join(contextDir, "executor-context.md"),
		QuestionsJSONL:               filepath.Join(clarifyDir, "questions.jsonl"),
		AnswersJSONL:                 filepath.Join(clarifyDir, "answers.jsonl"),
		DecisionsJSONL:               filepath.Join(clarifyDir, "decisions.jsonl"),
		ClarifierContextMD:           filepath.Join(clarifyDir, "clarifier-context.md"),
		ClarifierPromptMD:            filepath.Join(clarifyDir, "clarifier-prompt.md"),
		ClarifierAttemptsDir:         filepath.Join(clarifyDir, "clarifier-attempts"),
		ClarifierLastResultJSON:      filepath.Join(clarifyDir, "clarifier-last-result.json"),
		ContractJSON:                 filepath.Join(contractDir, "contract.json"),
		ContractMD:                   filepath.Join(contractDir, "contract.md"),
		PromptMD:                     filepath.Join(contractDir, "prompt.md"),
		PromptManifest:               filepath.Join(contractDir, "prompt-manifest.json"),
		ApprovalJSON:                 filepath.Join(contractDir, "approval.json"),
		DryRunJSON:                   filepath.Join(executeDir, "dry-run.json"),
		AttemptsDir:                  filepath.Join(executeDir, "attempts"),
		LastResultJSON:               filepath.Join(executeDir, "last-result.json"),
		GateReportJSON:               filepath.Join(gateDir, "gate-report.json"),
		GateValidationDir:            filepath.Join(gateDir, "validation"),
		MemoryCandidateJSON:          filepath.Join(runDir, "memory", "memory-candidate.json"),
		MemoryCandidateMD:            filepath.Join(runDir, "memory", "memory-candidate.md"),
		MemoryAcceptanceJSON:         filepath.Join(runDir, "memory", "memory-acceptance.json"),
		ReviewJSON:                   filepath.Join(reviewDir, "review.json"),
		ReviewFindingsJSONL:          filepath.Join(reviewDir, "findings.jsonl"),
		ReviewResolutionsJSONL:       filepath.Join(reviewDir, "resolutions.jsonl"),
		ReviewProposalsJSONL:         filepath.Join(reviewDir, "proposals.jsonl"),
		ReviewProposalDecisionsJSONL: filepath.Join(reviewDir, "proposal-decisions.jsonl"),
		ReviewContextMD:              filepath.Join(reviewDir, "reviewer-context.md"),
		ReviewPromptMD:               filepath.Join(reviewDir, "reviewer-prompt.md"),
		ReviewDryRunJSON:             filepath.Join(reviewDir, "reviewer-dry-run.json"),
		ReviewAttemptsDir:            filepath.Join(reviewDir, "reviewer-attempts"),
		ReviewLastResultJSON:         filepath.Join(reviewDir, "reviewer-last-result.json"),
		ReviewLoopSummaryJSON:        filepath.Join(reviewDir, "loop-summary.json"),
		ReviewFixDir:                 filepath.Join(reviewDir, "fix"),
		ReviewFixContextMD:           filepath.Join(reviewDir, "fix", "fixer-context.md"),
		ReviewFixPromptMD:            filepath.Join(reviewDir, "fix", "fixer-prompt.md"),
		ReviewFixDryRunJSON:          filepath.Join(reviewDir, "fix", "fixer-dry-run.json"),
		ReviewFixAttemptsDir:         filepath.Join(reviewDir, "fix", "attempts"),
		ReviewFixLastResultJSON:      filepath.Join(reviewDir, "fix", "last-result.json"),
	}
}

func reserveContractRunDir(createdAt time.Time, runsDir string) (string, string, error) {
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		return "", "", err
	}

	base := "run_" + createdAt.Format("20060102_150405")
	for suffix := 1; ; suffix++ {
		candidate := base
		if suffix > 1 {
			candidate = fmt.Sprintf("%s_%02d", base, suffix)
		}
		path := filepath.Join(runsDir, candidate)
		if err := os.Mkdir(path, 0o755); err == nil {
			return candidate, path, nil
		} else if os.IsExist(err) {
			continue
		} else {
			return "", "", err
		}
	}
}

func renderTaskMD(task string, generatedAt time.Time) []byte {
	var buffer bytes.Buffer
	fmt.Fprintln(&buffer, "# Task")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, task)
	fmt.Fprintln(&buffer)
	fmt.Fprintf(&buffer, "Generated: %s\n", generatedAt.Format(time.RFC3339))
	return buffer.Bytes()
}

func renderRepoContext(root string, paths artifacts.Paths, mapRunID string, generatedAt time.Time) []byte {
	var buffer bytes.Buffer
	fmt.Fprintln(&buffer, "# Repository Context")
	fmt.Fprintln(&buffer)
	fmt.Fprintf(&buffer, "Generated: %s\n\n", generatedAt.Format(time.RFC3339))
	fmt.Fprintf(&buffer, "Map run: %s\n", mapRunID)
	fmt.Fprintf(&buffer, "Repo map path: %s\n", toRepoRel(root, paths.RepoMap))
	fmt.Fprintf(&buffer, "LLMS path: %s\n", toRepoRel(root, paths.LLMS))
	fmt.Fprintf(&buffer, "Search index path: %s\n", toRepoRel(root, paths.SearchSQLite))
	fmt.Fprintln(&buffer, "Accepted memory context: context/memory-context.md")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "Notes:")
	fmt.Fprintln(&buffer, "- Pactum has not yet done agentic clarification.")
	fmt.Fprintln(&buffer, "- This is deterministic context assembled from existing map artifacts.")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Project map")
	fmt.Fprintln(&buffer)

	repoMap, err := os.ReadFile(paths.RepoMap)
	if err != nil {
		fmt.Fprintf(&buffer, "Project map is unavailable at %s.\n", toRepoRel(root, paths.RepoMap))
		return buffer.Bytes()
	}
	repoMap = sanitizeRepoMapForRunContext(root, repoMap)
	if len(repoMap) <= maxRepoMapContextBytes {
		buffer.Write(repoMap)
		if !bytes.HasSuffix(repoMap, []byte("\n")) {
			fmt.Fprintln(&buffer)
		}
		return buffer.Bytes()
	}
	buffer.Write(repoMap[:maxRepoMapContextBytes])
	if !bytes.HasSuffix(repoMap[:maxRepoMapContextBytes], []byte("\n")) {
		fmt.Fprintln(&buffer)
	}
	fmt.Fprintf(&buffer, "\n[Truncated to %d bytes from %s]\n", maxRepoMapContextBytes, toRepoRel(root, paths.RepoMap))
	return buffer.Bytes()
}

func sanitizeRepoMapForRunContext(root string, repoMap []byte) []byte {
	sanitized := bytes.ReplaceAll(repoMap, []byte(root), []byte("."))
	slashRoot := filepath.ToSlash(root)
	if slashRoot != root {
		sanitized = bytes.ReplaceAll(sanitized, []byte(slashRoot), []byte("."))
	}
	return sanitized
}

const runContextSearchLimit = 10

// buildRunSearchResults assembles the run's first-pass search context. Instead
// of running the whole task/contract text as one FTS query — which ANDs every
// token and matches nothing for a natural-language sentence — it extracts a
// handful of targeted queries (paths, code identifiers, domain terms) and
// combines their results. querySource is "task" or "contract".
func buildRunSearchResults(paths artifacts.Paths, mapStatus projectMapStatus, querySource string, text string) runSearchResults {
	queries := extractRunContextQueries(text)
	result := runSearchResults{
		Query:       text,
		Queries:     queries,
		QuerySource: querySource,
		Results:     []runSearchResultItem{},
	}
	if mapStatus.Status == "stale" {
		result.Warnings = append(result.Warnings, "Search index is stale. Run: pactum map refresh.")
		return result
	}
	if mapStatus.SearchIndex != "ready" {
		result.Warnings = append(result.Warnings, "Search index is missing. Run: pactum map refresh.")
		return result
	}
	if len(queries) == 0 {
		return result
	}
	combined, err := runContextSearch(paths.SearchSQLite, queries, runContextSearchLimit)
	if err != nil {
		if searchpkg.IsMissingIndex(err) {
			result.Warnings = append(result.Warnings, "Search index is missing. Run: pactum map refresh.")
			return result
		}
		result.Warnings = append(result.Warnings, "Search failed: "+err.Error())
		return result
	}
	result.Results = combined
	return result
}

// runContextSearch runs each targeted query through the local index and merges
// the hits, deduping by (kind, path, title, code_kind). Earlier queries are
// more important: results are kept in query order, then result order within a
// query, capped at limit. This is deterministic first-pass retrieval, not
// semantic ranking.
func runContextSearch(dbPath string, queries []string, limit int) ([]runSearchResultItem, error) {
	if limit <= 0 {
		limit = runContextSearchLimit
	}

	// Run each targeted query and keep its ranked hits.
	perQuery := make([][]searchpkg.Result, len(queries))
	longest := 0
	for i, query := range queries {
		response, err := searchpkg.Query(dbPath, searchpkg.QueryOptions{
			Query: query,
			Limit: limit,
			Kind:  searchpkg.KindAny,
		})
		if err != nil {
			return nil, err
		}
		perQuery[i] = response.Results
		if len(response.Results) > longest {
			longest = len(response.Results)
		}
	}

	// Round-robin across queries (position-major, query-minor) so every targeted
	// query gets representation before the cap fills, rather than the most
	// specific query draining every slot. Dedupe globally.
	seen := map[string]bool{}
	ordered := []runSearchResultItem{}
	for pos := 0; pos < longest; pos++ {
		for i, results := range perQuery {
			if pos >= len(results) {
				continue
			}
			hit := results[pos]
			key := hit.Kind + "\x00" + hit.Path + "\x00" + hit.Title + "\x00" + hit.CodeKind
			if seen[key] {
				continue
			}
			seen[key] = true
			ordered = append(ordered, runSearchResultItem{Result: hit, SourceQuery: queries[i]})
		}
	}

	// De-prioritize import-like hits: keep definitions/files/wiki first and
	// imports last, preserving round-robin order within each group. Imports are
	// rarely the place to start editing, so they should not crowd the top of the
	// run context.
	combined := make([]runSearchResultItem, 0, len(ordered))
	var imports []runSearchResultItem
	for _, item := range ordered {
		if item.Kind == searchpkg.KindImport {
			imports = append(imports, item)
			continue
		}
		combined = append(combined, item)
	}
	combined = append(combined, imports...)
	if len(combined) > limit {
		combined = combined[:limit]
	}
	for i := range combined {
		combined[i].Rank = i + 1
	}
	return combined, nil
}

// runContextStopwords are generic task verbs and filler words that should not
// become standalone search queries. Domain words (format, percent, currency,
// prompt, manifest, …) are intentionally not listed.
var runContextStopwords = map[string]bool{
	"add": true, "update": true, "create": true, "remove": true, "implement": true,
	"change": true, "fix": true, "helper": true, "function": true, "file": true,
	"use": true, "using": true, "follow": true, "following": true, "should": true,
	"would": true, "could": true, "with": true, "from": true, "into": true,
	"this": true, "that": true, "test": true, "tests": true, "the": true, "and": true,
	"for": true, "new": true,
}

var (
	runContextWordSplitRe = regexp.MustCompile(`[^A-Za-z0-9_./-]+`)
	runContextCamelRe     = regexp.MustCompile(`[A-Z]+[a-z0-9]*|[a-z0-9]+`)
)

// extractRunContextQueries derives an ordered, deduped set of targeted search
// queries from natural-language task/contract text. Order: path-like strings,
// then code identifiers, then split identifier / domain terms, then remaining
// plain words. Capped at 8.
func extractRunContextQueries(text string) []string {
	const maxQueries = 8

	var paths, idents, parts, words []string
	pathSeen, identSeen, partSeen, wordSeen := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	pathComponents := map[string]bool{}

	for _, token := range strings.Fields(text) {
		token = strings.Trim(token, ".,;:!?()[]{}\"'`")
		if token == "" || !looksLikePath(token) {
			continue
		}
		key := strings.ToLower(token)
		if !pathSeen[key] {
			pathSeen[key] = true
			paths = append(paths, token)
		}
		for _, component := range regexp.MustCompile(`[/.]`).Split(token, -1) {
			if component != "" {
				pathComponents[strings.ToLower(component)] = true
			}
		}
	}

	for _, token := range runContextWordSplitRe.Split(text, -1) {
		token = strings.Trim(token, "-_./")
		if token == "" {
			continue
		}
		lower := strings.ToLower(token)
		switch {
		case isCodeIdentifier(token):
			if !identSeen[lower] {
				identSeen[lower] = true
				idents = append(idents, token)
			}
			for _, part := range splitIdentifier(token) {
				pl := strings.ToLower(part)
				if len(pl) >= 3 && !runContextStopwords[pl] && !partSeen[pl] {
					partSeen[pl] = true
					parts = append(parts, strings.ToLower(part))
				}
			}
		default:
			if len(lower) >= 4 && !runContextStopwords[lower] && !pathComponents[lower] && !wordSeen[lower] {
				wordSeen[lower] = true
				words = append(words, lower)
			}
		}
	}

	out := []string{}
	globalSeen := map[string]bool{}
	for _, group := range [][]string{paths, idents, parts, words} {
		for _, query := range group {
			key := strings.ToLower(query)
			if globalSeen[key] {
				continue
			}
			globalSeen[key] = true
			out = append(out, query)
			if len(out) >= maxQueries {
				return out
			}
		}
	}
	return out
}

// looksLikePath reports whether a token looks like a file path or filename: it
// contains a slash, or ends in a short alphabetic extension (so "format.ts" and
// "apps/x/format.test.ts" qualify, but "v1.0" and "3.14" do not).
func looksLikePath(token string) bool {
	if strings.Contains(token, "/") {
		return true
	}
	dot := strings.LastIndex(token, ".")
	if dot <= 0 || dot == len(token)-1 {
		return false
	}
	ext := token[dot+1:]
	if len(ext) < 1 || len(ext) > 5 {
		return false
	}
	for _, r := range ext {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return false
		}
	}
	return true
}

// isCodeIdentifier reports whether a token looks like a code identifier:
// camelCase/PascalCase, snake_case, or kebab-case.
func isCodeIdentifier(token string) bool {
	if !strings.ContainsAny(token, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		return false
	}
	if strings.ContainsAny(token, "_-") {
		return true
	}
	for i := 1; i < len(token); i++ {
		if token[i] >= 'A' && token[i] <= 'Z' && token[i-1] >= 'a' && token[i-1] <= 'z' {
			return true
		}
	}
	return false
}

// splitIdentifier breaks a code identifier into its word parts across case
// boundaries and `_`/`-` separators.
func splitIdentifier(identifier string) []string {
	normalized := strings.NewReplacer("_", " ", "-", " ").Replace(identifier)
	var parts []string
	for _, chunk := range strings.Fields(normalized) {
		parts = append(parts, runContextCamelRe.FindAllString(chunk, -1)...)
	}
	return parts
}

func draftContractFor(runID string, task string) draftContract {
	return draftContract{
		Schema: contractSchema,
		RunID:  runID,
		Status: "draft",
		Goal:   task,
		Scope: draftContractScope{
			In:  []string{},
			Out: []string{},
		},
		AcceptanceCriteria: []string{},
		Validation: draftValidation{
			Commands: []string{},
		},
		Assumptions:   []string{},
		OpenQuestions: []string{},
		Clarifications: contractClarifySet{
			Questions: []clarifyQuestionStatus{},
		},
		MemoryContext: draftMemoryContext{
			UsedItems: []string{},
		},
	}
}

func renderContractMD(task string, mapRunID string, searchResults int) []byte {
	contract := draftContractFor("", task)
	return renderContractMDFromDraft(contract, mapRunID, searchResults)
}

func renderContractMDFromDraft(contract draftContract, mapRunID string, searchResults int) []byte {
	var buffer bytes.Buffer
	fmt.Fprintln(&buffer, "# Contract Draft")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Goal")
	writeMarkdownValue(&buffer, contract.Goal)
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Current status")
	fmt.Fprintf(&buffer, "Contract status: %s\n", contract.Status)
	fmt.Fprintln(&buffer, "Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Relevant repository context")
	fmt.Fprintf(&buffer, "- Map run: %s\n", mapRunID)
	fmt.Fprintf(&buffer, "- Repo map: %s\n", filepath.ToSlash(filepath.Join(artifacts.WorkspaceRel, "map", "repo-map.md")))
	fmt.Fprintf(&buffer, "- Search results: context/search-results.json (%d result(s))\n", searchResults)
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
			fmt.Fprintf(&buffer, "- %s%s — %s\n", question.ID, blocking, question.Question)
			if question.Rationale != "" {
				fmt.Fprintf(&buffer, "  Rationale: %s\n", question.Rationale)
			}
			if question.Answer != "" {
				fmt.Fprintf(&buffer, "  Answer: %s\n", question.Answer)
			} else {
				fmt.Fprintln(&buffer, "  Answer: pending")
			}
		}
	}
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
	fmt.Fprintln(&buffer, "## Open questions")
	if len(contract.OpenQuestions) == 0 {
		fmt.Fprintln(&buffer, "- None")
	} else {
		for _, question := range contract.OpenQuestions {
			fmt.Fprintf(&buffer, "- %s\n", question)
		}
	}
	return buffer.Bytes()
}

func renderPromptMD(task string) []byte {
	return renderPromptMDFromDraft(draftContractFor("", task))
}

func renderPromptMDFromDraft(contract draftContract) []byte {
	var buffer bytes.Buffer
	fmt.Fprintln(&buffer, "# Executor Prompt")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "This is a contract-draft placeholder. Run `pactum prompt build` after the contract is approved to build the executor prompt for `pactum execute`.")
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
	fmt.Fprintln(&buffer, "## Project context")
	fmt.Fprintln(&buffer, "- Accepted memory context: context/memory-context.md")
	return buffer.Bytes()
}

func writeMarkdownValue(buffer *bytes.Buffer, value string) {
	if strings.TrimSpace(value) == "" {
		fmt.Fprintln(buffer, "TBD")
		return
	}
	fmt.Fprintln(buffer, value)
}

func writeMarkdownListSection(buffer *bytes.Buffer, heading string, values []string) {
	fmt.Fprintf(buffer, "## %s\n", heading)
	if len(values) == 0 {
		fmt.Fprintln(buffer, "TBD")
		return
	}
	for _, value := range values {
		fmt.Fprintf(buffer, "- %s\n", value)
	}
}

func runArtifactRepoRel(runID string, artifactPath string) string {
	return filepath.ToSlash(filepath.Join(artifacts.WorkspaceRel, "runs", runID, artifactPath))
}

func toRepoRel(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	if strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func mustMarshalJSON(value any) []byte {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		panic(err)
	}
	return append(data, '\n')
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
