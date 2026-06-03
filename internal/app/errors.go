package app

import (
	"errors"
	"io"
	"strings"
)

const errorSchema = "pactum.error.v1"

// errNotInitialized is returned by mutating commands invoked outside an
// initialized Pactum workspace. It maps to exit code 1 (an action error),
// unlike the read-only "not initialized" notice which exits 0.
var errNotInitialized = errors.New("pactum is not initialized; run: pactum init")

// errorEnvelope is the machine-readable shape emitted for command errors when
// the user requested --json output.
type errorEnvelope struct {
	Schema string            `json:"schema"`
	Error  errorEnvelopeBody `json:"error"`
}

type errorEnvelopeBody struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

// classifyErrorCode maps a command error to a stable machine-readable code for
// the JSON error envelope. The ordering matters: more specific phrases are
// matched before generic ones.
func classifyErrorCode(err error) string {
	if errors.Is(err, errNotInitialized) {
		return "not_initialized"
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
	case strings.Contains(msg, "not found"):
		return "run_not_found"
	default:
		return "command_failed"
	}
}

// writeErrorEnvelope encodes err as a pactum.error.v1 JSON document on stdout.
func writeErrorEnvelope(stdout io.Writer, err error) error {
	return writeJSONResponse(stdout, errorEnvelope{
		Schema: errorSchema,
		Error: errorEnvelopeBody{
			Message: err.Error(),
			Code:    classifyErrorCode(err),
		},
	})
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
