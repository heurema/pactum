package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/heurema/pactum/internal/agents"
	searchpkg "github.com/heurema/pactum/internal/search"
)

const (
	workspaceSchema = "pactum.workspace.v1"
	configSchema    = "pactum.config.v1"

	gateScopeEnforcementBlock = "block"
	gateScopeEnforcementWarn  = "warn"
)

type App struct {
	WorkingDir     string
	Now            func() time.Time
	AgentRegistry  agents.Registry
	AgentTransport agents.Transport
}

type runner struct {
	App    App
	Stdout io.Writer
	// Stderr is the operator's stderr. Agent-running commands pass it as the live
	// output writer so the agent's stdout/stderr stream there as it runs, keeping
	// Stdout the clean result channel (human summary or --json).
	Stderr io.Writer
}

func Run(args []string, stdout, stderr io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "pactum: %v\n", err)
		return 1
	}
	return App{WorkingDir: wd, Now: time.Now}.Run(args, stdout, stderr)
}

// kongExit carries the exit code kong requests (for example from --help) out
// through a panic so App.Run can return it instead of letting kong terminate the
// process with os.Exit. This keeps help/parse exits testable in-process and lets
// Pactum be embedded without the parser killing the host process.
type kongExit struct{ code int }

func (a App) Run(args []string, stdout, stderr io.Writer) (code int) {
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

	defer func() {
		if r := recover(); r != nil {
			exit, ok := r.(kongExit)
			if !ok {
				panic(r)
			}
			code = exit.code
		}
	}()

	var command cli
	parser, err := kong.New(
		&command,
		kong.Name("pactum"),
		kong.Description("Contract-first CLI for agentic software work."),
		kong.UsageOnError(),
		kong.Writers(stdout, stderr),
		kong.Exit(func(c int) { panic(kongExit{code: c}) }),
	)
	if err != nil {
		fmt.Fprintf(stderr, "pactum: %v\n", err)
		return 1
	}
	ctx, err := parser.Parse(args)
	if err != nil {
		// Do not exit silently: report the parse error and a help pointer on
		// stderr, keeping stdout clean for scripts. Parser errors exit 2.
		fmt.Fprintf(stderr, "pactum: %v\n", err)
		fmt.Fprintln(stderr, "Run 'pactum --help' for usage.")
		return 2
	}
	if err := ctx.Run(&runner{App: a, Stdout: stdout, Stderr: stderr}); err != nil {
		// Command errors exit 1. With --json, emit a machine-readable error
		// envelope on stdout (stderr stays empty). Deterministic gate failures
		// already wrote their gate report JSON, so leave that as the only payload.
		// Otherwise use a human line on stderr.
		if jsonRequested(args) {
			var gateErr gateProcessError
			if errors.As(err, &gateErr) {
				return 1
			}
			// A failed agent attempt already wrote its run-only result JSON;
			// appending an envelope would put two documents on one stdout.
			// A preconditionError wrapping the attempt failure wins: there the
			// attempt ran inside a larger flow (e.g. the clarify loop) whose
			// own envelope is the single payload.
			var attemptErr agentAttemptFailedError
			var precondition *preconditionError
			if errors.As(err, &attemptErr) && !errors.As(err, &precondition) {
				return 1
			}
			if encErr := writeErrorEnvelope(stdout, err); encErr == nil {
				return 1
			}
		}
		fmt.Fprintf(stderr, "pactum %s: %v\n", ctx.Command(), err)
		return 1
	}
	return 0
}

