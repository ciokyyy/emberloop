package web

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Oversize input folding. The Handoff input is the Event log as blocks. Input
// over the summarizer window is summarized in chunks, and the chunk summaries
// are packed and folded the same way until the Handoff call can read them. A
// chunk the summarizer refuses is halved while it is over foldFloorChars; at
// the floor its tool results are cut to toolFloorCap characters of head and
// the chunk is sent once more. Input that still does not fit gives errNoFit,
// and the caller uses the deterministic Handoff. Sizes count bytes.

const (
	// toolResultCap is the most characters kept of one tool result, head and
	// tail together.
	toolResultCap = 2000
	// toolFloorCap is the most characters kept of one tool result at the fold
	// floor, head only.
	toolFloorCap = 500
	// foldFloorChars is the size at which a refused chunk stops halving.
	foldFloorChars = 8000
)

// errEmptyChunk is the failure of a chunk call whose reply has no text.
var errEmptyChunk = errors.New("chunk summary came back empty")

// errNoFit is the failure of input that does not fit the window, even folded.
var errNoFit = errors.New("input does not fit the summarizer window")

// summaryBlock is one Event as summarizer input. A tool call keeps its result
// apart from its label, so the result can be cut harder at the floor.
type summaryBlock struct {
	text   string // label and text; a tool call's label ends with "output: "
	result string // a tool call's result, already cut to toolResultCap
}

// size is the block's length in the rendered text, separator included.
func (b summaryBlock) size() int {
	return len(b.text) + len(b.result) + 2
}

// render is the block as summarizer text. At the fold floor a tool result is
// cut to toolFloorCap characters of head.
func (b summaryBlock) render(atFloor bool) string {
	if atFloor {
		return b.text + capText(b.result, toolFloorCap)
	}
	return b.text + b.result
}

// renderBlocks joins blocks in log order, each separated by a blank line.
func renderBlocks(blocks []summaryBlock, atFloor bool) string {
	parts := make([]string, len(blocks))
	for i, b := range blocks {
		parts[i] = b.render(atFloor)
	}
	return strings.Join(parts, "\n\n")
}

// folder runs the summarizer over input that does not fit one call.
type folder struct {
	sum          handoffSummarizer
	model        string
	instructions string
	note         func(string)
}

// fit returns the Handoff input for blocks: the rendered blocks when they fit
// budget characters. Otherwise the blocks are packed into chunks, each chunk is
// summarized, and the chunk summaries are folded the same way until they fit.
// errNoFit means folding did not make the input fit.
func (f folder) fit(blocks []summaryBlock, budget int) (string, error) {
	text := renderBlocks(blocks, false)
	if len(text) <= budget {
		return text, nil
	}
	runs := packBlocks(blocks, budget)
	f.note(fmt.Sprintf("fold: input of %d chars over the window of %d; summarizing %d chunks", len(text), budget, len(runs)))
	parts := make([]summaryBlock, 0, len(runs))
	for i, run := range runs {
		s, err := f.chunk(run, false, budget)
		if err != nil {
			return "", err
		}
		parts = append(parts, summaryBlock{text: fmt.Sprintf("PART %d OF %d:\n%s", i+1, len(runs), s)})
	}
	if len(renderBlocks(parts, false)) >= len(text) {
		f.note("fold: chunk summaries are not shorter than their input")
		return "", errNoFit
	}
	return f.fit(parts, budget)
}

