package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// ACPTransport drives an agent over the Agent Client Protocol via an adapter
// subprocess (claude-agent-acp / codex-acp) using the coder acp-go-sdk client.
// It produces the same attempt artifacts (stdout.log/stderr.log) and RunResult
// shape as CLITransport, so the attempt lifecycle is unaware of the protocol.
type ACPTransport struct{}

func (ACPTransport) Run(request RunRequest) (RunResult, error) {
	if err := validateRunRequest(request); err != nil {
		return RunResult{}, err
	}
	adapterCmd, adapterArgs, adapterEnv, err := acpAdapterCommand(request.Agent.Name, request.Model, request.ReadOnly)
	if err != nil {
		return RunResult{}, err
	}

	promptPath := filepath.Join(request.RepoRoot, filepath.FromSlash(request.PromptRepoPath))
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		return RunResult{}, err
	}

	layout, err := attemptArtifactLayout(request)
	if err != nil {
		return RunResult{}, err
	}
	stdoutFile, stderrFile, err := createAttemptLogs(layout)
	if err != nil {
		return RunResult{}, err
	}
	defer stdoutFile.Close()
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
	// Over ACP, streamed text is only one liveness signal: an agent can work
	// silently through tool calls for minutes. Every inbound client callback
	// ticks the same activity channel (same non-blocking send semantics as
	// activityWriter) so silent protocol traffic also resets the idle timer.
	var clientActivity func()
	if request.Timeout > 0 {
		activity := make(chan struct{}, 1)
		stdoutWriter = activityWriter{w: stdoutWriter, activity: activity}
		clientActivity = func() {
			select {
			case activity <- struct{}{}:
			default:
			}
		}
		stopIdle = startIdleTimeout(request.Timeout, activity, cancel, &idleTimedOut)
	}
	defer cancel()

	// The adapter speaks ACP (JSON-RPC) on its stdin/stdout; its own diagnostics
	// go to stderr (captured to the attempt stderr.log).
	cmd := exec.CommandContext(ctx, adapterCmd, adapterArgs...)
	cmd.Dir = request.RepoRoot
	cmd.Env = append(os.Environ(), adapterEnv...)
	cmd.Stderr = stderrFile
	// Run the adapter in its own process group so the whole tree (the npx wrapper,
	// the adapter, and the agent child it launches) can be reaped together. On
	// non-Unix platforms this is a no-op (see acp_transport_other.go).
	setProcessGroup(cmd)
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

	client := &acpClient{out: stdoutWriter, activity: clientActivity, onFirstOutput: request.OnFirstOutput, repoRoot: request.RepoRoot, writePathAllowed: request.WritePathAllowed, readOnly: request.ReadOnly}
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
	completedDespiteTimeout := false
	if timedOut {
		// A recorded prompt response means the turn finished before the kill —
		// the protocol-level completion signal the text-only stdout log cannot
		// carry; the stdout detector still covers any captured terminal marker.
		exitCode, completedDespiteTimeout = finalizeTimedOutAttempt(request.Agent, "", client.turnCompleted(), stderrFile, request.LiveOutput)
	}

	usage := client.tokenUsage()
	writeUsageWarning(usage, stderrFile, request.LiveOutput)

	return RunResult{
		ExitCode:                exitCode,
		StartedAt:               started.Format(time.RFC3339Nano),
		FinishedAt:              finished.Format(time.RFC3339Nano),
		DurationMillis:          finished.Sub(started).Milliseconds(),
		TimedOut:                timedOut,
		CompletedDespiteTimeout: completedDespiteTimeout,
		StdoutPath:              layout.stdoutArtifact,
		StderrPath:              layout.stderrArtifact,
		Usage:                   usage,
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

// acpAdapterCommand maps a built-in agent name and its resolved model pin to
// the command, args, and extra environment entries that launch its ACP server
// adapter. The adapters are external npm packages run via npx; they inherit the
// process environment (and thus the agent's auth) from the parent, with the
// returned entries appended on top. The pin is threaded the way each adapter
// accepts it: codex-acp takes the same `-c` config overrides as the codex CLI
// (the model TOML-quoted, matching ApplyModelSpec); claude-agent-acp launches
// Claude Code, which honors the ANTHROPIC_MODEL and CLAUDE_CODE_EFFORT_LEVEL
// env vars for the launched session. An empty model/effort adds nothing.
//
// readOnly is enforced per leg. claude-agent-acp routes the agent's writes and
// permission requests through the ACP client, where the read-only acpClient
// denies them — no adapter flag is needed. codex applies patches natively
// in-process and consults its own approval policy (a trusted repo asks no
// permission at all), so client-side denials cannot stop it; the sandbox is
// pinned at the adapter instead, mirroring the CLI reviewer's
// `--sandbox read-only`.
func acpAdapterCommand(agentName string, spec ModelSpec, readOnly bool) (string, []string, []string, error) {
	switch agentName {
	case BuiltinClaude:
		cmd, args := acpAdapterCommandPrefix("PACTUM_CLAUDE_ACP_COMMAND", "@agentclientprotocol/claude-agent-acp@latest")
		var env []string
		if spec.Model != "" {
			env = append(env, "ANTHROPIC_MODEL="+spec.Model)
		}
		if spec.Effort != "" {
			env = append(env, "CLAUDE_CODE_EFFORT_LEVEL="+spec.Effort)
		}
		return cmd, args, env, nil
	case BuiltinCodex:
		cmd, args := acpAdapterCommandPrefix("PACTUM_CODEX_ACP_COMMAND", "@zed-industries/codex-acp@latest")
		if readOnly {
			args = append(args, "-c", `sandbox_mode="read-only"`)
		}
		if spec.Model != "" {
			args = append(args, "-c", fmt.Sprintf("model=%q", spec.Model))
		}
		if spec.Effort != "" {
			args = append(args, "-c", "model_reasoning_effort="+spec.Effort)
		}
		return cmd, args, nil, nil
	default:
		return "", nil, nil, fmt.Errorf("no ACP adapter configured for agent %q", agentName)
	}
}

func acpAdapterCommandPrefix(envName string, defaultPackage string) (string, []string) {
	if override := strings.TrimSpace(os.Getenv(envName)); override != "" {
		return override, nil
	}
	return "npx", []string{"-y", defaultPackage}
}

// acpClient implements acp.Client: it auto-approves permission requests on
// write stages (shell commands are still only enforced post-hoc by the gate),
// services the agent's file reads/writes against the working tree, streams the
// agent's text to the attempt log, and records the turn's token usage. When
// writePathAllowed is set it also enforces the contract path-scope at the
// WriteTextFile boundary; when readOnly is set it denies writes and refuses
// permission requests outright.
type acpClient struct {
	out io.Writer

	// activity is the idle-watchdog tick, invoked at the top of every inbound
	// client method: any protocol traffic from the agent (session updates of
	// every kind, permission requests, file reads/writes, terminal calls)
	// proves it is alive, even when nothing is streamed to the output. Nil
	// when no timeout is armed (and for the CLI transport, which never builds
	// an acpClient); ticking is signal-only and never writes to the log.
	activity func()

	// onFirstOutput fires once on the first non-empty agent message chunk
	// written to the log — the protocol signal that the prompt cache is now
	// warm. Nil unless the caller is staggering a same-model group.
	onFirstOutput   func()
	firstOutputOnce sync.Once

	// repoRoot and writePathAllowed implement the real-time write scope guard.
	// repoRoot anchors the conversion of an absolute ACP path to a repo-relative
	// path; writePathAllowed (nil = allow all) reports whether that path is in
	// the contract scope. See WriteTextFile.
	repoRoot         string
	writePathAllowed func(repoRelPath string) bool
	// readOnly marks a read-only stage (review, clarifier round, contract
	// draft): WriteTextFile is denied regardless of writePathAllowed, and
	// RequestPermission rejects instead of auto-approving. The write capability
	// stays advertised either way — the agent must route writes through the
	// client where they are denied, not fall back to native writes.
	readOnly bool

	mu              sync.Mutex
	promptResponded bool
	stopReason      acp.StopReason
	usage           *acp.Usage
	codexUsage      *TokenUsage

	// Message-separator state for SessionUpdate (guarded by mu): whether any
	// chunk text has been written, the messageId of the last written chunk, and
	// whether that text ended with a newline. See the boundary rule there.
	textWritten      bool
	lastMessageID    string
	textEndedNewline bool
}

var _ acp.Client = (*acpClient)(nil)

func (c *acpClient) tick() {
	if c.activity != nil {
		c.activity()
	}
}

// fireFirstOutput signals the first visible output exactly once. Streamed
// agent text is the protocol's earliest "the cache is warm" marker, so the
// staggered review fan-out releases its held attempts here.
func (c *acpClient) fireFirstOutput() {
	if c.onFirstOutput != nil {
		c.firstOutputOnce.Do(c.onFirstOutput)
	}
}

func (c *acpClient) recordResult(resp acp.PromptResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.promptResponded = true
	c.stopReason = resp.StopReason
	c.usage = resp.Usage
}

func (c *acpClient) stopReasonValue() acp.StopReason {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopReason
}

// turnCompleted reports whether a prompt response was recorded before the
// kill — the turn genuinely finished. A refusal response is a refused turn,
// not completed work, so it does not count.
func (c *acpClient) turnCompleted() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Whitelist: only an end_turn stop reason is a completed turn. Refusal,
	// cancellation, and budget stops (max tokens / max turn requests) mean the
	// work did not finish and must not finalize as completed.
	return c.promptResponded && c.stopReason == acp.StopReasonEndTurn
}

