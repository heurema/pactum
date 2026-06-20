package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"regexp"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

// TransportErrorClass is the result of classifying a transport-layer error for
// retry decisions. Retryable marks transient failures safe to retry on a
// read-only stage. Kind is a stable token for logging and artifact records.
// Reason is a human-readable explanation.
type TransportErrorClass struct {
	Retryable bool
	Kind      string
	Reason    string
}

// ClassifyTransportError classifies a transport-layer error as retryable
// (transient) or permanent. It walks the full error chain via errors.Is and
// errors.As, and scans the complete error message for known text patterns.
//
// Conservative default: any error that does not match a known transient or
// permanent pattern is classified as Kind="unknown" with Retryable=false, so
// unrecognized errors do not trigger automatic retries.
func ClassifyTransportError(err error) TransportErrorClass {
	if err == nil {
		return TransportErrorClass{Kind: "ok", Reason: "no error"}
	}

	// context.DeadlineExceeded is always retryable; it is distinct from
	// context.Canceled which may be intentional.
	if errors.Is(err, context.DeadlineExceeded) {
		return TransportErrorClass{Retryable: true, Kind: "deadline", Reason: "context deadline exceeded"}
	}
	// context.Canceled alone is not retryable: it may be an intentional
	// cancellation. The lifecycle overrides this when RunResult.TimedOut is true.
	if errors.Is(err, context.Canceled) {
		return TransportErrorClass{Kind: "canceled", Reason: "context canceled"}
	}
	// io.EOF means the adapter subprocess closed its side of the connection.
	if errors.Is(err, io.EOF) {
		return TransportErrorClass{Retryable: true, Kind: "transport_drop", Reason: "adapter connection closed: EOF"}
	}

	// *net.OpError is checked before the generic net.Error interface because
	// *net.OpError implements net.Error — checking the interface first would
	// prevent the concrete type from ever being reached.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return TransportErrorClass{Retryable: true, Kind: "transport_drop", Reason: fmt.Sprintf("network operation failed: %s", opErr.Op)}
	}
	// net.Error covers timeouts and other temporary network failures not
	// represented by a concrete *net.OpError.
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return TransportErrorClass{Retryable: true, Kind: "network", Reason: "network timeout"}
		}
		return TransportErrorClass{Retryable: true, Kind: "network", Reason: "network error"}
	}

	// Adapter subprocess exited unexpectedly: a new process may succeed.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return TransportErrorClass{Retryable: true, Kind: "transport_drop", Reason: "adapter subprocess exited unexpectedly"}
	}

	// ACP JSON-RPC errors: classify by code first, then refine with text for the
	// Internal Error code (-32603) which can represent many upstream conditions.
	var reqErr *acp.RequestError
	if errors.As(err, &reqErr) {
		return classifyACPError(reqErr, err.Error())
	}

	// Text-only fallback: no typed error matched; scan the full message.
	return classifyByText(err.Error())
}

// classifyACPError classifies an ACP RequestError. For codes with a definitive
// semantic (auth, client error, cancellation) the code alone determines the
// outcome. For Internal Error (-32603) the full message text is inspected to
// distinguish rate-limit, quota, 5xx, and forwarded auth failures.
func classifyACPError(reqErr *acp.RequestError, fullMsg string) TransportErrorClass {
	switch reqErr.Code {
	case -32000: // Authentication required: permanent regardless of message text.
		return TransportErrorClass{Kind: "auth", Reason: "authentication required (ACP -32000)"}
	case -32600, -32601, -32602, -32700: // Client-side protocol errors.
		return TransportErrorClass{Kind: "client_error", Reason: fmt.Sprintf("ACP client error (code %d)", reqErr.Code)}
	case -32800: // Request cancelled by the ACP layer.
		return TransportErrorClass{Kind: "canceled", Reason: "ACP request cancelled"}
	case -32603: // Internal error: inspect text to distinguish upstream conditions.
		return classifyACPInternalError(fullMsg)
	default:
		return classifyByText(fullMsg)
	}
}

// classifyACPInternalError classifies an ACP Internal Error (-32603) by scanning
// the full error message. Precedence: quota > rate-limit > auth-phrase > 5xx >
// code-only auth (401/403) > client-error > transport-drop > default-retryable.
//
// Auth phrases ("unauthorized", "forbidden", etc.) are checked before 5xx so
// that a genuine outer auth failure is not incorrectly retried because the
// message also contains an incidental 5xx status code. Code-only auth signals
// (bare 401/403 with no accompanying phrase) are checked after 5xx so that an
// incidental 401/403 in an Internal Error body does not flip a genuine 5xx.
func classifyACPInternalError(fullMsg string) TransportErrorClass {
	sig := scanText(fullMsg)
	if sig.hasInsufficientQuota {
		return TransportErrorClass{Kind: "quota", Reason: "quota exceeded or insufficient quota"}
	}
	if sig.hasRateLimit {
		return TransportErrorClass{Retryable: true, Kind: "rate_limit", Reason: "rate limit or overloaded"}
	}
	// Auth phrases take precedence over an incidental 5xx: a genuine outer auth
	// failure (e.g. "Authentication required: gateway returned 500") must not retry.
	if sig.hasAuthPhrase {
		return TransportErrorClass{Kind: "auth", Reason: "auth or permission failure forwarded via ACP"}
	}
	// 5xx before code-only auth (401/403): an incidental 4xx status code in the
	// body does not flip a genuine server-side 5xx response.
	if sig.has5xx {
		return TransportErrorClass{Retryable: true, Kind: "server_error", Reason: "server error (5xx)"}
	}
	if sig.hasAuth {
		return TransportErrorClass{Kind: "auth", Reason: "auth or permission failure forwarded via ACP"}
	}
	if sig.hasClientErr {
		return TransportErrorClass{Kind: "client_error", Reason: "client error (4xx) forwarded via ACP"}
	}
	if sig.hasTransportDrop {
		return TransportErrorClass{Retryable: true, Kind: "transport_drop", Reason: "transport drop forwarded via ACP"}
	}
	// Default for -32603: treat as retryable (internal errors are often transient).
	return TransportErrorClass{Retryable: true, Kind: "server_error", Reason: "ACP internal error (code -32603)"}
}

