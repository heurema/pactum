# Contract Review Fixer Prompt

You are fixing a software change contract to address blocking review findings.

Current contract version: a9bdffcfaf6656d819859ea68f0d47918e920a5e305f7747c109f7296fecb5c2

## Current Contract

**Goal**: Add a transport retry/error classifier and apply automatic retries to READ-ONLY agent stages only. This is v1: write-stage (write-enabled execute/fixer) retry is explicitly OUT of scope and deferred to a later slice, because retrying after a possible write risks duplicate writes.

First understand the actual transport error shapes: read internal/agents/acp_transport.go and internal/agents/runner.go to see what errors agentTransport().Run actually returns (ACP adapter subprocess failures, JSON-RPC errors, network/timeout/context-cancellation, and model-API errors such as rate-limit/quota that the adapter forwards as text). Classify THOSE real shapes, not a hypothetical HTTP client.

Create internal/agents/retry.go with a TransportErrorClass type (fields: Retryable bool, Kind string, Reason string) and a function ClassifyTransportError(error) TransportErrorClass. The classifier must: walk the FULL error chain (errors.Unwrap / errors.As), not just the top-level message; classify as transient/retryable = transport drops, network errors, timeouts and context-deadline-exceeded, rate-limit/429/overloaded, and 5xx; classify as permanent/non-retryable = quota exhausted, auth/permission failures (401/403/insufficient_quota), and clear client errors; when both a transient and a permanent signal appear, give transient/5xx precedence only when it is the genuine outermost cause (do not let an incidentally-mentioned nested 4xx flip a real 5xx); be careful not to match incidental numbers in unrelated text. Also treat an empty/no-output transport result as retryable up to a small cap.

Apply retries in runAgentAttemptLifecycle (internal/app/agent_attempt.go) around the single agentTransport().Run call — NOT inside the loop engine (internal/loop). Retry ONLY when the stage is read-only (RunRequest.ReadOnly == true) AND the error classifies retryable (or the result was empty), using jittered exponential backoff with a small max-attempts cap. For write-enabled stages do NOT retry — keep current behavior exactly. Record each retry decision as a durable JSONL artifact line under the attempt directory (schema pactum.agent_retry_decision.v1alpha1 with fields: stage, attempt_id, kind, reason, retryable, read_only, attempt_number, delay_ms).

Scope: new internal/agents/retry.go (with tests) and internal/app/agent_attempt.go (with tests). Do NOT touch acp_transport.go write-boundary/permission handling, the gate, the loop engine, provider/transport selection, or add any write-stage retry. Acceptance: ClassifyTransportError has table-driven tests covering full-chain unwrap, transient cases (network/timeout/rate-limit/5xx), permanent cases (quota/auth), transient-over-incidental-nested-4xx precedence, and incidental-number non-matching; runAgentAttemptLifecycle retries a transient failure on a read-only stage and then succeeds, does NOT retry a permanent failure, does NOT retry any failure on a write-enabled stage, and writes retry-decision artifacts; make check passes.

**Scope in**:
  - Add internal/agents/retry.go with TransportErrorClass {Retryable, Kind, Reason} and ClassifyTransportError(error) implemented against the real ACP transport/runner error shapes.
  - Add internal/agents classifier tests for wrapped chains, errors.As-compatible network/timeout errors, context deadline errors, ACP/JSON-RPC/adapter text errors, rate-limit/429/overloaded, 5xx, quota/auth/401/403/insufficient_quota, clear client errors, incidental-number non-matches, and mixed transient/permanent precedence.
  - Update runAgentAttemptLifecycle in internal/app/agent_attempt.go to retry only around the single a.agentTransport().Run call, only when cfg.ReadOnly is true and the transport failure or empty/no-output result is retryable.
  - Use bounded jittered exponential backoff for read-only retries while keeping tests deterministic and fast.
  - Append durable retry decision JSONL records under the attempt directory with schema pactum.agent_retry_decision.v1alpha1 and fields stage, attempt_id, kind, reason, retryable, read_only, attempt_number, and delay_ms.
  - Add internal/app tests using fake transports for read-only transient retry success, read-only permanent no-retry, write-enabled no-retry, empty/no-output retry cap, and retry-decision artifact contents.

**Scope out**:
  - Changing the contract goal or answering clarification questions.
  - Retrying write-enabled stages, including execute runs and review-fix or contract-fixer stages.
  - Moving retry logic into internal/loop or changing loop-engine retry/corrective behavior.
  - Changing ACP write-boundary or permission enforcement, adapter command selection, provider/transport registry selection, gate behavior, prompt building, review proposal parsing, or usage accounting.
  - Behavioral changes to internal/agents/acp_transport.go or internal/agents/runner.go beyond preserving compatibility with the new classifier.

