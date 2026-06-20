package app

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/heurema/pactum/assets"
	"github.com/heurema/pactum/internal/version"
	"gopkg.in/yaml.v3"
)

// skillFSRoot is the path prefix inside assets.SkillFS for the pactum skill.
const skillFSRoot = "agent-skills/pactum"

// skillSchema is the JSON schema identifier for skill install/check responses.
const skillSchema = "pactum.skill.v1alpha1"

// --- response types ---

type skillInstallResponse struct {
	Schema  string               `json:"schema"`
	Version string               `json:"version"`
	Targets []skillInstallTarget `json:"targets"`
	Next    []string             `json:"next,omitempty"`
}

type skillInstallTarget struct {
	Agent string   `json:"agent"`
	Scope string   `json:"scope"`
	Path  string   `json:"path"`
	Files []string `json:"files"`
}

type skillCheckResponse struct {
	Schema  string        `json:"schema"`
	Version string        `json:"version"`
	Checks  []skillResult `json:"checks"`
	Next    []string      `json:"next,omitempty"`
}

type skillResult struct {
	Agent  string `json:"agent"`
	Scope  string `json:"scope"`
	Path   string `json:"path"`
	Status string `json:"status"` // "present", "missing", "invalid"
	Detail string `json:"detail,omitempty"`
}

// --- SkillInstall ---

// SkillInstall writes the embedded skill package to the appropriate agent skill
// directory. It resolves target paths from --agent and --scope, performs an
// idempotent overwrite, and prints a result with the installed paths and reload
// guidance.
func (a App) SkillInstall(stdout, stderr io.Writer, agent, scope string, jsonOutput bool) error {
	targets, err := resolveSkillTargets(agent, scope, a.WorkingDir)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return &preconditionError{
			msg:  "no agent targets detected; use --agent claude, --agent codex, or --agent all",
			code: "skill_no_targets",
			fix:  "pactum skill install --agent all",
		}
	}

	var installed []skillInstallTarget
	for _, t := range targets {
		files, err := installSkillPackage(t.destDir)
		if err != nil {
			return fmt.Errorf("install to %s: %w", t.destDir, err)
		}
		installed = append(installed, skillInstallTarget{
			Agent: t.agentName,
			Scope: t.scopeName,
			Path:  t.destDir,
			Files: files,
		})
	}

	if jsonOutput {
		resp := skillInstallResponse{
			Schema:  skillSchema,
			Version: version.Current().Version,
			Targets: installed,
			Next:    []string{fmt.Sprintf("pactum skill install --check --agent %s --scope %s", agent, scope)},
		}
		return writeJSONResponse(stdout, resp)
	}

	ver := version.Current().Version
	for _, tgt := range installed {
		fmt.Fprintf(stdout, "Installed pactum skill (%s)\n", ver)
		fmt.Fprintf(stdout, "  agent: %s\n", tgt.Agent)
		fmt.Fprintf(stdout, "  scope: %s\n", tgt.Scope)
		fmt.Fprintf(stdout, "  path:  %s\n", tgt.Path)
		fmt.Fprintf(stdout, "  files: %d\n", len(tgt.Files))
		fmt.Fprintln(stdout)
	}
	fmt.Fprintln(stdout, "Reload or restart your coding agent if the skill does not appear.")
	return nil
}

// --- SkillCheck ---

// SkillCheck verifies that the skill package is installed at the expected path
// for the selected agent/scope and that SKILL.md has valid YAML frontmatter.
// It does not write any files.
func (a App) SkillCheck(stdout, stderr io.Writer, agent, scope string, jsonOutput bool) error {
	targets, err := resolveSkillTargets(agent, scope, a.WorkingDir)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return &preconditionError{
			msg:  "no agent targets detected; use --agent claude, --agent codex, or --agent all",
			code: "skill_no_targets",
			fix:  "pactum skill install --agent all",
		}
	}

	var results []skillResult
	for _, t := range targets {
		r := checkSkillDir(t)
		results = append(results, r)
	}

	if jsonOutput {
		var next []string
		for _, r := range results {
			if r.Status != "present" {
				next = append(next, fmt.Sprintf("pactum skill install --agent %s --scope %s", r.Agent, r.Scope))
				break
			}
		}
		resp := skillCheckResponse{
			Schema:  skillSchema,
			Version: version.Current().Version,
			Checks:  results,
			Next:    next,
		}
		return writeJSONResponse(stdout, resp)
	}

	ver := version.Current().Version
	fmt.Fprintf(stdout, "pactum skill check (%s)\n", ver)
	for _, r := range results {
		icon := "✓"
		if r.Status != "present" {
			icon = "✗"
		}
		fmt.Fprintf(stdout, "%s  agent=%s scope=%s  path=%s  status=%s\n", icon, r.Agent, r.Scope, r.Path, r.Status)
		if r.Detail != "" {
			fmt.Fprintf(stdout, "   %s\n", r.Detail)
		}
	}
	fmt.Fprintln(stdout, "Reload or restart your coding agent if the skill does not appear.")
	return nil
}

// --- path resolution ---

type skillTarget struct {
	agentName string // "claude" or "codex"
	scopeName string // "user" or "repo"
	destDir   string // absolute path
}

