# Contract Draft

## Goal
In the autonomous review->fix loop, when a fixer round breaks the gate (validation/make check fails), stop with a distinct, meaningful terminal reason 'gate_failed' and escalate — instead of aborting as a generic infrastructure error and discarding the gate report. Today runReviewLoopGate calls GateRun, which on a 'failed' status writes the report to stdout AND returns a gateProcessError; runReviewLoopGate discards the report and the loop sets loopErr, terminating as terminal_reason 'error', indistinguishable from a real infra failure, with the gate-failure details lost

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260607_102704
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

## In scope
- runReviewLoopGate: when GateRun returns an error, detect a gateProcessError via errors.As (the gate RAN and produced a 'failed' report already written to the stdout buffer). In that case unmarshal the report from the buffer and return it ALONG WITH the gateProcessError, so the caller has both the populated report and the failed signal. For any other error (gate could not run / infra), return the empty report and the error as today
- Loop: record roundSummary.GateStatus and the gate report artifact even when the gate failed. If the gate ran and reported 'failed' (gateProcessError): set summary.TerminalReason = "gate_failed", append the round summary, and stop the loop as a CLEAN terminal (do NOT set loopErr). If the gate errored for any other reason: keep current behavior (loopErr -> terminal_reason "error"). On pass/needs_review: continue as today
- ReviewLoop returns nil for a gate_failed terminal (a legitimate stop outcome like stalemate/max_rounds), still writing the loop summary and the review_loop_finished event. The terminal_reason plus the recorded gate report artifact are the escalation signal; the human and JSON output make the gate failure and the gate report location clear
- Tests (deterministic, using the existing fake-runner loop harness): a fixer round whose result fails validation makes the loop stop with terminal_reason "gate_failed" (NOT "error"), records GateStatus "failed" + the gate report artifact in the round summary, and ReviewLoop returns nil; a genuine infrastructure gate error still terminates with terminal_reason "error" and a non-nil error
- Docs: update docs/flow.md (loop terminal reasons) and docs/backlog.md (resolve the gate-failure-in-loop item)

## Out of scope
- A gate-repair round (giving the fixer another chance to fix its own breakage) — this slice stops + escalates only; auto-repair is a separate idea
- Changing the CLI 'gate run' command's behavior or its non-zero exit on a failed gate
- Changing needs_review handling — only a 'failed' gate is terminal
- Native LLM API or provider abstraction; editing generated .heurema run artifacts

## Paths in scope
- internal/app/**
- docs/**


## Acceptance criteria
- A fixer that breaks make check in a round makes the loop stop with terminal_reason 'gate_failed' (distinct from 'error'); the round summary records GateStatus 'failed' and the gate report artifact; ReviewLoop returns nil; human/JSON output makes the gate failure + report location clear
- A genuine infrastructure gate error still terminates with terminal_reason 'error' and a non-nil error (unchanged)
- Gate pass / needs_review still continue the loop as before
- make check is green (incl. deadcode); go test -race ./... is clean

## Validation commands
- make check

## Assumptions
TBD

## Open questions
- None
