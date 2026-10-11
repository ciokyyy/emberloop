package web

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	el "example.com/emberloop"
	"github.com/MichaelKinsy/PiG/ai"
)

// The Handoff summarizer is a one-off pig completion with no tools and no
// Session behind it. It writes the Handoff body from the Event log.

// summaryMaxTokens caps one summarizer reply; a Handoff runs long.
const summaryMaxTokens = 8192

// handoffSummarizer is the one-off completion seam: one system prompt, one
// user message, the whole reply back. Implementations run with no tools.
type handoffSummarizer interface {
	Summarize(model, system, user string) (string, error)
}

// pigSummarizer is the production summarizer: a bare pig SDK completion
// under the user's pig agent dir.
type pigSummarizer struct{}

func (pigSummarizer) Summarize(model, system, user string) (string, error) {
	return pigOneShot("", model, system, user, summaryMaxTokens, compressTimeout)
}

// pigOneShot is one bare SDK completion: the model's own provider, a system
// prompt, one user message and a token cap. No Runtime, no Session, no tools,
// no session file. An empty agentDir means the user's pig agent dir.
func pigOneShot(agentDir, model, system, user string, maxTokens int, timeout time.Duration) (string, error) {
	svcs, m, err := buildPigServices(agentDir, nil, SessionMeta{Model: model, WorkDir: os.TempDir()})
	if err != nil {
		return "", err
	}
	defer svcs.Close()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	tc := ai.NormalizeContext(ai.Context{
		SystemPrompt: system,
		Messages:     []ai.Message{ai.UserMessage{Content: ai.UserText(user), Timestamp: time.Now().UnixMilli()}},
	})
	stream, err := m.Provider.Stream(ctx, tc, ai.StreamOptions{MaxTokens: maxTokens, Thinking: ai.ThinkingOff})
	if err != nil {
		return "", err
	}
	msg, err := stream.ResultContext(ctx)
	if err != nil {
		return "", err
	}
	if msg.StopReason == ai.StopReasonError || msg.StopReason == ai.StopReasonAborted {
		return "", fmt.Errorf("pig one-off: %s", msg.ErrorMessage)
	}
	var sb strings.Builder
	for _, block := range msg.Content {
		switch t := block.(type) {
		case ai.TextContent:
			sb.WriteString(t.Text)
		case *ai.TextContent:
			sb.WriteString(t.Text)
		}
	}
	return sb.String(), nil
}

// compressNote is the reason Compress cannot run on a Handoff Backend, or
// empty when it can: the compress model is unset, or it is not an authed pig
// model.
func compressNote(model string) string {
	if model == "" {
		return "no compress model: pick one in Settings"
	}
	if !pigModelAuthed(model) {
		return "compress model " + model + " is not an authed pig model"
	}
	return ""
}

// pigModelAuthed reports whether model is a pig catalog model whose provider
// holds usable auth in the user's pig agent dir. It is the same test the
// new-Session picker applies, for one model.
func pigModelAuthed(model string) bool {
	if isTestFauxModel(model) {
		return true
	}
	if _, ok := ai.LookupModel(model); !ok {
		return false
	}
	provider, _, _ := strings.Cut(model, "/")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return pigProviderAuthed(ctx, provider, pigCredentials(""))
}

// Handoff step names: the fallback step a compressed Event records.
const (
	stepSummary       = "summary"
	stepRepair        = "repair"
	stepLenient       = "lenient"
	stepDeterministic = "deterministic"
)

// handoffResult is one Handoff text and the step that wrote it.
type handoffResult struct {
	text     string
	fallback string
	retries  int
	err      string
}

// handoffRepairs is the number of repair retries after the first reply.
const handoffRepairs = 2

// writeHandoff asks the summarizer for the Handoff body. An empty or wrong-format
// reply gets up to handoffRepairs repair retries that restate the sections. When
// no reply passes, the last non-empty reply is wrapped under the sections. With
// no usable text at all, the deterministic Handoff is used.
func writeHandoff(sum handoffSummarizer, model, instructions string, events []Event) handoffResult {
	transcript := summaryUser(summaryTranscript(events))
	system := summaryPrompt(instructions)
	user := transcript
	var last string
	for attempt := 0; ; attempt++ {
		reply, err := sum.Summarize(model, system, user)
		if err != nil {
			return handoffResult{text: deterministicHandoff(events), fallback: stepDeterministic, retries: attempt, err: err.Error()}
		}
		reply = strings.TrimSpace(dropMemoryBlocks(reply))
		if reply != "" {
			last = reply
		}
		if el.CheckHandoff(reply) == nil {
			step := stepSummary
			if attempt > 0 {
				step = stepRepair
			}
			return handoffResult{text: assembleHandoff(reply, events), fallback: step, retries: attempt}
		}
		if attempt == handoffRepairs {
			break
		}
		user = transcript + "\n\n" + repairNote(reply)
	}
	if last != "" {
		return handoffResult{text: assembleHandoff(lenientWrap(last), events), fallback: stepLenient, retries: handoffRepairs}
	}
	return handoffResult{text: deterministicHandoff(events), fallback: stepDeterministic, retries: handoffRepairs}
}

