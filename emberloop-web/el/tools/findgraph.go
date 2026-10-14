package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// defaultGraphSourceForRoot builds the production graph source for a find
// search rooted at workDir, or nil when no index can answer: a disabled
// graph or a missing binary wires nothing, so the search degrades to the
// jev and grep sources. An unready index and query failures also answer nil
// without an error, so the cascade behavior is unchanged.
func defaultGraphSourceForRoot(workDir string) GraphSource {
	s := EffectiveSettings(workDir)
	if !s.GraphEnabled() {
		return nil
	}
	cfg, ok := GraphServerSetting(s)
	if !ok {
		return nil
	}
	// Keywords shape the cascade's lexical scan; the index ranks from the
	// query alone, so this source leaves them unread.
	return func(ctx context.Context, query string, _ []string, root string) ([]SearchHit, error) {
		reg := NewRegistry(map[string]MCPServerSetting{GraphServerName: cfg})
		defer reg.Close()
		call := func(c context.Context, tool string, args json.RawMessage) (string, error) {
			return reg.Call(c, GraphServerName, tool, args)
		}
		statusCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		project := ResolveGraphProject(statusCtx, call, workDir)
		if !graphQueryReady(statusCtx, call, project) {
			return nil, nil
		}
		qctx, qcancel := context.WithTimeout(ctx, 15*time.Second)
		defer qcancel()
		raw, err := json.Marshal(map[string]any{"project": project, "query": query})
		if err != nil {
			return nil, nil
		}
		text, err := reg.Call(qctx, GraphServerName, "search_graph", raw)
		if err != nil {
			return nil, err
		}
		searchRoot := root
		if searchRoot == "" {
			searchRoot = workDir
		}
		return parseGraphResults(text, searchRoot, workDir), nil
	}
}

// graphQueryReady reports whether the project's index status names a usable
// index. Errors and unparseable answers read as not ready: the caller
// degrades to the cascade sources.
func graphQueryReady(ctx context.Context, call GraphStatusCall, project string) bool {
	raw, err := json.Marshal(map[string]string{"project": project})
	if err != nil {
		return false
	}
	text, err := call(ctx, "index_status", raw)
	if err != nil {
		return false
	}
	return GraphStatusReady(text)
}

// graphFileRe finds file paths in a free-text graph answer: unix or windows
// separators ending in a code extension, with an optional :line suffix.
var graphFileRe = regexp.MustCompile(`[A-Za-z0-9_.\-][A-Za-z0-9_.\-/\\]*\.[A-Za-z0-9]{1,5}(?::\d+)?`)

// graphDefaultScore is the score of a graph hit the index sent without one:
// above the 0.20 passage cutoff, below a verified cascade score, so exact
// symbol-name matches still sort first on their symbol, not their number.
const graphDefaultScore = 0.75

// parseGraphResults converts a search_graph answer into tagged hits. It
// accepts the structured JSON shapes the binary has used (results/hits/files
// arrays of file/symbol/line/snippet/score objects, case-insensitively) and
// falls back to file paths in free text. Unrecognized text yields no hits,
// never an error: an index the parser cannot read is an absent source.
func parseGraphResults(text, searchRoot, workDir string) []SearchHit {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if hits := parseGraphJSON(text, searchRoot, workDir); len(hits) > 0 {
		return hits
	}
	return parseGraphTextPaths(text, searchRoot, workDir)
}

// graphResultObj is one leniently-decoded graph result: every field is
// optional and matched case-insensitively through the map walk below.
type graphResultObj struct {
	file     string
	symbol   string
	snippet  string
	line     int
	score    float64
	hasLine  bool
	hasScore bool
}

// parseGraphJSON walks decoded JSON for result-like objects.
func parseGraphJSON(text, searchRoot, workDir string) []SearchHit {
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return nil
	}
	var objs []graphResultObj
	collectGraphObjs(v, &objs)
	if len(objs) == 0 {
		return nil
	}
	var hits []SearchHit
	for _, o := range objs {
		path := graphHitPath(o.file, searchRoot, workDir)
		if path == "" {
			continue
		}
		score := o.score
		if !o.hasScore || score <= 0 {
			score = graphDefaultScore
		}
		start, end := 1, 1
		if o.hasLine && o.line > 0 {
			start, end = o.line, o.line
		}
		snippet := o.snippet
		if snippet == "" {
			snippet = o.symbol
		}
		if snippet == "" {
			snippet = path
		}
		hits = append(hits, SearchHit{
			Path:   path,
			Score:  score,
			Source: SourceGraph,
			Symbol: o.symbol,
			Ranges: []PassageRange{{StartLine: start, EndLine: end, Score: score, Snippet: snippet}},
		})
		if len(hits) >= 20 {
			break
		}
	}
	return hits
}

