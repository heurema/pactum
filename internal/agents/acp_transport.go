package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/heurema/pactum/internal/artifacts"
)

// ACPTransport drives an agent over the Agent Client Protocol via an adapter
// subprocess (claude-agent-acp / codex-acp) using the coder acp-go-sdk client.
// It produces the same attempt artifacts (stdout.log/stderr.log) and RunResult
// shape as CLITransport, so the attempt lifecycle is unaware of the protocol.
type ACPTransport struct{}

func (ACPTransport) Run(request RunRequest) (RunResult, error) {
	if strings.TrimSpace(request.RepoRoot) == "" {
		return RunResult{}, errors.New("repo root is required")
	}
	if strings.TrimSpace(request.RunID) == "" {
		return RunResult{}, errors.New("run id is required")
	}
	if strings.TrimSpace(request.AttemptID) == "" {
		return RunResult{}, errors.New("attempt id is required")
	}
	if strings.TrimSpace(request.PromptRepoPath) == "" {
		return RunResult{}, errors.New("prompt path is required")
	}
	adapterCmd, adapterArgs, err := acpAdapterCommand(request.Agent.Name)
	if err != nil {
		return RunResult{}, err
	}

	promptPath := filepath.Join(request.RepoRoot, filepath.FromSlash(request.PromptRepoPath))
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		return RunResult{}, err
	}

	artifactDir := strings.Trim(strings.TrimSpace(request.ArtifactDir), "/")
	if artifactDir == "" {
		artifactDir = filepath.ToSlash(filepath.Join("execute", "attempts"))
	}
	attemptDir := filepath.Join(request.RepoRoot, artifacts.WorkspaceRel, "runs", request.RunID, filepath.FromSlash(artifactDir), request.AttemptID)
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return RunResult{}, err
	}
	stdoutArtifact := filepath.ToSlash(filepath.Join(artifactDir, request.AttemptID, "stdout.log"))
	stderrArtifact := filepath.ToSlash(filepath.Join(artifactDir, request.AttemptID, "stderr.log"))
	stdoutFile, err := os.Create(filepath.Join(attemptDir, "stdout.log"))
	if err != nil {
		return RunResult{}, err
	}
	defer stdoutFile.Close()
	stderrFile, err := os.Create(filepath.Join(attemptDir, "stderr.log"))
	if err != nil {
		return RunResult{}, err
	}
	defer stderrFile.Close()

	// The agent's streamed text goes to the attempt stdout.log, teed to the live
	// writer (operator stderr) and wrapped in an activity writer so the idle
	// watchdog sees the streaming progress the protocol gives us.
	var stdoutWriter io.Writer = stdoutFile
	if request.LiveOutput != nil {
		stdoutWriter = io.MultiWriter(stdoutFile, &lockedWriter{w: request.LiveOutput})
	}
	ctx, cancel := context.WithCancel(context.Background())
	var idleTimedOut atomic.Bool
	stopIdle := func() {}
	if request.Timeout > 0 {
		activity := make(chan struct{}, 1)
		stdoutWriter = activityWriter{w: stdoutWriter, activity: activity}
		stopIdle = startIdleTimeout(request.Timeout, activity, cancel, &idleTimedOut)
	}
	defer cancel()

	// The adapter speaks ACP (JSON-RPC) on its stdin/stdout; its own diagnostics
	// go to stderr (captured to the attempt stderr.log).
	cmd := exec.CommandContext(ctx, adapterCmd, adapterArgs...)
	cmd.Dir = request.RepoRoot
	cmd.Env = os.Environ()
	cmd.Stderr = stderrFile
	// Run the adapter in its own process group so the whole tree (the npx wrapper,
	// the adapter, and the agent child it launches) can be reaped together.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	adapterIn, err := cmd.StdinPipe()
	if err != nil {
		return RunResult{}, err
	}
	adapterOut, err := cmd.StdoutPipe()
	if err != nil {
		return RunResult{}, err
	}

	started := time.Now().UTC()
	if err := cmd.Start(); err != nil {
		return RunResult{}, err
	}

	client := &acpClient{out: stdoutWriter}
	conn := acp.NewClientSideConnection(client, adapterIn, adapterOut)
	runErr := driveACPSession(ctx, conn, request.RepoRoot, string(prompt), client)

	finished := time.Now().UTC()
	stopIdle()
	cancel()
	killProcessGroup(cmd)

	exitCode := 0
	timedOut := idleTimedOut.Load()
	if runErr != nil {
		exitCode = -1
		fmt.Fprintln(stderrFile, runErr.Error())
	} else if client.stopReasonValue() == acp.StopReasonRefusal {
		exitCode = 1
	}
	if timedOut {
		exitCode = -1
	}

	return RunResult{
		Command:        adapterCmd,
		Args:           append([]string{}, adapterArgs...),
		ExitCode:       exitCode,
		StartedAt:      started.Format(time.RFC3339Nano),
		FinishedAt:     finished.Format(time.RFC3339Nano),
		DurationMillis: finished.Sub(started).Milliseconds(),
		TimedOut:       timedOut,
		StdoutPath:     stdoutArtifact,
		StderrPath:     stderrArtifact,
		Usage:          client.tokenUsage(),
	}, runErr
}

