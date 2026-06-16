package agents

// Transport runs a prepared agent attempt and returns its result. The
// ACPTransport drives built-in agents over the Agent Client Protocol; callers
// may inject an alternative Transport for testing.
type Transport interface {
	Run(request RunRequest) (RunResult, error)
}
