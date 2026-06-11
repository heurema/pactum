package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
)

func TestExportArchivesRunRecord(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runDir := filepath.Join(paths.RunsDir, runID)
	mustWriteFile(t, filepath.Join(runDir, "execute", "agent.log"), "raw transcript\n")

	before := snapshotWorkspaceFiles(t, paths.Workspace)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"export", runID, "--output", "record.zip", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("export exited %d, stderr: %s", code, stderr.String())
	}

	var response exportResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Schema != "pactum.export.v1" || response.RunID != runID || response.ArchiveFormat != "zip" {
		t.Fatalf("unexpected export response: %#v", response)
	}
	rootEntry := "pactum-run-" + runID + "/"
	if response.ArchiveRoot != rootEntry {
		t.Fatalf("archive root = %q, want %q", response.ArchiveRoot, rootEntry)
	}
	// Relative output paths resolve against the invocation working directory.
	output := filepath.Join(root, "record.zip")
	if response.Output != output {
		t.Fatalf("output = %q, want %q", response.Output, output)
	}
	info, err := os.Stat(output)
	assertNoError(t, err)
	if response.Bytes != info.Size() {
		t.Fatalf("bytes = %d, want archive size %d", response.Bytes, info.Size())
	}

	names, contents := readExportArchive(t, output)
	if response.Entries != len(names) {
		t.Fatalf("entries = %d, want %d", response.Entries, len(names))
	}
	if !sort.StringsAreSorted(names) {
		t.Fatalf("archive entries are not sorted: %v", names)
	}
	for _, name := range names {
		if !strings.HasPrefix(name, rootEntry) {
			t.Fatalf("entry %q is not rooted at %q", name, rootEntry)
		}
		if strings.Contains(name, `\`) {
			t.Fatalf("entry %q is not slash-separated", name)
		}
		assertDoesNotContainRoot(t, "archive entry", name, root)
	}
	for _, want := range []string{
		rootEntry,
		rootEntry + "contract/",
		rootEntry + "run.json",
		rootEntry + "task.md",
		rootEntry + "contract/contract.json",
		rootEntry + "execute/agent.log",
		rootEntry + "ledger/usage.jsonl",
		rootEntry + "ledger/events.filtered.jsonl",
	} {
		index := sort.SearchStrings(names, want)
		if index >= len(names) || names[index] != want {
			t.Fatalf("archive is missing %q:\n%v", want, names)
		}
	}

	// The sidecar holds exactly the workspace ledger events recorded for this
	// run, verbatim; init events for the map run are filtered out.
	sidecar := contents[rootEntry+"ledger/events.filtered.jsonl"]
	wantEvents := []string{}
	for _, line := range readLines(t, paths.EventsJSONL) {
		if strings.Contains(line, `"run_id":"`+runID+`"`) {
			wantEvents = append(wantEvents, line)
		}
	}
	if len(wantEvents) == 0 {
		t.Fatalf("test setup recorded no events for %s", runID)
	}
	if sidecar != strings.Join(wantEvents, "\n")+"\n" {
		t.Fatalf("filtered events sidecar mismatch:\n%s\nwant:\n%s", sidecar, strings.Join(wantEvents, "\n"))
	}
	if response.FilteredEvents != len(wantEvents) {
		t.Fatalf("filtered_events = %d, want %d", response.FilteredEvents, len(wantEvents))
	}

	// Export is read-only on Pactum state.
	after := snapshotWorkspaceFiles(t, paths.Workspace)
	if len(after) != len(before) {
		t.Fatalf("workspace file count changed during export: %d -> %d", len(before), len(after))
	}
	for rel, content := range before {
		if after[rel] != content {
			t.Fatalf("workspace file changed during export: %s", rel)
		}
	}
}

func TestExportIsByteStableAcrossMetadataChanges(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	first := filepath.Join(root, "first.zip")
	second := filepath.Join(root, "second.zip")

	runReviewCommand(t, app, "export", runID, "--output", first)

	// Metadata-only changes to the source files must not change the archive.
	target := filepath.Join(paths.RunsDir, runID, "task.md")
	assertNoError(t, os.Chmod(target, 0o600))
	assertNoError(t, os.Chtimes(target, time.Unix(0, 0), time.Unix(0, 0)))
	runReviewCommand(t, app, "export", runID, "--output", second)

	if mustReadFile(t, first) != mustReadFile(t, second) {
		t.Fatalf("repeated exports of unchanged contents are not byte-for-byte identical")
	}

	reader, err := zip.OpenReader(first)
	assertNoError(t, err)
	defer reader.Close()
	for _, file := range reader.File {
		wantMode := fs.FileMode(0o644)
		if file.FileInfo().IsDir() {
			wantMode = 0o755
		}
		if file.Mode().Perm() != wantMode {
			t.Fatalf("entry %q mode = %v, want %v", file.Name, file.Mode().Perm(), wantMode)
		}
		if !file.Modified.Equal(exportEntryEpoch) {
			t.Fatalf("entry %q timestamp = %v, want fixed %v", file.Name, file.Modified, exportEntryEpoch)
		}
	}
}

func TestExportResolvesCurrentRunAndPrintsSummary(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)
	output := filepath.Join(root, "current.zip")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"export", "--output", output}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("export exited %d, stderr: %s", code, stderr.String())
	}

	got := stdout.String()
	for _, want := range []string{"Pactum export", "  id: " + runID, "  output: " + output, "  format: zip", "  entries: ", "  filtered events: "} {
		if !strings.Contains(got, want) {
			t.Fatalf("human output missing %q:\n%s", want, got)
		}
	}
	// Essentials only: the archive contents are not printed.
	if strings.Contains(got, "contract/contract.json") {
		t.Fatalf("human output should not list archive contents:\n%s", got)
	}
	assertFile(t, output)
}

func TestExportFailsForMissingRun(t *testing.T) {
	root := t.TempDir()
	app, _, _ := setupContractRun(t, root)
	output := filepath.Join(root, "missing-run.zip")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"export", "run_29990101_000000", "--output", output}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "run not found") {
		t.Fatalf("export exited %d, stderr: %s", code, stderr.String())
	}
	assertNotExists(t, output)
}

func TestExportRejectsTraversalRunID(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)
	output := filepath.Join(root, "out.zip")

	for _, id := range []string{"..", "../..", runID + "/../.."} {
		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"export", id, "--output", output}, &stdout, &stderr)
		if code != 1 || !strings.Contains(stderr.String(), "invalid run id") {
			t.Fatalf("export %s exited %d, stderr: %s", id, code, stderr.String())
		}
		assertNotExists(t, output)
	}
}

func TestExportFailsWhenOutputExists(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)
	output := filepath.Join(root, "taken.zip")
	mustWriteFile(t, output, "occupied")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"export", runID, "--output", output}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "output path already exists") {
		t.Fatalf("export exited %d, stderr: %s", code, stderr.String())
	}
	if mustReadFile(t, output) != "occupied" {
		t.Fatalf("existing output file was modified")
	}
}

// TestExportFinalizeRefusesToReplaceOutputCreatedAfterStaging simulates a
// concurrent export winning the race between the early existence check and
// finalization: the output exists by the time the staged archive is
// published, and the finalize must refuse to replace it.
func TestExportFinalizeRefusesToReplaceOutputCreatedAfterStaging(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "winner.zip")
	mustWriteFile(t, output, "concurrent winner")

	_, err := writeExportArchive(output, []exportEntry{{name: "pactum-run-x/", dir: true}})
	if err == nil || !strings.Contains(err.Error(), "output path already exists") {
		t.Fatalf("finalize over existing output err = %v, want output path already exists", err)
	}
	if mustReadFile(t, output) != "concurrent winner" {
		t.Fatalf("existing output file was overwritten")
	}
	assertNoExportLeftovers(t, dir)
}

func TestExportRejectsBackslashInEntryName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("backslash is the path separator on Windows, not a filename character")
	}
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	offending := `back\slash.md`
	mustWriteFile(t, filepath.Join(paths.RunsDir, runID, offending), "not portable\n")
	output := filepath.Join(root, "out.zip")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"export", runID, "--output", output}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "backslash") || !strings.Contains(stderr.String(), offending) {
		t.Fatalf("export exited %d, stderr: %s", code, stderr.String())
	}
	assertNotExists(t, output)
	assertNoExportLeftovers(t, root)
}

func TestExportFailsWhenOutputParentMissing(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)
	output := filepath.Join(root, "reports", "run.zip")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"export", runID, "--output", output}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "output parent directory does not exist") {
		t.Fatalf("export exited %d, stderr: %s", code, stderr.String())
	}
	assertNotExists(t, output)
}

func TestExportRejectsOutputInsideRunDirectory(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runDir := filepath.Join(paths.RunsDir, runID)
	before := snapshotWorkspaceFiles(t, runDir)

	for _, output := range []string{
		filepath.Join(runDir, "self.zip"),
		filepath.Join(runDir, "execute", "self.zip"),
	} {
		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"export", runID, "--output", output}, &stdout, &stderr)
		if code != 1 || !strings.Contains(stderr.String(), "output path is inside the exported run directory") {
			t.Fatalf("export to %s exited %d, stderr: %s", output, code, stderr.String())
		}
	}

	after := snapshotWorkspaceFiles(t, runDir)
	if len(after) != len(before) {
		t.Fatalf("rejected export wrote into the run directory")
	}
}

func TestExportRejectsSymlinkedOutputParentIntoRunDirectory(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runDir := filepath.Join(paths.RunsDir, runID)
	escape := filepath.Join(root, "escape")
	assertNoError(t, os.Symlink(runDir, escape))
	before := snapshotWorkspaceFiles(t, runDir)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"export", runID, "--output", filepath.Join(escape, "self.zip")}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "output path is inside the exported run directory") {
		t.Fatalf("export exited %d, stderr: %s", code, stderr.String())
	}

	after := snapshotWorkspaceFiles(t, runDir)
	if len(after) != len(before) {
		t.Fatalf("rejected export wrote into the run directory")
	}
}

func TestExportFailsOnSymlinkAndRemovesPartialArchive(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	assertNoError(t, os.Symlink(filepath.Join(root, "README.md"), filepath.Join(paths.RunsDir, runID, "link.md")))
	output := filepath.Join(root, "out.zip")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"export", runID, "--output", output}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "not a regular file or directory") {
		t.Fatalf("export exited %d, stderr: %s", code, stderr.String())
	}
	assertNotExists(t, output)
	assertNoExportLeftovers(t, root)
}

func TestExportFailsWhenRunFileUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("file permissions are not enforced for root")
	}
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	assertNoError(t, os.Chmod(filepath.Join(paths.RunsDir, runID, "task.md"), 0o000))
	output := filepath.Join(root, "out.zip")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"export", runID, "--output", output}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "permission denied") {
		t.Fatalf("export exited %d, stderr: %s", code, stderr.String())
	}
	assertNotExists(t, output)
	assertNoExportLeftovers(t, root)
}

func TestExportFailsWhenEventsLedgerMissing(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	assertNoError(t, os.Remove(paths.EventsJSONL))
	output := filepath.Join(root, "out.zip")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"export", runID, "--output", output}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "events ledger") {
		t.Fatalf("export exited %d, stderr: %s", code, stderr.String())
	}
	assertNotExists(t, output)
	assertNoExportLeftovers(t, root)
}

func TestExportFailsOnMalformedEventsLedger(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	assertNoError(t, activeStore.AppendBytes(paths.EventsJSONL, []byte("not json\n")))
	output := filepath.Join(root, "out.zip")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"export", runID, "--output", output}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "malformed JSONL") {
		t.Fatalf("export exited %d, stderr: %s", code, stderr.String())
	}
	assertNotExists(t, output)
	assertNoExportLeftovers(t, root)
}

func TestExportIncludesEmptySidecarWhenRunHasNoEvents(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")
	app := testApp(root)
	runReviewCommand(t, app, "init")

	// A reserved run directory whose events were never recorded: the filtered
	// sidecar must still be present, just empty.
	paths := artifacts.New(root)
	runID := "run_20260601_000000"
	mustWriteFile(t, filepath.Join(paths.RunsDir, runID, "run.json"), "{\"status\":\"contract_draft\"}\n")
	output := filepath.Join(root, "out.zip")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"export", runID, "--output", output, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("export exited %d, stderr: %s", code, stderr.String())
	}
	var response exportResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.FilteredEvents != 0 {
		t.Fatalf("filtered_events = %d, want 0", response.FilteredEvents)
	}
	_, contents := readExportArchive(t, output)
	sidecar, ok := contents["pactum-run-"+runID+"/ledger/events.filtered.jsonl"]
	if !ok || sidecar != "" {
		t.Fatalf("empty sidecar missing or non-empty (present=%v): %q", ok, sidecar)
	}
}

func TestExportIncludesRunRecordEventsLedgerAlongsideSidecar(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	recordEvents := `{"type":"engine_event","run_id":"` + runID + `"}` + "\n"
	mustWriteFile(t, filepath.Join(paths.RunsDir, runID, "ledger", "events.jsonl"), recordEvents)
	output := filepath.Join(root, "out.zip")

	runReviewCommand(t, app, "export", runID, "--output", output)

	rootEntry := "pactum-run-" + runID + "/"
	_, contents := readExportArchive(t, output)
	if contents[rootEntry+"ledger/events.jsonl"] != recordEvents {
		t.Fatalf("run record events ledger not exported verbatim: %q", contents[rootEntry+"ledger/events.jsonl"])
	}
	wantEvents := []string{}
	for _, line := range readLines(t, paths.EventsJSONL) {
		if strings.Contains(line, `"run_id":"`+runID+`"`) {
			wantEvents = append(wantEvents, line)
		}
	}
	sidecar := contents[rootEntry+"ledger/events.filtered.jsonl"]
	if sidecar != strings.Join(wantEvents, "\n")+"\n" {
		t.Fatalf("filtered events sidecar mismatch:\n%s\nwant:\n%s", sidecar, strings.Join(wantEvents, "\n"))
	}
}

func TestExportFailsWhenSidecarPathIsShadowed(t *testing.T) {
	// Any run entry occupying the sidecar path — including a DIRECTORY, whose
	// archive name carries a trailing slash — or a non-directory entry at the
	// ledger/ component must fail the export: shadowing would produce an
	// archive holding a file and a directory at the same path.
	cases := []struct {
		name    string
		prepare func(t *testing.T, runDir string)
		wantErr string
	}{
		{
			name: "directory at the sidecar path",
			prepare: func(t *testing.T, runDir string) {
				assertNoError(t, os.MkdirAll(filepath.Join(runDir, "ledger", "events.filtered.jsonl"), 0o755))
			},
			wantErr: "already contains ledger/events.filtered.jsonl",
		},
		{
			name: "file at the sidecar path",
			prepare: func(t *testing.T, runDir string) {
				mustWriteFile(t, filepath.Join(runDir, "ledger", "events.filtered.jsonl"), "{}\n")
			},
			wantErr: "already contains ledger/events.filtered.jsonl",
		},
		{
			name: "regular file named ledger",
			prepare: func(t *testing.T, runDir string) {
				assertNoError(t, os.RemoveAll(filepath.Join(runDir, "ledger")))
				mustWriteFile(t, filepath.Join(runDir, "ledger"), "not a directory\n")
			},
			wantErr: "non-directory ledger entry",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			app, paths, runID := setupContractRun(t, root)
			tc.prepare(t, filepath.Join(paths.RunsDir, runID))
			output := filepath.Join(root, "out.zip")

			var stdout, stderr bytes.Buffer
			code := app.Run([]string{"export", runID, "--output", output}, &stdout, &stderr)
			if code == 0 {
				t.Fatalf("export must fail when the sidecar path is shadowed")
			}
			if !strings.Contains(stderr.String(), tc.wantErr) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tc.wantErr)
			}
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf("failed export must not leave an archive behind: %v", err)
			}
		})
	}
}

func readExportArchive(t *testing.T, path string) ([]string, map[string]string) {
	t.Helper()
	reader, err := zip.OpenReader(path)
	assertNoError(t, err)
	defer reader.Close()
	names := []string{}
	contents := map[string]string{}
	for _, file := range reader.File {
		names = append(names, file.Name)
		if file.FileInfo().IsDir() {
			continue
		}
		rc, err := file.Open()
		assertNoError(t, err)
		data, err := io.ReadAll(rc)
		rc.Close()
		assertNoError(t, err)
		contents[file.Name] = string(data)
	}
	return names, contents
}

func snapshotWorkspaceFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	assertNoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		snapshot[rel] = mustReadFile(t, path)
		return nil
	}))
	return snapshot
}

func assertNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("%s unexpectedly exists (err=%v)", path, err)
	}
}

// assertNoExportLeftovers verifies a failed export removed its temporary
// sibling archive.
func assertNoExportLeftovers(t *testing.T, dir string) {
	t.Helper()
	leftovers, err := filepath.Glob(filepath.Join(dir, ".pactum-export-*"))
	assertNoError(t, err)
	if len(leftovers) != 0 {
		t.Fatalf("failed export left temporary files: %v", leftovers)
	}
}

func TestExportRejectsBackslashInDirectoryEntryName(t *testing.T) {
	// Directory entry names get a trailing slash through a separate call site
	// of the backslash validation; pin that arm too.
	if runtime.GOOS == "windows" {
		t.Skip("backslash is the path separator on Windows, not a filename character")
	}
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	offending := `back\slash-dir`
	assertNoError(t, os.MkdirAll(filepath.Join(paths.RunsDir, runID, offending), 0o755))
	output := filepath.Join(root, "out.zip")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"export", runID, "--output", output}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "backslash") || !strings.Contains(stderr.String(), offending) {
		t.Fatalf("export exited %d, stderr: %s", code, stderr.String())
	}
	assertNotExists(t, output)
	assertNoExportLeftovers(t, root)
}