// driveACPSession runs one ACP prompt turn: initialize, open a session rooted at
// the repo, send the prompt, and record the turn result.
func driveACPSession(ctx context.Context, conn *acp.ClientSideConnection, cwd string, prompt string, client *acpClient) error {
	if _, err := conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion:    acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{Fs: acp.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true}},
	}); err != nil {
		return fmt.Errorf("acp initialize: %w", err)
	}
	session, err := conn.NewSession(ctx, acp.NewSessionRequest{Cwd: cwd, McpServers: []acp.McpServer{}})
	if err != nil {
		return fmt.Errorf("acp new session: %w", err)
	}
	resp, err := conn.Prompt(ctx, acp.PromptRequest{
		SessionId: session.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock(prompt)},
	})
	if err != nil {
		return fmt.Errorf("acp prompt: %w", err)
	}
	client.recordResult(resp)
	return nil
}

// killProcessGroup reaps the adapter and the whole process tree it launched
// (npx wrapper, adapter, agent child) via the process group set with Setpgid.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_, _ = cmd.Process.Wait()
}

// acpAdapterCommand maps a built-in agent name to the command that launches its
// ACP server adapter. The adapters are external npm packages run via npx; they
// inherit the process environment (and thus the agent's auth) from the parent.
func acpAdapterCommand(agentName string) (string, []string, error) {
	switch agentName {
	case BuiltinClaude:
		return "npx", []string{"-y", "@agentclientprotocol/claude-agent-acp@latest"}, nil
	case BuiltinCodex:
		return "npx", []string{"-y", "@zed-industries/codex-acp@latest"}, nil
	default:
		return "", nil, fmt.Errorf("no ACP adapter configured for agent %q", agentName)
	}
}

// acpClient implements acp.Client: it auto-approves permission requests (scope is
// still enforced post-hoc by the gate), services the agent's file reads/writes
// against the working tree, streams the agent's text to the attempt log, and
// records the turn's token usage.
type acpClient struct {
	out io.Writer

	mu         sync.Mutex
	stopReason acp.StopReason
	usage      *acp.Usage
}

var _ acp.Client = (*acpClient)(nil)

func (c *acpClient) recordResult(resp acp.PromptResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopReason = resp.StopReason
	c.usage = resp.Usage
}

func (c *acpClient) stopReasonValue() acp.StopReason {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopReason
}

func (c *acpClient) tokenUsage() TokenUsage {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.usage == nil {
		return TokenUsage{Captured: false, CaptureWarning: "acp prompt returned no usage"}
	}
	u := c.usage
	return TokenUsage{
		InputTokens:         int64(u.InputTokens),
		OutputTokens:        int64(u.OutputTokens),
		TotalTokens:         int64(u.TotalTokens),
		CacheReadTokens:     derefIntToInt64(u.CachedReadTokens),
		CacheCreationTokens: derefIntToInt64(u.CachedWriteTokens),
		ReasoningTokens:     derefIntToInt64(u.ThoughtTokens),
		Captured:            true,
	}
}

func derefIntToInt64(p *int) int64 {
	if p == nil {
		return 0
	}
	return int64(*p)
}

func (c *acpClient) RequestPermission(ctx context.Context, p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	for _, o := range p.Options {
		if o.Kind == acp.PermissionOptionKindAllowOnce || o.Kind == acp.PermissionOptionKindAllowAlways {
			return acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{Selected: &acp.RequestPermissionOutcomeSelected{OptionId: o.OptionId}}}, nil
		}
	}
	if len(p.Options) > 0 {
		return acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{Selected: &acp.RequestPermissionOutcomeSelected{OptionId: p.Options[0].OptionId}}}, nil
	}
	return acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{Cancelled: &acp.RequestPermissionOutcomeCancelled{}}}, nil
}

func (c *acpClient) SessionUpdate(ctx context.Context, p acp.SessionNotification) error {
	u := p.Update
	if u.AgentMessageChunk != nil && u.AgentMessageChunk.Content.Text != nil {
		c.mu.Lock()
		_, _ = io.WriteString(c.out, u.AgentMessageChunk.Content.Text.Text)
		c.mu.Unlock()
	}
	return nil
}

func (c *acpClient) WriteTextFile(ctx context.Context, p acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	if !filepath.IsAbs(p.Path) {
		return acp.WriteTextFileResponse{}, fmt.Errorf("acp write: path must be absolute: %s", p.Path)
	}
	if err := os.MkdirAll(filepath.Dir(p.Path), 0o755); err != nil {
		return acp.WriteTextFileResponse{}, err
	}
	return acp.WriteTextFileResponse{}, os.WriteFile(p.Path, []byte(p.Content), 0o644)
}

func (c *acpClient) ReadTextFile(ctx context.Context, p acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	if !filepath.IsAbs(p.Path) {
		return acp.ReadTextFileResponse{}, fmt.Errorf("acp read: path must be absolute: %s", p.Path)
	}
	b, err := os.ReadFile(p.Path)
	if err != nil {
		return acp.ReadTextFileResponse{}, err
	}
	return acp.ReadTextFileResponse{Content: string(b)}, nil
}

func (c *acpClient) CreateTerminal(ctx context.Context, p acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, fmt.Errorf("acp terminals are not supported")
}

func (c *acpClient) KillTerminal(ctx context.Context, p acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, nil
}

func (c *acpClient) TerminalOutput(ctx context.Context, p acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, nil
}

func (c *acpClient) ReleaseTerminal(ctx context.Context, p acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, nil
}

func (c *acpClient) WaitForTerminalExit(ctx context.Context, p acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, nil
}
