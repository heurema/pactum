//go:build !windows

package app

import (
	"os/exec"
	"syscall"
)

// setValidationCommandProcessGroup puts the validation command shell in its own
// process group so killValidationCommandProcessGroup can reach the whole tree.
func setValidationCommandProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killValidationCommandProcessGroup sends SIGKILL to the process group started
// by setValidationCommandProcessGroup, terminating the sh wrapper and any child
// processes it spawned.
func killValidationCommandProcessGroup(cmd *exec.Cmd) {
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
