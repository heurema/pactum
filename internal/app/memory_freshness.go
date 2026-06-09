package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/ledger"
)

const (
	memoryRefreshSchema = "pactum.memory_refresh.v1"

	memoryFreshnessFresh   = "fresh"
	memoryFreshnessStale   = "stale"
	memoryFreshnessUnknown = "unknown"

	memoryFileUnchanged = "unchanged"
	memoryFileChanged   = "changed"
	memoryFileMissing   = "missing"
	memoryFileUnknown   = "unknown"
)

type memoryItemFreshness struct {
	Status    string                `json:"status"`
	CheckedAt string                `json:"checked_at"`
	Reasons   []string              `json:"reasons"`
	Files     []memoryFreshnessFile `json:"files"`
}

type memoryFreshnessFile struct {
	Path           string `json:"path"`
	Status         string `json:"status"`
	AcceptedSHA256 string `json:"accepted_sha256,omitempty"`
	CurrentSHA256  string `json:"current_sha256,omitempty"`
}

type memoryRefreshRecord struct {
	Schema    string              `json:"schema"`
	ID        string              `json:"id"`
	CreatedAt string              `json:"created_at"`
	Items     []memoryRefreshItem `json:"items"`
}

type memoryRefreshItem struct {
	MemoryItemID string                `json:"memory_item_id"`
	Status       string                `json:"status"`
	Reasons      []string              `json:"reasons"`
	Files        []memoryFreshnessFile `json:"files"`
}

type memoryEffectiveFreshness struct {
	Status  string                `json:"status"`
	Reasons []string              `json:"reasons"`
	Files   []memoryFreshnessFile `json:"files,omitempty"`
}

type memoryFreshnessSummary struct {
	Status  string   `json:"status"`
	Reasons []string `json:"reasons"`
}

type memoryStaleResponse struct {
	Total   int                   `json:"total"`
	Fresh   int                   `json:"fresh"`
	Stale   int                   `json:"stale"`
	Unknown int                   `json:"unknown"`
	Items   []memoryStaleItemView `json:"items"`
}

type memoryStaleItemView struct {
	ID        string                 `json:"id"`
	Title     string                 `json:"title"`
	Freshness memoryFreshnessSummary `json:"freshness"`
	Files     []memoryFreshnessFile  `json:"files,omitempty"`
}

type memoryFreshnessCounts struct {
	Total   int
	Fresh   int
	Stale   int
	Unknown int
}

func (a App) MemoryRefresh(stdout io.Writer, jsonOutput bool) error {
	root, paths, ok, err := a.requireWorkspace(stdout, jsonOutput)
	if err != nil || !ok {
		return err
	}

	items, err := readMemoryItems(paths.MemoryItems)
	if err != nil {
		return err
	}
	refreshes, err := readMemoryRefreshes(paths.MemoryRefreshes)
	if err != nil {
		return err
	}
	now := a.nowUTC()
	refresh := memoryRefreshRecord{
		Schema:    memoryRefreshSchema,
		ID:        nextMemoryRefreshID(len(refreshes) + 1),
		CreatedAt: now.Format(time.RFC3339),
		Items:     computeMemoryRefreshItems(root, items, now.Format(time.RFC3339)),
	}
	if err := appendJSONLine(paths.MemoryRefreshes, refresh); err != nil {
		return err
	}
	if err := activeStore.WriteBytes(paths.ProjectMemory, []byte(renderProjectMemoryMD(root, items, effectiveMemoryFreshnessFromRefresh(refresh))), 0o644); err != nil {
		return err
	}
	if err := ledger.Append(activeStore, paths.EventsJSONL, ledger.Event{Type: "memory_refresh_completed", Timestamp: now}); err != nil {
		return err
	}
	if jsonOutput {
		return writeJSONResponse(stdout, refresh)
	}
	writeMemoryRefresh(stdout, paths, refresh)
	return nil
}

func (a App) MemoryStale(stdout io.Writer, jsonOutput bool) error {
	root, paths, ok, err := a.requireWorkspace(stdout, jsonOutput)
	if err != nil || !ok {
		return err
	}

	items, err := readMemoryItems(paths.MemoryItems)
	if err != nil {
		return err
	}
	freshnessByID, err := readLatestMemoryFreshnessOrComputeInline(paths, items)
	if err != nil {
		return err
	}
	response := buildMemoryStaleResponse(root, items, freshnessByID)
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeMemoryStale(stdout, response)
	return nil
}

