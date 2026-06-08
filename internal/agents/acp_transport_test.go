package agents

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestACPAdapterCommand(t *testing.T) {
	for _, agent := range []string{BuiltinClaude, BuiltinCodex} {
		cmd, args, err := acpAdapterCommand(agent)
		if err != nil {
			t.Fatalf("%s adapter: %v", agent, err)
		}
		if cmd != "npx" || len(args) == 0 {
			t.Fatalf("%s adapter: cmd=%q args=%v", agent, cmd, args)
		}
	}
	if _, _, err := acpAdapterCommand("nope"); err == nil {
		t.Fatal("expected error for unknown agent")
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
	u := c.tokenUsage()
	if !u.Captured || u.InputTokens != 100 || u.OutputTokens != 50 || u.TotalTokens != 150 ||
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
