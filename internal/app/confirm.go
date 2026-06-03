package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
)

// confirmDirectExecution asks the user to confirm direct (unsandboxed) agent
// execution. In non-interactive use it refuses (the caller bypasses this with
// --yes). The prompt is written to stdout and the answer read from os.Stdin.
func confirmDirectExecution(stdout io.Writer) (bool, error) {
	if !stdinIsInteractive() {
		return false, errors.New("refusing to run agent non-interactively without --yes")
	}
	fmt.Fprintln(stdout, "Pactum will run the selected built-in agent directly in this repository.")
	fmt.Fprintln(stdout, "Pactum does not sandbox execution.")
	fmt.Fprint(stdout, "Continue? [y/N] ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

func stdinIsInteractive() bool {
	fd := os.Stdin.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
