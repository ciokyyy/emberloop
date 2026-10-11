package tools

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"example.com/emberloop/tools/systemone"
)

// PassageRange represents a matched line interval with score and code snippet.
type PassageRange struct {
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
	Score     float64 `json:"score"`
	Snippet   string  `json:"snippet"`
}

// SearchHit represents a file match containing scored passage ranges.
// Source names the index that produced the hit: "jev" (the decisions-model
// cascade), "grep" (lexical ranking without the model) or "graph" (the
// codebase-memory index). Symbol holds the graph symbol name when one is
// known; it is empty for cascade hits.
type SearchHit struct {
	Path   string         `json:"path"`
	Score  float64        `json:"score"`
	Ranges []PassageRange `json:"ranges"`
	Source string         `json:"source"`
	Symbol string         `json:"symbol,omitempty"`
}

// Find hit sources. The cascade answers "jev" when the decisions model
// ranked it and "grep" on the lexical fallback; the code graph answers
// "graph".
const (
	SourceJev   = "jev"
	SourceGrep  = "grep"
	SourceGraph = "graph"
)

// GraphSource queries the codebase-memory index. It returns hits tagged
// SourceGraph (the caller forces the tag when missing) or an error when the
// query fails. A nil source, an error, or an empty result leaves the cascade
// behavior unchanged: the caller drops the graph result silently.
type GraphSource func(ctx context.Context, query string, keywords []string, root string) ([]SearchHit, error)

// MergePassageRanges combines adjacent or overlapping ranges into continuous blocks,
// preserving the highest probability score.
//
// Go Idiom: Slice sorting and in-place condensation. Sorting by start line allows
// a single O(N) sweep to coalesce intersecting intervals.
func MergePassageRanges(ranges []PassageRange) []PassageRange {
	if len(ranges) <= 1 {
		return ranges
	}

	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].StartLine < ranges[j].StartLine
	})

	var merged []PassageRange
	curr := ranges[0]

	for i := 1; i < len(ranges); i++ {
		next := ranges[i]
		// Adjacent or overlapping
		if next.StartLine <= curr.EndLine+1 {
			if next.EndLine > curr.EndLine {
				nextLines := strings.Split(next.Snippet, "\n")
				overlap := (curr.EndLine - next.StartLine) + 1
				var sb strings.Builder
				sb.WriteString(curr.Snippet)
				if overlap >= 0 && overlap < len(nextLines) {
					sb.WriteString("\n")
					sb.WriteString(strings.Join(nextLines[overlap:], "\n"))
				} else if overlap < 0 {
					sb.WriteString("\n")
					sb.WriteString(next.Snippet)
				}
				curr.Snippet = sb.String()
				curr.EndLine = next.EndLine
			}
			if next.Score > curr.Score {
				curr.Score = next.Score
			}
		} else {
			merged = append(merged, curr)
			curr = next
		}
	}
	merged = append(merged, curr)
	return merged
}

// CascadeSearcher implements the 4-stage jegrep semantic code search cascade.
// It is not safe for concurrent Search calls: per-search attribution lives in
// these fields, and find builds a fresh searcher per call, so no caller shares
// one across goroutines.
type CascadeSearcher struct {
	provider  systemone.Provider
	model     string
	threshold float64
	workDir   string
	// graph is the optional third source: the codebase-memory index. Nil
	// means absent, and the search degrades to the jev and grep sources.
	graph GraphSource
	// agentID and findCallID attribute decision calls in the agent's usage
	// log (ticket 04). They are set per Search from EL_AGENT_ID and a
	// fresh id; empty means this search logs nothing.
	agentID    string
	findCallID string
}

// NewCascadeSearcher creates an initialized CascadeSearcher.
func NewCascadeSearcher(provider systemone.Provider, model string, threshold float64, workDir string) *CascadeSearcher {
	if threshold <= 0 {
		threshold = 0.20
	}
	if model == "" {
		// The pinned selector, not the alias this used to spell: a fallback that
		// floats is a threshold measured against one release silently compared
		// against the next (ADR-0009 §2, systemone/selectors.go).
		model = systemone.JevModelPinned
	}
	return &CascadeSearcher{
		provider:  provider,
		model:     model,
		threshold: threshold,
		workDir:   workDir,
	}
}

