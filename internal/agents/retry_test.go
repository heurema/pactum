package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

// fakeNetError implements net.Error for tests.
type fakeNetError struct {
	msg     string
	timeout bool
}

func (e *fakeNetError) Error() string   { return e.msg }
func (e *fakeNetError) Timeout() bool   { return e.timeout }
func (e *fakeNetError) Temporary() bool { return false }

func TestClassifyTransportError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
		kind      string
	}{
		// nil
		{
			name:      "nil error",
			err:       nil,
			retryable: false,
			kind:      "ok",
		},

		// context errors
		{
			name:      "context.DeadlineExceeded",
			err:       context.DeadlineExceeded,
			retryable: true,
			kind:      "deadline",
		},
		{
			name:      "context.DeadlineExceeded wrapped two levels",
			err:       fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", context.DeadlineExceeded)),
			retryable: true,
			kind:      "deadline",
		},
		{
			name:      "context.Canceled is not retryable",
			err:       context.Canceled,
			retryable: false,
			kind:      "canceled",
		},
		{
			name:      "context.Canceled wrapped",
			err:       fmt.Errorf("acp prompt: %w", context.Canceled),
			retryable: false,
			kind:      "canceled",
		},

		// io.EOF
		{
			name:      "io.EOF",
			err:       io.EOF,
			retryable: true,
			kind:      "transport_drop",
		},
		{
			name:      "io.EOF wrapped",
			err:       fmt.Errorf("acp initialize: %w", io.EOF),
			retryable: true,
			kind:      "transport_drop",
		},

		// network errors
		{
			name:      "net.Error timeout",
			err:       &fakeNetError{msg: "i/o timeout", timeout: true},
			retryable: true,
			kind:      "network",
		},
		{
			name:      "net.Error non-timeout",
			err:       &fakeNetError{msg: "connection reset by peer", timeout: false},
			retryable: true,
			kind:      "network",
		},
		{
			name:      "net.Error wrapped via errors.As",
			err:       fmt.Errorf("acp session: %w", &fakeNetError{msg: "EOF", timeout: false}),
			retryable: true,
			kind:      "network",
		},
		{
			name:      "net.OpError wrapped",
			err:       fmt.Errorf("acp prompt: %w", &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset")}),
			retryable: true,
			kind:      "transport_drop",
		},

		// subprocess exit
		{
			name:      "exec.ExitError wrapped",
			err:       fmt.Errorf("acp prompt: %w", &exec.ExitError{}),
			retryable: true,
			kind:      "transport_drop",
		},

		// ACP RequestError: auth (-32000)
		{
			name:      "ACP -32000 auth required",
			err:       &acp.RequestError{Code: -32000, Message: "Authentication required"},
			retryable: false,
			kind:      "auth",
		},
		{
			name:      "ACP -32000 auth required with incidental 500 in message",
			err:       fmt.Errorf("acp prompt: %w", &acp.RequestError{Code: -32000, Message: "Authentication required: upstream had 500 internal error"}),
			retryable: false,
			kind:      "auth",
		},

		// ACP RequestError: client errors
		{
			name:      "ACP -32600 invalid request",
			err:       &acp.RequestError{Code: -32600, Message: "Invalid request"},
			retryable: false,
			kind:      "client_error",
		},
		{
			name:      "ACP -32700 parse error",
			err:       &acp.RequestError{Code: -32700, Message: "Parse error"},
			retryable: false,
			kind:      "client_error",
		},

		// ACP RequestError: request cancelled
		{
			name:      "ACP -32800 request cancelled",
			err:       &acp.RequestError{Code: -32800, Message: "Request cancelled"},
			retryable: false,
			kind:      "canceled",
		},

		// ACP RequestError: internal error (-32603) with rate limit signals
		{
			name:      "ACP -32603 with 429 rate limit",
			err:       &acp.RequestError{Code: -32603, Message: "Error: 429 Too Many Requests"},
			retryable: true,
			kind:      "rate_limit",
		},
		{
			name:      "ACP -32603 with rate limit phrase",
			err:       &acp.RequestError{Code: -32603, Message: "rate limit exceeded, please retry"},
			retryable: true,
			kind:      "rate_limit",
		},
		{
			name:      "ACP -32603 overloaded",
			err:       &acp.RequestError{Code: -32603, Message: "model overloaded, please try again"},
			retryable: true,
			kind:      "rate_limit",
		},

		// ACP -32603: 429 + insufficient_quota → permanent wins
		{
			name:      "ACP -32603 with 429 and insufficient_quota",
			err:       &acp.RequestError{Code: -32603, Message: "429 error: insufficient_quota exceeded"},
			retryable: false,
			kind:      "quota",
		},
		{
			name:      "ACP -32603 with rate_limit phrase and quota exceeded",
			err:       &acp.RequestError{Code: -32603, Message: "rate limit: quota_exceeded for your plan"},
			retryable: false,
			kind:      "quota",
		},

		// ACP -32603: quota (no rate limit signal)
		{
			name:      "ACP -32603 with insufficient_quota only",
			err:       &acp.RequestError{Code: -32603, Message: "insufficient_quota: your credits have run out"},
			retryable: false,
			kind:      "quota",
		},

		// ACP -32603: 5xx → retryable
		{
			name:      "ACP -32603 with 503",
			err:       &acp.RequestError{Code: -32603, Message: "503 Service Unavailable"},
			retryable: true,
			kind:      "server_error",
		},
		{
			name:      "ACP -32603 with 500",
			err:       &acp.RequestError{Code: -32603, Message: "upstream returned 500 Internal Server Error"},
			retryable: true,
			kind:      "server_error",
		},

		// ACP -32603: outer 5xx not flipped by incidental nested 4xx
		{
			name:      "ACP -32603 outer 5xx with incidental 403 in body",
			err:       &acp.RequestError{Code: -32603, Message: "503 Service Unavailable: upstream auth check returned 403"},
			retryable: true,
			kind:      "server_error",
		},

		// ACP -32603: auth phrase + incidental 5xx → auth wins (permanent)
		{
			name:      "ACP -32603 auth phrase with incidental 5xx",
			err:       &acp.RequestError{Code: -32603, Message: "Authentication required: upstream 500 gateway error"},
			retryable: false,
			kind:      "auth",
		},

		// ACP -32603: auth-only (no 5xx alongside it) → permanent
		{
			name:      "ACP -32603 with 401 only",
			err:       &acp.RequestError{Code: -32603, Message: "upstream returned 401 Unauthorized"},
			retryable: false,
			kind:      "auth",
		},
		{
			name:      "ACP -32603 with forbidden",
			err:       &acp.RequestError{Code: -32603, Message: "forbidden: invalid API key"},
			retryable: false,
			kind:      "auth",
		},

		// ACP -32603: default (no recognizable signal) → retryable (internal error default)
		{
			name:      "ACP -32603 no recognizable signal",
			err:       &acp.RequestError{Code: -32603, Message: "Internal error"},
			retryable: true,
			kind:      "server_error",
		},

		// Text-only: quota
		{
			name:      "text: quota exceeded",
			err:       errors.New("quota_exceeded: you have used all your credits"),
			retryable: false,
			kind:      "quota",
		},

		// Text-only: auth
		{
			name:      "text: 401 Unauthorized",
			err:       errors.New("HTTP 401 Unauthorized: invalid token"),
			retryable: false,
			kind:      "auth",
		},
		{
			name:      "text: unauthorized phrase",
			err:       errors.New("unauthorized: the provided API key is not valid"),
			retryable: false,
			kind:      "auth",
		},

		// Text-only: transport drop
		{
			name:      "text: connection refused",
			err:       errors.New("dial tcp: connection refused"),
			retryable: true,
			kind:      "transport_drop",
		},

		// Incidental numbers: word-boundary safety
		{
			name: "incidental: port 50043 does not trigger 5xx",
			// "50043" contains "500" but there is no word boundary after the three-digit span.
			err:       errors.New("dial tcp 127.0.0.1:50043: connection refused"),
			retryable: true, // classified as transport_drop via "connection refused", NOT via 5xx
			kind:      "transport_drop",
		},
		{
			name: "incidental: number in path does not trigger 4xx",
			// "42900" contains "429" but no word boundary after the span.
			err:       errors.New("request to /v1/messages took 42900ms"),
			retryable: false,
			kind:      "unknown",
		},

		// Unknown errors
		{
			name:      "unknown error returns Retryable=false Kind=unknown",
			err:       errors.New("some completely unrecognized error message"),
			retryable: false,
			kind:      "unknown",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cls := ClassifyTransportError(tc.err)
			if cls.Retryable != tc.retryable {
				t.Errorf("Retryable = %v, want %v (Kind=%q Reason=%q)", cls.Retryable, tc.retryable, cls.Kind, cls.Reason)
			}
			if cls.Kind != tc.kind {
				t.Errorf("Kind = %q, want %q (Retryable=%v Reason=%q)", cls.Kind, tc.kind, cls.Retryable, cls.Reason)
			}
			if cls.Reason == "" {
				t.Errorf("Reason must be non-empty")
			}
		})
	}
}

