package app

import (
	"fmt"
	"io"

	"github.com/heurema/pactum/internal/agents"
)

func (a App) AgentsDoctor(stdout io.Writer, agentName string, jsonOutput bool) error {
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
	writeAgentsDoctor(stdout, report)
	return nil
}

func writeAgentsDoctor(stdout io.Writer, report agents.DoctorReport) {
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
}
