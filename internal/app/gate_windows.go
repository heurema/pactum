//go:build windows

package app

import "os/exec"

// setValidationCommandProcessGroup is a no-op on Windows: process groups are a
// Unix facility. Timeout kills only the sh process; its children may outlive it.
func setValidationCommandProcessGroup(cmd *exec.Cmd) {}

// killValidationCommandProcessGroup terminates the sh process. Without a Unix
// process group it cannot reach children that sh launched.
func killValidationCommandProcessGroup(cmd *exec.Cmd) {
	_ = cmd.Process.Kill()
}