// TestClassifyTransportErrorChainWalk verifies that errors.As and errors.Is walk
// the full error chain and do not stop at the outer wrapping layer.
func TestClassifyTransportErrorChainWalk(t *testing.T) {
	// errors.Is chain: DeadlineExceeded wrapped three levels deep.
	deepDeadline := fmt.Errorf("a: %w", fmt.Errorf("b: %w", fmt.Errorf("c: %w", context.DeadlineExceeded)))
	cls := ClassifyTransportError(deepDeadline)
	if !cls.Retryable || cls.Kind != "deadline" {
		t.Errorf("deep DeadlineExceeded: got Retryable=%v Kind=%q, want true/deadline", cls.Retryable, cls.Kind)
	}

	// errors.As chain: net.Error wrapped inside an ACP-style fmt.Errorf.
	wrappedNet := fmt.Errorf("acp prompt: %w", &fakeNetError{msg: "connection reset", timeout: false})
	cls = ClassifyTransportError(wrappedNet)
	if !cls.Retryable || cls.Kind != "network" {
		t.Errorf("wrapped net.Error: got Retryable=%v Kind=%q, want true/network", cls.Retryable, cls.Kind)
	}

	// errors.As chain: *acp.RequestError wrapped inside a generic error.
	wrappedACP := fmt.Errorf("transport layer: %w", &acp.RequestError{Code: -32000, Message: "auth"})
	cls = ClassifyTransportError(wrappedACP)
	if cls.Retryable || cls.Kind != "auth" {
		t.Errorf("wrapped acp -32000: got Retryable=%v Kind=%q, want false/auth", cls.Retryable, cls.Kind)
	}
}

// TestClassifyTransportErrorUnknownRetryableFalse ensures that any error not
// matching a known pattern returns Retryable=false with Kind="unknown".
func TestClassifyTransportErrorUnknownRetryableFalse(t *testing.T) {
	for _, msg := range []string{
		"some novel adapter error",
		"exit status 137",
		"runtime: out of memory",
	} {
		cls := ClassifyTransportError(errors.New(msg))
		if cls.Retryable {
			t.Errorf("unrecognized error %q: Retryable must be false, got true", msg)
		}
		if cls.Kind != "unknown" {
			t.Errorf("unrecognized error %q: Kind=%q, want 'unknown'", msg, cls.Kind)
		}
		if cls.Reason == "" {
			t.Errorf("unrecognized error %q: Reason must be non-empty", msg)
		}
	}
}