func (c *acpClient) tokenUsage() TokenUsage {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.usage == nil {
		if c.codexUsage != nil {
			return *c.codexUsage
		}
		return TokenUsage{Captured: false, CaptureWarning: "acp prompt returned no usage"}
	}
	u := c.usage
	// ACP PromptResponse.Usage reports cache/reasoning as sub-counts of the
	// input/output totals. Preserve them for the cost layer without adding them
	// into the parent counts again.
	cacheRead := derefIntToInt64(u.CachedReadTokens)
	cacheWrite := derefIntToInt64(u.CachedWriteTokens)
	reasoning := derefIntToInt64(u.ThoughtTokens)
	inputTokens := int64(u.InputTokens)
	outputTokens := int64(u.OutputTokens)
	totalTokens := maxInt64(int64(u.TotalTokens), inputTokens+outputTokens)
	return TokenUsage{
		InputTokens:         inputTokens,
		OutputTokens:        outputTokens,
		TotalTokens:         totalTokens,
		CacheReadTokens:     cacheRead,
		CacheCreationTokens: cacheWrite,
		ReasoningTokens:     reasoning,
		Captured:            true,
	}
}

func derefIntToInt64(p *int) int64 {
	if p == nil {
		return 0
	}
	return int64(*p)
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

type codexACPUsageMeta struct {
	TotalTokenUsage json.RawMessage `json:"total_token_usage"`
}

type codexACPTokenUsage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
	TotalTokens           int64 `json:"total_tokens"`
}

