# Task

Run contract validation commands through a shell so shell features work. In internal/app/gate.go the gate tokenizes each validation command with strings.Fields and runs it directly via exec.Command(fields[0], fields[1:]...), so commands using shell features (command substitution $(...), quotes, pipes, globs, &&) are mis-parsed and fail. Change the gate to execute each validation command through the system shell (sh -c <command>) so the shell interprets the string. Preserve timeout/context handling and existing behavior for simple commands, and add a unit test covering a shell-feature command (e.g. command substitution or a quoted argument).

Generated: 2026-06-15T19:42:56Z
