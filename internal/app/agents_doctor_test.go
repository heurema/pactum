package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
)

func TestAgentsDoctorBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"agents", "doctor"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("agents doctor before init exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Pactum is not initialized. Run: pactum init") {
		t.Fatalf("agents doctor before init output mismatch:\n%s", got)
	}
}

func TestAgentsDoctorDefaultConfig(t *testing.T) {
	root := t.TempDir()
	_, _ = setupInitializedWorkspace(t, root)

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"agents", "doctor"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("agents doctor exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Agent adapters",
		"Default executor: codex",
		"claude:",
		"codex:",
		"status:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("agents doctor output missing %q:\n%s", want, got)
		}
	}
}

func TestAgentsDoctorJSON(t *testing.T) {
	root := t.TempDir()
	_, _ = setupInitializedWorkspace(t, root)

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"agents", "doctor", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("agents doctor --json exited %d, stderr: %s", code, stderr.String())
	}
	var report agents.DoctorReport
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &report))
	if report.DefaultExecutor != "codex" || len(report.Adapters) < 2 {
		t.Fatalf("unexpected doctor json: %#v", report)
	}
	if !doctorReportHasAdapter(report, "codex") || !doctorReportHasAdapter(report, "claude") {
		t.Fatalf("doctor json missing default adapters: %#v", report)
	}
}

func TestAgentsDoctorCustomHelperAdapter(t *testing.T) {
	root := t.TempDir()
	_, paths := setupInitializedWorkspace(t, root)
	configureHelperAgent(t, paths, "helper")

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"agents", "doctor", "--agent", "helper"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("agents doctor helper exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"helper:",
		"command: " + os.Args[0],
		"input: prompt_file",
		"status: ready",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("agents doctor helper output missing %q:\n%s", want, got)
		}
	}
}

func TestAgentsDoctorMissingAdapterFails(t *testing.T) {
	root := t.TempDir()
	_, _ = setupInitializedWorkspace(t, root)

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"agents", "doctor", "--agent", "missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("agents doctor missing adapter should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "agent adapter not configured: missing") {
		t.Fatalf("missing adapter stderr mismatch:\n%s", got)
	}
}

func TestAgentsDoctorUnsupportedInput(t *testing.T) {
	root := t.TempDir()
	_, paths := setupInitializedWorkspace(t, root)
	config := defaultConfigFile()
	config.Agents.Adapters["bad-input"] = agents.AdapterConfig{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestExecutionHelperProcess"},
		Input:   "stdin",
	}
	assertNoError(t, writeYAML(paths.Config, config))

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"agents", "doctor", "--agent", "bad-input"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("agents doctor unsupported input exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"bad-input:",
		"status: unsupported_input",
		"unsupported input mode: stdin",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("unsupported input output missing %q:\n%s", want, got)
		}
	}
}

func TestAgentsDoctorReadOnlyLedger(t *testing.T) {
	root := t.TempDir()
	app, paths := setupInitializedWorkspace(t, root)
	before := mustReadFile(t, paths.EventsJSONL)

	for _, args := range [][]string{
		{"agents", "doctor"},
		{"agents", "doctor", "--json"},
	} {
		var stdout, stderr bytes.Buffer
		code := app.Run(args, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%v exited %d, stderr: %s", args, code, stderr.String())
		}
	}

	if after := mustReadFile(t, paths.EventsJSONL); after != before {
		t.Fatalf("agents doctor changed events.jsonl\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func setupInitializedWorkspace(t *testing.T, root string) (App, artifacts.Paths) {
	t.Helper()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")
	app := testApp(root)
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}
	return app, artifacts.New(root)
}

func doctorReportHasAdapter(report agents.DoctorReport, name string) bool {
	for _, adapter := range report.Adapters {
		if adapter.Name == name {
			return true
		}
	}
	return false
}
