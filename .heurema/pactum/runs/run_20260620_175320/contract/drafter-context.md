# Contract Drafter Context

## Run
- Run id: run_20260620_175320
- Run status: contract_draft

## Contract goal
Add a transport retry/error classifier and apply automatic retries to READ-ONLY agent stages only. This is v1: write-stage (write-enabled execute/fixer) retry is explicitly OUT of scope and deferred to a later slice, because retrying after a possible write risks duplicate writes.

First understand the actual transport error shapes: read internal/agents/acp_transport.go and internal/agents/runner.go to see what errors agentTransport().Run actually returns (ACP adapter subprocess failures, JSON-RPC errors, network/timeout/context-cancellation, and model-API errors such as rate-limit/quota that the adapter forwards as text). Classify THOSE real shapes, not a hypothetical HTTP client.

Create internal/agents/retry.go with a TransportErrorClass type (fields: Retryable bool, Kind string, Reason string) and a function ClassifyTransportError(error) TransportErrorClass. The classifier must: walk the FULL error chain (errors.Unwrap / errors.As), not just the top-level message; classify as transient/retryable = transport drops, network errors, timeouts and context-deadline-exceeded, rate-limit/429/overloaded, and 5xx; classify as permanent/non-retryable = quota exhausted, auth/permission failures (401/403/insufficient_quota), and clear client errors; when both a transient and a permanent signal appear, give transient/5xx precedence only when it is the genuine outermost cause (do not let an incidentally-mentioned nested 4xx flip a real 5xx); be careful not to match incidental numbers in unrelated text. Also treat an empty/no-output transport result as retryable up to a small cap.

Apply retries in runAgentAttemptLifecycle (internal/app/agent_attempt.go) around the single agentTransport().Run call — NOT inside the loop engine (internal/loop). Retry ONLY when the stage is read-only (RunRequest.ReadOnly == true) AND the error classifies retryable (or the result was empty), using jittered exponential backoff with a small max-attempts cap. For write-enabled stages do NOT retry — keep current behavior exactly. Record each retry decision as a durable JSONL artifact line under the attempt directory (schema pactum.agent_retry_decision.v1alpha1 with fields: stage, attempt_id, kind, reason, retryable, read_only, attempt_number, delay_ms).

Scope: new internal/agents/retry.go (with tests) and internal/app/agent_attempt.go (with tests). Do NOT touch acp_transport.go write-boundary/permission handling, the gate, the loop engine, provider/transport selection, or add any write-stage retry. Acceptance: ClassifyTransportError has table-driven tests covering full-chain unwrap, transient cases (network/timeout/rate-limit/5xx), permanent cases (quota/auth), transient-over-incidental-nested-4xx precedence, and incidental-number non-matching; runAgentAttemptLifecycle retries a transient failure on a read-only stage and then succeeds, does NOT retry a permanent failure, does NOT retry any failure on a write-enabled stage, and writes retry-decision artifacts; make check passes.

## Current contract fields
- In scope:
  - none
- Out of scope:
  - none
- Acceptance criteria:
  - none
- Validation commands:
  - none
- Assumptions:
  - none

## Answered clarifications
- None

## Repository context
# Repository Context

Generated: 2026-06-20T17:53:20Z

Map run: map_20260620_115144
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

Project map is unavailable at .heurema/pactum/map/repo-map.md.

## Search results
{
  "query": "Add a transport retry/error classifier and apply automatic retries to READ-ONLY agent stages only. This is v1: write-stage (write-enabled execute/fixer) retry is explicitly OUT of scope and deferred to a later slice, because retrying after a possible write risks duplicate writes.\n\nFirst understand the actual transport error shapes: read internal/agents/acp_transport.go and internal/agents/runner.go to see what errors agentTransport().Run actually returns (ACP adapter subprocess failures, JSON-RPC errors, network/timeout/context-cancellation, and model-API errors such as rate-limit/quota that the adapter forwards as text). Classify THOSE real shapes, not a hypothetical HTTP client.\n\nCreate internal/agents/retry.go with a TransportErrorClass type (fields: Retryable bool, Kind string, Reason string) and a function ClassifyTransportError(error) TransportErrorClass. The classifier must: walk the FULL error chain (errors.Unwrap / errors.As), not just the top-level message; classify as transient/retryable = transport drops, network errors, timeouts and context-deadline-exceeded, rate-limit/429/overloaded, and 5xx; classify as permanent/non-retryable = quota exhausted, auth/permission failures (401/403/insufficient_quota), and clear client errors; when both a transient and a permanent signal appear, give transient/5xx precedence only when it is the genuine outermost cause (do not let an incidentally-mentioned nested 4xx flip a real 5xx); be careful not to match incidental numbers in unrelated text. Also treat an empty/no-output transport result as retryable up to a small cap.\n\nApply retries in runAgentAttemptLifecycle (internal/app/agent_attempt.go) around the single agentTransport().Run call — NOT inside the loop engine (internal/loop). Retry ONLY when the stage is read-only (RunRequest.ReadOnly == true) AND the error classifies retryable (or the result was empty), using jittered exponential backoff with a small max-attempts cap. For write-enabled stages do NOT retry — keep current behavior exactly. Record each retry decision as a durable JSONL artifact line under the attempt directory (schema pactum.agent_retry_decision.v1alpha1 with fields: stage, attempt_id, kind, reason, retryable, read_only, attempt_number, delay_ms).\n\nScope: new internal/agents/retry.go (with tests) and internal/app/agent_attempt.go (with tests). Do NOT touch acp_transport.go write-boundary/permission handling, the gate, the loop engine, provider/transport selection, or add any write-stage retry. Acceptance: ClassifyTransportError has table-driven tests covering full-chain unwrap, transient cases (network/timeout/rate-limit/5xx), permanent cases (quota/auth), transient-over-incidental-nested-4xx precedence, and incidental-number non-matching; runAgentAttemptLifecycle retries a transient failure on a read-only stage and then succeeds, does NOT retry a permanent failure, does NOT retry any failure on a write-enabled stage, and writes retry-decision artifacts; make check passes.",
  "queries": [
    "retry/error",
    "execute/fixer",
    "internal/agents/acp_transport.go",
    "internal/agents/runner.go",
    "agentTransport().Run",
    "network/timeout/context-cancellation",
    "rate-limit/quota",
    "internal/agents/retry.go"
  ],
  "query_source": "task",
  "results": [],
  "warnings": [
    "Search index is stale. Run: pactum map refresh."
  ]
}

## Drafter guidance
- Propose only additions to the contract fields listed in the prompt.
- Do not change or restate the contract goal.
- Do not answer clarification questions.
- Do not edit files.
- Treat repository map/search context as navigation hints, not semantic truth.
