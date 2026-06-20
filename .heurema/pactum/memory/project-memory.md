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

### mem_011 - Stagger the cold start of same-model reviewer groups in the review panel fan-...
- Run: run_20260612_175035
- Freshness: fresh
- Files: docs/agents.md, docs/cost-budget-design.md, internal/agents/acp_transport.go, internal/agents/acp_transport_test.go, internal/agents/executor_test.go, internal/agents/runner.go, internal/agents/types.go, internal/app/agent_attempt.go, internal/app/app.go, internal/app/review_loop.go, internal/app/review_stagger_test.go
- Summary: Reviewed run run_20260612_175035 with gate status needs_review and review status approved. Goal: Stagger the cold start of same-model reviewer groups in the review panel fan-out to stop paying duplicate prompt-cache write premiums. Backg...
- Candidate: runs/run_20260612_175035/memory/memory-candidate.json

### mem_012 - Capture Codex token usage from ACP usage_update metadata and add per-engine A...
- Run: run_20260612_230148
- Freshness: fresh
- Files: docs/agents.md, docs/cost-budget-design.md, internal/agents/acp_transport.go, internal/agents/acp_transport_test.go
- Summary: Reviewed run run_20260612_230148 with gate status needs_review and review status approved. Goal: Capture Codex token usage from ACP usage_update metadata and add per-engine ACP adapter command overrides.
- Candidate: runs/run_20260612_230148/memory/memory-candidate.json

### mem_013 - Dogfood Pactum with a local codex-acp adapter that returns official ACP Promp...
- Run: run_20260613_083052
- Freshness: fresh
- Files: .heurema/pactum/runs/run_20260613_083052/ledger/usage.jsonl, internal/agents/acp_transport.go
- Summary: Reviewed run run_20260613_083052 with gate status passed and review status approved. Goal: Dogfood Pactum with a local codex-acp adapter that returns official ACP PromptResponse.Usage, and align Pactum's ACP usage code/docs with response...
- Candidate: runs/run_20260613_083052/memory/memory-candidate.json

### mem_014 - Run a no-edit Pactum execution through the rebuilt Pactum binary and local co...
- Run: run_20260613_090426
- Freshness: unknown
- Files: none
- Summary: Reviewed run run_20260613_090426 with gate status passed and review status approved. Goal: Run a no-edit Pactum execution through the rebuilt Pactum binary and local codex-acp adapter to verify captured and coherent ACP PromptResponse.Us...
- Candidate: runs/run_20260613_090426/memory/memory-candidate.json

### mem_015 - Extract a shared bounded-loop engine into a new internal/loop package and por...
- Run: run_20260616_192306
- Freshness: fresh
- Files: docs/contract-review-design.md, internal/app/app.go, internal/app/contract_review.go, internal/app/contract_review_test.go, internal/loop/loop.go, internal/loop/loop_test.go
- Summary: Reviewed run run_20260616_192306 with gate status needs_review and review status approved. Goal: Extract a shared bounded-loop engine into a new internal/loop package and port the contract_review loop onto it, behaviour-preserving, plus ...
- Candidate: runs/run_20260616_192306/memory/memory-candidate.json

### mem_016 - Port the code-review loop (internal/app/review_loop.go) onto the existing int...
- Run: run_20260617_060708
- Freshness: fresh
- Files: docs/backlog.md, internal/app/review_loop.go, internal/app/review_loop_test.go
- Summary: Reviewed run run_20260617_060708 with gate status needs_review and review status approved. Goal: Port the code-review loop (internal/app/review_loop.go) onto the existing internal/loop engine, behaviour-preserving. The engine internal/lo...
- Candidate: runs/run_20260617_060708/memory/memory-candidate.json

### mem_017 - Rework the pactum config to the new pipeline shape and wire it through the ex...
- Run: run_20260617_090147
- Freshness: fresh
- Files: docs/agents.md, internal/app/agent_attempt_transport_test.go, internal/app/agent_resolve.go, internal/app/app.go, internal/app/app_test.go, internal/app/clarify_loop.go, internal/app/clarify_loop_test.go, internal/app/clarify_round.go, internal/app/cli.go, internal/app/config.go, internal/app/config_test.go, internal/app/contract_draft.go, internal/app/contract_review.go, internal/app/contract_review_test.go, internal/app/execute.go, internal/app/gate.go, internal/app/gate_test.go, internal/app/resolve.go, internal/app/review.go, internal/app/review_fix.go, internal/app/review_loop.go, internal/app/review_loop_test.go
- Summary: Reviewed run run_20260617_090147 with gate status needs_review and review status approved. Goal: Rework the pactum config to the new pipeline shape and wire it through the existing code; behaviour-preserving (no new runtime capability), ...
- Candidate: runs/run_20260617_090147/memory/memory-candidate.json

