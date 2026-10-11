package web

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// ticketBox is one checkbox in a ticket file as the ticket page shows it:
// the line text plus whether it ticks.
type ticketBox struct {
	Text    string `json:"text"`
	Checked bool   `json:"checked"`
}

// ticketAgent is one session on a ticket: its worker, a reviewer, or a
// scout the worker spawned. Role is worker, reviewer-spec,
// reviewer-standards or scout; Name is the session title.
type ticketAgent struct {
	SessionID string `json:"sessionId"`
	Role      string `json:"role"`
	Harness   string `json:"harness"`
	Status    string `json:"status"`
	Name      string `json:"name"`
}

// ticketRound is one review round on a ticket: one prompted tip with its
// verdicts. N counts from 1 per ticket. A pass carried without a rerun
// appears as HeldFromRound pointing at the round that ran it, never as a
// fresh verdict. SentFindings records each finding delivered to the Worker
// and its optional or required Disposition; Declined is legacy display data.
type ticketRound struct {
	N              int                    `json:"n"`
	Tip            string                 `json:"tip"`
	StartedAt      time.Time              `json:"startedAt"`
	EndedAt        *time.Time             `json:"endedAt,omitempty"`
	Reviews        map[string]roundReview `json:"reviews,omitempty"`
	SentFindings   []workerReviewFinding  `json:"sentFindings,omitempty"`
	Declined       []string               `json:"declined,omitempty"`
	SentToWorkerAt *time.Time             `json:"sentToWorkerAt,omitempty"`
}

// roundReview is one reviewer type's verdict inside a round.
type roundReview struct {
	Verdict       string          `json:"verdict"`
	Findings      []ReviewFinding `json:"findings,omitempty"`
	HeldFromRound *int            `json:"heldFromRound,omitempty"`
	At            string          `json:"at"`
	DurationMs    int64           `json:"durationMs,omitempty"`
}

// ReviewDisposition is one Worker's decision about a finding sent by the
// Run driver. Declined requires a reason; deferred requires a Ticket number.
type ReviewDisposition struct {
	FindingID   string `json:"findingId"`
	Disposition string `json:"disposition"`
	Reason      string `json:"reason,omitempty"`
	Ticket      string `json:"ticket,omitempty"`
}

// workerReviewFinding is the review finding as delivered to a Worker and
// recorded on its review round. Blocking is true only for blocker and major.
type workerReviewFinding struct {
	FindingID   string             `json:"findingId"`
	Reviewer    string             `json:"reviewer"`
	File        string             `json:"file"`
	Line        int                `json:"line"`
	Severity    string             `json:"severity"`
	Problem     string             `json:"problem"`
	Blocking    bool               `json:"blocking"`
	Disposition *ReviewDisposition `json:"disposition,omitempty"`
}

