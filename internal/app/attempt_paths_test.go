package app

// Per-command attempt-path locators used only by tests. Production code resolves
// these inline through the shared agent-attempt lifecycle (see agent_attempt.go),
// so these wrappers live in test scope rather than the production build.
// `executionAttemptPaths` is the exception — it has real production callers in
// execute reporting and the gate, so it stays in execute.go.

func clarifierAttemptPaths(runPaths contractRunPathSet, attemptID string) attemptPathSet {
	return agentAttemptPaths(runPaths.ClarifierAttemptsDir, attemptID)
}

func contractDrafterAttemptPaths(runPaths contractRunPathSet, attemptID string) attemptPathSet {
	return agentAttemptPaths(runPaths.ContractDrafterAttemptsDir, attemptID)
}

func contractReviewerAttemptPaths(runPaths contractRunPathSet, attemptID string) attemptPathSet {
	return agentAttemptPaths(runPaths.ContractReviewAttemptsDir, attemptID)
}

func reviewFixAttemptPaths(runPaths contractRunPathSet, attemptID string) attemptPathSet {
	return agentAttemptPaths(runPaths.ReviewFixAttemptsDir, attemptID)
}
