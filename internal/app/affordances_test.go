package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

// affordanceEnvelope decodes the pactum.error.v1alpha1 envelope with pointer fields
// so tests can distinguish an absent fix/next from an empty one.
type affordanceEnvelope struct {
	Schema string `json:"schema"`
	Error  struct {
		Message string  `json:"message"`
		Code    string  `json:"code"`
		Fix     *string `json:"fix"`
	} `json:"error"`
	Next *[]string `json:"next"`
}

// TestJSONErrorEnvelopePinnedPreconditions pins one representative --json
// failure per recognized precondition: schema, stable code, message, the exact
// fix (or its absence), safe next commands, nonzero exit, and empty stderr.
func TestJSONErrorEnvelopePinnedPreconditions(t *testing.T) {
	cases := []struct {
		name        string
		setup       func(t *testing.T) (App, []string, string)
		wantCode    string
		wantMessage func(runID string) string
		wantFix     func(runID string) string
		wantNext    func(runID string) []string
	}{
		{
			name: "not_initialized",
			setup: func(t *testing.T) (App, []string, string) {
				return testApp(t.TempDir()), []string{"contract", "approve", "--json"}, ""
			},
			wantCode:    "not_initialized",
			wantMessage: func(string) string { return "pactum is not initialized; run: pactum init" },
			wantFix:     func(string) string { return "pactum init" },
		},
		{
			name: "run_not_found",
			setup: func(t *testing.T) (App, []string, string) {
				app, _, runID := setupContractRun(t, t.TempDir())
				return app, []string{"task", "show", "run_missing", "--json"}, runID
			},
			wantCode:    "run_not_found",
			wantMessage: func(string) string { return "run not found: run_missing" },
			wantNext:    func(string) []string { return []string{"pactum task list"} },
		},
		{
			name: "project_map_stale",
			setup: func(t *testing.T) (App, []string, string) {
				root := t.TempDir()
				app, _, runID := setupApprovedAndBuiltPrompt(t, root)
				mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\nchanged\n")
				return app, []string{"execute", "plan", runID, "--json"}, runID
			},
			wantCode:    "project_map_stale",
			wantMessage: func(string) string { return "cannot prepare execution: project map is stale" },
			wantFix:     func(string) string { return "pactum map refresh" },
		},
		{
			name: "contract_not_approved",
			setup: func(t *testing.T) (App, []string, string) {
				app, _, runID := setupContractRun(t, t.TempDir())
				return app, []string{"prompt", "build", runID, "--json"}, runID
			},
			wantCode:    "contract_not_approved",
			wantMessage: func(string) string { return "cannot build executor prompt: contract is not approved" },
			wantFix:     func(runID string) string { return "pactum contract approve " + runID },
		},
		{
			name: "blocking_clarifications_open",
			setup: func(t *testing.T) (App, []string, string) {
				app, _, runID := setupContractRun(t, t.TempDir())
				var stdout, stderr bytes.Buffer
				if code := app.Run([]string{"clarify", "add", runID, "Need a decision?", "--blocking"}, &stdout, &stderr); code != 0 {
					t.Fatalf("clarify add exited %d, stderr: %s", code, stderr.String())
				}
				return app, []string{"contract", "approve", runID, "--json"}, runID
			},
			wantCode:    "blocking_clarifications_open",
			wantMessage: func(string) string { return "cannot approve contract: blocking clarification questions remain" },
			wantNext:    func(runID string) []string { return []string{"pactum clarify show " + runID} },
		},
		{
			name: "prompt_not_built",
			setup: func(t *testing.T) (App, []string, string) {
				app, _, runID := setupApprovedPromptContract(t, t.TempDir())
				return app, []string{"execute", "plan", runID, "--json"}, runID
			},
			wantCode:    "prompt_not_built",
			wantMessage: func(string) string { return "cannot prepare execution: executor prompt has not been built" },
			wantFix:     func(runID string) string { return "pactum prompt build " + runID },
		},
		{
			name: "no_execution_attempt",
			setup: func(t *testing.T) (App, []string, string) {
				app, _, runID := setupApprovedAndBuiltPrompt(t, t.TempDir())
				return app, []string{"gate", "run", runID, "--json"}, runID
			},
			wantCode:    "no_execution_attempt",
			wantMessage: func(string) string { return "cannot run gate: no completed execution attempts found" },
			wantNext:    func(runID string) []string { return []string{"pactum execute plan " + runID + " --agent codex"} },
		},
		{
			name: "gate_report_missing",
			setup: func(t *testing.T) (App, []string, string) {
				app, _, runID := setupApprovedAndBuiltPrompt(t, t.TempDir())
				return app, []string{"review", "approve", runID, "--json"}, runID
			},
			wantCode:    "gate_report_missing",
			wantMessage: func(string) string { return "cannot approve review: gate report not found" },
			wantFix:     func(runID string) string { return "pactum gate run " + runID },
		},
		{
			name: "pending_review_proposals",
			setup: func(t *testing.T) (App, []string, string) {
				app, _, runID, runPaths := setupApprovedPreparedReview(t, t.TempDir(), "passed")
				runReviewCommand(t, app, "review", "approve", runID)
				appendReviewProposalForTest(t, runPaths, runID, "p_001", "pending proposal", false)
				return app, []string{"memory", "propose", runID, "--json"}, runID
			},
			wantCode:    "pending_review_proposals",
			wantMessage: func(string) string { return "cannot propose memory: pending review proposals remain" },
			wantNext:    func(runID string) []string { return []string{"pactum review show " + runID} },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, args, runID := tc.setup(t)
			var stdout, stderr bytes.Buffer
			if code := app.Run(args, &stdout, &stderr); code != 1 {
				t.Fatalf("%v exited %d, want 1, stdout: %s stderr: %s", args, code, stdout.String(), stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("%v wrote stderr in --json mode:\n%s", args, stderr.String())
			}
			var envelope affordanceEnvelope
			assertNoError(t, json.Unmarshal(stdout.Bytes(), &envelope))
			if envelope.Schema != errorSchema {
				t.Fatalf("schema = %q, want %q", envelope.Schema, errorSchema)
			}
			if envelope.Error.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q", envelope.Error.Code, tc.wantCode)
			}
			if got, want := envelope.Error.Message, tc.wantMessage(runID); got != want {
				t.Fatalf("message = %q, want %q", got, want)
			}
			if tc.wantFix == nil {
				if envelope.Error.Fix != nil {
					t.Fatalf("fix = %q, want absent", *envelope.Error.Fix)
				}
			} else if envelope.Error.Fix == nil || *envelope.Error.Fix != tc.wantFix(runID) {
				t.Fatalf("fix = %v, want %q", envelope.Error.Fix, tc.wantFix(runID))
			}
			if tc.wantNext == nil {
				if envelope.Next != nil {
					t.Fatalf("next = %v, want absent", *envelope.Next)
				}
			} else if envelope.Next == nil || !equalStrings(*envelope.Next, tc.wantNext(runID)) {
				t.Fatalf("next = %v, want %v", envelope.Next, tc.wantNext(runID))
			}
		})
	}
}

