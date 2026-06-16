package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildACPWouldRun(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home dir: %v", err)
	}

	// An absolute path under the home directory is recorded as '~/...'.
	absOverride := filepath.Join(home, "repos", "codex-acp")
	t.Setenv("PACTUM_CODEX_ACP_COMMAND", absOverride)
	t.Setenv("PACTUM_CLAUDE_ACP_COMMAND", "")

	got, err := BuildACPWouldRun(BuiltinCodex, ModelSpec{}, false)
	if err != nil {
		t.Fatalf("codex home-path override: %v", err)
	}
	if strings.Contains(got.Command, home) {
		t.Fatalf("codex home-path must be sanitized, got command=%q", got.Command)
	}
	if got.Command != "~/repos/codex-acp" {
		t.Fatalf("codex home-path command = %q, want %q", got.Command, "~/repos/codex-acp")
	}

	// Same for the claude adapter.
	absClaudeOverride := filepath.Join(home, "repos", "claude-agent-acp")
	t.Setenv("PACTUM_CLAUDE_ACP_COMMAND", absClaudeOverride)

	got, err = BuildACPWouldRun(BuiltinClaude, ModelSpec{}, false)
	if err != nil {
		t.Fatalf("claude home-path override: %v", err)
	}
	if strings.Contains(got.Command, home) {
		t.Fatalf("claude home-path must be sanitized, got command=%q", got.Command)
	}
	if got.Command != "~/repos/claude-agent-acp" {
		t.Fatalf("claude home-path command = %q, want %q", got.Command, "~/repos/claude-agent-acp")
	}

	// A non-home value such as 'npx' is left unchanged.
	clearACPAdapterOverrides(t)

	got, err = BuildACPWouldRun(BuiltinCodex, ModelSpec{}, false)
	if err != nil {
		t.Fatalf("codex default: %v", err)
	}
	if got.Command != "npx" {
		t.Fatalf("non-home command must be unchanged, got %q", got.Command)
	}

	got, err = BuildACPWouldRun(BuiltinClaude, ModelSpec{}, false)
	if err != nil {
		t.Fatalf("claude default: %v", err)
	}
	if got.Command != "npx" {
		t.Fatalf("non-home command must be unchanged, got %q", got.Command)
	}

	// A relative override path is left unchanged.
	t.Setenv("PACTUM_CODEX_ACP_COMMAND", "vendor/bin/codex-acp")
	t.Setenv("PACTUM_CLAUDE_ACP_COMMAND", "")

	got, err = BuildACPWouldRun(BuiltinCodex, ModelSpec{}, false)
	if err != nil {
		t.Fatalf("codex relative override: %v", err)
	}
	if got.Command != "vendor/bin/codex-acp" {
		t.Fatalf("relative override must be unchanged, got %q", got.Command)
	}
}

func TestBuildDryRunPlan_HomePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home dir: %v", err)
	}
	absModel := filepath.Join(home, "models", "my-model")

	t.Run("claude", func(t *testing.T) {
		t.Setenv("PACTUM_CLAUDE_ACP_COMMAND", filepath.Join(home, "repos", "claude-agent-acp"))
		t.Setenv("PACTUM_CODEX_ACP_COMMAND", "")

		plan, err := BuildDryRunPlan("run_test", "2026-01-01T00:00:00Z",
			AgentDescriptor{Name: BuiltinClaude}, ModelSpec{Model: absModel}, false, "prompt.md")
		if err != nil {
			t.Fatalf("BuildDryRunPlan: %v", err)
		}
		if strings.Contains(plan.WouldRun.Command, home) {
			t.Errorf("WouldRun.Command must not contain home path, got %q", plan.WouldRun.Command)
		}
		for _, e := range plan.WouldRun.Env {
			if strings.Contains(e, home) {
				t.Errorf("WouldRun.Env entry must not contain home path, got %q", e)
			}
		}
	})

	t.Run("codex", func(t *testing.T) {
		t.Setenv("PACTUM_CODEX_ACP_COMMAND", filepath.Join(home, "repos", "codex-acp"))
		t.Setenv("PACTUM_CLAUDE_ACP_COMMAND", "")

		plan, err := BuildDryRunPlan("run_test", "2026-01-01T00:00:00Z",
			AgentDescriptor{Name: BuiltinCodex}, ModelSpec{Model: absModel}, false, "prompt.md")
		if err != nil {
			t.Fatalf("BuildDryRunPlan: %v", err)
		}
		if strings.Contains(plan.WouldRun.Command, home) {
			t.Errorf("WouldRun.Command must not contain home path, got %q", plan.WouldRun.Command)
		}
		for _, a := range plan.WouldRun.Args {
			if strings.Contains(a, home) {
				t.Errorf("WouldRun.Args entry must not contain home path, got %q", a)
			}
		}
	})
}

func TestSanitizeHomePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home dir: %v", err)
	}

	cases := []struct {
		in   string
		want string
	}{
		{home + "/repos/codex-acp", "~/repos/codex-acp"},
		{home, "~"},
		{"npx", "npx"},
		{"codex-acp", "codex-acp"},
		{"/usr/local/bin/codex", "/usr/local/bin/codex"},
		{"/tmp/not-home", "/tmp/not-home"},
		// A path that merely contains the home prefix as a substring (not a
		// directory prefix) must not be mangled.
		{"/other" + home + "/repos", "/other" + home + "/repos"},
		// KEY=VALUE env entries where value is a home path.
		{"ANTHROPIC_MODEL=" + home + "/models/m", "ANTHROPIC_MODEL=~/models/m"},
		{"ANTHROPIC_MODEL=" + home, "ANTHROPIC_MODEL=~"},
		// Non-home value in KEY=VALUE left unchanged.
		{"ANTHROPIC_MODEL=claude-3-opus", "ANTHROPIC_MODEL=claude-3-opus"},
		// KEY="VALUE" quoted form (codex -c flag).
		{`model="` + home + `/models/m"`, `model="~/models/m"`},
		{`model="npx"`, `model="npx"`},
	}
	for _, tc := range cases {
		got := sanitizeHomePath(tc.in)
		if got != tc.want {
			t.Errorf("sanitizeHomePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
