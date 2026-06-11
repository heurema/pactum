package app

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	exportResponseSchema = "pactum.export.v1"
	exportArchiveFormat  = "zip"
)

// exportEntryEpoch is the fixed timestamp stamped on every archive entry so
// repeated exports of unchanged inputs are byte-for-byte identical. ZIP cannot
// represent times before 1980.
var exportEntryEpoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

type exportResponse struct {
	Schema         string `json:"schema"`
	RunID          string `json:"run_id"`
	Output         string `json:"output"`
	ArchiveFormat  string `json:"archive_format"`
	ArchiveRoot    string `json:"archive_root"`
	Entries        int    `json:"entries"`
	Bytes          int64  `json:"bytes"`
	FilteredEvents int    `json:"filtered_events"`
}

// exportEntry is one archive entry in its final slash-separated form.
// Directory names end in "/". A file entry carries either a source path under
// the run directory or, for the generated events sidecar, inline content.
type exportEntry struct {
	name    string
	dir     bool
	source  string
	content []byte
}

// Export archives a run's on-disk record into a deterministic ZIP at output.
// It is read-only on Pactum state: the only write is the archive itself,
// staged as a temporary sibling file and renamed into place on success.
func (a App) Export(stdout io.Writer, runID string, output string, jsonOutput bool) error {
	_, paths, ok, err := a.requireWorkspace(stdout, jsonOutput)
	if err != nil || !ok {
		return err
	}
	// A run id is a plain directory name; a path-shaped id could traverse
	// outside the runs directory and export an arbitrary directory.
	if runID != filepath.Base(runID) || !looksLikeRunID(runID) {
		return fmt.Errorf("invalid run id: %s", runID)
	}
	if !runExists(paths, runID) {
		return fmt.Errorf("run not found: %s", runID)
	}

	runDir := filepath.Join(paths.RunsDir, runID)
	output, err = a.resolveExportOutput(output, runDir)
	if err != nil {
		return err
	}

	archiveRoot := "pactum-run-" + runID + "/"
	entries, err := collectExportEntries(runDir, archiveRoot)
	if err != nil {
		return err
	}
	events, eventCount, err := filterRunEvents(paths.EventsJSONL, runID)
	if err != nil {
		return err
	}
	entries, err = appendEventsSidecar(entries, archiveRoot, events)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })

	size, err := writeExportArchive(output, entries)
	if err != nil {
		return err
	}

	response := exportResponse{
		Schema:         exportResponseSchema,
		RunID:          runID,
		Output:         output,
		ArchiveFormat:  exportArchiveFormat,
		ArchiveRoot:    archiveRoot,
		Entries:        len(entries),
		Bytes:          size,
		FilteredEvents: eventCount,
	}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeExport(stdout, response)
	return nil
}

// resolveExportOutput validates the requested archive path: relative paths
// resolve against the invocation working directory, the path must not already
// exist, its parent directory must already exist, and it must not point inside
// the exported run directory (the archive would land in the record it
// snapshots). The inside-run check compares symlink-resolved paths so a
// symlinked parent directory cannot smuggle the archive into the run record.
func (a App) resolveExportOutput(output string, runDir string) (string, error) {
	output = strings.TrimSpace(output)
	if !filepath.IsAbs(output) {
		output = filepath.Join(a.WorkingDir, output)
	}
	output = filepath.Clean(output)
	if _, err := os.Lstat(output); err == nil {
		return "", fmt.Errorf("output path already exists: %s", output)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(output)
	if !isDir(parent) {
		return "", fmt.Errorf("output parent directory does not exist: %s", parent)
	}
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", err
	}
	resolvedRunDir, err := filepath.EvalSymlinks(runDir)
	if err != nil {
		return "", err
	}
	target := filepath.Join(resolvedParent, filepath.Base(output))
	if rel, err := filepath.Rel(resolvedRunDir, target); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("output path is inside the exported run directory: %s", output)
	}
	return output, nil
}

