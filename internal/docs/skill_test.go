package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// skillPackageDir is the repo-local agent skill package. readRepoFile and
// repoRoot are shared helpers defined alongside the other docs tests.
const skillPackageDir = "assets/agent-skills/pactum"

func assertRepoFileExists(t *testing.T, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repoRoot(t), filepath.FromSlash(rel))); err != nil {
		t.Fatalf("expected %s to exist: %v", rel, err)
	}
}

func assertRepoPathAbsent(t *testing.T, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repoRoot(t), filepath.FromSlash(rel))); !os.IsNotExist(err) {
		t.Fatalf("%s should not exist yet (deferred), err=%v", rel, err)
	}
}

// TestSkillPackageExists checks the portable skill package layout.
func TestSkillPackageExists(t *testing.T) {
	for _, rel := range []string{
		skillPackageDir + "/SKILL.md",
		skillPackageDir + "/references/workflow.md",
		skillPackageDir + "/references/install.md",
		skillPackageDir + "/references/safety.md",
	} {
		assertRepoFileExists(t, rel)
	}
}

// TestSkillFrontmatterIsPortable checks the canonical frontmatter is the
// portable intersection (name + description) — no Claude-only keys.
func TestSkillFrontmatterIsPortable(t *testing.T) {
	skill := readRepoFile(t, skillPackageDir+"/SKILL.md")
	if !strings.HasPrefix(skill, "---\n") {
		t.Fatalf("SKILL.md must start with YAML frontmatter:\n%s", firstLines(skill, 5))
	}
	end := strings.Index(skill[4:], "\n---")
	if end < 0 {
		t.Fatalf("SKILL.md frontmatter is not closed:\n%s", firstLines(skill, 10))
	}
	frontmatter := skill[:end+4]
	for _, want := range []string{"name: pactum", "description:"} {
		if !strings.Contains(frontmatter, want) {
			t.Fatalf("SKILL.md frontmatter missing %q:\n%s", want, frontmatter)
		}
	}
	// allowed-tools is Claude-specific; the portable skill must not bake it in.
	if strings.Contains(frontmatter, "allowed-tools") {
		t.Fatalf("portable SKILL.md frontmatter must not include Claude-specific allowed-tools:\n%s", frontmatter)
	}
}

// TestSkillInlinesSafetyAndStop checks the mandatory safety rules are inline in
// SKILL.md (not only behind references), so the skill is self-sufficient.
func TestSkillInlinesSafetyAndStop(t *testing.T) {
	skill := readRepoFile(t, skillPackageDir+"/SKILL.md")
	for _, want := range []string{
		"which pactum",
		"pactum task new",
		"pactum execute plan",
		"pactum execute run",     // mentioned as the thing NOT to run by default
		"references/workflow.md", // explicitly tells the agent to read references
		".heurema",
		"source of truth",
	} {
		if !strings.Contains(skill, want) {
			t.Fatalf("SKILL.md missing %q:\n%s", want, skill)
		}
	}
	if !strings.Contains(skill, "Do not run `pactum execute run`") {
		t.Fatalf("SKILL.md must state execute run is not default:\n%s", skill)
	}
}

// TestSkillWorkflowReference checks the full workflow lives in references.
func TestSkillWorkflowReference(t *testing.T) {
	workflow := readRepoFile(t, skillPackageDir+"/references/workflow.md")
	for _, want := range []string{
		"pactum init",
		"pactum map refresh",
		"pactum task new",
		"pactum search",
		"pactum contract revise",
		"pactum contract approve --by manual",
		"pactum prompt build",
		"pactum execute plan",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("workflow.md missing %q", want)
		}
	}
}

// TestSkillInstallReference checks install guidance is honest and complete.
func TestSkillInstallReference(t *testing.T) {
	install := readRepoFile(t, skillPackageDir+"/references/install.md")
	for _, want := range []string{
		".agents/skills/pactum/",
		".claude/skills/pactum/",
		"go install ./cmd/pactum",
		"make build",
	} {
		if !strings.Contains(install, want) {
			t.Fatalf("install.md missing %q", want)
		}
	}
	for _, planned := range []string{"npm", "uvx"} {
		if !strings.Contains(install, planned) {
			t.Fatalf("install.md should note %q as planned/considered", planned)
		}
	}
}

