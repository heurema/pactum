package gitctx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Build constructs a *exec.Cmd for the given read-only git command without
// executing it. root is passed as the -C argument to git; args must start with
// an allowed read-only subcommand. Every built command has GIT_OPTIONAL_LOCKS=0
// added to its environment so git skips advisory lock files.
// It is the shared command-building seam used by Run and available for test inspection.
func Build(ctx context.Context, root string, args ...string) (*exec.Cmd, error) {
	if err := validate(args); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	return cmd, nil
}

// Run validates args, builds the command, and runs it with ctx.
// Returns the command's stdout bytes.
func Run(ctx context.Context, root string, args ...string) ([]byte, error) {
	cmd, err := Build(ctx, root, args...)
	if err != nil {
		return nil, err
	}
	return cmd.Output()
}

// denyFlags are globally rejected regardless of subcommand.
var denyFlags = map[string]bool{
	"--no-index": true, "--contents": true, "--ignore-revs-file": true,
	"--output": true, "-o": true, "-c": true,
	"--git-dir": true, "--work-tree": true, "--exec-path": true,
}

func validate(args []string) error {
	if len(args) == 0 {
		return errors.New("gitctx: empty argument list")
	}
	for _, a := range args {
		if strings.ContainsRune(a, '\x00') {
			return errors.New("gitctx: argument contains NUL byte")
		}
	}
	subcmd := args[0]
	rest := args[1:]
	switch subcmd {
	case "branch", "tag":
		return fmt.Errorf("gitctx: subcommand %q is not allowed", subcmd)
	case "ls-files", "rev-parse", "show-ref", "for-each-ref", "status", "diff":
		// validated below
	default:
		return fmt.Errorf("gitctx: subcommand %q is not allowed", subcmd)
	}
	for _, a := range rest {
		if denyFlags[a] {
			return fmt.Errorf("gitctx: flag %q is not allowed", a)
		}
	}
	switch subcmd {
	case "ls-files":
		return validateLsFiles(rest)
	case "rev-parse":
		return validateRevParse(rest)
	case "show-ref":
		return validateShowRef(rest)
	case "for-each-ref":
		return validateForEachRef(rest)
	case "status":
		return validateStatus(rest)
	case "diff":
		return validateDiff(rest)
	}
	panic("unreachable")
}

var lsFilesFlags = map[string]bool{
	"-z": true, "--cached": true, "--others": true, "--exclude-standard": true,
}

func validateLsFiles(args []string) error {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			if !lsFilesFlags[a] {
				return fmt.Errorf("gitctx: ls-files: flag %q not allowed", a)
			}
		} else {
			return fmt.Errorf("gitctx: ls-files: non-flag argument %q not allowed", a)
		}
	}
	return nil
}

func validateRevParse(args []string) error {
	if len(args) != 2 || args[0] != "--verify" || args[1] != "HEAD" {
		return errors.New("gitctx: rev-parse: only [--verify HEAD] is allowed")
	}
	return nil
}

func validateShowRef(args []string) error {
	count := 0
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			return fmt.Errorf("gitctx: show-ref: flags are not allowed, got %q", a)
		}
		if err := checkPathArg(a); err != nil {
			return err
		}
		count++
	}
	if count > 1 {
		return errors.New("gitctx: show-ref: at most one ref-name argument allowed")
	}
	return nil
}

func validateForEachRef(args []string) error {
	count := 0
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			return fmt.Errorf("gitctx: for-each-ref: flags are not allowed, got %q", a)
		}
		if err := checkPathArg(a); err != nil {
			return err
		}
		count++
	}
	if count > 1 {
		return errors.New("gitctx: for-each-ref: at most one ref-pattern argument allowed")
	}
	return nil
}

func validateStatus(args []string) error {
	if len(args) != 1 || args[0] != "--porcelain" {
		return errors.New("gitctx: status: only [--porcelain] is allowed")
	}
	return nil
}

var diffFlags = map[string]bool{
	"--name-only": true, "--name-status": true,
}

func validateDiff(args []string) error {
	flagCount, nonFlagCount := 0, 0
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			if !diffFlags[a] {
				return fmt.Errorf("gitctx: diff: flag %q not allowed", a)
			}
			flagCount++
		} else {
			if err := checkCommitish(a); err != nil {
				return err
			}
			nonFlagCount++
		}
	}
	if flagCount != 1 {
		return errors.New("gitctx: diff: exactly one of --name-only or --name-status required")
	}
	if nonFlagCount > 2 {
		return errors.New("gitctx: diff: at most two commit-ish arguments allowed")
	}
	return nil
}

func checkPathArg(arg string) error {
	if filepath.IsAbs(filepath.FromSlash(arg)) {
		return fmt.Errorf("gitctx: absolute path argument %q not allowed", arg)
	}
	if arg == ".." || strings.HasPrefix(arg, "../") || strings.Contains(arg, "/..") {
		return fmt.Errorf("gitctx: path traversal in %q not allowed", arg)
	}
	if strings.HasPrefix(arg, ":") {
		return fmt.Errorf("gitctx: pathspec magic in %q not allowed", arg)
	}
	return nil
}

// checkCommitish validates a non-flag argument for git diff.
// The contract limits these to commit-ish operands (e.g. HEAD, HEAD~1, abc123).
// It rejects absolute paths, .. traversal, : pathspec magic, and slash-separated
// paths — the latter are pathspec operands, not commit-ish references.
func checkCommitish(arg string) error {
	if filepath.IsAbs(filepath.FromSlash(arg)) {
		return fmt.Errorf("gitctx: absolute path argument %q not allowed", arg)
	}
	if strings.HasPrefix(arg, ":") {
		return fmt.Errorf("gitctx: pathspec magic in %q not allowed", arg)
	}
	if strings.Contains(arg, "/") {
		return fmt.Errorf("gitctx: path-like argument %q not allowed as diff commit-ish", arg)
	}
	if arg == ".." {
		return fmt.Errorf("gitctx: path traversal in %q not allowed", arg)
	}
	return nil
}
