# Task

Add gofmt enforcement to the build gate. Update the Makefile so that 'make check' fails when any tracked Go file is not gofmt-formatted (add a gofmt -l step that errors on non-empty output), and apply gofmt -w to fix any existing formatting drift. Do not change unrelated gate steps.

Generated: 2026-06-15T19:20:13Z
