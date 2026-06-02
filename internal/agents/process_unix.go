//go:build !windows

package agents

import (
	"context"
	"os/exec"
	"syscall"
	"time"
)

type osProcessRunner struct{}

func (osProcessRunner) Run(ctx context.Context, spec processSpec) error {
	command := exec.Command(spec.Command, spec.Args...)
	command.Dir = spec.Dir
	command.Env = spec.Env
	command.Stdin = spec.Stdin
	command.Stdout = spec.Stdout
	command.Stderr = spec.Stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := command.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		terminateProcessGroup(command.Process.Pid, syscall.SIGTERM)
		select {
		case err := <-done:
			if err != nil {
				return err
			}
			return ctx.Err()
		case <-time.After(2 * time.Second):
			terminateProcessGroup(command.Process.Pid, syscall.SIGKILL)
			err := <-done
			if err != nil {
				return err
			}
			return ctx.Err()
		}
	}
}

func terminateProcessGroup(pid int, signal syscall.Signal) {
	if pid <= 0 {
		return
	}
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		_ = syscall.Kill(pid, signal)
		return
	}
	_ = syscall.Kill(-pgid, signal)
}
