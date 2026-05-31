package codeindex

import (
	"path/filepath"
	"strings"
)

func LanguageForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".jsx":
		return "jsx"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".cs":
		return "csharp"
	default:
		return ""
	}
}

func IsSupported(language string) bool {
	for _, supported := range SupportedLanguages() {
		if language == supported {
			return true
		}
	}
	return false
}
