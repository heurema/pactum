package app

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

const errorSchema = "pactum.error.v1"

// preconditionError is a recognized workflow precondition failure carrying the
// affordances an agent reads instead of memorizing stage order: a stable
// machine-readable code, an optional exact remedial command (fix), and
// optional safe follow-up commands (next). fix is set only when a single
// exact runnable command remedies the failure — never a placeholder; next
// carries only safe, directly runnable commands.
type preconditionError struct {
	msg  string
	code string
	fix  string
	next []string
	err  error
}

func (e *preconditionError) Error() string { return e.msg }
func (e *preconditionError) Unwrap() error { return e.err }

// errNotInitialized is returned by mutating commands invoked outside an
// initialized Pactum workspace. It maps to exit code 1 (an action error),
// unlike the read-only "not initialized" notice which exits 0.
var errNotInitialized = &preconditionError{
	msg:  "pactum is not initialized; run: pactum init",
	code: "not_initialized",
	fix:  "pactum init",
}

func runNotFoundError(runID string) error {
	return &preconditionError{
		msg:  "run not found: " + runID,
		code: "run_not_found",
		next: []string{"pactum task list"},
	}
}

func projectMapStaleError(action string) error {
	return &preconditionError{
		msg:  fmt.Sprintf("cannot %s: project map is stale", action),
		code: "project_map_stale",
		fix:  "pactum map refresh",
	}
}

func contractNotApprovedError(action string, runID string) error {
	return &preconditionError{
		msg:  fmt.Sprintf("cannot %s: contract is not approved", action),
		code: "contract_not_approved",
		fix:  "pactum contract approve " + runID,
	}
}

// blockingClarificationsOpenError has no fix: answering clarification
// questions needs human-authored content, so next points at safe inspection.
func blockingClarificationsOpenError(action string, runID string) error {
	return &preconditionError{
		msg:  fmt.Sprintf("cannot %s: blocking clarification questions remain", action),
		code: "blocking_clarifications_open",
		next: []string{"pactum clarify show " + runID},
	}
}

func promptNotBuiltError(action string, runID string) error {
	return &preconditionError{
		msg:  fmt.Sprintf("cannot %s: executor prompt has not been built", action),
		code: "prompt_not_built",
		fix:  "pactum prompt build " + runID,
	}
}

// noExecutionAttemptError has no fix: real agent execution stays
// human-approved, so it never suggests `pactum execute run`; next points at
// the safe preparation step instead.
func noExecutionAttemptError(msg string, runID string) error {
	return &preconditionError{
		msg:  msg,
		code: "no_execution_attempt",
		next: []string{"pactum execute plan " + runID + " --agent codex"},
	}
}

func gateReportMissingError(action string, runID string) error {
	return &preconditionError{
		msg:  fmt.Sprintf("cannot %s: gate report not found", action),
		code: "gate_report_missing",
		fix:  "pactum gate run " + runID,
	}
}

// pendingReviewProposalsError has no fix: accepting or rejecting proposals is
// a per-proposal human decision, so next points at safe inspection.
func pendingReviewProposalsError(runID string) error {
	return &preconditionError{
		msg:  "cannot propose memory: pending review proposals remain",
		code: "pending_review_proposals",
		next: []string{"pactum review show " + runID},
	}
}

func clarifyLoopFailedError(runID string, err error) error {
	return &preconditionError{
		msg:  fmt.Sprintf("run %s was created, but its clarify loop failed (re-run it with: pactum clarify run %s): %v", runID, runID, err),
		code: "clarify_loop_failed",
		fix:  "pactum clarify run " + runID,
		err:  err,
	}
}

// errorEnvelope is the machine-readable shape emitted for command errors when
// the user requested --json output.
type errorEnvelope struct {
	Schema string            `json:"schema"`
	Error  errorEnvelopeBody `json:"error"`
	Next   []string          `json:"next,omitempty"`
}

type errorEnvelopeBody struct {
	Message string `json:"message"`
	Code    string `json:"code"`
	Fix     string `json:"fix,omitempty"`
}

// classifyErrorCode maps a command error to a stable machine-readable code for
// the JSON error envelope. Recognized precondition failures carry their code;
// the string fallbacks cover error paths that are not typed. The ordering
// matters: more specific phrases are matched before generic ones.
func classifyErrorCode(err error) string {
	var precondition *preconditionError
	if errors.As(err, &precondition) {
		return precondition.code
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "contract is not approved"):
		return "contract_not_approved"
	case strings.Contains(msg, "unsupported agent"), strings.Contains(msg, "unknown agent"):
		return "unsupported_agent"
	case strings.Contains(msg, "project map is stale"), strings.Contains(msg, "map is stale"):
		return "project_map_stale"
	case strings.Contains(msg, "not initialized"):
		return "not_initialized"
	case strings.Contains(msg, "run not found"):
		// Anchored on the exact runNotFoundError phrasing: a generic
		// "<thing> not found" must stay command_failed, not borrow the
		// stable run_not_found code.
		return "run_not_found"
	default:
		return "command_failed"
	}
}

// writeErrorEnvelope encodes err as a pactum.error.v1 JSON document on stdout.
func writeErrorEnvelope(stdout io.Writer, err error) error {
	envelope := errorEnvelope{
		Schema: errorSchema,
		Error: errorEnvelopeBody{
			Message: err.Error(),
			Code:    classifyErrorCode(err),
		},
	}
	var precondition *preconditionError
	if errors.As(err, &precondition) {
		envelope.Error.Fix = precondition.fix
		envelope.Next = precondition.next
	}
	return writeJSONResponse(stdout, envelope)
}

// jsonRequested reports whether the raw args asked for --json output. It is used
// at the top level to decide whether a command error should be rendered as a
// JSON envelope (stdout) or a human line (stderr). Parsing the flag generically
// here avoids threading the decision through every command's error path.
func jsonRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--json" || strings.HasPrefix(arg, "--json=") {
			return true
		}
		if arg == "--" {
			break
		}
	}
	return false
}
