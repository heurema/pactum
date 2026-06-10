# Task

Fix ACP agent-message chunk gluing: separate streamed message chunks with a newline in the attempt log so a fenced suggestions block glued to prose is parseable, and warn when a suggestions-schema marker is present but zero blocks parse — a parse miss must not masquerade as convergence

Generated: 2026-06-10T21:10:53Z
