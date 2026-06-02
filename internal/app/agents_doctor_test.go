package app

import (
	"bytes"
	"encoding/json"
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
		"Built-in agents",
		"Default executor: codex",
		"Default reviewer: codex",
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
	if report.DefaultExecutor != "codex" || report.DefaultReviewer != "codex" || len(report.Agents) != 2 {
		t.Fatalf("unexpected doctor json: %#v", report)
	}
	if !doctorReportHasAgent(report, "codex") || !doctorReportHasAgent(report, "claude") {
		t.Fatalf("doctor json missing built-in agents: %#v", report)
	}
}

func TestAgentsDoctorSelectedBuiltIn(t *testing.T) {
	root := t.TempDir()
	_, _ = setupInitializedWorkspace(t, root)

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"agents", "doctor", "--agent", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("agents doctor codex exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"codex:",
		"command: codex",
		"input: prompt_file",
		"status:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("agents doctor codex output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "claude:") {
		t.Fatalf("selected doctor should not include claude:\n%s", got)
	}
}

func TestAgentsDoctorMissingAgentFails(t *testing.T) {
	root := t.TempDir()
	_, _ = setupInitializedWorkspace(t, root)

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"agents", "doctor", "--agent", "missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("agents doctor missing adapter should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "unsupported agent: missing") {
		t.Fatalf("missing adapter stderr mismatch:\n%s", got)
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

func doctorReportHasAgent(report agents.DoctorReport, name string) bool {
	for _, agent := range report.Agents {
		if agent.Name == name {
			return true
		}
	}
	return false
}
