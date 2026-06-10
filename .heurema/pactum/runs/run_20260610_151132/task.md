# Task

Simplify the agent registry to {name, model, effort}: drop the agent field, infer the engine solely from the required model (claude-*/aliases -> claude; gpt-*/codex-*/o<digit> -> codex), keep the at-least-one-agent rule, and reshape the workspace config atomically

Generated: 2026-06-10T15:11:32Z
