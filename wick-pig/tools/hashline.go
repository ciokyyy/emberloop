package tools

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Hashline: line-anchored file edits.
//
// The grammar, the applier and the tag are ported from wick's
// apps/backend/tools/edit.go. One thing differs on purpose: read serves plain
// text only. Binary, oversized text, PDFs, notebooks, images and archives are
// refused with a reason, because ctx.ExecuteTool("read") resolves back to the
// pig-side tool and there is no route to pig's handling of those. See readPlain.
//
// ponytail: headerRegex requires [path#TAG], so a patch without a tag is
// refused with "no [path#TAG] header". The "tag optional" wording in earlier
// notes was never true of the code. Relax the regex if models keep skipping
// the read.

// maxServedFile bounds what read renders itself. Anything larger is refused
// rather than dumped into a prompt.
const maxServedFile = 2 << 20 // 2 MiB

// contentTag is the 4-hex snapshot tag, computed from raw bytes. It is
// stateless by design: sha256 of the file, first two bytes. No store, so it
// cannot drift from what is on disk.
func contentTag(content []byte) string {
	h := sha256.Sum256(content)
	return fmt.Sprintf("%02X%02X", h[0], h[1])
}

// ---------------------------------------------------------------------------
// patch grammar
// ---------------------------------------------------------------------------

type opKind int

const (
	opReplace opKind = iota
	opInsertBefore
	opInsertAfter
	opAppend
	opDelete
)

type patchOp struct {
	kind      opKind
	startLine int
	endLine   int
	body      []string
}

var (
	headerRegex    = regexp.MustCompile(`(?m)^\s*\[([^#\s]+)#([0-9A-Fa-f]{4})\]`)
	putRangeRegex  = regexp.MustCompile(`^PUT\s+(\d+)\.=(\d+):$`)
	putBeforeRegex = regexp.MustCompile(`^PUT\s+<(\d+):$`)
	putAfterRegex  = regexp.MustCompile(`^PUT\s+>(\d+):$`)
	putTailRegex   = regexp.MustCompile(`^PUT\s+>\$:$`)
	cutRangeRegex  = regexp.MustCompile(`^CUT\s+(\d+)\.=(\d+)$`)
	cutSingleRegex = regexp.MustCompile(`^CUT\s+(\d+)$`)
)

// ---------------------------------------------------------------------------
// edit
// ---------------------------------------------------------------------------

const editDescription = `Apply line-anchored modifications. One call, every change to that file.

Ops:
- PUT N.=M:  replace lines N-M (inclusive)
- PUT <N:    insert before line N
- PUT >N:    insert after line N
- PUT >$:    append to end of file
- CUT N / CUT N.=M: delete (no body)

Line numbers are the file as you last read it, never renumbered. An operation
at line 40 still targets line 40 even when an earlier operation in the same
call inserted ten lines above it, because the whole call is applied against
the original in one pass. Never re-derive anchors after an operation.

Body rule: every PUT op MUST be followed on the NEXT line by 1+ body lines each
starting with '+'. The '+' is stripped. Never put content on the same line as
PUT. Empty PUT body deletes — use CUT for deletes.

Header: if you were given [path#TAG], open with it. It is verified, and a stale
tag is refused rather than applied. A successful call returns the file's NEW tag
as [path#TAG]; reuse it for the next call on that file without re-reading.`

// applyHashline parses and applies one patch. baseDir bounds every path, so a
// patch cannot reach outside the workspace.
func applyHashline(baseDir, input string) (string, error) {
	text := strings.TrimSpace(input)
	text = strings.TrimPrefix(text, "*** Begin Patch")
	text = strings.TrimSuffix(text, "*** End Patch")
	text = strings.TrimSpace(text)

	header := headerRegex.FindStringSubmatch(text)
	if len(header) < 3 {
		return "", fmt.Errorf("patch has no [path#TAG] header naming a file")
	}
	relPath := filepath.Clean(header[1])
	expectedTag := strings.ToUpper(header[2])

	full, err := resolveInside(baseDir, relPath)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("read %q: %w", relPath, err)
	}

	// The tag is optional here, so the staleness refusal only applies when the
	// caller actually presented one.
	actualTag := contentTag(data)
	if expectedTag != "" && actualTag != expectedTag {
		return "", fmt.Errorf(
			"stale snapshot tag: you sent %s, %s is now %s; re-read it before editing",
			expectedTag, relPath, actualTag)
	}

	ops, err := parseHashlineOps(text)
	if err != nil {
		return "", err
	}
	if len(ops) == 0 {
		return "", fmt.Errorf("no valid hashline operations found in patch")
	}

	newBytes, lines, err := applyPatchBytes(data, ops)
	if err != nil {
		return "", err
	}

	// The tag proves the file matched the bytes this call read. It says nothing
	// about the moment before the write, which is the window a second writer
	// lands in. Re-read and refuse rather than discard their commit: an edit is
	// worth retrying, a lost commit is not. Skipped when no tag was presented,
	// because there is no baseline to compare against.
	if expectedTag != "" {
		current, err := os.ReadFile(full)
		if err != nil {
			return "", fmt.Errorf("re-read %q before writing: %w", relPath, err)
		}
		if nowTag := contentTag(current); nowTag != actualTag {
			return "", fmt.Errorf(
				"%s changed while this edit was being applied (tag %s is now %s); nothing was written, re-read it and try again",
				relPath, actualTag, nowTag)
		}
	}

	if err := os.WriteFile(full, newBytes, 0o644); err != nil {
		return "", fmt.Errorf("write %q: %w", relPath, err)
	}
	return fmt.Sprintf("[%s#%s] updated successfully (%d lines)", relPath, contentTag(newBytes), lines), nil
}

