package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
)

const (
	memorySelectionSchema       = "pactum.memory_selection.v1alpha1"
	defaultMemorySelectionLimit = 5
)

var memorySelectionStopwords = map[string]struct{}{
	"the":       {},
	"and":       {},
	"for":       {},
	"with":      {},
	"from":      {},
	"into":      {},
	"this":      {},
	"that":      {},
	"should":    {},
	"would":     {},
	"could":     {},
	"add":       {},
	"update":    {},
	"create":    {},
	"remove":    {},
	"implement": {},
}

type memorySelectionDocument struct {
	Schema      string               `json:"schema"`
	RunID       string               `json:"run_id"`
	CreatedAt   string               `json:"created_at"`
	Query       string               `json:"query"`
	QuerySource string               `json:"query_source"`
	Limit       int                  `json:"limit"`
	Selected    []memorySelectedItem `json:"selected"`
	Notes       []string             `json:"notes"`
}

type memorySelectedItem struct {
	ID         string                 `json:"id"`
	Score      int                    `json:"score"`
	Title      string                 `json:"title"`
	Summary    string                 `json:"summary"`
	Files      []string               `json:"files"`
	Tags       []string               `json:"tags"`
	Candidate  string                 `json:"candidate"`
	AcceptedAt string                 `json:"accepted_at"`
	Freshness  memoryFreshnessSummary `json:"freshness"`
}

type memorySearchResponse struct {
	Query    string               `json:"query"`
	Limit    int                  `json:"limit"`
	Selected []memorySelectedItem `json:"selected"`
}

type scoredMemoryItem struct {
	record memoryItemRecord
	score  int
}

func (a App) MemorySearch(stdout io.Writer, query string, limit int, jsonOutput bool) error {
	_, paths, ok, err := a.requireWorkspace(stdout, false)
	if err != nil || !ok {
		return err
	}

	limit = normalizeMemorySelectionLimit(limit)
	selected, _, err := selectAcceptedMemoryItems(paths, query, limit)
	if err != nil {
		return err
	}
	response := memorySearchResponse{
		Query:    query,
		Limit:    limit,
		Selected: selected,
	}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeMemorySearch(stdout, response)
	return nil
}

func buildAcceptedMemorySelection(paths artifacts.Paths, runID string, query string, querySource string, limit int, createdAt string) (memorySelectionDocument, error) {
	limit = normalizeMemorySelectionLimit(limit)
	selected, noUsefulTokens, err := selectAcceptedMemoryItems(paths, query, limit)
	if err != nil {
		return memorySelectionDocument{}, err
	}
	notes := []string{
		"Selection is deterministic lexical matching over accepted memory items.",
		"Memory is context, not semantic truth.",
	}
	if noUsefulTokens {
		notes = append(notes, "No useful memory query tokens were available.")
	}
	return memorySelectionDocument{
		Schema:      memorySelectionSchema,
		RunID:       runID,
		CreatedAt:   createdAt,
		Query:       sanitizeMemoryText(paths.Root, query),
		QuerySource: querySource,
		Limit:       limit,
		Selected:    selected,
		Notes:       notes,
	}, nil
}

func writeAcceptedMemoryContext(paths artifacts.Paths, runPaths contractRunPathSet, runID string, query string, querySource string, limit int, createdAt time.Time) (memorySelectionDocument, error) {
	selection, err := buildAcceptedMemorySelection(paths, runID, query, querySource, limit, createdAt.Format(time.RFC3339))
	if err != nil {
		return memorySelectionDocument{}, err
	}
	if err := writeJSON(runPaths.MemorySelectionJSON, selection); err != nil {
		return memorySelectionDocument{}, err
	}
	if err := activeStore.WriteBytes(runPaths.MemoryContextMD, []byte(renderMemoryContextMD(selection)), 0o644); err != nil {
		return memorySelectionDocument{}, err
	}
	return selection, nil
}

