package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newSkillApp returns an App whose WorkingDir is a fresh temp directory.
func newSkillApp(t *testing.T) (App, string) {
	t.Helper()
	dir := t.TempDir()
	return App{WorkingDir: dir}, dir
}

// overrideHome sets HOME to a temp directory for the duration of the test,
// returning the temp dir and a cleanup function.
func overrideHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

// TestSkillInstallClaude verifies that --agent claude --scope repo installs the
// skill package to .claude/skills/pactum/ under the working directory.
func TestSkillInstallClaude(t *testing.T) {
	app, wd := newSkillApp(t)
	overrideHome(t)

	var out bytes.Buffer
	if err := app.SkillInstall(&out, os.Stderr, "claude", "repo", false); err != nil {
		t.Fatalf("SkillInstall claude/repo: %v", err)
	}

	dest := filepath.Join(wd, ".claude", "skills", "pactum")
	assertSkillPresent(t, dest)
}

// TestSkillInstallCodex verifies that --agent codex --scope repo installs the
// skill package to .agents/skills/pactum/ under the working directory.
func TestSkillInstallCodex(t *testing.T) {
	app, wd := newSkillApp(t)
	overrideHome(t)

	var out bytes.Buffer
	if err := app.SkillInstall(&out, os.Stderr, "codex", "repo", false); err != nil {
		t.Fatalf("SkillInstall codex/repo: %v", err)
	}

	dest := filepath.Join(wd, ".agents", "skills", "pactum")
	assertSkillPresent(t, dest)
}

// TestSkillInstallUserScope verifies that --scope user installs to $HOME/.
func TestSkillInstallUserScope(t *testing.T) {
	app, _ := newSkillApp(t)
	home := overrideHome(t)

	var out bytes.Buffer
	if err := app.SkillInstall(&out, os.Stderr, "claude", "user", false); err != nil {
		t.Fatalf("SkillInstall claude/user: %v", err)
	}

	dest := filepath.Join(home, ".claude", "skills", "pactum")
	assertSkillPresent(t, dest)
}

// TestSkillInstallAll verifies that --agent all installs to both claude and
// codex directories.
func TestSkillInstallAll(t *testing.T) {
	app, wd := newSkillApp(t)
	overrideHome(t)

	var out bytes.Buffer
	if err := app.SkillInstall(&out, os.Stderr, "all", "repo", false); err != nil {
		t.Fatalf("SkillInstall all/repo: %v", err)
	}

	assertSkillPresent(t, filepath.Join(wd, ".claude", "skills", "pactum"))
	assertSkillPresent(t, filepath.Join(wd, ".agents", "skills", "pactum"))
}

// TestSkillInstallIdempotent verifies that re-running install over an existing
// skill directory succeeds and leaves files intact.
func TestSkillInstallIdempotent(t *testing.T) {
	app, wd := newSkillApp(t)
	overrideHome(t)

	var out bytes.Buffer
	for i := 0; i < 2; i++ {
		if err := app.SkillInstall(&out, os.Stderr, "claude", "repo", false); err != nil {
			t.Fatalf("SkillInstall run %d: %v", i+1, err)
		}
	}

	dest := filepath.Join(wd, ".claude", "skills", "pactum")
	assertSkillPresent(t, dest)
}

// TestSkillInstallJSONOutput verifies the JSON response shape includes schema,
// version, targets, and next.
func TestSkillInstallJSONOutput(t *testing.T) {
	app, wd := newSkillApp(t)
	overrideHome(t)

	var out bytes.Buffer
	if err := app.SkillInstall(&out, os.Stderr, "claude", "repo", true); err != nil {
		t.Fatalf("SkillInstall --json: %v", err)
	}

	var resp skillInstallResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("parse JSON response: %v", err)
	}
	if resp.Schema != skillSchema {
		t.Errorf("schema = %q, want %q", resp.Schema, skillSchema)
	}
	if resp.Version == "" {
		t.Error("version field is empty")
	}
	if len(resp.Targets) == 0 {
		t.Error("targets is empty")
	}
	target := resp.Targets[0]
	if target.Agent != "claude" {
		t.Errorf("target.agent = %q, want claude", target.Agent)
	}
	if target.Scope != "repo" {
		t.Errorf("target.scope = %q, want repo", target.Scope)
	}
	wantPath := filepath.Join(wd, ".claude", "skills", "pactum")
	if target.Path != wantPath {
		t.Errorf("target.path = %q, want %q", target.Path, wantPath)
	}
	if len(target.Files) == 0 {
		t.Error("target.files is empty")
	}
	wantNext := "pactum skill install --check --agent claude --scope repo"
	if len(resp.Next) == 0 || resp.Next[0] != wantNext {
		t.Errorf("next[0] = %q, want %q", resp.Next[0], wantNext)
	}
}

// TestSkillCheckPresent verifies that --check reports "present" after install.
func TestSkillCheckPresent(t *testing.T) {
	app, _ := newSkillApp(t)
	overrideHome(t)

	var out bytes.Buffer
	if err := app.SkillInstall(&out, os.Stderr, "claude", "repo", false); err != nil {
		t.Fatalf("install: %v", err)
	}

	out.Reset()
	if err := app.SkillCheck(&out, os.Stderr, "claude", "repo", true); err != nil {
		t.Fatalf("SkillCheck: %v", err)
	}

	var resp skillCheckResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if len(resp.Checks) == 0 {
		t.Fatal("no checks returned")
	}
	if resp.Checks[0].Status != "present" {
		t.Errorf("status = %q, want present", resp.Checks[0].Status)
	}
}

