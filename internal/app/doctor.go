package app

import (
	"fmt"
	"io"

	"github.com/heurema/pactum/internal/agents"
)

func (a App) Doctor(stdout io.Writer, agentName string, jsonOutput bool) error {
	_, _, ok, err := a.requireWorkspace(stdout, false)
	if err != nil || !ok {
		return err
	}

	report, err := agents.DiagnoseAgents(a.agentRegistry(), agentName)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSONResponse(stdout, report)
	}
	writeDoctor(stdout, report)
	return nil
}

func writeDoctor(stdout io.Writer, report agents.DoctorReport) {
	fmt.Fprintln(stdout, "Built-in agents")
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "Default executor: %s\n", report.DefaultExecutor)
	fmt.Fprintf(stdout, "Default reviewer: %s\n", report.DefaultReviewer)
	fmt.Fprintln(stdout)
	for i, agent := range report.Agents {
		fmt.Fprintf(stdout, "%s:\n", agent.Name)
		fmt.Fprintf(stdout, "  command: %s\n", agent.Command)
		fmt.Fprintf(stdout, "  input: %s\n", agent.Input)
		if agent.Path == "" {
			fmt.Fprintln(stdout, "  path: missing")
		} else {
			fmt.Fprintf(stdout, "  path: %s\n", agent.Path)
		}
		fmt.Fprintf(stdout, "  status: %s\n", agent.Status)
		if len(agent.Issues) > 0 {
			fmt.Fprintln(stdout, "  issues:")
			for _, issue := range agent.Issues {
				fmt.Fprintf(stdout, "    - %s\n", issue)
			}
		}
		if i < len(report.Agents)-1 {
			fmt.Fprintln(stdout)
		}
	}
	if len(report.Agents) > 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, `Note: "on_path" means the command was found on PATH — auth and edit-capability are not verified.`)
	}
}
