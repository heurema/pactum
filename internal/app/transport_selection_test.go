package app

import (
	"testing"

	"github.com/heurema/pactum/internal/agents"
)

func TestAgentTransportDefaultIsACP(t *testing.T) {
	if _, ok := (App{}.agentTransport()).(agents.ACPTransport); !ok {
		t.Fatal("default agentTransport should be ACPTransport")
	}
}

func TestAgentTransportInjectionWinsOverDefault(t *testing.T) {
	injected := &recordingAgentTransport{}
	app := App{AgentTransport: injected}
	if got := app.agentTransport(); got != agents.Transport(injected) {
		t.Fatalf("agentTransport = %T, want the injected transport", got)
	}
}