// collectGraphObjs appends every map that names a file. Only the file fields
// qualify: a symbol without a file (or an envelope's metadata, which often
// carries a "name") cannot map to a workspace path, so it is skipped rather
// than turning the symbol into a bogus file path.
func collectGraphObjs(v any, out *[]graphResultObj) {
	switch t := v.(type) {
	case []any:
		for _, e := range t {
			collectGraphObjs(e, out)
		}
	case map[string]any:
		lower := map[string]any{}
		for k, val := range t {
			lower[strings.ToLower(k)] = val
		}
		// Recurse first: envelopes hold arrays of results beside metadata.
		for _, val := range t {
			collectGraphObjs(val, out)
		}
		file := graphStringField(lower, "file", "path", "filepath", "filename")
		if file == "" {
			return
		}
		o := graphResultObj{file: file, symbol: graphStringField(lower, "symbol", "name", "function", "method", "class")}
		o.snippet = graphStringField(lower, "snippet", "code", "text", "content", "excerpt")
		if n, ok := graphLineField(lower, "line", "lineno", "startline", "start_line"); ok {
			o.line, o.hasLine = n, true
		}
		if f, ok := graphScoreField(lower, "score", "relevance", "confidence"); ok {
			o.score, o.hasScore = f, true
		}
		*out = append(*out, o)
	}
}

// graphStringField returns the first non-empty string field.
func graphStringField(m map[string]any, names ...string) string {
	for _, n := range names {
		if s, ok := m[n].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// graphLineField returns the first integer-like field.
func graphLineField(m map[string]any, names ...string) (int, bool) {
	for _, n := range names {
		switch v := m[n].(type) {
		case float64:
			if v > 0 {
				return int(v), true
			}
		case int:
			if v > 0 {
				return v, true
			}
		case int64:
			if v > 0 {
				return int(v), true
			}
		case json.Number:
			if i, err := v.Int64(); err == nil && i > 0 {
				return int(i), true
			}
		}
	}
	return 0, false
}

// graphScoreField returns the first float-like field in 0-1 or any positive
// range. Values above 1 are clamped to 0.95, the cascade ceiling.
func graphScoreField(m map[string]any, names ...string) (float64, bool) {
	for _, n := range names {
		var f float64
		var ok bool
		switch v := m[n].(type) {
		case float64:
			f, ok = v, true
		case int:
			f, ok = float64(v), true
		case int64:
			f, ok = float64(v), true
		case json.Number:
			if fv, err := v.Float64(); err == nil {
				f, ok = fv, true
			}
		}
		if !ok {
			continue
		}
		if f <= 0 {
			continue
		}
		if f > 1 {
			f = 0.95
		}
		return f, true
	}
	return 0, false
}

// graphHitPath relativizes a graph file to the search root. Absolute paths
// under the root stay; bare names and repo-relative paths inside the root
// are kept as named. Anything that escapes the search root, and ignored
// build output, is dropped. A bare symbol is never a file path: callers
// pass only file fields, so there is no symbol fallback here.
func graphHitPath(file, searchRoot, workDir string) string {
	clean := strings.TrimSpace(file)
	if clean == "" {
		return ""
	}
	// Strip a trailing :line the binary sometimes appends.
	if i := strings.LastIndex(clean, ":"); i >= 0 {
		rest := clean[i+1:]
		isNum := rest != ""
		for _, r := range rest {
			if r < '0' || r > '9' {
				isNum = false
				break
			}
		}
		if isNum {
			clean = clean[:i]
		}
	}
	clean = filepath.Clean(clean)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return ""
	}
	if filepath.IsAbs(clean) {
		rel, err := filepath.Rel(searchRoot, clean)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return ""
		}
		if shouldIgnorePath(clean) || shouldIgnorePath(rel) {
			return ""
		}
		return rel
	}
	// Repo-relative or bare: answer relative to the search root, the same
	// base the cascade relativizes to, so one file fuses in mergeHitsByPath
	// with or without confinement. Anything escaping the search root, and
	// ignored noise, is dropped.
	if shouldIgnorePath(clean) {
		return ""
	}
	rel, err := filepath.Rel(searchRoot, filepath.Join(workDir, clean))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return rel
}

// parseGraphTextPaths builds one hit per distinct file path in free text.
func parseGraphTextPaths(text, searchRoot, workDir string) []SearchHit {
	seen := map[string]bool{}
	var hits []SearchHit
	for _, m := range graphFileRe.FindAllString(text, -1) {
		path := graphHitPath(m, searchRoot, workDir)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		hits = append(hits, SearchHit{
			Path:   path,
			Score:  graphDefaultScore,
			Source: SourceGraph,
			Ranges: []PassageRange{{StartLine: 1, EndLine: 1, Score: graphDefaultScore, Snippet: m}},
		})
		if len(hits) >= 20 {
			break
		}
	}
	return hits
}
