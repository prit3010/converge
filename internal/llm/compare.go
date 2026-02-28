package llm

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	openai "github.com/sashabaranov/go-openai"

	"github.com/prittamravi/converge/internal/db"
	"github.com/prittamravi/converge/internal/diff"
	"github.com/prittamravi/converge/internal/snapshot"
	"github.com/prittamravi/converge/internal/store"
)

const (
	defaultCompareModel    = "gpt-4o-mini"
	defaultMaxDiffLines    = 800
	defaultMaxDiffContext  = 120
	fallbackMaxDiffContext = 30
)

type CompareOptions struct {
	Model        string
	MaxDiffLines int
}

type CompareResult struct {
	Summary    string   `json:"summary"`
	Winner     string   `json:"winner"`
	Highlights []string `json:"highlights"`
	Error      string   `json:"error,omitempty"`
}

type Comparer struct {
	db    *db.DB
	store *store.Store
}

func NewComparer(database *db.DB, objectStore *store.Store) *Comparer {
	return &Comparer{db: database, store: objectStore}
}

func (c *Comparer) Compare(ctx context.Context, cellAID, cellBID string, opts CompareOptions) (*CompareResult, error) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		res := &CompareResult{Error: "OPENAI_API_KEY is not set"}
		return res, fmt.Errorf(res.Error)
	}

	cellA, err := c.db.GetCell(cellAID)
	if err != nil {
		return nil, fmt.Errorf("cell %s not found", cellAID)
	}
	cellB, err := c.db.GetCell(cellBID)
	if err != nil {
		return nil, fmt.Errorf("cell %s not found", cellBID)
	}

	prompt, err := c.buildPrompt(cellA, cellB, opts)
	if err != nil {
		return nil, fmt.Errorf("build compare prompt: %w", err)
	}

	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = defaultCompareModel
	}

	client := openai.NewClient(apiKey)
	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: "You are a precise code-change reviewer. Compare Cell A to Cell B only. In unified diffs, '-' lines belong to Cell A and '+' lines belong to Cell B. Do not reverse this direction and do not infer edits that are not shown. Return EXACT format: SUMMARY: <2-3 sentences>\nWINNER: <cell_id or tie> - <one sentence why>\nHIGHLIGHTS:\n- <bullet>\n- <bullet>\n- <bullet>",
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		Temperature: 0.2,
	})
	if err != nil {
		res := &CompareResult{Error: err.Error()}
		return res, fmt.Errorf("openai compare call: %w", err)
	}
	if len(resp.Choices) == 0 {
		res := &CompareResult{Error: "model returned no choices"}
		return res, fmt.Errorf(res.Error)
	}

	return parseCompareResponse(resp.Choices[0].Message.Content), nil
}