// WithGraphSource attaches the codebase-memory index as find's third source.
// Nil removes it. The searcher is not safe for concurrent Search calls, so
// set this once before searching.
func (c *CascadeSearcher) WithGraphSource(g GraphSource) *CascadeSearcher {
	c.graph = g
	return c
}

// Ignored directories mirror common build output and dependency folders.
var ignoredSearchDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true, "node_modules": true,
	"target": true, "dist": true, "build": true, "coverage": true,
	".next": true, ".svelte-kit": true, ".nuxt": true, "vendor": true,
	"__pycache__": true, ".venv": true,
}

// Ignored lockfiles and manifests never implement behavior; they only add noise.
var ignoredSearchFiles = map[string]bool{
	"bun.lock": true, "bun.lockb": true, "package-lock.json": true,
	"pnpm-lock.yaml": true, "yarn.lock": true, "Cargo.lock": true,
	"Gemfile.lock": true, "poetry.lock": true, "composer.lock": true,
	"go.sum": true,
}

// Ignored extensions are generated maps, bundles, binaries, or media.
var ignoredSearchExts = []string{
	".lock", ".map", ".min.js", ".bundle.js",
	".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico",
	".woff", ".woff2", ".ttf", ".eot",
	".mp4", ".webm", ".mov", ".zip", ".tar", ".gz",
	".exe", ".dll", ".so", ".dylib", ".bin", ".pdf",
}

// Search stopwords keeps query-derived terms focused; explicit grep_keywords bypass this filter.
var searchStopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "from": true,
	"that": true, "this": true, "where": true, "what": true, "which": true,
	"when": true, "does": true, "are": true, "how": true, "why": true,
	"had": true, "has": true, "was": true, "were": true, "been": true,
	"into": true, "over": true, "under": true, "about": true, "there": true,
	"their": true, "them": true, "then": true, "than": true, "also": true,
	"such": true, "some": true, "any": true, "all": true, "our": true,
	"your": true, "its": true, "can": true, "will": true,
	"a": true, "an": true, "is": true, "it": true, "of": true,
	"or": true, "to": true, "in": true, "on": true, "as": true, "by": true,
	"at": true, "be": true, "do": true, "if": true, "so": true,
}

// shouldIgnorePath reports whether a file path is build output, dependency, lockfile, or media noise.
func shouldIgnorePath(path string) bool {
	base := filepath.Base(path)
	if strings.HasPrefix(base, ".") {
		return true
	}
	if ignoredSearchFiles[base] {
		return true
	}
	lowerBase := strings.ToLower(base)
	for _, ext := range ignoredSearchExts {
		if strings.HasSuffix(lowerBase, ext) {
			return true
		}
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if ignoredSearchDirs[part] {
			return true
		}
	}
	return false
}

// extractTerms merges explicit keywords with filtered query words.
// Explicit keywords pass through untouched; query words must clear length,
// stopword, and numeric filters.
func extractTerms(query string, keywords []string) []string {
	seen := map[string]bool{}
	var terms []string
	for _, kw := range keywords {
		clean := strings.ToLower(strings.Trim(kw, `"'.,;:!?()[]{}<>`))
		if clean == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		terms = append(terms, clean)
	}
	for _, word := range strings.Fields(strings.ToLower(query)) {
		clean := strings.Trim(word, `"'.,;:!?()[]{}<>`)
		if len(clean) < 3 || seen[clean] || searchStopwords[clean] {
			continue
		}
		if _, err := fmt.Sscanf(clean, "%d", new(int)); err == nil {
			continue
		}
		seen[clean] = true
		terms = append(terms, clean)
	}
	return terms
}

// isWordChar treats letters, digits, and underscore as word constituents for boundary checks.
func isWordChar(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// countWholeWord counts case-insensitive whole-word occurrences of term in text.
func countWholeWord(text, term string) int {
	lower := strings.ToLower(text)
	n := 0
	for i := 0; i+len(term) <= len(lower); i++ {
		if !strings.HasPrefix(lower[i:], term) {
			continue
		}
		beforeOK := i == 0 || !isWordChar(lower[i-1])
		afterOK := i+len(term) == len(lower) || !isWordChar(lower[i+len(term)])
		if beforeOK && afterOK {
			n++
		}
	}
	return n
}

// isDefinitionLine reports whether a line defines (not merely mentions) a term.
func isDefinitionLine(line, term string) bool {
	lower := strings.TrimSpace(strings.ToLower(line))
	if strings.HasPrefix(lower, "//") || strings.HasPrefix(lower, "#") || strings.HasPrefix(lower, "*") {
		return false
	}
	prefixes := []string{"func " + term, "package " + term, "type " + term, "class " + term, "def " + term, "const " + term, "var " + term, "module " + term}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			rest := lower[len(p):]
			if rest == "" || !isWordChar(rest[0]) {
				return true
			}
		}
	}
	return false
}