### mem_018 - Add a new top-level CLI command 'pactum usage [run_id]' that summarizes agent...
- Run: run_20260617_115334
- Freshness: fresh
- Files: docs/backlog.md, docs/cost-budget-design.md, internal/app/cli.go, internal/app/commands.go, internal/app/usage.go, internal/app/usage_test.go
- Summary: Reviewed run run_20260617_115334 with gate status needs_review and review status approved. Goal: Add a new top-level CLI command 'pactum usage [run_id]' that summarizes agent token usage from a run's ledger/usage.jsonl — a plain 'where d...
- Candidate: runs/run_20260617_115334/memory/memory-candidate.json

### mem_019 - Surface cache reuse and effective cost in the existing 'pactum usage' command...
- Run: run_20260617_150710
- Freshness: fresh
- Files: docs/token-efficiency-research.md, internal/app/usage.go, internal/app/usage_test.go
- Summary: Reviewed run run_20260617_150710 with gate status needs_review and review status approved. Goal: Surface cache reuse and effective cost in the existing 'pactum usage' command. The data is already captured and computed — it is just not sh...
- Candidate: runs/run_20260617_150710/memory/memory-candidate.json

### mem_020 - Add the plan-DAG schema and structural validation to the contract — slice 1 o...
- Run: run_20260617_182429
- Freshness: fresh
- Files: docs/flow.md, internal/app/clarify.go, internal/app/contract.go, internal/app/contract_plan_test.go, internal/app/run.go
- Summary: Reviewed run run_20260617_182429 with gate status needs_review and review status approved. Goal: Add the plan-DAG schema and structural validation to the contract — slice 1 of the plan-DAG arc (see docs/contract-plan-dag-design.md). SCHE...
- Candidate: runs/run_20260617_182429/memory/memory-candidate.json

### mem_021 - Make pactum's code-review loop never silently drop reviewer findings, and rec...
- Run: run_20260617_210449
- Freshness: fresh
- Files: docs/agents.md, internal/app/agent_output_test.go, internal/app/contract_review.go, internal/app/review.go, internal/app/review_loop.go, internal/app/review_loop_test.go, internal/app/review_proposals.go, internal/app/review_proposals_test.go, internal/app/review_test.go
- Summary: Reviewed run run_20260617_210449 with gate status needs_review and review status approved. Goal: Make pactum's code-review loop never silently drop reviewer findings, and recover automatically when a reviewer omits the structured finding...
- Candidate: runs/run_20260617_210449/memory/memory-candidate.json

### mem_022 - Plan-DAG slice 2: the contract drafter emits an optional plan.tasks[] DAG, an...
- Run: run_20260618_054400
- Freshness: fresh
- Files: docs/flow.md, internal/app/cli.go, internal/app/commands.go, internal/app/contract_draft.go, internal/app/plan.go, internal/app/plan_test.go, internal/app/run.go
- Summary: Reviewed run run_20260618_054400 with gate status needs_review and review status approved. Goal: Plan-DAG slice 2: the contract drafter emits an optional plan.tasks[] DAG, and add `pactum plan show` to render the static DAG. This is slic...
- Candidate: runs/run_20260618_054400/memory/memory-candidate.json

### mem_023 - Plan-DAG slice 3: the plan immune system (entry) — static non-vacuous validat...
- Run: run_20260618_071101
- Freshness: fresh
- Files: docs/flow.md, internal/app/app.go, internal/app/cli.go, internal/app/commands.go, internal/app/config.go, internal/app/config_test.go, internal/app/contract.go, internal/app/contract_plan_test.go, internal/app/plan_review.go, internal/app/plan_review_test.go, internal/app/plan_test.go, internal/app/run.go
- Summary: Reviewed run run_20260618_071101 with gate status needs_review and review status approved. Goal: Plan-DAG slice 3: the plan immune system (entry) — static non-vacuous validation + a single-pass `plan_review` pipeline stage. This is slice...
- Candidate: runs/run_20260618_071101/memory/memory-candidate.json