// TestSkillCheckMissing verifies that --check reports "missing" when the skill
// has not been installed.
func TestSkillCheckMissing(t *testing.T) {
	app, _ := newSkillApp(t)
	overrideHome(t)

	var out bytes.Buffer
	if err := app.SkillCheck(&out, os.Stderr, "claude", "repo", true); err != nil {
		t.Fatalf("SkillCheck: %v", err)
	}

	var resp skillCheckResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if len(resp.Checks) == 0 {
		t.Fatal("no checks returned")
	}
	if resp.Checks[0].Status != "missing" {
		t.Errorf("status = %q, want missing", resp.Checks[0].Status)
	}
	// Missing status should produce a remedial next command.
	if len(resp.Next) == 0 {
		t.Error("expected next to contain a remedial install command")
	}
}

// TestSkillCheckInvalidFrontmatter verifies that --check reports "invalid" when
// SKILL.md exists but does not have valid YAML frontmatter.
func TestSkillCheckInvalidFrontmatter(t *testing.T) {
	app, wd := newSkillApp(t)
	overrideHome(t)

	// Write a SKILL.md with no frontmatter.
	dest := filepath.Join(wd, ".claude", "skills", "pactum")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte("no frontmatter here"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := app.SkillCheck(&out, os.Stderr, "claude", "repo", true); err != nil {
		t.Fatalf("SkillCheck: %v", err)
	}

	var resp skillCheckResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if len(resp.Checks) == 0 {
		t.Fatal("no checks returned")
	}
	if resp.Checks[0].Status != "invalid" {
		t.Errorf("status = %q, want invalid", resp.Checks[0].Status)
	}
}

// TestSkillCheckIncomplete verifies that --check reports "invalid" when
// SKILL.md has valid frontmatter but other package files are missing.
func TestSkillCheckIncomplete(t *testing.T) {
	app, wd := newSkillApp(t)
	overrideHome(t)

	dest := filepath.Join(wd, ".claude", "skills", "pactum")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write only SKILL.md; omit references/ and any other embedded files.
	skillMD := "---\nname: pactum\ndescription: Test skill\n---\n# Pactum\n"
	if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := app.SkillCheck(&out, os.Stderr, "claude", "repo", true); err != nil {
		t.Fatalf("SkillCheck: %v", err)
	}
	var resp skillCheckResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if len(resp.Checks) == 0 {
		t.Fatal("no checks returned")
	}
	if resp.Checks[0].Status != "invalid" {
		t.Errorf("status = %q, want invalid (incomplete package)", resp.Checks[0].Status)
	}
}

// TestSkillAutoDetectNone verifies that --agent auto returns skill_no_targets
// when neither agent CLI nor dot-directory is present.
func TestSkillAutoDetectNone(t *testing.T) {
	app, _ := newSkillApp(t)
	overrideHome(t)
	// Isolate PATH to an empty temp dir so no agent CLI is found there.
	t.Setenv("PATH", t.TempDir())

	var out bytes.Buffer
	err := app.SkillInstall(&out, os.Stderr, "auto", "repo", false)
	if err == nil {
		t.Fatal("expected precondition error when no agent is detected")
	}
	var pre *preconditionError
	if !isPreconditionError(err, &pre) {
		t.Fatalf("expected preconditionError, got %T: %v", err, err)
	}
	if pre.code != "skill_no_targets" {
		t.Errorf("error.code = %q, want skill_no_targets", pre.code)
	}
}

// TestSkillInstallSKILLMDFrontmatter verifies the installed SKILL.md has valid
// YAML frontmatter with name: pactum.
func TestSkillInstallSKILLMDFrontmatter(t *testing.T) {
	app, wd := newSkillApp(t)
	overrideHome(t)

	var out bytes.Buffer
	if err := app.SkillInstall(&out, os.Stderr, "claude", "repo", false); err != nil {
		t.Fatalf("install: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(wd, ".claude", "skills", "pactum", "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		t.Error("installed SKILL.md does not start with YAML frontmatter")
	}
	if !strings.Contains(content, "name: pactum") {
		t.Error("installed SKILL.md frontmatter missing 'name: pactum'")
	}
}

// --- helpers ---

func assertSkillPresent(t *testing.T, destDir string) {
	t.Helper()
	skillMD := filepath.Join(destDir, "SKILL.md")
	if _, err := os.Stat(skillMD); err != nil {
		t.Errorf("expected SKILL.md at %s: %v", skillMD, err)
	}
	refDir := filepath.Join(destDir, "references")
	if _, err := os.Stat(refDir); err != nil {
		t.Errorf("expected references/ dir at %s: %v", refDir, err)
	}
}

func isPreconditionError(err error, out **preconditionError) bool {
	if err == nil {
		return false
	}
	if p, ok := err.(*preconditionError); ok {
		if out != nil {
			*out = p
		}
		return true
	}
	return false
}
