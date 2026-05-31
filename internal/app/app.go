package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/ledger"
	"github.com/heurema/pactum/internal/projectmap"
)

type App struct {
	WorkingDir string
	Now        func() time.Time
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

	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}

	switch args[0] {
	case "init":
		if len(args) > 2 {
			fmt.Fprintln(stderr, "usage: pactum init [path]")
			return 2
		}
		target := "."
		if len(args) == 2 {
			target = args[1]
		}
		root, err := a.resolveInitRoot(target)
		if err != nil {
			fmt.Fprintf(stderr, "pactum init: %v\n", err)
			return 1
		}
		if err := a.Init(root); err != nil {
			fmt.Fprintf(stderr, "pactum init: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Initialized Pactum workspace at %s\n", artifacts.New(root).Workspace)
		return 0
	case "status":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "usage: pactum status")
			return 2
		}
		if err := a.Status(stdout); err != nil {
			fmt.Fprintf(stderr, "pactum status: %v\n", err)
			return 1
		}
		return 0
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		printUsage(stderr)
		return 2
	}
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
	if err := ledger.Append(paths.EventsJSONL, ledger.Event{Type: "map_refresh_started", Timestamp: now, RunID: runID, RepoRoot: root}); err != nil {
		return err
	}

	scan, err := projectmap.Scan(root)
	if err != nil {
		return err
	}
	if err := projectmap.WriteJSONL(paths.FilesJSONL, scan.Files); err != nil {
		return err
	}
	if err := projectmap.WriteJSONL(paths.HashesJSONL, scan.Hashes); err != nil {
		return err
	}
	if err := os.WriteFile(paths.EntriesJSONL, nil, 0o644); err != nil {
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
		Artifacts: map[string]string{
			"repo_map":     "map/repo-map.md",
			"llms":         "map/llms.txt",
			"files":        "map/files.jsonl",
			"entries":      "map/entries.jsonl",
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
	files := map[string][]byte{
		paths.Config: []byte(defaultConfig()),
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

func defaultConfig() string {
	return strings.TrimSpace(`
schema: pactum.config.v1
default_profile: balanced
project_map:
  refresh: auto
  include_go_ast: false
  include_vendor: false
  include_generated: false
  max_file_bytes: 500000
limits:
  clarify:
    max_iterations: 5
    max_questions_per_round: 5
  execute:
    max_iterations: 10
  review:
    max_rounds: 4
budget:
  mode: warn
  max_usd: null
memory:
  enabled: true
  include_stale: warn
`) + "\n"
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
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

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: pactum <command>")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "commands:")
	fmt.Fprintln(w, "  init [path]   create a Pactum workspace and project map")
	fmt.Fprintln(w, "  status        print Pactum workspace status")
}