// lexicalFileScore calibrates a 0-1 relevance score from filename, path, density, definitions, and coverage.
func lexicalFileScore(path, content string, terms []string) float64 {
	if len(terms) == 0 {
		return 0.30
	}
	score := 0.30
	lowerBase := strings.ToLower(filepath.Base(path))
	lowerDir := strings.ToLower(filepath.Dir(path))
	matched := 0
	for _, term := range terms {
		termMatched := false
		if strings.Contains(lowerBase, term) {
			score += 0.20
			termMatched = true
		} else if strings.Contains(lowerDir, term) {
			score += 0.05
			termMatched = true
		}
		if n := countWholeWord(content, term); n > 0 {
			termMatched = true
			if n > 5 {
				n = 5
			}
			score += 0.05 * float64(n)
		}
		if termMatched {
			matched++
		}
	}
	defs := 0
	for _, line := range strings.Split(content, "\n") {
		for _, term := range terms {
			if isDefinitionLine(line, term) {
				defs++
				break
			}
		}
		if defs >= 2 {
			break
		}
	}
	score += 0.12 * float64(defs)
	score += 0.10 * float64(matched) / float64(len(terms))
	if score > 0.95 {
		score = 0.95
	}
	return score
}

// scorePassageLexical calibrates a passage score favoring definitions and whole-word hits.
func scorePassageLexical(chunk, filePath string, terms []string) float64 {
	score := 0.35
	lowerBase := strings.ToLower(filepath.Base(filePath))
	for _, term := range terms {
		if strings.Contains(lowerBase, term) {
			score += 0.10
			break
		}
	}
	defHit := false
	for _, line := range strings.Split(chunk, "\n") {
		for _, term := range terms {
			if isDefinitionLine(line, term) {
				defHit = true
				break
			}
		}
		if defHit {
			break
		}
	}
	if defHit {
		score += 0.25
	}
	wholeHits := 0
	for _, term := range terms {
		if countWholeWord(chunk, term) > 0 {
			wholeHits++
		}
	}
	if wholeHits > 2 {
		wholeHits = 2
	}
	score += 0.10 * float64(wholeHits)
	if score > 0.95 {
		score = 0.95
	}
	return score
}

// lexicalFileScoreFromDisk reads a file and calibrates its score; unreadable files score on filename alone.
func lexicalFileScoreFromDisk(path string, terms []string) float64 {
	data, err := os.ReadFile(path) // #nosec G304 -- search ranking intentionally reads files under the caller-selected workspace root
	if err != nil {
		return lexicalFileScore(path, "", terms)
	}
	content := string(data)
	if len(content) > 1024*1024 {
		content = content[:1024*1024]
	}
	return lexicalFileScore(path, content, terms)
}

func lexicalFileScoreForRanking(path string, terms []string, readContent bool) float64 {
	if !readContent {
		return lexicalFileScore(path, "", terms)
	}
	return lexicalFileScoreFromDisk(path, terms)
}

// newFindCallID names one find search in the usage log, grouping the decision
// calls of its stages so per-search spend can be summed. The rand fallback
// stays unique per call (pid plus nanotime), so even it never collapses two
// searches onto one id.
func newFindCallID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("find-unknown-%d-%d", os.Getpid(), time.Now().UnixNano())
	}
	return "find-" + hex.EncodeToString(b[:])
}

// logFindCall records one decision call against the calling agent. It never
// fails the search: attribution is telemetry, and a missing agent id or an
// unwritable log must not break find.
func (c *CascadeSearcher) logFindCall(req systemone.Request, resp systemone.Response) {
	if c.agentID == "" || c.findCallID == "" {
		return
	}
	_ = LogFindCall(FindUsage{
		Agent:      c.agentID,
		Model:      c.model,
		FindCallID: c.findCallID,
		Questions:  len(req.Questions),
		Generation: resp.Generation,
		Provider:   resp.Provider,
		Usage:      resp.Usage,
	})
}

