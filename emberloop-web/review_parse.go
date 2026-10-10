package web

import (
	"regexp"
	"strconv"
	"strings"
)

// ReviewFinding is one parsed reviewer finding: the file and line plus the
// severity and problem text. It mirrors the gate prompt's finding line
// format `file:line severity: problem` (severity blocker|major|minor).
type ReviewFinding struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Problem  string `json:"problem"`
}

// ParsedReview is the Daemon's reading of a reviewer's final assistant
// message: the verdict plus its findings. OK is false when no VERDICT line
// or a malformed finding line was found, in which case the Daemon
// re-prompts the reviewer once for the format.
type ParsedReview struct {
	Verdict  string
	Findings []ReviewFinding
	OK       bool
}

var (
	reviewVerdictLine = regexp.MustCompile(`(?i)^\s*VERDICT\s*:\s*(pass|changes)\s*$`)
	// A finding attempt: file:line <word>: problem. The word must be a
	// known severity; anything else is a format error, not prose.
	reviewFindingLine = regexp.MustCompile(`^\s*(\S+):(\d+)\s+(\S+)\s*:\s*(.+?)\s*$`)
)

// ParseReview reads a reviewer's final assistant message. The verdict is
// the first own-line `VERDICT: pass|changes` (case-insensitive); findings
// are the `file:line severity: problem` lines after it. Surrounding prose
// and blank lines are ignored, CRLF included; an indented non-finding line
// continues the previous finding's problem. A finding-shaped line with an
// unknown severity, or no verdict line at all, is not OK.
func ParseReview(text string) ParsedReview {
	norm := strings.ReplaceAll(text, "\r\n", "\n")
	norm = strings.ReplaceAll(norm, "\r", "\n")
	lines := strings.Split(norm, "\n")
	vi := -1
	verdict := ""
	for i, ln := range lines {
		if m := reviewVerdictLine.FindStringSubmatch(ln); m != nil {
			vi = i
			verdict = strings.ToLower(m[1])
			break
		}
	}
	if vi < 0 {
		return ParsedReview{OK: false}
	}
	var out []ReviewFinding
	for _, ln := range lines[vi+1:] {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		if m := reviewFindingLine.FindStringSubmatch(ln); m != nil {
			sev := strings.ToLower(m[3])
			if sev != "blocker" && sev != "major" && sev != "minor" {
				return ParsedReview{OK: false}
			}
			n, _ := strconv.Atoi(m[2])
			prob := strings.TrimSpace(m[4])
			if prob == "" {
				return ParsedReview{OK: false}
			}
			out = append(out, ReviewFinding{File: m[1], Line: n, Severity: sev, Problem: prob})
			continue
		}
		// Continuation of a multi-line problem: an indented line that is
		// not itself a finding belongs to the previous finding.
		if len(out) > 0 && (strings.HasPrefix(ln, " ") || strings.HasPrefix(ln, "\t")) {
			t := strings.TrimSpace(ln)
			if t != "" {
				out[len(out)-1].Problem += "\n" + t
			}
			continue
		}
		// Anything else is prose around the format: ignored.
	}
	return ParsedReview{Verdict: verdict, Findings: out, OK: true}
}

// reviewReport renders findings as the refusal/report text the gate and the
// worker tell carry: one `file:line severity: problem` line each.
func reviewReport(fs []ReviewFinding) string {
	lines := make([]string, 0, len(fs))
	for _, f := range fs {
		prob := strings.ReplaceAll(f.Problem, "\n", " / ")
		lines = append(lines, f.File+":"+strconv.Itoa(f.Line)+" "+f.Severity+": "+prob)
	}
	return strings.Join(lines, "\n")
}
