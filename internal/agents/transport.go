package agents

// Transport runs a prepared agent attempt and returns its result. The default
// CLITransport runs the agent as a one-shot CLI subprocess (the codex/claude
// builtins); an alternative transport (e.g. an Agent Client Protocol client) can
// drive the same RunRequest a different way while returning the same RunResult,
// so the attempt lifecycle stays unaware of how the agent is reached.
type Transport interface {
	Run(request RunRequest) (RunResult, error)
}

// CLITransport is the default Transport: it runs the agent CLI as a one-shot
// subprocess via RunSubprocess.
type CLITransport struct{}

func (CLITransport) Run(request RunRequest) (RunResult, error) {
	return RunSubprocess(request)
}
