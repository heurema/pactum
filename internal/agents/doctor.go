package agents

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

const (
	DoctorStatusReady            = "ready"
	DoctorStatusMissingCommand   = "missing_command"
	DoctorStatusUnsupportedInput = "unsupported_input"
	DoctorStatusInvalidConfig    = "invalid_config"
)

type DoctorReport struct {
	DefaultExecutor string          `json:"default_executor"`
	Adapters        []AdapterDoctor `json:"adapters"`
}

type AdapterDoctor struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Input   string   `json:"input"`
	Path    string   `json:"path"`
	Status  string   `json:"status"`
	Issues  []string `json:"issues"`
}

func DiagnoseAdapters(config AgentConfig, selectedAgent string) (DoctorReport, error) {
	config = NormalizeConfig(config)
	selectedAgent = strings.TrimSpace(selectedAgent)

	names := make([]string, 0, len(config.Adapters))
	if selectedAgent != "" {
		if _, ok := config.Adapters[selectedAgent]; !ok {
			return DoctorReport{}, fmt.Errorf("agent adapter not configured: %s", selectedAgent)
		}
		names = append(names, selectedAgent)
	} else {
		for name := range config.Adapters {
			names = append(names, name)
		}
		sort.Strings(names)
	}

	report := DoctorReport{
		DefaultExecutor: config.DefaultExecutor,
		Adapters:        make([]AdapterDoctor, 0, len(names)),
	}
	for _, name := range names {
		report.Adapters = append(report.Adapters, diagnoseAdapter(name, config.Adapters[name]))
	}
	return report, nil
}

func diagnoseAdapter(name string, adapter AdapterConfig) AdapterDoctor {
	command := strings.TrimSpace(adapter.Command)
	input := strings.TrimSpace(adapter.Input)
	doctor := AdapterDoctor{
		Name:    name,
		Command: adapter.Command,
		Input:   adapter.Input,
		Status:  DoctorStatusReady,
		Issues:  []string{},
	}

	if command == "" {
		doctor.Status = DoctorStatusInvalidConfig
		doctor.Issues = append(doctor.Issues, "command is required")
	}
	if input == "" {
		if doctor.Status == DoctorStatusReady {
			doctor.Status = DoctorStatusInvalidConfig
		}
		doctor.Issues = append(doctor.Issues, "input mode is required")
	} else if input != InputPromptFile {
		if doctor.Status == DoctorStatusReady {
			doctor.Status = DoctorStatusUnsupportedInput
		}
		doctor.Issues = append(doctor.Issues, "unsupported input mode: "+input)
	}
	if command != "" {
		path, err := exec.LookPath(command)
		if err != nil {
			if doctor.Status == DoctorStatusReady {
				doctor.Status = DoctorStatusMissingCommand
			}
			doctor.Issues = append(doctor.Issues, "command not found on PATH")
		} else {
			doctor.Path = path
		}
	}

	return doctor
}