// classifyByText classifies an error by scanning its full message text when no
// typed error signal was recognized. Precedence mirrors classifyACPInternalError
// but without the -32603 default-retryable fallback: an unrecognized text is
// returned as Kind="unknown" with Retryable=false.
func classifyByText(fullMsg string) TransportErrorClass {
	sig := scanText(fullMsg)
	if sig.hasInsufficientQuota {
		return TransportErrorClass{Kind: "quota", Reason: "quota exceeded or insufficient quota"}
	}
	if sig.hasRateLimit {
		return TransportErrorClass{Retryable: true, Kind: "rate_limit", Reason: "rate limit or overloaded"}
	}
	if sig.has5xx {
		return TransportErrorClass{Retryable: true, Kind: "server_error", Reason: "server error (5xx)"}
	}
	if sig.hasAuth {
		return TransportErrorClass{Kind: "auth", Reason: "authentication or permission failure"}
	}
	if sig.hasTransportDrop {
		return TransportErrorClass{Retryable: true, Kind: "transport_drop", Reason: "transport drop or connection error"}
	}
	if sig.hasClientErr {
		return TransportErrorClass{Kind: "client_error", Reason: "client error (4xx)"}
	}
	return TransportErrorClass{Kind: "unknown", Reason: truncateErrorReason(fullMsg)}
}

type textSignals struct {
	hasRateLimit         bool
	hasInsufficientQuota bool
	has5xx               bool
	hasAuth              bool
	// hasAuthPhrase is set when reAuth (phrase-based: "unauthorized", "forbidden",
	// etc.) matches, independently of the code-only re401/re403 signals. Used in
	// classifyACPInternalError to give genuine outer auth phrases precedence over
	// an incidental 5xx status code in the same message.
	hasAuthPhrase    bool
	hasClientErr     bool
	hasTransportDrop bool
}

var (
	// re429 matches the bare 429 status code on a word boundary — 50043 does NOT
	// match because there is no word boundary after the three-digit span.
	re429 = regexp.MustCompile(`\b429\b`)
	// reRateLimit matches common rate-limit and overload phrases.
	reRateLimit = regexp.MustCompile(`(?i)(rate[\-_\s]?limit|too\s+many\s+requests|throttl|overload)`)
	// reQuota matches quota-exhaustion phrases.
	reQuota = regexp.MustCompile(`(?i)(insufficient[\s_]quota|quota[\s_]exceeded|quota_exceeded|out[\s_]+of[\s_]+credits)`)
	// re5xx matches HTTP 5xx status codes on word boundaries only, so port numbers
	// like 50043 are not matched (no boundary after the three-digit span).
	re5xx = regexp.MustCompile(`\b5[0-9]{2}\b`)
	// re401 / re403 match those exact status codes on word boundaries.
	re401 = regexp.MustCompile(`\b401\b`)
	re403 = regexp.MustCompile(`\b403\b`)
	// reAuth matches common authentication/authorization error phrases.
	reAuth = regexp.MustCompile(`(?i)(unauthorized|forbidden|authentication\s+required|permission\s+denied|invalid\s+api[\s_]key|api[\s_]key\s+invalid)`)
	// re4xx matches any HTTP 4xx code on a word boundary (used as a catch-all
	// after the more-specific 401/403/quota/rate-limit checks).
	re4xx = regexp.MustCompile(`\b4[0-9]{2}\b`)
	// reTransportDrop matches low-level connection failures forwarded as text.
	reTransportDrop = regexp.MustCompile(`(?i)(connection\s+(refused|reset|closed|dropped)|broken\s+pipe|pipe\s+broken|\bEOF\b|i/o\s+timeout|no\s+such\s+(file|process))`)
)

func scanText(msg string) textSignals {
	return textSignals{
		hasRateLimit:         re429.MatchString(msg) || reRateLimit.MatchString(msg),
		hasInsufficientQuota: reQuota.MatchString(msg),
		has5xx:               re5xx.MatchString(msg),
		hasAuth:              re401.MatchString(msg) || re403.MatchString(msg) || reAuth.MatchString(msg),
		hasAuthPhrase:        reAuth.MatchString(msg),
		hasClientErr:         re4xx.MatchString(msg),
		hasTransportDrop:     reTransportDrop.MatchString(msg),
	}
}

func truncateErrorReason(s string) string {
	const maxLen = 120
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