// applyPatchBytes splices operations over byte ranges, so a multi-megabyte file
// costs no per-line allocation.
func applyPatchBytes(data []byte, ops []patchOp) ([]byte, int, error) {
	units := bytes.SplitAfter(data, []byte{'\n'})
	if len(units) > 0 && len(units[len(units)-1]) == 0 {
		units = units[:len(units)-1]
	}
	starts := make([]int, len(units)+1)
	for i, unit := range units {
		starts[i+1] = starts[i] + len(unit)
	}
	lineCount := len(units)

	for _, op := range ops {
		switch op.kind {
		case opReplace, opDelete:
			if op.startLine < 1 || op.endLine > lineCount || op.startLine > op.endLine {
				return nil, 0, fmt.Errorf("invalid line range %d..%d for file of %d lines", op.startLine, op.endLine, lineCount)
			}
		case opInsertBefore, opInsertAfter:
			if op.startLine < 1 || op.startLine > lineCount {
				return nil, 0, fmt.Errorf("invalid insert target line %d for file of %d lines", op.startLine, lineCount)
			}
		case opAppend:
		}
	}

	// Descending order makes the overlap check independent of input order.
	sort.Slice(ops, func(i, j int) bool { return ops[i].startLine > ops[j].startLine })
	for i := range len(ops) - 1 {
		current, next := ops[i], ops[i+1]
		if current.kind != opAppend && next.endLine >= current.startLine {
			return nil, 0, fmt.Errorf("overlapping operations detected around lines %d-%d", next.endLine, current.startLine)
		}
	}
	ordered := append([]patchOp(nil), ops...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].startLine < ordered[j].startLine })

	capacity := len(data)
	for _, op := range ordered {
		for _, line := range op.body {
			capacity += len(line) + 1
		}
	}
	out := make([]byte, 0, capacity)
	cursor := 0
	for _, op := range ordered {
		switch op.kind {
		case opReplace:
			start, end := starts[op.startLine-1], starts[op.endLine]
			out = append(out, data[cursor:start]...)
			out = appendBody(out, op.body, end > start && data[end-1] == '\n')
			cursor = end
		case opDelete:
			start, end := starts[op.startLine-1], starts[op.endLine]
			out = append(out, data[cursor:start]...)
			cursor = end
		case opInsertBefore:
			start := starts[op.startLine-1]
			out = append(out, data[cursor:start]...)
			out = appendBody(out, op.body, true)
			cursor = start
		case opInsertAfter:
			end := starts[op.startLine]
			out = append(out, data[cursor:end]...)
			separator := end < len(data) || (len(data) > 0 && data[len(data)-1] == '\n')
			out = appendBody(out, op.body, separator)
			cursor = end
		case opAppend:
			out = append(out, data[cursor:]...)
			separator := len(data) > 0 && data[len(data)-1] == '\n'
			out = appendBody(out, op.body, separator)
			cursor = len(data)
		}
	}
	out = append(out, data[cursor:]...)
	newLines := bytes.Count(out, []byte{'\n'})
	if len(out) > 0 && out[len(out)-1] != '\n' {
		newLines++
	}
	return out, newLines, nil
}

func appendBody(dst []byte, body []string, trailingNewline bool) []byte {
	for i, line := range body {
		dst = append(dst, line...)
		if i < len(body)-1 || trailingNewline {
			dst = append(dst, '\n')
		}
	}
	return dst
}

