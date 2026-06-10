# Task

Rework model pinning: replace the global agents.executor_model/reviewer_model strings with a per-agent agents.models map (per agent: executor/reviewer pins), so a model pin only ever applies to its own agent and a multi-agent review panel composes with per-agent models

Generated: 2026-06-09T21:09:26Z