// TestSkillSafetyReference checks the execution-safety rules.
func TestSkillSafetyReference(t *testing.T) {
	safety := readRepoFile(t, skillPackageDir+"/references/safety.md")
	for _, want := range []string{
		"pactum execute run",
		"pactum review run",
		"explicit",
		"not sandboxed",
		".heurema",
	} {
		if !strings.Contains(safety, want) {
			t.Fatalf("safety.md missing %q", want)
		}
	}
}

// TestAgentsMdPointsToSkill checks the committed, repo-facing AGENTS.md points
// at the skill package and carries the core safety rules. Operator-specific
// notes are kept in a separate local-only file, so they are not asserted here.
func TestAgentsMdPointsToSkill(t *testing.T) {
	agents := readRepoFile(t, "AGENTS.md")
	if !strings.Contains(agents, "assets/agent-skills/pactum") {
		t.Fatalf("AGENTS.md should point to the skill package:\n%s", agents)
	}
	if !strings.Contains(agents, ".heurema") {
		t.Fatalf("AGENTS.md should warn against committing .heurema:\n%s", agents)
	}
	if !strings.Contains(agents, "make check") {
		t.Fatalf("AGENTS.md should require make check before reporting changes:\n%s", agents)
	}
}

// TestReadmeLinksSkillDocs checks the README documentation section links the
// new docs.
func TestReadmeLinksSkillDocs(t *testing.T) {
	readme := readRepoFile(t, "README.md")
	for _, want := range []string{"docs/agent-skill.md", "docs/skill-install.md", "assets/agent-skills/pactum"} {
		if !strings.Contains(readme, want) {
			t.Fatalf("README.md should reference %q", want)
		}
	}
}

// TestSkillDocsAvoidStaleAndPrematureClaims guards against stale commands and
// against claiming deferred (marketplace / self-install) features exist.
func TestSkillDocsAvoidStaleAndPrematureClaims(t *testing.T) {
	files := []string{
		skillPackageDir + "/SKILL.md",
		skillPackageDir + "/references/workflow.md",
		skillPackageDir + "/references/install.md",
		skillPackageDir + "/references/safety.md",
		"docs/agent-skill.md",
		"docs/skill-install.md",
		"AGENTS.md",
	}
	forbidden := []string{
		`pactum run "`,
		"--contract-only",
		"--allow-execute",
		"--mode yolo",
		"agents.adapters",
		"map v2",
		"pactum.map.v2",
		// deferred features must not be presented as available
		"/plugin marketplace add heurema/pactum",
		"/plugin install pactum@pactum",
		"pactum install",
		"pactum skill install",
		// M23.0 removed these command spellings.
		"pactum agents doctor",
		"pactum clarify ask",
		"pactum clarify loop",
		"pactum clarify status",
		"pactum clarify list",
		"pactum execute dry-run",
		"pactum execute status",
		"pactum review dry-run",
		"review add-finding",
		"pactum review resolve",
		"review propose-findings",
		"review accept-proposal",
		"review reject-proposal",
		"review apply-fix-outcomes",
		"contract show-draft",
		"contract accept-draft",
		"pactum task current",
	}
	for _, rel := range files {
		content := readRepoFile(t, rel)
		for _, phrase := range forbidden {
			if strings.Contains(content, phrase) {
				t.Fatalf("%s must not mention %q (stale or deferred)", rel, phrase)
			}
		}
	}
}

// TestSkillPackagingDeferred asserts marketplace/auto-discovery artifacts are
// not introduced yet.
func TestSkillPackagingDeferred(t *testing.T) {
	for _, rel := range []string{
		".claude-plugin/plugin.json",
		".claude-plugin/marketplace.json",
		".agents/skills/pactum/SKILL.md",
		".claude/skills/pactum/SKILL.md",
		skillPackageDir + "/codex/AGENTS.md",
		skillPackageDir + "/codex/AGENTS.example.md",
	} {
		assertRepoPathAbsent(t, rel)
	}
}

func TestSkillDocsMentionBothAgents(t *testing.T) {
	doc := readRepoFile(t, "docs/agent-skill.md")
	for _, want := range []string{"Codex", "Claude"} {
		if !strings.Contains(doc, want) {
			t.Fatalf("docs/agent-skill.md should mention %q", want)
		}
	}
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
