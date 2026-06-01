package app

import (
	"fmt"
	"io"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
)

func (a App) AgentsDoctor(stdout io.Writer, agentName string, jsonOutput bool) error {
	root, workspace, err := a.resolveStatusRoot()
	if err != nil {
		return err
	}
	if workspace == "" {
		fmt.Fprintln(stdout, "Pactum is not initialized. Run: pactum init")
		return nil
	}

	config, err := readConfig(artifacts.New(root).Config)
	if err != nil {
		return err
	}
	report, err := agents.DiagnoseAdapters(config.Agents, agentName)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSONResponse(stdout, report)
	}
	writeAgentsDoctor(stdout, report)
	return nil
}

func writeAgentsDoctor(stdout io.Writer, report agents.DoctorReport) {
	fmt.Fprintln(stdout, "Agent adapters")
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "Default executor: %s\n", report.DefaultExecutor)
	fmt.Fprintln(stdout)
	for i, adapter := range report.Adapters {
		fmt.Fprintf(stdout, "%s:\n", adapter.Name)
		fmt.Fprintf(stdout, "  command: %s\n", adapter.Command)
		fmt.Fprintf(stdout, "  input: %s\n", adapter.Input)
		if adapter.Path == "" {
			fmt.Fprintln(stdout, "  path: missing")
		} else {
			fmt.Fprintf(stdout, "  path: %s\n", adapter.Path)
		}
		fmt.Fprintf(stdout, "  status: %s\n", adapter.Status)
		if len(adapter.Issues) > 0 {
			fmt.Fprintln(stdout, "  issues:")
			for _, issue := range adapter.Issues {
				fmt.Fprintf(stdout, "    - %s\n", issue)
			}
		}
		if i < len(report.Adapters)-1 {
			fmt.Fprintln(stdout)
		}
	}
}
