package web

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ReviewFinding is one reviewer finding: the file and line plus the
// severity and problem text. It mirrors the submit tool's finding object
// `{file, line, severity, problem}` (severity blocker|major|minor) and the
// legacy gate prompt's finding line format `file:line severity: problem`.
type ReviewFinding struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Problem  string `json:"problem"`
}

// ReviewRan is one command the reviewer ran, with its exit code. A pass
// verdict must name at least one, so a pass without evidence is refused.
type ReviewRan struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exitCode"`
}

// UnmarshalJSON accepts both `exitCode` and the short `exit` for the exit
// code, so hand-written submits do not fail on the key name.
func (r *ReviewRan) UnmarshalJSON(data []byte) error {
	var raw struct {
		Command  string `json:"command"`
		ExitCode *int   `json:"exitCode"`
		Exit     *int   `json:"exit"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.Command = raw.Command
	if raw.ExitCode != nil {
		r.ExitCode = *raw.ExitCode
	} else if raw.Exit != nil {
		r.ExitCode = *raw.Exit
	}
	return nil
}

// MarshalJSON writes the canonical `command` and `exitCode` keys.
func (r ReviewRan) MarshalJSON() ([]byte, error) {
	type plain ReviewRan
	return json.Marshal(plain(r))
}

// ReviewerSubmit is the reviewer `submit` tool payload: the verdict, the
// full tip SHA it judges, the commands run with exit codes, and the
// findings. It is the typed replacement for the legacy `VERDICT: pass|changes`
// free-text reply, which stays only as a fallback for a Backend without the
// tool.
type ReviewerSubmit struct {
	Verdict  string          `json:"verdict"`
	Tip      string          `json:"tip"`
	Ran      []ReviewRan     `json:"ran"`
	Findings []ReviewFinding `json:"findings"`
}

// ValidateReviewerSubmit checks one reviewer submit payload for the tool
// error cases: unknown verdict, empty tip, unknown severity, a `changes`
// without a blocker or major, and a `pass` without `ran`. A stale tip is not
// an error: it records and does not count, judged by the caller against the
// current branch tip.
func ValidateReviewerSubmit(s ReviewerSubmit) error {
	v := strings.ToLower(strings.TrimSpace(s.Verdict))
	if v != "pass" && v != "changes" {
		return fmt.Errorf("verdict must be pass or changes")
	}
	if strings.TrimSpace(s.Tip) == "" {
		return fmt.Errorf("tip is required: the full SHA you judged")
	}
	for i, f := range s.Findings {
		sev := strings.ToLower(strings.TrimSpace(f.Severity))
		if sev != "blocker" && sev != "major" && sev != "minor" {
			return fmt.Errorf("finding %d: severity must be blocker, major or minor", i)
		}
		if strings.TrimSpace(f.File) == "" || strings.TrimSpace(f.Problem) == "" {
			return fmt.Errorf("finding %d: file and problem are required", i)
		}
	}
	for i, r := range s.Ran {
		if strings.TrimSpace(r.Command) == "" {
			return fmt.Errorf("ran %d: command is required", i)
		}
	}
	if v == "changes" && !hasBlockerOrMajor(s.Findings) {
		return fmt.Errorf("changes requires at least one blocker or major finding")
	}
	if v == "pass" && len(s.Ran) == 0 {
		return fmt.Errorf("pass requires ran: name the commands you ran with their exit codes")
	}
	return nil
}

// hasBlockerOrMajor reports a blocker or major in the findings, case-insensitive.
func hasBlockerOrMajor(fs []ReviewFinding) bool {
	for _, f := range fs {
		if sev := strings.ToLower(strings.TrimSpace(f.Severity)); sev == "blocker" || sev == "major" {
			return true
		}
	}
	return false
}

// ParsedReview is the Daemon's reading of a reviewer's final assistant
// message: the verdict plus its findings. OK is false when no VERDICT line
// or a malformed finding line was found, in which case the Daemon
// re-prompts the reviewer once for the format. It stays only as a fallback
// for a Backend without the submit tool.
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
