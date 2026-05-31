package artifacts

import "path/filepath"

const (
	ToolName     = "pactum"
	ToolVersion  = "0.1.0"
	WorkspaceRel = ".heurema/pactum"
)

type Paths struct {
	Root      string
	Workspace string

	Manifest  string
	Config    string
	Gitignore string

	MapDir         string
	MapManifest    string
	LLMS           string
	RepoMap        string
	AreasDir       string
	AreaIndex      string
	FilesJSONL     string
	CodeItemsJSONL string
	HashesJSONL    string
	MapRunsDir     string

	RunsDir string

	MemoryDir        string
	ProjectMemory    string
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
	mapDir := filepath.Join(workspace, "map")
	memoryDir := filepath.Join(workspace, "memory")
	ledgerDir := filepath.Join(workspace, "ledger")

	return Paths{
		Root:      root,
		Workspace: workspace,

		Manifest:  filepath.Join(workspace, "manifest.json"),
		Config:    filepath.Join(workspace, "config.yaml"),
		Gitignore: filepath.Join(workspace, ".gitignore"),

		MapDir:         mapDir,
		MapManifest:    filepath.Join(mapDir, "manifest.json"),
		LLMS:           filepath.Join(mapDir, "llms.txt"),
		RepoMap:        filepath.Join(mapDir, "repo-map.md"),
		AreasDir:       filepath.Join(mapDir, "areas"),
		AreaIndex:      filepath.Join(mapDir, "areas", "_index.md"),
		FilesJSONL:     filepath.Join(mapDir, "files.jsonl"),
		CodeItemsJSONL: filepath.Join(mapDir, "code-items.jsonl"),
		HashesJSONL:    filepath.Join(mapDir, "hashes.jsonl"),
		MapRunsDir:     filepath.Join(mapDir, "runs"),

		RunsDir: filepath.Join(workspace, "runs"),

		MemoryDir:        memoryDir,
		ProjectMemory:    filepath.Join(memoryDir, "project-memory.md"),
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
		p.MapDir,
		p.AreasDir,
		p.MapRunsDir,
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