// Search executes the 4-stage cascade over candidate files, then merges the
// optional graph source. A nil graph, a graph error, or an empty graph
// result leaves the cascade answer unchanged.
func (c *CascadeSearcher) Search(ctx context.Context, query string, keywords []string, targetDir string) ([]SearchHit, error) {
	// One find-call id per search groups the decision calls of its stages
	// in the calling agent's usage log. Outside a session there is no agent
	// to attribute to, and the search logs nothing. Both fields reset first,
	// so a reused searcher never attributes to a previous search's agent.
	c.agentID, c.findCallID = "", ""
	if agent := os.Getenv("EL_AGENT_ID"); agent != "" {
		c.agentID, c.findCallID = agent, newFindCallID()
	}
	searchRoot := c.workDir
	if targetDir != "" {
		// Confined like read and write: a path a model chose is untrusted.
		dir, err := resolveInside(c.workDir, targetDir)
		if err != nil {
			return nil, err
		}
		searchRoot = dir
	}

	// Stage 1: Lexical scan to isolate candidate files (cap 128)
	candidates, err := c.stage1LexicalScan(ctx, searchRoot, keywords, query)
	stageErr := err
	if (err != nil || len(candidates) == 0) && c.graph == nil {
		return nil, err
	}

	// Stage 2: Filename ranking via System One (cap 20)
	var topFiles []string
	if len(candidates) > 0 {
		topFiles, err = c.stage2FilenameRanking(ctx, candidates, query, keywords)
		if err != nil {
			topFiles = candidates
			if len(topFiles) > 20 {
				topFiles = topFiles[:20]
			}
		}
	}

	// Stage 3 & 4: Passage extraction, sketch scoring, and range verification
	hits := make([]SearchHit, 0, len(topFiles))
	unreadable := 0
	for _, file := range topFiles {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		hit, err := c.processFilePassages(ctx, file, searchRoot, query, keywords)
		if err != nil {
			// One unreadable file is not a failed search: the cascade is a
			// ranking over candidates, and this one produced nothing.
			unreadable++
			continue
		}
		if len(hit.Ranges) > 0 && hit.Score >= c.threshold {
			hits = append(hits, hit)
		}
	}
	_ = unreadable

	if c.graph != nil && ctx.Err() == nil {
		if ghits, gerr := c.graph(ctx, query, keywords, searchRoot); gerr == nil && len(ghits) > 0 {
			for _, g := range ghits {
				if g.Path == "" || len(g.Ranges) == 0 {
					continue
				}
				if g.Source == "" {
					g.Source = SourceGraph
				}
				if g.Score >= c.threshold {
					hits = append(hits, g)
				}
			}
		}
		// A graph error is a downgrade, not a failure: the cascade hits
		// stand, exactly as when the index is absent.
	}

	// One file lists once: a path the graph and the cascade both name fuses
	// into a single hit carrying the merged ranges and the winner's source.
	// Terms extract once for the merge and the sort below.
	terms := extractTerms(query, keywords)
	hits = mergeHitsByPath(hits, terms)
	sortFindHits(hits, terms)

	// A stage-1 error with no hits anywhere stays an error: a failing graph
	// must not turn "no search terms" into an empty success.
	if len(hits) == 0 && stageErr != nil {
		return nil, stageErr
	}
	return hits, nil
}

// mergeHitsByPath fuses hits that name the same path into one entry, so a
// file the graph and the cascade both name lists once. Ranges merge through
// MergePassageRanges and cap at three, as cascade hits do; the score is the
// highest either source gave. The source and symbol come from the winner: an
// exact symbol-name hit beats a passage guess, else the higher score wins,
// else the first hit (the cascade's) stands.
func mergeHitsByPath(hits []SearchHit, terms []string) []SearchHit {
	if len(hits) <= 1 {
		return hits
	}
	byPath := make(map[string]int, len(hits))
	out := make([]SearchHit, 0, len(hits))
	for _, h := range hits {
		i, dup := byPath[h.Path]
		if !dup {
			byPath[h.Path] = len(out)
			out = append(out, h)
			continue
		}
		dst := &out[i]
		hExact, dExact := isExactSymbolHit(h, terms), isExactSymbolHit(*dst, terms)
		merged := MergePassageRanges(append(append([]PassageRange(nil), dst.Ranges...), h.Ranges...))
		if len(merged) > 3 {
			merged = merged[:3]
		}
		max := dst.Score
		if h.Score > max {
			max = h.Score
		}
		// The winner lends its source and symbol: an exact symbol-name hit
		// beats a passage guess at any score, else the higher score wins
		// and a tie keeps the first hit (the cascade's).
		if (hExact && !dExact) || (hExact == dExact && h.Score > dst.Score) {
			dst.Source, dst.Symbol = h.Source, h.Symbol
		}
		dst.Ranges = merged
		dst.Score = max
	}
	return out
}