// chunk summarizes one run of blocks. A run over the budget is not sent. A run
// the summarizer refuses is halved while it is over foldFloorChars. At the
// floor its tool results are cut to toolFloorCap characters of head and the
// run is sent once more. Any other call failure is returned as it is.
func (f folder) chunk(blocks []summaryBlock, atFloor bool, budget int) (string, error) {
	text := renderBlocks(blocks, atFloor)
	var reply string
	err := errSummaryOverflow
	if len(text) <= budget {
		reply, err = f.sum.Summarize(f.model, foldPrompt(f.instructions), partUser(text))
	}
	switch {
	case err == nil:
		s := strings.TrimSpace(dropMemoryBlocks(reply))
		if s == "" {
			return "", errEmptyChunk
		}
		return s, nil
	case !errors.Is(err, errSummaryOverflow):
		return "", err
	case !atFloor && len(blocks) > 1 && len(text) > foldFloorChars:
		f.note(fmt.Sprintf("fold: chunk of %d chars over the window: halved", len(text)))
		left, right := halveBlocks(blocks)
		a, err := f.chunk(left, false, budget)
		if err != nil {
			return "", err
		}
		b, err := f.chunk(right, false, budget)
		if err != nil {
			return "", err
		}
		return a + "\n\n" + b, nil
	case !atFloor && longResult(blocks):
		f.note(fmt.Sprintf("fold: chunk of %d chars at the floor: tool results cut to %d chars of head", len(text), toolFloorCap))
		return f.chunk(blocks, true, budget)
	default:
		f.note(fmt.Sprintf("fold: chunk of %d chars still over the window at the floor", len(text)))
		return "", errNoFit
	}
}

// packBlocks splits blocks, in order, into runs of at most budget characters.
// A block over the budget makes a run of its own.
func packBlocks(blocks []summaryBlock, budget int) [][]summaryBlock {
	var runs [][]summaryBlock
	var run []summaryBlock
	size := 0
	for _, b := range blocks {
		n := b.size()
		if len(run) > 0 && size+n > budget {
			runs = append(runs, run)
			run, size = nil, 0
		}
		run = append(run, b)
		size += n
	}
	if len(run) > 0 {
		runs = append(runs, run)
	}
	return runs
}

// halveBlocks splits a run of two or more blocks where its size first reaches
// half. Both halves are non-empty.
func halveBlocks(blocks []summaryBlock) ([]summaryBlock, []summaryBlock) {
	total := 0
	for _, b := range blocks {
		total += b.size()
	}
	cut, acc := 1, 0
	for i := 0; i < len(blocks)-1; i++ {
		acc += blocks[i].size()
		cut = i + 1
		if acc*2 >= total {
			break
		}
	}
	return blocks[:cut], blocks[cut:]
}

// longResult reports whether a tool result in blocks is over toolFloorCap.
func longResult(blocks []summaryBlock) bool {
	for _, b := range blocks {
		if len(b.result) > toolFloorCap {
			return true
		}
	}
	return false
}

// foldPrompt is the system prompt of a chunk call. The Handoff call reads the
// chunk summaries and writes the Handoff body.
func foldPrompt(instructions string) string {
	prompt := "Summarize this part of a Session transcript. Keep user requests and " +
		"corrections, decisions with their reasons, files touched, errors and any " +
		"unfinished step. Code, paths, errors and IDs stay exact. Plain text, no " +
		"fenced blocks."
	if ins := strings.TrimSpace(instructions); ins != "" {
		prompt += "\nFocus from the user for this compress:\n" + ins + "\n"
	}
	return prompt
}

// partUser is the one user message of a chunk call.
func partUser(text string) string {
	return "Part of the Session transcript:\n" + text
}

// capHeadTail keeps about n bytes of s, the head and the tail, and marks what
// it cut.
func capHeadTail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	head := headOf(s, n/2)
	tail := tailOf(s, n-n/2)
	return head + fmt.Sprintf("\n[... %d chars cut ...]\n", len(s)-len(head)-len(tail)) + tail
}

// headOf is the first n bytes of s, backed off to a whole character.
func headOf(s string, n int) string {
	for n > 0 && n < len(s) && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// tailOf is the last n bytes of s, moved on to a whole character.
func tailOf(s string, n int) string {
	i := len(s) - n
	for i < len(s) && !utf8.RuneStart(s[i]) {
		i++
	}
	return s[i:]
}
