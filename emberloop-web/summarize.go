package web

import (
	"context"
	"encoding/json"
	"errors"
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
// Window is the input size, in characters, that one call takes for model.
type handoffSummarizer interface {
	Window(model string) int
	Summarize(model, system, user string) (string, error)
}

// errSummaryOverflow is the failure of a call whose input is over the model's
// context window. The writer folds the input and tries again.
var errSummaryOverflow = errors.New("summarizer input over the context window")

// pigSummarizer is the production summarizer: a bare pig SDK completion
// under the user's pig agent dir.
type pigSummarizer struct{}

func (pigSummarizer) Summarize(model, system, user string) (string, error) {
	return pigOneShot("", model, system, user, summaryMaxTokens, compressTimeout)
}

// summaryCharsPerToken turns a token count into the character budget of one
// call. It is a rough estimate; a provider that still refuses the input makes
// the writer fold again.
const summaryCharsPerToken = 4

// defaultSummaryWindow is the context window, in tokens, assumed for a model
// that the pig catalog does not list.
const defaultSummaryWindow = 128000

// Window is the input budget of one call to model: its context window less
// the reply cap, at summaryCharsPerToken characters per token.
func (pigSummarizer) Window(model string) int {
	tokens := catalogWindow(model)
	if tokens == 0 {
		tokens = defaultSummaryWindow
	}
	return (tokens - summaryMaxTokens) * summaryCharsPerToken
}

// catalogWindow is the context window, in tokens, that the pig catalog lists
// for model; 0 when it lists none.
func catalogWindow(model string) int {
	if m, ok := ai.LookupModel(model); ok {
		return m.ContextWindow
	}
	return 0
}

// pigOneShot is one bare SDK completion: the model's own provider, a system
// prompt, one user message and a token cap. No Runtime, no Session, no tools,
// no session file. An empty agentDir means the user's pig agent dir. Input the
// provider refuses for its window fails with errSummaryOverflow.
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
	window := catalogWindow(model)
	stream, err := m.Provider.Stream(ctx, tc, ai.StreamOptions{MaxTokens: maxTokens, Thinking: ai.ThinkingOff})
	if err != nil {
		return "", overflowOr(err, window)
	}
	msg, err := stream.ResultContext(ctx)
	if err != nil {
		return "", overflowOr(err, window)
	}
	if ai.IsContextOverflow(*msg, window) {
		return "", errSummaryOverflow
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

// overflowOr is errSummaryOverflow when the failure text names a context
// overflow in a provider's wording, else err itself.
func overflowOr(err error, window int) error {
	msg := ai.AssistantMessage{StopReason: ai.StopReasonError, ErrorMessage: err.Error()}
	if ai.IsContextOverflow(msg, window) {
		return errSummaryOverflow
	}
	return err
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

// handoffRefolds bounds how often the writer folds the Handoff input again,
// at half its size, after the summarizer refused the Handoff call.
const handoffRefolds = 2

// writeHandoff asks the summarizer for the Handoff body. Input over the window
// is folded first (summarize_fold.go). A reply that is empty or in the wrong
// format gets up to handoffRepairs repair retries that restate the sections.
// When no reply passes, the last non-empty reply is wrapped under the
// sections. With no usable text at all, the deterministic Handoff is used.
// Each fold and fallback step is reported to note with its reason.
func writeHandoff(sum handoffSummarizer, model, instructions string, events []Event, note func(string)) handoffResult {
	system := summaryPrompt(instructions)
	f := folder{sum: sum, model: model, instructions: instructions, note: note}
	blocks := summaryBlocks(events)
	budget := sum.Window(model) - len(system) - len(foldPrompt(instructions))
	for refolds := 0; ; refolds++ {
		text, err := f.fit(blocks, budget)
		if errors.Is(err, errNoFit) {
			note("deterministic: input does not fit the summarizer window")
			return deterministicResult(events, 0, "")
		}
		if err != nil {
			note("deterministic: summarizer failed: " + err.Error())
			return deterministicResult(events, 0, err.Error())
		}
		res, err := handoffChain(sum, model, system, text, events, note)
		if !errors.Is(err, errSummaryOverflow) {
			return res
		}
		if refolds == handoffRefolds {
			note(fmt.Sprintf("deterministic: handoff input still over the window after %d refolds", handoffRefolds))
			return deterministicResult(events, 0, "")
		}
		budget = len(text) / 2
		note(fmt.Sprintf("fold: handoff input of %d chars over the window; folding again at %d chars", len(text), budget))
	}
}

// handoffChain asks for the Handoff body over text, with the repair retries
// and fallbacks. A call the window refuses returns errSummaryOverflow. Any
// other call failure ends at the deterministic Handoff with the error text.
func handoffChain(sum handoffSummarizer, model, system, text string, events []Event, note func(string)) (handoffResult, error) {
	transcript := summaryUser(text)
	user := transcript
	var last string
	for attempt := 0; ; attempt++ {
		reply, err := sum.Summarize(model, system, user)
		if errors.Is(err, errSummaryOverflow) {
			return handoffResult{}, err
		}
		if err != nil {
			note("deterministic: summarizer failed: " + err.Error())
			return deterministicResult(events, attempt, err.Error()), nil
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
			return handoffResult{text: assembleHandoff(reply, events), fallback: step, retries: attempt}, nil
		}
		if attempt == handoffRepairs {
			break
		}
		why := "reply missed the sections"
		if reply == "" {
			why = "reply empty"
		}
		note(fmt.Sprintf("repair %d of %d: %s", attempt+1, handoffRepairs, why))
		user = transcript + "\n\n" + repairNote(reply)
	}
	if last != "" {
		note(fmt.Sprintf("lenient: no usable reply after %d repairs; wrapped under the sections", handoffRepairs))
		return handoffResult{text: assembleHandoff(lenientWrap(last), events), fallback: stepLenient, retries: handoffRepairs}, nil
	}
	note(fmt.Sprintf("deterministic: no usable reply after %d repairs", handoffRepairs))
	return deterministicResult(events, handoffRepairs, ""), nil
}

// deterministicResult is the Handoff built from the Event log alone. errText
// is the summarizer error when a call failed, else empty.
func deterministicResult(events []Event, retries int, errText string) handoffResult {
	return handoffResult{text: deterministicHandoff(events), fallback: stepDeterministic, retries: retries, err: errText}
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

// summaryBlocks renders the Event log for the summarizer, in log order:
// prompts, assistant replies, tool calls (last state) and errors. A tool
// result is cut to head and tail here, and cut harder at the fold floor.
func summaryBlocks(events []Event) []summaryBlock {
	var blocks []summaryBlock
	toolAt := map[string]int{}
	var reply strings.Builder
	flush := func() {
		if reply.Len() > 0 {
			blocks = append(blocks, summaryBlock{text: "ASSISTANT:\n" + dropMemoryBlocks(strings.TrimSpace(reply.String()))})
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
			blocks = append(blocks, summaryBlock{text: label + text})
		case "error":
			flush()
			blocks = append(blocks, summaryBlock{text: "ERROR:\n" + dropMemoryBlocks(e.Text)})
		case "tool_call_started", "tool_call_updated":
			flush()
			var tc ToolCall
			if json.Unmarshal([]byte(e.Text), &tc) != nil {
				break
			}
			block := summaryBlock{
				text:   dropMemoryBlocks(fmt.Sprintf("TOOL %s (%s)\ninput: %s\noutput: ", tc.Name, tc.Status, tc.Input)),
				result: capHeadTail(dropMemoryBlocks(tc.Output), toolResultCap),
			}
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
	return blocks
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
