package app

import (
	"fmt"
	"io"
)

const notReadySchema = "pactum.not_ready.v1"

// notReadyResponse is the machine-readable shape for a read-only command whose
// artifact does not exist yet. It keeps --json output parseable instead of
// leaking a plain-text notice onto stdout.
type notReadyResponse struct {
	Schema           string `json:"schema"`
	Ready            bool   `json:"ready"`
	RunID            string `json:"run_id"`
	Message          string `json:"message"`
	SuggestedCommand string `json:"suggested_command,omitempty"`
}

// writeNotReady emits a not-ready response. With --json it writes a structured
// {ready:false,...} document; otherwise the human message. It always returns nil
// (read-only guidance — the command exits 0).
func writeNotReady(stdout io.Writer, jsonOutput bool, runID, message, suggested string) error {
	if jsonOutput {
		return writeJSONResponse(stdout, notReadyResponse{
			Schema:           notReadySchema,
			Ready:            false,
			RunID:            runID,
			Message:          message,
			SuggestedCommand: suggested,
		})
	}
	fmt.Fprintln(stdout, message)
	return nil
}
