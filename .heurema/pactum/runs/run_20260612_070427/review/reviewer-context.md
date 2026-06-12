# Reviewer Context

## Run
- Run id: run_20260612_070427
- Run status: contract_approved

## Contract
- Goal: Slice 1 of the agent file-navigation arc (design reference: docs/agent-file-navigation-design.md). Make search results symbol-addressable so an agent's first read of a large file is a ranged read instead of a grep-window-rewindow loop. (1) Plumb the per-symbol data the code-items index already stores — StartLine, EndLine, and Signature (see codeindex.Item) — through the search layer into code_item results: extend the FTS5 document metadata or result hydration so search.Result carries start_line, end_line, and signature for kind=code_item hits (other kinds leave them empty), expose them in pactum search human output as path:start-end and the signature, and in search --json. (2) Add a --symbol <name> filter to pactum search that restricts results to code_item hits whose symbol name matches (exact match preferred, case-insensitive; document the matching rule), so an agent can resolve a known identifier straight to its address. (3) Render the same addresses in the executor context: renderExecutorContext search-result lines gain the line range and signature for code_item hits (path:start-end — signature), and the prompt guidance text tells the agent to read symbol ranges directly instead of scanning whole files. (4) The map/search index rebuild stays deterministic; index schema or stored-shape changes must keep pactum map refresh reproducible — same tree in, same index out; bump internal schema markers if the stored shape changes rather than silently rereading stale rows. Tests pin: a code_item search result carries the correct range and signature; --symbol returns exactly the matching symbols; executor-context rendering shows ranged addresses; non-code_item results are unchanged.
- In scope:
  - Extend search indexing and result hydration so kind=code_item results can carry StartLine, EndLine, and Signature from codeindex.Item as start_line, end_line, and signature.
  - Update pactum search human output to render valid code_item ranges as path:start-end and include the signature when present.
  - Update pactum search --json so valid code_item symbol metadata is emitted as start_line, end_line, and signature while non-code_item result JSON remains unchanged.
  - Add pactum search --symbol <name> with exact case-insensitive matching against codeindex.Item.Name as exposed by search.Result.Title.
  - Allow --symbol without a positional query; when both query and --symbol are provided, apply the lexical query first and then restrict to exact symbol-name matches.
  - Reject --symbol combined with any --kind value other than omitted/default any or explicit code_item, with a clear usage error.
  - Return all duplicate exact symbol-name matches, ordered by existing deterministic ranking and tie-breakers and capped by --limit.
  - Update executor context search-result rendering so code_item hits with valid range metadata render as path:start-end and include the signature when present.
  - Update executor-context retrieval guidance to tell agents to read symbol ranges directly when ranged code_item results are available.
  - Update run-context search deduplication so distinct code_item document IDs or distinct valid start_line/end_line ranges are preserved.
  - Handle legacy or incompatible search index schemas as stale and instruct the user to run pactum map refresh.
  - Bump internal search index schema or stored-shape markers if the FTS5 stored shape changes.
  - Add focused tests for search result symbol metadata, --symbol behavior, executor-context rendering, stale schema handling, invalid metadata omission, and unchanged non-code_item output.
- Out of scope:
  - Do not change the contract goal.
  - Do not add embeddings, vector search, model-based context compression, or an LSP runtime dependency.
  - Do not implement pactum outline or contract-scoped skeleton rendering from later slices of docs/agent-file-navigation-design.md.
  - Do not change codeindex.Item extraction semantics except as needed to consume existing StartLine, EndLine, and Signature fields.
  - Do not include start_line, end_line, or signature fields in JSON results for repo_map, llms, wiki, file, or import hits.
  - Do not match --symbol against Parent, Package, Signature, CodeKind, import path, or synthesized qualified names.
  - Do not treat duplicate symbol names as an ambiguity error.
- Acceptance criteria:
  - A code_item search result created from a codeindex.Item with StartLine=3, EndLine=3, and Signature="type Runner struct" carries start_line=3, end_line=3, and signature="type Runner struct".
  - Human pactum search output for a code_item hit with valid range metadata shows the address as path:start-end and includes the signature.
  - pactum search --json includes start_line, end_line, and signature only for code_item hits with valid symbol metadata.
  - JSON output for repo_map, llms, wiki, file, and import hits does not gain start_line, end_line, or signature fields.
  - pactum search --symbol Runner works without a positional query and returns exact case-insensitive code_item title/name matches.
  - pactum search runner --symbol Runner applies the lexical query and then restricts results to exact case-insensitive symbol-name matches.
  - --symbol does not match Parent, Package, Signature, CodeKind, import path, or qualified names when the plain symbol name differs.
  - When multiple code_item results share the same exact symbol name, --symbol returns all matching results in deterministic result order subject to --limit.
  - pactum search --symbol Runner --kind file and other non-code_item kind filters fail with a clear usage error explaining that --symbol only applies to code_item results.
  - Executor context renders code_item search results with valid metadata as path:start-end and includes the signature when present.
  - Executor context preserves distinct code_item hits with different document IDs or different valid start_line/end_line ranges, even when path, title, and code_kind match.
  - Code_item hits with missing or invalid range metadata are still returned but do not render path:0-0 or dangling separators.
  - An incompatible or legacy search index schema is treated as stale and produces guidance to run pactum map refresh instead of silently reading stale rows.
  - Given the same tree and generated_at input, rebuilding the search index produces deterministic documents and result metadata.
- Validation commands:
  - go test ./internal/search
  - go test ./internal/app
  - make check

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 5
- Fresh: 4
- Stale: 1
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: passed
- Execution attempt id: attempt_002
- Execution exit code: 0
- Validation command results:
  - command_001: go test ./internal/search (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: go test ./internal/app (exit 0, timed out: false, result: gate/validation/command_002/result.json)
  - command_003: make check (exit 0, timed out: false, result: gate/validation/command_003/result.json)
- Change summary:
  - changed files:
    - none
  - new files:
    - none
  - missing files:
    - none

## Existing manual review
- Review status: pending
- Current findings summary: findings=0 open=0 resolved=0 blocking_open=0
- Existing findings:
  - none
- Existing resolutions:
  - none
- Proposal summary: pending=0 accepted=0 rejected=0
- Existing proposals:
  - none

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
- Execution result: execute/last-result.json

## Reviewer guidance
- This context is not complete semantic truth.
- Use `pactum search "<term>"` and inspect files before proposing findings.
- Do not invent changes.
- Do not approve automatically.
- If you are not certain an issue is real after verification, do not flag it.
