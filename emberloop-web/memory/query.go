package memory

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Caps on search input. Free text is cut before tokenizing, each term is cut
// to MaxTokenRunes, and a query keeps at most MaxQueryTokens distinct terms.
const (
	MaxQueryRunes  = 512
	MaxTokenRunes  = 40
	MaxQueryTokens = 24
)

// Query turns free text into an FTS5 MATCH expression. Every term is a quoted
// word of letters and digits, and the terms are joined with OR. Punctuation,
// quotes, operators and column filters in the input never reach FTS5 as
// syntax. Query returns "" when the text holds no term.
func Query(text string) string {
	terms := Tokens(text)
	quoted := make([]string, len(terms))
	for i, term := range terms {
		quoted[i] = `"` + term + `"`
	}
	return strings.Join(quoted, " OR ")
}

// Tokens returns the search terms of text: every word, then the parts of each
// camelCase word. Terms are lower-cased, deduplicated and capped. So
// "TestUploadHandler" yields testuploadhandler, test, upload and handler.
func Tokens(text string) []string {
	text = firstRunes(text, MaxQueryRunes)
	seen := make(map[string]bool)
	var out []string
	add := func(term string) {
		term = strings.ToLower(firstRunes(term, MaxTokenRunes))
		if term == "" || seen[term] || len(out) >= MaxQueryTokens {
			return
		}
		seen[term] = true
		out = append(out, term)
	}
	for _, word := range words(text) {
		add(word)
		if parts := camelParts(word); len(parts) > 1 {
			for _, part := range parts {
				add(part)
			}
		}
	}
	return out
}

// SplitText is the identifier-split copy indexed beside the stored text. Each
// camelCase word is replaced by its parts, so "TestUploadHandler" becomes
// "Test Upload Handler" and "upload handler" finds it. The stored text keeps
// the original spelling.
func SplitText(text string) string {
	var out []string
	for _, word := range words(text) {
		if parts := camelParts(word); len(parts) > 1 {
			out = append(out, parts...)
		} else {
			out = append(out, word)
		}
	}
	return strings.Join(out, " ")
}

// words splits text into runs of letters, digits and combining marks. Every
// other rune separates words, so underscores, dots, slashes and dashes split
// snake_case, dotted paths and file paths.
func words(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsMark(r)
	})
}

// camelParts splits one word at camelCase boundaries. "TestUploadHandler"
// gives Test, Upload, Handler; "HTTPServer" gives HTTP, Server; "utf8Encode"
// gives utf8, Encode. A word with no boundary comes back as a single part.
func camelParts(word string) []string {
	r := []rune(word)
	var parts []string
	start := 0
	for i := 1; i < len(r); i++ {
		prev, cur := r[i-1], r[i]
		boundary := (unicode.IsLower(prev) || unicode.IsDigit(prev)) && unicode.IsUpper(cur) ||
			unicode.IsUpper(prev) && unicode.IsUpper(cur) && i+1 < len(r) && unicode.IsLower(r[i+1])
		if boundary {
			parts = append(parts, string(r[start:i]))
			start = i
		}
	}
	if start == 0 {
		return []string{word}
	}
	return append(parts, string(r[start:]))
}

// firstRunes returns at most n runes of s.
func firstRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	i := 0
	for pos := range s {
		if i == n {
			return s[:pos]
		}
		i++
	}
	return s
}
