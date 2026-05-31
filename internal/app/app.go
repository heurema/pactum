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
	searchpkg "github.com/heurema/pactum/internal/search"
	"gopkg.in/yaml.v3"
)

type App struct {
	WorkingDir string
	Now        func() time.Time
}

type cli struct {
	Init   initCmd   `cmd:"" help:"Create a Pactum workspace and project map."`
	Map    mapCmd    `cmd:"" help:"Advanced project map commands."`
	Search searchCmd `cmd:"" help:"Search the Pactum project map."`
	Status statusCmd `cmd:"" help:"Print Pactum workspace status."`
}

type initCmd struct {
	Path string `arg:"" optional:"" default:"." name:"path" help:"Repository path to initialize."`
}

type mapCmd struct {
	Refresh mapRefreshCmd `cmd:"" help:"Rebuild generated project map artifacts."`
}

type mapRefreshCmd struct {
	JSONOutput bool `name:"json" help:"Print machine-readable JSON output."`
	Full       bool `help:"Accepted for now; performs the same full rebuild as the default."`
}

type statusCmd struct {
	JSONOutput bool `name:"json" help:"Print machine-readable JSON output."`
}

type searchCmd struct {
	Query      string `arg:"" name:"query" help:"Search query."`
	Limit      int    `help:"Maximum number of results." default:"10"`
	Kind       string `help:"Document kind filter." default:"any" enum:"any,repo_map,llms,file,code_item"`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

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
	return r.App.Status(r.Stdout, c.JSONOutput)
}

func (c *searchCmd) Run(r *runner) error {
	return r.App.Search(r.Stdout, c.Query, c.Limit, c.Kind, c.JSONOutput)
}

func (c *mapRefreshCmd) Run(r *runner) error {
	_ = c.Full
	root, workspace, err := r.App.resolveStatusRoot()
	if err != nil {
		return err
	}
	if workspace == "" {
		return errors.New("Pactum is not initialized. Run: pactum init")
	}
	result, err := r.App.RefreshMap(root)
	if err != nil {
		return err
	}
	if c.JSONOutput {
		encoder := json.NewEncoder(r.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	writeMapRefreshResult(r.Stdout, result)
	return nil
}

func (a App) Init(root string) error {
	if err := projectmap.ValidateRoot(root); err != nil {
		return err
	}
	paths := artifacts.New(root)
	if err := ensureDirs(paths.Dirs()); err != nil {
		return err
	}

	now := a.nowUTC()
	runID := "map_" + now.Format("20060102_150405")

	if err := ledger.Append(paths.EventsJSONL, ledger.Event{Type: "init_started", Timestamp: now, RunID: runID, RepoRoot: root}); err != nil {
		return err
	}
	if err := writeStaticWorkspaceFiles(paths); err != nil {
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
	if err := writeJSON(paths.Manifest, manifest); err != nil {
		return err
	}
	result, err := a.refreshMap(root, now)
	if err != nil {
		return err
	}
	return ledger.Append(paths.EventsJSONL, ledger.Event{Type: "init_finished", Timestamp: result.FinishedAt, RunID: result.RunID, RepoRoot: root})
}

func (a App) Status(stdout io.Writer, jsonOutput bool) error {
	root, workspace, err := a.resolveStatusRoot()
	if err != nil {
		return err
	}
	if workspace == "" {
		if jsonOutput {
			return writeStatusNotInitialized(stdout)
		}
		fmt.Fprintln(stdout, "Pactum is not initialized. Run: pactum init")
		return nil
	}

	report, err := a.workspaceStatus(root)
	if err != nil {
		return err
	}
	if jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	writeWorkspaceStatus(stdout, report)
	return nil
}

func (a App) nowUTC() time.Time {
	if a.Now == nil {
		return time.Now().UTC()
	}
	return a.Now().UTC()
}

func writeWorkspaceStatus(stdout io.Writer, report statusResponse) {
	fmt.Fprintln(stdout, "Pactum status")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Repository:")
	fmt.Fprintf(stdout, "  root: %s\n", report.RepoRoot)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Workspace:")
	fmt.Fprintf(stdout, "  path: %s\n", report.Workspace)
	fmt.Fprintln(stdout, "  initialized: yes")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Project map:")
	fmt.Fprintf(stdout, "  status: %s\n", report.ProjectMap.Status)
	fmt.Fprintf(stdout, "  run: %s\n", report.ProjectMap.RunID)
	fmt.Fprintf(stdout, "  files indexed: %d\n", report.ProjectMap.FilesIndexed)
	fmt.Fprintf(stdout, "  code items: %d\n", report.ProjectMap.CodeItems)
	fmt.Fprintf(stdout, "  search index: %s\n", report.ProjectMap.SearchIndex)
	if len(report.ProjectMap.StaleReasons) > 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Stale reasons:")
		for _, reason := range report.ProjectMap.StaleReasons {
			fmt.Fprintf(stdout, "  - %s\n", reason)
		}
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Suggested:")
		fmt.Fprintln(stdout, "  pactum map refresh")
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Runs:")
	fmt.Fprintf(stdout, "  active: %d\n", report.Runs.Active)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Memory:")
	fmt.Fprintf(stdout, "  items: %d\n", report.Memory.Items)
	fmt.Fprintf(stdout, "  stale: %d\n", report.Memory.Stale)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Usage:")
	fmt.Fprintf(stdout, "  total tokens: %d\n", report.Usage.TotalTokens)
	fmt.Fprintf(stdout, "  estimated cost: $%.2f\n", report.Usage.EstimatedCostUSD)
}

func (a App) Search(stdout io.Writer, query string, limit int, kind string, jsonOutput bool) error {
	root, workspace, err := a.resolveStatusRoot()
	if err != nil {
		return err
	}
	if workspace == "" {
		fmt.Fprintln(stdout, "Pactum is not initialized. Run: pactum init")
		return nil
	}

	paths := artifacts.New(root)
	response, err := searchpkg.Query(paths.SearchSQLite, searchpkg.QueryOptions{
		Query: query,
		Limit: limit,
		Kind:  kind,
	})
	if err != nil {
		if searchpkg.IsMissingIndex(err) {
			fmt.Fprintln(stdout, "Search index is missing. Run: pactum map refresh")
			return nil
		}
		return err
	}

	if jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(response)
	}

	writeSearchResults(stdout, response)
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

func writeSearchResults(stdout io.Writer, response searchpkg.Response) {
	fmt.Fprintf(stdout, "Search results for: %s\n\n", response.Query)
	if len(response.Results) == 0 {
		fmt.Fprintln(stdout, "No results found.")
		return
	}
	for _, result := range response.Results {
		fmt.Fprintf(stdout, "%d. %s %s\n", result.Rank, result.Kind, result.Path)
		switch result.Kind {
		case searchpkg.KindCodeItem:
			fmt.Fprintf(stdout, "   kind: %s\n", result.CodeKind)
			fmt.Fprintf(stdout, "   name: %s\n", result.Title)
			if result.Language != "" {
				fmt.Fprintf(stdout, "   language: %s\n", result.Language)
			}
		case searchpkg.KindFile:
			if result.Language != "" {
				fmt.Fprintf(stdout, "   language: %s\n", result.Language)
			}
			if result.CodeKind != "" {
				fmt.Fprintf(stdout, "   kind: %s\n", result.CodeKind)
			}
		default:
			fmt.Fprintf(stdout, "   title: %s\n", result.Title)
		}
		if result.Rank < len(response.Results) {
			fmt.Fprintln(stdout)
		}
	}
}
