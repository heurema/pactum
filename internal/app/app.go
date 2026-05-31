package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/codeindex"
	"github.com/heurema/pactum/internal/ledger"
	"github.com/heurema/pactum/internal/projectmap"
	"gopkg.in/yaml.v3"
)

type App struct {
	WorkingDir string
	Now        func() time.Time
}

type cli struct {
	Init   initCmd   `cmd:"" help:"Create a Pactum workspace and project map."`
	Status statusCmd `cmd:"" help:"Print Pactum workspace status."`
}

type initCmd struct {
	Path string `arg:"" optional:"" default:"." name:"path" help:"Repository path to initialize."`
}

type statusCmd struct{}

type runner struct {
	App    App
	Stdout io.Writer
}

type workspaceManifest struct {
	Schema        string    `json:"schema"`
	Tool          string    `json:"tool"`
	ToolVersion   string    `json:"tool_version"`
	RepoRoot      string    `json:"repo_root"`
	InitializedAt time.Time `json:"initialized_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Map           struct {
		CurrentRunID string `json:"current_run_id"`
	} `json:"map"`
	Status string `json:"status"`
}

type configFile struct {
	Schema         string           `yaml:"schema"`
	DefaultProfile string           `yaml:"default_profile"`
	ProjectMap     projectMapConfig `yaml:"project_map"`
	Limits         limitsConfig     `yaml:"limits"`
	Budget         budgetConfig     `yaml:"budget"`
	Memory         memoryConfig     `yaml:"memory"`
}

type projectMapConfig struct {
	Refresh      string `yaml:"refresh"`
	MaxFileBytes int    `yaml:"max_file_bytes"`
	CodeIndex    string `yaml:"code_index"`
}

type limitsConfig struct {
	Clarify iterationLimits `yaml:"clarify"`
	Execute executeLimits   `yaml:"execute"`
	Review  reviewLimits    `yaml:"review"`
}

type iterationLimits struct {
	MaxIterations        int `yaml:"max_iterations"`
	MaxQuestionsPerRound int `yaml:"max_questions_per_round"`
}

type executeLimits struct {
	MaxIterations int `yaml:"max_iterations"`
}

type reviewLimits struct {
	MaxRounds int `yaml:"max_rounds"`
}

type budgetConfig struct {
	Mode   string   `yaml:"mode"`
	MaxUSD *float64 `yaml:"max_usd"`
}

type memoryConfig struct {
	Enabled      bool   `yaml:"enabled"`
	IncludeStale string `yaml:"include_stale"`
}

func Run(args []string, stdout, stderr io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "pactum: %v\n", err)
		return 1
	}
	return App{WorkingDir: wd, Now: time.Now}.Run(args, stdout, stderr)
}

func (a App) Run(args []string, stdout, stderr io.Writer) int {
	if a.Now == nil {
		a.Now = time.Now
	}
	if a.WorkingDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "pactum: %v\n", err)
			return 1
		}
		a.WorkingDir = wd
	}

	var command cli
	parser, err := kong.New(
		&command,
		kong.Name("pactum"),
		kong.Description("Contract-first CLI for agentic software work."),
		kong.UsageOnError(),
		kong.Writers(stdout, stderr),
	)
	if err != nil {
		fmt.Fprintf(stderr, "pactum: %v\n", err)
		return 1
	}
	ctx, err := parser.Parse(args)
	if err != nil {
		return 2
	}
	if err := ctx.Run(&runner{App: a, Stdout: stdout}); err != nil {
		fmt.Fprintf(stderr, "pactum %s: %v\n", ctx.Command(), err)
		return 1
	}
	return 0
}

func (c *initCmd) Run(r *runner) error {
	root, err := r.App.resolveInitRoot(c.Path)
	if err != nil {
		return err
	}
	if err := r.App.Init(root); err != nil {
		return err
	}
	fmt.Fprintf(r.Stdout, "Initialized Pactum workspace at %s\n", artifacts.New(root).Workspace)
	return nil
}

func (c *statusCmd) Run(r *runner) error {
	return r.App.Status(r.Stdout)
}

func (a App) Init(root string) error {
	if err := projectmap.ValidateRoot(root); err != nil {
		return err
	}
	paths := artifacts.New(root)
	if err := ensureDirs(paths.Dirs()); err != nil {
		return err
	}

	now := a.Now().UTC()
	runID := "map_" + now.Format("20060102_150405")

	if err := ledger.Append(paths.EventsJSONL, ledger.Event{Type: "init_started", Timestamp: now, RunID: runID, RepoRoot: root}); err != nil {
		return err
	}
	if err := writeStaticWorkspaceFiles(paths); err != nil {
		return err
	}
	config, err := readConfig(paths.Config)
	if err != nil {
		return err
	}
	if err := ledger.Append(paths.EventsJSONL, ledger.Event{Type: "map_refresh_started", Timestamp: now, RunID: runID, RepoRoot: root}); err != nil {
		return err
	}

	scan, err := projectmap.Scan(root, projectmap.ScanOptions{
		MaxFileBytes:  int64(config.ProjectMap.MaxFileBytes),
		CodeIndexMode: config.ProjectMap.CodeIndex,
	})
	if err != nil {
		return err
	}
	if err := projectmap.WriteJSONL(paths.FilesJSONL, scan.Files); err != nil {
		return err
	}
	if err := projectmap.WriteJSONL(paths.HashesJSONL, scan.Hashes); err != nil {
		return err
	}
	if err := projectmap.WriteJSONL(paths.CodeItemsJSONL, scan.CodeItems); err != nil {
		return err
	}
	if err := os.WriteFile(paths.RepoMap, projectmap.RenderRepoMap(root, now, scan), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(paths.LLMS, projectmap.RenderLLMS(), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(paths.AreaIndex, projectmap.RenderAreaIndex(), 0o644); err != nil {
		return err
	}

	mapManifest := projectmap.Manifest{
		Schema:       "pactum.map.manifest.v1",
		RunID:        runID,
		GeneratedAt:  now,
		RepoRoot:     root,
		FilesIndexed: len(scan.Files),
		FilesIgnored: scan.FilesIgnored,
		FilesSkipped: scan.FilesSkipped,
		CodeIndex: projectmap.CodeIndexManifest{
			Mode:               scan.CodeIndexMode,
			SupportedLanguages: codeindex.SupportedLanguages(),
			LanguagesSeen:      scan.CodeIndexLanguagesSeen,
			LanguagesIndexed:   scan.CodeIndexLanguagesIndexed,
			Items:              len(scan.CodeItems),
		},
		Warnings: scan.Warnings,
		Artifacts: map[string]string{
			"repo_map":     "map/repo-map.md",
			"llms":         "map/llms.txt",
			"files":        "map/files.jsonl",
			"code_items":   "map/code-items.jsonl",
			"hashes":       "map/hashes.jsonl",
			"areas_index":  "map/areas/_index.md",
			"map_manifest": "map/manifest.json",
		},
	}
	if err := writeJSON(paths.MapManifest, mapManifest); err != nil {
		return err
	}
	if err := ledger.Append(paths.EventsJSONL, ledger.Event{Type: "map_refresh_finished", Timestamp: now, RunID: runID, RepoRoot: root}); err != nil {
		return err
	}

	manifest := workspaceManifest{
		Schema:        "pactum.workspace.v1",
		Tool:          artifacts.ToolName,
		ToolVersion:   artifacts.ToolVersion,
		RepoRoot:      root,
		InitializedAt: now,
		UpdatedAt:     now,
		Status:        "initialized",
	}
	manifest.Map.CurrentRunID = runID
	if err := writeJSON(paths.Manifest, manifest); err != nil {
		return err
	}
	return ledger.Append(paths.EventsJSONL, ledger.Event{Type: "init_finished", Timestamp: now, RunID: runID, RepoRoot: root})
}

func (a App) Status(stdout io.Writer) error {
	root, workspace, err := a.resolveStatusRoot()
	if err != nil {
		return err
	}
	if workspace == "" {
		fmt.Fprintln(stdout, "Pactum is not initialized. Run: pactum init")
		return nil
	}

	paths := artifacts.New(root)
	if _, err := readConfig(paths.Config); err != nil {
		return err
	}
	manifest, err := readWorkspaceManifest(paths.Manifest)
	if err != nil {
		return err
	}
	mapManifest, err := readMapManifest(paths.MapManifest)
	if err != nil {
		return err
	}

	fmt.Fprintln(stdout, "Pactum status")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Repository:")
	fmt.Fprintf(stdout, "  root: %s\n", manifest.RepoRoot)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Workspace:")
	fmt.Fprintf(stdout, "  path: %s\n", paths.Workspace)
	fmt.Fprintln(stdout, "  initialized: yes")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Project map:")
	fmt.Fprintln(stdout, "  status: fresh")
	fmt.Fprintf(stdout, "  run: %s\n", mapManifest.RunID)
	fmt.Fprintf(stdout, "  files indexed: %d\n", mapManifest.FilesIndexed)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Runs:")
	fmt.Fprintln(stdout, "  active: 0")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Memory:")
	fmt.Fprintln(stdout, "  items: 0")
	fmt.Fprintln(stdout, "  stale: 0")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Usage:")
	fmt.Fprintln(stdout, "  total tokens: 0")
	fmt.Fprintln(stdout, "  estimated cost: $0.00")

	return nil
}

func (a App) resolveInitRoot(target string) (string, error) {
	path := target
	if !filepath.IsAbs(path) {
		path = filepath.Join(a.WorkingDir, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", err
	}
	if root, ok := findUp(abs, ".git"); ok {
		return root, nil
	}
	return abs, nil
}

func (a App) resolveStatusRoot() (root string, workspace string, err error) {
	start, err := filepath.Abs(a.WorkingDir)
	if err != nil {
		return "", "", err
	}
	if root, ok := findUp(start, ".git"); ok {
		paths := artifacts.New(root)
		if isDir(paths.Workspace) {
			return root, paths.Workspace, nil
		}
		return root, "", nil
	}
	for current := start; ; current = filepath.Dir(current) {
		paths := artifacts.New(current)
		if isDir(paths.Workspace) {
			return current, paths.Workspace, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return start, "", nil
}

func findUp(start, marker string) (string, bool) {
	for current := start; ; current = filepath.Dir(current) {
		if _, err := os.Stat(filepath.Join(current, marker)); err == nil {
			return current, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
	}
}

func ensureDirs(paths []string) error {
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func writeStaticWorkspaceFiles(paths artifacts.Paths) error {
	if err := writeDefaultConfigIfMissing(paths.Config); err != nil {
		return err
	}
	files := map[string][]byte{
		paths.Gitignore: []byte(strings.TrimSpace(`
locks/
tmp/
cache/
ledger/usage.jsonl
ledger/cost.json
`) + "\n"),
		paths.ProjectMemory: []byte("# Project Memory\n\nNo project memory has been extracted yet.\n"),
		paths.StaleReport:   []byte("{\"stale\":0,\"items\":[]}\n"),
		paths.UsageJSONL:    nil,
		paths.CostJSON:      []byte("{\"total_tokens\":0,\"estimated_cost_usd\":0}\n"),
	}
	for path, content := range files {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func writeDefaultConfigIfMissing(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeYAML(path, defaultConfigFile())
}

func defaultConfigFile() configFile {
	return configFile{
		Schema:         "pactum.config.v1",
		DefaultProfile: "balanced",
		ProjectMap: projectMapConfig{
			Refresh:      "auto",
			MaxFileBytes: 500000,
			CodeIndex:    codeindex.ModeAuto,
		},
		Limits: limitsConfig{
			Clarify: iterationLimits{
				MaxIterations:        5,
				MaxQuestionsPerRound: 5,
			},
			Execute: executeLimits{
				MaxIterations: 10,
			},
			Review: reviewLimits{
				MaxRounds: 4,
			},
		},
		Budget: budgetConfig{
			Mode:   "warn",
			MaxUSD: nil,
		},
		Memory: memoryConfig{
			Enabled:      true,
			IncludeStale: "warn",
		},
	}
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func writeYAML(path string, value any) error {
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(value); err != nil {
		_ = encoder.Close()
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buffer.Bytes(), 0o644)
}

func readWorkspaceManifest(path string) (workspaceManifest, error) {
	var manifest workspaceManifest
	if err := readJSON(path, &manifest); err != nil {
		return workspaceManifest{}, err
	}
	if manifest.Schema == "" || manifest.RepoRoot == "" {
		return workspaceManifest{}, errors.New("workspace manifest is incomplete")
	}
	return manifest, nil
}

func readConfig(path string) (configFile, error) {
	var config configFile
	data, err := os.ReadFile(path)
	if err != nil {
		return configFile{}, err
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return configFile{}, err
	}
	if config.Schema == "" {
		return configFile{}, errors.New("config is incomplete")
	}
	return config, nil
}

func readMapManifest(path string) (projectmap.Manifest, error) {
	var manifest projectmap.Manifest
	if err := readJSON(path, &manifest); err != nil {
		return projectmap.Manifest{}, err
	}
	if manifest.RunID == "" {
		return projectmap.Manifest{}, errors.New("project map manifest is incomplete")
	}
	return manifest, nil
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