func parseCodexACPUsageMeta(meta map[string]any) (TokenUsage, bool) {
	if meta == nil {
		return TokenUsage{}, false
	}
	value, ok := meta["codex/token_usage"]
	if !ok {
		return TokenUsage{}, false
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return TokenUsage{}, false
	}
	var payload codexACPUsageMeta
	if err := json.Unmarshal(raw, &payload); err != nil || isEmptyRaw(payload.TotalTokenUsage) {
		return TokenUsage{}, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload.TotalTokenUsage, &fields); err != nil || len(fields) == 0 {
		return TokenUsage{}, false
	}
	for _, key := range []string{"input_tokens", "cached_input_tokens", "output_tokens", "total_tokens"} {
		if _, ok := fields[key]; !ok {
			return TokenUsage{}, false
		}
	}
	var u codexACPTokenUsage
	if err := json.Unmarshal(payload.TotalTokenUsage, &u); err != nil {
		return TokenUsage{}, false
	}
	return TokenUsage{
		InputTokens:     u.InputTokens,
		OutputTokens:    u.OutputTokens + u.ReasoningOutputTokens,
		TotalTokens:     u.TotalTokens,
		CacheReadTokens: u.CachedInputTokens,
		ReasoningTokens: u.ReasoningOutputTokens,
		Captured:        true,
		Raw:             raw,
	}, true
}

