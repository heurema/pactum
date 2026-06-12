# Project Memory

## Accepted memory items

### mem_001 - Add an export command that dumps a run's full record as a single archive
- Run: run_20260610_220413
- Freshness: fresh
- Files: docs/flow.md, internal/app/cli.go, internal/app/commands.go, internal/app/export.go, internal/app/export_test.go
- Summary: Reviewed run run_20260610_220413 with gate status needs_review and review status approved. Goal: Add an export command that dumps a run's full record as a single archive
- Candidate: runs/run_20260610_220413/memory/memory-candidate.json

### mem_002 - Normalize the CLI command grammar for agent-first use: every stage exposes a ...
- Run: run_20260611_080712
- Freshness: stale
- Files: AGENTS.md, README.md, assets/agent-skills/pactum/SKILL.md, assets/agent-skills/pactum/references/safety.md, assets/agent-skills/pactum/references/workflow.md, docs/agent-skill.md, docs/agents.md, docs/flow.md, docs/install.md, docs/loop-architecture-design.md, docs/memory.md, docs/skill-install.md, internal/app/agent_attempt_transport_test.go, internal/app/agents_doctor.go, internal/app/agents_doctor_test.go, internal/app/app_test.go, internal/app/clarify_loop.go, internal/app/clarify_loop_test.go, internal/app/cli.go, internal/app/cli_grammar_test.go, internal/app/cli_v2_test.go, internal/app/commands.go, internal/app/contract_draft_test.go, internal/app/doctor.go, internal/app/doctor_test.go, internal/app/dogfood_hardening_test.go, internal/app/execute.go, internal/app/execute_report.go, internal/app/execute_report_test.go, internal/app/execute_test.go, internal/app/memory_prompt_boundary_test.go, internal/app/memory_selection_test.go, internal/app/memory_test.go, internal/app/prompt.go, internal/app/prompt_test.go, internal/app/resolve.go, internal/app/review.go, internal/app/review_fix.go, internal/app/review_test.go, internal/app/task.go, internal/app/task_clarify_test.go, internal/docs/docs_test.go, internal/docs/packaging_test.go, internal/docs/skill_test.go, scripts/smoke.sh
- Summary: Reviewed run run_20260611_080712 with gate status needs_review and review status approved. Goal: Normalize the CLI command grammar for agent-first use: every stage exposes a uniform verb set, duplicates and aliases are removed, hyphenate...
- Candidate: runs/run_20260611_080712/memory/memory-candidate.json

### mem_003 - Remove the interactive confirmation layer from the CLI: the consumer is an AI...
- Run: run_20260611_092851
- Freshness: stale
- Files: CHANGELOG.md, README.md, assets/agent-skills/pactum/SKILL.md, assets/agent-skills/pactum/references/safety.md, docs/agent-skill.md, docs/agents.md, docs/flow.md, go.mod, internal/app/agent_attempt.go, internal/app/agent_attempt_timeout_test.go, internal/app/agent_attempt_transport_test.go, internal/app/clarify.go, internal/app/clarify_loop.go, internal/app/clarify_loop_test.go, internal/app/clarify_suggest.go, internal/app/clarify_suggest_test.go, internal/app/clarify_test.go, internal/app/cli.go, internal/app/cli_v2_test.go, internal/app/commands.go, internal/app/confirm.go, internal/app/contract.go, internal/app/contract_draft.go, internal/app/contract_draft_test.go, internal/app/execute.go, internal/app/execute_report_test.go, internal/app/execute_test.go, internal/app/gate.go, internal/app/gate_test.go, internal/app/memory.go, internal/app/principal.go, internal/app/resolve.go, internal/app/review.go, internal/app/review_fix.go, internal/app/review_loop.go, internal/app/review_loop_test.go, internal/app/review_proposals.go, internal/app/review_test.go, internal/app/task.go, internal/app/task_clarify_test.go, internal/app/usage_test.go, internal/docs/docs_test.go
- Summary: Reviewed run run_20260611_092851 with gate status needs_review and review status approved. Goal: Remove the interactive confirmation layer from the CLI: the consumer is an AI agent relaying decisions already made in conversation, so the ...
- Candidate: runs/run_20260611_092851/memory/memory-candidate.json

### mem_004 - Tell the security truth in the user-facing docs and add a security policy. RE...
- Run: run_20260611_110706
- Freshness: fresh
- Files: README.md, SECURITY.md, docs/agents.md, internal/docs/docs_test.go
- Summary: Reviewed run run_20260611_110706 with gate status needs_review and review status approved. Goal: Tell the security truth in the user-facing docs and add a security policy. README's built-in agents section currently describes the codex ex...
- Candidate: runs/run_20260611_110706/memory/memory-candidate.json

