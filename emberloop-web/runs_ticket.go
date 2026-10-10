package web

import (
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
// fresh verdict. Declined holds the changes findings sent back, with the
// time they reached the worker.
type ticketRound struct {
	N              int                    `json:"n"`
	Tip            string                 `json:"tip"`
	StartedAt      time.Time              `json:"startedAt"`
	EndedAt        *time.Time             `json:"endedAt,omitempty"`
	Reviews        map[string]roundReview `json:"reviews,omitempty"`
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
