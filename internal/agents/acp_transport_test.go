package agents

import (
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
