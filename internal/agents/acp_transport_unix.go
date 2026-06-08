//go:build unix

package agents

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the adapter in its own process group so the whole tree
// (the npx wrapper, the adapter, and the agent child it launches) can be reaped
// together by killProcessGroup. Process groups are a Unix facility; see
// acp_transport_other.go for the non-Unix fallback.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup reaps the adapter and the whole process tree it launched
// (npx wrapper, adapter, agent child) via the process group set with Setpgid.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_, _ = cmd.Process.Wait()
}