// cascadeSource names the cascade hit source for this searcher: the
// decisions model ("jev") when one is wired, else the lexical fallback
// ("grep").
func (c *CascadeSearcher) cascadeSource() string {
	if c.provider != nil {
		return SourceJev
	}
	return SourceGrep
}

// sortFindHits orders merged hits with the existing score fusion, except
// graph hits that name an exact symbol sort first: a symbol the index knows
// by name outranks a scored passage guess. Terms come extracted once per
// search by the caller.
func sortFindHits(hits []SearchHit, terms []string) {
	sort.SliceStable(hits, func(i, j int) bool {
		ei, ej := isExactSymbolHit(hits[i], terms), isExactSymbolHit(hits[j], terms)
		if ei != ej {
			return ei
		}
		if hits[i].Score == hits[j].Score {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].Score > hits[j].Score
	})
}

// isExactSymbolHit reports whether a graph hit names a symbol that exactly
// equals a search term. Only graph hits qualify: the cascade has no symbol
// names, only passages. The match is case-insensitive; when the hit carries
// no symbol, the file base name without extension stands in, so an index
// that answers files still promotes an exact file-symbol match.
func isExactSymbolHit(h SearchHit, terms []string) bool {
	if h.Source != SourceGraph {
		return false
	}
	names := []string{strings.TrimSpace(h.Symbol)}
	if base := strings.TrimSuffix(filepath.Base(h.Path), filepath.Ext(h.Path)); base != "" {
		names = append(names, base)
	}
	for _, n := range names {
		if n == "" {
			continue
		}
		for _, t := range terms {
			if strings.EqualFold(n, t) {
				return true
			}
		}
	}
	return false
}

// stage1LexicalScan locates candidate files containing keywords or query terms.
func (c *CascadeSearcher) stage1LexicalScan(ctx context.Context, root string, keywords []string, query string) ([]string, error) {
	terms := extractTerms(query, keywords)

	// Try ripgrep first
	files, err := c.ripgrepFiles(ctx, root, terms)
	if err == nil && len(files) > 0 {
		if len(files) > 128 {
			files = files[:128]
		}
		return files, nil
	}
	if err != nil {
		// ripgrep is the fast path, not the contract: the walk below answers
		// the same question, so a missing or failing rg is a downgrade.
	}

	// Fallback to pure Go directory walk
	files, err = c.walkFiles(ctx, root, terms, 128)
	return files, err
}

func (c *CascadeSearcher) ripgrepFiles(ctx context.Context, root string, terms []string) ([]string, error) {
	if len(terms) == 0 {
		return nil, fmt.Errorf("no search terms")
	}

	pattern := strings.Join(terms, "|")
	args := []string{"-l", "-i", "--max-filesize", "4M", "--hidden", "-e", pattern, root}
	for dir := range ignoredSearchDirs {
		args = append(args, "-g", "!"+dir+"/**")
	}
	for file := range ignoredSearchFiles {
		args = append(args, "-g", "!"+file)
	}
	for _, ext := range ignoredSearchExts {
		args = append(args, "-g", "!*"+ext)
	}
	args = append(args, "-g", "!.*")
	cmd := exec.CommandContext(ctx, "rg", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var results []string
	ignored := 0
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		path := strings.TrimSpace(scanner.Text())
		if path != "" && !shouldIgnorePath(path) {
			results = append(results, path)
		} else {
			ignored++
		}
	}
	return results, nil
}
func (c *CascadeSearcher) walkFiles(ctx context.Context, root string, terms []string, limit int) ([]string, error) {
	var results []string
	// Every skip below is a swallowed error or a deliberate filter, and this is
	// the fallback path: a scan that quietly returns nothing looks exactly like
	// a tree with no match, which is the one thing an operator cannot debug.
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer rootFS.Close()

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if ignoredSearchDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if len(results) >= limit {
			return filepath.SkipAll
		}

		if shouldIgnorePath(path) {
			return nil
		}
		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}

		file, err := rootFS.Open(relativePath)
		if err != nil {
			return nil
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > 4*1024*1024 || info.Size() == 0 {
			if err != nil {
			}
			return nil
		}
		data, err := io.ReadAll(io.LimitReader(file, 4*1024*1024))
		if err != nil {
			return nil
		}

		content := strings.ToLower(string(data))
		for _, term := range terms {
			if strings.Contains(content, strings.ToLower(term)) {
				results = append(results, path)
				break
			}
		}
		return nil
	})
	return results, err
}

