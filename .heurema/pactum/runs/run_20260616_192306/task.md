# Task

Extract a shared bounded-loop engine into a new internal/loop package and port the contract_review loop onto it, behaviour-preserving, plus add the ledger events contract_review currently lacks.

internal/loop provides Run(ctx, Limits, Step) (Outcome, error). Types: Limits{Max,Patience,Settle int}; RoundContext{Round int}; HumanExit{Reason string}; RoundResult{Clean bool, Progress bool, Human *HumanExit, Summary string}; Outcome{Reason string, Rounds int, Last RoundResult}; Step func(ctx, RoundContext)(RoundResult,error). Run owns ONLY the round counter and the stop machine, blind to performers and domain: per round call step; a Clean round grows a clean-streak (reset on non-clean) and Settle>0 && streak>=Settle => Reason 'settled'; a non-Clean non-Progress round grows a stale-streak (reset otherwise) and Patience>0 && streak>=Patience => 'stalemate'; Human!=nil => immediate 'human'; reaching Max => 'max'. Table tests for settled, stalemate, max, human, and error propagation.

Then replace contract_review.go's hand-rolled 'for round := 1; round <= maxRounds; round++' loop with a loop.Run call whose Step closure runs the existing per-round work (reviewer fan-out, finding merge, fixer apply, stop-signal computation) unchanged; stop semantics must match today exactly. Also emit the round/finished ledger events contract_review lacks today, mirroring what review_loop.go emits, adapted to the contract-review schema.

Constraints: do NOT change any config or config schema; do NOT touch review_loop.go or clarify_loop.go; do NOT add multi-model/best-of-N/judge behaviour. Validation: go build ./..., go test ./..., make check must pass.

Generated: 2026-06-16T19:23:06Z
