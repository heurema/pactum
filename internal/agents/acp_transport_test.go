package agents

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestACPAdapterCommand(t *testing.T) {
	// An unpinned spec adds no override args and no env entries for either agent.
	for _, agent := range []string{BuiltinClaude, BuiltinCodex} {
		cmd, args, env, err := acpAdapterCommand(agent, ModelSpec{}, false)
		if err != nil {
			t.Fatalf("%s adapter: %v", agent, err)
		}
		if cmd != "npx" || len(args) != 2 {
			t.Fatalf("%s adapter: cmd=%q args=%v", agent, cmd, args)
		}
		if len(env) != 0 {
			t.Fatalf("%s adapter: unpinned spec must add no env, got %v", agent, env)
		}
	}
	if _, _, _, err := acpAdapterCommand("nope", ModelSpec{}, false); err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestACPAdapterCommandThreadsModelPin(t *testing.T) {
	// codex: the pin becomes the same `-c` config overrides ApplyModelSpec passes
	// to the codex CLI, with the model TOML-quoted.
	_, args, env, err := acpAdapterCommand(BuiltinCodex, ModelSpec{Model: "gpt-5", Effort: "high"}, false)
	if err != nil {
		t.Fatalf("codex adapter: %v", err)
	}
	want := []string{"-y", "@zed-industries/codex-acp@latest", "-c", `model="gpt-5"`, "-c", "model_reasoning_effort=high"}
	if len(args) != len(want) {
		t.Fatalf("codex args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("codex args = %v, want %v", args, want)
		}
	}
	if len(env) != 0 {
		t.Fatalf("codex pin must not add env, got %v", env)
	}

	// codex: effort-only pin adds only the effort override.
	_, args, _, err = acpAdapterCommand(BuiltinCodex, ModelSpec{Effort: "high"}, false)
	if err != nil {
		t.Fatalf("codex effort-only: %v", err)
	}
	if len(args) != 4 || args[2] != "-c" || args[3] != "model_reasoning_effort=high" {
		t.Fatalf("codex effort-only args = %v", args)
	}

	// claude: the pin becomes env vars for the Claude Code session the adapter
	// launches; the adapter args stay untouched.
	_, args, env, err = acpAdapterCommand(BuiltinClaude, ModelSpec{Model: "claude-sonnet-4", Effort: "high"}, false)
	if err != nil {
		t.Fatalf("claude adapter: %v", err)
	}
	if len(args) != 2 {
		t.Fatalf("claude pin must not change args, got %v", args)
	}
	wantEnv := []string{"ANTHROPIC_MODEL=claude-sonnet-4", "CLAUDE_CODE_EFFORT_LEVEL=high"}
	if len(env) != len(wantEnv) || env[0] != wantEnv[0] || env[1] != wantEnv[1] {
		t.Fatalf("claude env = %v, want %v", env, wantEnv)
	}

	// claude: model-only pin sets only ANTHROPIC_MODEL.
	_, _, env, err = acpAdapterCommand(BuiltinClaude, ModelSpec{Model: "claude-sonnet-4"}, false)
	if err != nil {
		t.Fatalf("claude model-only: %v", err)
	}
	if len(env) != 1 || env[0] != "ANTHROPIC_MODEL=claude-sonnet-4" {
		t.Fatalf("claude model-only env = %v", env)
	}
}

func TestACPAdapterCommandReadOnly(t *testing.T) {
	// codex: read-only pins the sandbox at the adapter — codex applies patches
	// natively and consults its own approval policy, so client-side denials
	// cannot stop it. Mirrors the CLI reviewer's --sandbox read-only.
	_, args, _, err := acpAdapterCommand(BuiltinCodex, ModelSpec{}, true)
	if err != nil {
		t.Fatalf("codex read-only: %v", err)
	}
	if len(args) != 4 || args[2] != "-c" || args[3] != `sandbox_mode="read-only"` {
		t.Fatalf("codex read-only args = %v", args)
	}

	// codex: the sandbox pin composes with a model pin.
	_, args, _, err = acpAdapterCommand(BuiltinCodex, ModelSpec{Model: "gpt-5"}, true)
	if err != nil {
		t.Fatalf("codex read-only+model: %v", err)
	}
	want := []string{"-y", "@zed-industries/codex-acp@latest", "-c", `sandbox_mode="read-only"`, "-c", `model="gpt-5"`}
	if len(args) != len(want) {
		t.Fatalf("codex read-only+model args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("codex read-only+model args = %v, want %v", args, want)
		}
	}

	// claude: read-only adds no adapter flag or env — enforcement lives in the
	// ACP client (write denial + permission rejection), which claude routes
	// writes and permission requests through.
	_, args, env, err := acpAdapterCommand(BuiltinClaude, ModelSpec{}, true)
	if err != nil {
		t.Fatalf("claude read-only: %v", err)
	}
	if len(args) != 2 || len(env) != 0 {
		t.Fatalf("claude read-only must not change args/env: args=%v env=%v", args, env)
	}

	// codex: model-only pin (no read-only) adds only the model override.
	_, args, _, err = acpAdapterCommand(BuiltinCodex, ModelSpec{Model: "gpt-5"}, false)
	if err != nil {
		t.Fatalf("codex model-only: %v", err)
	}
	if len(args) != 4 || args[3] != `model="gpt-5"` {
		t.Fatalf("codex model-only args = %v", args)
	}

	// claude: effort-only pin sets only CLAUDE_CODE_EFFORT_LEVEL.
	_, _, env, err = acpAdapterCommand(BuiltinClaude, ModelSpec{Effort: "high"}, false)
	if err != nil {
		t.Fatalf("claude effort-only: %v", err)
	}
	if len(env) != 1 || env[0] != "CLAUDE_CODE_EFFORT_LEVEL=high" {
		t.Fatalf("claude effort-only env = %v", env)
	}
}

func TestACPClientTokenUsage(t *testing.T) {
	c := &acpClient{}
	if u := c.tokenUsage(); u.Captured {
		t.Fatalf("expected captured=false without usage: %+v", u)
	}

	cacheRead, cacheWrite, thought := 30, 10, 5
	c.recordResult(acp.PromptResponse{
		StopReason: acp.StopReasonEndTurn,
		Usage: &acp.Usage{
			InputTokens:       100,
			OutputTokens:      50,
			TotalTokens:       150,
			CachedReadTokens:  &cacheRead,
			CachedWriteTokens: &cacheWrite,
			ThoughtTokens:     &thought,
		},
	})
	// OTel-inclusive parity with the CLI parsers (docs/cost-budget-design.md):
	// InputTokens folds in cache read+write (100+30+10=140), OutputTokens folds
	// in reasoning (50+5=55), TotalTokens is the provider-reported sum (150), and
	// the cache/reasoning sub-counts are preserved.
	u := c.tokenUsage()
	if !u.Captured || u.InputTokens != 140 || u.OutputTokens != 55 || u.TotalTokens != 150 ||
		u.CacheReadTokens != 30 || u.CacheCreationTokens != 10 || u.ReasoningTokens != 5 {
		t.Fatalf("usage mapping wrong: %+v", u)
	}
	if c.stopReasonValue() != acp.StopReasonEndTurn {
		t.Fatalf("stop reason not recorded: %v", c.stopReasonValue())
	}
}

func TestACPClientWriteTextFileScopeGuard(t *testing.T) {
	repoRoot := t.TempDir()
	write := func(c *acpClient, abs, content string) error {
		_, err := c.WriteTextFile(context.Background(), acp.WriteTextFileRequest{Path: abs, Content: content})
		return err
	}

	// A nil predicate preserves allow-all behavior: the write lands on disk.
	nilGuard := &acpClient{repoRoot: repoRoot}
	nilTarget := filepath.Join(repoRoot, "internal", "app", "nil.go")
	if err := write(nilGuard, nilTarget, "nil"); err != nil {
		t.Fatalf("nil predicate should allow write: %v", err)
	}
	if got, err := os.ReadFile(nilTarget); err != nil || string(got) != "nil" {
		t.Fatalf("nil predicate write missing: got=%q err=%v", got, err)
	}

	// An allowing predicate writes as before.
	allow := &acpClient{repoRoot: repoRoot, writePathAllowed: func(string) bool { return true }}
	allowTarget := filepath.Join(repoRoot, "internal", "app", "allow.go")
	if err := write(allow, allowTarget, "allow"); err != nil {
		t.Fatalf("allow predicate should write: %v", err)
	}
	if _, err := os.Stat(allowTarget); err != nil {
		t.Fatalf("allowed write missing: %v", err)
	}

	// A rejecting predicate denies the write and does not touch disk.
	deny := &acpClient{repoRoot: repoRoot, writePathAllowed: func(string) bool { return false }}
	denyTarget := filepath.Join(repoRoot, "docs", "denied.md")
	if err := write(deny, denyTarget, "deny"); err == nil {
		t.Fatal("rejecting predicate should deny write")
	}
	if _, err := os.Stat(denyTarget); !os.IsNotExist(err) {
		t.Fatalf("denied write must not touch disk: %v", err)
	}

	// A path that escapes the repo is denied even when the predicate allows.
	escape := &acpClient{repoRoot: repoRoot, writePathAllowed: func(string) bool { return true }}
	escapeTarget := filepath.Join(filepath.Dir(repoRoot), "escape.go")
	if err := write(escape, escapeTarget, "escape"); err == nil {
		t.Fatal("escape path should be denied")
	}
	if _, err := os.Stat(escapeTarget); !os.IsNotExist(err) {
		t.Fatalf("escape write must not touch disk: %v", err)
	}
}

func TestACPClientReadOnlyDeniesWrites(t *testing.T) {
	repoRoot := t.TempDir()

	// A read-only client denies the write before touching disk, even when the
	// scope predicate would allow it.
	c := &acpClient{repoRoot: repoRoot, readOnly: true, writePathAllowed: func(string) bool { return true }}
	target := filepath.Join(repoRoot, "internal", "app", "denied.go")
	_, err := c.WriteTextFile(context.Background(), acp.WriteTextFileRequest{Path: target, Content: "deny"})
	if err == nil {
		t.Fatal("read-only client should deny write")
	}
	if !strings.Contains(err.Error(), "acp write denied: read-only stage") {
		t.Fatalf("read-only denial error mismatch: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("read-only denial must not touch disk: %v", err)
	}

	// Reads keep working on a read-only stage.
	source := filepath.Join(repoRoot, "README.md")
	if err := os.WriteFile(source, []byte("# readme"), 0o644); err != nil {
		t.Fatal(err)
	}
	read, err := c.ReadTextFile(context.Background(), acp.ReadTextFileRequest{Path: source})
	if err != nil || read.Content != "# readme" {
		t.Fatalf("read-only client should read: content=%q err=%v", read.Content, err)
	}
}

func TestACPClientReadOnlyRefusesPermissionRequests(t *testing.T) {
	readOnly := &acpClient{readOnly: true}

	// With a reject option present, the reject option is selected.
	resp, err := readOnly.RequestPermission(context.Background(), acp.RequestPermissionRequest{Options: []acp.PermissionOption{
		{Kind: acp.PermissionOptionKindAllowOnce, OptionId: "allow"},
		{Kind: acp.PermissionOptionKindRejectOnce, OptionId: "reject"},
	}})
	if err != nil {
		t.Fatalf("read-only permission: %v", err)
	}
	if resp.Outcome.Selected == nil || resp.Outcome.Selected.OptionId != "reject" {
		t.Fatalf("read-only client should select the reject option: %+v", resp.Outcome)
	}

	// Without a reject option, the request is cancelled, never auto-approved.
	resp, err = readOnly.RequestPermission(context.Background(), acp.RequestPermissionRequest{Options: []acp.PermissionOption{
		{Kind: acp.PermissionOptionKindAllowOnce, OptionId: "allow"},
	}})
	if err != nil {
		t.Fatalf("read-only permission without reject: %v", err)
	}
	if resp.Outcome.Cancelled == nil || resp.Outcome.Selected != nil {
		t.Fatalf("read-only client should cancel without a reject option: %+v", resp.Outcome)
	}

	// A write-stage client (readOnly false) keeps the auto-approve behavior.
	writeStage := &acpClient{}
	resp, err = writeStage.RequestPermission(context.Background(), acp.RequestPermissionRequest{Options: []acp.PermissionOption{
		{Kind: acp.PermissionOptionKindRejectOnce, OptionId: "reject"},
		{Kind: acp.PermissionOptionKindAllowOnce, OptionId: "allow"},
	}})
	if err != nil {
		t.Fatalf("write-stage permission: %v", err)
	}
	if resp.Outcome.Selected == nil || resp.Outcome.Selected.OptionId != "allow" {
		t.Fatalf("write-stage client should auto-approve: %+v", resp.Outcome)
	}
}
