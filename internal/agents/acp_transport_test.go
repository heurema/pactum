package agents

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func clearACPAdapterOverrides(t *testing.T) {
	t.Helper()
	t.Setenv("PACTUM_CLAUDE_ACP_COMMAND", "")
	t.Setenv("PACTUM_CODEX_ACP_COMMAND", "")
}

func TestACPAdapterCommand(t *testing.T) {
	clearACPAdapterOverrides(t)

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
	clearACPAdapterOverrides(t)

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
	clearACPAdapterOverrides(t)

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

func TestACPAdapterCommandOverrideReplacesOnlyDefaultPrefix(t *testing.T) {
	t.Setenv("PACTUM_CLAUDE_ACP_COMMAND", "vendor/bin/claude-agent-acp")
	t.Setenv("PACTUM_CODEX_ACP_COMMAND", "vendor/bin/codex-acp")

	cmd, args, env, err := acpAdapterCommand(BuiltinClaude, ModelSpec{Model: "claude-sonnet-4", Effort: "high"}, false)
	if err != nil {
		t.Fatalf("claude override: %v", err)
	}
	if cmd != "vendor/bin/claude-agent-acp" {
		t.Fatalf("claude override cmd = %q", cmd)
	}
	if len(args) != 0 {
		t.Fatalf("claude override must drop only npx/package args, got %v", args)
	}
	wantEnv := []string{"ANTHROPIC_MODEL=claude-sonnet-4", "CLAUDE_CODE_EFFORT_LEVEL=high"}
	if !reflect.DeepEqual(env, wantEnv) {
		t.Fatalf("claude override env = %v, want %v", env, wantEnv)
	}

	cmd, args, env, err = acpAdapterCommand(BuiltinCodex, ModelSpec{Model: "gpt-5", Effort: "high"}, true)
	if err != nil {
		t.Fatalf("codex override: %v", err)
	}
	if cmd != "vendor/bin/codex-acp" {
		t.Fatalf("codex override cmd = %q", cmd)
	}
	wantArgs := []string{"-c", `sandbox_mode="read-only"`, "-c", `model="gpt-5"`, "-c", "model_reasoning_effort=high"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("codex override args = %v, want %v", args, wantArgs)
	}
	if len(env) != 0 {
		t.Fatalf("codex override must preserve empty env, got %v", env)
	}
}

func TestACPAdapterCommandOverrideIgnoresEmptyAndWhitespace(t *testing.T) {
	t.Setenv("PACTUM_CLAUDE_ACP_COMMAND", " \t\n ")
	t.Setenv("PACTUM_CODEX_ACP_COMMAND", "")

	cmd, args, _, err := acpAdapterCommand(BuiltinClaude, ModelSpec{}, false)
	if err != nil {
		t.Fatalf("claude whitespace override: %v", err)
	}
	if cmd != "npx" || !reflect.DeepEqual(args, []string{"-y", "@agentclientprotocol/claude-agent-acp@latest"}) {
		t.Fatalf("claude whitespace override should use default: cmd=%q args=%v", cmd, args)
	}

	cmd, args, _, err = acpAdapterCommand(BuiltinCodex, ModelSpec{}, false)
	if err != nil {
		t.Fatalf("codex empty override: %v", err)
	}
	if cmd != "npx" || !reflect.DeepEqual(args, []string{"-y", "@zed-industries/codex-acp@latest"}) {
		t.Fatalf("codex empty override should use default: cmd=%q args=%v", cmd, args)
	}
}

func TestACPAdapterCommandOverrideIsSingleExecutable(t *testing.T) {
	t.Setenv("PACTUM_CODEX_ACP_COMMAND", "npx -y @zed-industries/codex-acp@1.2.3")
	t.Setenv("PACTUM_CLAUDE_ACP_COMMAND", "")

	cmd, args, _, err := acpAdapterCommand(BuiltinCodex, ModelSpec{Model: "gpt-5"}, false)
	if err != nil {
		t.Fatalf("codex command-line-looking override: %v", err)
	}
	if cmd != "npx -y @zed-industries/codex-acp@1.2.3" {
		t.Fatalf("override must be treated as one executable path, got %q", cmd)
	}
	if !reflect.DeepEqual(args, []string{"-c", `model="gpt-5"`}) {
		t.Fatalf("override args = %v", args)
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
	// PromptResponse.Usage input/output are the parent counts; cache/reasoning
	// are preserved as sub-counts without double-counting them.
	u := c.tokenUsage()
	if !u.Captured || u.InputTokens != 100 || u.OutputTokens != 50 || u.TotalTokens != 150 ||
		u.CacheReadTokens != 30 || u.CacheCreationTokens != 10 || u.ReasoningTokens != 5 {
		t.Fatalf("usage mapping wrong: %+v", u)
	}
	if c.stopReasonValue() != acp.StopReasonEndTurn {
		t.Fatalf("stop reason not recorded: %v", c.stopReasonValue())
	}
}

func TestACPClientTokenUsageFallsBackToCodexACPMetadata(t *testing.T) {
	c := &acpClient{}
	if err := c.SessionUpdate(context.Background(), acp.SessionNotification{Update: acp.SessionUpdate{
		UsageUpdate: &acp.SessionUsageUpdate{Meta: map[string]any{"codex/token_usage": map[string]any{
			"total_token_usage": map[string]any{
				"input_tokens":            100,
				"cached_input_tokens":     25,
				"output_tokens":           30,
				"reasoning_output_tokens": 7,
				"total_tokens":            137,
			},
		}}},
	}}); err != nil {
		t.Fatalf("usage update: %v", err)
	}

	u := c.tokenUsage()
	if !u.Captured || u.InputTokens != 100 || u.OutputTokens != 37 || u.TotalTokens != 137 ||
		u.CacheReadTokens != 25 || u.CacheCreationTokens != 0 || u.ReasoningTokens != 7 {
		t.Fatalf("codex acp metadata usage mapping wrong: %+v", u)
	}
	if len(u.Raw) == 0 {
		t.Fatal("codex acp metadata fallback should retain raw usage metadata")
	}
}

func TestACPClientTokenUsageFallsBackToCodexACPMetadataWithoutReasoning(t *testing.T) {
	c := &acpClient{}
	if err := c.SessionUpdate(context.Background(), acp.SessionNotification{Update: acp.SessionUpdate{
		UsageUpdate: &acp.SessionUsageUpdate{Meta: map[string]any{"codex/token_usage": map[string]any{
			"total_token_usage": map[string]any{
				"input_tokens":        100,
				"cached_input_tokens": 25,
				"output_tokens":       30,
				"total_tokens":        130,
			},
		}}},
	}}); err != nil {
		t.Fatalf("usage update: %v", err)
	}

	u := c.tokenUsage()
	if !u.Captured || u.InputTokens != 100 || u.OutputTokens != 30 || u.TotalTokens != 130 ||
		u.CacheReadTokens != 25 || u.ReasoningTokens != 0 {
		t.Fatalf("codex acp metadata without reasoning should be captured: %+v", u)
	}
}

func TestACPClientCodexACPMetadataKeepsLatestValidTotals(t *testing.T) {
	c := &acpClient{}
	send := func(meta map[string]any) {
		if err := c.SessionUpdate(context.Background(), acp.SessionNotification{Update: acp.SessionUpdate{
			UsageUpdate: &acp.SessionUsageUpdate{Meta: meta},
		}}); err != nil {
			t.Fatalf("usage update: %v", err)
		}
	}

	send(map[string]any{"codex/token_usage": map[string]any{"total_token_usage": map[string]any{
		"input_tokens": 10, "cached_input_tokens": 2, "output_tokens": 3, "reasoning_output_tokens": 1, "total_tokens": 14,
	}}})
	send(map[string]any{"codex/token_usage": map[string]any{"total_token_usage": map[string]any{
		"input_tokens": 100, "cached_input_tokens": 20, "output_tokens": 30, "reasoning_output_tokens": 4, "total_tokens": 134,
	}}})
	send(map[string]any{"codex/token_usage": map[string]any{"total_token_usage": "not an object"}})
	send(map[string]any{})

	u := c.tokenUsage()
	if !u.Captured || u.InputTokens != 100 || u.CacheReadTokens != 20 || u.OutputTokens != 34 || u.TotalTokens != 134 || u.ReasoningTokens != 4 {
		t.Fatalf("latest valid codex acp metadata not retained: %+v", u)
	}
}

func TestACPClientPromptResponseUsageWinsOverCodexACPMetadata(t *testing.T) {
	cacheRead, cacheWrite, thought := 1, 2, 3
	c := &acpClient{}
	if err := c.SessionUpdate(context.Background(), acp.SessionNotification{Update: acp.SessionUpdate{
		UsageUpdate: &acp.SessionUsageUpdate{Meta: map[string]any{"codex/token_usage": map[string]any{"total_token_usage": map[string]any{
			"input_tokens": 100, "cached_input_tokens": 20, "output_tokens": 30, "reasoning_output_tokens": 4, "total_tokens": 134,
		}}}},
	}}); err != nil {
		t.Fatalf("usage update: %v", err)
	}
	c.recordResult(acp.PromptResponse{Usage: &acp.Usage{
		InputTokens:       5,
		OutputTokens:      6,
		TotalTokens:       7,
		CachedReadTokens:  &cacheRead,
		CachedWriteTokens: &cacheWrite,
		ThoughtTokens:     &thought,
	}})

	u := c.tokenUsage()
	if !u.Captured || u.InputTokens != 5 || u.OutputTokens != 6 || u.TotalTokens != 11 ||
		u.CacheReadTokens != 1 || u.CacheCreationTokens != 2 || u.ReasoningTokens != 3 {
		t.Fatalf("prompt response usage should win over metadata fallback: %+v", u)
	}
}

func TestDriveACPSessionCapturesPromptResponseUsage(t *testing.T) {
	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()
	t.Cleanup(func() {
		_ = clientToAgentR.Close()
		_ = clientToAgentW.Close()
		_ = agentToClientR.Close()
		_ = agentToClientW.Close()
	})

	cacheRead, cacheWrite, thought := 1, 2, 3
	agent := &promptUsageAgent{usage: &acp.Usage{
		InputTokens:       5,
		OutputTokens:      6,
		TotalTokens:       7,
		CachedReadTokens:  &cacheRead,
		CachedWriteTokens: &cacheWrite,
		ThoughtTokens:     &thought,
	}}
	agentConn := acp.NewAgentSideConnection(agent, agentToClientW, clientToAgentR)
	agent.conn = agentConn

	client := &acpClient{out: io.Discard, repoRoot: t.TempDir()}
	conn := acp.NewClientSideConnection(client, clientToAgentW, agentToClientR)
	if err := driveACPSession(context.Background(), conn, client.repoRoot, "prompt", client); err != nil {
		t.Fatalf("drive ACP session: %v", err)
	}

	u := client.tokenUsage()
	if !u.Captured || u.CaptureWarning != "" || u.InputTokens != 5 || u.OutputTokens != 6 || u.TotalTokens != 11 ||
		u.CacheReadTokens != 1 || u.CacheCreationTokens != 2 || u.ReasoningTokens != 3 {
		t.Fatalf("prompt response usage should be captured without warning: %+v", u)
	}
}

func TestACPClientCodexACPMetadataMissesPreserveNoUsageWarning(t *testing.T) {
	for _, tc := range []struct {
		name string
		meta map[string]any
	}{
		{name: "absent", meta: nil},
		{name: "wrong key", meta: map[string]any{"other": map[string]any{}}},
		{name: "missing total", meta: map[string]any{"codex/token_usage": map[string]any{"last_token_usage": map[string]any{"input_tokens": 1}}}},
		{name: "empty total", meta: map[string]any{"codex/token_usage": map[string]any{"total_token_usage": map[string]any{}}}},
		{name: "unknown-only total", meta: map[string]any{"codex/token_usage": map[string]any{"total_token_usage": map[string]any{"foo": 1}}}},
		{name: "missing token field", meta: map[string]any{"codex/token_usage": map[string]any{"total_token_usage": map[string]any{
			"input_tokens": 1, "cached_input_tokens": 2, "output_tokens": 3, "reasoning_output_tokens": 0,
		}}}},
		{name: "wrong token field type", meta: map[string]any{"codex/token_usage": map[string]any{"total_token_usage": map[string]any{
			"input_tokens": "1", "cached_input_tokens": 2, "output_tokens": 3, "reasoning_output_tokens": 0, "total_tokens": 6,
		}}}},
		{name: "malformed total", meta: map[string]any{"codex/token_usage": map[string]any{"total_token_usage": "nope"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &acpClient{}
			if err := c.SessionUpdate(context.Background(), acp.SessionNotification{Update: acp.SessionUpdate{
				UsageUpdate: &acp.SessionUsageUpdate{Meta: tc.meta},
			}}); err != nil {
				t.Fatalf("usage update: %v", err)
			}
			u := c.tokenUsage()
			if u.Captured || u.CaptureWarning != "acp prompt returned no usage" {
				t.Fatalf("metadata miss should preserve no-usage warning: %+v", u)
			}
		})
	}
}

func TestACPTransportWritesNoUsageWarning(t *testing.T) {
	repoRoot := t.TempDir()
	promptPath := filepath.Join(repoRoot, "prompt.md")
	if err := os.WriteFile(promptPath, []byte("prompt"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	adapterPath := filepath.Join(repoRoot, "exit-adapter.sh")
	if err := os.WriteFile(adapterPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write adapter: %v", err)
	}
	t.Setenv("PACTUM_CODEX_ACP_COMMAND", adapterPath)
	t.Setenv("PACTUM_CLAUDE_ACP_COMMAND", "")

	result, err := (ACPTransport{}).Run(RunRequest{
		RepoRoot:       repoRoot,
		RunID:          "run_1",
		AttemptID:      "attempt_1",
		Agent:          AgentDescriptor{Name: BuiltinCodex},
		PromptRepoPath: "prompt.md",
	})
	if err == nil {
		t.Fatal("exiting adapter should fail ACP session")
	}
	if result.Usage.Captured || result.Usage.CaptureWarning != "acp prompt returned no usage" {
		t.Fatalf("expected uncaptured usage warning in result: %+v", result.Usage)
	}
	stderrPath := filepath.Join(repoRoot, ".heurema", "pactum", "runs", "run_1", filepath.FromSlash(result.StderrPath))
	stderr, readErr := os.ReadFile(stderrPath)
	if readErr != nil {
		t.Fatalf("read stderr: %v", readErr)
	}
	if !strings.Contains(string(stderr), "usage capture warning: acp prompt returned no usage") {
		t.Fatalf("stderr missing usage warning:\n%s", stderr)
	}
}

func TestACPClientCodexACPMetadataCapturesExplicitZeroTotal(t *testing.T) {
	c := &acpClient{}
	if err := c.SessionUpdate(context.Background(), acp.SessionNotification{Update: acp.SessionUpdate{
		UsageUpdate: &acp.SessionUsageUpdate{Meta: map[string]any{"codex/token_usage": map[string]any{"total_token_usage": map[string]any{
			"input_tokens": 0, "cached_input_tokens": 0, "output_tokens": 0, "reasoning_output_tokens": 0, "total_tokens": 0,
		}}}},
	}}); err != nil {
		t.Fatalf("usage update: %v", err)
	}
	u := c.tokenUsage()
	if !u.Captured || u.InputTokens != 0 || u.OutputTokens != 0 || u.TotalTokens != 0 || u.CaptureWarning != "" {
		t.Fatalf("explicit zero total should be captured as real usage: %+v", u)
	}
}

func TestACPClientTurnCompleted(t *testing.T) {
	// No recorded prompt response: the turn never finished before the kill.
	c := &acpClient{}
	if c.turnCompleted() {
		t.Fatal("turn must not count as completed without a recorded response")
	}

	// A recorded end-turn response means the turn genuinely finished.
	c.recordResult(acp.PromptResponse{StopReason: acp.StopReasonEndTurn})
	if !c.turnCompleted() {
		t.Fatal("a recorded end_turn response is a completed turn")
	}

	// A refusal is a refused turn, not completed work.
	refused := &acpClient{}
	refused.recordResult(acp.PromptResponse{StopReason: acp.StopReasonRefusal})
	if refused.turnCompleted() {
		t.Fatal("a refusal response must not count as completed work")
	}
}

type promptUsageAgent struct {
	conn  *acp.AgentSideConnection
	usage *acp.Usage
}

var _ acp.Agent = (*promptUsageAgent)(nil)

func (a *promptUsageAgent) Authenticate(context.Context, acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}

func (a *promptUsageAgent) Initialize(context.Context, acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{
		ProtocolVersion:   acp.ProtocolVersionNumber,
		AgentCapabilities: acp.AgentCapabilities{},
	}, nil
}

func (a *promptUsageAgent) Logout(context.Context, acp.LogoutRequest) (acp.LogoutResponse, error) {
	return acp.LogoutResponse{}, nil
}

func (a *promptUsageAgent) Cancel(context.Context, acp.CancelNotification) error {
	return nil
}

func (a *promptUsageAgent) CloseSession(context.Context, acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	return acp.CloseSessionResponse{}, nil
}

func (a *promptUsageAgent) ListSessions(context.Context, acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, nil
}

func (a *promptUsageAgent) NewSession(context.Context, acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	return acp.NewSessionResponse{SessionId: acp.SessionId("sess_usage")}, nil
}

func (a *promptUsageAgent) Prompt(ctx context.Context, p acp.PromptRequest) (acp.PromptResponse, error) {
	_ = a.conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: p.SessionId,
		Update: acp.SessionUpdate{UsageUpdate: &acp.SessionUsageUpdate{Meta: map[string]any{
			"codex/token_usage": map[string]any{"total_token_usage": map[string]any{
				"input_tokens": 100, "cached_input_tokens": 20, "output_tokens": 30, "reasoning_output_tokens": 4, "total_tokens": 134,
			}},
		}}},
	})
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn, Usage: a.usage}, nil
}

func (a *promptUsageAgent) ResumeSession(context.Context, acp.ResumeSessionRequest) (acp.ResumeSessionResponse, error) {
	return acp.ResumeSessionResponse{}, nil
}

func (a *promptUsageAgent) SetSessionConfigOption(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, nil
}

func (a *promptUsageAgent) SetSessionMode(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, nil
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
	// scope predicate would allow it. The denied write still ticks the idle
	// watchdog — a refused tool call proves the agent is alive.
	ticks := 0
	c := &acpClient{repoRoot: repoRoot, readOnly: true, writePathAllowed: func(string) bool { return true }, activity: func() { ticks++ }}
	target := filepath.Join(repoRoot, "internal", "app", "denied.go")
	_, err := c.WriteTextFile(context.Background(), acp.WriteTextFileRequest{Path: target, Content: "deny"})
	if err == nil {
		t.Fatal("read-only client should deny write")
	}
	if ticks != 1 {
		t.Fatalf("denied write must still tick the watchdog: ticks = %d", ticks)
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

func TestACPClientActivityTicksOnAllInboundCalls(t *testing.T) {
	repoRoot := t.TempDir()
	var out strings.Builder
	ticks := 0
	c := &acpClient{out: &out, activity: func() { ticks++ }, repoRoot: repoRoot}
	ctx := context.Background()

	// Silent session updates — a tool call, a tool-call update, a thought
	// chunk, a plan — tick the watchdog and write nothing to the output.
	silent := []acp.SessionNotification{
		{Update: acp.SessionUpdate{ToolCall: &acp.SessionUpdateToolCall{}}},
		{Update: acp.SessionUpdate{ToolCallUpdate: &acp.SessionToolCallUpdate{}}},
		{Update: acp.SessionUpdate{AgentThoughtChunk: &acp.SessionUpdateAgentThoughtChunk{Content: acp.TextBlock("thinking")}}},
		{Update: acp.SessionUpdate{Plan: &acp.SessionUpdatePlan{}}},
	}
	for i, n := range silent {
		before := ticks
		if err := c.SessionUpdate(ctx, n); err != nil {
			t.Fatalf("silent update %d: %v", i, err)
		}
		if ticks != before+1 {
			t.Fatalf("silent update %d must tick exactly once, ticks %d -> %d", i, before, ticks)
		}
	}
	if out.String() != "" {
		t.Fatalf("silent updates must write nothing to the output, got %q", out.String())
	}

	// An agent message chunk ticks and writes its text.
	before := ticks
	err := c.SessionUpdate(ctx, acp.SessionNotification{Update: acp.SessionUpdate{
		AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("hello")},
	}})
	if err != nil {
		t.Fatalf("message chunk: %v", err)
	}
	if ticks != before+1 {
		t.Fatalf("message chunk must tick, ticks %d -> %d", before, ticks)
	}
	if out.String() != "hello" {
		t.Fatalf("message chunk must write its text, got %q", out.String())
	}

	// Permission requests, client-serviced file reads/writes, and the terminal
	// methods all tick: any inbound protocol traffic proves the agent is alive.
	source := filepath.Join(repoRoot, "read.txt")
	if err := os.WriteFile(source, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	before = ticks
	if _, err := c.RequestPermission(ctx, acp.RequestPermissionRequest{}); err != nil {
		t.Fatalf("permission: %v", err)
	}
	if _, err := c.ReadTextFile(ctx, acp.ReadTextFileRequest{Path: source}); err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, err := c.WriteTextFile(ctx, acp.WriteTextFileRequest{Path: filepath.Join(repoRoot, "write.txt"), Content: "y"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = c.CreateTerminal(ctx, acp.CreateTerminalRequest{}) // errs (unsupported) but must still tick
	if _, err := c.KillTerminal(ctx, acp.KillTerminalRequest{}); err != nil {
		t.Fatalf("kill terminal: %v", err)
	}
	if _, err := c.TerminalOutput(ctx, acp.TerminalOutputRequest{}); err != nil {
		t.Fatalf("terminal output: %v", err)
	}
	if _, err := c.ReleaseTerminal(ctx, acp.ReleaseTerminalRequest{}); err != nil {
		t.Fatalf("release terminal: %v", err)
	}
	if _, err := c.WaitForTerminalExit(ctx, acp.WaitForTerminalExitRequest{}); err != nil {
		t.Fatalf("wait terminal: %v", err)
	}
	if ticks != before+8 {
		t.Fatalf("each inbound call must tick exactly once, ticks %d -> %d (want +8)", before, ticks)
	}
	// Ticking is signal-only: across everything, only the streamed text landed.
	if out.String() != "hello" {
		t.Fatalf("output must hold only streamed agent text, got %q", out.String())
	}
}

func TestACPClientSeparatesMessageBoundariesWithNewline(t *testing.T) {
	var out strings.Builder
	c := &acpClient{out: &out}
	ctx := context.Background()
	send := func(text string, messageID string) {
		chunk := &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock(text)}
		if messageID != "" {
			chunk.MessageId = &messageID
		}
		err := c.SessionUpdate(ctx, acp.SessionNotification{Update: acp.SessionUpdate{AgentMessageChunk: chunk}})
		if err != nil {
			t.Fatalf("session update %q: %v", text, err)
		}
	}

	// Delta chunks sharing a messageId stream one message: no separator inside.
	send("Hel", "msg_1")
	send("lo.", "msg_1")
	// A messageId change is a message boundary: the fenced block that would
	// otherwise glue to the prose gets its own line.
	send("```json\n{\"k\":1}\n```", "msg_2")
	// An empty chunk writes nothing and does not move the boundary state.
	send("", "msg_3")
	// A new id after a trailing newline needs no extra separator.
	send("done.\n", "msg_4")
	send("end", "msg_5")

	want := "Hello.\n```json\n{\"k\":1}\n```\ndone.\nend"
	if out.String() != want {
		t.Fatalf("out = %q, want %q", out.String(), want)
	}
}

func TestACPClientNeverSeparatesIdlessChunks(t *testing.T) {
	// Adapters that stamp no messageId stream raw token deltas: a separator
	// between any two of them would land inside words or JSON string literals
	// and corrupt the output, so id-less chunks must concatenate verbatim —
	// even when that glues a fence to prose (the parse layer handles glue).
	var out strings.Builder
	c := &acpClient{out: &out}
	ctx := context.Background()
	send := func(text string) {
		chunk := &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock(text)}
		err := c.SessionUpdate(ctx, acp.SessionNotification{Update: acp.SessionUpdate{AgentMessageChunk: chunk}})
		if err != nil {
			t.Fatalf("session update %q: %v", text, err)
		}
	}

	send("{\"questions\": [{\"text\": \"What does ")
	send("full record")
	send(" mean?\"}]}")
	send("All settled.")
	send("```json")

	want := "{\"questions\": [{\"text\": \"What does full record mean?\"}]}All settled.```json"
	if out.String() != want {
		t.Fatalf("out = %q, want %q", out.String(), want)
	}
}

func TestACPClientNilActivityIsSafe(t *testing.T) {
	// Without an armed timeout the callback is nil; every inbound method must
	// be a safe no-op tick (no panic), preserving its normal behavior.
	repoRoot := t.TempDir()
	var out strings.Builder
	c := &acpClient{out: &out, repoRoot: repoRoot}
	ctx := context.Background()

	err := c.SessionUpdate(ctx, acp.SessionNotification{Update: acp.SessionUpdate{ToolCall: &acp.SessionUpdateToolCall{}}})
	if err != nil {
		t.Fatalf("tool call: %v", err)
	}
	err = c.SessionUpdate(ctx, acp.SessionNotification{Update: acp.SessionUpdate{
		AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("hi")},
	}})
	if err != nil {
		t.Fatalf("message chunk: %v", err)
	}
	if out.String() != "hi" {
		t.Fatalf("nil callback must not change writes, got %q", out.String())
	}
	source := filepath.Join(repoRoot, "read.txt")
	if err := os.WriteFile(source, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RequestPermission(ctx, acp.RequestPermissionRequest{}); err != nil {
		t.Fatalf("permission: %v", err)
	}
	if _, err := c.ReadTextFile(ctx, acp.ReadTextFileRequest{Path: source}); err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, err := c.WriteTextFile(ctx, acp.WriteTextFileRequest{Path: filepath.Join(repoRoot, "write.txt"), Content: "y"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = c.CreateTerminal(ctx, acp.CreateTerminalRequest{})
	if _, err := c.KillTerminal(ctx, acp.KillTerminalRequest{}); err != nil {
		t.Fatalf("kill terminal: %v", err)
	}
	if _, err := c.TerminalOutput(ctx, acp.TerminalOutputRequest{}); err != nil {
		t.Fatalf("terminal output: %v", err)
	}
	if _, err := c.ReleaseTerminal(ctx, acp.ReleaseTerminalRequest{}); err != nil {
		t.Fatalf("release terminal: %v", err)
	}
	if _, err := c.WaitForTerminalExit(ctx, acp.WaitForTerminalExitRequest{}); err != nil {
		t.Fatalf("wait terminal: %v", err)
	}
}

func TestACPClientFiresFirstOutputOnFirstAgentMessage(t *testing.T) {
	ctx := context.Background()
	var out strings.Builder
	fires := 0
	c := &acpClient{out: &out, onFirstOutput: func() { fires++ }}

	update := func(u acp.SessionUpdate) {
		if err := c.SessionUpdate(ctx, acp.SessionNotification{Update: u}); err != nil {
			t.Fatalf("session update: %v", err)
		}
	}

	// A thought chunk writes nothing visible; an empty message chunk writes
	// nothing either. Neither is first output.
	update(acp.SessionUpdate{AgentThoughtChunk: &acp.SessionUpdateAgentThoughtChunk{Content: acp.TextBlock("thinking")}})
	update(acp.SessionUpdate{AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("")}})
	if fires != 0 {
		t.Fatalf("first output fired %d times before any visible text, want 0", fires)
	}

	// The first non-empty agent message chunk fires exactly once; later chunks
	// do not fire again.
	update(acp.SessionUpdate{AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("hello")}})
	update(acp.SessionUpdate{AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock(" world")}})
	if fires != 1 {
		t.Fatalf("first output fired %d times, want exactly 1", fires)
	}
}

func TestACPClientNilFirstOutputIsSafe(t *testing.T) {
	var out strings.Builder
	c := &acpClient{out: &out}
	if err := c.SessionUpdate(context.Background(), acp.SessionNotification{Update: acp.SessionUpdate{
		AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("hi")},
	}}); err != nil {
		t.Fatalf("session update with nil first-output callback: %v", err)
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
