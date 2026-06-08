//go:build !unix

package agents

import "os/exec"

// setProcessGroup is a no-op off Unix: putting the adapter in its own process
// group relies on the Unix-only syscall.SysProcAttr.Setpgid, so non-Unix builds
// fall back to killing the adapter process directly (see killProcessGroup).
func setProcessGroup(cmd *exec.Cmd) {}

// killProcessGroup terminates the adapter process. Without a Unix process group
// it cannot reap the whole tree the adapter launched (the npx wrapper and the
// agent child), but it still kills the process the transport started.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
}