// declinedReviewFinding is a prior Worker disposition included in later
// Reviewer prompts. It retains the original citation and problem alongside
// the reason the Worker declined it.
type declinedReviewFinding struct {
	Round     int    `json:"round"`
	FindingID string `json:"findingId"`
	Reviewer  string `json:"reviewer"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	Severity  string `json:"severity"`
	Problem   string `json:"problem"`
	Reason    string `json:"reason"`
}

// declinedFindingsForRounds returns the Ticket's declined findings in round
// order, so every later review can see the original citation and reason.
func declinedFindingsForRounds(rounds []ticketRound) []declinedReviewFinding {
	var out []declinedReviewFinding
	for _, round := range rounds {
		for _, finding := range round.SentFindings {
			if finding.Disposition == nil ||
				!strings.EqualFold(strings.TrimSpace(finding.Disposition.Disposition), "declined") {
				continue
			}
			out = append(out, declinedReviewFinding{
				Round: round.N, FindingID: finding.FindingID, Reviewer: finding.Reviewer,
				File: finding.File, Line: finding.Line, Severity: finding.Severity,
				Problem: finding.Problem, Reason: finding.Disposition.Reason,
			})
		}
	}
	return out
}

// workerFindingID names one finding by the round and reviewer that produced
// it. The index is one-based and stable for that recorded verdict.
func workerFindingID(round int, reviewer string, index int) string {
	return fmt.Sprintf("r%d-%s-%d", round, reviewer, index+1)
}

// workerFindingsForRound creates the ordered, stable set delivered to the
// Worker: every finding, including non-blocking minors, from recorded verdicts.
func workerFindingsForRound(round ticketRound) []workerReviewFinding {
	var out []workerReviewFinding
	for _, reviewer := range requiredReviewerTypes {
		review, ok := round.Reviews[reviewer]
		if !ok || review.Verdict == "" {
			continue
		}
		for i, f := range review.Findings {
			severity := strings.ToLower(strings.TrimSpace(f.Severity))
			out = append(out, workerReviewFinding{
				FindingID: workerFindingID(round.N, reviewer, i), Reviewer: reviewer,
				File: f.File, Line: f.Line, Severity: severity, Problem: f.Problem,
				Blocking: severity == "blocker" || severity == "major",
			})
		}
	}
	return out
}

// workerReviewFeedback is the exact prompt sent with a closed review round.
// IDs let the Worker attach each submit Disposition to one finding.
func workerReviewFeedback(tip string, findings []workerReviewFinding) string {
	if len(findings) == 0 {
		return ""
	}
	raw, err := json.MarshalIndent(findings, "", "  ")
	if err != nil {
		return ""
	}
	return fmt.Sprintf("Recorded review findings at tip %s (blocking is true for blocker/major and false for minor):\n%s\nFor every blocking finding, include a Disposition in your next done submit. Minor Dispositions are optional. Use dispositions [{findingId, disposition}], where disposition is fixed, declined (also supply reason), or deferred (also supply ticket as a Ticket number). Copy each findingId exactly. A pass with minors does not block the Ticket.", tip, raw)
}

// validateWorkerDispositions validates a submit against the findings this
// Worker actually received. Every unanswered blocker/major is mandatory;
// minors may be omitted. It returns canonical values keyed by finding id.
func validateWorkerDispositions(findings []workerReviewFinding, submitted []ReviewDisposition) (map[string]ReviewDisposition, error) {
	byID := make(map[string]workerReviewFinding, len(findings))
	for _, finding := range findings {
		byID[finding.FindingID] = finding
	}
	out := make(map[string]ReviewDisposition, len(submitted))
	for i, d := range submitted {
		d.FindingID = strings.TrimSpace(d.FindingID)
		d.Disposition = strings.ToLower(strings.TrimSpace(d.Disposition))
		d.Reason = strings.TrimSpace(d.Reason)
		d.Ticket = strings.TrimSpace(d.Ticket)
		if d.FindingID == "" {
			return nil, fmt.Errorf("disposition %d: findingId is required", i)
		}
		if _, ok := byID[d.FindingID]; !ok {
			return nil, fmt.Errorf("disposition %d: findingId %q was not sent to the Worker", i, d.FindingID)
		}
		if _, ok := out[d.FindingID]; ok {
			return nil, fmt.Errorf("disposition for finding %q is duplicated", d.FindingID)
		}
		switch d.Disposition {
		case "fixed":
		case "declined":
			if d.Reason == "" {
				return nil, fmt.Errorf("disposition for finding %q: declined requires a reason", d.FindingID)
			}
		case "deferred":
			if d.Ticket == "" {
				return nil, fmt.Errorf("disposition for finding %q: deferred requires a Ticket number", d.FindingID)
			}
		default:
			return nil, fmt.Errorf("disposition for finding %q must be fixed, declined or deferred", d.FindingID)
		}
		out[d.FindingID] = d
	}
	for _, finding := range findings {
		if finding.Blocking && finding.Disposition == nil {
			if _, ok := out[finding.FindingID]; !ok {
				return nil, fmt.Errorf("missing Disposition for blocking finding %s (%s:%d)", finding.FindingID, finding.File, finding.Line)
			}
		}
	}
	return out, nil
}

// parseTicketBoxes reads checkbox lines the way the driver's tick does: a
// `- [ ]`/`- [x]` (or `*`) marker, text after it trimmed. Only real marker
// lines count, so prose mentioning brackets never ticks.
func parseTicketBoxes(content string) []ticketBox {
	var out []ticketBox
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- [") && !strings.HasPrefix(trimmed, "* [") {
			continue
		}
		mark := trimmed[2:]
		switch {
		case strings.HasPrefix(mark, "[ ]"):
			out = append(out, ticketBox{Text: strings.TrimSpace(mark[len("[ ]"):]), Checked: false})
		case strings.HasPrefix(mark, "[x]"), strings.HasPrefix(mark, "[X]"):
			out = append(out, ticketBox{Text: strings.TrimSpace(mark[len("[x]"):]), Checked: true})
		}
	}
	if out == nil {
		out = []ticketBox{}
	}
	return out
}

// parseTicketTitle reads the ticket's first `# ...` heading with a leading
// number stripped, the same rule the spec preview titles by. Empty when the
// file holds no heading; callers fall back to the recorded preview title.
func parseTicketTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "# ") {
			continue
		}
		head := strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		i := 0
		for i < len(head) && head[i] >= '0' && head[i] <= '9' {
			i++
		}
		if i > 0 && i < len(head) && (head[i] == ':' || head[i] == '-' || head[i] == '.') {
			head = strings.TrimSpace(head[i+1:])
		}
		return head
	}
	return ""
}

// parseFollowups returns the text a worker submit named after `Follow-ups:`.
// Empty when the submit names none: no marker, nothing after it, or nothing
// but none/nothing. Mirrors the core driver's follow-ups rule.
func parseFollowups(text string) string {
	i := strings.Index(text, "Follow-ups:")
	if i < 0 {
		return ""
	}
	note := strings.TrimSpace(text[i+len("Follow-ups:"):])
	if note == "" || strings.EqualFold(note, "none") || strings.EqualFold(note, "nothing") {
		return ""
	}
	return note
}

// readTicketFile returns the ticket file's current content for the live
// table: boxes, body and title re-read at snapshot time, so driver ticks
// show without a poll. Missing files read empty; callers fall back.
func readTicketFile(path string) string {
	if path == "" {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(raw)
}

// reviewerReplyText joins the assistant chunks after the last prompt in
// order, keeping line breaks: the reviewer's VERDICT and finding lines
// only parse from the raw text. (lastAssistantReply flattens to one line
// for group delivery, which would merge the verdict with its findings.)
func reviewerReplyText(events []Event) string {
	start := 0
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == "prompt" {
			start = i + 1
			break
		}
	}
	var b strings.Builder
	for _, e := range events[start:] {
		if e.Kind == "assistant_chunk" {
			b.WriteString(e.Text)
		}
	}
	return b.String()
}
