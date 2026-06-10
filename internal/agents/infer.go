package agents

import (
	"strings"
)

// claudeModelAliases are the bare model names the claude CLI accepts in place
// of a full model id; they carry no vendor prefix, so inference matches them
// exactly.
var claudeModelAliases = map[string]bool{
	"opus":   true,
	"sonnet": true,
	"haiku":  true,
	"fable":  true,
}

// InferAgentFromModel maps a model identifier to the built-in agent that runs
// it. The two vendors' model families are prefix-distinct, so the model alone
// determines the engine: a model starting with "claude" or equal to a claude
// alias (opus, sonnet, haiku, fable) runs on claude; a model starting with
// "gpt" or "codex", or with "o" followed by a digit (o3, o4-mini, ...), runs
// on codex. Matching is case-insensitive on the trimmed model. The second
// return value is false when the model matches no known form.
func InferAgentFromModel(model string) (string, bool) {
	model = strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(model, "claude"), claudeModelAliases[model]:
		return BuiltinClaude, true
	case strings.HasPrefix(model, "gpt"), strings.HasPrefix(model, "codex"):
		return BuiltinCodex, true
	case len(model) >= 2 && model[0] == 'o' && model[1] >= '0' && model[1] <= '9':
		return BuiltinCodex, true
	default:
		return "", false
	}
}