// TestTaskNewClarifyLoopFailureJSONEnvelope pins the partial-success case: the
// run was created but the clarify loop failed, so --json emits the error
// envelope (not a task response) with the re-run command as the fix. The live
// clarifier output streams to stderr by design, so stderr is not asserted.
func TestTaskNewClarifyLoopFailureJSONEnvelope(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, _ := setupContractRun(t, root)
	app = configureClarifyLoopHelpers(t, app, paths)
	setClarifyLoopHelperEnv(t, filepath.Join(stateDir, "sequence"), "broken")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"task", "new", "integrate clarify", "--clarify", "--reviewer", clarifyLoopClarifierName, "--json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("task new --clarify --json exited %d, want 1, stdout: %s", code, stdout.String())
	}
	var envelope affordanceEnvelope
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &envelope))
	if envelope.Schema != errorSchema || envelope.Error.Code != "clarify_loop_failed" {
		t.Fatalf("envelope = %#v, want schema %q code clarify_loop_failed", envelope, errorSchema)
	}
	if !strings.Contains(envelope.Error.Message, "run "+taskNewSecondRunID+" was created") {
		t.Fatalf("message does not name the created run: %q", envelope.Error.Message)
	}
	if envelope.Error.Fix == nil || *envelope.Error.Fix != "pactum clarify run "+taskNewSecondRunID {
		t.Fatalf("fix = %v, want pactum clarify run %s", envelope.Error.Fix, taskNewSecondRunID)
	}
}

