//go:build windows

package agents

import (
	"context"
	"os/exec"
)

type osProcessRunner struct{}

func (osProcessRunner) Run(ctx context.Context, spec processSpec) error {
	command := exec.CommandContext(ctx, spec.Command, spec.Args...)
	command.Dir = spec.Dir
	command.Env = spec.Env
	command.Stdin = spec.Stdin
	command.Stdout = spec.Stdout
	command.Stderr = spec.Stderr
	return command.Run()
}