// collectExportEntries walks the run directory and returns one entry per
// regular file and directory, named under archiveRoot. Anything else fails the
// export: following a symlink could leak files outside the run record, and
// special files do not belong in a portable archive.
func collectExportEntries(runDir string, archiveRoot string) ([]exportEntry, error) {
	entries := []exportEntry{}
	err := filepath.WalkDir(runDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(runDir, path)
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			name := archiveRoot
			if rel != "." {
				name += filepath.ToSlash(rel) + "/"
			}
			entries = append(entries, exportEntry{name: name, dir: true})
		case entry.Type().IsRegular():
			entries = append(entries, exportEntry{name: archiveRoot + filepath.ToSlash(rel), source: path})
		default:
			return fmt.Errorf("run record entry is not a regular file or directory: %s", rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// filterRunEvents reads the workspace events ledger and returns the verbatim
// lines recorded for runID. The ledger is part of the audit trail, so a
// missing, unreadable, or malformed ledger fails the export rather than
// producing a silently incomplete record.
func filterRunEvents(path string, runID string) ([]byte, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("events ledger: %w", err)
	}
	defer file.Close()

	var filtered bytes.Buffer
	count := 0
	lineNo := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event struct {
			RunID string `json:"run_id"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, 0, fmt.Errorf("events ledger line %d is malformed JSONL: %v", lineNo, err)
		}
		if event.RunID != runID {
			continue
		}
		filtered.WriteString(line)
		filtered.WriteByte('\n')
		count++
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("events ledger: %w", err)
	}
	return filtered.Bytes(), count, nil
}

// appendEventsSidecar adds the export-only filtered events copy under
// ledger/events.filtered.jsonl in the archive, creating the ledger/ directory
// entry when the run record does not have one. The name is distinct from
// ledger/events.jsonl so a run record carrying its own events ledger still
// exports verbatim; any run entry occupying the sidecar path (file or
// directory) or a non-directory entry occupying the ledger/ path component is
// a collision the export refuses to shadow — shadowing would emit an archive
// holding a file and a directory at the same path.
func appendEventsSidecar(entries []exportEntry, archiveRoot string, events []byte) ([]exportEntry, error) {
	sidecarName := archiveRoot + "ledger/events.filtered.jsonl"
	ledgerDir := archiveRoot + "ledger/"
	hasLedgerDir := false
	for _, entry := range entries {
		if entry.name == sidecarName || entry.name == sidecarName+"/" {
			return nil, fmt.Errorf("run record already contains ledger/events.filtered.jsonl; cannot add the filtered events sidecar")
		}
		if entry.name == strings.TrimSuffix(ledgerDir, "/") {
			return nil, fmt.Errorf("run record contains a non-directory ledger entry; cannot add the filtered events sidecar")
		}
		if entry.name == ledgerDir {
			hasLedgerDir = true
		}
	}
	if !hasLedgerDir {
		entries = append(entries, exportEntry{name: ledgerDir, dir: true})
	}
	return append(entries, exportEntry{name: sidecarName, content: events}), nil
}

// writeExportArchive stages the ZIP as a hidden temporary sibling of output
// and renames it into place only after a fully successful write, so a failed
// export never leaves a partial archive behind. Entry metadata is normalized
// (fixed timestamp, 0644 files, 0755 directories, no owner/group) so the
// archive bytes depend only on the exported contents.
func writeExportArchive(output string, entries []exportEntry) (int64, error) {
	temp, err := os.CreateTemp(filepath.Dir(output), ".pactum-export-*.zip")
	if err != nil {
		return 0, err
	}
	// No-op after the final rename; otherwise cleans up the partial archive.
	defer os.Remove(temp.Name())
	defer temp.Close()

	writer := zip.NewWriter(temp)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Modified: exportEntryEpoch}
		if entry.dir {
			header.SetMode(fs.ModeDir | 0o755)
			if _, err := writer.CreateHeader(header); err != nil {
				return 0, err
			}
			continue
		}
		header.Method = zip.Deflate
		header.SetMode(0o644)
		target, err := writer.CreateHeader(header)
		if err != nil {
			return 0, err
		}
		if err := writeExportEntryBody(target, entry); err != nil {
			return 0, err
		}
	}
	if err := writer.Close(); err != nil {
		return 0, err
	}
	if err := temp.Close(); err != nil {
		return 0, err
	}
	// CreateTemp opens the staging file 0600; widen to the 0644 the archive
	// would get from a regular create, masked by the user's umask so an export
	// is no more readable than any other file the user writes.
	if err := os.Chmod(temp.Name(), 0o644&^fs.FileMode(processUmask())); err != nil {
		return 0, err
	}
	info, err := os.Stat(temp.Name())
	if err != nil {
		return 0, err
	}
	if err := os.Rename(temp.Name(), output); err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func writeExportEntryBody(target io.Writer, entry exportEntry) error {
	if entry.source == "" {
		_, err := target.Write(entry.content)
		return err
	}
	file, err := os.Open(entry.source)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(target, file)
	return err
}

func writeExport(stdout io.Writer, response exportResponse) {
	fmt.Fprintln(stdout, "Pactum export")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Archive:")
	fmt.Fprintf(stdout, "  output: %s\n", response.Output)
	fmt.Fprintf(stdout, "  format: %s\n", response.ArchiveFormat)
	fmt.Fprintf(stdout, "  root: %s\n", response.ArchiveRoot)
	fmt.Fprintf(stdout, "  entries: %d\n", response.Entries)
	fmt.Fprintf(stdout, "  bytes: %d\n", response.Bytes)
	fmt.Fprintf(stdout, "  filtered events: %d\n", response.FilteredEvents)
}