// stage2FilenameRanking ranks candidate filenames using System One in batches of 64.
// Without a provider (or on judge failure) it falls back to calibrated lexical file scores.
func (c *CascadeSearcher) stage2FilenameRanking(ctx context.Context, files []string, query string, keywords []string) ([]string, error) {
	if len(files) == 0 {
		return files, nil
	}
	terms := extractTerms(query, keywords)
	readContent := len(files) <= 100

	if c.provider == nil {
		return rankFilesLexically(files, terms, 20), nil
	}

	type scoredFile struct {
		path  string
		score float64
	}
	scored := make([]scoredFile, 0, len(files))

	// Batch in chunks of 16: each file travels with its first lines, and 64 of
	// those would crowd the judge's context.
	batchSize := 16
	for i := 0; i < len(files); i += batchSize {
		end := i + batchSize
		if end > len(files) {
			end = len(files)
		}
		batch := files[i:end]

		questions := make(map[string]systemone.Question, len(batch))
		heads := make(map[string]string, len(batch))
		for _, f := range batch {
			rel, relErr := filepath.Rel(c.workDir, f)
			if relErr != nil {
				rel = filepath.Base(f)
			}
			heads[rel] = fileHead(f)
			// A path alone ranks files at AUC 0.87; the path plus its first
			// lines ranks them at 0.96 (docs/benchmarks/jev-prompt-benchmark.md).
			questions[rel] = systemone.Question{
				Type:         systemone.TypeNoul,
				Instructions: fmt.Sprintf("Is the file `%s` (see its entry in `files`) one that would have to change to implement `query`?", rel),
				Criteria: map[string]string{
					"true":  "The file implements or directly changes the behaviour described in `query`",
					"false": "The file is unrelated, or only incidentally touches what `query` describes",
				},
			}
		}

		req := systemone.Request{
			Model:     c.model,
			State:     map[string]any{"query": query, "files": heads},
			Questions: questions,
		}

		resp, err := c.provider.Evaluate(ctx, req)
		if err != nil {
			// The judge is an improvement over the lexical ranking, not a
			// requirement: the batch still gets ranked, so this is a
			// fail-open that costs accuracy rather than a failed search.
			for _, f := range batch {
				scored = append(scored, scoredFile{path: f, score: lexicalFileScoreForRanking(f, terms, readContent)})
			}
			continue
		}
		c.logFindCall(req, resp)

		for _, f := range batch {
			rel, relErr := filepath.Rel(c.workDir, f)
			if relErr != nil {
				rel = filepath.Base(f)
			}
			prob := lexicalFileScoreForRanking(f, terms, readContent)
			if ans, ok := resp.Answers[rel]; ok && ans.Noul != nil {
				prob = *ans.Noul
			}
			scored = append(scored, scoredFile{path: f, score: prob})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	limit := 20
	if len(scored) < limit {
		limit = len(scored)
	}
	top := make([]string, limit)
	for i := 0; i < limit; i++ {
		top[i] = scored[i].path
	}
	return top, nil
}

// rankFilesLexically sorts files by calibrated lexical score, descending.
//
// Go Idiom: Decorate-sort-undecorate. Scores attach to paths for the sort,
// then strip back to paths so callers keep the same []string contract.
func rankFilesLexically(files []string, terms []string, limit int) []string {
	type scoredFile struct {
		path  string
		score float64
	}
	scored := make([]scoredFile, 0, len(files))
	readContent := len(files) <= 100
	for _, f := range files {
		scored = append(scored, scoredFile{path: f, score: lexicalFileScoreForRanking(f, terms, readContent)})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].path < scored[j].path
		}
		return scored[i].score > scored[j].score
	})
	if limit > 0 && len(scored) > limit {
		scored = scored[:limit]
	}
	out := make([]string, len(scored))
	for i, s := range scored {
		out[i] = s.path
	}
	return out
}

