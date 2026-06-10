package app

import (
	"os"
	"testing"

	"github.com/heurema/pactum/internal/agents"
)

// transportName names the concrete transport agentTransport selected, so the
// table below reads as env value -> transport.
func transportName(t *testing.T, transport agents.Transport) string {
	t.Helper()
	switch transport.(type) {
	case agents.ACPTransport:
		return "acp"
	case agents.CLITransport:
		return "cli"
	default:
		t.Fatalf("unexpected transport type %T", transport)
		return ""
	}
}

func TestAgentTransportSelection(t *testing.T) {
	tests := []struct {
		name  string
		env   string
		unset bool
		want  string
	}{
		{name: "unset env selects the ACP default", unset: true, want: "acp"},
		{name: "empty env selects the ACP default", env: "", want: "acp"},
		{name: "acp selects ACP", env: "acp", want: "acp"},
		{name: "cli selects the CLI escape hatch", env: "cli", want: "cli"},
		{name: "cli is trimmed and case-insensitive", env: "  CLI  ", want: "cli"},
		{name: "unknown value falls back to the ACP default", env: "grpc", want: "acp"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("PACTUM_AGENT_TRANSPORT", test.env)
			if test.unset {
				os.Unsetenv("PACTUM_AGENT_TRANSPORT")
			}
			if got := transportName(t, App{}.agentTransport()); got != test.want {
				t.Fatalf("agentTransport with PACTUM_AGENT_TRANSPORT=%q = %s, want %s", test.env, got, test.want)
			}
		})
	}
}

func TestAgentTransportInjectionWinsOverEnv(t *testing.T) {
	t.Setenv("PACTUM_AGENT_TRANSPORT", "acp")
	injected := &recordingAgentTransport{}
	app := App{AgentTransport: injected}
	if got := app.agentTransport(); got != agents.Transport(injected) {
		t.Fatalf("agentTransport = %T, want the injected transport", got)
	}
}