func parseHashlineOps(text string) ([]patchOp, error) {
	var ops []patchOp
	var current *patchOp

	finish := func() error {
		if current == nil {
			return nil
		}
		if current.kind != opDelete && len(current.body) == 0 {
			return fmt.Errorf("PUT needs at least one body line starting with '+'; use CUT to delete")
		}
		ops = append(ops, *current)
		current = nil
		return nil
	}

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || headerRegex.MatchString(trimmed) {
			continue
		}

		switch {
		case strings.HasPrefix(trimmed, "PUT"):
			if err := finish(); err != nil {
				return nil, err
			}
			if m := putRangeRegex.FindStringSubmatch(trimmed); len(m) == 3 {
				s, _ := strconv.Atoi(m[1])
				e, _ := strconv.Atoi(m[2])
				current = &patchOp{kind: opReplace, startLine: s, endLine: e}
				continue
			}
			if m := putBeforeRegex.FindStringSubmatch(trimmed); len(m) == 2 {
				s, _ := strconv.Atoi(m[1])
				current = &patchOp{kind: opInsertBefore, startLine: s, endLine: s}
				continue
			}
			if m := putAfterRegex.FindStringSubmatch(trimmed); len(m) == 2 {
				s, _ := strconv.Atoi(m[1])
				current = &patchOp{kind: opInsertAfter, startLine: s, endLine: s}
				continue
			}
			if putTailRegex.MatchString(trimmed) {
				current = &patchOp{kind: opAppend, startLine: 999999, endLine: 999999}
				continue
			}
			return nil, fmt.Errorf("body lines must start with '+' on the next line; content never goes on the PUT line (got %q)", trimmed)

		case strings.HasPrefix(trimmed, "CUT"):
			if err := finish(); err != nil {
				return nil, err
			}
			if m := cutRangeRegex.FindStringSubmatch(trimmed); len(m) == 3 {
				s, _ := strconv.Atoi(m[1])
				e, _ := strconv.Atoi(m[2])
				ops = append(ops, patchOp{kind: opDelete, startLine: s, endLine: e})
				continue
			}
			if m := cutSingleRegex.FindStringSubmatch(trimmed); len(m) == 2 {
				s, _ := strconv.Atoi(m[1])
				ops = append(ops, patchOp{kind: opDelete, startLine: s, endLine: s})
				continue
			}
			return nil, fmt.Errorf("invalid CUT syntax: %q", trimmed)

		case strings.HasPrefix(line, "+"):
			if current == nil {
				return nil, fmt.Errorf("body line with no operation above it: %q", line)
			}
			current.body = append(current.body, line[1:])
			continue

		default:
			return nil, fmt.Errorf("body lines must start with '+', got %q", line)
		}
	}
	return ops, finish()
}

// ---------------------------------------------------------------------------
// read
// ---------------------------------------------------------------------------

// readPlain renders a source file with line numbers and its snapshot tag. The
// error names why a target is not rendered here.
//
// Nothing is delegated to pig. ctx.ExecuteTool("read") resolves back to THIS
// tool rather than to the host builtin — a read of a binary file returns the
// re-entrancy error rather than pig's rendering — so once this tool shadows
// read there is no route to its PDF/notebook/archive handling. Verified
// against pig 0.4.0; see scripts/read-precedence.sh.
func readPlain(baseDir, target string, offset, limit int) (string, error) {
	full, err := resolveInside(baseDir, target)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", fmt.Errorf("%s: %w", target, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", target)
	}
	// ponytail: whole-file ceiling. offset/limit do not bypass it, because edit
	// anchors on a tag over the whole file and a windowed tag would never
	// match. If multi-MB text ever has to be readable here, stream the window
	// and drop the tag for that path.
	if info.Size() > maxServedFile {
		return "", fmt.Errorf("%s is %d bytes, over the %d byte read limit; pull the part you need with bash", target, info.Size(), maxServedFile)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("%s: %w", target, err)
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return "", fmt.Errorf("%s is binary; read it with bash rather than with read", target)
	}

	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if offset < 1 {
		offset = 1
	}
	if limit <= 0 || limit > len(lines) {
		limit = len(lines)
	}
	end := offset - 1 + limit
	if end > len(lines) {
		end = len(lines)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[%s#%s] %d lines\n", target, contentTag(data), len(lines))
	for i := offset - 1; i < end; i++ {
		fmt.Fprintf(&b, "%6d→%s\n", i+1, lines[i])
	}
	return b.String(), nil
}

// ---------------------------------------------------------------------------
// paths
// ---------------------------------------------------------------------------

// resolveInside joins target onto baseDir and refuses anything that escapes it.
func resolveInside(baseDir, target string) (string, error) {
	if target == "" {
		return "", fmt.Errorf("no path given")
	}
	// filepath.Join would graft an already-absolute target onto the base and
	// yield a path that does not exist, so an absolute target is taken as
	// given — then checked against the base like any other.
	full := target
	if !filepath.IsAbs(target) {
		full = filepath.Join(baseDir, target)
	}
	rel, err := filepath.Rel(baseDir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q is outside the workspace", target)
	}
	return full, nil
}