// TestNextArraysMirrorStageAffordances pins the top-level next arrays across
// the lifecycle surfaces: they hold concrete runnable commands with the run id
// filled, match the human Next: hints where those exist, and stay empty where
// the next move needs a human.
func TestNextArraysMirrorStageAffordances(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	// Fresh run: the draft awaits approval.
	next := decodeNext(t, app, "task", "new", "another task", "--json")
	assertNext(t, next, "pactum contract approve "+taskNewSecondRunID)
	runReviewCommand(t, app, "task", "use", runID)

	// Open blocking question: next points at safe inspection, not templates.
	next = decodeNext(t, app, "clarify", "add", runID, "Need a decision?", "--blocking", "--json")
	assertNext(t, next, "pactum clarify show "+runID)
	next = decodeNext(t, app, "clarify", "answer", runID, "q_001", "Decided.", "--json")
	assertNext(t, next, "pactum contract approve "+runID)

	// Approval advances to the prompt boundary.
	next = decodeNext(t, app, "contract", "approve", runID, "--json")
	assertNext(t, next, "pactum prompt build "+runID)

	// Prompt build mirrors its human Next: hint with the run id filled.
	next = decodeNext(t, app, "prompt", "build", runID, "--json")
	assertNext(t, next, "pactum execute plan "+runID+" --agent codex")

	var human, stderr bytes.Buffer
	if code := app.Run([]string{"prompt", "build", runID}, &human, &stderr); code != 0 {
		t.Fatalf("prompt build exited %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(human.String(), "Next:\n  pactum execute plan "+runID+" --agent codex\n") {
		t.Fatalf("human prompt build Next: hint changed:\n%s", human.String())
	}

	// Real execution is human-approved, so the plan has no next.
	next = decodeNext(t, app, "execute", "plan", runID, "--json")
	assertNext(t, next)

	// status and task show expose the same stage affordance, keeping the
	// next_command compatibility fields.
	var stdout bytes.Buffer
	if code := app.Run([]string{"status", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("status exited %d, stderr: %s", code, stderr.String())
	}
	var status struct {
		Runs struct {
			NextCommand string `json:"next_command"`
		} `json:"runs"`
		Next *[]string `json:"next"`
	}
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &status))
	if status.Runs.NextCommand != "pactum execute plan" {
		t.Fatalf("status runs.next_command = %q, want unchanged bare command", status.Runs.NextCommand)
	}
	if status.Next == nil || !equalStrings(*status.Next, []string{"pactum execute plan " + runID + " --agent codex"}) {
		t.Fatalf("status next = %v", status.Next)
	}

	stdout.Reset()
	if code := app.Run([]string{"task", "show", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("task show exited %d, stderr: %s", code, stderr.String())
	}
	var show struct {
		NextCommand string    `json:"next_command"`
		Next        *[]string `json:"next"`
	}
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &show))
	if show.NextCommand != "pactum execute plan" {
		t.Fatalf("task show next_command = %q, want unchanged bare command", show.NextCommand)
	}
	if show.Next == nil || !equalStrings(*show.Next, []string{"pactum execute plan " + runID + " --agent codex"}) {
		t.Fatalf("task show next = %v", show.Next)
	}

	// Read-only not-ready guidance keeps exit 0 and suggested_command, and adds
	// the same command as fix.
	stdout.Reset()
	if code := app.Run([]string{"gate", "show", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("gate show exited %d, stderr: %s", code, stderr.String())
	}
	var notReady struct {
		Schema           string `json:"schema"`
		Ready            bool   `json:"ready"`
		SuggestedCommand string `json:"suggested_command"`
		Fix              string `json:"fix"`
	}
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &notReady))
	if notReady.Schema != notReadySchema || notReady.Ready {
		t.Fatalf("gate show not-ready response mismatch: %#v", notReady)
	}
	if want := "pactum gate run " + runID; notReady.SuggestedCommand != want || notReady.Fix != want {
		t.Fatalf("gate show suggested_command/fix = %q/%q, want %q", notReady.SuggestedCommand, notReady.Fix, want)
	}
}

