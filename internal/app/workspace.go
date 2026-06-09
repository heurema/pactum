package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/ledger"
	"github.com/heurema/pactum/internal/projectmap"
	"github.com/heurema/pactum/internal/version"
)

// ensureInitialized returns errNotInitialized when no workspace exists. Mutating
// commands without a run id argument use it so they exit 1 (not 0) before init.
func (a App) ensureInitialized() error {
	_, workspace, err := a.resolveStatusRoot()
	if err != nil {
		return err
	}
	if workspace == "" {
		return errNotInitialized
	}
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

	if err := ledger.Append(activeStore, paths.EventsJSONL, ledger.Event{Type: "init_started", Timestamp: now, RunID: runID}); err != nil {
		return err
	}
	if err := writeStaticWorkspaceFiles(paths); err != nil {
		return err
	}
	manifest := workspaceManifest{
		Schema:        workspaceSchema,
		Tool:          artifacts.ToolName,
		ToolVersion:   version.Version,
		RepoRoot:      ".",
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
	return ledger.Append(activeStore, paths.EventsJSONL, ledger.Event{Type: "init_finished", Timestamp: result.FinishedAt, RunID: result.RunID})
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

// requireWorkspace resolves the repository root and confirms that a Pactum
// workspace has been initialized. When it has not, it emits the standard notice
// (in the requested format) and returns ok=false so callers can return their
// zero value immediately.
func (a App) requireWorkspace(stdout io.Writer, jsonOutput bool) (root string, paths artifacts.Paths, ok bool, err error) {
	root, workspace, err := a.resolveStatusRoot()
	if err != nil {
		return "", artifacts.Paths{}, false, err
	}
	if workspace == "" {
		return "", artifacts.Paths{}, false, notInitialized(stdout, jsonOutput)
	}
	return root, artifacts.New(root), true, nil
}

// notInitialized writes the standard "workspace not initialized" notice. The
// returned error is the encoder error (nil for the text form), so callers can
// return it directly as a graceful exit.
func notInitialized(stdout io.Writer, jsonOutput bool) error {
	if jsonOutput {
		return writeStatusNotInitialized(stdout)
	}
	fmt.Fprintln(stdout, "Pactum is not initialized. Run: pactum init")
	return nil
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
		if err := activeStore.MkdirAll(path); err != nil {
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
# Regenerable artifacts derived from the repo, the project map, or memory.
locks/
cache/
tmp/
map/
runs/*/context/

# Raw command transcripts. The run outcome lives in git history and the durable
# learnings are distilled into the committed memory records; the structured run
# records (contracts, decisions, the ledger, gate verdicts, review findings) are
# versioned, but these logs are not.
*.log
`) + "\n"),
		paths.ProjectMemory: []byte("# Project Memory\n\nNo project memory has been extracted yet.\n"),
		paths.MemoryItems:   nil,
		paths.StaleReport:   []byte("{\"stale\":0,\"items\":[]}\n"),
		paths.UsageJSONL:    nil,
		paths.CostJSON:      []byte("{\"total_tokens\":0,\"estimated_cost_usd\":0}\n"),
	}
	for path, content := range files {
		if err := activeStore.WriteBytes(path, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}
