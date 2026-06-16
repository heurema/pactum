package agents

import (
	"os/exec"
	"strings"
)

const (
	// DoctorStatusOnPath means the agent command was found on PATH via
	// exec.LookPath. It does NOT verify authentication or edit-capability.
	DoctorStatusOnPath         = "on_path"
	DoctorStatusMissingCommand = "missing_command"

	DoctorReportSchema = "pactum.agents_doctor.v1alpha1"
)

type DoctorReport struct {
	Schema          string        `json:"schema"`
	DefaultExecutor string        `json:"default_executor"`
	DefaultReviewer string        `json:"default_reviewer"`
	Agents          []AgentDoctor `json:"agents"`
}

type AgentDoctor struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Input   string   `json:"input"`
	Path    string   `json:"path"`
	Status  string   `json:"status"`
	Issues  []string `json:"issues"`
}

func DiagnoseAgents(registry Registry, selectedAgent string) (DoctorReport, error) {
	if registry == nil {
		registry = BuiltinRegistry{}
	}
	selectedAgent = strings.TrimSpace(selectedAgent)

	descriptors := registry.ListBuiltins()
	if selectedAgent != "" {
		descriptor, err := registry.ResolveExecutor(selectedAgent)
		if err != nil {
			return DoctorReport{}, err
		}
		descriptors = []AgentDescriptor{descriptor}
	}

	report := DoctorReport{
		Schema:          DoctorReportSchema,
		DefaultExecutor: registry.DefaultExecutor(),
		DefaultReviewer: registry.DefaultReviewer(),
		Agents:          make([]AgentDoctor, 0, len(descriptors)),
	}
	for _, descriptor := range descriptors {
		report.Agents = append(report.Agents, diagnoseAgent(descriptor))
	}
	return report, nil
}

func diagnoseAgent(agent AgentDescriptor) AgentDoctor {
	// For ACP-only built-ins (no CLI command), resolve the adapter launcher
	// (typically npx) and check its availability on PATH instead.
	cmd := agent.Command
	if cmd == "" {
		cmd, _, _, _ = acpAdapterCommand(agent.Name, ModelSpec{}, false)
	}

	doctor := AgentDoctor{
		Name:    agent.Name,
		Command: cmd,
		Input:   agent.Input,
		Status:  DoctorStatusOnPath,
		Issues:  []string{},
	}

	path, err := exec.LookPath(cmd)
	if err != nil {
		doctor.Status = DoctorStatusMissingCommand
		doctor.Issues = append(doctor.Issues, "command not found on PATH")
	} else {
		doctor.Path = path
	}
	return doctor
}
