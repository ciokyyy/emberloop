package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"example.com/emberloop/tools/systemone"
)

// findTool is ported from el's apps/backend/tools/find.go: semantic grep
// through the CascadeSearcher, asking a decisions model through OpenRouter's
// systemone endpoint. With no OpenRouter key it falls back to lexical ranking.
// When the codebase-memory index exists for the project, the code graph
// answers as a third source beside the decisions model and grep.
type findTool struct {
	// provider is a test seam. Nil means: build the hosted one per call, from
	// whichever key is available then.
	provider systemone.Provider
	// lexicalOnly skips the key lookup, for tests that must not touch the network.
	lexicalOnly bool
	// graph is a test seam for the third source. Nil means: read the
	// codebase-memory index when one exists, else degrade to the cascade.
	graph GraphSource
}

func (findTool) Name() string   { return "find" }
func (findTool) ReadOnly() bool { return true }
func (findTool) Description() string {
	return "Semantic grep: locate files and line ranges that implement a concept or behavior. Ranked by a decisions model; falls back to keyword ranking when none is available. Includes code-graph hits (symbols, callers) when the codebase-memory index exists; each hit names its source (jev, grep, graph)."
}
func (findTool) Schema() map[string]any {
	return obj(map[string]any{
		"query": map[string]any{"type": "string", "description": "Concept, behavior, or implementation to find in plain language."},
		"grep_keywords": map[string]any{
			"type": "array", "items": map[string]any{"type": "string"},
			"description": "Identifiers or symbols likely to appear verbatim in matching source.",
		},
		"path": map[string]any{"type": "string", "description": "Optional subdirectory to narrow the search scope."},
	}, "query", "grep_keywords")
}

func (f findTool) Execute(ctx context.Context, root string, args json.RawMessage) (string, error) {
	var a struct {
		Query        string   `json:"query"`
		GrepKeywords []string `json:"grep_keywords"`
		Path         string   `json:"path"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid find arguments: %w", err)
	}
	if strings.TrimSpace(a.Query) == "" {
		return "", errors.New("query cannot be empty")
	}

	cfg := LoadSettings()
	provider := f.provider
	if provider == nil && !f.lexicalOnly {
		// Typed-nil trap: only assign when there is a key, so a missing key
		// leaves the interface itself nil and the cascade takes its lexical path.
		if key := systemone.APIKey(ctx); key != "" {
			provider = systemone.NewOpenRouter(key)
		}
	}
	searcher := NewCascadeSearcher(provider, systemone.CanonicalModel(cfg.DecisionModel), cfg.Threshold, root)
	if f.graph != nil {
		searcher.WithGraphSource(f.graph)
	} else {
		// Nil when the graph is disabled or its binary is absent: the
		// availability check lives in defaultGraphSourceForRoot, so it is
		// written once. WithGraphSource(nil) wires nothing.
		searcher.WithGraphSource(defaultGraphSourceForRoot(root))
	}

	hits, err := searcher.Search(ctx, a.Query, a.GrepKeywords, a.Path)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}
	if len(hits) == 0 {
		return fmt.Sprintf("no hits for %q (τ %.2f)", a.Query, searcher.threshold), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d hit(s) for %q (τ %.2f), strongest first\n\n", len(hits), a.Query, searcher.threshold)
	for _, hit := range hits {
		fmt.Fprintf(&sb, "%s  [%s]  %.2f\n", hit.Path, hit.Source, hit.Score)
		for _, r := range hit.Ranges {
			fmt.Fprintf(&sb, "  %s:%d-%d  %.2f  %s\n", hit.Path, r.StartLine, r.EndLine, r.Score, previewLine(r.Snippet, a.GrepKeywords))
		}
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

// previewLine is the first non-blank line of a snippet, or the first one that
// carries a keyword.
func previewLine(snippet string, keywords []string) string {
	preview := ""
	for _, line := range strings.Split(strings.TrimSpace(snippet), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if preview == "" {
			preview = trimmed
		}
		for _, kw := range keywords {
			if strings.Contains(strings.ToLower(trimmed), strings.ToLower(kw)) {
				return trimmed
			}
		}
	}
	return preview
}
