package gitctx_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/gitctx"
)

func TestBuildCmdSeam(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}

	root := t.TempDir()
	cmd, err := gitctx.Build(context.Background(), root, "rev-parse", "--verify", "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	if cmd.Path != gitPath {
		t.Errorf("Cmd.Path = %q, want %q", cmd.Path, gitPath)
	}
	if strings.Contains(cmd.Path, "/sh") || strings.Contains(cmd.Path, "/bash") {
		t.Errorf("Cmd.Path %q looks like a shell, not the git binary", cmd.Path)
	}

	var foundLock bool
	for _, e := range cmd.Env {
		if e == "GIT_OPTIONAL_LOCKS=0" {
			foundLock = true
			break
		}
	}
	if !foundLock {
		t.Errorf("GIT_OPTIONAL_LOCKS=0 not found in Cmd.Env: %v", cmd.Env)
	}

	var sawDashC bool
	for i, a := range cmd.Args {
		if a == "-C" && i+1 < len(cmd.Args) {
			sawDashC = true
			if cmd.Args[i+1] != root {
				t.Errorf("arg after -C = %q, want %q", cmd.Args[i+1], root)
			}
			break
		}
	}
	if !sawDashC {
		t.Errorf("expected -C in Cmd.Args: %v", cmd.Args)
	}
}

func TestValidateAllowed(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"ls-files no flags", []string{"ls-files"}},
		{"ls-files -z", []string{"ls-files", "-z"}},
		{"ls-files --cached", []string{"ls-files", "--cached"}},
		{"ls-files --others", []string{"ls-files", "--others"}},
		{"ls-files --exclude-standard", []string{"ls-files", "--exclude-standard"}},
		{"ls-files all flags", []string{"ls-files", "-z", "--cached", "--others", "--exclude-standard"}},
		{"rev-parse --verify HEAD", []string{"rev-parse", "--verify", "HEAD"}},
		{"show-ref no args", []string{"show-ref"}},
		{"show-ref one ref", []string{"show-ref", "refs/heads/main"}},
		{"for-each-ref no args", []string{"for-each-ref"}},
		{"for-each-ref one pattern", []string{"for-each-ref", "refs/heads/*"}},
		{"status --porcelain", []string{"status", "--porcelain"}},
		{"diff --name-only no commits", []string{"diff", "--name-only"}},
		{"diff --name-status no commits", []string{"diff", "--name-status"}},
		{"diff --name-only one commit", []string{"diff", "--name-only", "HEAD"}},
		{"diff --name-only two commits", []string{"diff", "--name-only", "HEAD~1", "HEAD"}},
		{"diff --name-status two commits", []string{"diff", "--name-status", "abc123", "def456"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			_, err := gitctx.Build(context.Background(), root, tc.args...)
			if err != nil {
				t.Errorf("Build(%v) returned unexpected error: %v", tc.args, err)
			}
		})
	}
}

func TestValidateRejected(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		// Empty argument list
		{"empty args", []string{}},
		// Blocked subcommands
		{"branch", []string{"branch"}},
		{"branch -a", []string{"branch", "-a"}},
		{"tag", []string{"tag"}},
		{"tag -l", []string{"tag", "-l"}},
		// Write and unknown subcommands
		{"commit", []string{"commit", "-m", "msg"}},
		{"push", []string{"push", "origin", "main"}},
		{"checkout", []string{"checkout", "main"}},
		{"reset", []string{"reset", "--hard"}},
		{"clean", []string{"clean", "-f"}},
		{"stash", []string{"stash"}},
		{"unknown subcommand", []string{"unknown-subcmd"}},
		// NUL bytes
		{"NUL in subcommand", []string{"ls-files\x00"}},
		{"NUL in arg", []string{"ls-files", "-z\x00"}},
		// Globally blocked flags
		{"--no-index", []string{"ls-files", "--no-index"}},
		{"--contents", []string{"ls-files", "--contents"}},
		{"--ignore-revs-file", []string{"diff", "--name-only", "--ignore-revs-file"}},
		{"--output", []string{"ls-files", "--output"}},
		{"-o", []string{"ls-files", "-o"}},
		{"-c", []string{"ls-files", "-c"}},
		{"--git-dir", []string{"ls-files", "--git-dir"}},
		{"--work-tree", []string{"ls-files", "--work-tree"}},
		{"--exec-path", []string{"ls-files", "--exec-path"}},
		// Absolute path arguments
		{"absolute path show-ref", []string{"show-ref", "/absolute/ref"}},
		{"absolute path for-each-ref", []string{"for-each-ref", "/refs/heads/main"}},
		{"absolute path diff commit", []string{"diff", "--name-only", "/absolute/commit"}},
		// .. traversal
		{"dotdot alone show-ref", []string{"show-ref", ".."}},
		{"dotdot prefix for-each-ref", []string{"for-each-ref", "../heads"}},
		{"dotdot in path show-ref", []string{"show-ref", "refs/../heads/main"}},
		// Pathspec magic
		{"pathspec magic show-ref", []string{"show-ref", ":HEAD"}},
		{"pathspec magic for-each-ref", []string{"for-each-ref", ":refs/heads/*"}},
		// ls-files specific
		{"ls-files unknown flag", []string{"ls-files", "--deleted"}},
		{"ls-files non-flag arg", []string{"ls-files", "somefile"}},
		// rev-parse specific
		{"rev-parse no args", []string{"rev-parse"}},
		{"rev-parse --verify only", []string{"rev-parse", "--verify"}},
		{"rev-parse wrong second arg", []string{"rev-parse", "--verify", "main"}},
		{"rev-parse extra args", []string{"rev-parse", "--verify", "HEAD", "extra"}},
		// show-ref specific
		{"show-ref with flag", []string{"show-ref", "--heads"}},
		{"show-ref two args", []string{"show-ref", "refs/heads/main", "refs/tags/v1"}},
		// for-each-ref specific
		{"for-each-ref with flag", []string{"for-each-ref", "--format=%(refname)"}},
		{"for-each-ref two args", []string{"for-each-ref", "refs/heads/*", "refs/tags/*"}},
		// status specific
		{"status no args", []string{"status"}},
		{"status wrong flag", []string{"status", "--short"}},
		{"status two flags", []string{"status", "--porcelain", "--short"}},
		// diff specific
		{"diff no flags", []string{"diff"}},
		{"diff both flags", []string{"diff", "--name-only", "--name-status"}},
		{"diff unknown flag", []string{"diff", "--stat"}},
		{"diff three commits", []string{"diff", "--name-only", "a", "b", "c"}},
		{"diff path operand", []string{"diff", "--name-only", "src/main.go"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			_, err := gitctx.Build(context.Background(), root, tc.args...)
			if err == nil {
				t.Errorf("Build(%v) expected error, got nil", tc.args)
			}
		})
	}
}
