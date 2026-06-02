package app

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// staleMilestonePhrases are wordings that described earlier pre-execution
// milestones. Execution, prompt build, gate, review, and memory flows are now
// implemented, so these phrases must not reappear in generated artifacts or help
// text. See M5.0 dogfood hardening.
var staleMilestonePhrases = []string{
	"not implemented yet",
	"does not execute agents in this milestone",
	"when execution becomes available",
	"not executable yet",
}

// removedFlagConcepts are flags and config keys that no longer exist. Generated
// output and help must never suggest them.
var removedFlagConcepts = []string{
	"--allow-execute",
	"--mode yolo",
	"agents.adapters",
}

// TestGeneratedPromptArtifactsHaveNoStaleMilestoneWording builds a full run
// through prompt build plus a prepared review and asserts the deterministic
// context artifacts a future agent reads carry no stale milestone wording.
func TestGeneratedPromptArtifactsHaveNoStaleMilestoneWording(t *testing.T) {
	promptRoot := t.TempDir()
	_, promptPaths, promptRunID := setupApprovedAndBuiltPrompt(t, promptRoot)
	promptRunPaths := contractRunPaths(filepath.Join(promptPaths.RunsDir, promptRunID))

	reviewRoot := t.TempDir()
	reviewApp, _, reviewRunID, reviewRunPaths := setupApprovedPreparedReview(t, reviewRoot, "passed")
	var reviewOut, reviewErr bytes.Buffer
	if code := reviewApp.Run([]string{"review", "dry-run", reviewRunID}, &reviewOut, &reviewErr); code != 0 {
		t.Fatalf("review dry-run exited %d, stderr: %s", code, reviewErr.String())
	}

	artifactsToCheck := map[string]string{
		"prompt.md":           promptRunPaths.PromptMD,
		"executor-context.md": promptRunPaths.ExecutorContext,
		"repo-context.md":     promptRunPaths.RepoContext,
		"contract.md":         promptRunPaths.ContractMD,
		"reviewer-context.md": reviewRunPaths.ReviewContextMD,
	}
	for name, path := range artifactsToCheck {
		content := mustReadFile(t, path)
		for _, phrase := range staleMilestonePhrases {
			if strings.Contains(content, phrase) {
				t.Errorf("%s contains stale milestone wording %q:\n%s", name, phrase, content)
			}
		}
	}
}

// TestDraftArtifactsHaveNoStaleMilestoneWording covers the pre-approval draft
// artifacts (contract.md and the prompt.md placeholder) emitted at run creation
// time, before prompt build overwrites them.
func TestDraftArtifactsHaveNoStaleMilestoneWording(t *testing.T) {
	root := t.TempDir()
	_, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	for name, path := range map[string]string{
		"contract.md": runPaths.ContractMD,
		"prompt.md":   runPaths.PromptMD,
	} {
		content := mustReadFile(t, path)
		for _, phrase := range staleMilestonePhrases {
			if strings.Contains(content, phrase) {
				t.Errorf("draft %s contains stale milestone wording %q:\n%s", name, phrase, content)
			}
		}
	}
}

// TestGeneratedArtifactsDoNotSuggestRemovedFlags ensures generated artifacts and
// the default config never point users at removed flags or config keys.
func TestGeneratedArtifactsDoNotSuggestRemovedFlags(t *testing.T) {
	root := t.TempDir()
	_, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	artifactsToCheck := map[string]string{
		"prompt.md":           runPaths.PromptMD,
		"executor-context.md": runPaths.ExecutorContext,
		"repo-context.md":     runPaths.RepoContext,
		"contract.md":         runPaths.ContractMD,
		"config.yaml":         paths.Config,
	}
	for name, path := range artifactsToCheck {
		content := mustReadFile(t, path)
		for _, flag := range removedFlagConcepts {
			if strings.Contains(content, flag) {
				t.Errorf("%s suggests removed flag/concept %q:\n%s", name, flag, content)
			}
		}
	}
}

// TestReadOnlyCommandsDoNotAppendLedgerEvents asserts that representative
// read-only commands neither error nor append to the ledger.
func TestReadOnlyCommandsDoNotAppendLedgerEvents(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)

	before := mustReadFile(t, paths.EventsJSONL)

	readOnlyCommands := [][]string{
		{"status"},
		{"search", "cache", "--kind", "code_item"},
		{"memory", "search", "cache"},
		{"memory", "stale"},
		{"memory", "show", runID},
		{"prompt", "show", runID},
		{"execute", "status", runID},
		{"execute", "show", runID},
		{"gate", "show", runID},
		{"review", "status", runID},
		{"review", "show", runID},
		{"contract", "show", runID},
		{"clarify", "status", runID},
	}
	for _, args := range readOnlyCommands {
		var stdout, stderr bytes.Buffer
		code := app.Run(args, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("read-only command %v exited %d, stderr: %s", args, code, stderr.String())
		}
	}

	after := mustReadFile(t, paths.EventsJSONL)
	if before != after {
		t.Fatalf("read-only commands mutated the ledger.\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestDogfoodCommandHelpHasNoStaleConcepts runs --help for the execute, review,
// and memory commands exercised during dogfood and asserts the help text renders
// and never mentions stale milestone wording or removed flags.
func TestDogfoodCommandHelpHasNoStaleConcepts(t *testing.T) {
	app := testApp(t.TempDir())

	helpConcepts := append([]string{}, staleMilestonePhrases...)
	helpConcepts = append(helpConcepts, removedFlagConcepts...)
	helpConcepts = append(helpConcepts, "not implemented", "milestone", "yolo", "adapters")

	commands := [][]string{
		{"execute", "run"},
		{"execute", "dry-run"},
		{"review", "run"},
		{"review", "dry-run"},
		{"memory", "refresh"},
		{"memory", "stale"},
		{"memory", "search"},
	}
	for _, base := range commands {
		args := append(append([]string{}, base...), "--help")
		var stdout, stderr bytes.Buffer
		code := app.Run(args, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("help for %v exited %d, stderr: %s", base, code, stderr.String())
		}
		help := stdout.String() + stderr.String()
		if !strings.Contains(help, "Usage:") {
			t.Fatalf("help for %v did not render usage:\n%s", base, help)
		}
		for _, concept := range helpConcepts {
			if strings.Contains(help, concept) {
				t.Errorf("help for %v mentions stale/removed concept %q:\n%s", base, concept, help)
			}
		}
	}
}
