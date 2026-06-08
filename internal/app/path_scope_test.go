package app

import "testing"

func TestPathGlobMatches(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{
			name:    "star matches within one segment",
			pattern: "internal/app/*.go",
			path:    "internal/app/gate.go",
			want:    true,
		},
		{
			name:    "star does not cross segment boundary",
			pattern: "internal/app/*.go",
			path:    "internal/app/sub/gate.go",
			want:    false,
		},
		{
			name:    "star can match an empty run",
			pattern: "internal/app/gate*.go",
			path:    "internal/app/gate.go",
			want:    true,
		},
		{
			name:    "double star matches nested segments",
			pattern: "internal/app/**",
			path:    "internal/app/sub/gate.go",
			want:    true,
		},
		{
			name:    "double star matches one segment",
			pattern: "internal/app/**",
			path:    "internal/app/gate.go",
			want:    true,
		},
		{
			name:    "double star matches zero segments",
			pattern: "internal/app/**",
			path:    "internal/app",
			want:    true,
		},
		{
			name:    "double star in middle matches zero segments",
			pattern: "internal/**/gate.go",
			path:    "internal/gate.go",
			want:    true,
		},
		{
			name:    "double star in middle matches multiple segments",
			pattern: "internal/**/gate.go",
			path:    "internal/app/sub/gate.go",
			want:    true,
		},
		{
			name:    "double star prefix matches root file",
			pattern: "**/*.go",
			path:    "main.go",
			want:    true,
		},
		{
			name:    "double star prefix matches nested file",
			pattern: "**/*.go",
			path:    "internal/app/gate.go",
			want:    true,
		},
		{
			name:    "segment boundary no match",
			pattern: "internal/*",
			path:    "internal/app/gate.go",
			want:    false,
		},
		{
			name:    "suffix no match",
			pattern: "docs/*.md",
			path:    "docs/flow.txt",
			want:    false,
		},
		{
			name:    "prefix no match",
			pattern: "docs/*.md",
			path:    "README.md",
			want:    false,
		},
		{
			name:    "slash normalized",
			pattern: `.\internal\app\*.go`,
			path:    `internal\app\gate.go`,
			want:    true,
		},
		{
			name:    "empty pattern no match",
			pattern: "",
			path:    "README.md",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pathGlobMatches(tt.pattern, tt.path); got != tt.want {
				t.Fatalf("pathGlobMatches(%q, %q) = %t, want %t", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

func TestPathGlobMatchesAny(t *testing.T) {
	if !pathGlobMatchesAny([]string{"docs/**", "internal/app/*.go"}, "internal/app/gate.go") {
		t.Fatalf("path should match one declared pattern")
	}
	if pathGlobMatchesAny([]string{"docs/**", "internal/app/*.go"}, "internal/app/sub/gate.go") {
		t.Fatalf("path should not match any declared pattern")
	}
}

func TestContractWritePathAllowed(t *testing.T) {
	tests := []struct {
		name        string
		inScope     []string
		outOfScope  []string
		repoRelPath string
		want        bool
	}{
		{
			name:        "in-scope allowed",
			inScope:     []string{"internal/app/**", "docs/*.md"},
			repoRelPath: "internal/app/gate.go",
			want:        true,
		},
		{
			name:        "not in declared scope denied",
			inScope:     []string{"internal/app/**"},
			repoRelPath: "internal/agents/acp_transport.go",
			want:        false,
		},
		{
			name:        "out-of-scope denied",
			inScope:     []string{"internal/**"},
			outOfScope:  []string{"internal/secret/**"},
			repoRelPath: "internal/secret/key.go",
			want:        false,
		},
		{
			name:        "escape-repo denied",
			inScope:     []string{"internal/app/**"},
			repoRelPath: "../outside.go",
			want:        false,
		},
		{
			name:        "empty in-scope allows non-out-of-scope",
			outOfScope:  []string{"docs/**"},
			repoRelPath: "internal/app/gate.go",
			want:        true,
		},
		{
			name:        "empty in-scope still denies out-of-scope",
			outOfScope:  []string{"docs/**"},
			repoRelPath: "docs/agents.md",
			want:        false,
		},
		{
			name:        "no scope declared allows everything",
			repoRelPath: "anything/at/all.go",
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed := contractWritePathAllowed(draftContract{PathsInScope: tt.inScope, PathsOutOfScope: tt.outOfScope})
			if got := allowed(tt.repoRelPath); got != tt.want {
				t.Fatalf("contractWritePathAllowed(%q) = %t, want %t", tt.repoRelPath, got, tt.want)
			}
		})
	}
}

func TestNonEmptyPathGlobsDropsDegeneratePatterns(t *testing.T) {
	// "/", "./", and whitespace normalize to nothing; keeping them would flag
	// every file as undeclared, so they must be filtered out entirely.
	got := nonEmptyPathGlobs([]string{"/", "./", "  ", "", "internal/app/**"})
	if len(got) != 1 || got[0] != "internal/app/**" {
		t.Fatalf("nonEmptyPathGlobs kept degenerate globs: %#v", got)
	}
}