func (a App) Status(stdout io.Writer, jsonOutput bool) error {
	root, _, ok, err := a.requireWorkspace(stdout, jsonOutput)
	if err != nil || !ok {
		return err
	}

	report, err := a.workspaceStatus(root)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSONResponse(stdout, report)
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
	if report.Runs.LatestRunID != "" {
		fmt.Fprintf(stdout, "  latest: %s\n", report.Runs.LatestRunID)
		fmt.Fprintf(stdout, "  latest status: %s\n", report.Runs.LatestStatus)
	}
	if report.Runs.CurrentRunID != "" {
		fmt.Fprintf(stdout, "  current: %s\n", report.Runs.CurrentRunID)
	}
	if report.Runs.NextCommand != "" {
		fmt.Fprintf(stdout, "  next: %s\n", report.Runs.NextCommand)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Memory:")
	fmt.Fprintf(stdout, "  items: %d\n", report.Memory.Items)
	fmt.Fprintf(stdout, "  stale: %d\n", report.Memory.Stale)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Usage:")
	if report.Usage.RunID != "" {
		fmt.Fprintf(stdout, "  run: %s\n", report.Usage.RunID)
	}
	fmt.Fprintf(stdout, "  total tokens: %d\n", report.Usage.TotalTokens)
	fmt.Fprintf(stdout, "  captured calls: %d\n", report.Usage.CapturedCalls)
	fmt.Fprintf(stdout, "  uncaptured calls: %d\n", report.Usage.UncapturedCalls)
	fmt.Fprintf(stdout, "  cache read ratio: %.2f%%\n", report.Usage.CacheReadRatio*100)
	fmt.Fprintf(stdout, "  estimated cost: $%.2f\n", report.Usage.EstimatedCostUSD)
}

func (a App) Search(stdout io.Writer, query string, limit int, kind string, symbol string, jsonOutput bool) error {
	symbol = strings.TrimSpace(symbol)
	if symbol != "" {
		normalizedKind, err := searchpkg.NormalizeKind(kind)
		if err != nil {
			return err
		}
		if normalizedKind != searchpkg.KindAny && normalizedKind != searchpkg.KindCodeItem {
			return fmt.Errorf("--symbol only applies to code_item results; drop --kind %s", normalizedKind)
		}
	}
	if strings.TrimSpace(query) == "" && symbol == "" {
		return errors.New("usage: pactum search <query>, or pactum search --symbol <name>")
	}

	_, paths, ok, err := a.requireWorkspace(stdout, false)
	if err != nil || !ok {
		return err
	}

	response, err := searchpkg.Query(paths.SearchSQLite, searchpkg.QueryOptions{
		Query:  query,
		Limit:  limit,
		Kind:   kind,
		Symbol: symbol,
	})
	if err != nil {
		if searchpkg.IsMissingIndex(err) {
			fmt.Fprintln(stdout, "Search index is missing. Run: pactum map refresh")
			return nil
		}
		if searchpkg.IsStaleIndex(err) {
			fmt.Fprintln(stdout, "Search index is stale. Run: pactum map refresh")
			return nil
		}
		return err
	}

	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}

	writeSearchResults(stdout, response)
	return nil
}

func (a App) agentRegistry() agents.Registry {
	if a.AgentRegistry != nil {
		return a.AgentRegistry
	}
	return agents.BuiltinRegistry{}
}

func (a App) agentTransport() agents.Transport {
	if a.AgentTransport != nil {
		return a.AgentTransport
	}
	if a.acpTransportEnabled() {
		return agents.ACPTransport{}
	}
	return agents.CLITransport{}
}

// acpTransportEnabled reports whether the ACP transport is selected. ACP is
// the default; the PACTUM_AGENT_TRANSPORT env var is a debug escape hatch, not
// config: only "cli" (case-insensitive, trimmed) selects the one-shot CLI
// transport, while empty, "acp", or any other value keeps the ACP default.
func (a App) acpTransportEnabled() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("PACTUM_AGENT_TRANSPORT")), "cli")
}

func writeSearchResults(stdout io.Writer, response searchpkg.Response) {
	fmt.Fprintf(stdout, "Search results for: %s\n\n", response.Query)
	if len(response.Results) == 0 {
		fmt.Fprintln(stdout, "No results found.")
		return
	}
	for _, result := range response.Results {
		fmt.Fprintf(stdout, "%d. %s %s\n", result.Rank, result.Kind, result.Address())
		switch result.Kind {
		case searchpkg.KindCodeItem, searchpkg.KindImport:
			fmt.Fprintf(stdout, "   kind: %s\n", result.CodeKind)
			fmt.Fprintf(stdout, "   name: %s\n", result.Title)
			if result.Language != "" {
				fmt.Fprintf(stdout, "   language: %s\n", result.Language)
			}
			if result.Signature != "" {
				fmt.Fprintf(stdout, "   signature: %s\n", result.Signature)
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