**Acceptance criteria**:
  - ClassifyTransportError returns Retryable=true with stable non-empty Kind and Reason for transport drops, network errors, net.Error timeouts, context.DeadlineExceeded, rate-limit/429/overloaded text (without an accompanying insufficient_quota signal in the same error), and genuine outermost 5xx signals.
  - ClassifyTransportError returns Retryable=false with stable non-empty Kind and Reason for quota exhausted/insufficient_quota (including HTTP 429 responses whose body carries an insufficient_quota signal), auth or permission failures including 401/403, and clear non-retryable client errors.
  - ClassifyTransportError returns Retryable=false with Kind='unknown' and a non-empty Reason for any error that does not match a known transient or permanent pattern, providing a conservative default that prevents unrecognized errors from triggering retries.
  - Classifier tests prove the full error chain is inspected via errors.Unwrap/errors.As, incidental numbers in unrelated text are ignored, an outer genuine 5xx is not flipped by an incidental or nested 4xx, genuine permanent outer causes remain non-retryable, and an unrecognized error returns Retryable=false with Kind='unknown'.
  - Classifier tests include a case where the same error contains both a 429/rate-limit signal and an insufficient_quota signal, and verify that ClassifyTransportError returns Retryable=false (permanent wins).
  - For a read-only lifecycle attempt whose first transport call fails with a retryable transient error and second call succeeds, the fake transport is called twice and result/last-result artifacts reflect the successful call.
  - For a read-only lifecycle attempt with a permanent transport error, the fake transport is called once and existing failure reporting behavior is preserved.
  - For a write-enabled lifecycle attempt with a retryable transient transport error, the fake transport is called once, no retry is scheduled, and existing write-stage failure behavior is preserved.
  - For an empty/no-output transport result on a read-only stage, retries stop after at most three total transport calls (the initial call plus at most two retries) and cannot loop indefinitely; if all calls return empty results the final empty result is returned as-is.
  - When a read-only transport call returns context.Canceled and RunResult.TimedOut is true (idle timeout signaled by the runner canceling the context after the idle deadline), the lifecycle treats the attempt as retryable regardless of what ClassifyTransportError returns for the context.Canceled error alone.
  - When context.Canceled arrives with RunResult.TimedOut false and without a transport-drop or provider transient signal that ClassifyTransportError independently marks Retryable=true, the lifecycle does NOT retry.
  - RunResult.WallClockTimeout true or RunResult.CompletedDespiteTimeout true suppresses any retry on any stage including read-only stages; the result is returned as-is even if the error or empty-output heuristic would otherwise indicate retryable.
  - Retry-decision JSONL artifact lines are valid JSON, live under the corresponding attempt directory, use schema pactum.agent_retry_decision.v1alpha1, include the required fields with no absolute local paths, and record both retry and final no-retry decisions with delay_ms set to 0 when no retry is scheduled.
  - Existing ACP read-only write/permission enforcement, execute attempt, review attempt, and timeout tests continue to pass.

**Validation commands**:
  - go test ./internal/agents ./internal/app
  - make check

**Assumptions**:
  - The retry-decision artifact filename is retry-decisions.jsonl in each attempt directory unless an existing repository convention requires a different name.
  - The v1 max-attempts cap is three total transport calls per read-only lifecycle attempt: the initial call plus at most two retries.
  - attempt_number is a one-based transport-call number within the same Pactum attempt directory, not a new Pactum attempt ID.
  - A retry decision record is written for every evaluated failed or empty/no-output transport result, including decisions not to retry.
  - Plain context.Canceled without a coincident RunResult.TimedOut=true, transport-drop, or provider transient signal is not independently considered retryable because it may represent intentional cancellation. When context.Canceled coincides with RunResult.TimedOut=true, the lifecycle treats it as a retryable idle timeout rather than intentional cancellation.
  - An empty/no-output transport result means the transport Run call returns a nil error and RunResult.StdoutPath either does not exist or its content consists entirely of whitespace (zero non-whitespace bytes after trimming). A non-nil error is always routed through ClassifyTransportError rather than the empty-result path; a non-zero exit code alone does not constitute an empty result. A legitimately-empty response (e.g., a read-only stage that genuinely produces no text) is retried up to the max-attempts cap and returned as-is if all attempts remain empty.
  - When a 429 response carries an insufficient_quota body signal, the permanent/non-retryable classification wins over the transient 429 rate-limit classification. A plain 429 or overloaded signal without an insufficient_quota body signal remains transient/retryable.