### mem_005 - Make the CLI announce legal moves so an agent never guesses the pipeline stat...
- Run: run_20260611_113834
- Freshness: fresh
- Files: assets/agent-skills/pactum/SKILL.md, assets/agent-skills/pactum/references/workflow.md, docs/agent-skill.md, internal/app/affordances_test.go, internal/app/agent_attempt.go, internal/app/app.go, internal/app/app_test.go, internal/app/clarify.go, internal/app/clarify_loop.go, internal/app/clarify_suggest.go, internal/app/commands.go, internal/app/contract.go, internal/app/contract_draft.go, internal/app/errors.go, internal/app/execute.go, internal/app/gate.go, internal/app/map.go, internal/app/memory.go, internal/app/memory_freshness.go, internal/app/prompt.go, internal/app/prompt_test.go, internal/app/readiness.go, internal/app/resolve.go, internal/app/review.go, internal/app/review_fix.go, internal/app/review_fix_outcomes.go, internal/app/review_loop.go, internal/app/review_proposals.go, internal/app/status.go, internal/app/task.go
- Summary: Reviewed run run_20260611_113834 with gate status needs_review and review status approved. Goal: Make the CLI announce legal moves so an agent never guesses the pipeline state machine: (1) structured error envelopes — when a command fail...
- Candidate: runs/run_20260611_113834/memory/memory-candidate.json

### mem_006 - Smooth the pipeline so no command is pure ritual, then compress the agent ski...
- Run: run_20260611_160606
- Freshness: fresh
- Files: README.md, assets/agent-skills/pactum/references/workflow.md, docs/real-agent-execution-dogfood.md, internal/app/clarify_loop.go, internal/app/clarify_round.go, internal/app/commands.go, internal/app/gate.go, internal/app/prompt.go, internal/app/review.go
- Summary: Reviewed run run_20260611_160606 with gate status passed and review status approved. Goal: Smooth the pipeline so no command is pure ritual, then compress the agent skill against the final grammar. This is the last grammar break; hard re...
- Candidate: runs/run_20260611_160606/memory/memory-candidate.json

### mem_007 - Fix three valid external review findings. (1) pactum export must preserve its...
- Run: run_20260611_180550
- Freshness: fresh
- Files: docs/flow.md, docs/memory.md, internal/app/affordances_test.go, internal/app/errors.go, internal/app/export.go, internal/app/export_test.go, internal/app/export_unix.go, internal/app/export_windows.go, internal/app/memory.go, internal/app/memory_freshness_test.go, internal/app/memory_test.go, internal/app/resolve.go, internal/app/status.go, internal/app/task.go
- Summary: Reviewed run run_20260611_180550 with gate status needs_review and review status approved. Goal: Fix three valid external review findings. (1) pactum export must preserve its no-overwrite guarantee at rename time: today two concurrent ex...
- Candidate: runs/run_20260611_180550/memory/memory-candidate.json

### mem_008 - Two small CI hardening items from the project audit. (1) Vulnerability scanni...
- Run: run_20260611_191330
- Freshness: fresh
- Files: .github/workflows/ci.yml, CHANGELOG.md, Makefile, README.md, cmd/heurema-hygiene/main.go, cmd/heurema-hygiene/main_test.go, docs/backlog.md, docs/install.md, go.mod, go.sum
- Summary: Reviewed run run_20260611_191330 with gate status needs_review and review status approved. Goal: Two small CI hardening items from the project audit. (1) Vulnerability scanning: add govulncheck to the repository toolchain the same way de...
- Candidate: runs/run_20260611_191330/memory/memory-candidate.json

### mem_009 - Slice 1 of the agent file-navigation arc (design reference: docs/agent-file-n...
- Run: run_20260612_070427
- Freshness: fresh
- Files: CHANGELOG.md, assets/agent-skills/pactum/SKILL.md, docs/flow.md, internal/app/run.go, internal/search/query.go, internal/search/symbol_test.go
- Summary: Reviewed run run_20260612_070427 with gate status needs_review and review status approved. Goal: Slice 1 of the agent file-navigation arc (design reference: docs/agent-file-navigation-design.md). Make search results symbol-addressable so...
- Candidate: runs/run_20260612_070427/memory/memory-candidate.json

### mem_010 - Combined config and usage polish slice. (1) Hide the unfinished budget surfac...
- Run: run_20260612_161619
- Freshness: fresh
- Files: docs/backlog.md, docs/loop-architecture-design.md, internal/app/app_test.go, internal/app/map.go, internal/app/status.go, internal/app/usage.go, internal/app/usage_test.go
- Summary: Reviewed run run_20260612_161619 with gate status needs_review and review status approved. Goal: Combined config and usage polish slice. (1) Hide the unfinished budget surface: review.budget (mode/max_tokens) gates nothing real — remove ...
- Candidate: runs/run_20260612_161619/memory/memory-candidate.json
