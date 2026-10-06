package tools

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"example.com/wick/statedb"
)

// Overflow head and tail: how many lines of a stored output the summary
// carries, so the summary itself stays small while both ends stay visible.
const (
	overflowHeadLines = 10
	overflowTailLines = 10
	overflowMaxTerms  = 8
)

// MaybeOverflow is the shared helper every oversized tool result passes
// through. At or under overflow.threshold_bytes it returns the content
// unchanged; above it the content is stored in the state database and a
// short summary comes back: head and tail, total size and line count, the
// ctx: handle, and searchable terms. intent, when non-empty, also returns
// the chunks matching it inline (the execute ticket's use).
func MaybeOverflow(root, session, source, contentType, content, intent string) (string, bool, error) {
	_, out, stored, err := storeOverflow(root, session, source, contentType, content, intent)
	return out, stored, err
}

// storeOverflow is MaybeOverflow with the blob id: callers that name the
// handle alongside inline content (URL read's narrowed view) need it.
func storeOverflow(root, session, source, contentType, content, intent string) (int64, string, bool, error) {
	threshold := EffectiveSettings(root).EffectiveOverflowThresholdBytes()
	if len(content) <= threshold {
		return 0, content, false, nil
	}
	db, err := statedb.Open(statedb.DBPath())
	if err != nil {
		return 0, "", false, err
	}
	defer db.Close()
	id, err := statedb.InsertCtxBlob(db, statedb.CtxBlob{
		Session: session, Source: source, ContentType: contentType, Bytes: content,
	}, statedb.ChunkText(content))
	if err != nil {
		return 0, "", false, err
	}
	lines := splitOverflowLines(content)
	var b strings.Builder
	fmt.Fprintf(&b, "[overflow] output stored as ctx:%d (%d bytes, %d lines) from %s\n",
		id, len(content), len(lines), source)
	head := lines
	if len(head) > overflowHeadLines {
		head = head[:overflowHeadLines]
	}
	fmt.Fprintf(&b, "head (%d of %d lines):\n", len(head), len(lines))
	for _, l := range head {
		b.WriteString(l + "\n")
	}
	if len(lines) > overflowHeadLines+overflowTailLines {
		fmt.Fprintf(&b, "... [%d lines omitted] ...\n", len(lines)-overflowHeadLines-overflowTailLines)
		tail := lines[len(lines)-overflowTailLines:]
		fmt.Fprintf(&b, "tail (%d lines):\n", len(tail))
		for _, l := range tail {
			b.WriteString(l + "\n")
		}
	} else if len(lines) > len(head) {
		for _, l := range lines[len(head):] {
			b.WriteString(l + "\n")
		}
	}
	if terms := overflowTerms(content); len(terms) > 0 {
		fmt.Fprintf(&b, "searchable terms: %s\n", strings.Join(terms, ", "))
	}
	fmt.Fprintf(&b, "page it with read ctx:%d:<start>-<end>; search it with ctx_search\n", id)
	if strings.TrimSpace(intent) != "" {
		hits, err := statedb.SearchCtx(db, intent, &id, 3)
		if err != nil {
			return 0, "", false, err
		}
		if len(hits) > 0 {
			fmt.Fprintf(&b, "matching excerpts for intent %q:\n", intent)
			for _, h := range hits {
				fmt.Fprintf(&b, "ctx:%d:%d-%d: %s\n", h.BlobID, h.StartLine, h.EndLine, h.Snippet)
			}
		}
	}
	return id, b.String(), true, nil
}

// truncateHeadTail caps text at head+tail bytes with an omission note: the
// bounded fallback when the overflow store itself is unavailable.
func truncateHeadTail(text string, head, tail int) string {
	if len(text) <= head+tail {
		return text
	}
	return text[:head] + fmt.Sprintf("\n... [%d bytes omitted] ...\n", len(text)-head-tail) + text[len(text)-tail:]
}

// overflowSession is the session a stored output is attributed to: the
// calling agent, or empty outside any session.
func overflowSession() string { return os.Getenv("WICK_AGENT_ID") }

func splitOverflowLines(content string) []string {
	s := strings.ReplaceAll(content, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// ParseCtxHandle parses a ctx: handle: ctx:<id> or ctx:<id>:<range>, where
// the range is a read line selector (N, N-M, N+M, comma lists). ok is false
// for anything else.
func ParseCtxHandle(s string) (id int64, spec string, ok bool) {
	body, found := strings.CutPrefix(strings.TrimSpace(s), "ctx:")
	if !found || body == "" {
		return 0, "", false
	}
	head, tail, hasRange := strings.Cut(body, ":")
	if !hasRange {
		head = body
	}
	n, err := strconv.ParseInt(head, 10, 64)
	if err != nil || n < 1 {
		return 0, "", false
	}
	if !hasRange || strings.TrimSpace(tail) == "" {
		return n, "", true
	}
	if !isLineSpec(strings.TrimSpace(tail)) {
		return 0, "", false
	}
	return n, strings.TrimSpace(tail), true
}

// overflowStopwords are tokens too common to suggest as search terms.
var overflowStopwords = map[string]bool{
	"about": true, "after": true, "again": true, "also": true, "around": true,
	"because": true, "been": true, "before": true, "between": true, "both": true,
	"each": true, "from": true, "have": true, "here": true, "into": true,
	"line": true, "lines": true, "more": true, "most": true, "other": true,
	"over": true, "should": true, "some": true, "such": true, "than": true,
	"that": true, "their": true, "them": true, "then": true, "there": true,
	"these": true, "this": true, "those": true, "through": true, "under": true,
	"until": true, "when": true, "where": true, "which": true, "while": true,
	"will": true, "with": true, "within": true, "would": true, "your": true,
}

// overflowTerms suggests up to eight searchable terms for a stored output:
// frequent lowercase tokens of four or more characters, stopwords excluded,
// most frequent first.
func overflowTerms(content string) []string {
	freq := map[string]int{}
	for _, f := range strings.FieldsFunc(strings.ToLower(content), func(r rune) bool {
		return !(r == '_' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	}) {
		if len(f) < 4 || overflowStopwords[f] {
			continue
		}
		freq[f]++
	}
	type term struct {
		word string
		n    int
	}
	var terms []term
	for w, n := range freq {
		terms = append(terms, term{w, n})
	}
	sort.Slice(terms, func(i, j int) bool {
		if terms[i].n != terms[j].n {
			return terms[i].n > terms[j].n
		}
		return terms[i].word < terms[j].word
	})
	var out []string
	for i, t := range terms {
		if i >= overflowMaxTerms {
			break
		}
		out = append(out, t.word)
	}
	return out
}