func (c *acpClient) RequestPermission(ctx context.Context, p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	c.tick()
	// On a read-only stage the agent's write/exec tool calls are refused: pick a
	// reject option when the agent offers one, otherwise cancel the request.
	// The cancelled outcome is spec-reserved for prompt-turn cancellation, but
	// both adapters map it to a per-tool denial (the turn continues), and they
	// always offer reject options in practice, so this fallback is rarely hit.
	if c.readOnly {
		for _, o := range p.Options {
			if o.Kind == acp.PermissionOptionKindRejectOnce || o.Kind == acp.PermissionOptionKindRejectAlways {
				return acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{Selected: &acp.RequestPermissionOutcomeSelected{OptionId: o.OptionId}}}, nil
			}
		}
		return acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{Cancelled: &acp.RequestPermissionOutcomeCancelled{}}}, nil
	}
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
	// Every update kind ticks the watchdog; only agent message text is written
	// to the output — tool calls, thoughts, and plans leave the log untouched.
	c.tick()
	u := p.Update
	if u.UsageUpdate != nil {
		if usage, ok := parseCodexACPUsageMeta(u.UsageUpdate.Meta); ok {
			c.mu.Lock()
			c.codexUsage = &usage
			c.mu.Unlock()
		}
	}
	if u.AgentMessageChunk != nil && u.AgentMessageChunk.Content.Text != nil {
		text := u.AgentMessageChunk.Content.Text.Text
		if text == "" {
			return nil
		}
		messageID := ""
		if u.AgentMessageChunk.MessageId != nil {
			messageID = *u.AgentMessageChunk.MessageId
		}
		c.mu.Lock()
		// Chunks of different messages are separated by a newline, so a fenced
		// block opening a later message starts on a fresh line instead of gluing
		// to the previous message's prose (which would hide it from
		// extractFencedJSONBlocks). Only an id change between stamped chunks is
		// a boundary: adapters that stamp deltas share one messageId per
		// message, while adapters that stamp nothing stream raw token deltas —
		// injecting separators between those would corrupt the text (a newline
		// inside a JSON string literal breaks the block), so id-less chunks are
		// never separated and a glued fence is handled at the parse layer
		// instead. No separator when the log already ends with a newline.
		if c.textWritten && !c.textEndedNewline && messageID != "" && messageID != c.lastMessageID {
			_, _ = io.WriteString(c.out, "\n")
		}
		_, _ = io.WriteString(c.out, text)
		c.textWritten = true
		if messageID != "" {
			c.lastMessageID = messageID
		}
		c.textEndedNewline = strings.HasSuffix(text, "\n")
		c.mu.Unlock()
		// The first non-empty chunk reached the log: the cache is warm. Fire
		// outside the lock so the callback cannot deadlock against it.
		c.fireFirstOutput()
	}
	return nil
}

func (c *acpClient) WriteTextFile(ctx context.Context, p acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	c.tick()
	if c.readOnly {
		return acp.WriteTextFileResponse{}, fmt.Errorf("acp write denied: read-only stage: %s", p.Path)
	}
	if !filepath.IsAbs(p.Path) {
		return acp.WriteTextFileResponse{}, fmt.Errorf("acp write: path must be absolute: %s", p.Path)
	}
	if err := c.checkWriteScope(p.Path); err != nil {
		return acp.WriteTextFileResponse{}, err
	}
	if err := os.MkdirAll(filepath.Dir(p.Path), 0o755); err != nil {
		return acp.WriteTextFileResponse{}, err
	}
	return acp.WriteTextFileResponse{}, os.WriteFile(p.Path, []byte(p.Content), 0o644)
}

// checkWriteScope enforces the contract path-scope at the file-write boundary.
// The absolute ACP path is converted to a repo-relative slash path against
// repoRoot; a path that escapes the repo (relative starts with "..") or that
// writePathAllowed rejects is denied — the agent receives a write failure and
// disk is not touched. A nil writePathAllowed predicate skips the scope check
// (allow all), preserving the pre-guard behavior for every existing caller and
// the CLI transport, which never builds an acpClient at all.
func (c *acpClient) checkWriteScope(absPath string) error {
	if c.writePathAllowed == nil {
		return nil
	}
	if strings.TrimSpace(c.repoRoot) == "" {
		return fmt.Errorf("acp write denied: repo root unknown for scope check: %s", absPath)
	}
	rel, err := filepath.Rel(c.repoRoot, absPath)
	if err != nil {
		return fmt.Errorf("acp write denied: cannot resolve %s against repo root: %w", absPath, err)
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return fmt.Errorf("acp write denied: path escapes repo scope: %s", absPath)
	}
	if !c.writePathAllowed(rel) {
		return fmt.Errorf("acp write denied: path out of contract scope: %s", rel)
	}
	return nil
}

func (c *acpClient) ReadTextFile(ctx context.Context, p acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	c.tick()
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
	c.tick()
	return acp.CreateTerminalResponse{}, fmt.Errorf("acp terminals are not supported")
}

func (c *acpClient) KillTerminal(ctx context.Context, p acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	c.tick()
	return acp.KillTerminalResponse{}, nil
}

func (c *acpClient) TerminalOutput(ctx context.Context, p acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	c.tick()
	return acp.TerminalOutputResponse{}, nil
}

func (c *acpClient) ReleaseTerminal(ctx context.Context, p acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	c.tick()
	return acp.ReleaseTerminalResponse{}, nil
}

func (c *acpClient) WaitForTerminalExit(ctx context.Context, p acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	c.tick()
	return acp.WaitForTerminalExitResponse{}, nil
}