func buildAcceptedMemoryItemFreshness(root string, files []string, checkedAt string) *memoryItemFreshness {
	return memoryItemFreshnessFromFiles(root, files, nil, checkedAt)
}

func computeMemoryRefreshItems(root string, items []memoryItemRecord, checkedAt string) []memoryRefreshItem {
	result := make([]memoryRefreshItem, 0, len(items))
	for _, item := range items {
		accepted := acceptedMemoryHashes(item)
		freshness := memoryItemFreshnessFromFiles(root, item.Files, accepted, checkedAt)
		result = append(result, memoryRefreshItem{
			MemoryItemID: item.ID,
			Status:       freshness.Status,
			Reasons:      append([]string{}, freshness.Reasons...),
			Files:        cloneMemoryFreshnessFiles(freshness.Files),
		})
	}
	return result
}

func memoryItemFreshnessFromFiles(root string, files []string, acceptedHashes map[string]string, checkedAt string) *memoryItemFreshness {
	freshness := &memoryItemFreshness{
		Status:    memoryFreshnessFresh,
		CheckedAt: checkedAt,
		Reasons:   []string{},
		Files:     []memoryFreshnessFile{},
	}
	if len(files) == 0 {
		freshness.Status = memoryFreshnessUnknown
		freshness.Reasons = append(freshness.Reasons, "No tracked files")
		return freshness
	}

	hasStale := false
	hasUnknown := false
	for _, raw := range files {
		file := memoryFreshnessForFile(root, raw, acceptedHashes)
		freshness.Files = append(freshness.Files, file)
		switch file.Status {
		case memoryFileMissing:
			hasStale = true
			freshness.Reasons = append(freshness.Reasons, "missing file "+file.Path)
		case memoryFileChanged:
			hasStale = true
			freshness.Reasons = append(freshness.Reasons, "changed file "+file.Path)
		case memoryFileUnknown:
			hasUnknown = true
			freshness.Reasons = append(freshness.Reasons, "unknown file "+file.Path)
		}
	}
	freshness.Reasons = uniqueSortedStrings(freshness.Reasons)
	switch {
	case hasStale:
		freshness.Status = memoryFreshnessStale
	case hasUnknown:
		freshness.Status = memoryFreshnessUnknown
	default:
		freshness.Status = memoryFreshnessFresh
	}
	return freshness
}

func memoryFreshnessForFile(root string, rawPath string, acceptedHashes map[string]string) memoryFreshnessFile {
	path, fullPath, ok := normalizeMemoryFreshnessPath(root, rawPath)
	if !ok {
		return memoryFreshnessFile{Path: sanitizeSelectedMemoryPath(root, rawPath), Status: memoryFileUnknown}
	}
	file := memoryFreshnessFile{Path: path}
	if acceptedHashes != nil {
		file.AcceptedSHA256 = acceptedHashes[path]
		if file.AcceptedSHA256 == "" {
			file.Status = memoryFileUnknown
			return file
		}
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			file.Status = memoryFileMissing
			return file
		}
		file.Status = memoryFileUnknown
		return file
	}
	if !info.Mode().IsRegular() {
		file.Status = memoryFileUnknown
		return file
	}
	hash, err := fileSHA256(fullPath)
	if err != nil {
		file.Status = memoryFileUnknown
		return file
	}
	file.CurrentSHA256 = hash
	if acceptedHashes == nil {
		file.AcceptedSHA256 = hash
		file.Status = memoryFileUnchanged
		return file
	}
	if hash == file.AcceptedSHA256 {
		file.Status = memoryFileUnchanged
	} else {
		file.Status = memoryFileChanged
	}
	return file
}

func normalizeMemoryFreshnessPath(root string, value string) (string, string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || isWindowsDriveAbsolutePath(value) {
		return "", "", false
	}
	if filepath.IsAbs(value) {
		rel, err := filepath.Rel(root, value)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return "", "", false
		}
		value = rel
	}
	value = filepath.ToSlash(filepath.Clean(value))
	if value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return "", "", false
	}
	fullPath := filepath.Join(root, filepath.FromSlash(value))
	rel, err := filepath.Rel(root, fullPath)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", "", false
	}
	return filepath.ToSlash(rel), fullPath, true
}