func (c *Comparer) buildPrompt(cellA, cellB *db.Cell, opts CompareOptions) (string, error) {
	manifestA, err := c.db.GetManifest(cellA.ID)
	if err != nil {
		return "", err
	}
	manifestB, err := c.db.GetManifest(cellB.ID)
	if err != nil {
		return "", err
	}

	mapA := make(map[string]string, len(manifestA))
	for _, e := range manifestA {
		mapA[e.Path] = e.Hash
	}
	mapB := make(map[string]string, len(manifestB))
	for _, e := range manifestB {
		mapB[e.Path] = e.Hash
	}

	result := diff.CompareManifests(mapA, mapB)
	maxDiffLines := opts.MaxDiffLines
	if maxDiffLines <= 0 {
		maxDiffLines = defaultMaxDiffLines
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Cell A: %s (branch=%s, msg=%q, loc=%d, files=%d)\n", cellA.ID, cellA.Branch, cellA.Message, cellA.TotalLOC, cellA.TotalFiles)
	fmt.Fprintf(&sb, "Cell B: %s (branch=%s, msg=%q, loc=%d, files=%d)\n", cellB.ID, cellB.Branch, cellB.Message, cellB.TotalLOC, cellB.TotalFiles)
	sb.WriteString("Diff direction: A -> B. In each patch, '-' lines are from Cell A and '+' lines are from Cell B.\n")
	fmt.Fprintf(&sb, "High-level counts: +%d added, ~%d modified, -%d removed\n\n", len(result.Added), len(result.Modified), len(result.Removed))

	if len(result.Added) > 0 {
		sorted := append([]string(nil), result.Added...)
		sort.Strings(sorted)
		fmt.Fprintf(&sb, "Added files: %s\n", strings.Join(sorted, ", "))
	}
	if len(result.Removed) > 0 {
		sorted := append([]string(nil), result.Removed...)
		sort.Strings(sorted)
		fmt.Fprintf(&sb, "Removed files: %s\n", strings.Join(sorted, ", "))
	}
	if len(result.Added) > 0 || len(result.Removed) > 0 {
		sb.WriteString("\n")
	}

	remainingDiffLines := maxDiffLines
	for _, path := range result.Modified {
		if remainingDiffLines <= 0 {
			break
		}
		oldData, errOld := c.store.Read(mapA[path])
		newData, errNew := c.store.Read(mapB[path])
		if errOld != nil || errNew != nil {
			continue
		}
		if !snapshot.IsText(oldData) || !snapshot.IsText(newData) {
			fmt.Fprintf(&sb, "### %s (binary diff skipped)\n\n", path)
			continue
		}

		patch := diff.ExpandedUnifiedDiff(path, string(oldData), string(newData), defaultMaxDiffContext)
		lines := splitNonEmptyLines(patch)
		if len(lines) > remainingDiffLines {
			patch = diff.ExpandedUnifiedDiff(path, string(oldData), string(newData), fallbackMaxDiffContext)
			lines = splitNonEmptyLines(patch)
		}
		if len(lines) > remainingDiffLines {
			fmt.Fprintf(&sb, "### %s (diff omitted due to max-diff-lines limit)\n\n", path)
			continue
		}
		fmt.Fprintf(&sb, "### %s\n%s\n", path, patch)
		remainingDiffLines -= len(lines)
	}

	if remainingDiffLines > 0 {
		for _, path := range result.Added {
			if remainingDiffLines <= 0 {
				break
			}
			data, err := c.store.Read(mapB[path])
			if err != nil || !snapshot.IsText(data) {
				continue
			}
			contentLines := splitNonEmptyLines(string(data))
			if len(contentLines) > remainingDiffLines {
				continue
			}
			fmt.Fprintf(&sb, "### %s (new file)\n%s\n", path, string(data))
			remainingDiffLines -= len(contentLines)
		}
	}

	return sb.String(), nil
}

func splitNonEmptyLines(in string) []string {
	lines := strings.Split(in, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func parseCompareResponse(content string) *CompareResult {
	result := &CompareResult{}
	lines := strings.Split(content, "\n")
	section := ""
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "SUMMARY:"):
			result.Summary = strings.TrimSpace(strings.TrimPrefix(line, "SUMMARY:"))
			section = "summary"
		case strings.HasPrefix(line, "WINNER:"):
			result.Winner = strings.TrimSpace(strings.TrimPrefix(line, "WINNER:"))
			section = "winner"
		case strings.HasPrefix(line, "HIGHLIGHTS:"):
			section = "highlights"
		case section == "summary":
			if result.Summary == "" {
				result.Summary = line
			} else {
				result.Summary += " " + line
			}
		case section == "highlights" && strings.HasPrefix(line, "-"):
			highlight := strings.TrimSpace(strings.TrimPrefix(line, "-"))
			if highlight != "" {
				result.Highlights = append(result.Highlights, highlight)
			}
		}
	}

	if result.Summary == "" {
		trimmed := strings.TrimSpace(content)
		result.Summary = trimmed
	}

	return result
}