## Blocking Findings to Address

1. [opus/completeness] No acceptance criterion covers a read-only stage whose retryable transient ERROR persists through the max-attempts cap. The empty-output path specifies the terminus ('cannot loop indefinitely', 'final empty result returned as-is') but the error path — the primary feature path — has only a 'fails then succeeds' case. The loop bound and the final returned value (last error vs last RunResult) for error-exhaustion are unstated, making that path un-gatable.
   Evidence: Acceptance: 'first transport call fails with a retryable transient error and second call succeeds' (success case only) vs the empty case 'retries stop after at most three total transport calls … if all calls return empty results the final empty result is returned as-is.' No equivalent terminal criterion for a persistently-failing transient error.
2. [opus/scope-fidelity] scope.out forbids retrying the 'contract-fixer' stage, but scope.in specifies the retry gate as `RunRequest.ReadOnly == true`, and the contract_fixer lifecycle config sets ReadOnly: true (internal/app/contract_review.go:767). Under the specified gate, contract_fixer WOULD be retried — contradicting scope.out. The two cannot both hold given the code. The mismatch is also silent: the write-stage no-retry acceptance test exercises a ReadOnly==false stage, so tests pass green while contract_fixer is retried in violation of scope.out. The contract must either (a) drop contract-fixer from the no-retry list (a ReadOnly:true fixer cannot duplicate-write on retry since the agent does not write files), or (b) add an explicit stage-name exclusion to the gate — but it currently specifies neither, leaving the implementer with an unresolvable conflict.
   Evidence: Goal/scope.in: 'Retry ONLY when the stage is read-only (RunRequest.ReadOnly == true)'. scope.out: 'Retrying write-enabled stages, including execute runs and review-fix or contract-fixer stages.' Code: internal/app/contract_review.go:756/767 — Stage: "contract_fixer" with ReadOnly: true.
3. [opus/assumptions-surfaced] Unsurfaced contradiction on whether write-enabled stage failures get a retry-decision record. The Assumptions section says a record 'is written for every evaluated failed or empty/no-output transport result, including decisions not to retry,' but the goal and acceptance require write-stage behavior to be 'kept exactly'/'preserved.' Whether a write-enabled failure emits a new retry-decisions.jsonl line is therefore ambiguous, and a gater cannot deterministically verify the write-stage criterion.
   Evidence: Assumptions: 'A retry decision record is written for every evaluated failed or empty/no-output transport result, including decisions not to retry.' vs Goal: 'For write-enabled stages do NOT retry — keep current behavior exactly.' and Acceptance: 'existing write-stage failure behavior is preserved.'
4. [codex-xhigh/completeness] Some classifier acceptance criteria use broad terms without concrete observable boundaries, making the contract less gateable. In particular, 'clear client errors' and 'genuine outermost 5xx signals' should name representative error shapes or matching rules expected from the actual ACP/runner errors.
   Evidence: Acceptance criteria include: 'genuine outermost 5xx signals' and 'clear non-retryable client errors'; scope also says tests should cover 'clear client errors' without defining which real shapes qualify.
5. [codex-xhigh/assumptions-surfaced] The contract requires retry-decision JSONL artifacts but does not surface the assumption for artifact-write failure handling: whether a failed append should fail the lifecycle, suppress retry, be logged best-effort, or preserve the original transport result. This affects executor behavior and acceptance tests.
   Evidence: Scope in: "Append durable retry decision JSONL records under the attempt directory..." Acceptance: "Retry-decision JSONL artifact lines are valid JSON... and record both retry and final no-retry decisions..."

## Fixer Instructions

- Address each blocking finding by updating the relevant contract field.
- Do NOT change the goal field — it is out of scope for the fixer.
- Only include the contract fields you are changing in the output.
- base_version must exactly match the version shown above.

## Output

Output your reasoning, then a single JSON block with the revise payload:

```json
{
  "schema": "pactum.contract_revise.v1alpha1",
  "base_version": "a9bdffcfaf6656d819859ea68f0d47918e920a5e305f7747c109f7296fecb5c2",
  "contract": {
    "acceptance_criteria": ["...updated criteria..."],
    "validation": {"commands": ["...updated commands..."]}
  }
}
```

Omit any contract field you are not changing. Do not include the goal field.