### mem_024 - Plan-DAG slice 4b: the minimal topological execute loop — run a contract's pl...
- Run: run_20260618_124220
- Freshness: fresh
- Files: docs/agents.md, docs/flow.md, internal/app/execute_dag.go, internal/app/execute_dag_test.go
- Summary: Reviewed run run_20260618_124220 with gate status passed and review status approved. Goal: Plan-DAG slice 4b: the minimal topological execute loop — run a contract's plan.tasks[] DAG node-by-node, sequentially, unattended. This is slice ...
- Candidate: runs/run_20260618_124220/memory/memory-candidate.json

### mem_025 - Make the review loop (both contract_review and code_review, which share inter...
- Run: run_20260619_123215
- Freshness: fresh
- Files: docs/flow.md, internal/app/contract.go, internal/app/contract_review.go, internal/app/contract_review_test.go, internal/app/resolve.go, internal/app/review.go, internal/app/review_loop.go, internal/app/review_loop_test.go, internal/app/review_test.go, internal/app/run.go
- Summary: Reviewed run run_20260619_123215 with gate status needs_review and review status approved. Goal: Make the review loop (both contract_review and code_review, which share internal/app/review_loop.go) never silently terminate with an OPEN B...
- Candidate: runs/run_20260619_123215/memory/memory-candidate.json

### mem_026 - Add an absolute per-attempt WALL-CLOCK CAP to the ACP agent transport so an a...
- Run: run_20260619_155159
- Freshness: fresh
- Files: docs/agents.md, docs/backlog.md, internal/agents/acp_transport.go, internal/agents/acp_transport_wallclock_test.go, internal/agents/acp_transport_wallclock_unix_test.go, internal/agents/runner.go, internal/agents/types.go, internal/app/agent_attempt.go, internal/app/agent_attempt_timeout_test.go, internal/app/agent_attempt_transport_test.go, internal/app/clarify_loop.go, internal/app/clarify_round.go, internal/app/config.go, internal/app/config_test.go, internal/app/contract_draft.go, internal/app/contract_review.go, internal/app/execute.go, internal/app/process.go, internal/app/review_fix.go, internal/app/review_loop.go
- Summary: Reviewed run run_20260619_155159 with gate status needs_review and review status approved. Goal: Add an absolute per-attempt WALL-CLOCK CAP to the ACP agent transport so an agent attempt can never hang indefinitely, even when it trickles...
- Candidate: runs/run_20260619_155159/memory/memory-candidate.json

### mem_027 - Give the CONTRACT-REVIEW loop the same operator finding-resolution that CODE-...
- Run: run_20260620_072128
- Freshness: fresh
- Files: docs/contract-review-design.md, internal/app/cli.go, internal/app/cli_grammar_test.go, internal/app/commands.go, internal/app/contract.go, internal/app/contract_review.go, internal/app/contract_review_resolve_test.go, internal/app/contract_review_test.go, internal/app/resolve.go, internal/app/run.go
- Summary: Reviewed run run_20260620_072128 with gate status needs_review and review status approved. Goal: Give the CONTRACT-REVIEW loop the same operator finding-resolution that CODE-REVIEW already has, so a contract that ends `blockers_open` but...
- Candidate: runs/run_20260620_072128/memory/memory-candidate.json

### mem_028 - Make the pactum agent skill self-sufficient and one-command installable, so a...
- Run: run_20260620_063231
- Freshness: fresh
- Files: assets/agent-skills/pactum/SKILL.md, assets/agent-skills/pactum/references/install.md, assets/agent-skills/pactum/references/safety.md, assets/agent-skills/pactum/references/workflow.md, assets/embed.go, docs/agent-skill.md, docs/skill-install.md, internal/app/cli.go, internal/app/commands.go, internal/app/resolve.go, internal/app/skill.go, internal/app/skill_test.go, internal/docs/skill_test.go
- Summary: Reviewed run run_20260620_063231 with gate status needs_review and review status approved. Goal: Make the pactum agent skill self-sufficient and one-command installable, so a stranger driving pactum through their coding agent (Claude Cod...
- Candidate: runs/run_20260620_063231/memory/memory-candidate.json
