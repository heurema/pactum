package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/ledger"
)

const (
	clarificationQuestionSchema = "pactum.clarification_question.v1"
	clarificationAnswerSchema   = "pactum.clarification_answer.v1"
	clarificationDecisionSchema = "pactum.clarification_decision.v1"
)

type clarificationQuestionRecord struct {
	Schema             string    `json:"schema"`
	ID                 string    `json:"id"`
	RunID              string    `json:"run_id"`
	Question           string    `json:"question"`
	Blocking           bool      `json:"blocking"`
	Kind               string    `json:"kind,omitempty"`
	Rationale          string    `json:"rationale,omitempty"`
	RecommendedAnswer  string    `json:"recommended_answer,omitempty"`
	Confidence         string    `json:"confidence,omitempty"`
	DependsOn          []string  `json:"depends_on,omitempty"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	Source             string    `json:"source"`
	ClarifierAttemptID string    `json:"clarifier_attempt_id,omitempty"`
}

type clarificationAnswerRecord struct {
	Schema     string    `json:"schema"`
	ID         string    `json:"id"`
	RunID      string    `json:"run_id"`
	QuestionID string    `json:"question_id"`
	Answer     string    `json:"answer"`
	CreatedAt  time.Time `json:"created_at"`
	Source     string    `json:"source"`
}

type clarificationDecisionRecord struct {
	Schema     string    `json:"schema"`
	ID         string    `json:"id"`
	RunID      string    `json:"run_id"`
	QuestionID string    `json:"question_id"`
	Decision   string    `json:"decision"`
	CreatedAt  time.Time `json:"created_at"`
	Source     string    `json:"source"`
	// DecidedBy is the explicit CLI principal (--by). Automatic loop decisions
	// record only Source as their provenance and omit it.
	DecidedBy string `json:"decided_by,omitempty"`
}

type contractClarifySet struct {
	Questions []clarifyQuestionStatus `json:"questions"`
}

type clarifyQuestionStatus struct {
	ID                string   `json:"id"`
	Question          string   `json:"question"`
	Blocking          bool     `json:"blocking"`
	Kind              string   `json:"kind,omitempty"`
	Rationale         string   `json:"rationale,omitempty"`
	RecommendedAnswer string   `json:"recommended_answer,omitempty"`
	Confidence        string   `json:"confidence,omitempty"`
	DependsOn         []string `json:"depends_on,omitempty"`
	Status            string   `json:"status"`
	Blocked           bool     `json:"blocked,omitempty"`
	Answer            string   `json:"answer,omitempty"`
}

type clarifyStatusResponse struct {
	RunID        string                  `json:"run_id"`
	RunStatus    string                  `json:"run_status"`
	Total        int                     `json:"total"`
	Answered     int                     `json:"answered"`
	Open         int                     `json:"open"`
	BlockingOpen int                     `json:"blocking_open"`
	Converged    bool                    `json:"converged"`
	Coverage     []clarifyKindCoverage   `json:"coverage"`
	Questions    []clarifyQuestionStatus `json:"questions"`
}

// clarifyKindCoverage tallies, for one question kind, how many questions exist
// and how many are still open vs answered (and open-and-blocking). It turns the
// flat open count into a per-dimension view so an unprobed dimension is visible.
type clarifyKindCoverage struct {
	Kind         string `json:"kind"`
	Total        int    `json:"total"`
	Open         int    `json:"open"`
	Answered     int    `json:"answered"`
	BlockingOpen int    `json:"blocking_open"`
}

// canonicalClarificationKinds are the contract dimensions worth ensuring
// coverage of, in the fixed display order. Each always appears in the coverage
// breakdown (even at zero) so an unprobed dimension is visible; 'other' is a
// catch-all surfaced only when used and is not listed here.
var canonicalClarificationKinds = []string{"terminology", "scope", "acceptance", "edge_case", "assumption"}

type clarifyAskResponse struct {
	RunID     string                      `json:"run_id"`
	RunStatus string                      `json:"run_status"`
	Question  clarificationQuestionRecord `json:"question"`
	// ApprovalReset reports that recording this question regressed an
	// already-approved run back to clarifying (approval reset to pending). It is
	// omitted (false) when the run was not approved. The reset itself is allowed;
	// the field only makes the otherwise-silent regression visible.
	ApprovalReset bool `json:"approval_reset,omitempty"`
}

type clarifyAnswerResponse struct {
	RunID     string                      `json:"run_id"`
	RunStatus string                      `json:"run_status"`
	Answer    clarificationAnswerRecord   `json:"answer"`
	Decision  clarificationDecisionRecord `json:"decision"`
	// ApprovalReset reports that recording this answer regressed an
	// already-approved run back to clarifying (approval reset to pending). It is
	// omitted (false) when the run was not approved.
	ApprovalReset bool `json:"approval_reset,omitempty"`
}

func (a App) ClarifyAsk(stdout io.Writer, runID string, question string, blocking bool, jsonOutput bool) error {
	context, ok, err := a.loadClarifyContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}

	now := a.nowUTC()
	questions, err := readClarificationQuestions(context.RunPaths.QuestionsJSONL)
	if err != nil {
		return err
	}
	record := clarificationQuestionRecord{
		Schema:    clarificationQuestionSchema,
		ID:        nextClarificationID("q", len(questions)+1),
		RunID:     runID,
		Question:  question,
		Blocking:  blocking,
		Status:    "open",
		CreatedAt: now,
		Source:    "manual",
	}
	if err := appendJSONLine(context.RunPaths.QuestionsJSONL, record); err != nil {
		return err
	}
	status, approvalReset, err := a.refreshClarificationArtifacts(context, now)
	if err != nil {
		return err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "clarification_question_added", Timestamp: now, RunID: runID}); err != nil {
		return err
	}

	response := clarifyAskResponse{RunID: runID, RunStatus: status.RunStatus, Question: record, ApprovalReset: approvalReset}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeClarifyAskResponse(stdout, response)
	return nil
}

func (a App) ClarifyAnswer(stdout io.Writer, runID string, questionID string, answer string, decidedBy string, jsonOutput bool) error {
	context, ok, err := a.loadClarifyContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	questions, err := readClarificationQuestions(context.RunPaths.QuestionsJSONL)
	if err != nil {
		return err
	}
	if !hasClarificationQuestion(questions, questionID) {
		return fmt.Errorf("question not found: %s", questionID)
	}
	now := a.nowUTC()
	answerRecord, decisionRecord, err := recordClarificationAnswer(context.RunPaths, runID, questionID, answer, "manual", "manual_answer", normalizePrincipal(context.Root, decidedBy), now)
	if err != nil {
		return err
	}
	status, approvalReset, err := a.refreshClarificationArtifacts(context, now)
	if err != nil {
		return err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "clarification_answer_recorded", Timestamp: now, RunID: runID}); err != nil {
		return err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "clarification_decision_recorded", Timestamp: now, RunID: runID}); err != nil {
		return err
	}

	response := clarifyAnswerResponse{RunID: runID, RunStatus: status.RunStatus, Answer: answerRecord, Decision: decisionRecord, ApprovalReset: approvalReset}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeClarifyAnswerResponse(stdout, response)
	return nil
}

// recordClarificationAnswer creates and appends the answer record and its
// mirroring decision record for a question, with the given record sources, so
// the manual ClarifyAnswer path and the clarify loop's auto-resolve share one
// write path. decidedBy is the explicit CLI principal; automatic callers pass
// "" so their decisions carry only the source. It neither refreshes the
// clarification artifacts nor appends ledger events — callers own those (the
// loop refreshes once per round, after all of the round's auto-resolves).
// TODO: Later revisions may reject duplicate answers or model answer revisions explicitly.
func recordClarificationAnswer(runPaths contractRunPathSet, runID string, questionID string, answer string, answerSource string, decisionSource string, decidedBy string, now time.Time) (clarificationAnswerRecord, clarificationDecisionRecord, error) {
	answers, err := readClarificationAnswers(runPaths.AnswersJSONL)
	if err != nil {
		return clarificationAnswerRecord{}, clarificationDecisionRecord{}, err
	}
	decisions, err := readClarificationDecisions(runPaths.DecisionsJSONL)
	if err != nil {
		return clarificationAnswerRecord{}, clarificationDecisionRecord{}, err
	}
	answerRecord := clarificationAnswerRecord{
		Schema:     clarificationAnswerSchema,
		ID:         nextClarificationID("a", len(answers)+1),
		RunID:      runID,
		QuestionID: questionID,
		Answer:     answer,
		CreatedAt:  now,
		Source:     answerSource,
	}
	decisionRecord := clarificationDecisionRecord{
		Schema:     clarificationDecisionSchema,
		ID:         nextClarificationID("d", len(decisions)+1),
		RunID:      runID,
		QuestionID: questionID,
		Decision:   answer,
		CreatedAt:  now,
		Source:     decisionSource,
		DecidedBy:  decidedBy,
	}
	if err := appendJSONLine(runPaths.AnswersJSONL, answerRecord); err != nil {
		return clarificationAnswerRecord{}, clarificationDecisionRecord{}, err
	}
	if err := appendJSONLine(runPaths.DecisionsJSONL, decisionRecord); err != nil {
		return clarificationAnswerRecord{}, clarificationDecisionRecord{}, err
	}
	return answerRecord, decisionRecord, nil
}

func (a App) ClarifyStatus(stdout io.Writer, runID string, jsonOutput bool) error {
	context, ok, err := a.loadClarifyContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	status, err := buildClarificationStatus(context.RunPaths, context.State)
	if err != nil {
		return err
	}
	if context.State.Status == "contract_approved" && status.BlockingOpen == 0 {
		status.RunStatus = "contract_approved"
	}
	if jsonOutput {
		return writeJSONResponse(stdout, status)
	}
	writeClarifyStatus(stdout, status)
	return nil
}

type clarifyContext struct {
	Root     string
	Paths    artifacts.Paths
	RunPaths contractRunPathSet
	State    contractRunState
}

func (a App) loadClarifyContext(stdout io.Writer, runID string, jsonOutput bool) (clarifyContext, bool, error) {
	base, ok, err := a.loadRunStateContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return clarifyContext{}, false, err
	}
	return clarifyContext{Root: base.Root, Paths: base.Paths, RunPaths: base.RunPaths, State: base.State}, true, nil
}

// refreshClarificationArtifacts recomputes the clarification status, rewrites the
// run state and contract artifacts, and resets approval when the run was already
// approved. It returns the refreshed status and the approvalReset bool from
// resetApprovalIfApproved so callers can surface the otherwise-silent regression
// (approved -> clarifying) to the user; approvalReset is false when the run was
// not approved.
func (a App) refreshClarificationArtifacts(context clarifyContext, updatedAt time.Time) (clarifyStatusResponse, bool, error) {
	status, err := buildClarificationStatus(context.RunPaths, context.State)
	if err != nil {
		return clarifyStatusResponse{}, false, err
	}

	state := context.State
	state.Status = status.RunStatus
	state.UpdatedAt = updatedAt
	_, approvalReset, err := resetApprovalIfApproved(context.Paths, context.RunPaths, context.Root, context.State.RunID, updatedAt)
	if err != nil {
		return clarifyStatusResponse{}, false, err
	}
	if err := writeJSON(context.RunPaths.RunJSON, state); err != nil {
		return clarifyStatusResponse{}, false, err
	}

	contract, err := readDraftContract(context.RunPaths.ContractJSON)
	if err != nil {
		return clarifyStatusResponse{}, false, err
	}
	applyClarificationStatusToContract(&contract, status)
	contract.Status = "draft"
	if err := writeContractArtifacts(context.RunPaths, contract, state.MapRunID); err != nil {
		return clarifyStatusResponse{}, false, err
	}
	status.RunStatus = state.Status
	return status, approvalReset, nil
}

func buildClarificationStatus(runPaths contractRunPathSet, state contractRunState) (clarifyStatusResponse, error) {
	questions, err := readClarificationQuestions(runPaths.QuestionsJSONL)
	if err != nil {
		return clarifyStatusResponse{}, err
	}
	answers, err := readClarificationAnswers(runPaths.AnswersJSONL)
	if err != nil {
		return clarifyStatusResponse{}, err
	}
	latestAnswers := latestAnswersByQuestion(answers)

	response := clarifyStatusResponse{
		RunID:     state.RunID,
		RunStatus: state.Status,
		Questions: []clarifyQuestionStatus{},
	}
	for _, question := range questions {
		answer, answered := latestAnswers[question.ID]
		questionStatus := "open"
		answerText := ""
		if answered {
			questionStatus = "answered"
			answerText = answer.Answer
			response.Answered++
		} else {
			response.Open++
			if question.Blocking {
				response.BlockingOpen++
			}
		}
		blocked := false
		if !answered {
			for _, prerequisiteID := range question.DependsOn {
				if _, prerequisiteAnswered := latestAnswers[prerequisiteID]; !prerequisiteAnswered {
					blocked = true
					break
				}
			}
		}
		response.Questions = append(response.Questions, clarifyQuestionStatus{
			ID:                question.ID,
			Question:          question.Question,
			Blocking:          question.Blocking,
			Kind:              question.Kind,
			Rationale:         question.Rationale,
			RecommendedAnswer: question.RecommendedAnswer,
			Confidence:        question.Confidence,
			DependsOn:         question.DependsOn,
			Status:            questionStatus,
			Blocked:           blocked,
			Answer:            answerText,
		})
	}
	response.Total = len(response.Questions)
	response.Coverage = buildClarifyKindCoverage(response.Questions)
	response.Converged = response.BlockingOpen == 0
	if response.BlockingOpen > 0 {
		response.RunStatus = "clarifying"
	} else {
		response.RunStatus = "contract_draft"
	}
	return response, nil
}

// buildClarifyKindCoverage tallies the questions per kind. Every canonical
// dimension gets an entry in the fixed canonicalClarificationKinds order even
// when it has no questions, so an unprobed dimension stays visible; a single
// 'other' entry is appended only when some question falls outside the canonical
// set (including kind-less manual questions). The per-kind Total/Open/Answered/
// BlockingOpen are tallied exactly like the overall counters so they sum back.
func buildClarifyKindCoverage(questions []clarifyQuestionStatus) []clarifyKindCoverage {
	indexByKind := make(map[string]int, len(canonicalClarificationKinds)+1)
	coverage := make([]clarifyKindCoverage, 0, len(canonicalClarificationKinds)+1)
	for _, kind := range canonicalClarificationKinds {
		indexByKind[kind] = len(coverage)
		coverage = append(coverage, clarifyKindCoverage{Kind: kind})
	}
	otherIndex := -1
	for _, question := range questions {
		index, ok := indexByKind[question.Kind]
		if !ok {
			if otherIndex == -1 {
				otherIndex = len(coverage)
				coverage = append(coverage, clarifyKindCoverage{Kind: "other"})
			}
			index = otherIndex
		}
		coverage[index].Total++
		if question.Status == "answered" {
			coverage[index].Answered++
		} else {
			coverage[index].Open++
			if question.Blocking {
				coverage[index].BlockingOpen++
			}
		}
	}
	return coverage
}

func readContractRunState(path string) (contractRunState, error) {
	var state contractRunState
	return state, readJSON(path, &state)
}

func readDraftContract(path string) (draftContract, error) {
	var contract draftContract
	return contract, readJSON(path, &contract)
}

func readClarificationQuestions(path string) ([]clarificationQuestionRecord, error) {
	return readJSONLines[clarificationQuestionRecord](path)
}

func readClarificationAnswers(path string) ([]clarificationAnswerRecord, error) {
	return readJSONLines[clarificationAnswerRecord](path)
}

func readClarificationDecisions(path string) ([]clarificationDecisionRecord, error) {
	return readJSONLines[clarificationDecisionRecord](path)
}

func readJSONLines[T any](path string) ([]T, error) {
	file, err := activeStore.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []T{}, nil
		}
		return nil, err
	}
	defer file.Close()

	records := []T{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record T
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func appendJSONLine(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return activeStore.AppendBytes(path, data)
}

func writeJSONLines[T any](path string, values []T) error {
	var buffer strings.Builder
	encoder := json.NewEncoder(&buffer)
	for _, value := range values {
		if err := encoder.Encode(value); err != nil {
			return err
		}
	}
	return activeStore.WriteBytes(path, []byte(buffer.String()), 0o644)
}

func nextClarificationID(prefix string, index int) string {
	return fmt.Sprintf("%s_%03d", prefix, index)
}

func hasClarificationQuestion(questions []clarificationQuestionRecord, questionID string) bool {
	for _, question := range questions {
		if question.ID == questionID {
			return true
		}
	}
	return false
}

func latestAnswersByQuestion(answers []clarificationAnswerRecord) map[string]clarificationAnswerRecord {
	latest := make(map[string]clarificationAnswerRecord)
	for _, answer := range answers {
		latest[answer.QuestionID] = answer
	}
	return latest
}

func openClarificationQuestionTexts(questions []clarifyQuestionStatus) []string {
	open := []string{}
	for _, question := range questions {
		if question.Status == "open" {
			open = append(open, question.Question)
		}
	}
	return open
}

func readRunSearchResultCount(path string) int {
	var results runSearchResults
	if err := readJSON(path, &results); err != nil {
		return 0
	}
	return len(results.Results)
}

// writeApprovalResetWarning surfaces that a just-recorded clarification regressed
// an already-approved run back to the clarification stage (approval reset to
// pending). Re-clarifying an approved run is a legitimate operation, so this warns
// rather than blocks; it exists only because the reset (and its
// contract_approval_reset ledger event) were otherwise silent. runStatus is the
// run's status after the reset ("clarifying" when blocking questions remain open,
// otherwise "contract_draft") so the prose never contradicts the printed status.
func writeApprovalResetWarning(stdout io.Writer, runID string, runStatus string) {
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "Warning: this run was approved; recording this clarification reset approval to pending and moved it back to the clarification stage (status now %q).\n", runStatus)
	fmt.Fprintf(stdout, "Re-approve with 'pactum contract approve %s' once the clarifications are resolved.\n", runID)
}

func writeClarifyAskResponse(stdout io.Writer, response clarifyAskResponse) {
	fmt.Fprintln(stdout, "Clarification question added")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", response.RunStatus)
	if response.ApprovalReset {
		writeApprovalResetWarning(stdout, response.RunID, response.RunStatus)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Question:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Question.ID)
	fmt.Fprintf(stdout, "  blocking: %t\n", response.Question.Blocking)
	fmt.Fprintf(stdout, "  status: %s\n", response.Question.Status)
	fmt.Fprintf(stdout, "  text: %s\n", response.Question.Question)
}

func writeClarifyAnswerResponse(stdout io.Writer, response clarifyAnswerResponse) {
	fmt.Fprintln(stdout, "Clarification answer recorded")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", response.RunStatus)
	if response.ApprovalReset {
		writeApprovalResetWarning(stdout, response.RunID, response.RunStatus)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Answer:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Answer.ID)
	fmt.Fprintf(stdout, "  question: %s\n", response.Answer.QuestionID)
}

func writeClarifyStatus(stdout io.Writer, status clarifyStatusResponse) {
	fmt.Fprintln(stdout, "Clarification status")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", status.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", status.RunStatus)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Questions:")
	fmt.Fprintf(stdout, "  total: %d\n", status.Total)
	fmt.Fprintf(stdout, "  answered: %d\n", status.Answered)
	fmt.Fprintf(stdout, "  open: %d\n", status.Open)
	fmt.Fprintf(stdout, "  blocking open: %d\n", status.BlockingOpen)
	fmt.Fprintf(stdout, "  converged: %s\n", yesNo(status.Converged))
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Coverage by dimension:")
	for _, coverage := range status.Coverage {
		fmt.Fprintf(stdout, "  - %s: total %d, answered %d, open %d, blocking open %d\n", coverage.Kind, coverage.Total, coverage.Answered, coverage.Open, coverage.BlockingOpen)
	}
	if status.Open > 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Open questions:")
		for _, question := range status.Questions {
			if question.Status != "open" {
				continue
			}
			blocking := ""
			if question.Blocking {
				blocking = " [blocking]"
			}
			fmt.Fprintf(stdout, "  - %s%s %s\n", question.ID, blocking, question.Question)
			if question.Kind != "" {
				fmt.Fprintf(stdout, "    kind: %s\n", question.Kind)
			}
			if question.RecommendedAnswer != "" {
				confidence := question.Confidence
				if confidence == "" {
					confidence = "unknown"
				}
				fmt.Fprintf(stdout, "    recommended answer (confidence %s): %s\n", confidence, question.RecommendedAnswer)
			}
			if len(question.DependsOn) > 0 {
				fmt.Fprintf(stdout, "    depends on: %s\n", strings.Join(question.DependsOn, ", "))
			}
			if question.Blocked {
				fmt.Fprintln(stdout, "    blocked: waiting on unanswered prerequisites")
			}
		}
	}
}