// TestMemoryProposeNextMatchesHumanHints pins that memory propose emits the
// same affordance set in JSON as its human Next: block, and that a fully
// finished run has no next move.
func TestMemoryProposeNextMatchesHumanHints(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupApprovedReviewedMemoryRun(t, root)

	next := decodeNext(t, app, "memory", "propose", runID, "--json")
	assertNext(t, next, "pactum memory show "+runID, "pactum memory accept "+runID)

	var human, stderr bytes.Buffer
	if code := app.Run([]string{"memory", "propose", runID}, &human, &stderr); code != 0 {
		t.Fatalf("memory propose exited %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(human.String(), "Next:\n  pactum memory show "+runID+"\n  pactum memory accept "+runID+"\n") {
		t.Fatalf("human memory propose Next: block changed:\n%s", human.String())
	}

	next = decodeNext(t, app, "memory", "accept", runID, "--json")
	assertNext(t, next)
}

// TestNextAffordancesAcrossLifecycleStages drives one run through the whole
// pipeline with helper agents and pins the next affordance each stage's
// actual JSON response emits — including the legality gates: an open blocking
// finding and a proposal collected after review approval both point at safe
// inspection instead of a command the CLI would reject.
func TestNextAffordancesAcrossLifecycleStages(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	// A recorded draft proposal awaits acceptance. The proposal is not
	// accepted here: the original contract keeps an empty validation list, so
	// the gate below stays deterministic.
	app = configureHelperContractDrafters(t, app, paths, "helper")
	t.Setenv("PACTUM_CONTRACT_DRAFTER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_DRAFTER_EXPECTED_CWD", root)
	next := decodeNext(t, app, "contract", "draft", runID, "--reviewer", "helper", "--json")
	assertNext(t, next, "pactum contract accept "+runID)

	next = decodeNext(t, app, "contract", "approve", runID, "--json")
	assertNext(t, next, "pactum prompt build "+runID)

	// Registering the helper agent changed the config, so refresh the map
	// before building the prompt. The workspace refreshes print no human
	// Next: hints, so JSON mirrors that with the explicit empty array.
	next = decodeNext(t, app, "map", "refresh", "--json")
	assertNext(t, next)
	next = decodeNext(t, app, "memory", "refresh", "--json")
	assertNext(t, next)

	next = decodeNext(t, app, "prompt", "build", runID, "--json")
	assertNext(t, next, "pactum execute plan "+runID+" --agent codex")

	app = configureHelperAgent(app, "helper")
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)
	next = decodeNext(t, app, "execute", "run", runID, "--agent", "helper", "--json")
	assertNext(t, next, "pactum gate run "+runID)

	// A gated run with nothing open already affords approval: the review
	// scaffold is implicit, so no preparation step sits in between.
	next = decodeNext(t, app, "gate", "run", runID, "--json")
	assertNext(t, next, "pactum review approve "+runID)

	// review run scaffolds the review, runs the panel, and reports the loop
	// summary; with a clean helper the review stays approvable.
	app = configureHelperReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_REVIEWER_EXPECTED_CWD", root)
	next = decodeNext(t, app, "review", "run", runID, "--reviewer", "helper", "--no-fix", "--max-rounds", "1", "--json")
	assertNext(t, next, "pactum review approve "+runID)

	// An open blocking finding makes approval illegal, so next points at
	// inspection until it is resolved.
	next = decodeNext(t, app, "review", "finding", "add", runID, "needs a fix", "--blocking", "--json")
	assertNext(t, next, "pactum review show "+runID)

	app = configureHelperFixers(t, app, paths, "helper")
	t.Setenv("PACTUM_FIXER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_FIXER_EXPECTED_CWD", root)
	next = decodeNext(t, app, "review", "fix", "run", runID, "--agent", "helper", "--json")
	assertNext(t, next, "pactum review fix apply "+runID)

	next = decodeNext(t, app, "review", "finding", "resolve", runID, "f_001", "--note", "fixed", "--json")
	assertNext(t, next, "pactum review approve "+runID)

	next = decodeNext(t, app, "review", "approve", runID, "--json")
	assertNext(t, next, "pactum memory propose "+runID)

	// A proposal collected after approval re-blocks memory propose, so next
	// returns to inspection until it is decided.
	appendReviewProposalForTest(t, runPaths, runID, "p_001", "late proposal", false)
	next = decodeNext(t, app, "task", "show", runID, "--json")
	assertNext(t, next, "pactum review show "+runID)
	next = decodeNext(t, app, "review", "proposal", "reject", runID, "p_001", "--reason", "out of scope", "--json")
	assertNext(t, next, "pactum memory propose "+runID)

	next = decodeNext(t, app, "memory", "propose", runID, "--json")
	assertNext(t, next, "pactum memory show "+runID, "pactum memory accept "+runID)
	next = decodeNext(t, app, "memory", "accept", runID, "--json")
	assertNext(t, next)
}

