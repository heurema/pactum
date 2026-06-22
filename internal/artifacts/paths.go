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

	MapDir       string
	MapManifest  string
	LLMS         string
	RepoMap      string
	AreasDir     string
	AreaIndex    string
	FilesJSONL   string
	HashesJSONL  string
	SearchSQLite string
	MapRunsDir   string

	WikiDir         string
	WikiAreasDir    string
	WikiOverview    string
	WikiStructure   string
	WikiCommands    string
	WikiEntrypoints string
	WikiConfig      string
	WikiTests       string

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
	mapDir := filepath.Join(workspace, "map")
	memoryDir := filepath.Join(workspace, "memory")
	ledgerDir := filepath.Join(workspace, "ledger")

	return Paths{
		Root:      root,
		Workspace: workspace,

		Manifest:  filepath.Join(workspace, "manifest.json"),
		Config:    filepath.Join(workspace, "config.yaml"),
		Gitignore: filepath.Join(workspace, ".gitignore"),

		MapDir:       mapDir,
		MapManifest:  filepath.Join(mapDir, "manifest.json"),
		LLMS:         filepath.Join(mapDir, "llms.txt"),
		RepoMap:      filepath.Join(mapDir, "repo-map.md"),
		AreasDir:     filepath.Join(mapDir, "areas"),
		AreaIndex:    filepath.Join(mapDir, "areas", "_index.md"),
		FilesJSONL:   filepath.Join(mapDir, "files.jsonl"),
		HashesJSONL:  filepath.Join(mapDir, "hashes.jsonl"),
		SearchSQLite: filepath.Join(mapDir, "search.sqlite"),
		MapRunsDir:   filepath.Join(mapDir, "runs"),

		WikiDir:         filepath.Join(mapDir, "wiki"),
		WikiAreasDir:    filepath.Join(mapDir, "wiki", "areas"),
		WikiOverview:    filepath.Join(mapDir, "wiki", "overview.md"),
		WikiStructure:   filepath.Join(mapDir, "wiki", "structure.md"),
		WikiCommands:    filepath.Join(mapDir, "wiki", "commands.md"),
		WikiEntrypoints: filepath.Join(mapDir, "wiki", "entrypoints.md"),
		WikiConfig:      filepath.Join(mapDir, "wiki", "config.md"),
		WikiTests:       filepath.Join(mapDir, "wiki", "tests.md"),

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
		p.MapDir,
		p.AreasDir,
		p.WikiDir,
		p.WikiAreasDir,
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