// memorySelectionCounts summarizes selected memory items by freshness status.
func memorySelectionCounts(selection memorySelectionDocument) promptManifestMemorySelected {
	counts := promptManifestMemorySelected{Total: len(selection.Selected)}
	for _, item := range selection.Selected {
		switch normalizeMemoryFreshnessStatus(item.Freshness.Status) {
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

// memorySourceSHA256 returns a deterministic hash over the global accepted
// memory source files. Missing and empty files hash to distinct, stable values
// so execution can detect any change to accepted memory after prompt build.
func memorySourceSHA256(paths artifacts.Paths) (string, error) {
	hasher := sha256.New()
	for _, source := range []struct {
		label string
		path  string
	}{
		{label: "items.jsonl", path: paths.MemoryItems},
		{label: "refreshes.jsonl", path: paths.MemoryRefreshes},
	} {
		data, err := activeStore.ReadBytes(source.path)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(hasher, "%s\x00missing\x00", source.label)
				continue
			}
			return "", err
		}
		fmt.Fprintf(hasher, "%s\x00present\x00%d\x00", source.label, len(data))
		hasher.Write(data)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func selectAcceptedMemoryItems(paths artifacts.Paths, query string, limit int) ([]memorySelectedItem, bool, error) {
	limit = normalizeMemorySelectionLimit(limit)
	queryTokens := memoryTokenSet(query)
	if len(queryTokens) == 0 {
		return []memorySelectedItem{}, true, nil
	}
	items, err := readMemoryItems(paths.MemoryItems)
	if err != nil {
		return nil, false, err
	}
	freshnessByID, err := readLatestMemoryFreshness(paths, items)
	if err != nil {
		return nil, false, err
	}
	scored := make([]scoredMemoryItem, 0, len(items))
	for _, item := range items {
		score := scoreMemoryItem(queryTokens, item)
		freshness := effectiveMemoryFreshnessForItem(item, freshnessByID)
		if freshness.Status == memoryFreshnessStale {
			score -= 2
		}
		if score <= 0 {
			continue
		}
		scored = append(scored, scoredMemoryItem{record: item, score: score})
	}
	sort.Slice(scored, func(i, j int) bool {
		left := scored[i]
		right := scored[j]
		if left.score != right.score {
			return left.score > right.score
		}
		if left.record.AcceptedAt != right.record.AcceptedAt {
			return left.record.AcceptedAt > right.record.AcceptedAt
		}
		return left.record.ID < right.record.ID
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	selected := make([]memorySelectedItem, 0, len(scored))
	for _, item := range scored {
		selected = append(selected, memorySelectedItemFromRecord(paths.Root, item.record, item.score, effectiveMemoryFreshnessForItem(item.record, freshnessByID)))
	}
	return selected, false, nil
}

func scoreMemoryItem(queryTokens map[string]struct{}, item memoryItemRecord) int {
	score := 0
	score += scoreTokenOverlap(queryTokens, memoryTokenSet(item.Title), 4)
	score += scoreTokenOverlap(queryTokens, memoryTokenSet(item.Summary), 3)
	score += scoreTokenOverlap(queryTokens, memoryTokensForValues(item.Tags), 2)
	score += scoreTokenOverlap(queryTokens, memoryTokensForValues(item.Files), 1)
	return score
}

func scoreTokenOverlap(queryTokens map[string]struct{}, fieldTokens map[string]struct{}, weight int) int {
	score := 0
	for token := range queryTokens {
		if _, ok := fieldTokens[token]; ok {
			score += weight
		}
	}
	return score
}

func memoryTokensForValues(values []string) map[string]struct{} {
	tokens := map[string]struct{}{}
	for _, value := range values {
		for token := range memoryTokenSet(value) {
			tokens[token] = struct{}{}
		}
	}
	return tokens
}

func memoryTokenSet(value string) map[string]struct{} {
	tokens := map[string]struct{}{}
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	}) {
		if len(token) < 3 {
			continue
		}
		if _, stop := memorySelectionStopwords[token]; stop {
			continue
		}
		tokens[token] = struct{}{}
	}
	return tokens
}

func memorySelectedItemFromRecord(root string, item memoryItemRecord, score int, freshness memoryEffectiveFreshness) memorySelectedItem {
	return memorySelectedItem{
		ID:         item.ID,
		Score:      score,
		Title:      sanitizeMemoryText(root, item.Title),
		Summary:    sanitizeMemoryText(root, item.Summary),
		Files:      sanitizeSelectedMemoryPaths(root, item.Files),
		Tags:       sanitizeMemoryTexts(root, item.Tags),
		Candidate:  sanitizeSelectedMemoryPath(root, item.Candidate),
		AcceptedAt: item.AcceptedAt,
		Freshness: memoryFreshnessSummary{
			Status:  freshness.Status,
			Reasons: sanitizeMemoryTexts(root, freshness.Reasons),
		},
	}
}

func sanitizeSelectedMemoryPaths(root string, values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		path := sanitizeSelectedMemoryPath(root, value)
		if path == "" {
			continue
		}
		result = append(result, path)
	}
	return result
}

func sanitizeSelectedMemoryPath(root string, value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		if rel, err := filepath.Rel(root, value); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	value = sanitizeRepoRootInText(root, value)
	value = strings.TrimPrefix(value, "./")
	return value
}

func normalizeMemorySelectionLimit(limit int) int {
	if limit <= 0 {
		return defaultMemorySelectionLimit
	}
	return limit
}

func memoryQueryFromContract(contract draftContract) string {
	parts := []string{contract.Goal}
	parts = append(parts, contract.Scope.In...)
	parts = append(parts, contract.AcceptanceCriteria...)
	parts = append(parts, contract.Validation.Commands...)
	return strings.Join(parts, "\n")
}

func renderMemoryContextMD(selection memorySelectionDocument) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Accepted Memory Context")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Selection")
	fmt.Fprintf(&b, "- Query source: %s\n", selection.QuerySource)
	fmt.Fprintf(&b, "- Query: %s\n", compactMemoryContextText(selection.Query))
	fmt.Fprintf(&b, "- Limit: %d\n", selection.Limit)
	fmt.Fprintf(&b, "- Selected items: %d\n", len(selection.Selected))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Guidance")
	fmt.Fprintln(&b, "- Accepted memory is context, not semantic truth.")
	fmt.Fprintln(&b, "- Verify against repo map, search, and source files before using it.")
	fmt.Fprintln(&b, "- Do not treat memory as a substitute for current code.")
	fmt.Fprintln(&b, "- Stale memory may be outdated. Verify before using.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Selected memory items")
	fmt.Fprintln(&b)
	if len(selection.Selected) == 0 {
		fmt.Fprintln(&b, "No accepted memory items matched this run.")
		return b.String()
	}
	for i, item := range selection.Selected {
		if i > 0 {
			fmt.Fprintln(&b)
		}
		fmt.Fprintf(&b, "### %s - %s\n", item.ID, compactMemoryContextText(item.Title))
		fmt.Fprintf(&b, "- Score: %d\n", item.Score)
		fmt.Fprintf(&b, "- Freshness: %s\n", item.Freshness.Status)
		fmt.Fprintf(&b, "- Reasons: %s\n", memoryContextListValue(item.Freshness.Reasons))
		fmt.Fprintf(&b, "- Summary: %s\n", compactMemoryContextText(item.Summary))
		fmt.Fprintf(&b, "- Files: %s\n", memoryContextListValue(item.Files))
		fmt.Fprintf(&b, "- Tags: %s\n", memoryContextListValue(item.Tags))
		fmt.Fprintf(&b, "- Candidate: %s\n", valueOrNone(item.Candidate))
	}
	return b.String()
}

func compactMemoryContextText(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return "none"
	}
	return value
}

func memoryContextListValue(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func writeMemorySearch(stdout io.Writer, response memorySearchResponse) {
	fmt.Fprintln(stdout, "Memory search")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Query:")
	fmt.Fprintf(stdout, "  %s\n", response.Query)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Results:")
	if len(response.Selected) == 0 {
		fmt.Fprintln(stdout, "  No accepted memory items matched.")
		return
	}
	for index, item := range response.Selected {
		fmt.Fprintf(stdout, "  %d. %s score=%d\n", index+1, item.ID, item.Score)
		fmt.Fprintf(stdout, "     title: %s\n", item.Title)
		fmt.Fprintf(stdout, "     freshness: %s\n", item.Freshness.Status)
		if len(item.Freshness.Reasons) > 0 {
			fmt.Fprintf(stdout, "     reasons: %s\n", memoryContextListValue(item.Freshness.Reasons))
		}
		fmt.Fprintf(stdout, "     summary: %s\n", item.Summary)
		fmt.Fprintf(stdout, "     files: %s\n", memoryContextListValue(item.Files))
	}
}