// processFilePassages extracts, scores, and verifies passages in a file.
func (c *CascadeSearcher) processFilePassages(ctx context.Context, filePath, searchRoot, query string, keywords []string) (SearchHit, error) {
	data, err := os.ReadFile(filePath) // #nosec G304 -- passage extraction intentionally reads files under the caller-selected workspace root
	if err != nil {
		return SearchHit{}, err
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return SearchHit{}, nil
	}

	terms := extractTerms(query, keywords)
	windowSize := 10
	step := 5
	var rawRanges []PassageRange

	for i := 0; i < len(lines); i += step {
		end := i + windowSize
		if end > len(lines) {
			end = len(lines)
		}
		chunk := strings.Join(lines[i:end], "\n")

		// Quick lexical check against calibrated terms
		matched := false
		for _, term := range terms {
			if strings.Contains(strings.ToLower(chunk), term) {
				matched = true
				break
			}
		}

		if matched {
			rawRanges = append(rawRanges, PassageRange{
				StartLine: i + 1,
				EndLine:   end,
				Score:     scorePassageLexical(chunk, filePath, terms),
				Snippet:   chunk,
			})
		}
	}

	// If System One is available, score passages (Stage 3 & 4)
	if c.provider != nil {
		relFile, relErr := filepath.Rel(c.workDir, filePath)
		if relErr != nil {
			relFile = filepath.Base(filePath)
		}
		questions := make(map[string]systemone.Question, len(rawRanges))
		for _, r := range rawRanges {
			qID := fmt.Sprintf("%s:%d-%d", relFile, r.StartLine, r.EndLine)
			questions[qID] = systemone.Question{
				Type:         systemone.TypeNoul,
				Instructions: fmt.Sprintf("Does this code passage implement %q?\n\nPassage:\n```\n%s\n```", query, r.Snippet),
			}
		}

		req := systemone.Request{
			Model:     c.model,
			State:     map[string]any{"file": relFile, "query": query},
			Questions: questions,
		}

		resp, err := c.provider.Evaluate(ctx, req)
		if err != nil {
			// The lexical scores stand in for the judge: the file is still
			// searched, with a ranking nobody verified. That is a silent loss of
			// quality unless it is named, so it is named.
		} else {
			c.logFindCall(req, resp)
			var verifiedRanges []PassageRange
			for _, r := range rawRanges {
				qID := fmt.Sprintf("%s:%d-%d", relFile, r.StartLine, r.EndLine)
				if ans, ok := resp.Answers[qID]; ok && ans.Noul != nil {
					if *ans.Noul >= c.threshold {
						r.Score = *ans.Noul
						verifiedRanges = append(verifiedRanges, r)
					}
				}
			}
			rawRanges = verifiedRanges
		}
	}

	if len(rawRanges) == 0 {
		return SearchHit{}, nil
	}

	merged := MergePassageRanges(rawRanges)
	if len(merged) > 3 {
		merged = merged[:3]
	}

	maxScore := 0.0
	for _, r := range merged {
		if r.Score > maxScore {
			maxScore = r.Score
		}
	}

	relPath, err := filepath.Rel(searchRoot, filePath)
	if err != nil {
		relPath = filePath
	}

	return SearchHit{
		Path:   relPath,
		Score:  maxScore,
		Ranges: merged,
		Source: c.cascadeSource(),
	}, nil
}

// fileHeadLines and fileHeadBytes bound how much of a file the filename judge
// reads: enough to see what the file is, little enough that sixteen of them fit.
const (
	fileHeadLines = 30
	fileHeadBytes = 1500
)

// fileHead returns the first lines of a file, or "" when it cannot be read: a
// file the judge cannot see is ranked on its path, as before.
func fileHead(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(data) > 16*fileHeadBytes {
		data = data[:16*fileHeadBytes]
	}
	lines := strings.SplitN(string(data), "\n", fileHeadLines+1)
	if len(lines) > fileHeadLines {
		lines = lines[:fileHeadLines]
	}
	head := strings.Join(lines, "\n")
	if len(head) > fileHeadBytes {
		head = head[:fileHeadBytes]
	}
	return head
}