// resolveSkillTargets maps (agent, scope) to install targets. "auto" detects
// which agents are present; "all" returns every known alpha target.
func resolveSkillTargets(agent, scope, workingDir string) ([]skillTarget, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}

	switch agent {
	case "claude":
		return []skillTarget{resolveTarget("claude", scope, home, workingDir)}, nil
	case "codex":
		return []skillTarget{resolveTarget("codex", scope, home, workingDir)}, nil
	case "all":
		return []skillTarget{
			resolveTarget("claude", scope, home, workingDir),
			resolveTarget("codex", scope, home, workingDir),
		}, nil
	case "auto":
		var targets []skillTarget
		if detectAgent("claude", home, workingDir) {
			targets = append(targets, resolveTarget("claude", scope, home, workingDir))
		}
		if detectAgent("codex", home, workingDir) {
			targets = append(targets, resolveTarget("codex", scope, home, workingDir))
		}
		return targets, nil
	default:
		return nil, fmt.Errorf("unknown agent %q; use claude, codex, auto, or all", agent)
	}
}

// resolveTarget computes the destination directory for a single (agent, scope)
// pair.
//
// Skill directory conventions (alpha):
//   - claude, repo  → <workingDir>/.claude/skills/pactum/
//   - claude, user  → $HOME/.claude/skills/pactum/
//   - codex, repo   → <workingDir>/.agents/skills/pactum/
//   - codex, user   → $HOME/.agents/skills/pactum/
func resolveTarget(agentName, scope, home, workingDir string) skillTarget {
	dir := skillDir(agentName, scope, home, workingDir)
	return skillTarget{agentName: agentName, scopeName: scope, destDir: dir}
}

func skillDir(agentName, scope, home, workingDir string) string {
	var base, dotDir string
	switch agentName {
	case "claude":
		dotDir = ".claude"
	default: // codex
		dotDir = ".agents"
	}
	if scope == "user" {
		base = home
	} else {
		base = workingDir
	}
	return filepath.Join(base, dotDir, "skills", "pactum")
}

// detectAgent reports whether an agent appears to be present on this machine.
// It checks the CLI name on PATH and the agent's top-level dot-directory in
// either the user home or working directory. It never starts a real agent process.
func detectAgent(agentName, home, workingDir string) bool {
	cliName := agentName
	if _, err := exec.LookPath(cliName); err == nil {
		return true
	}
	var dotDir string
	switch agentName {
	case "claude":
		dotDir = ".claude"
	default:
		dotDir = ".agents"
	}
	for _, base := range []string{home, workingDir} {
		if isDir(filepath.Join(base, dotDir)) {
			return true
		}
	}
	return false
}

// --- filesystem operations ---

// embeddedSkillFiles returns the relative file paths in the embedded skill package.
func embeddedSkillFiles() ([]string, error) {
	var paths []string
	err := fs.WalkDir(assets.SkillFS, skillFSRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		paths = append(paths, strings.TrimPrefix(path, skillFSRoot+"/"))
		return nil
	})
	return paths, err
}

// installSkillPackage copies all files from the embedded skill FS into destDir,
// creating it if it does not exist. Existing files are overwritten. Returns the
// list of relative file paths written.
func installSkillPackage(destDir string) ([]string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("create skill directory: %w", err)
	}

	var written []string
	err := fs.WalkDir(assets.SkillFS, skillFSRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Strip the skillFSRoot prefix to get the relative path within the package.
		rel := strings.TrimPrefix(path, skillFSRoot+"/")
		if d.IsDir() {
			if path == skillFSRoot {
				return nil
			}
			return os.MkdirAll(filepath.Join(destDir, rel), 0o755)
		}
		data, err := assets.SkillFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", path, err)
		}
		dest := filepath.Join(destDir, rel)
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}
		written = append(written, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return written, nil
}

// checkSkillDir verifies that the skill package is present and SKILL.md has
// valid YAML frontmatter. It does not write any files.
func checkSkillDir(t skillTarget) skillResult {
	r := skillResult{Agent: t.agentName, Scope: t.scopeName, Path: t.destDir}

	skillMD := filepath.Join(t.destDir, "SKILL.md")
	data, err := os.ReadFile(skillMD)
	if err != nil {
		r.Status = "missing"
		r.Detail = fmt.Sprintf("SKILL.md not found at %s", skillMD)
		return r
	}
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		r.Status = "invalid"
		r.Detail = "SKILL.md does not start with YAML frontmatter"
		return r
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		r.Status = "invalid"
		r.Detail = "SKILL.md frontmatter is not closed"
		return r
	}
	fmBlock := content[:end+4]
	var parsedFM struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal([]byte(fmBlock), &parsedFM); err != nil {
		r.Status = "invalid"
		r.Detail = fmt.Sprintf("SKILL.md frontmatter is not valid YAML: %v", err)
		return r
	}
	if parsedFM.Name != "pactum" {
		r.Status = "invalid"
		r.Detail = fmt.Sprintf("SKILL.md frontmatter name = %q, want pactum", parsedFM.Name)
		return r
	}

	expectedFiles, err := embeddedSkillFiles()
	if err != nil {
		r.Status = "invalid"
		r.Detail = fmt.Sprintf("failed to read embedded skill package: %v", err)
		return r
	}
	var missing []string
	for _, rel := range expectedFiles {
		if _, err := os.Stat(filepath.Join(t.destDir, rel)); os.IsNotExist(err) {
			missing = append(missing, rel)
		}
	}
	if len(missing) > 0 {
		r.Status = "invalid"
		r.Detail = fmt.Sprintf("skill package is incomplete, missing: %s", strings.Join(missing, ", "))
		return r
	}

	r.Status = "present"
	return r
}
