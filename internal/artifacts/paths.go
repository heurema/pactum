package artifacts

import "path/filepath"

const (
	ToolName     = "pactum"
	WorkspaceRel = ".heurema/pactum"
)

type Paths struct {
	Root      string
	Workspace string

	Manifest  string
	Config    string
	Gitignore string

	RunsDir string

	MemoryDir        string
	ProjectMemory    string
	MemoryItems      string
	MemoryRefreshes  string
	FeaturesDir      string
	DecisionsDir     string
	PatternsDir      string
	ReviewLessonsDir string
	PreferencesDir   string
	SkillsDir        string
	AnchorsDir       string
	StaleReport      string

	LedgerDir   string
	EventsJSONL string
	UsageJSONL  string
	CostJSON    string

	CacheDir string
	TmpDir   string
}

func New(root string) Paths {
	workspace := filepath.Join(root, WorkspaceRel)
	memoryDir := filepath.Join(workspace, "memory")
	ledgerDir := filepath.Join(workspace, "ledger")

	return Paths{
		Root:      root,
		Workspace: workspace,

		Manifest:  filepath.Join(workspace, "manifest.json"),
		Config:    filepath.Join(workspace, "config.yaml"),
		Gitignore: filepath.Join(workspace, ".gitignore"),

		RunsDir: filepath.Join(workspace, "runs"),

		MemoryDir:        memoryDir,
		ProjectMemory:    filepath.Join(memoryDir, "project-memory.md"),
		MemoryItems:      filepath.Join(memoryDir, "items.jsonl"),
		MemoryRefreshes:  filepath.Join(memoryDir, "refreshes.jsonl"),
		FeaturesDir:      filepath.Join(memoryDir, "features"),
		DecisionsDir:     filepath.Join(memoryDir, "decisions"),
		PatternsDir:      filepath.Join(memoryDir, "patterns"),
		ReviewLessonsDir: filepath.Join(memoryDir, "review-lessons"),
		PreferencesDir:   filepath.Join(memoryDir, "preferences"),
		SkillsDir:        filepath.Join(memoryDir, "skills"),
		AnchorsDir:       filepath.Join(memoryDir, "anchors"),
		StaleReport:      filepath.Join(memoryDir, "stale-report.json"),

		LedgerDir:   ledgerDir,
		EventsJSONL: filepath.Join(ledgerDir, "events.jsonl"),
		UsageJSONL:  filepath.Join(ledgerDir, "usage.jsonl"),
		CostJSON:    filepath.Join(ledgerDir, "cost.json"),

		CacheDir: filepath.Join(workspace, "cache"),
		TmpDir:   filepath.Join(workspace, "tmp"),
	}
}

func (p Paths) Dirs() []string {
	return []string{
		p.Workspace,
		p.RunsDir,
		p.MemoryDir,
		p.FeaturesDir,
		p.DecisionsDir,
		p.PatternsDir,
		p.ReviewLessonsDir,
		p.PreferencesDir,
		p.SkillsDir,
		p.AnchorsDir,
		p.LedgerDir,
		p.CacheDir,
		p.TmpDir,
	}
}