// TestNextSwitchesToProposeForStaleMemoryCandidate pins the staleness gate on
// the memory_proposed affordance: a candidate whose pinned review state no
// longer matches must be regenerated, so next advertises memory propose while
// reproposal is legal and falls back to inspection when it is not.
func TestNextSwitchesToProposeForStaleMemoryCandidate(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedReviewedMemoryRun(t, root)
	runMemoryCommand(t, app, "memory", "propose", runID)

	// Fresh candidate: accept is the legal move.
	next := decodeNext(t, app, "task", "show", runID, "--json")
	assertNext(t, next, "pactum memory accept "+runID)

	// A proposal collected and rejected after the candidate changes the
	// pinned review state while propose stays legal: advertise regeneration.
	appendReviewProposalForTest(t, runPaths, runID, "p_003", "late proposal", false)
	runMemoryCommand(t, app, "review", "proposal", "reject", runID, "p_003", "--reason", "out of scope")
	next = decodeNext(t, app, "task", "show", runID, "--json")
	assertNext(t, next, "pactum memory propose "+runID)

	// A pending proposal makes propose illegal too: point at inspection.
	appendReviewProposalForTest(t, runPaths, runID, "p_004", "later proposal", false)
	next = decodeNext(t, app, "task", "show", runID, "--json")
	assertNext(t, next, "pactum review show "+runID)
}

// TestNextCommandSwitchesToProposeForStaleMemoryCandidate pins the same
// staleness gate on the legacy next_command compatibility field: task and
// status surfaces must not advertise memory accept for a stale candidate.
func TestNextCommandSwitchesToProposeForStaleMemoryCandidate(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedReviewedMemoryRun(t, root)
	runMemoryCommand(t, app, "memory", "propose", runID)

	if got := taskShowNextCommand(t, app, runID); got != "pactum memory accept" {
		t.Fatalf("fresh candidate next_command = %q, want pactum memory accept", got)
	}

	// The pinned review state changed while propose stays legal: regenerate.
	appendReviewProposalForTest(t, runPaths, runID, "p_003", "late proposal", false)
	runMemoryCommand(t, app, "review", "proposal", "reject", runID, "p_003", "--reason", "out of scope")
	if got := taskShowNextCommand(t, app, runID); got != "pactum memory propose" {
		t.Fatalf("stale candidate next_command = %q, want pactum memory propose", got)
	}

	// A pending proposal makes propose illegal too: point at inspection.
	appendReviewProposalForTest(t, runPaths, runID, "p_004", "later proposal", false)
	if got := taskShowNextCommand(t, app, runID); got != "pactum review show" {
		t.Fatalf("pending-proposal next_command = %q, want pactum review show", got)
	}
}

func taskShowNextCommand(t *testing.T, app App, runID string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"task", "show", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("task show exited %d, stderr: %s", code, stderr.String())
	}
	var show taskShowResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &show))
	return show.NextCommand
}

// TestNextDoesNotAdvertiseApproveForFailedGate pins the failed-gate legality
// gate: review approve rejects a failed gate, so a gated run with one keeps
// pointing at inspection even with nothing open.
func TestNextDoesNotAdvertiseApproveForFailedGate(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupGatePreparedRun(t, root, []string{"false"}, true)
	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr); code == 0 {
		t.Fatalf("gate run with a failing validation command exited 0:\n%s", stdout.String())
	}
	next := decodeNext(t, app, "task", "show", runID, "--json")
	assertNext(t, next, "pactum review show "+runID)
}