// repairNote asks for the whole handoff again and restates the required
// sections. A non-empty previous reply rides along for reference.
func repairNote(prev string) string {
	var b strings.Builder
	b.WriteString("Your previous reply was not a usable handoff. Reply with the whole handoff again.\n")
	b.WriteString("Use exactly these sections, in this order, with no text before the first one:\n")
	for _, s := range el.HandoffSections() {
		b.WriteString(s + "\n")
	}
	if p := strings.TrimSpace(prev); p != "" {
		b.WriteString("\nPrevious reply, for reference:\n" + capText(p, handoffFieldCap))
	}
	return b.String()
}

// lenientWrap places a reply that misses sections under the required headings.
// A reply with no heading goes under Task. Each missing section gets a stub
// that points at the text above it.
func lenientWrap(text string) string {
	sections := el.HandoffSections()
	missing := missingSections(text)
	var b strings.Builder
	if len(missing) == len(sections) {
		b.WriteString(sections[0] + "\n" + text + "\n")
		missing = missing[1:]
	} else {
		b.WriteString(text + "\n")
	}
	for _, s := range missing {
		b.WriteString("\n" + s + "\nNot split out by the summarizer. See above.\n")
	}
	return b.String()
}

// missingSections lists the required sections that text lacks, in order.
func missingSections(text string) []string {
	var missing []string
	for _, s := range el.HandoffSections() {
		if !strings.Contains(text, s) {
			missing = append(missing, s)
		}
	}
	return missing
}

// summaryPrompt is the summarizer system prompt: the pinned recipe, then the
// compress instructions when there are any.
func summaryPrompt(instructions string) string {
	prompt := el.CompressPrompt
	if ins := strings.TrimSpace(instructions); ins != "" {
		prompt += "\nFocus from the user for this compress:\n" + ins + "\n"
	}
	return prompt
}

// summaryUser is the summarizer's one user message: the transcript.
func summaryUser(transcript string) string {
	return "Session transcript:\n" + transcript
}

// summaryTranscript renders the Event log for the summarizer, in log order:
// prompts, assistant replies, tool calls (last state) and errors.
func summaryTranscript(events []Event) string {
	var blocks []string
	toolAt := map[string]int{}
	var reply strings.Builder
	flush := func() {
		if reply.Len() > 0 {
			blocks = append(blocks, "ASSISTANT:\n"+dropMemoryBlocks(strings.TrimSpace(reply.String())))
			reply.Reset()
		}
	}
	for _, e := range events {
		switch e.Kind {
		case "assistant_chunk":
			reply.WriteString(e.Text)
		case "prompt":
			flush()
			label, text := "USER:\n", e.Text
			if e.Sender == "system" || e.Handoff {
				label, text = "SYSTEM:\n", dropMemoryBlocks(e.Text)
			}
			blocks = append(blocks, label+text)
		case "error":
			flush()
			blocks = append(blocks, "ERROR:\n"+dropMemoryBlocks(e.Text))
		case "tool_call_started", "tool_call_updated":
			flush()
			var tc ToolCall
			if json.Unmarshal([]byte(e.Text), &tc) != nil {
				break
			}
			block := dropMemoryBlocks(fmt.Sprintf("TOOL %s (%s)\ninput: %s\noutput: %s", tc.Name, tc.Status, tc.Input, tc.Output))
			if i, ok := toolAt[tc.ID]; ok && tc.ID != "" {
				blocks[i] = block
			} else {
				if tc.ID != "" {
					toolAt[tc.ID] = len(blocks)
				}
				blocks = append(blocks, block)
			}
		}
	}
	flush()
	return strings.Join(blocks, "\n\n")
}

// assembleHandoff frames a summary body as the Handoff text, with the last
// user prompt verbatim after it.
func assembleHandoff(body string, events []Event) string {
	var b strings.Builder
	b.WriteString(handoffHeader + "\n" + body + "\n")
	if p := lastUserPrompt(events); p != "" {
		b.WriteString("\n## Last user prompt (verbatim)\n" + p + "\n")
	}
	return el.FinishHandoff(b.String(), openTask(events))
}

// lastUserPrompt is the newest prompt the user typed, verbatim.
func lastUserPrompt(events []Event) string {
	for i := len(events) - 1; i >= 0; i-- {
		if e := events[i]; e.Kind == "prompt" && e.Sender != "system" && !e.Handoff {
			return e.Text
		}
	}
	return ""
}

// deterministicHandoff is the Handoff built from the Event log alone.
func deterministicHandoff(events []Event) string {
	return el.FinishHandoff(buildHandoff(events), openTask(events))
}

// openTask reports whether a tool call started and never finished. The
// Session was compressed mid-step, so the Handoff ends with the continue line.
func openTask(events []Event) bool {
	open := map[string]bool{}
	for _, e := range events {
		if e.Kind != "tool_call_started" && e.Kind != "tool_call_updated" {
			continue
		}
		var tc ToolCall
		if json.Unmarshal([]byte(e.Text), &tc) != nil || tc.ID == "" {
			continue
		}
		open[tc.ID] = tc.Status != "completed" && tc.Status != "failed"
	}
	for _, o := range open {
		if o {
			return true
		}
	}
	return false
}

// dropMemoryBlocks removes fenced memory blocks. A line that starts with
// "```memory" opens one, and the next line that is exactly "```" closes it.
// An unclosed block drops the rest of the text.
func dropMemoryBlocks(text string) string {
	var kept []string
	inBlock := false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case inBlock:
			if trimmed == "```" {
				inBlock = false
			}
		case strings.HasPrefix(trimmed, "```memory"):
			inBlock = true
		default:
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}