func isWindowsDriveAbsolutePath(value string) bool {
	if len(value) < 3 || value[1] != ':' || (value[2] != '\\' && value[2] != '/') {
		return false
	}
	ch := value[0]
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func acceptedMemoryHashes(item memoryItemRecord) map[string]string {
	hashes := map[string]string{}
	if item.Freshness == nil {
		return hashes
	}
	for _, file := range item.Freshness.Files {
		if file.Path == "" || file.AcceptedSHA256 == "" {
			continue
		}
		hashes[filepath.ToSlash(file.Path)] = file.AcceptedSHA256
	}
	return hashes
}

func readMemoryRefreshes(path string) ([]memoryRefreshRecord, error) {
	return readJSONLines[memoryRefreshRecord](path)
}

func latestMemoryRefresh(path string) (memoryRefreshRecord, bool, error) {
	refreshes, err := readMemoryRefreshes(path)
	if err != nil {
		return memoryRefreshRecord{}, false, err
	}
	if len(refreshes) == 0 {
		return memoryRefreshRecord{}, false, nil
	}
	return refreshes[len(refreshes)-1], true, nil
}

func readLatestMemoryFreshness(paths artifacts.Paths, items []memoryItemRecord) (map[string]memoryEffectiveFreshness, error) {
	refresh, ok, err := latestMemoryRefresh(paths.MemoryRefreshes)
	if err != nil {
		return nil, err
	}
	if ok {
		return effectiveMemoryFreshnessFromRefresh(refresh), nil
	}
	return effectiveMemoryFreshnessFromItems(items), nil
}

func readLatestMemoryFreshnessOrComputeInline(paths artifacts.Paths, items []memoryItemRecord) (map[string]memoryEffectiveFreshness, error) {
	refresh, ok, err := latestMemoryRefresh(paths.MemoryRefreshes)
	if err != nil {
		return nil, err
	}
	if ok {
		return effectiveMemoryFreshnessFromRefresh(refresh), nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return effectiveMemoryFreshnessFromRefresh(memoryRefreshRecord{Items: computeMemoryRefreshItems(paths.Root, items, now)}), nil
}

func effectiveMemoryFreshnessFromItems(items []memoryItemRecord) map[string]memoryEffectiveFreshness {
	result := map[string]memoryEffectiveFreshness{}
	for _, item := range items {
		if item.Freshness == nil {
			result[item.ID] = unknownMemoryFreshness()
			continue
		}
		result[item.ID] = memoryEffectiveFreshness{
			Status:  normalizeMemoryFreshnessStatus(item.Freshness.Status),
			Reasons: append([]string{}, item.Freshness.Reasons...),
			Files:   cloneMemoryFreshnessFiles(item.Freshness.Files),
		}
	}
	return result
}

func effectiveMemoryFreshnessFromRefresh(refresh memoryRefreshRecord) map[string]memoryEffectiveFreshness {
	result := map[string]memoryEffectiveFreshness{}
	for _, item := range refresh.Items {
		result[item.MemoryItemID] = memoryEffectiveFreshness{
			Status:  normalizeMemoryFreshnessStatus(item.Status),
			Reasons: append([]string{}, item.Reasons...),
			Files:   cloneMemoryFreshnessFiles(item.Files),
		}
	}
	return result
}

func effectiveMemoryFreshnessForItem(item memoryItemRecord, freshnessByID map[string]memoryEffectiveFreshness) memoryEffectiveFreshness {
	if freshnessByID != nil {
		if freshness, ok := freshnessByID[item.ID]; ok {
			return normalizeEffectiveMemoryFreshness(freshness)
		}
	}
	if item.Freshness != nil {
		return normalizeEffectiveMemoryFreshness(memoryEffectiveFreshness{
			Status:  item.Freshness.Status,
			Reasons: item.Freshness.Reasons,
			Files:   item.Freshness.Files,
		})
	}
	return unknownMemoryFreshness()
}

func unknownMemoryFreshness() memoryEffectiveFreshness {
	return memoryEffectiveFreshness{Status: memoryFreshnessUnknown, Reasons: []string{}, Files: []memoryFreshnessFile{}}
}

func normalizeEffectiveMemoryFreshness(freshness memoryEffectiveFreshness) memoryEffectiveFreshness {
	freshness.Status = normalizeMemoryFreshnessStatus(freshness.Status)
	freshness.Reasons = append([]string{}, freshness.Reasons...)
	freshness.Files = cloneMemoryFreshnessFiles(freshness.Files)
	return freshness
}

func normalizeMemoryFreshnessStatus(status string) string {
	switch status {
	case memoryFreshnessFresh, memoryFreshnessStale, memoryFreshnessUnknown:
		return status
	default:
		return memoryFreshnessUnknown
	}
}

func cloneMemoryFreshnessFiles(files []memoryFreshnessFile) []memoryFreshnessFile {
	result := make([]memoryFreshnessFile, len(files))
	copy(result, files)
	return result
}

func buildMemoryStaleResponse(root string, items []memoryItemRecord, freshnessByID map[string]memoryEffectiveFreshness) memoryStaleResponse {
	response := memoryStaleResponse{Items: []memoryStaleItemView{}}
	for _, item := range items {
		freshness := effectiveMemoryFreshnessForItem(item, freshnessByID)
		response.Total++
		switch freshness.Status {
		case memoryFreshnessFresh:
			response.Fresh++
		case memoryFreshnessStale:
			response.Stale++
		default:
			response.Unknown++
		}
		if freshness.Status == memoryFreshnessFresh {
			continue
		}
		response.Items = append(response.Items, memoryStaleItemView{
			ID:    item.ID,
			Title: sanitizeMemoryText(root, item.Title),
			Freshness: memoryFreshnessSummary{
				Status:  freshness.Status,
				Reasons: append([]string{}, freshness.Reasons...),
			},
			Files: cloneMemoryFreshnessFiles(freshness.Files),
		})
	}
	sort.Slice(response.Items, func(i, j int) bool {
		if response.Items[i].Freshness.Status != response.Items[j].Freshness.Status {
			return response.Items[i].Freshness.Status == memoryFreshnessStale
		}
		return response.Items[i].ID < response.Items[j].ID
	})
	return response
}

func memoryRefreshCounts(refresh memoryRefreshRecord) memoryFreshnessCounts {
	counts := memoryFreshnessCounts{Total: len(refresh.Items)}
	for _, item := range refresh.Items {
		switch normalizeMemoryFreshnessStatus(item.Status) {
		case memoryFreshnessFresh:
			counts.Fresh++
		case memoryFreshnessStale:
			counts.Stale++
		default:
			counts.Unknown++
		}
	}
	return counts
}

func memoryStaleCounts(response memoryStaleResponse) memoryFreshnessCounts {
	return memoryFreshnessCounts{
		Total:   response.Total,
		Fresh:   response.Fresh,
		Stale:   response.Stale,
		Unknown: response.Unknown,
	}
}

func nextMemoryRefreshID(index int) string {
	return fmt.Sprintf("memory_refresh_%03d", index)
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func writeMemoryRefresh(stdout io.Writer, paths artifacts.Paths, refresh memoryRefreshRecord) {
	counts := memoryRefreshCounts(refresh)
	fmt.Fprintln(stdout, "Memory refresh completed")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Items:")
	fmt.Fprintf(stdout, "  total: %d\n", counts.Total)
	fmt.Fprintf(stdout, "  fresh: %d\n", counts.Fresh)
	fmt.Fprintf(stdout, "  stale: %d\n", counts.Stale)
	fmt.Fprintf(stdout, "  unknown: %d\n", counts.Unknown)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  refresh: %s\n", toRepoRel(paths.Root, paths.MemoryRefreshes))
	fmt.Fprintf(stdout, "  project memory: %s\n", toRepoRel(paths.Root, paths.ProjectMemory))
}

func writeMemoryStale(stdout io.Writer, response memoryStaleResponse) {
	counts := memoryStaleCounts(response)
	fmt.Fprintln(stdout, "Memory freshness")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Items:")
	fmt.Fprintf(stdout, "  total: %d\n", counts.Total)
	fmt.Fprintf(stdout, "  fresh: %d\n", counts.Fresh)
	fmt.Fprintf(stdout, "  stale: %d\n", counts.Stale)
	fmt.Fprintf(stdout, "  unknown: %d\n", counts.Unknown)
	fmt.Fprintln(stdout)
	writeMemoryStaleGroup(stdout, "Stale", response.Items, memoryFreshnessStale)
	fmt.Fprintln(stdout)
	writeMemoryStaleGroup(stdout, "Unknown", response.Items, memoryFreshnessUnknown)
}

func writeMemoryStaleGroup(stdout io.Writer, heading string, items []memoryStaleItemView, status string) {
	fmt.Fprintf(stdout, "%s:\n", heading)
	wrote := false
	for _, item := range items {
		if item.Freshness.Status != status {
			continue
		}
		wrote = true
		fmt.Fprintf(stdout, "  - %s %s\n", item.ID, valueOrNone(item.Title))
		if len(item.Freshness.Reasons) == 0 {
			fmt.Fprintln(stdout, "    reason: none")
		} else {
			for _, reason := range item.Freshness.Reasons {
				fmt.Fprintf(stdout, "    reason: %s\n", reason)
			}
		}
	}
	if !wrote {
		fmt.Fprintln(stdout, "  none")
	}
}