// TestNextFallsBackToInspectionOnUnreadableClarifications pins the
// unreadable-records legality gate: when the clarification records cannot be
// read the CLI cannot prove contract approval is legal, so next points at
// inspection instead of pactum contract approve.
func TestNextFallsBackToInspectionOnUnreadableClarifications(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	mustWriteFile(t, runPaths.QuestionsJSONL, "not json\n")
	if got := nextCommandsForRun(paths, runID); !equalStrings(got, []string{"pactum clarify show " + runID}) {
		t.Fatalf("nextCommandsForRun = %v, want the clarify show fallback", got)
	}
	next := decodeNext(t, app, "task", "show", runID, "--json")
	assertNext(t, next, "pactum clarify show "+runID)
}

// TestEmittedAffordancesUseCurrentGrammar walks command strings the CLI emits
// through fix and next affordances and parses each one against the current
// command grammar, so a renamed or removed verb (clarify status, execute
// dry-run, ...) can never be suggested to an agent. Every value comes from a
// real emission: the typed precondition errors, the read-only not-ready
// responses, and the ambiguous-run status hint. The stage next arrays are
// covered by the lifecycle tests above, whose decodeNext parses every decoded
// command against the same grammar.
func TestEmittedAffordancesUseCurrentGrammar(t *testing.T) {
	const runID = "run_20260531_184012"
	affordances := map[string]struct{}{}
	collect := func(commands ...string) {
		for _, command := range commands {
			if command != "" {
				affordances[command] = struct{}{}
			}
		}
	}
	collectErr := func(err error) {
		var precondition *preconditionError
		if !errors.As(err, &precondition) {
			t.Fatalf("expected precondition error, got %v", err)
		}
		collect(precondition.fix)
		collect(precondition.next...)
	}

	// Every recognized precondition's fix and next.
	collectErr(errNotInitialized)
	collectErr(runNotFoundError(runID))
	collectErr(projectMapStaleError("build executor prompt"))
	collectErr(contractNotApprovedError("build executor prompt", runID))
	collectErr(blockingClarificationsOpenError("approve contract", runID))
	collectErr(promptNotBuiltError("prepare execution", runID))
	collectErr(noExecutionAttemptError("cannot run gate: no completed execution attempts found", runID))
	collectErr(gateReportMissingError("approve review", runID))
	collectErr(pendingReviewProposalsError(runID))
	collectErr(clarifyLoopFailedError(runID, errors.New("boom")))

	// Stage affordances from a live run (contract_draft with and without an
	// open blocking question).
	root := t.TempDir()
	app, paths, liveRunID := setupContractRun(t, root)
	collect(nextCommandsForRun(paths, liveRunID)...)
	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"clarify", "add", liveRunID, "Need a decision?", "--blocking"}, &stdout, &stderr); code != 0 {
		t.Fatalf("clarify add exited %d, stderr: %s", code, stderr.String())
	}
	collect(nextCommandsForRun(paths, liveRunID)...)

	// Read-only not-ready guidance, extracted from the real responses.
	collect(decodeNotReadyFix(t, app, "prompt", "show", liveRunID, "--json")...)
	collect(decodeNotReadyFix(t, app, "gate", "show", liveRunID, "--json")...)
	collect(decodeNotReadyFix(t, app, "memory", "show", liveRunID, "--json")...)
	collect(decodeNotReadyFix(t, app, "contract", "show", "--draft", liveRunID, "--json")...)
	collect(decodeNotReadyFix(t, app, "review", "show", liveRunID, "--json")...)

	// With several active runs and no current-run pointer, status points at
	// selecting one.
	if code := app.Run([]string{"task", "new", "second task"}, &stdout, &stderr); code != 0 {
		t.Fatalf("task new exited %d, stderr: %s", code, stderr.String())
	}
	assertNoError(t, os.Remove(currentRunPointerPath(paths)))
	statusNext := decodeNext(t, app, "status", "--json")
	assertNext(t, statusNext, "pactum task use "+taskNewSecondRunID)
	collect(*statusNext...)

	if len(affordances) == 0 {
		t.Fatal("no affordances collected")
	}
	for command := range affordances {
		assertPactumCommandParses(t, command)
	}
}

// assertPactumCommandParses fails when command is not a placeholder-free
// pactum invocation that parses under the current CLI grammar.
func assertPactumCommandParses(t *testing.T, command string) {
	t.Helper()
	if strings.ContainsAny(command, "<>") {
		t.Fatalf("affordance %q contains a placeholder", command)
	}
	rest, ok := strings.CutPrefix(command, "pactum ")
	if !ok || strings.TrimSpace(rest) == "" {
		t.Fatalf("affordance %q is not a pactum command", command)
	}
	var grammar cli
	parser, err := kong.New(&grammar, kong.Name("pactum"))
	assertNoError(t, err)
	if _, err := parser.Parse(strings.Fields(rest)); err != nil {
		t.Fatalf("affordance %q does not parse under the current grammar: %v", command, err)
	}
}

// decodeNext runs a --json command that must succeed and returns its
// top-level next array (nil when the field is missing). Every decoded command
// is parsed against the current grammar, so any test that decodes a next
// array also pins it as runnable.
func decodeNext(t *testing.T, app App, args ...string) *[]string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := app.Run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("%v exited %d, stdout: %s stderr: %s", args, code, stdout.String(), stderr.String())
	}
	var response struct {
		Next *[]string `json:"next"`
	}
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Next != nil {
		for _, command := range *response.Next {
			assertPactumCommandParses(t, command)
		}
	}
	return response.Next
}

// decodeNotReadyFix runs a read-only --json command whose artifact does not
// exist yet and returns the suggested_command and fix the not-ready response
// carries.
func decodeNotReadyFix(t *testing.T, app App, args ...string) []string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := app.Run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("%v exited %d, stdout: %s stderr: %s", args, code, stdout.String(), stderr.String())
	}
	var response struct {
		Schema           string `json:"schema"`
		Ready            bool   `json:"ready"`
		SuggestedCommand string `json:"suggested_command"`
		Fix              string `json:"fix"`
	}
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Schema != notReadySchema || response.Ready {
		t.Fatalf("%v did not emit a not-ready response: %#v", args, response)
	}
	if response.Fix == "" || response.Fix != response.SuggestedCommand {
		t.Fatalf("%v fix/suggested_command mismatch: %#v", args, response)
	}
	return []string{response.SuggestedCommand, response.Fix}
}

// assertNext fails unless next is present and exactly the wanted commands; an
// empty want asserts the explicit empty array.
func assertNext(t *testing.T, next *[]string, want ...string) {
	t.Helper()
	if next == nil {
		t.Fatalf("next missing, want %v", want)
	}
	if !equalStrings(*next, want) {
		t.Fatalf("next = %v, want %v", *next, want)
	}
}

// TestConditionalNextBranches pins the conditional arms of the next-selection
// logic: a failed gate and a failed execution emit next: [] (no safe scripted
// move), while their passing counterparts emit affordances; nextReviewCommands
// gates review approve on decided proposals and no open blocking findings.
func TestConditionalNextBranches(t *testing.T) {
	t.Run("failed gate emits no next", func(t *testing.T) {
		root := t.TempDir()
		app, _, runID := setupGatePreparedRunWithRevision(t, root, map[string]any{"goal": "add deterministic gate", "paths_in_scope": []string{"internal/app/**"}}, true)
		mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\nchanged\n")

		var stdout, stderr bytes.Buffer
		if code := app.Run([]string{"gate", "run", runID, "--json"}, &stdout, &stderr); code == 0 {
			t.Fatalf("gate run with blocked scope should fail")
		}
		var response struct {
			Next []string `json:"next"`
		}
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
		if len(response.Next) != 0 {
			t.Fatalf("failed gate must emit no next moves, got %v", response.Next)
		}
	})

	t.Run("review approve gated on pending proposals", func(t *testing.T) {
		root := t.TempDir()
		_, _, runID, runPaths := setupPreparedReview(t, root, "needs_review")

		// No proposals, no open blocking findings: approval is legal.
		next := nextReviewCommands(runPaths, runID)
		if !slicesContain(next, "pactum review approve "+runID) {
			t.Fatalf("clean review must offer approve, got %v", next)
		}

		// A pending proposal removes approve and keeps inspection only.
		appendReviewProposalForTest(t, runPaths, runID, "p_001", "needs a decision", false)
		next = nextReviewCommands(runPaths, runID)
		if slicesContain(next, "pactum review approve "+runID) {
			t.Fatalf("pending proposal must gate approve, got %v", next)
		}
		if !slicesContain(next, "pactum review show "+runID) {
			t.Fatalf("inspection affordance missing, got %v", next)
		}
	})
}

func slicesContain(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
